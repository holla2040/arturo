package dashboard

import (
	"embed"
	"net/http"
)

//go:embed index.html
var content embed.FS

// Handler returns an http.Handler that serves the embedded dashboard.
func Handler() http.Handler {
	data, _ := content.ReadFile("index.html")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	})
}
