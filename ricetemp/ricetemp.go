// Package ricetemp makes templates from a ricebox.
package ricetemp

import (
	"html/template"
	"os"

	"github.com/GeertJohan/go.rice"
	"github.com/dustin/go-humanize"
)

func bytes(size int64) string {
	return humanize.Bytes(uint64(size))
}

// MustMakeTemplates makes templates, and panic on error
func MustMakeTemplates(rb *rice.Box) *template.Template {
	templates, err := MakeTemplates(rb)
	if err != nil {
		panic(err)
	}
	return templates
}

// MakeTemplates takes a rice.Box and returns a html.Template
func MakeTemplates(rb *rice.Box) (*template.Template, error) {
	tmpl := template.New("")

	funcMap := template.FuncMap{
		"bytes":   bytes,
		"reltime": humanize.Time,
	}
	tmpl.Funcs(funcMap)

	err := rb.Walk("", func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			_, err := tmpl.New(path).Parse(rb.MustString(path))
			if err != nil {
				return err
			}
		}
		return nil
	})
	return tmpl, err
}
