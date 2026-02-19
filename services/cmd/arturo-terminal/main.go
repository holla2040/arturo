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
	listenAddr := flag.String("listen", ":8082", "HTTP listen address")
	controllerURL := flag.String("controller", "http://localhost:8080", "Controller URL to proxy to")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	handler := terminal.Handler(*controllerURL)

	server := &http.Server{
		Addr:    *listenAddr,
		Handler: handler,
	}

	go func() {
		log.Printf("arturo-terminal listening on %s (controller: %s)", *listenAddr, *controllerURL)
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
