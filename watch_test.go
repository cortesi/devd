package devd

import (
	"reflect"
	"testing"

	"github.com/cortesi/devd/termlog"
)

var filterFilesTests = []struct {
	pattern  string
	files    []string
	expected []string
}{
	{
		"*",
		[]string{"main.cpp", "main.go", "main.h", "foo.go", "bar.py"},
		[]string{},
	},
	{
		"*.go",
		[]string{"main.cpp", "main.go", "main.h", "foo.go", "bar.py"},
		[]string{"main.cpp", "main.h", "bar.py"},
	},
	// Invalid patterns won't match anything. This would trigger a warning at
	// runtime.
	{
		"[[",
		[]string{"main.cpp", "main.go", "main.h", "foo.go", "bar.py"},
		[]string{"main.cpp", "main.go", "main.h", "foo.go", "bar.py"},
	},
}

func TestFilterFiles(t *testing.T) {
	logger := termlog.NewLog()
	logger.Quiet()
	for i, tt := range filterFilesTests {
		result := filterFiles("", tt.files, []string{tt.pattern}, logger)
		if !reflect.DeepEqual(result, tt.expected) {
			t.Errorf(
				"Test %d (pattern %s), expected \"%v\" got \"%v\"",
				i, tt.pattern, tt.expected, result,
			)
		}
	}
}

func TestBatch(t *testing.T) {
	input := make(chan string, 2)
	input <- "nonexistent"
	input <- "./testdata/index.html"

	output := batch(0, input)
	ret := <-output

	if !reflect.DeepEqual(ret, []string{"./testdata/index.html"}) {
		t.Error("Unexpected return from batch.")
	}
}
