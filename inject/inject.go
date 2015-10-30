// Package inject gives the ability to copy data and inject a payload before a
// specified marker. In order to let the user respond to the change in length,
// the API is split into two parts - Sniff checks whether the marker occurs
// within a specified number of initial bytes, and Copy sends the data to the
// destination.
//
// The package tries to avoid double-injecting a payload by checking whether
// the payload occurs within the first Within + len(Payload) bytes.
package inject

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"regexp"
)

// CopyInject copies data, and injects a payload before a specified marker
type CopyInject struct {
	// Number of initial bytes within which to search for marker
	Within int
	// A marker, BEFORE which the payload is inserted
	Marker *regexp.Regexp
	// The payload to be inserted
	Payload []byte
}

// Injector keeps injection state
type Injector struct {
	// Has the marker been found?
	Found bool

	conf        *CopyInject
	src         io.Reader
	offset      int
	sniffedData []byte
}

// Extra reports the number of extra bytes that will be injected
func (injector *Injector) Extra() int {
	if injector.Found {
		return len(injector.conf.Payload)
	}
	return 0
}

func min(a int, b int) int {
	if a > b {
		return b
	}
	return a
}

// Sniff reads the first SniffLen bytes of the source, and checks for the
// marker. Returns an Injector instance.
func (ci *CopyInject) Sniff(src io.Reader) (*Injector, error) {
	injector := &Injector{
		conf: ci,
		src:  src,
	}
	if ci.Within == 0 || ci.Marker == nil {
		return injector, nil
	}
	buf := make([]byte, ci.Within+len(ci.Payload))
	n, err := io.ReadFull(src, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return nil, fmt.Errorf("inject could not read data to sniff: %s", err)
	}
	injector.sniffedData = buf[:n]
	if bytes.Index(buf, ci.Payload) > -1 {
		return injector, nil
	}
	loc := ci.Marker.FindIndex(injector.sniffedData[:min(n, ci.Within)])
	if loc != nil {
		injector.Found = true
		injector.offset = loc[0]
	}
	return injector, nil
}

// ServeTemplate renders and serves a template to an http.ResponseWriter
func (ci *CopyInject) ServeTemplate(statuscode int, w http.ResponseWriter, t *template.Template, data interface{}) error {
	buff := bytes.NewBuffer(make([]byte, 0, 0))
	err := t.Execute(buff, data)
	if err != nil {
		return err
	}

	length := buff.Len()
	inj, err := ci.Sniff(buff)
	if err != nil {
		return err
	}
	w.Header().Set(
		"Content-Length", fmt.Sprintf("%d", length+inj.Extra()),
	)
	w.WriteHeader(statuscode)
	_, err = inj.Copy(w)
	if err != nil {
		return err
	}
	return nil
}

// Copy copies the data from src to dst, injecting the Payload if Sniff found
// the marker.
func (injector *Injector) Copy(dst io.Writer) (int64, error) {
	var preludeLen int64
	if injector.Found {
		startn, err := io.Copy(
			dst,
			bytes.NewBuffer(
				injector.sniffedData[:injector.offset],
			),
		)
		if err != nil {
			return startn, err
		}
		payloadn, err := io.Copy(dst, bytes.NewBuffer(injector.conf.Payload))
		if err != nil {
			return startn + payloadn, err
		}
		endn, err := io.Copy(
			dst, bytes.NewBuffer(injector.sniffedData[injector.offset:]),
		)
		if err != nil {
			return startn + payloadn + endn, err
		}
		preludeLen = startn + payloadn + endn
	} else {
		n, err := io.Copy(dst, bytes.NewBuffer(injector.sniffedData))
		if err != nil {
			return n, err
		}
		preludeLen = int64(len(injector.sniffedData))
	}
	n, err := io.Copy(dst, injector.src)
	return n + preludeLen, err
}
