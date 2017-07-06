// Package termlog provides facilities for logging to a terminal geared towards
// interactive use.
package termlog

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/fatih/color"
	"golang.org/x/crypto/ssh/terminal"
)

const (
	say = iota
	notice
	warn
	shout
	header
)

const defaultTimeFmt = "15:04:05: "
const indent = "  "

// A single global output Mutex, because fatih/color has a single global output
// writer
var outputMutex = sync.Mutex{}

// Palette defines the colour of output
type Palette struct {
	Timestamp *color.Color
	Say       *color.Color
	Notice    *color.Color
	Warn      *color.Color
	Shout     *color.Color
	Header    *color.Color
}

// DefaultPalette is a sensbile default palette, with the following foreground
// colours:
//
// 	Say: Terminal default
// 	Notice: Blue
// 	Warn: Yellow
// 	Shout: Red
// 	Timestamp: Cyan
var DefaultPalette = Palette{
	Say:       color.New(),
	Notice:    color.New(color.FgBlue),
	Warn:      color.New(color.FgYellow),
	Shout:     color.New(color.FgRed),
	Timestamp: color.New(color.FgCyan),
	Header:    color.New(color.FgBlue),
}

// Logger logs things
type Logger interface {
	Say(format string, args ...interface{})
	Notice(format string, args ...interface{})
	Warn(format string, args ...interface{})
	Shout(format string, args ...interface{})

	SayAs(name string, format string, args ...interface{})
	NoticeAs(name string, format string, args ...interface{})
	WarnAs(name string, format string, args ...interface{})
	ShoutAs(name string, format string, args ...interface{})
}

// Group is a collected group of log entries. Logs are only displayed once the
// Done method is called.
type Group interface {
	Logger
	Done()
	Quiet()
}

// Stream is a stream of log entries with a header
type Stream interface {
	Logger
	Quiet()
	Header()
}

// TermLog is the top-level termlog interface
type TermLog interface {
	Logger
	Group() Group
	Stream(header string) Stream
	Quiet()
}

type linesource interface {
	getID() string
	getHeader() string
}

type line struct {
	name   string
	str    string
	source linesource
}

// Log is the top-level log structure
type Log struct {
	Palette *Palette
	TimeFmt string
	enabled map[string]bool
	quiet   bool
	lastid  string
}

// NewLog creates a new Log instance and initialises it with a set of defaults.
func NewLog() *Log {
	l := &Log{
		Palette: &DefaultPalette,
		enabled: make(map[string]bool),
		TimeFmt: defaultTimeFmt,
	}
	l.enabled[""] = true
	if !terminal.IsTerminal(int(os.Stdout.Fd())) || os.Getenv("TERM") == "dumb" {
		l.Color(false)
	}
	return l
}

func (l *Log) format(timestamp bool, level int, format string, args []interface{}) string {
	ts := ""
	if timestamp {
		f := l.Palette.Timestamp.SprintfFunc()
		ts = f(
			"%s", time.Now().Format(l.TimeFmt),
		)
	}
	var p *color.Color
	switch level {
	case say:
		p = l.Palette.Say
	case notice:
		p = l.Palette.Notice
	case warn:
		p = l.Palette.Warn
	case shout:
		p = l.Palette.Shout
	case header:
		p = l.Palette.Header
	default:
		panic("unknown log level")
	}
	return ts + p.SprintfFunc()(format, args...)
}

// Color sets the state of colour output - true to turn on, false to disable.
func (*Log) Color(state bool) {
	color.NoColor = !state
}

// Enable logging for a specified name
func (l *Log) Enable(name string) {
	l.enabled[name] = true
}

// Quiet disables all output
func (l *Log) Quiet() {
	l.quiet = true
}

func (l *Log) header(source linesource) {
	l.lastid = source.getID()
	hdr := l.format(true, header, source.getHeader(), nil)
	fmt.Fprintf(color.Output, hdr+"\n")
}

func (l *Log) output(quiet bool, lines ...*line) {
	if quiet {
		return
	}
	outputMutex.Lock()
	defer outputMutex.Unlock()
	for _, line := range lines {
		if _, ok := l.enabled[line.name]; !ok {
			continue
		}
		id := line.source.getID()
		if id != "" && id != l.lastid {
			l.header(line.source)
		}
		fmt.Fprintf(color.Output, "%s\n", line.str)
	}
}

// Say logs a line
func (l *Log) Say(format string, args ...interface{}) {
	l.output(l.quiet, &line{"", l.format(true, say, format, args), l})
}

// Notice logs a line with the Notice color
func (l *Log) Notice(format string, args ...interface{}) {
	l.output(l.quiet, &line{"", l.format(true, notice, format, args), l})
}

// Warn logs a line with the Warn color
func (l *Log) Warn(format string, args ...interface{}) {
	l.output(l.quiet, &line{"", l.format(true, warn, format, args), l})
}

// Shout logs a line with the Shout color
func (l *Log) Shout(format string, args ...interface{}) {
	l.output(l.quiet, &line{"", l.format(true, shout, format, args), l})
}

// SayAs logs a line
func (l *Log) SayAs(name string, format string, args ...interface{}) {
	l.output(l.quiet, &line{name, l.format(true, say, format, args), l})
}

// NoticeAs logs a line with the Notice color
func (l *Log) NoticeAs(name string, format string, args ...interface{}) {
	l.output(l.quiet, &line{name, l.format(true, notice, format, args), l})
}

// WarnAs logs a line with the Warn color
func (l *Log) WarnAs(name string, format string, args ...interface{}) {
	l.output(l.quiet, &line{name, l.format(true, warn, format, args), l})
}

// ShoutAs logs a line with the Shout color
func (l *Log) ShoutAs(name string, format string, args ...interface{}) {
	l.output(l.quiet, &line{name, l.format(true, shout, format, args), l})
}

// Group creates a new log group
func (l *Log) Group() Group {
	return &group{
		lines: make([]*line, 0),
		log:   l,
		quiet: l.quiet,
	}
}

// Stream creates a new log group
func (l *Log) Stream(header string) Stream {
	return &stream{
		header: header,
		log:    l,
		quiet:  l.quiet,
	}
}

func (l *Log) getID() string {
	return ""
}

func (l *Log) getHeader() string {
	return ""
}

// SetOutput sets the output writer for termlog (stdout by default).
func SetOutput(w io.Writer) {
	color.Output = w
}
