package web

import (
	"embed"
	"fmt"
	"io/fs"
)

//go:embed static/htmx-1.9.12.min.js static/application.js static/output.css
var assets embed.FS

func StaticFiles() (fs.FS, error) {
	files, err := fs.Sub(assets, "static")
	if err != nil {
		return nil, fmt.Errorf("open embedded static files: %w", err)
	}
	return files, nil
}
