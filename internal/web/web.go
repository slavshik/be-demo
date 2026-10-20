// Package web embeds the browser UI into the server binary.
package web

import (
	"embed"
	"net/http"
)

//go:embed index.html
var files embed.FS

// Handler serves the UI. A request for "/" gets index.html.
func Handler() http.Handler {
	return http.FileServerFS(files)
}
