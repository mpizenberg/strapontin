package api

import (
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

func init() {
	mime.AddExtensionType(".webmanifest", "application/manifest+json")
}

// frontend serves static assets, and index.html for every other path so the
// client-side router can handle it.
func (s *Server) frontend(w http.ResponseWriter, r *http.Request) {
	if s.static == nil {
		http.NotFound(w, r)
		return
	}
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name != "" && name != "index.html" {
		if f, err := fs.Stat(s.static, name); err == nil && !f.IsDir() {
			if strings.HasPrefix(name, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			http.ServeFileFS(w, r, s.static, name)
			return
		}
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFileFS(w, r, s.static, "index.html")
}
