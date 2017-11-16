package filter

import (
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
)

var filterFilesTests = []struct {
	includes []string
	excludes []string
	files    []string
	expected []string
	err      bool
}{
	{
		nil,
		[]string{"*"},
		[]string{"main.cpp", "main.go", "main.h", "foo.go", "bar.py"},
		[]string{},
		false,
	},
	{
		[]string{"*"},
		nil,
		[]string{"main.cpp", "main.go", "main.h", "foo.go", "bar.py"},
		[]string{"main.cpp", "main.go", "main.h", "foo.go", "bar.py"},
		false,
	},
	{
		[]string{"*"},
		[]string{"*.go"},
		[]string{"main.cpp", "main.go", "main.h", "foo.go", "bar.py"},
		[]string{"main.cpp", "main.h", "bar.py"},
		false,
	},
	{
		[]string{"main.*"},
		[]string{"*.cpp"},
		[]string{"main.cpp", "main.go", "main.h", "foo.go", "bar.py"},
		[]string{"main.go", "main.h"},
		false,
	},
	{
		nil, nil,
		[]string{"main.cpp", "main.go", "main.h", "foo.go", "bar.py"},
		[]string{},
		false,
	},

	{
		[]string{"**/*"},
		nil,
		[]string{"foo", "/test/foo", "/test/foo.go"},
		[]string{"foo", "/test/foo", "/test/foo.go"},
		false,
	},
}

func TestFilterFiles(t *testing.T) {
	for i, tt := range filterFilesTests {
		t.Run(fmt.Sprintf("%.2d", i), func(t *testing.T) {
			result, err := Files(tt.files, tt.includes, tt.excludes)
			if !tt.err && err != nil {
				t.Errorf("Test %d: error %s", i, err)
			}
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf(
					"Test %d (inc: %v, ex: %v), expected \"%v\" got \"%v\"",
					i, tt.includes, tt.excludes, tt.expected, result,
				)
			}
		})
	}
}

var SplitPatternTests = []struct {
	pattern  string
	expected string
}{
	{"foo", "foo"},
	{"test/foo", "test/foo"},
	{"test/foo*", "test/foo"},
	{"test/*.**", "test/"},
	{"**/*", ""},
	{"foo*/bar", "foo"},
	{"foo/**/bar", "foo/"},
	{"/voing/**", "/voing/"},
}

func TestSplitPattern(t *testing.T) {
	for i, tt := range SplitPatternTests {
		bdir, _ := SplitPattern(tt.pattern)
		if filepath.ToSlash(bdir) != filepath.ToSlash(tt.expected) {
			t.Errorf("%d: %q - Expected %q, got %q", i, tt.pattern, tt.expected, bdir)
		}
	}
}
