package devd

import (
	"crypto/tls"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"golang.org/x/net/context"

	"github.com/GeertJohan/go.rice"
	"github.com/goji/httpauth"

	"github.com/cortesi/devd/httpctx"
	"github.com/cortesi/devd/inject"
	"github.com/cortesi/devd/livereload"
	"github.com/cortesi/devd/ricetemp"
	"github.com/cortesi/devd/slowdown"
	"github.com/cortesi/devd/timer"
	"github.com/cortesi/termlog"
)

const (
	// Version is the current version of devd
	Version  = "0.6"
	portLow  = 8000
	portHigh = 10000
)

func pickPort(addr string, low int, high int, tls bool) (net.Listener, error) {
	firstTry := 80
	if tls {
		firstTry = 443
	}
	hl, err := net.Listen("tcp", fmt.Sprintf("%v:%d", addr, firstTry))
	if err == nil {
		return hl, nil
	}
	for i := low; i < high; i++ {
		hl, err := net.Listen("tcp", fmt.Sprintf("%v:%d", addr, i))
		if err == nil {
			return hl, nil
		}
	}
	return nil, fmt.Errorf("Could not find open port.")
}

func getTLSConfig(path string) (t *tls.Config, err error) {
	config := &tls.Config{}
	if config.NextProtos == nil {
		config.NextProtos = []string{"http/1.1"}
	}
	config.Certificates = make([]tls.Certificate, 1)
	config.Certificates[0], err = tls.LoadX509KeyPair(path, path)
	if err != nil {
		return nil, err
	}
	return config, nil
}

// This filthy hack works in conjunction with hostPortStrip to restore the
// original request host after mux match.
func revertOriginalHost(r *http.Request) {
	original := r.Header.Get("_devd_original_host")
	if original != "" {
		r.Host = original
		r.Header.Del("_devd_original_host")
	}
}

// We can remove the mangling once this is fixed:
// 		https://github.com/golang/go/issues/10463
func hostPortStrip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err == nil {
			original := r.Host
			r.Host = host
			r.Header.Set("_devd_original_host", original)
		}
		next.ServeHTTP(w, r)
	})
}

func matchStringAny(regexps []*regexp.Regexp, s string) bool {
	for _, r := range regexps {
		if r.MatchString(s) {
			return true
		}
	}
	return false
}

func formatURL(tls bool, httpIP string, port int) string {
	proto := "http"
	if tls {
		proto = "https"
	}
	host := httpIP
	if httpIP == "0.0.0.0" || httpIP == "127.0.0.1" {
		host = "devd.io"
	}
	if port == 443 && tls {
		return fmt.Sprintf("https://%s", host)
	}
	if port == 80 && !tls {
		return fmt.Sprintf("http://%s", host)
	}
	return fmt.Sprintf("%s://%s:%d", proto, host, port)
}

// Credentials is a simple username/password pair
type Credentials struct {
	username string
	password string
}

// CredentialsFromSpec creates a set of credentials from a spec
func CredentialsFromSpec(spec string) (*Credentials, error) {
	parts := strings.SplitN(spec, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("Invalid credential spec: %s", spec)
	}
	return &Credentials{parts[0], parts[1]}, nil
}

// Devd represents the devd server options
type Devd struct {
	Routes RouteCollection

	// Shaping
	Latency  int
	DownKbps uint
	UpKbps   uint

	// Add headers
	AddHeaders *http.Header

	// Livereload and watch static routes
	LivereloadRoutes bool
	// Livereload, but don't watch static routes
	Livereload bool
	WatchPaths []string
	Excludes   []string

	// Logging
	IgnoreLogs []*regexp.Regexp

	// Password protection
	Credentials *Credentials

	lrserver *livereload.Server
}

// WrapHandler wraps an httpctx.Handler in the paraphernalia needed by devd for
// logging, latency, and so forth.
func (dd *Devd) WrapHandler(log termlog.TermLog, next httpctx.Handler) http.Handler {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		revertOriginalHost(r)
		timr := timer.Timer{}
		sublog := log.Group()
		defer func() {
			timing := termlog.DefaultPalette.Timestamp.SprintFunc()("timing: ")
			sublog.SayAs("timer", timing+timr.String())
			sublog.Done()
		}()
		if matchStringAny(dd.IgnoreLogs, fmt.Sprintf("%s%s", r.URL.Host, r.RequestURI)) {
			sublog.Quiet()
		}
		timr.RequestHeaders()
		time.Sleep(time.Millisecond * time.Duration(dd.Latency))

		dpath := r.URL.String()
		if !strings.HasPrefix(dpath, "/") {
			dpath = "/" + dpath
		}
		sublog.Say("%s %s", r.Method, dpath)
		LogHeader(sublog, r.Header)
		ctx := timr.NewContext(context.Background())
		ctx = termlog.NewContext(ctx, sublog)
		if dd.AddHeaders != nil {
			for h, vals := range *dd.AddHeaders {
				for _, v := range vals {
					w.Header().Set(h, v)
				}
			}
		}
		next.ServeHTTPContext(
			ctx,
			&ResponseLogWriter{Log: sublog, Resp: w, Timer: &timr},
			r,
		)
	})
	return h
}

// HasLivereload tells us if livereload is enabled
func (dd *Devd) HasLivereload() bool {
	if dd.Livereload || dd.LivereloadRoutes || len(dd.WatchPaths) > 0 {
		return true
	}
	return false
}

// AddRoutes adds route specifications to the server
func (dd *Devd) AddRoutes(specs []string) error {
	dd.Routes = make(RouteCollection)
	for _, s := range specs {
		err := dd.Routes.Add(s)
		if err != nil {
			return fmt.Errorf("Invalid route specification: %s", err)
		}
	}
	return nil
}

// AddIgnores adds log ignore patterns to the server
func (dd *Devd) AddIgnores(specs []string) error {
	dd.IgnoreLogs = make([]*regexp.Regexp, 0, 0)
	for _, expr := range specs {
		v, err := regexp.Compile(expr)
		if err != nil {
			return fmt.Errorf("%s", err)
		}
		dd.IgnoreLogs = append(dd.IgnoreLogs, v)
	}
	return nil
}

// HandleNotFound handles pages not found. In particular, this handler is used
// when we have no matching route for a request. This also means it's not
// useful to inject the livereload paraphernalia here.
func HandleNotFound(templates *template.Template) httpctx.Handler {
	return httpctx.HandlerFunc(func(ctx context.Context, w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		err := templates.Lookup("404.html").Execute(w, nil)
		if err != nil {
			logger := termlog.FromContext(ctx)
			logger.Shout("Could not execute template: %s", err)
		}
	})
}

// Router constructs the main Devd router that serves all requests
func (dd *Devd) Router(logger termlog.TermLog, templates *template.Template) (http.Handler, error) {
	mux := http.NewServeMux()
	hasGlobal := false

	ci := inject.CopyInject{}
	if dd.HasLivereload() {
		ci = livereload.Injector
	}

	for match, route := range dd.Routes {
		if match == "/" {
			hasGlobal = true
		}
		handler := dd.WrapHandler(
			logger,
			route.Endpoint.Handler(templates, ci),
		)
		handler = http.StripPrefix(route.Path, handler)
		mux.Handle(match, handler)
	}
	if dd.HasLivereload() {
		lr := livereload.NewServer("livereload", logger)
		mux.Handle(livereload.EndpointPath, lr)
		mux.Handle(livereload.ScriptPath, http.HandlerFunc(lr.ServeScript))
		seen := make(map[string]bool)
		for _, route := range dd.Routes {
			if _, ok := seen[route.Host]; route.Host != "" && ok == false {
				mux.Handle(route.Host+livereload.EndpointPath, lr)
				mux.Handle(
					route.Host+livereload.ScriptPath,
					http.HandlerFunc(lr.ServeScript),
				)
				seen[route.Host] = true
			}
		}
		if dd.LivereloadRoutes {
			err := WatchRoutes(dd.Routes, lr, dd.Excludes, logger)
			if err != nil {
				return nil, fmt.Errorf("Could not watch routes for livereload: %s", err)
			}
		}
		if len(dd.WatchPaths) > 0 {
			err := WatchPaths(dd.WatchPaths, dd.Excludes, lr, logger)
			if err != nil {
				return nil, fmt.Errorf("Could not watch path for livereload: %s", err)
			}
		}
		dd.lrserver = lr
	}
	if !hasGlobal {
		mux.Handle(
			"/",
			dd.WrapHandler(logger, HandleNotFound(templates)),
		)
	}
	var h = http.Handler(mux)
	if dd.Credentials != nil {
		h = httpauth.SimpleBasicAuth(
			dd.Credentials.username, dd.Credentials.password,
		)(h)
	}
	return hostPortStrip(h), nil
}

// Serve starts the devd server. The callback is called with the serving URL
// just before service starts.
func (dd *Devd) Serve(address string, port int, certFile string, logger termlog.TermLog, callback func(string)) error {
	templates, err := ricetemp.MakeTemplates(rice.MustFindBox("templates"))
	if err != nil {
		return fmt.Errorf("Error loading templates: %s", err)
	}
	mux, err := dd.Router(logger, templates)
	if err != nil {
		return err
	}
	var tlsConfig *tls.Config
	var tlsEnabled bool
	if certFile != "" {
		tlsConfig, err = getTLSConfig(certFile)
		if err != nil {
			return fmt.Errorf("Could not load certs: %s", err)
		}
		tlsEnabled = true
	}

	var hl net.Listener
	if port > 0 {
		hl, err = net.Listen("tcp", fmt.Sprintf("%v:%d", address, port))
	} else {
		hl, err = pickPort(address, portLow, portHigh, tlsEnabled)
	}
	if err != nil {
		return err
	}

	if tlsConfig != nil {
		hl = tls.NewListener(hl, tlsConfig)
	}

	hl = slowdown.NewSlowListener(hl, dd.UpKbps*1024, dd.DownKbps*1024)
	url := formatURL(tlsEnabled, address, hl.Addr().(*net.TCPAddr).Port)
	logger.Say("Listening on %s (%s)", url, hl.Addr().String())
	server := &http.Server{Addr: hl.Addr().String(), Handler: mux}
	callback(url)

	if dd.HasLivereload() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGHUP)
		go func() {
			for {
				<-c
				logger.Say("Received signal - reloading")
				dd.lrserver.Reload([]string{"*"})
			}
		}()
	}

	err = server.Serve(hl)
	logger.Shout("Server stopped: %v", err)
	return nil
}
