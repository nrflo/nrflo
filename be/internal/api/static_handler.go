package api

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// spaHandler returns an http.Handler that serves static files from fsys
// with SPA fallback (index.html for unknown paths). Returns nil if fsys
// has no index.html (dev mode with no UI build).
func spaHandler(fsys fs.FS) http.Handler {
	// Check if index.html exists; if not, no UI was built
	if _, err := fs.Stat(fsys, "index.html"); err != nil {
		return nil
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Never intercept API paths — let the mux return 404 naturally
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}

		// Clean the path, strip leading slash
		p := path.Clean(strings.TrimPrefix(r.URL.Path, "/"))
		if p == "." {
			p = "index.html"
		}

		// Try to open the exact file
		f, err := fsys.Open(p)
		if err == nil {
			f.Close()
			// Hashed assets get long cache
			if strings.HasPrefix(p, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			http.FileServerFS(fsys).ServeHTTP(w, r)
			return
		}

		// SPA fallback: serve index.html with no-cache
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		indexFile, err := fs.ReadFile(fsys, "index.html")
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexFile)
	})
}
