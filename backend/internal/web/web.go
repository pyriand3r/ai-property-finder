// Package web embeds the compiled Vue SPA into the Go binary via
// //go:embed all:dist so the backend serves it on "/" without a separate
// web server or reverse proxy (AGENTS.md tech stack: single-binary
// deployment). dist/ is populated by `npm run build`; a .gitkeep
// placeholder keeps the embed directive resolvable in a fresh checkout
// before any frontend build has run.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// SPAHandler serves assets from the embedded dist directory and falls back
// to index.html for unknown paths so client-side routing (vue-router,
// history mode) can resolve deep links. API routes are mounted separately
// and never reach this handler.
func SPAHandler(writer http.ResponseWriter, request *http.Request) {
	subFS, err := fs.Sub(distFS, "dist")
	if err != nil {
		http.NotFound(writer, request)
		return
	}
	fileServer := http.FileServer(http.FS(subFS))
	path := request.URL.Path
	if path != "/" && !fileExists(subFS, path) {
		request.URL.Path = "/"
	}
	fileServer.ServeHTTP(writer, request)
}

func fileExists(subFS fs.FS, path string) bool {
	cleaned := strings.TrimPrefix(path, "/")
	if cleaned == "" {
		return false
	}
	info, err := fs.Stat(subFS, cleaned)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
