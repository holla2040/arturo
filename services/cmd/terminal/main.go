package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/holla2040/arturo/internal/terminal"
)

func main() {
	listenAddr := flag.String("listen", ":8000", "HTTP listen address")
	controllerURL := flag.String("controller", "http://localhost:8002", "Controller URL to proxy to")
	devMode := flag.Bool("dev", false, "serve static files from disk with live reload")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	handler := terminal.Handler(*controllerURL, *devMode)

	server := &http.Server{
		Addr:    *listenAddr,
		Handler: handler,
	}

	mode := "production (embedded)"
	if *devMode {
		mode = "development (live reload)"
	}

	go func() {
		log.Printf("terminal listening on %s (controller: %s, mode: %s)", *listenAddr, *controllerURL, mode)
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("Shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(shutdownCtx)

	log.Println("Shutdown complete")
}
