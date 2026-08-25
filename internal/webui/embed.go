package webui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:dist
var staticFiles embed.FS

// GetStaticFS returns the embedded filesystem for the frontend assets
func GetStaticFS() (http.FileSystem, error) {
	// Get the dist subdirectory
	distFS, err := fs.Sub(staticFiles, "dist")
	if err != nil {
		return nil, err
	}
	return http.FS(distFS), nil
}

// StaticHandler returns an http.Handler for serving the embedded frontend
func StaticHandler() (http.Handler, error) {
	fs, err := GetStaticFS()
	if err != nil {
		return nil, err
	}
	return http.FileServer(fs), nil
}
