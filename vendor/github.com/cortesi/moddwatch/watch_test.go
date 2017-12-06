package moddwatch

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/rjeczalik/notify"
)

var alwaysEqual = cmp.Comparer(func(_, _ interface{}) bool { return true })
var cmpOptions = cmp.Options{
	cmp.FilterValues(
		func(x, y interface{}) bool {
			vx, vy := reflect.ValueOf(x), reflect.ValueOf(y)
			return (vx.IsValid() && vy.IsValid() &&
				vx.Type() == vy.Type()) &&
				(vx.Kind() == reflect.Slice || vx.Kind() == reflect.Map) &&
				(vx.Len() == 0 && vy.Len() == 0)
		},
		alwaysEqual,
	),
	cmp.FilterPath(
		func(p cmp.Path) bool {
			if p.String() == "URL.RawQuery" {
				return true
			}
			return false
		},
		cmp.Comparer(func(a, b interface{}) bool {
			qa, _ := url.ParseQuery(a.(string))
			qb, _ := url.ParseQuery(b.(string))
			return cmp.Equal(qa, qb)
		}),
	),
}

// WithTempDir creates a temp directory, changes the current working directory
// to it, and returns a function that can be called to clean up. Use it like
// this:
//      defer WithTempDir(t)()
func WithTempDir(t *testing.T) func() {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("TempDir: %v", err)
	}
	tmpdir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("TempDir: %v", err)
	}
	err = os.Chdir(tmpdir)
	if err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	return func() {
		err := os.Chdir(cwd)
		if err != nil {
			t.Fatalf("Chdir: %v", err)
		}
		err = os.RemoveAll(tmpdir)
		if err != nil {
			t.Fatalf("Removing tmpdir: %s", err)
		}
	}
}

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
		ret := batch(time.Millisecond*10, MaxLullWait, tst.exists, input)
		if !reflect.DeepEqual(*ret, tst.expected) {
			t.Errorf("Test %d: expected\n%#v\ngot\n%#v", i, tst.expected, ret)
		}
	}
}

func abs(path string) string {
	wd, err := os.Getwd()
	if err != nil {
		panic("Could not get current working directory")
	}
	return filepath.ToSlash(filepath.Join(wd, path))
}

var isUnderTests = []struct {
	parent   string
	child    string
	expected bool
}{
	{"/foo", "/foo/bar", true},
	{"/foo", "/foo", true},
	{"/foo", "/foobar/bar", false},
}

func TestIsUnder(t *testing.T) {
	for i, tst := range isUnderTests {
		ret := isUnder(tst.parent, tst.child)
		if ret != tst.expected {
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
	if !reflect.DeepEqual(m.All(), []string{"add", "change"}) {
		t.Error("Unexpeced return from Mod.All")
	}

	m = Mod{
		Added:   []string{abs("add")},
		Deleted: []string{abs("rm")},
		Changed: []string{abs("change")},
	}
	if _, err := m.normPaths("."); err != nil {
		t.Error(err)
	}
}

func testListBasic(t *testing.T) {
	var findTests = []struct {
		include  []string
		exclude  []string
		expected []string
	}{
		{
			[]string{"**"},
			[]string{},
			[]string{"a/a.test1", "a/b.test2", "b/a.test1", "b/b.test2", "x", "x.test1"},
		},
		{
			[]string{"**/*.test1"},
			[]string{},
			[]string{"a/a.test1", "b/a.test1", "x.test1"},
		},
		{
			[]string{"a"},
			[]string{},
			[]string{},
		},
		{
			[]string{"x"},
			[]string{},
			[]string{"x"},
		},
		{
			[]string{"a/a.test1"},
			[]string{},
			[]string{"a/a.test1"},
		},
		{
			[]string{"**"},
			[]string{"*.test1"},
			[]string{"a/a.test1", "a/b.test2", "b/a.test1", "b/b.test2", "x"},
		},
		{
			[]string{"**"},
			[]string{"a/**"},
			[]string{"b/a.test1", "b/b.test2", "x", "x.test1"},
		},
		{
			[]string{"**"},
			[]string{"a/*"},
			[]string{"b/a.test1", "b/b.test2", "x", "x.test1"},
		},
		{
			[]string{"**"},
			[]string{"**/*.test1", "**/*.test2"},
			[]string{"x"},
		},
	}

	defer WithTempDir(t)()
	paths := []string{
		"a/a.test1",
		"a/b.test2",
		"b/a.test1",
		"b/b.test2",
		"x",
		"x.test1",
	}
	for _, p := range paths {
		dst := filepath.Join(".", p)
		err := os.MkdirAll(filepath.Dir(dst), 0777)
		if err != nil {
			t.Fatalf("Error creating test dir: %v", err)
		}
		err = ioutil.WriteFile(dst, []byte("test"), 0777)
		if err != nil {
			t.Fatalf("Error writing test file: %v", err)
		}
	}

	for i, tt := range findTests {
		ret, err := List(".", tt.include, tt.exclude)
		if err != nil {
			t.Fatal(err)
		}
		expected := tt.expected
		for i := range ret {
			ret[i] = filepath.ToSlash(ret[i])
		}
		if !reflect.DeepEqual(ret, expected) {
			t.Errorf(
				"%d: %#v, %#v - Expected\n%#v\ngot:\n%#v",
				i, tt.include, tt.exclude, expected, ret,
			)
		}
	}
}

func testList(t *testing.T) {
	var findTests = []struct {
		include  []string
		exclude  []string
		expected []string
	}{
		{
			[]string{"**"},
			[]string{},
			[]string{"a/a.test1", "a/b.test2", "a/sub/c.test2", "b/a.test1", "b/b.test2", "x", "x.test1"},
		},
		{
			[]string{"**/*.test1"},
			[]string{},
			[]string{"a/a.test1", "b/a.test1", "x.test1"},
		},
		{
			[]string{"**"},
			[]string{"*.test1"},
			[]string{"a/a.test1", "a/b.test2", "a/sub/c.test2", "b/a.test1", "b/b.test2", "x"},
		},
		{
			[]string{"**"},
			[]string{"a/**"},
			[]string{"b/a.test1", "b/b.test2", "x", "x.test1"},
		},
		{
			[]string{"**"},
			[]string{"a/**"},
			[]string{"b/a.test1", "b/b.test2", "x", "x.test1"},
		},
		{
			[]string{"**"},
			[]string{"**/*.test1", "**/*.test2"},
			[]string{"x"},
		},
		{
			[]string{"a/relsymlink"},
			[]string{},
			[]string{},
		},
		{
			[]string{"a/relfilesymlink"},
			[]string{},
			[]string{"x"},
		},
		{
			[]string{"a/relsymlink/**"},
			[]string{},
			[]string{"b/a.test1", "b/b.test2"},
		},
		{
			[]string{"a/**", "a/relsymlink/**"},
			[]string{},
			[]string{"a/a.test1", "a/b.test2", "a/sub/c.test2", "b/a.test1", "b/b.test2"},
		},
		{
			[]string{"a/abssymlink/**"},
			[]string{},
			[]string{"b/a.test1", "b/b.test2"},
		},
		{
			[]string{"a/**", "a/abssymlink/**"},
			[]string{},
			[]string{"a/a.test1", "a/b.test2", "a/sub/c.test2", "b/a.test1", "b/b.test2"},
		},
	}

	defer WithTempDir(t)()
	paths := []string{
		"a/a.test1",
		"a/b.test2",
		"a/sub/c.test2",
		"b/a.test1",
		"b/b.test2",
		"x",
		"x.test1",
	}
	for _, p := range paths {
		dst := filepath.Join(".", p)
		err := os.MkdirAll(filepath.Dir(dst), 0777)
		if err != nil {
			t.Fatalf("Error creating test dir: %v", err)
		}
		err = ioutil.WriteFile(dst, []byte("test"), 0777)
		if err != nil {
			t.Fatalf("Error writing test file: %v", err)
		}
	}
	if err := os.Symlink("../../b", "./a/relsymlink"); err != nil {
		t.Fatal(err)
		return
	}
	if err := os.Symlink("../../x", "./a/relfilesymlink"); err != nil {
		t.Fatal(err)
		return
	}

	sabs, err := filepath.Abs("./b")
	if err != nil {
		t.Fatal(err)
		return
	}
	if err = os.Symlink(sabs, "./a/abssymlink"); err != nil {
		t.Fatal(err)
		return
	}

	for i, tt := range findTests {
		t.Run(
			fmt.Sprintf("%.3d", i),
			func(t *testing.T) {
				ret, err := List(".", tt.include, tt.exclude)
				if err != nil {
					t.Fatal(err)
				}
				expected := tt.expected
				for i := range ret {
					if filepath.IsAbs(ret[i]) {
						wd, err := os.Getwd()
						rel, err := filepath.Rel(wd, filepath.ToSlash(ret[i]))
						if err != nil {
							t.Fatal(err)
							return
						}
						ret[i] = rel
					} else {
						ret[i] = filepath.ToSlash(ret[i])
					}
				}
				if !reflect.DeepEqual(ret, expected) {
					t.Errorf(
						"%d: %#v, %#v - Expected\n%#v\ngot:\n%#v",
						i, tt.include, tt.exclude, expected, ret,
					)
				}
			},
		)
	}
}

func TestList(t *testing.T) {
	testListBasic(t)
	if runtime.GOOS != "windows" {
		testList(t)
	}
}

const timeout = 2 * time.Second

func wait(p string) {
	p = filepath.FromSlash(p)
	for {
		_, err := os.Stat(p)
		if err != nil {
			continue
		} else {
			break
		}
	}
}

func touch(p string) {
	p = filepath.FromSlash(p)
	d := filepath.Dir(p)
	err := os.MkdirAll(d, 0777)
	if err != nil {
		panic(err)
	}

	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0777)
	if err != nil {
		panic(err)
	}
	if _, err := f.Write([]byte("teststring")); err != nil {
		panic(err)
	}
	if err := f.Close(); err != nil {
		panic(err)
	}
	ioutil.ReadFile(p)
}

func events(p string) []string {
	parts := []string{}
	for _, p := range strings.Split(p, "\n") {
		if strings.HasPrefix(p, ":") {
			p = strings.TrimSpace(p)
			if !strings.HasSuffix(p, ":") {
				parts = append(parts, strings.TrimSpace(p))
			}
		}
	}
	return parts
}

func _testWatch(
	t *testing.T,
	modfunc func(),
	includes []string,
	excludes []string,
	expected Mod,
) {
	defer WithTempDir(t)()

	err := os.MkdirAll("a", 0777)
	if err != nil {
		t.Fatal(err)
	}

	err = os.MkdirAll("b", 0777)
	if err != nil {
		t.Fatal(err)
	}

	ch := make(chan *Mod, 1024)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
		return
	}
	watcher, err := Watch(
		cwd,
		includes,
		excludes,
		time.Millisecond*200,
		ch,
	)
	if err != nil {
		t.Fatal(err)
		return
	}
	defer watcher.Stop()
	go func() {
		time.Sleep(2 * time.Second)
		watcher.Stop()
	}()

	// There's some race condition in rjeczalik/notify. If we don't wait a bit
	// here, we sometimes don't receive notifications for the initial event.
	go func() {
		touch("a/initial")
	}()
	for {
		evt, more := <-ch
		if !more {
			t.Errorf("Never saw initial sync event")
			return
		}
		if cmp.Equal(evt.Added, []string{"a/initial"}) {
			break
		} else {
			t.Errorf("Unexpected initial sync event:\n%#v", evt)
			return
		}
	}

	go modfunc()
	ret := Mod{}
	for {
		evt, more := <-ch
		if more {
			ret = ret.Join(*evt)
			if cmp.Equal(ret, expected, cmpOptions) {
				return
			}
		} else {
			break
		}
	}
	t.Errorf("Never saw expected result, did see\n%s", ret)
}

func TestWatch(t *testing.T) {
	t.Run(
		"simple",
		func(t *testing.T) {
			_testWatch(
				t,
				func() {
					touch("a/touched")
					touch("a/initial")
				},
				[]string{"**"},
				[]string{},
				Mod{
					Added:   []string{"a/touched"},
					Changed: []string{"a/initial"},
				},
			)
		},
	)
	t.Run(
		"direct",
		func(t *testing.T) {
			_testWatch(
				t,
				func() {
					touch("a/direct")
				},
				[]string{"a/initial", "a/direct"},
				[]string{},
				Mod{
					Added: []string{"a/direct"},
				},
			)
		},
	)
	t.Run(
		"directprexisting",
		func(t *testing.T) {
			_testWatch(
				t,
				func() {
					touch("a/initial")
				},
				[]string{"a/initial"},
				[]string{},
				Mod{
					Changed: []string{"a/initial"},
				},
			)
		},
	)
	t.Run(
		"deepdirect",
		func(t *testing.T) {
			// On Linux, We can't currently pick up changes within directories
			// created after the watch started. See here for more:
			//
			// https://github.com/cortesi/modd/issues/44
			if runtime.GOOS != "linux" {
				_testWatch(
					t,
					func() {
						touch("a/deep/directory/direct")
					},
					[]string{"a/initial", "a/deep/directory/direct"},
					[]string{},
					Mod{
						Added: []string{"a/deep/directory/direct"},
					},
				)
			}
		},
	)
}
