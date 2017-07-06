package termlog

import "bytes"

// LogTest lets test suites work with log output
type LogTest struct {
	Log  *Log
	buff *bytes.Buffer
}

// NewLogTest creates a new LogTest
func NewLogTest() *LogTest {
	outputMutex.Lock()
	defer outputMutex.Unlock()
	buff := new(bytes.Buffer)
	l := NewLog()
	l.Color(false)
	SetOutput(buff)
	return &LogTest{l, buff}
}

func (lt *LogTest) String() string {
	outputMutex.Lock()
	defer outputMutex.Unlock()
	return lt.buff.String()
}
