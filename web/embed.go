package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed dist/*
var distFS embed.FS

// DistHandler returns an http.Handler serving the embedded SPA assets
func DistHandler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}

	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		// Check if the requested file exists in the embedded filesystem
		_, err := fs.Stat(sub, path)
		if err != nil {
			// Fallback to index.html for client-side React Router
			r.URL.Path = "/"
			http.ServeFileFS(w, r, sub, "index.html")
			return
		}

		fileServer.ServeHTTP(w, r)
	})
}
