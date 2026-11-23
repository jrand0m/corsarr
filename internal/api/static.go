package api

import (
	"bytes"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// spa serves the embedded UI: existing files as-is (hashed assets cached
// long), any other path gets index.html so client-side routes work. With a
// base path, index.html gets a <base href> so relative asset URLs resolve.
func spa(ui fs.FS, basePath string) http.Handler {
	files := http.FileServerFS(ui)
	index := indexHTML(ui, basePath)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "the UI only answers GET", "")
			return
		}
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p != "" && p != "index.html" && exists(ui, p) {
			if strings.HasPrefix(p, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			files.ServeHTTP(w, r)
			return
		}
		if index == nil {
			http.Error(w, "UI not built: run `just build ui`", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(index)
	})
}

func exists(ui fs.FS, p string) bool {
	st, err := fs.Stat(ui, p)
	return err == nil && !st.IsDir()
}

func indexHTML(ui fs.FS, basePath string) []byte {
	b, err := fs.ReadFile(ui, "index.html")
	if err != nil {
		return nil
	}
	base := strings.TrimSuffix(basePath, "/") + "/"
	tag := []byte(`<head><base href="` + base + `">`)
	return bytes.Replace(b, []byte("<head>"), tag, 1)
}
