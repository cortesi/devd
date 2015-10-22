package inject

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
)

func inject(ci CopyInject, data string) (found bool, dstdata string, err error) {
	src := bytes.NewBuffer([]byte(data))
	dst := bytes.NewBuffer(make([]byte, 0))
	injector, err := ci.Sniff(src)
	if err != nil {
		return false, "", err
	}
	_, err = injector.Copy(dst)
	if err != nil {
		return false, "", err
	}
	return injector.Found, string(dst.Bytes()), nil
}

func TestReverseProxy(t *testing.T) {
	var sniffTests = []struct {
		snifflen int
		marker   string
		payload  string

		src    string
		result string
	}{
		{0, "mark", "inject", "nomatch", "nomatch"},
		{1, "mark", "inject", "nomatch", "nomatch"},
		{10, "mark", "inject", "nomatch", "nomatch"},
		{100, "mark", "inject", "nomatch", "nomatch"},
		{10, "mark", "inject", "imarki", "iinjectmarki"},
		{5, "mark", "inject", "imarki", "iinjectmarki"},
		{4, "mark", "inject", "marki", "injectmarki"},
		{10, "mark", "inject", "imark", "iinjectmark"},
		{5, "mark", "inject", "imark", "iinjectmark"},
		{100, "mark", "inject", "imark", "iinjectmark"},
	}
	for i, tt := range sniffTests {
		ci := CopyInject{
			Within:  tt.snifflen,
			Marker:  regexp.MustCompile(tt.marker),
			Payload: []byte(tt.payload),
		}
		found, dst, err := inject(ci, tt.src)

		// Sanity checkss
		if err != nil {
			t.Errorf("Test %d, unexpected error:\n%s\n", i, err)
		}
		if found && strings.Index(dst, tt.payload) == -1 {
			t.Errorf(
				"Test %d, payload not found.", i,
			)
		}
		var expected int
		if found {
			expected = len(tt.src) + len(tt.payload)
		} else {
			expected = len(tt.src)
		}
		if len(dst) != expected {
			t.Errorf(
				"Test %d, expected %d bytes copied, found %d", i, len(dst), expected,
			)
		}
		if dst != tt.result {
			t.Errorf("Test %d, expected '%v', got '%v'", i, tt.result, dst)
		}

		// Idempotence
		found, dst2, err := inject(ci, dst)
		if err != nil {
			t.Errorf("Test %d, unexpected error:\n%s\n", i, err)
		}
		if found {
			t.Errorf("Test %d, idempotence violation", i)
		}
		if dst != dst2 {
			t.Errorf("Test %d, idempotence violation", i)
		}
	}
}

func TestNil(t *testing.T) {
	ci := CopyInject{}
	val := "onetwothree"
	src := bytes.NewBuffer([]byte(val))
	injector, err := ci.Sniff(src)
	if injector.Found || err != nil {
		t.Error("Unexpected")
	}
	dst := bytes.NewBuffer(make([]byte, 0))
	injector.Copy(dst)
	if string(dst.Bytes()) != val {
		t.Errorf("Expected %s, got %s", val, string(dst.Bytes()))
	}
}
