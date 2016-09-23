package devd

import (
	"crypto/tls"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/cortesi/devd/fileserver"
	"github.com/cortesi/devd/httpctx"
	"github.com/cortesi/devd/inject"
	"github.com/cortesi/devd/reverseproxy"
)

const defaultDomain = "devd.io"

func checkURL(s string) (isURL bool, err error) {
	var parsed *url.URL

	parsed, err = url.Parse(s)
	if err != nil {
		return
	}

	switch {
	case parsed.Scheme == "": // No scheme means local file system
		isURL = false
	case parsed.Scheme == "http", parsed.Scheme == "https":
		isURL = true
	case parsed.Scheme == "ws":
		err = fmt.Errorf("Websocket protocol not supported: %s", s)
	default:
		// A route of "localhost:1234/abc" without the "http" or "https" triggers this case.
		// Unfortunately a route of "localhost/abc" just looks like a file and is not caught here.
		err = fmt.Errorf("Unknown scheme '%s': did you mean http or https?: %s", parsed.Scheme, s)
	}
	return
}

// Endpoint is the destination of a Route - either on the filesystem or
// forwarding to another URL
type endpoint interface {
	Handler(templates *template.Template, ci inject.CopyInject) httpctx.Handler
	String() string
}

// An endpoint that forwards to an upstream URL
type forwardEndpoint url.URL

func (ep forwardEndpoint) Handler(templates *template.Template, ci inject.CopyInject) httpctx.Handler {
	u := url.URL(ep)
	rp := reverseproxy.NewSingleHostReverseProxy(&u, ci)
	rp.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	return rp
}

func newForwardEndpoint(path string) (*forwardEndpoint, error) {
	url, err := url.Parse(path)
	if err != nil {
		return nil, fmt.Errorf("Could not parse route URL: %s", err)
	}
	f := forwardEndpoint(*url)
	return &f, nil
}

func (ep forwardEndpoint) String() string {
	return "forward to " + ep.Scheme + "://" + ep.Host + ep.Path
}

// An enpoint that serves a filesystem location
type filesystemEndpoint string

func newFilesystemEndpoint(path string) (*filesystemEndpoint, error) {
	f := filesystemEndpoint(path)
	return &f, nil
}

func (ep filesystemEndpoint) Handler(templates *template.Template, ci inject.CopyInject) httpctx.Handler {
	return &fileserver.FileServer{
		Root:      http.Dir(ep),
		Inject:    ci,
		Templates: templates,
	}
}

func (ep filesystemEndpoint) String() string {
	return "reads files from " + string(ep)
}

// Route is a mapping from a (host, path) tuple to an endpoint.
type Route struct {
	Host     string
	Path     string
	Endpoint endpoint
}

// Constructs a new route from a string specifcation. Specifcations are of the
// form ANCHOR=VALUE.
func newRoute(s string) (*Route, error) {
	seq := strings.SplitN(s, "=", 2)
	var path, value string
	if len(seq) == 1 {
		path = "/"
		value = seq[0]
	} else {
		path = seq[0]
		value = seq[1]
	}
	if path == "" || value == "" {
		return nil, errors.New("Invalid specification")
	}
	host := ""
	if path[0] != '/' {
		seq := strings.SplitN(path, "/", 2)
		host = seq[0] + "." + defaultDomain
		switch len(seq) {
		case 1:
			path = "/"
		case 2:
			path = "/" + seq[1]
		}
	}
	var ep endpoint
	var err error
	var isURL bool

	isURL, err = checkURL(value)

	if err != nil {
		return nil, err
	}

	if isURL {
		ep, err = newForwardEndpoint(value)
	} else {
		ep, err = newFilesystemEndpoint(value)
	}
	if err != nil {
		return nil, err
	}
	return &Route{host, path, ep}, nil
}

// MuxMatch produces a match clause suitable for passing to a Mux
func (f Route) MuxMatch() string {
	// Path is guaranteed to start with /
	return f.Host + f.Path
}

// RouteCollection is a collection of routes
type RouteCollection map[string]Route

func (f *RouteCollection) String() string {
	return fmt.Sprintf("%v", *f)
}

// Add a route to the collection
func (f RouteCollection) Add(value string) error {
	s, err := newRoute(value)
	if err != nil {
		return err
	}
	if _, exists := f[s.MuxMatch()]; exists {
		return errors.New("Route already exists.")
	}
	f[s.MuxMatch()] = *s
	return nil
}
