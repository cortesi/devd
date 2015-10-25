// Package httpctx provides a context-aware HTTP handler adaptor
package httpctx

import (
	"net/http"

	"golang.org/x/net/context"
)

// Handler is a request handler with an added context
type Handler interface {
	ServeHTTPContext(context.Context, http.ResponseWriter, *http.Request)
}

// A HandlerFunc is an adaptor to turn a function in to a Handler
type HandlerFunc func(
	context.Context, http.ResponseWriter, *http.Request,
)

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
