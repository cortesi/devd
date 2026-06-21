package devd

import (
	"os"
	"path"
	"testing"
)

func TestGenerateCert(t *testing.T) {
	d, err := os.MkdirTemp("", "devdtest")
	if err != nil {
		t.Error(err)
		return
	}
	defer func() { _ = os.Remove(d) }()
	dst := path.Join(d, "certbundle")
	err = GenerateCert(dst)
	if err != nil {
		t.Error(err)
	}

	_, err = getTLSConfig(dst)
	if err != nil {
		t.Error(err)
	}
}
