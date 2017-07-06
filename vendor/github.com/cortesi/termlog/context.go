package termlog

import "golang.org/x/net/context"

// NewContext creates a new context with an included Logger
func NewContext(ctx context.Context, logger Logger) context.Context {
	return context.WithValue(ctx, "termlog", logger)
}

// FromContext retrieves a Logger from a context. If no logger is present, we
// return a new silenced logger that will produce no output.
func FromContext(ctx context.Context) Logger {
	logger, ok := ctx.Value("termlog").(Logger)
	if !ok {
		l := NewLog()
		l.Quiet()
		return l
	}
	return logger
}
