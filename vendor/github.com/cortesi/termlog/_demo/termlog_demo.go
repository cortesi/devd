package main

import (
	"github.com/cortesi/termlog"
	"github.com/fatih/color"
)

func testpatt(l termlog.Logger) {
	l.Say("Log")
	l.Notice("Notice!")
	l.Warn("Warn!")
	l.Shout("Error!")
}

func main() {
	l := termlog.NewLog()
	testpatt(l)

	g := l.Group()
	g.Say("This is a group...")
	testpatt(g)
	g.Done()

	g = l.Group()
	g.Say("Here are some composed colours...")
	g.Say(
		"%s %s %s",
		color.RedString("red"),
		color.GreenString("green"),
		color.BlueString("blue"),
	)
	g.Done()

	l.Enable("demo")
	g = l.Group()
	g.SayAs("disabled", "These don't show...")
	g.SayAs("demo", "Some named log entries...")
	g.SayAs("demo", "This is a test")
	g.SayAs("disabled", "This is a test")
	g.Done()

	g = l.Group()
	g.Say("Disabled colour output...")
	l.NoColor()
	testpatt(g)
	g.Done()
}
