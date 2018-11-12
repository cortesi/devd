package devd

import (
	"crypto/tls"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"time"

	"github.com/cortesi/devd/fileserver"
	"github.com/cortesi/devd/httpctx"
	"github.com/cortesi/devd/inject"
	"github.com/cortesi/devd/reverseproxy"
	"github.com/cortesi/devd/routespec"
)

// Endpoint is the destination of a Route - either on the filesystem or
// forwarding to another URL
type endpoint interface {
	Handler(prefix string, templates *template.Template, ci inject.CopyInject) httpctx.Handler
	String() string
}

// An endpoint that forwards to an upstream URL
type forwardEndpoint url.URL

func (ep forwardEndpoint) Handler(prefix string, templates *template.Template, ci inject.CopyInject) httpctx.Handler {
	u := url.URL(ep)
	rp := reverseproxy.NewSingleHostReverseProxy(&u, ci)
	rp.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	rp.FlushInterval = 200 * time.Millisecond
	return httpctx.StripPrefix(prefix, rp)
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
type filesystemEndpoint struct {
	Root           string
	notFoundRoutes []routespec.RouteSpec
}

func newFilesystemEndpoint(path string, notfound []string) (*filesystemEndpoint, error) {
	rparts := []routespec.RouteSpec{}
	for _, p := range notfound {
		rp, err := routespec.ParseRouteSpec(p)
		if err != nil {
			return nil, err
		}
		if rp.IsURL {
			return nil, fmt.Errorf("Not found over-ride target cannot be a URL.")
		}
		rparts = append(rparts, *rp)
	}
	return &filesystemEndpoint{path, rparts}, nil
}

func (ep filesystemEndpoint) Handler(prefix string, templates *template.Template, ci inject.CopyInject) httpctx.Handler {
	return &fileserver.FileServer{
		Version:        "devd " + Version,
		Root:           http.Dir(ep.Root),
		Inject:         ci,
		Templates:      templates,
		NotFoundRoutes: ep.notFoundRoutes,
		Prefix:         prefix,
	}
}

func (ep filesystemEndpoint) String() string {
	return "reads files from " + ep.Root
}

// Route is a mapping from a (host, path) tuple to an endpoint.
type Route struct {
	Host     string
	Path     string
	Endpoint endpoint
}

// Constructs a new route from a string specifcation. Specifcations are of the
// form ANCHOR=VALUE.
func newRoute(s string, notfound []string) (*Route, error) {
	rp, err := routespec.ParseRouteSpec(s)
	if err != nil {
		return nil, err
	}

	var ep endpoint

	if rp.IsURL {
		ep, err = newForwardEndpoint(rp.Value)
	} else {
		ep, err = newFilesystemEndpoint(rp.Value, notfound)
	}
	if err != nil {
		return nil, err
	}
	return &Route{rp.Host, rp.Path, ep}, nil
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
func (f RouteCollection) Add(value string, notfound []string) error {
	s, err := newRoute(value, notfound)
	if err != nil {
		return err
	}
	if _, exists := f[s.MuxMatch()]; exists {
		return errors.New("Route already exists.")
	}
	f[s.MuxMatch()] = *s
	return nil
}
