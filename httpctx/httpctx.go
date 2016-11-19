// Package httpctx provides a context-aware HTTP handler adaptor
package httpctx

import (
	"net/http"
	"strings"

	"golang.org/x/net/context"
)

// Handler is a request handler with an added context
type Handler interface {
	ServeHTTPContext(context.Context, http.ResponseWriter, *http.Request)
}

// A HandlerFunc is an adaptor to turn a function in to a Handler
type HandlerFunc func(context.Context, http.ResponseWriter, *http.Request)

// ServeHTTPContext calls the underlying handler function
func (h HandlerFunc) ServeHTTPContext(
	ctx context.Context, rw http.ResponseWriter, req *http.Request,
) {
	h(ctx, rw, req)
}

// Adapter turns a context.Handler to an http.Handler
type Adapter struct {
	Ctx     context.Context
	Handler Handler
}

func (ca *Adapter) ServeHTTP(
	rw http.ResponseWriter, req *http.Request,
) {
	ca.Handler.ServeHTTPContext(ca.Ctx, rw, req)
}

// StripPrefix strips a prefix from the request URL
func StripPrefix(prefix string, h Handler) Handler {
	if prefix == "" {
		return h
	}
	return HandlerFunc(func(ctx context.Context, w http.ResponseWriter, r *http.Request) {
		if p := strings.TrimPrefix(r.URL.Path, prefix); len(p) < len(r.URL.Path) {
			r.URL.Path = p
			h.ServeHTTPContext(ctx, w, r)
		}
	})
}
