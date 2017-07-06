package termlog

import (
	"bytes"
	"strings"
	"testing"

	"golang.org/x/net/context"
)

func tstring(t *testing.T, buff *bytes.Buffer, s string) {
	l, err := buff.ReadString('\n')
	if err != nil {
		t.Error(err)
	}
	if !strings.HasSuffix(l, s+"\n") {
		t.Errorf("Expected string to end with %s, found %s", s, l)
	}
}

func TestBasic(t *testing.T) {
	buff := new(bytes.Buffer)
	SetOutput(buff)
	l := NewLog()
	l.Color(false)
	l.Enable("on")
	l.Say("say")
	l.Notice("notice")
	l.Warn("warn")
	l.Shout("shout")

	// Not enabled
	l.SayAs("off", "off")
	l.SayAs("on", "on - say")
	l.NoticeAs("on", "on - notice")
	l.WarnAs("on", "on - warn")
	l.ShoutAs("on", "on - shout")

	tstring(t, buff, "say")
	tstring(t, buff, "notice")
	tstring(t, buff, "warn")
	tstring(t, buff, "shout")

	tstring(t, buff, "on - say")
	tstring(t, buff, "on - notice")
	tstring(t, buff, "on - warn")
	tstring(t, buff, "on - shout")
}

func TestStream(t *testing.T) {
	buff := new(bytes.Buffer)
	SetOutput(buff)
	l := NewLog()
	l.Color(false)
	s1 := l.Stream("header a")
	s2 := l.Stream("header b")
	if s1.(*stream).getID() == s2.(*stream).getID() {
		t.Fail()
	}

	s1.Say("hello 1")
	s1.Say("hello again")
	tstring(t, buff, "header a")
	tstring(t, buff, "hello 1")
	tstring(t, buff, "hello again")
	s2.Say("hello 2")
	tstring(t, buff, "header b")
	tstring(t, buff, "hello 2")

	s1.SayAs("debug", "debug 1")
	s2.Say("normal")
	tstring(t, buff, "normal")

	l.Enable("on")
	s1.Say("say")
	s1.Notice("notice")
	s1.Warn("warn")
	s1.Shout("shout")

	// Not enabled
	s1.SayAs("off", "off")
	s1.SayAs("on", "on - say")
	s1.NoticeAs("on", "on - notice")
	s1.WarnAs("on", "on - warn")
	s1.ShoutAs("on", "on - shout")

	tstring(t, buff, "header a")
	tstring(t, buff, "say")
	tstring(t, buff, "notice")
	tstring(t, buff, "warn")
	tstring(t, buff, "shout")

	tstring(t, buff, "on - say")
	tstring(t, buff, "on - notice")
	tstring(t, buff, "on - warn")
	tstring(t, buff, "on - shout")

	s2.Quiet()
	s2.Say("foo")
	s1.Say("bar")
	tstring(t, buff, "bar")
}

func TestGroup(t *testing.T) {
	buff := new(bytes.Buffer)
	SetOutput(buff)
	l := NewLog()
	l.Enable("on")

	g1 := l.Group()
	g2 := l.Group()
	g3 := l.Group()

	// Groups can be silenced
	g4 := l.Group()
	g4.Quiet()
	g4.Say("quiet")
	g4.Done()

	// Groups can be empty
	g5 := l.Group()
	g5.Done()

	g1.Say("g1 - say")
	g2.Say("g2 - say")
	g3.SayAs("on", "on - g2 - say")
	g3.SayAs("off", "off")
	g1.Notice("g1 - notice")
	g2.Notice("g2 - notice")
	g3.NoticeAs("on", "on - g2 - notice")
	g1.Warn("g1 - warn")
	g2.Warn("g2 - warn")
	g3.WarnAs("on", "on - g2 - warn")
	g1.Shout("g1 - shout")
	g2.Shout("g2 - shout")
	g3.ShoutAs("on", "on - g2 - shout")

	g1.Done()
	g2.Done()
	g3.Done()

	tstring(t, buff, "g1 - say")
	tstring(t, buff, "g1 - notice")
	tstring(t, buff, "g1 - warn")
	tstring(t, buff, "g1 - shout")
	tstring(t, buff, "g2 - say")
	tstring(t, buff, "g2 - notice")
	tstring(t, buff, "g2 - warn")
	tstring(t, buff, "g2 - shout")

	tstring(t, buff, "on - g2 - say")
	tstring(t, buff, "on - g2 - notice")
	tstring(t, buff, "on - g2 - warn")
	tstring(t, buff, "on - g2 - shout")
}

func TestContext(t *testing.T) {
	ctx := context.Background()
	// Silenced log
	e := FromContext(ctx)
	e.Shout("nothing")

	l := NewLog()
	n := NewContext(ctx, l)
	b := FromContext(n)
	b.Shout("something")
}
