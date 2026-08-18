package studio

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
)

//go:embed all:frontend/dist
var embeddedFS embed.FS

func (s *Server) registerStaticFileServer(mux *http.ServeMux) {
	// Try serving from disk first if in development mode
	distDir := filepath.Join("studio", "frontend", "dist")
	if info, err := os.Stat(distDir); err == nil && info.IsDir() {
		fsHandler := http.FileServer(http.Dir(distDir))
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if stringsHasPrefix(r.URL.Path, "/api") {
				http.NotFound(w, r)
				return
			}
			path := filepath.Join(distDir, r.URL.Path)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				http.ServeFile(w, r, filepath.Join(distDir, "index.html"))
				return
			}
			fsHandler.ServeHTTP(w, r)
		})
		return
	}

	// Try serving from embedded filesystem
	sub, err := fs.Sub(embeddedFS, "frontend/dist")
	if err == nil {
		fsHandler := http.FileServer(http.FS(sub))
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if stringsHasPrefix(r.URL.Path, "/api") {
				http.NotFound(w, r)
				return
			}
			f, err := sub.Open(r.URL.Path[1:])
			if err != nil {
				// Fallback to index.html for SPA routing
				indexFile, err := sub.Open("index.html")
				if err == nil {
					defer indexFile.Close()
					stat, _ := indexFile.Stat()
					if rseeker, ok := indexFile.(io.ReadSeeker); ok {
						http.ServeContent(w, r, "index.html", stat.ModTime(), rseeker)
						return
					}
				}
			} else {
				_ = f.Close()
			}
			fsHandler.ServeHTTP(w, r)
		})
		return
	}

	// Fallback message if frontend is not built
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if stringsHasPrefix(r.URL.Path, "/api") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><body style="font-family:sans-serif;background:#09090b;color:#e4e4e7;padding:40px;text-align:center;"><h2>CoCo Admin Studio API Active</h2><p style="color:#a1a1aa">Frontend UI is available at <a style="color:#10b981" href="http://localhost:5173">http://localhost:5173</a> or build via <code>npm run build</code> in <code>studio/frontend</code>.</p></body></html>`))
	})
}

func stringsHasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[0:len(prefix)] == prefix
}
