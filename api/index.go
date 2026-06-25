package handler

import (
	"net/http"

	"github.com/peera/movizius-go-service/internal/shared/router"
)

// app is built once and reused across invocations of the serverless function.
var app = router.New()

// Handler is the Vercel Go Serverless Function entrypoint. It delegates every
// request to the internal application router.
func Handler(w http.ResponseWriter, r *http.Request) {
	app.ServeHTTP(w, r)
}
