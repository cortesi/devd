// Package fileserver provides a filesystem HTTP handler, based on the built-in
// Go FileServer. Extensions include better directory listings, support for
// injection, better and use of Context.
package fileserver

import (
	"errors"
	"fmt"
	"html/template"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/context"

	"github.com/cortesi/devd/inject"
	"github.com/cortesi/devd/routespec"
	"github.com/cortesi/termlog"
)

const sniffLen = 512

func rawHeaderGet(h http.Header, key string) string {
	if v := h[key]; len(v) > 0 {
		return v[0]
	}
	return ""
}

// fileSlice implements sort.Interface, which allows to sort by file name with
// directories first.
type fileSlice []os.FileInfo

func (p fileSlice) Len() int {
	return len(p)
}

func (p fileSlice) Less(i, j int) bool {
	a, b := p[i], p[j]
	if a.IsDir() && !b.IsDir() {
		return true
	}
	if b.IsDir() && !a.IsDir() {
		return false
	}
	if strings.HasPrefix(a.Name(), ".") && !strings.HasPrefix(b.Name(), ".") {
		return false
	}
	if strings.HasPrefix(b.Name(), ".") && !strings.HasPrefix(a.Name(), ".") {
		return true
	}
	return a.Name() < b.Name()
}

func (p fileSlice) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

type dirData struct {
	Version string
	Name    string
	Files   fileSlice
}

type fourohfourData struct {
	Version string
}

func stripPrefix(prefix string, path string) string {
	if prefix == "" {
		return path
	}
	if p := strings.TrimPrefix(path, prefix); len(p) < len(path) {
		return p
	}
	return path
}

// errSeeker is returned by ServeContent's sizeFunc when the content
// doesn't seek properly. The underlying Seeker's error text isn't
// included in the sizeFunc reply so it's not sent over HTTP to end
// users.
var errSeeker = errors.New("seeker can't seek")

// if name is empty, filename is unknown. (used for mime type, before sniffing)
// if modtime.IsZero(), modtime is unknown.
// content must be seeked to the beginning of the file.
// The sizeFunc is called at most once. Its error, if any, is sent in the HTTP response.
func serveContent(ci inject.CopyInject, w http.ResponseWriter, r *http.Request, name string, modtime time.Time, sizeFunc func() (int64, error), content io.ReadSeeker) error {
	if checkLastModified(w, r, modtime) {
		return nil
	}
	done := checkETag(w, r)
	if done {
		return nil
	}

	code := http.StatusOK

	// If Content-Type isn't set, use the file's extension to find it, but
	// if the Content-Type is unset explicitly, do not sniff the type.
	ctypes, haveType := w.Header()["Content-Type"]
	var ctype string
	if !haveType {
		ctype = mime.TypeByExtension(filepath.Ext(name))
		if ctype == "" {
			// read a chunk to decide between utf-8 text and binary
			var buf [sniffLen]byte
			n, _ := io.ReadFull(content, buf[:])
			ctype = http.DetectContentType(buf[:n])
			_, err := content.Seek(0, os.SEEK_SET) // rewind to output whole file
			if err != nil {
				http.Error(w, "seeker can't seek", http.StatusInternalServerError)
				return err
			}
		}
		w.Header().Set("Content-Type", ctype)
	} else if len(ctypes) > 0 {
		ctype = ctypes[0]
	}

	injector, err := ci.Sniff(content, ctype)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return err
	}

	size, err := sizeFunc()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return err
	}

	if injector.Found() {
		size = size + int64(injector.Extra())
	}

	if size >= 0 {
		if w.Header().Get("Content-Encoding") == "" {
			w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
		}
	}

	w.WriteHeader(code)
	if r.Method != "HEAD" {
		_, err := injector.Copy(w)
		if err != nil {
			return err
		}
	}
	return nil
}

// modtime is the modification time of the resource to be served, or IsZero().
// return value is whether this request is now complete.
func checkLastModified(w http.ResponseWriter, r *http.Request, modtime time.Time) bool {
	if modtime.IsZero() {
		return false
	}

	// The Date-Modified header truncates sub-second precision, so
	// use mtime < t+1s instead of mtime <= t to check for unmodified.
	if t, err := time.Parse(http.TimeFormat, r.Header.Get("If-Modified-Since")); err == nil && modtime.Before(t.Add(1*time.Second)) {
		h := w.Header()
		delete(h, "Content-Type")
		delete(h, "Content-Length")
		w.WriteHeader(http.StatusNotModified)
		return true
	}
	w.Header().Set("Last-Modified", modtime.UTC().Format(http.TimeFormat))
	return false
}

// checkETag implements If-None-Match checks.
// The ETag must have been previously set in the ResponseWriter's headers.
//
// The return value is whether this request is now considered done.
func checkETag(w http.ResponseWriter, r *http.Request) (done bool) {
	etag := rawHeaderGet(w.Header(), "Etag")
	if inm := rawHeaderGet(r.Header, "If-None-Match"); inm != "" {
		// Must know ETag.
		if etag == "" {
			return false
		}

		// TODO(bradfitz): non-GET/HEAD requests require more work:
		// sending a different status code on matches, and
		// also can't use weak cache validators (those with a "W/
		// prefix).  But most users of ServeContent will be using
		// it on GET or HEAD, so only support those for now.
		if r.Method != "GET" && r.Method != "HEAD" {
			return false
		}

		// TODO(bradfitz): deal with comma-separated or multiple-valued
		// list of If-None-match values.  For now just handle the common
		// case of a single item.
		if inm == etag || inm == "*" {
			h := w.Header()
			delete(h, "Content-Type")
			delete(h, "Content-Length")
			w.WriteHeader(http.StatusNotModified)
			return true
		}
	}
	return false
}

// localRedirect gives a Moved Permanently response.
// It does not convert relative paths to absolute paths like Redirect does.
func localRedirect(w http.ResponseWriter, r *http.Request, newPath string) {
	if q := r.URL.RawQuery; q != "" {
		newPath += "?" + q
	}
	w.Header().Set("Location", newPath)
	w.WriteHeader(http.StatusMovedPermanently)
}

// FileServer returns a handler that serves HTTP requests
// with the contents of the file system rooted at root.
//
// To use the operating system's file system implementation,
// use http.Dir:
//
//     http.Handle("/", &fileserver.FileServer{Root: http.Dir("/tmp")})
type FileServer struct {
	Version        string
	Root           http.FileSystem
	Inject         inject.CopyInject
	Templates      *template.Template
	NotFoundRoutes []routespec.RouteSpec
	Prefix         string
}

func (fserver *FileServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fserver.ServeHTTPContext(context.Background(), w, r)
}

// ServeHTTPContext is like ServeHTTP, but with added context
func (fserver *FileServer) ServeHTTPContext(
	ctx context.Context, w http.ResponseWriter, r *http.Request,
) {
	logger := termlog.FromContext(ctx)
	logger.SayAs("debug", "debug fileserver: serving with FileServer...")

	upath := stripPrefix(fserver.Prefix, r.URL.Path)
	if !strings.HasPrefix(upath, "/") {
		upath = "/" + upath
	}
	fserver.serveFile(logger, w, r, path.Clean(upath), true)
}

// Given a path and a "not found" over-ride specification, return an array of
// over-ride paths that should be considered for serving, in priority order. We
// assume that path is a sub-path above a certain root, and we never return
// paths that would fall outside this.
//
// We also sanity check file extensions to make sure that the expected file
// type matches what we serve. This prevents an over-ride for *.html files from
// serving up data when, say, a missing .png is requested.
func notFoundSearchPaths(pth string, spec string) []string {
	var ret []string
	if strings.HasPrefix(spec, "/") {
		ret = []string{path.Clean(spec)}
	} else {
		for {
			pth = path.Dir(pth)
			if pth == "/" {
				ret = append(ret, path.Join(pth, spec))
				break
			}
			ret = append(ret, path.Join(pth, spec))
		}
	}
	return ret
}

// Get the media type for an extension, via a MIME lookup, defaulting to
// "text/html".
func _getType(ext string) string {
	typ := mime.TypeByExtension(ext)
	if typ == "" {
		return "text/html"
	}
	smime, _, err := mime.ParseMediaType(typ)
	if err != nil {
		return "text/html"
	}
	return smime
}

// Checks whether the incoming request has the same expected type as an
// over-ride specification.
func matchTypes(spec string, req string) bool {
	smime := _getType(path.Ext(spec))
	rmime := _getType(path.Ext(req))
	if smime == rmime {
		return true
	}
	return false
}

func (fserver *FileServer) serve404(w http.ResponseWriter) error {
	d := fourohfourData{
		Version: fserver.Version,
	}
	err := fserver.Inject.ServeTemplate(
		http.StatusNotFound,
		w,
		fserver.Templates.Lookup("404.html"),
		&d,
	)
	if err != nil {
		return err
	}
	return nil
}

func (fserver *FileServer) dirList(logger termlog.Logger, w http.ResponseWriter, name string, f http.File) {
	w.Header().Set("Cache-Control", "no-store, must-revalidate")
	files, err := f.Readdir(0)
	if err != nil {
		logger.Shout("Error reading directory for listing: %s", err)
		return
	}
	sortedFiles := fileSlice(files)
	sort.Sort(sortedFiles)
	data := dirData{
		Version: fserver.Version,
		Name:    name,
		Files:   sortedFiles,
	}
	err = fserver.Inject.ServeTemplate(
		http.StatusOK,
		w,
		fserver.Templates.Lookup("dirlist.html"),
		data,
	)
	if err != nil {
		logger.Shout("Failed to generate dir listing: %s", err)
	}
}

func (fserver *FileServer) notFound(
	logger termlog.Logger,
	w http.ResponseWriter,
	r *http.Request,
	name string,
	dir *http.File,
) (err error) {
	sm := http.NewServeMux()
	seen := make(map[string]bool)
	for _, nfr := range fserver.NotFoundRoutes {
		seen[nfr.MuxMatch()] = true
		sm.HandleFunc(
			nfr.MuxMatch(),
			func(nfr routespec.RouteSpec) func(w http.ResponseWriter, r *http.Request) {
				return func(w http.ResponseWriter, r *http.Request) {
					if matchTypes(nfr.Value, r.URL.Path) {
						for _, pth := range notFoundSearchPaths(name, nfr.Value) {
							next, err := fserver.serveNotFoundFile(w, r, pth)
							if err != nil {
								logger.Shout("Unable to serve not-found override: %s", err)
							}
							if !next {
								return
							}
						}
					}
					err = fserver.serve404(w)
					if err != nil {
						logger.Shout("Internal error: %s", err)
					}
				}
			}(nfr),
		)
	}
	if _, exists := seen["/"]; !exists {
		sm.HandleFunc(
			"/",
			func(response http.ResponseWriter, request *http.Request) {
				if dir != nil {
					d, err := (*dir).Stat()
					if err != nil {
						logger.Shout("Internal error: %s", err)
						return
					}
					if checkLastModified(response, request, d.ModTime()) {
						return
					}
					fserver.dirList(logger, response, name, *dir)
					return
				}
				err = fserver.serve404(w)
				if err != nil {
					logger.Shout("Internal error: %s", err)
				}
			},
		)
	}
	handle, _ := sm.Handler(r)
	handle.ServeHTTP(w, r)
	return err
}

// If the next return value is true, the caller should proceed to the next
// over-ride path if there is one. If the err return value is non-nil, serving
// should stop.
func (fserver *FileServer) serveNotFoundFile(
	w http.ResponseWriter,
	r *http.Request,
	name string,
) (next bool, err error) {
	f, err := fserver.Root.Open(name)
	if err != nil {
		return true, nil
	}
	defer func() { _ = f.Close() }()

	d, err := f.Stat()
	if err != nil || d.IsDir() {
		return true, nil
	}

	// serverContent will check modification time
	sizeFunc := func() (int64, error) { return d.Size(), nil }
	err = serveContent(fserver.Inject, w, r, d.Name(), d.ModTime(), sizeFunc, f)
	if err != nil {
		return false, fmt.Errorf("Error serving file: %s", err)
	}
	return false, nil
}

// name is '/'-separated, not filepath.Separator.
func (fserver *FileServer) serveFile(
	logger termlog.Logger,
	w http.ResponseWriter,
	r *http.Request,
	name string,
	redirect bool,
) {
	const indexPage = "/index.html"

	// redirect .../index.html to .../
	// can't use Redirect() because that would make the path absolute,
	// which would be a problem running under StripPrefix
	if strings.HasSuffix(r.URL.Path, indexPage) {
		logger.SayAs(
			"debug", "debug fileserver: redirecting %s -> ./", indexPage,
		)
		localRedirect(w, r, "./")
		return
	}

	f, err := fserver.Root.Open(name)
	if err != nil {
		logger.WarnAs("debug", "debug fileserver: %s", err)
		if err := fserver.notFound(logger, w, r, name, nil); err != nil {
			logger.Shout("Internal error: %s", err)
		}
		return
	}
	defer func() { _ = f.Close() }()

	d, err1 := f.Stat()
	if err1 != nil {
		logger.WarnAs("debug", "debug fileserver: %s", err)
		if err := fserver.notFound(logger, w, r, name, nil); err != nil {
			logger.Shout("Internal error: %s", err)
		}
		return
	}

	if redirect {
		// redirect to canonical path: / at end of directory url
		url := r.URL.Path
		if !strings.HasPrefix(url, "/") {
			url = "/" + url
		}
		if d.IsDir() {
			if url[len(url)-1] != '/' {
				localRedirect(w, r, path.Base(url)+"/")
				return
			}
		} else if url[len(url)-1] == '/' {
			localRedirect(w, r, "../"+path.Base(url))
			return
		}
	}

	// use contents of index.html for directory, if present
	if d.IsDir() {
		index := name + indexPage
		ff, err := fserver.Root.Open(index)
		if err == nil {
			defer func() { _ = ff.Close() }()
			dd, err := ff.Stat()
			if err == nil {
				name = index
				d = dd
				f = ff
			}
		}
	}

	// Still a directory? (we didn't find an index.html file)
	if d.IsDir() {
		if err := fserver.notFound(logger, w, r, name, &f); err != nil {
			logger.Shout("Internal error: %s", err)
		}
		return
	}

	// serverContent will check modification time
	sizeFunc := func() (int64, error) { return d.Size(), nil }
	err = serveContent(fserver.Inject, w, r, d.Name(), d.ModTime(), sizeFunc, f)
	if err != nil {
		logger.Warn("Error serving file: %s", err)
	}
}
