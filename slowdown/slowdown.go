// Package slowdown provides an implementation of net.Listener that limits
// bandwidth.
package slowdown

import (
	"io"
	"net"
	"time"

	"github.com/juju/ratelimit"
)

// The maximum rate you should specify for readrate or writerate.If this is too
// high, the token bucket implementation seems to break down.
var MaxRate uint = (1024 * 1024) * 1000

var blockSize = int64(1024)
var capacity = int64(blockSize * 4)

type slowReader struct {
	reader io.Reader
	bucket *ratelimit.Bucket
}

func (sr *slowReader) Read(b []byte) (n int, err error) {
	read := 0
	for read < len(b) {
		sr.bucket.Wait(blockSize)
		upper := int64(read) + blockSize
		if upper > int64(len(b)) {
			upper = int64(len(b))
		}
		slice := b[read:upper]
		n, err := sr.reader.Read(slice)
		read += n
		if err != nil || n < len(slice) {
			return read, err
		}
	}
	return read, nil
}

type slowWriter struct {
	writer io.Writer
	bucket *ratelimit.Bucket
}

func (w *slowWriter) Write(b []byte) (n int, err error) {
	written := 0
	for written < len(b) {
		w.bucket.Wait(blockSize)

		upper := int64(written) + blockSize
		if upper > int64(len(b)) {
			upper = int64(len(b))
		}
		n, err := w.writer.Write(b[written:upper])
		written += n
		if err != nil {
			return written, err
		}
	}
	return written, nil
}

// SlowConn is a slow connection
type SlowConn struct {
	conn     net.Conn
	listener *SlowListener
	reader   *slowReader
	writer   *slowWriter
}

func newSlowConn(conn net.Conn, listener *SlowListener) *SlowConn {
	return &SlowConn{
		conn,
		listener,
		&slowReader{conn, listener.readbucket},
		&slowWriter{conn, listener.writebucket},
	}
}

// Read reads data from the connection.
// Read can be made to time out and return a Error with Timeout() == true
// after a fixed time limit; see SetDeadline and SetReadDeadline.
func (sc *SlowConn) Read(b []byte) (n int, err error) {
	return sc.reader.Read(b)
}

// Write writes data to the connection.
// Write can be made to time out and return a Error with Timeout() == true
// after a fixed time limit; see SetDeadline and SetWriteDeadline.
func (sc *SlowConn) Write(b []byte) (n int, err error) {
	return sc.writer.Write(b)
}

// Close closes the connection.
// Any blocked Read or Write operations will be unblocked and return errors.
func (sc *SlowConn) Close() error {
	return sc.conn.Close()
}

// LocalAddr returns the local network address.
func (sc *SlowConn) LocalAddr() net.Addr {
	return sc.conn.LocalAddr()
}

// RemoteAddr returns the remote network address.
func (sc *SlowConn) RemoteAddr() net.Addr {
	return sc.conn.RemoteAddr()
}

// SetDeadline sets the read and write deadlines associated
// with the connection. It is equivalent to calling both
// SetReadDeadline and SetWriteDeadline.
//
// A deadline is an absolute time after which I/O operations
// fail with a timeout (see type Error) instead of
// blocking. The deadline applies to all future I/O, not just
// the immediately following call to Read or Write.
//
// An idle timeout can be implemented by repeatedly extending
// the deadline after successful Read or Write calls.
//
// A zero value for t means I/O operations will not time out.
func (sc *SlowConn) SetDeadline(t time.Time) error {
	return sc.conn.SetDeadline(t)
}

// SetReadDeadline sets the deadline for future Read calls.
// A zero value for t means Read will not time out.
func (sc *SlowConn) SetReadDeadline(t time.Time) error {
	return sc.conn.SetReadDeadline(t)
}

// SetWriteDeadline sets the deadline for future Write calls.
// Even if write times out, it may return n > 0, indicating that
// some of the data was successfully written.
// A zero value for t means Write will not time out.
func (sc *SlowConn) SetWriteDeadline(t time.Time) error {
	return sc.conn.SetWriteDeadline(t)
}

// SlowListener is a listener that limits global IO over all connections
type SlowListener struct {
	listener    net.Listener
	readbucket  *ratelimit.Bucket
	writebucket *ratelimit.Bucket
}

// NewSlowListener creates a SlowListener with specified read and write rates.
// Both the readrate and the writerate are specified in bytes per second. A
// value of 0 disables throttling.
func NewSlowListener(listener net.Listener, readrate uint, writerate uint) net.Listener {
	if readrate == 0 {
		readrate = MaxRate
	}
	if writerate == 0 {
		writerate = MaxRate
	}
	return &SlowListener{
		listener:    listener,
		readbucket:  ratelimit.NewBucketWithRate(float64(readrate), capacity),
		writebucket: ratelimit.NewBucketWithRate(float64(writerate), capacity),
	}
}

// Accept waits for and returns the next connection to the listener.
func (l *SlowListener) Accept() (net.Conn, error) {
	conn, err := l.listener.Accept()
	if err != nil {
		return nil, err
	}
	return newSlowConn(conn, l), nil
}

// Close closes the listener.
func (l *SlowListener) Close() error {
	return l.listener.Close()
}

// Addr returns the listener's network address.
func (l *SlowListener) Addr() net.Addr {
	return l.listener.Addr()
}
