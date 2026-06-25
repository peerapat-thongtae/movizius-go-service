// Command api runs the Movizius HTTP server locally (and on non-Vercel hosts).
package main

import (
	"net/http"
	"os"

	"github.com/peera/movizius-go-service/internal/shared/router"
	"github.com/peera/movizius-go-service/pkg/logger"
)

func main() {
	log := logger.New()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port

	log.Info("starting server", "addr", addr)
	if err := http.ListenAndServe(addr, router.New()); err != nil {
		log.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
