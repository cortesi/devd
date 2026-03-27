// Package fstmpl makes templates from a fs.FS
package fstmpl

import (
	"html/template"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/dustin/go-humanize"
)

func bytes(size int64) string {
	return humanize.Bytes(uint64(size))
}

func fileType(f os.FileInfo) string {
	if f.IsDir() {
		return "dir"
	}
	if strings.HasPrefix(f.Name(), ".") {
		return "hidden"
	}
	return "file"
}

// MustMakeTemplates makes templates, and panic on error
func MustMakeTemplates(fs fs.ReadDirFS) *template.Template {
	templates, err := MakeTemplates(fs)
	if err != nil {
		panic(err)
	}
	return templates
}

// MakeTemplates takes a fs.ReadDirFS and returns a html.Template
func MakeTemplates(fs fs.ReadDirFS) (*template.Template, error) {
	tmpl := template.New("")

	funcMap := template.FuncMap{
		"bytes":    bytes,
		"reltime":  humanize.Time,
		"fileType": fileType,
	}
	tmpl.Funcs(funcMap)

	ds, err := fs.ReadDir(".")
	if err != nil {
		return nil, err
	}

	for _, d := range ds {
		f, _ := fs.Open(d.Name())
		fbs, _ := io.ReadAll(f)

		_, err := tmpl.New(d.Name()).Parse(string(fbs))
		if err != nil {
			return nil, err
		}
	}

	return tmpl, err
}
