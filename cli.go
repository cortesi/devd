package devd

import (
	"crypto/tls"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"regexp"
	"time"

	"golang.org/x/net/context"

	"github.com/GeertJohan/go.rice"

	"github.com/cortesi/devd/inject"
	"github.com/cortesi/devd/livereload"
	"github.com/cortesi/devd/ricetemp"
	"github.com/cortesi/devd/slowdown"
	"github.com/cortesi/devd/termlog"
	"github.com/cortesi/devd/timer"
)

const (
	// Version is the current version of devd
	Version  = "0.2"
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

// We can remove the mangling once this is fixed:
// 		https://github.com/golang/go/issues/10463
func hostPortStrip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err == nil {
			r.Host = host
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

// Devd represents the devd server options
type Devd struct {
	Routes RouteCollection

	// Shaping
	Latency  int
	DownKbps uint
	UpKbps   uint

	// Livereload
	LivereloadRoutes bool
	WatchPaths       []string
	Excludes         []string

	// Logging
	IgnoreLogs []*regexp.Regexp
}

// RouteHandler handles a single route
func (dd *Devd) RouteHandler(log termlog.Logger, route Route, templates *template.Template) http.Handler {
	ci := inject.CopyInject{}
	if dd.HasLivereload() {
		ci = livereload.Injector
	}
	next := route.Endpoint.Handler(templates, ci)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		timr := timer.Timer{}
		sublog := log.Group()
		defer func() {
			timing := termlog.DefaultPalette.Timestamp.SprintFunc()("timing: ")
			sublog.SayAs(
				"timer",
				timing+timr.String(),
			)
			sublog.Done()
		}()

		if matchStringAny(dd.IgnoreLogs, fmt.Sprintf("%s%s", route.Host, r.RequestURI)) {
			sublog.Quiet()
		}
		timr.RequestHeaders()
		time.Sleep(time.Millisecond * time.Duration(dd.Latency))
		sublog.Say("%s %s", r.Method, r.URL)
		LogHeader(sublog, r.Header)
		ctx := timr.NewContext(context.Background())
		ctx = termlog.NewContext(ctx, sublog)
		next.ServeHTTPContext(
			ctx,
			&ResponseLogWriter{
				Log:   sublog,
				Resp:  w,
				Timer: &timr,
			},
			r,
		)
	})
	return http.StripPrefix(route.Path, h)
}

// HasLivereload tells us if liverload is enabled
func (dd *Devd) HasLivereload() bool {
	if dd.LivereloadRoutes || len(dd.WatchPaths) > 0 {
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

// Handler construc5ts the Devd handler
func (dd *Devd) Handler(logger termlog.Logger, templates *template.Template) (http.Handler, error) {
	mux := http.NewServeMux()
	for match, route := range dd.Routes {
		mux.Handle(match, dd.RouteHandler(logger, route, templates))
	}
	if dd.HasLivereload() {
		lr := livereload.NewServer("livereload", logger)
		mux.Handle(livereload.EndpointPath, lr)
		mux.Handle(livereload.ScriptPath, http.HandlerFunc(lr.ServeScript))
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
	}
	return hostPortStrip(mux), nil
}

// Serve starts the devd server. The callback is called with the serving URL
// just before service starts.
func (dd *Devd) Serve(address string, port int, certFile string, logger termlog.Logger, callback func(string)) error {
	templates, err := ricetemp.MakeTemplates(rice.MustFindBox("templates"))
	if err != nil {
		return fmt.Errorf("Error loading templates: %s", err)
	}
	mux, err := dd.Handler(logger, templates)
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
		return fmt.Errorf("Could not bind to port: %s", err)
	}

	if tlsConfig != nil {
		hl = tls.NewListener(hl, tlsConfig)
	}

	hl = slowdown.NewSlowListener(hl, dd.UpKbps*1024, dd.DownKbps*1024)
	url := formatURL(tlsEnabled, address, hl.Addr().(*net.TCPAddr).Port)
	logger.Say("Listening on %s (%s)", url, hl.Addr().String())
	server := &http.Server{Addr: hl.Addr().String(), Handler: mux}
	callback(url)
	err = server.Serve(hl)
	logger.Shout("Server stopped: %v", err)
	return nil
}
