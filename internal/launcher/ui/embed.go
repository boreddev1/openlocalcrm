package ui

import (
	"embed"
	"net/http"
)

//go:embed index.html style.css app.js
var DistFS embed.FS

func Handler() http.Handler {
	return http.FileServer(http.FS(DistFS))
}
