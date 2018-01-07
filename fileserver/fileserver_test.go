// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fileserver

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GeertJohan/go.rice"
	"github.com/cortesi/devd/inject"
	"github.com/cortesi/devd/ricetemp"
	"github.com/cortesi/devd/routespec"
	"github.com/cortesi/termlog"
)

// ServeFile replies to the request with the contents of the named file or directory.
func ServeFile(w http.ResponseWriter, r *http.Request, name string) {
	dir, file := filepath.Split(name)
	logger := termlog.NewLog()
	logger.Quiet()

	fs := FileServer{
		"version",
		http.Dir(dir),
		inject.CopyInject{},
		ricetemp.MustMakeTemplates(rice.MustFindBox("../templates")),
		[]routespec.RouteSpec{},
		"",
	}
	fs.serveFile(logger, w, r, file, false)
}

func ServeContent(w http.ResponseWriter, req *http.Request, name string, modtime time.Time, content io.ReadSeeker) error {
	sizeFunc := func() (int64, error) {
		size, err := content.Seek(0, os.SEEK_END)
		if err != nil {
			return 0, errSeeker
		}
		_, err = content.Seek(0, os.SEEK_SET)
		if err != nil {
			return 0, errSeeker
		}
		return size, nil
	}
	return serveContent(inject.CopyInject{}, w, req, name, modtime, sizeFunc, content)
}

const (
	testFile    = "testdata/file"
	testFileLen = 11
)

type wantRange struct {
	start, end int64 // range [start,end)
}

var itoa = strconv.Itoa

var notFoundSearchPathsSpecs = []struct {
	path   string
	spec   string
	result []string
}{
	{"/index.html", "/foo.html", []string{"/foo.html"}},
	{"/dir/index.html", "/", []string{"/"}},
	{"/dir/index.html", "foo.html", []string{"/dir/foo.html", "/foo.html"}},
	{"/", "foo.html", []string{"/foo.html"}},
	{"/", "../../foo.html", []string{"/foo.html"}},
	{"/", "/../../foo.html", []string{"/foo.html"}},
}

func TestNotFoundSearchPaths(t *testing.T) {
	for _, tt := range notFoundSearchPathsSpecs {
		paths := notFoundSearchPaths(tt.path, tt.spec)
		if !reflect.DeepEqual(paths, tt.result) {
			t.Errorf("Wanted %#v, got %#v", tt.result, paths)
		}
	}
}

var matchTypesSpecs = []struct {
	spec   string
	path   string
	result bool
}{
	{"/index.html", "/foo.png", false},
	{"/index.html", "/foo.html", true},
	{"/index/", "/foo.html", true},
	{"/index", "/foo.html", true},
	{"/index.unknown", "/foo.unknown", true},
	{"/index.html", "/foo/", true},
	{"/index.html", "/foo/bar.htm", true},
	{"/index", "/foo/bar.html", true},
	{"/index", "/foo/bar.htm", true},
	{"/index", "/foo", true},
	{"/usr/bob.foo", "/foo", true},
}

func TestMatchTypes(t *testing.T) {
	for _, tt := range matchTypesSpecs {
		m := matchTypes(tt.spec, tt.path)
		if m != tt.result {
			t.Errorf("Wanted %#v, got %#v", tt.result, m)
		}
	}
}

func TestServeFile(t *testing.T) {
	defer afterTest(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeFile(w, r, "testdata/file")
	}))
	defer ts.Close()

	var err error

	file, err := ioutil.ReadFile(testFile)
	if err != nil {
		t.Fatal("reading file:", err)
	}

	// set up the Request (re-used for all tests)
	var req http.Request
	req.Header = make(http.Header)
	if req.URL, err = url.Parse(ts.URL); err != nil {
		t.Fatal("ParseURL:", err)
	}
	req.Method = "GET"

	// straight GET
	_, body := getBody(t, "straight get", req)
	if !bytes.Equal(body, file) {
		t.Fatalf("body mismatch: got %q, want %q", body, file)
	}
}

var fsRedirectTestData = []struct {
	original, redirect string
}{
	{"/test/index.html", "/test/"},
	{"/test/testdata", "/test/testdata/"},
	{"/test/testdata/file/", "/test/testdata/file"},
}

func TestFSRedirect(t *testing.T) {
	defer afterTest(t)
	ts := httptest.NewServer(
		http.StripPrefix(
			"/test",
			&FileServer{
				"version",
				http.Dir("."),
				inject.CopyInject{},
				ricetemp.MustMakeTemplates(rice.MustFindBox("../templates")),
				[]routespec.RouteSpec{},
				"",
			},
		),
	)
	defer ts.Close()

	for _, data := range fsRedirectTestData {
		res, err := http.Get(ts.URL + data.original)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if g, e := res.Request.URL.Path, data.redirect; g != e {
			t.Errorf("redirect from %s: got %s, want %s", data.original, g, e)
		}
	}
}

type testFileSystem struct {
	open func(name string) (http.File, error)
}

func (fs *testFileSystem) Open(name string) (http.File, error) {
	return fs.open(name)
}

func _TestFileServerCleans(t *testing.T) {
	defer afterTest(t)
	ch := make(chan string, 1)
	fs := &FileServer{
		"version",
		&testFileSystem{
			func(name string) (http.File, error) {
				ch <- name
				return nil, errors.New("file does not exist")
			},
		},
		inject.CopyInject{},
		ricetemp.MustMakeTemplates(rice.MustFindBox("../templates")),
		[]routespec.RouteSpec{},
		"",
	}
	tests := []struct {
		reqPath, openArg string
	}{
		{"/foo.txt", "/foo.txt"},
		{"/../foo.txt", "/foo.txt"},
	}
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	for n, test := range tests {
		rec := httptest.NewRecorder()
		req.URL.Path = test.reqPath
		fs.ServeHTTP(rec, req)
		if got := <-ch; got != test.openArg {
			t.Errorf("test %d: got %q, want %q", n, got, test.openArg)
		}
	}
}

func mustRemoveAll(dir string) {
	err := os.RemoveAll(dir)
	if err != nil {
		panic(err)
	}
}

func TestFileServerImplicitLeadingSlash(t *testing.T) {
	defer afterTest(t)
	tempDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("TempDir: %v", err)
	}
	defer mustRemoveAll(tempDir)
	if err := ioutil.WriteFile(filepath.Join(tempDir, "foo.txt"), []byte("Hello world"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	fs := &FileServer{
		"version",
		http.Dir(tempDir),
		inject.CopyInject{},
		ricetemp.MustMakeTemplates(rice.MustFindBox("../templates")),
		[]routespec.RouteSpec{},
		"",
	}

	ts := httptest.NewServer(http.StripPrefix("/bar/", fs))
	defer ts.Close()
	get := func(suffix string) string {
		res, err := http.Get(ts.URL + suffix)
		if err != nil {
			t.Fatalf("Get %s: %v", suffix, err)
		}
		b, err := ioutil.ReadAll(res.Body)
		if err != nil {
			t.Fatalf("ReadAll %s: %v", suffix, err)
		}
		_ = res.Body.Close()
		return string(b)
	}
	if s := get("/bar/"); !strings.Contains(s, ">foo.txt<") {
		t.Logf("expected a directory listing with foo.txt, got %q", s)
	}
	if s := get("/bar/foo.txt"); s != "Hello world" {
		t.Logf("expected %q, got %q", "Hello world", s)
	}
}

func TestDirJoin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping test on windows")
	}
	wfi, err := os.Stat("/etc/hosts")
	if err != nil {
		t.Skip("skipping test; no /etc/hosts file")
	}
	test := func(d http.Dir, name string) {
		f, err := d.Open(name)
		if err != nil {
			t.Fatalf("open of %s: %v", name, err)
		}
		defer func() { _ = f.Close() }()
		gfi, err := f.Stat()
		if err != nil {
			t.Fatalf("stat of %s: %v", name, err)
		}
		if !os.SameFile(gfi, wfi) {
			t.Errorf("%s got different file", name)
		}
	}
	test(http.Dir("/etc/"), "/hosts")
	test(http.Dir("/etc/"), "hosts")
	test(http.Dir("/etc/"), "../../../../hosts")
	test(http.Dir("/etc"), "/hosts")
	test(http.Dir("/etc"), "hosts")
	test(http.Dir("/etc"), "../../../../hosts")

	// Not really directories, but since we use this trick in
	// ServeFile, test it:
	test(http.Dir("/etc/hosts"), "")
	test(http.Dir("/etc/hosts"), "/")
	test(http.Dir("/etc/hosts"), "../")
}

func TestEmptyDirOpenCWD(t *testing.T) {
	test := func(d http.Dir) {
		name := "fileserver_test.go"
		f, err := d.Open(name)
		if err != nil {
			t.Fatalf("open of %s: %v", name, err)
		}
		defer func() { _ = f.Close() }()
	}
	test(http.Dir(""))
	test(http.Dir("."))
	test(http.Dir("./"))
}

func TestServeFileContentType(t *testing.T) {
	defer afterTest(t)
	const ctype = "icecream/chocolate"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.FormValue("override") {
		case "1":
			w.Header().Set("Content-Type", ctype)
		case "2":
			// Explicitly inhibit sniffing.
			w.Header()["Content-Type"] = []string{}
		}
		ServeFile(w, r, "testdata/file")
	}))
	defer ts.Close()
	get := func(override string, want []string) {
		resp, err := http.Get(ts.URL + "?override=" + override)
		if err != nil {
			t.Fatal(err)
		}
		if h := resp.Header["Content-Type"]; !reflect.DeepEqual(h, want) {
			t.Errorf("Content-Type mismatch: got %v, want %v", h, want)
		}
		_ = resp.Body.Close()
	}
	get("0", []string{"text/plain; charset=utf-8"})
	get("1", []string{ctype})
	get("2", nil)
}

func TestServeFileMimeType(t *testing.T) {
	defer afterTest(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeFile(w, r, "testdata/style.css")
	}))
	defer ts.Close()
	resp, err := http.Get(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	want := "text/css; charset=utf-8"
	if h := resp.Header.Get("Content-Type"); h != want {
		t.Errorf("Content-Type mismatch: got %q, want %q", h, want)
	}
}

func TestServeFileFromCWD(t *testing.T) {
	defer afterTest(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeFile(w, r, "fileserver_test.go")
	}))
	defer ts.Close()
	r, err := http.Get(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Body.Close()
	if r.StatusCode != 200 {
		t.Fatalf("expected 200 OK, got %s", r.Status)
	}
}

func TestServeFileWithContentEncoding(t *testing.T) {
	defer afterTest(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "foo")
		ServeFile(w, r, "testdata/file")
	}))
	defer ts.Close()
	resp, err := http.Get(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if g, e := resp.ContentLength, int64(-1); g != e {
		t.Errorf("Content-Length mismatch: got %d, want %d", g, e)
	}
}

func TestServeIndexHtml(t *testing.T) {
	defer afterTest(t)
	const want = "index.html says hello\n"

	fs := &FileServer{
		"version",
		http.Dir("."),
		inject.CopyInject{},
		ricetemp.MustMakeTemplates(rice.MustFindBox("../templates")),
		[]routespec.RouteSpec{},
		"",
	}
	ts := httptest.NewServer(fs)
	defer ts.Close()

	for _, path := range []string{"/testdata/", "/testdata/index.html"} {
		res, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		b, err := ioutil.ReadAll(res.Body)
		if err != nil {
			t.Fatal("reading Body:", err)
		}
		if s := string(b); s != want {
			t.Errorf("for path %q got %q, want %q", path, s, want)
		}
		_ = res.Body.Close()
	}
}

func TestFileServerZeroByte(t *testing.T) {
	defer afterTest(t)
	fs := &FileServer{
		"version",
		http.Dir("."),
		inject.CopyInject{},
		ricetemp.MustMakeTemplates(rice.MustFindBox("../templates")),
		[]routespec.RouteSpec{},
		"",
	}
	ts := httptest.NewServer(fs)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/..\x00")
	if err != nil {
		t.Fatal(err)
	}
	b, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Fatal("reading Body:", err)
	}
	if res.StatusCode == 200 {
		t.Errorf("got status 200; want an error. Body is:\n%s", string(b))
	}
}

type fakeFileInfo struct {
	dir      bool
	basename string
	modtime  time.Time
	ents     []*fakeFileInfo
	contents string
}

func (f *fakeFileInfo) Name() string       { return f.basename }
func (f *fakeFileInfo) Sys() interface{}   { return nil }
func (f *fakeFileInfo) ModTime() time.Time { return f.modtime }
func (f *fakeFileInfo) IsDir() bool        { return f.dir }
func (f *fakeFileInfo) Size() int64        { return int64(len(f.contents)) }
func (f *fakeFileInfo) Mode() os.FileMode {
	if f.dir {
		return 0755 | os.ModeDir
	}
	return 0644
}

type fakeFile struct {
	io.ReadSeeker
	fi   *fakeFileInfo
	path string // as opened
}

func (f *fakeFile) Close() error               { return nil }
func (f *fakeFile) Stat() (os.FileInfo, error) { return f.fi, nil }
func (f *fakeFile) Readdir(count int) ([]os.FileInfo, error) {
	if !f.fi.dir {
		return nil, os.ErrInvalid
	}
	var fis []os.FileInfo
	for _, fi := range f.fi.ents {
		fis = append(fis, fi)
	}
	return fis, nil
}

type fakeFS map[string]*fakeFileInfo

func (fs fakeFS) Open(name string) (http.File, error) {
	name = path.Clean(name)
	f, ok := fs[name]
	if !ok {
		return nil, os.ErrNotExist
	}
	return &fakeFile{ReadSeeker: strings.NewReader(f.contents), fi: f, path: name}, nil
}

func TestNotFoundOverride(t *testing.T) {
	defer afterTest(t)
	ffile := &fakeFileInfo{
		basename: "foo.html",
		modtime:  time.Unix(1000000000, 0).UTC(),
		contents: "I am a fake file",
	}
	fsys := fakeFS{
		"/": &fakeFileInfo{
			dir:     true,
			modtime: time.Unix(123, 0).UTC(),
			ents:    []*fakeFileInfo{},
		},
		"/one": &fakeFileInfo{
			dir:     true,
			modtime: time.Unix(123, 0).UTC(),
			ents:    []*fakeFileInfo{ffile},
		},
		"/one/foo.html": ffile,
	}

	fs := &FileServer{
		"version",
		fsys,
		inject.CopyInject{},
		ricetemp.MustMakeTemplates(rice.MustFindBox("../templates")),
		[]routespec.RouteSpec{
			{Host: "", Path: "/", Value: "foo.html"},
		},
		"",
	}

	ts := httptest.NewServer(fs)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/one/nonexistent.html")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != 200 {
		t.Error("Expected to find over-ride file.")
	}

	res, err = http.Get(ts.URL + "/one/two/nonexistent.html")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != 200 {
		t.Error("Expected to find over-ride file.")
	}

	res, err = http.Get(ts.URL + "/nonexistent.html")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != 404 {
		t.Error("Expected to find over-ride file.")
	}

	res, err = http.Get(ts.URL + "/two/nonexistent.html")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != 404 {
		t.Error("Expected to find over-ride file.")
	}

}

func TestDirectoryIfNotModified(t *testing.T) {
	defer afterTest(t)
	const indexContents = "I am a fake index.html file"
	fileMod := time.Unix(1000000000, 0).UTC()
	fileModStr := fileMod.Format(http.TimeFormat)
	dirMod := time.Unix(123, 0).UTC()
	indexFile := &fakeFileInfo{
		basename: "index.html",
		modtime:  fileMod,
		contents: indexContents,
	}
	fsys := fakeFS{
		"/": &fakeFileInfo{
			dir:     true,
			modtime: dirMod,
			ents:    []*fakeFileInfo{indexFile},
		},
		"/index.html": indexFile,
	}

	fs := &FileServer{
		"version",
		fsys,
		inject.CopyInject{},
		ricetemp.MustMakeTemplates(rice.MustFindBox("../templates")),
		[]routespec.RouteSpec{},
		"",
	}

	ts := httptest.NewServer(fs)
	defer ts.Close()

	res, err := http.Get(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != indexContents {
		t.Fatalf("Got body %q; want %q", b, indexContents)
	}
	_ = res.Body.Close()

	lastMod := res.Header.Get("Last-Modified")
	if lastMod != fileModStr {
		t.Fatalf("initial Last-Modified = %q; want %q", lastMod, fileModStr)
	}

	req, _ := http.NewRequest("GET", ts.URL, nil)
	req.Header.Set("If-Modified-Since", lastMod)

	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 304 {
		t.Fatalf("Code after If-Modified-Since request = %v; want 304", res.StatusCode)
	}
	_ = res.Body.Close()

	// Advance the index.html file's modtime, but not the directory's.
	indexFile.modtime = indexFile.modtime.Add(1 * time.Hour)

	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 {
		t.Fatalf("Code after second If-Modified-Since request = %v; want 200; res is %#v", res.StatusCode, res)
	}
	_ = res.Body.Close()
}

func mustStat(t *testing.T, fileName string) os.FileInfo {
	fi, err := os.Stat(fileName)
	if err != nil {
		t.Fatal(err)
	}
	return fi
}

func TestServeContent(t *testing.T) {
	defer afterTest(t)
	type serveParam struct {
		name        string
		modtime     time.Time
		content     io.ReadSeeker
		contentType string
		etag        string
	}
	servec := make(chan serveParam, 1)
	lock := sync.Mutex{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := <-servec
		if p.etag != "" {
			w.Header().Set("ETag", p.etag)
		}
		if p.contentType != "" {
			w.Header().Set("Content-Type", p.contentType)
		}
		lock.Lock()
		defer lock.Unlock()
		err := ServeContent(w, r, p.name, p.modtime, p.content)
		if err != nil {
			t.Fail()
		}
	}))
	defer ts.Close()

	type testCase struct {
		// One of file or content must be set:
		file    string
		content io.ReadSeeker

		modtime          time.Time
		serveETag        string // optional
		serveContentType string // optional
		reqHeader        map[string]string
		wantLastMod      string
		wantContentType  string
		wantStatus       int
	}
	htmlModTime := mustStat(t, "testdata/index.html").ModTime()
	tests := map[string]testCase{
		"no_last_modified": {
			file:            "testdata/style.css",
			wantContentType: "text/css; charset=utf-8",
			wantStatus:      200,
		},
		"with_last_modified": {
			file:            "testdata/index.html",
			wantContentType: "text/html; charset=utf-8",
			modtime:         htmlModTime,
			wantLastMod:     htmlModTime.UTC().Format(http.TimeFormat),
			wantStatus:      200,
		},
		"not_modified_modtime": {
			file:    "testdata/style.css",
			modtime: htmlModTime,
			reqHeader: map[string]string{
				"If-Modified-Since": htmlModTime.UTC().Format(http.TimeFormat),
			},
			wantStatus: 304,
		},
		"not_modified_modtime_with_contenttype": {
			file:             "testdata/style.css",
			serveContentType: "text/css", // explicit content type
			modtime:          htmlModTime,
			reqHeader: map[string]string{
				"If-Modified-Since": htmlModTime.UTC().Format(http.TimeFormat),
			},
			wantStatus: 304,
		},
		"not_modified_etag": {
			file:      "testdata/style.css",
			serveETag: `"foo"`,
			reqHeader: map[string]string{
				"If-None-Match": `"foo"`,
			},
			wantStatus: 304,
		},
		"not_modified_etag_no_seek": {
			content:   panicOnSeek{nil}, // should never be called
			serveETag: `"foo"`,
			reqHeader: map[string]string{
				"If-None-Match": `"foo"`,
			},
			wantStatus: 304,
		},
		// An If-Range resource for entity "A", but entity "B" is now current.
		// The Range request should be ignored.
		"range_no_match": {
			file:      "testdata/style.css",
			serveETag: `"A"`,
			reqHeader: map[string]string{
				"Range":    "bytes=0-4",
				"If-Range": `"B"`,
			},
			wantStatus:      200,
			wantContentType: "text/css; charset=utf-8",
		},
	}
	for testName, tt := range tests {
		var content io.ReadSeeker
		if tt.file != "" {
			f, err := os.Open(tt.file)
			if err != nil {
				t.Fatalf("test %q: %v", testName, err)
			}
			defer func() {
				lock.Lock()
				defer lock.Unlock()
				_ = f.Close()
			}()
			content = f
		} else {
			content = tt.content
		}

		servec <- serveParam{
			name:        filepath.Base(tt.file),
			content:     content,
			modtime:     tt.modtime,
			etag:        tt.serveETag,
			contentType: tt.serveContentType,
		}
		req, err := http.NewRequest("GET", ts.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		for k, v := range tt.reqHeader {
			req.Header.Set(k, v)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, err = io.Copy(ioutil.Discard, res.Body)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if res.StatusCode != tt.wantStatus {
			t.Errorf("test %q: status = %d; want %d", testName, res.StatusCode, tt.wantStatus)
		}
		if g, e := res.Header.Get("Content-Type"), tt.wantContentType; g != e {
			t.Errorf("test %q: content-type = %q, want %q", testName, g, e)
		}
		if g, e := res.Header.Get("Last-Modified"), tt.wantLastMod; g != e {
			t.Errorf("test %q: last-modified = %q, want %q", testName, g, e)
		}
	}
}

func getBody(t *testing.T, testName string, req http.Request) (*http.Response, []byte) {
	r, err := http.DefaultClient.Do(&req)
	if err != nil {
		t.Fatalf("%s: for URL %q, send error: %v", testName, req.URL.String(), err)
	}
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("%s: for URL %q, reading body: %v", testName, req.URL.String(), err)
	}
	return r, b
}

type panicOnSeek struct{ io.ReadSeeker }
