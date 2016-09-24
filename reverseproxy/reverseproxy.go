// Package reverseproxy is a reverse proxy implementation based on the built-in
// httuptil.Reverseproxy. Extensions include better logging and support for
// injection.
package reverseproxy

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/context"

	"github.com/cortesi/devd/inject"
	"github.com/cortesi/termlog"
	"github.com/dustin/go-humanize"
)

// onExitFlushLoop is a callback set by tests to detect the state of the
// flushLoop() goroutine.
var onExitFlushLoop func()

// ReverseProxy is an HTTP Handler that takes an incoming request and
// sends it to another server, proxying the response back to the
// client.
type ReverseProxy struct {
	// Director must be a function which modifies
	// the request into a new request to be sent
	// using Transport. Its response is then copied
	// back to the original client unmodified.
	Director func(*http.Request)

	// The transport used to perform proxy requests.
	// If nil, http.DefaultTransport is used.
	Transport http.RoundTripper

	// FlushInterval specifies the flush interval
	// to flush to the client while copying the
	// response body.
	// If zero, no periodic flushing is done.
	FlushInterval time.Duration

	Inject inject.CopyInject
}

func singleJoiningSlash(a, b string) string {
	if b == "" {
		return a
	}

	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	}
	return a + b
}

// NewSingleHostReverseProxy returns a new ReverseProxy that rewrites
// URLs to the scheme, host, and base path provided in target. If the
// target's path is "/base" and the incoming request was for "/dir",
// the target request will be for /base/dir.
func NewSingleHostReverseProxy(target *url.URL, ci inject.CopyInject) *ReverseProxy {
	targetQuery := target.RawQuery
	director := func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.URL.Path = singleJoiningSlash(target.Path, req.URL.Path)
		if req.Header.Get("X-Forwarded-Host") == "" {
			req.Header.Set("X-Forwarded-Host", req.Host)
		}

		// Set "identity"-only content encoding, in order for injector to
		// work on text response
		req.Header.Set("Accept-Encoding", "identity")

		req.Host = req.URL.Host
		if targetQuery == "" || req.URL.RawQuery == "" {
			req.URL.RawQuery = targetQuery + req.URL.RawQuery
		} else {
			req.URL.RawQuery = targetQuery + "&" + req.URL.RawQuery
		}
	}
	return &ReverseProxy{Director: director, Inject: ci}
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

// Hop-by-hop headers. These are removed when sent to the backend.
// http://www.w3.org/Protocols/rfc2616/rfc2616-sec13.html
var hopHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te", // canonicalized version of "TE"
	"Trailers",
	"Transfer-Encoding",
	"Upgrade",
}

// ServeHTTPContext serves HTTP with a context
func (p *ReverseProxy) ServeHTTPContext(
	ctx context.Context, rw http.ResponseWriter, req *http.Request,
) {
	log := termlog.FromContext(ctx)
	transport := p.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}

	outreq := new(http.Request)
	*outreq = *req // includes shallow copies of maps, but okay

	p.Director(outreq)
	outreq.Proto = "HTTP/1.1"
	outreq.ProtoMajor = 1
	outreq.ProtoMinor = 1
	outreq.Close = false

	// Remove hop-by-hop headers to the backend.  Especially
	// important is "Connection" because we want a persistent
	// connection, regardless of what the client sent to us.  This
	// is modifying the same underlying map from req (shallow
	// copied above) so we only copy it if necessary.
	copiedHeaders := false
	for _, h := range hopHeaders {
		if outreq.Header.Get(h) != "" {
			if !copiedHeaders {
				outreq.Header = make(http.Header)
				copyHeader(outreq.Header, req.Header)
				copiedHeaders = true
			}
			outreq.Header.Del(h)
		}
	}

	if clientIP, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
		// If we aren't the first proxy retain prior
		// X-Forwarded-For information as a comma+space
		// separated list and fold multiple headers into one.
		if prior, ok := outreq.Header["X-Forwarded-For"]; ok {
			clientIP = strings.Join(prior, ", ") + ", " + clientIP
		}
		outreq.Header.Set("X-Forwarded-For", clientIP)
	}

	res, err := transport.RoundTrip(outreq)
	if err != nil {
		log.Shout("reverse proxy error: %v", err)
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer res.Body.Close()
	if req.ContentLength > 0 {
		log.Say(fmt.Sprintf("%s uploaded", humanize.Bytes(uint64(req.ContentLength))))
	}

	inject, err := p.Inject.Sniff(res.Body)
	if err != nil {
		log.Shout("reverse proxy error: %v", err)
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}

	if inject.Found {
		cl, err := strconv.ParseInt(res.Header.Get("Content-Length"), 10, 32)
		if err == nil {
			cl = cl + int64(inject.Extra())
			res.Header.Set("Content-Length", strconv.FormatInt(cl, 10))
		}
	}
	copyHeader(rw.Header(), res.Header)
	rw.WriteHeader(res.StatusCode)
	p.copyResponse(ctx, rw, inject)
}

func (p *ReverseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.ServeHTTPContext(context.Background(), w, r)
}

func (p *ReverseProxy) copyResponse(ctx context.Context, dst io.Writer, inject *inject.Injector) {
	log := termlog.FromContext(ctx)
	if p.FlushInterval != 0 {
		if wf, ok := dst.(writeFlusher); ok {
			mlw := &maxLatencyWriter{
				dst:     wf,
				latency: p.FlushInterval,
				done:    make(chan bool),
			}
			go mlw.flushLoop()
			defer mlw.stop()
			dst = mlw
		}
	}
	_, err := inject.Copy(dst)
	if err != nil {
		log.Shout("Error forwarding data: %s", err)
	}
}

type writeFlusher interface {
	io.Writer
	http.Flusher
}

type maxLatencyWriter struct {
	sync.Mutex // protects Write + Flush

	dst     writeFlusher
	latency time.Duration

	done chan bool
}

func (m *maxLatencyWriter) Write(p []byte) (int, error) {
	m.Lock()
	defer m.Unlock()
	return m.dst.Write(p)
}

func (m *maxLatencyWriter) flushLoop() {
	t := time.NewTicker(m.latency)
	defer t.Stop()
	for {
		select {
		case <-m.done:
			if onExitFlushLoop != nil {
				onExitFlushLoop()
			}
			return
		case <-t.C:
			m.Lock()
			m.dst.Flush()
			m.Unlock()
		}
	}
}

func (m *maxLatencyWriter) stop() { m.done <- true }
