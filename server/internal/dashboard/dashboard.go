package dashboard

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed index.html
var content embed.FS

// Handler returns an http.Handler that serves the embedded dashboard.
// It serves index.html for the root path.
func Handler() http.Handler {
	sub, _ := fs.Sub(content, ".")
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Serve index.html for the root path
		if r.URL.Path == "/" {
			r.URL.Path = "/index.html"
		}
		fileServer.ServeHTTP(w, r)
	})
}
