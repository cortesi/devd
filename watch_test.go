package devd

import (
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
		err := false

		if len(result) != len(tt.expected) {
			err = true
		} else {
			for j := range result {
				if result[j] != tt.expected[j] {
					err = true
					break
				}
			}
		}

		if err {
			t.Errorf("Test %d (pattern %s), expected \"%s\" got \"%s\"", i, tt.pattern, tt.expected, result)
		}
	}
}
