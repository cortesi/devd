// Package fileserver provides a filesystem HTTP handler, based on the built-in
// Go FileServer. Extensions include better directory listings, support for
// injection, better and use of Context.
package fileserver

import (
	"errors"
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
	return a.Name() < b.Name()
}

func (p fileSlice) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

type dirData struct {
	Name  string
	Files fileSlice
}

func dirList(ci inject.CopyInject, logger termlog.Logger, w http.ResponseWriter, name string, f http.File, templates *template.Template) {
	w.Header().Set("Cache-Control", "no-store, must-revalidate")
	files, err := f.Readdir(0)
	if err != nil {
		logger.Shout("Error reading directory for listing: %s", err)
		return
	}
	sortedFiles := fileSlice(files)
	sort.Sort(sortedFiles)
	data := dirData{Name: name, Files: sortedFiles}
	err = ci.ServeTemplate(http.StatusOK, w, templates.Lookup("dirlist.html"), data)
	if err != nil {
		logger.Shout("Failed to generate dir listing: %s", err)
	}
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

	injector, err := ci.Sniff(content)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return err
	}

	size, err := sizeFunc()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return err
	}

	if injector.Found {
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
	Root      http.FileSystem
	Inject    inject.CopyInject
	Templates *template.Template
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

	upath := r.URL.Path
	if !strings.HasPrefix(upath, "/") {
		upath = "/" + upath
		r.URL.Path = upath
	}
	fserver.serveFile(logger, w, r, path.Clean(upath), true)
}

func notFound(ci inject.CopyInject, templates *template.Template, w http.ResponseWriter) error {
	err := ci.ServeTemplate(http.StatusNotFound, w, templates.Lookup("404.html"), nil)
	if err != nil {
		return err
	}
	return nil
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
		if err := notFound(fserver.Inject, fserver.Templates, w); err != nil {
			logger.Shout("Internal error: %s", err)
		}
		return
	}
	defer f.Close()

	d, err1 := f.Stat()
	if err1 != nil {
		logger.WarnAs("debug", "debug fileserver: %s", err)
		if err := notFound(fserver.Inject, fserver.Templates, w); err != nil {
			logger.Shout("Internal error: %s", err)
		}
		return
	}

	if redirect {
		// redirect to canonical path: / at end of directory url
		// r.URL.Path always begins with /
		url := r.URL.Path
		if d.IsDir() {
			if url[len(url)-1] != '/' {
				localRedirect(w, r, path.Base(url)+"/")
				return
			}
		} else {
			if url[len(url)-1] == '/' {
				localRedirect(w, r, "../"+path.Base(url))
				return
			}
		}
	}

	// use contents of index.html for directory, if present
	if d.IsDir() {
		index := name + indexPage
		ff, err := fserver.Root.Open(index)
		if err == nil {
			defer ff.Close()
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
		if checkLastModified(w, r, d.ModTime()) {
			return
		}
		dirList(fserver.Inject, logger, w, name, f, fserver.Templates)
		return
	}

	// serverContent will check modification time
	sizeFunc := func() (int64, error) { return d.Size(), nil }
	err = serveContent(fserver.Inject, w, r, d.Name(), d.ModTime(), sizeFunc, f)
	if err != nil {
		logger.Warn("Error serving file: %s", err)
	}
}
