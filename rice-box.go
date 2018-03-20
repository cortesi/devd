package devd

import (
	"github.com/GeertJohan/go.rice/embedded"
	"time"
)

func init() {

	// define files
	file2 := &embedded.EmbeddedFile{
		Filename:    "404.html",
		FileModTime: time.Unix(1503017339, 0),
		Content:     string("<html>\n    <head>\n        <style>\n            p {\n                padding: 20px;\n                font-size: 3em;\n            }\n            .footer {\n                width: 100%;\n                margin-top: 2em;\n                text-align: right;\n                font-style: italic;\n            }\n        </style>\n    </head>\n    <body>\n        <p>404: Page not found</p>\n        <div class=\"footer\">\n            {{ .Version }}\n        </div>\n    </body>\n</html>\n"),
	}
	file3 := &embedded.EmbeddedFile{
		Filename:    "dirlist.html",
		FileModTime: time.Unix(1503017339, 0),
		Content:     string("<html>\n    <head>\n        <style>\n            #files {\n                border-collapse: collapse;\n            }\n            .dir a {\n                color: #0741d9;\n            }\n            .file a {\n                color: #0787d9;\n            }\n            .hidden a {\n                color: #a5b1b9;\n            }\n            #files tr {\n                border-bottom: 1px solid #c0c0c0;\n            }\n            #files td {\n                padding: 10px;\n            }\n            #files .dir .name {\n                font-weight: bold\n            }\n            #files .empty {\n                font-style: italic;\n            }\n            .footer {\n                width: 100%;\n                margin-top: 2em;\n                text-align: right;\n                font-style: italic;\n            }\n        </style>\n    </head>\n    <body>\n        <h1>{{.Name}}</h1>\n        <table id=\"files\">\n            {{ range .Files }}\n    \t\t\t<tr class=\"{{ . | fileType  }}\">\n                    <td class=\"name\">\n                        <a href=\"{{.Name}}\">{{.Name}}{{ if .IsDir }}/{{ end }}</a>\n                    </td>\n                    <td class=\"size\">{{ .Size | bytes }}</td>\n                    <td class=\"modified\">{{ .ModTime | reltime }}</td>\n                </tr>\n            {{ else }}\n                <tr><td class=\"empty\" span=\"2\">No files found.</td></tr>\n            {{ end }}\n        </table>\n        <div class=\"footer\">\n            {{ .Version }}\n        </div>\n    </body>\n</html>\n"),
	}

	// define dirs
	dir1 := &embedded.EmbeddedDir{
		Filename:   "",
		DirModTime: time.Unix(1503017339, 0),
		ChildFiles: []*embedded.EmbeddedFile{
			file2, // "404.html"
			file3, // "dirlist.html"

		},
	}

	// link ChildDirs
	dir1.ChildDirs = []*embedded.EmbeddedDir{}

	// register embeddedBox
	embedded.RegisterEmbeddedBox(`templates`, &embedded.EmbeddedBox{
		Name: `templates`,
		Time: time.Unix(1503017339, 0),
		Dirs: map[string]*embedded.EmbeddedDir{
			"": dir1,
		},
		Files: map[string]*embedded.EmbeddedFile{
			"404.html":     file2,
			"dirlist.html": file3,
		},
	})
}
