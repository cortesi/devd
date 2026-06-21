package templates

import (
	"embed"
)

//go:embed 404.html
//go:embed dirlist.html
var FS embed.FS
