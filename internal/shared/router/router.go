// Package router builds the application's HTTP router and wires feature routes.
package router

import (
	"net/http"

	"github.com/peera/movizius-go-service/internal/health"
	"github.com/peera/movizius-go-service/internal/shared/response"
)

// New constructs the application mux with all feature routes registered.
func New() *http.ServeMux {
	mux := http.NewServeMux()

	// Root hello-world — keeps the Vercel deploy smoke test working.
	mux.HandleFunc("GET /{$}", root)

	// Feature wiring (constructor injection per the architecture guide).
	health.NewHandler(health.NewService()).RegisterRoutes(mux)

	return mux
}

func root(w http.ResponseWriter, r *http.Request) {
	response.Success(w, http.StatusOK, map[string]string{"message": "hello world"})
}
