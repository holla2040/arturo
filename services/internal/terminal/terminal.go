package terminal

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"
)

//go:embed static
var embedded embed.FS

// Handler returns an http.Handler that serves the terminal UI and
// reverse-proxies all other requests to the controller at controllerURL.
// When devMode is true, static files are served from disk and an SSE
// endpoint at /dev/reload pushes change notifications to the browser.
func Handler(controllerURL string, devMode bool) http.Handler {
	target, err := url.Parse(controllerURL)
	if err != nil {
		panic("terminal: invalid controller URL: " + err.Error())
	}
	proxy := httputil.NewSingleHostReverseProxy(target)

	// Choose filesystem: embed or disk.
	var staticFS http.FileSystem
	if devMode {
		staticFS = http.Dir("internal/terminal/static")
	} else {
		sub, err := fs.Sub(embedded, "static")
		if err != nil {
			panic("terminal: embed sub failed: " + err.Error())
		}
		staticFS = http.FS(sub)
	}

	mux := http.NewServeMux()

	// Serve static assets.
	fileServer := http.FileServer(staticFS)
	mux.Handle("GET /static/", http.StripPrefix("/static/", fileServer))

	// Live-reload snippet injected in dev mode.
	const reloadSnippet = `<script>(function(){` +
		`var es=new EventSource('/dev/reload');` +
		`es.onmessage=function(){location.reload();};` +
		`es.onerror=function(){setTimeout(function(){location.reload();},2000);};` +
		`})()</script>`

	// Serve index.html at exactly "/".
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		f, err := staticFS.Open("index.html")
		if err != nil {
			http.Error(w, "index.html not found", http.StatusInternalServerError)
			return
		}
		defer f.Close()

		buf := new(bytes.Buffer)
		buf.ReadFrom(f)
		data := buf.Bytes()

		if devMode {
			// Inject live-reload script before </body>.
			data = bytes.Replace(data, []byte("</body>"), []byte(reloadSnippet+"\n</body>"), 1)
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if devMode {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		}
		w.Write(data)
	})

	// Dev-only SSE endpoint.
	if devMode {
		dw := newDirWatcher("internal/terminal/static")
		go dw.run()
		mux.HandleFunc("GET /dev/reload", dw.serveSSE)
	}

	// Everything else goes to the controller.
	mux.Handle("/", proxy)

	return mux
}

// dirWatcher polls static files for changes and notifies SSE clients.
type dirWatcher struct {
	dir     string
	mu      sync.Mutex
	clients []chan struct{}
	hashes  map[string][32]byte
}

func newDirWatcher(dir string) *dirWatcher {
	return &dirWatcher{
		dir:    dir,
		hashes: make(map[string][32]byte),
	}
}

func (dw *dirWatcher) run() {
	// Initial hash snapshot.
	dw.snapshot()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		if dw.changed() {
			dw.notify()
		}
	}
}

func (dw *dirWatcher) snapshot() {
	dw.hashes = dw.hashFiles()
}

func (dw *dirWatcher) hashFiles() map[string][32]byte {
	out := make(map[string][32]byte)
	filepath.Walk(dw.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		out[path] = sha256.Sum256(data)
		return nil
	})
	return out
}

func (dw *dirWatcher) changed() bool {
	current := dw.hashFiles()
	if len(current) != len(dw.hashes) {
		dw.hashes = current
		return true
	}
	for k, v := range current {
		if old, ok := dw.hashes[k]; !ok || old != v {
			dw.hashes = current
			return true
		}
	}
	return false
}

func (dw *dirWatcher) notify() {
	dw.mu.Lock()
	defer dw.mu.Unlock()
	for _, ch := range dw.clients {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (dw *dirWatcher) addClient() chan struct{} {
	ch := make(chan struct{}, 1)
	dw.mu.Lock()
	dw.clients = append(dw.clients, ch)
	dw.mu.Unlock()
	return ch
}

func (dw *dirWatcher) removeClient(ch chan struct{}) {
	dw.mu.Lock()
	defer dw.mu.Unlock()
	for i, c := range dw.clients {
		if c == ch {
			dw.clients = append(dw.clients[:i], dw.clients[i+1:]...)
			break
		}
	}
}

func (dw *dirWatcher) serveSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := dw.addClient()
	defer dw.removeClient(ch)

	log.Println("dev: live-reload client connected")

	for {
		select {
		case <-ch:
			fmt.Fprintf(w, "data: reload\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}
