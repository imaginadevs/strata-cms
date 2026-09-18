// Package static serves the embedded single-page app for the same-origin
// deployment (the frontend dist/ is embedded into the binary at build time).
package static

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var embedded embed.FS

// Handler returns an http.Handler that serves the embedded SPA, plus a bool
// reporting whether a real frontend build is present. It "activates" only when
// an index.html is embedded (production/Docker build); otherwise it returns
// (nil, false) so the caller can keep the API-only 404 behaviour used in local
// dev, where the SPA is served by the Vite dev server instead.
//
// Behaviour when active:
//   - a request that maps to a real embedded file (index.html, assets/*, …) is
//     served directly; hashed assets get a long immutable cache;
//   - every other path falls back to index.html so client-side routes resolve
//     to the SPA.
func Handler() (http.Handler, bool) {
	dist, err := fs.Sub(embedded, "dist")
	if err != nil {
		return nil, false
	}
	return handlerFor(dist)
}

// handlerFor builds the SPA handler over any filesystem (split out for testing).
func handlerFor(dist fs.FS) (http.Handler, bool) {
	index, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		return nil, false // no real build embedded → stay API-only
	}
	files := http.FileServer(http.FS(dist))

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")

		if clean != "" {
			if info, statErr := fs.Stat(dist, clean); statErr == nil && !info.IsDir() {
				if strings.HasPrefix(clean, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(w, r)
				return
			}
		}

		// Unknown path → client-side route → serve index.html (never cached).
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(index)
	})
	return h, true
}
