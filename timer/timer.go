// Package timer adds HTTP request and response timing information to a
// context.
package timer

import (
	"fmt"
	"time"

	"golang.org/x/net/context"
)

// Timer collects request and response timing information
type Timer struct {
	// When the request headers have been received - earliest timing point can get
	// right now.
	tsRequestHeaders int64
	// When the response headers have been received
	tsResponseHeaders int64
	// When the response is completely written
	tsResponseDone int64
}

func (t Timer) String() string {
	if t.tsRequestHeaders == 0 {
		return "timer"
	}
	return fmt.Sprintf(
		"%.2fms total, %.2fms to response headers, %.2fms sending response body",
		float64(t.tsResponseDone-t.tsRequestHeaders)/1000000.0,
		float64(t.tsResponseHeaders-t.tsRequestHeaders)/1000000.0,
		float64(t.tsResponseDone-t.tsResponseHeaders)/1000000.0,
	)
}

// RequestHeaders sets the time at which request headers were received
func (t *Timer) RequestHeaders() {
	t.tsRequestHeaders = time.Now().UnixNano()
}

// ResponseHeaders sets the time at which request headers were received
func (t *Timer) ResponseHeaders() {
	t.tsResponseHeaders = time.Now().UnixNano()
}

// ResponseDone sets the time at which request headers were received
func (t *Timer) ResponseDone() {
	t.tsResponseDone = time.Now().UnixNano()
}

// NewContext creates a new context with the timer included
func (t *Timer) NewContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, "timer", t)
}

// FromContext creates a new context with the timer included
func FromContext(ctx context.Context) *Timer {
	timer, ok := ctx.Value("timer").(*Timer)
	if !ok {
		// Return a dummy timer
		return &Timer{}
	}
	return timer
}
