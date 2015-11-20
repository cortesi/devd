package devd

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/cortesi/devd/timer"
	"github.com/cortesi/termlog"
	"github.com/dustin/go-humanize"
	"github.com/fatih/color"
)

// ResponseLogWriter is a ResponseWriter that logs
type ResponseLogWriter struct {
	Log         termlog.Logger
	Resp        http.ResponseWriter
	Timer       *timer.Timer
	wroteHeader bool
}

func (rl *ResponseLogWriter) logCode(code int, status string) {
	var codestr string
	switch {
	case code >= 200 && code < 300:
		codestr = color.GreenString("%d %s", code, status)
	case code >= 300 && code < 400:
		codestr = color.BlueString("%d %s", code, status)
	case code >= 400 && code < 500:
		codestr = color.YellowString("%d %s", code, status)
	case code >= 500 && code < 600:
		codestr = color.RedString("%d %s", code, status)
	default:
		codestr = fmt.Sprintf("%d %s", code, status)
	}
	cl := rl.Header().Get("content-length")
	clstr := ""
	if cl != "" {
		cli, err := strconv.Atoi(cl)
		if err != nil {
			rl.Log.Warn("Invalid content-length header")
		} else if cli > 0 {
			clstr = fmt.Sprintf("%s", humanize.Bytes(uint64(cli)))
		}
	}
	rl.Log.Say("<- %s %s", codestr, clstr)
}

// Header returns the header map that will be sent by WriteHeader.
// Changing the header after a call to WriteHeader (or Write) has
// no effect.
func (rl *ResponseLogWriter) Header() http.Header {
	return rl.Resp.Header()
}

// Write writes the data to the connection as part of an HTTP reply.
// If WriteHeader has not yet been called, Write calls WriteHeader(http.StatusOK)
// before writing the data.  If the Header does not contain a
// Content-Type line, Write adds a Content-Type set to the result of passing
// the initial 512 bytes of written data to DetectContentType.
func (rl *ResponseLogWriter) Write(data []byte) (int, error) {
	if !rl.wroteHeader {
		rl.WriteHeader(http.StatusOK)
	}
	ret, err := rl.Resp.Write(data)
	rl.Timer.ResponseDone()
	return ret, err
}

// WriteHeader sends an HTTP response header with status code.
// If WriteHeader is not called explicitly, the first call to Write
// will trigger an implicit WriteHeader(http.StatusOK).
// Thus explicit calls to WriteHeader are mainly used to
// send error codes.
func (rl *ResponseLogWriter) WriteHeader(code int) {
	rl.wroteHeader = true
	rl.logCode(code, http.StatusText(code))
	LogHeader(rl.Log, rl.Resp.Header())
	rl.Timer.ResponseHeaders()
	rl.Resp.WriteHeader(code)
	rl.Timer.ResponseDone()
}
