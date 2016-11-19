package routespec

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
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

// A RouteSpec is a parsed route specification
type RouteSpec struct {
	Host  string
	Path  string
	Value string
	IsURL bool
}

// MuxMatch produces a match clause suitable for passing to a Mux
func (rp *RouteSpec) MuxMatch() string {
	// Path is guaranteed to start with /
	return rp.Host + rp.Path
}

// ParseRouteSpec parses a string route specification
func ParseRouteSpec(s string) (*RouteSpec, error) {
	seq := strings.SplitN(s, "=", 2)
	var path, value, host string
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
	isURL, err := checkURL(value)
	if err != nil {
		return nil, err
	}
	return &RouteSpec{host, path, value, isURL}, nil
}
