package main

import (
	"github.com/cortesi/devd"
	"gopkg.in/alecthomas/kingpin.v2"
)

func main() {
	address := kingpin.Flag("address", "Address to listen on").
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

	port := kingpin.Flag(
		"port",
		"Port to listen on - if not specified, devd will auto-pick a sensible port",
	).
		Short('p').
		Int()

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

	debug := kingpin.Flag("debug", "Debugging for devd development").
		Default("false").
		Bool()

	excludes := kingpin.Flag("exclude", "Glob pattern for files to exclude from livereload.").
		PlaceHolder("PATTERN").
		Short('x').
		Strings()

	routes := kingpin.Arg(
		"route",
		`Routes have the following forms:
			[SUBDOMAIN]/<PATH>=<DIR>
			[SUBDOMAIN]/<PATH>=<URL>
			<DIR>
			<URL>
		`,
	).Required().Strings()

	kingpin.Version(devd.Version)

	kingpin.Parse()

	realAddr := *address
	if *allInterfaces {
		realAddr = "0.0.0.0"
	}

	dd := devd.Devd{
		Routes:      *routes,
		OpenBrowser: *openBrowser,
		CertFile:    *certFile,
		Address:     realAddr,
		Port:        *port,

		// Shaping
		Latency:  *latency,
		DownKbps: *downKbps,
		UpKbps:   *upKbps,

		// Livereload
		LivereloadRoutes: *livereloadRoutes,
		Watch:            *watch,
		Excludes:         *excludes,

		// Logging
		Debug:      *debug,
		LogHeaders: *logHeaders,
		LogTime:    *logTime,
		IgnoreLogs: *ignoreLogs,
	}
	err := dd.Serve()
	if err != nil {
		kingpin.Fatalf("%s", err)
	}
}
