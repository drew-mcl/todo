// Package ui serves the built client from inside the binary.
package ui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var files embed.FS

// Handler serves the built client, falling back to index.html so client-side
// routes survive a reload. Hashed assets are cached hard; index.html never is.
func Handler() http.Handler {
	dist, err := fs.Sub(files, "dist")
	if err != nil {
		panic("ui: dist missing from the binary; run 'npm run build' in ui/ first: " + err.Error())
	}
	assets := http.FileServer(http.FS(dist))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			serveIndex(w, r, dist)
			return
		}
		if f, err := dist.Open(path); err == nil {
			f.Close()
			if strings.HasPrefix(path, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			assets.ServeHTTP(w, r)
			return
		}
		serveIndex(w, r, dist)
	})
}

func serveIndex(w http.ResponseWriter, r *http.Request, dist fs.FS) {
	index, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		http.Error(w, "The client was not built into this binary.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(index)
}
