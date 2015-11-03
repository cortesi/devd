package modd

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/rjeczalik/notify"
)

type TEventInfo struct {
	event notify.Event
	path  string
}

func (te TEventInfo) Path() string {
	return te.path
}

func (te TEventInfo) Event() notify.Event {
	return te.event
}

func (te TEventInfo) Sys() interface{} {
	return nil
}

type testExistenceChecker struct {
	paths map[string]bool
}

func (e *testExistenceChecker) Check(p string) bool {
	_, ok := e.paths[p]
	return ok
}

func exists(paths ...string) *testExistenceChecker {
	et := testExistenceChecker{make(map[string]bool)}
	for _, p := range paths {
		et.paths[p] = true
	}
	return &et
}

var batchTests = []struct {
	events   []TEventInfo
	exists   *testExistenceChecker
	expected Mod
}{
	{
		[]TEventInfo{
			TEventInfo{notify.Create, "foo"},
			TEventInfo{notify.Create, "bar"},
		},
		exists("bar", "foo"),
		Mod{Added: []string{"bar", "foo"}},
	},
	{
		[]TEventInfo{
			TEventInfo{notify.Rename, "foo"},
			TEventInfo{notify.Rename, "bar"},
		},
		exists("foo"),
		Mod{Added: []string{"foo"}, Deleted: []string{"bar"}},
	},
	{
		[]TEventInfo{
			TEventInfo{notify.Write, "foo"},
		},
		exists("foo"),
		Mod{Changed: []string{"foo"}},
	},
	{
		[]TEventInfo{
			TEventInfo{notify.Write, "foo"},
			TEventInfo{notify.Remove, "foo"},
		},
		exists(),
		Mod{Deleted: []string{"foo"}},
	},
	{
		[]TEventInfo{
			TEventInfo{notify.Remove, "foo"},
		},
		exists("foo"),
		Mod{},
	},
	{
		[]TEventInfo{
			TEventInfo{notify.Create, "foo"},
			TEventInfo{notify.Create, "bar"},
			TEventInfo{notify.Remove, "bar"},
		},
		exists("bar", "foo"),
		Mod{Added: []string{"bar", "foo"}},
	},
	{
		[]TEventInfo{
			TEventInfo{notify.Create, "foo"},
		},
		exists(),
		Mod{},
	},
}

func TestBatch(t *testing.T) {
	for i, tst := range batchTests {
		input := make(chan notify.EventInfo, len(tst.events))
		for _, e := range tst.events {
			input <- e
		}
		ret := batch(".", 0, tst.exists, input)
		if !reflect.DeepEqual(*ret, tst.expected) {
			t.Errorf("Test %d: expected\n%#v\ngot\n%#v", i, tst.expected, *ret)
		}
	}
}

func abs(path string) string {
	wd, err := os.Getwd()
	if err != nil {
		panic("Could not get current working directory")
	}
	return filepath.Join(wd, path)
}

var normPathTests = []struct {
	base     string
	abspath  string
	expected string
}{
	{"./tmp", abs("./tmp/bar"), "tmp/bar"},
	{abs("./tmp"), abs("./tmp/bar"), abs("tmp/bar")},
	{"tmp", abs("tmp/bar"), "tmp/bar"},
}

func TestNormPath(t *testing.T) {
	for i, tst := range normPathTests {
		ret, err := normPath(tst.base, tst.abspath)
		if err != nil || ret != tst.expected {
			t.Errorf("Test %d: expected %#v, got %#v", i, tst.expected, ret)
		}
	}
}

func TestMod(t *testing.T) {
	if !(Mod{}.Empty()) {
		t.Error("Expected mod to be empty.")
	}
	m := Mod{
		Added:   []string{"add"},
		Deleted: []string{"rm"},
		Changed: []string{"change"},
	}
	if m.Empty() {
		t.Error("Expected mod not to be empty")
	}
	if !reflect.DeepEqual(m.All(), []string{"add", "change", "rm"}) {
		t.Error("Unexpeced return from Mod.All")
	}

	m = Mod{
		Added:   []string{abs("add")},
		Deleted: []string{abs("rm")},
		Changed: []string{abs("change")},
	}
	if err := m.normPaths("/"); err != nil {
		t.Error(err)
	}
}
