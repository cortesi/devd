package main

import (
	"net/http"
	"os"
	"path"

	"github.com/cortesi/devd"
	"github.com/cortesi/termlog"
	"github.com/mitchellh/go-homedir"
	"github.com/toqueteos/webbrowser"
	"gopkg.in/alecthomas/kingpin.v2"
)

func main() {
	address := kingpin.Flag("address", "Address to listen on").
		Short('A').
		Default("127.0.0.1").
		String()

	allInterfaces := kingpin.Flag("all", "Listen on all addresses").
		Short('a').
		Bool()

	certFile := kingpin.Flag("cert", "Certificate bundle file - enables TLS").
		Short('c').
		PlaceHolder("PATH").
		ExistingFile()

	forceColor := kingpin.Flag("color", "Enable colour output, even if devd is not connected to a terminal").
		Short('C').
		Bool()

	downKbps := kingpin.Flag(
		"down",
		"Throttle downstream from the client to N kilobytes per second",
	).
		PlaceHolder("N").
		Short('d').
		Default("0").
		Uint()

	logHeaders := kingpin.Flag("logheaders", "Log headers").
		Short('H').
		Default("false").
		Bool()

	ignoreLogs := kingpin.Flag(
		"ignore",
		"Disable logging matching requests. Regexes are matched over 'host/path'",
	).
		Short('I').
		PlaceHolder("REGEX").
		Strings()

	livereloadNaked := kingpin.Flag("livereload", "Enable livereload").
		Short('L').
		Default("false").
		Bool()

	livereloadRoutes := kingpin.Flag("livewatch", "Enable livereload and watch for static file changes").
		Short('l').
		Default("false").
		Bool()

	moddMode := kingpin.Flag("modd", "Modd is our parent - synonym for -LCt").
		Short('m').
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

	port := kingpin.Flag(
		"port",
		"Port to listen on - if not specified, devd will auto-pick a sensible port",
	).
		Short('p').
		Int()

	credspec := kingpin.Flag(
		"password",
		"HTTP basic password protection",
	).
		PlaceHolder("USER:PASS").
		Short('P').
		String()

	quiet := kingpin.Flag("quiet", "Silence all logs").
		Short('q').
		Default("false").
		Bool()

	tls := kingpin.Flag("tls", "Serve TLS with auto-generated self-signed certificate (~/.devd.cert)").
		Short('s').
		Default("false").
		Bool()

	noTimestamps := kingpin.Flag("notimestamps", "Disable timestamps in output").
		Short('t').
		Default("false").
		Bool()

	logTime := kingpin.Flag("logtime", "Log timing").
		Short('T').
		Default("false").
		Bool()

	upKbps := kingpin.Flag(
		"up",
		"Throttle upstream from the client to N kilobytes per second",
	).
		PlaceHolder("N").
		Short('u').
		Default("0").
		Uint()

	watch := kingpin.Flag("watch", "Watch path to trigger livereload").
		PlaceHolder("PATH").
		Short('w').
		Strings()

	cors := kingpin.Flag("crossdomain", "Set the CORS header Access-Control-Allowed: *").
		Short('X').
		Default("false").
		Bool()

	excludes := kingpin.Flag("exclude", "Glob pattern for files to exclude from livereload").
		PlaceHolder("PATTERN").
		Short('x').
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

	kingpin.CommandLine.HelpFlag.Short('h')
	kingpin.Version(devd.Version)

	kingpin.Parse()

	if *moddMode {
		*forceColor = true
		*noTimestamps = true
		*livereloadNaked = true
	}

	realAddr := *address
	if *allInterfaces {
		realAddr = "0.0.0.0"
	}

	var creds *devd.Credentials
	if *credspec != "" {
		var err error
		creds, err = devd.CredentialsFromSpec(*credspec)
		if err != nil {
			kingpin.Fatalf("%s", err)
			return
		}
	}

	hdrs := make(http.Header)
	if *cors {
		hdrs.Set("Access-Control-Allow-Origin", "*")
	}

	dd := devd.Devd{
		// Shaping
		Latency:  *latency,
		DownKbps: *downKbps,
		UpKbps:   *upKbps,

		AddHeaders: &hdrs,

		// Livereload
		LivereloadRoutes: *livereloadRoutes,
		Livereload:       *livereloadNaked,
		WatchPaths:       *watch,
		Excludes:         *excludes,

		Credentials: creds,
	}

	if err := dd.AddRoutes(*routes); err != nil {
		kingpin.Fatalf("%s", err)
	}

	if err := dd.AddIgnores(*ignoreLogs); err != nil {
		kingpin.Fatalf("%s", err)
	}

	logger := termlog.NewLog()
	if *quiet {
		logger.Quiet()
	}
	if *debug {
		logger.Enable("debug")
	}
	if *logTime {
		logger.Enable("timer")
	}
	if *logHeaders {
		logger.Enable("headers")
	}
	if *forceColor {
		logger.Color(true)
	}
	if *noTimestamps {
		logger.TimeFmt = ""
	}

	for _, i := range dd.Routes {
		logger.Say("Route %s -> %s", i.MuxMatch(), i.Endpoint.String())
	}

	if *tls {
		home, err := homedir.Dir()
		if err != nil {
			kingpin.Fatalf("Could not get user's homedir: %s", err)
		}
		dst := path.Join(home, ".devd.cert")
		if _, err := os.Stat(dst); os.IsNotExist(err) {
			err := devd.GenerateCert(dst)
			if err != nil {
				kingpin.Fatalf("Could not generate cert: %s", err)
			}
		}
		*certFile = dst
	}

	err := dd.Serve(
		realAddr,
		*port,
		*certFile,
		logger,
		func(url string) {
			if *openBrowser {
				err := webbrowser.Open(url)
				if err != nil {
					kingpin.Fatalf("Failed to open browser: %s", err)
				}
			}
		},
	)
	if err != nil {
		kingpin.Fatalf("%s", err)
	}
}
