package terminal

import (
	"embed"
	"net/http"
	"net/http/httputil"
	"net/url"
)

//go:embed index.html
var content embed.FS

// Handler returns an http.Handler that serves the embedded terminal UI at "/"
// and reverse-proxies all other requests to the controller at controllerURL.
func Handler(controllerURL string) http.Handler {
	data, _ := content.ReadFile("index.html")

	target, err := url.Parse(controllerURL)
	if err != nil {
		panic("terminal: invalid controller URL: " + err.Error())
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	mux := http.NewServeMux()

	// Serve the embedded HTML at exactly "/"
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	})

	// Everything else goes to the controller
	mux.Handle("/", proxy)

	return mux
}
