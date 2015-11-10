package devd

import (
	"io/ioutil"
	"os"
	"path"
	"testing"
)

func TestGenerateCert(t *testing.T) {
	d, err := ioutil.TempDir("", "devdtest")
	if err != nil {
		t.Error(err)
		return
	}
	defer os.Remove(d)
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
