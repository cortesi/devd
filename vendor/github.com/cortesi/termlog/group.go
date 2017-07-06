package termlog

// Group is a group of lines that constitue a single log entry that won't be
// split. Lines in a group are indented.
type group struct {
	lines []*line
	quiet bool
	log   *Log
}

func (g *group) addLine(name string, level int, format string, args []interface{}) {
	ts := false
	if len(g.lines) == 0 {
		ts = true
	} else {
		format = "\t" + format
	}
	g.lines = append(
		g.lines,
		&line{name: name, str: g.log.format(ts, level, format, args), source: g},
	)
}

// Say logs a line
func (g *group) Say(format string, args ...interface{}) {
	g.addLine("", say, format, args)
}

// Notice logs a line with the Notice color
func (g *group) Notice(format string, args ...interface{}) {
	g.addLine("", notice, format, args)
}

// Warn logs a line with the Warn color
func (g *group) Warn(format string, args ...interface{}) {
	g.addLine("", warn, format, args)
}

// Shout logs a line with the Shout color
func (g *group) Shout(format string, args ...interface{}) {
	g.addLine("", shout, format, args)
}

// SayAs logs a line
func (g *group) SayAs(name string, format string, args ...interface{}) {
	g.addLine(name, say, format, args)
}

// NoticeAs logs a line with the Notice color
func (g *group) NoticeAs(name string, format string, args ...interface{}) {
	g.addLine(name, notice, format, args)
}

// WarnAs logs a line with the Warn color
func (g *group) WarnAs(name string, format string, args ...interface{}) {
	g.addLine(name, warn, format, args)
}

// ShoutAs logs a line with the Shout color
func (g *group) ShoutAs(name string, format string, args ...interface{}) {
	g.addLine(name, shout, format, args)
}

// Done outputs the group to screen
func (g *group) Done() {
	g.log.output(g.quiet, g.lines...)
}

// Quiet disables output for this subgroup
func (g *group) Quiet() {
	g.quiet = true
}

func (g *group) getID() string {
	return ""
}

func (g *group) getHeader() string {
	return ""
}
