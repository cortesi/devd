package watch

import (
	"fmt"
	"reflect"
	"sort"
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
	expected []string
}{
	{
		[]TEventInfo{
			TEventInfo{notify.Create, "foo"},
			TEventInfo{notify.Create, "bar"},
		},
		exists("foo", "bar"),
		[]string{"foo", "bar"},
	},
	{
		[]TEventInfo{
			TEventInfo{notify.Create, "foo"},
		},
		exists(),
		[]string(nil),
	},
}

func TestBatch(t *testing.T) {
	for i, tst := range batchTests {
		input := make(chan notify.EventInfo, len(tst.events))
		for _, e := range tst.events {
			input <- e
		}
		ret := batch(0, tst.exists, input)
		sort.Strings(ret)
		sort.Strings(tst.expected)
		if !reflect.DeepEqual(ret, tst.expected) {
			fmt.Println(cap(ret), cap(tst.expected))
			t.Errorf("Test %d: expected %#v, got %#v", i, tst.expected, ret)
		}
	}
}
