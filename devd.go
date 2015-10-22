package main

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
	"github.com/toqueteos/webbrowser"
	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/cortesi/devd/inject"
	"github.com/cortesi/devd/livereload"
	"github.com/cortesi/devd/ricetemp"
	"github.com/cortesi/devd/slowdown"
	"github.com/cortesi/devd/termlog"
	"github.com/cortesi/devd/timer"
)

const (
	defaultDomain = "devd.io"
	version       = "0.1"
	portLow       = 8000
	portHigh      = 10000
)

var (
	injectLivereload = inject.CopyInject{
		Within:  1024 * 5,
		Marker:  regexp.MustCompile(`<\/head>`),
		Payload: []byte(`<script src="/livereload.js"></script>`),
	}
)

var ()

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

func devdHandler(log termlog.Logger, route Route, templates *template.Template, logHeaders bool, ignoreHeaders []*regexp.Regexp, livereload bool, latency int) http.Handler {
	ci := inject.CopyInject{}
	if livereload {
		ci = injectLivereload
	}
	next := route.Endpoint.Handler(templates, ci)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sublog termlog.Logger
		if matchStringAny(ignoreHeaders, fmt.Sprintf("%s%s", route.Host, r.RequestURI)) {
			sublog = termlog.DummyLogger{}
		} else {
			sublog = log.Group()
		}
		timr := timer.Timer{}
		defer func() {
			timing := termlog.DefaultPalette.Timestamp.SprintFunc()("timing: ")
			sublog.SayAs(
				"timer",
				timing+timr.String(),
			)
			sublog.Done()
		}()
		timr.RequestHeaders()
		time.Sleep(time.Millisecond * time.Duration(latency))
		sublog.Say("%s %s", r.Method, r.URL)
		if logHeaders {
			LogHeader(sublog, r.Header)
		}
		ctx := timr.NewContext(context.Background())
		ctx = termlog.NewContext(ctx, sublog)
		next.ServeHTTPContext(
			ctx,
			&ResponseLogWriter{
				log:        sublog,
				rw:         w,
				timr:       &timr,
				logHeaders: logHeaders,
			},
			r,
		)
	})
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

func main() {
	httpIP := kingpin.Flag("address", "Address to listen on").
		Short('A').
		Default("127.0.0.1").
		String()

	allInterfaces := kingpin.Flag("a", "Listen on all addresses").
		Short('a').
		Bool()

	certFile := kingpin.Flag("cert", "Certificate bundle file - enables TLS").
		Short('c').
		PlaceHolder("PATH").
		Default("").
		ExistingFile()

	throttleDownKbps := kingpin.Flag(
		"down",
		"Throttle downstream from the client to N kilobytes per second",
	).
		PlaceHolder("N").
		Short('d').
		Default("0").
		Int()

	logHeaders := kingpin.Flag("logheaders", "Log headers").
		Short('H').
		Default("false").
		Bool()

	ignoreHeaders := kingpin.Flag(
		"ignore",
		"Disable logging matching requests. Regexes are matched over 'host/path'",
	).
		Short('I').
		PlaceHolder("REGEX").
		Strings()

	livereloadRoutes := kingpin.Flag("livereload", "Enable livereload for static files").
		Short('l').
		Default("false").
		Bool()

	latency := kingpin.Flag("latency", "Add N milliseconds of round-trip latency").
		PlaceHolder("N").
		Short('n').
		Default("0").
		Int()

	openBrowser := kingpin.Flag("open", "Open browser window on startup").
		Short('o').
		Default("false").
		Bool()

	httpPort := kingpin.Flag(
		"port",
		"Port to listen on - if not specified, devd will auto-pick a sensible port",
	).
		Short('p').
		Int()

	enableTimer := kingpin.Flag("logtime", "Log timing").
		Short('T').
		Default("false").
		Bool()

	throttleUpKbps := kingpin.Flag(
		"up",
		"Throttle upstream from the client to N kilobytes per second",
	).
		PlaceHolder("N").
		Short('u').
		Default("0").
		Int()

	watch := kingpin.Flag("watch", "Watch path to trigger livereload").
		PlaceHolder("PATH").
		Short('w').
		Strings()

	debug := kingpin.Flag("debug", "Debugging for devd development").
		Default("false").
		Bool()

	routes := kingpin.Arg(
		"route",
		`Routes have the following forms:
			[SUBDOMAIN]/<PATH>=<DIR>
			[SUBDOMAIN]/<PATH>=<URL>
			<DIR>
			<URL>
		`,
	).Required().Strings()
	kingpin.Version(version)

	kingpin.Parse()

	logger := termlog.NewLog()

	if *debug {
		logger.Enable("debug")
	}
	if *enableTimer {
		logger.Enable("timer")
	}
	if *throttleDownKbps == 0 {
		*throttleDownKbps = slowdown.MaxRate
	}
	if *throttleUpKbps == 0 {
		*throttleUpKbps = slowdown.MaxRate
	}

	if *allInterfaces {
		*httpIP = "0.0.0.0"
	}

	tlsEnabled := false
	if *certFile != "" {
		tlsEnabled = true
	}

	var hl net.Listener
	var err error
	if *httpPort > 0 {
		hl, err = net.Listen("tcp", fmt.Sprintf("%v:%d", *httpIP, *httpPort))
	} else {
		hl, err = pickPort(*httpIP, portLow, portHigh, tlsEnabled)
	}
	if err != nil {
		kingpin.Fatalf("Could not bind to port: %s", err)
		return
	}

	templates := ricetemp.MustMakeTemplates(rice.MustFindBox("templates"))
	if err != nil {
		kingpin.Fatalf("Error loading templates: %s", err)
		return
	}

	ignores := make([]*regexp.Regexp, 0, 0)
	for _, expr := range *ignoreHeaders {
		v, err := regexp.Compile(expr)
		if err != nil {
			kingpin.Fatalf("%s", err)
		}
		ignores = append(ignores, v)
	}

	routeColl := make(routeCollection)
	for _, s := range *routes {
		err := routeColl.Set(s)
		if err != nil {
			kingpin.FatalUsage("Invalid route specification: %s", err)
		}
	}

	mux := http.NewServeMux()
	var livereloadEnabled = false
	if *livereloadRoutes || len(*watch) > 0 {
		livereloadEnabled = true
	}

	for match, route := range routeColl {
		handler := devdHandler(
			logger,
			route,
			templates,
			*logHeaders,
			ignores,
			livereloadEnabled,
			*latency,
		)
		mux.Handle(match, http.StripPrefix(route.Path, handler))
	}

	lr := livereload.NewServer("livereload", logger)
	if livereloadEnabled {
		mux.Handle("/livereload", lr)
		mux.Handle("/livereload.js", http.HandlerFunc(lr.ServeScript))
	}
	if *livereloadRoutes {
		err = WatchRoutes(routeColl, lr)
		if err != nil {
			kingpin.Fatalf("Could not watch routes for livereload: %s", err)
		}
	}
	if len(*watch) > 0 {
		err = WatchPaths(*watch, lr)
		if err != nil {
			kingpin.Fatalf("Could not watch path for livereload: %s", err)
		}
	}

	var tlsConfig *tls.Config
	if *certFile != "" {
		tlsConfig, err = getTLSConfig(*certFile)
		if err != nil {
			kingpin.Fatalf("Could not load certs: %s", err)
			return
		}
		hl = tls.NewListener(hl, tlsConfig)
	}
	hl = slowdown.NewSlowListener(
		hl,
		float64(*throttleUpKbps)*1024,
		float64(*throttleDownKbps)*1024,
	)

	url := formatURL(tlsEnabled, *httpIP, hl.Addr().(*net.TCPAddr).Port)
	logger.Say("Listening on %s (%s)", url, hl.Addr().String())
	if *openBrowser {
		go func() {
			webbrowser.Open(url)
		}()
	}
	server := &http.Server{
		Addr:    hl.Addr().String(),
		Handler: hostPortStrip(mux),
	}
	err = server.Serve(hl)
	logger.Shout("Server stopped: %v", err)
}
