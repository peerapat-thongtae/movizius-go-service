package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/peera/movizius-go-service/internal/shared/response"
)

// statusRecorder wraps http.ResponseWriter to capture the status code written by a handler.
type statusRecorder struct {
	http.ResponseWriter
	status  int
	written bool
}

func (rec *statusRecorder) WriteHeader(status int) {
	rec.status = status
	rec.written = true
	rec.ResponseWriter.WriteHeader(status)
}

func (rec *statusRecorder) Write(b []byte) (int, error) {
	if !rec.written {
		rec.status = http.StatusOK
		rec.written = true
	}
	return rec.ResponseWriter.Write(b)
}

// Recover wraps every route with panic recovery and 5xx access logging.
// It keeps the server up on a handler panic (logs the stack, returns 500) and
// logs any response with a 5xx status. Detailed error causes are logged in the
// handlers themselves; this is the safety-net/access layer.
func Recover(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

			defer func() {
				if rv := recover(); rv != nil {
					if log != nil {
						log.Error("panic recovered",
							"error", rv,
							"method", r.Method,
							"path", r.URL.Path,
							"stack", string(debug.Stack()),
						)
					}
					if !rec.written {
						response.Error(w, http.StatusInternalServerError, "internal server error")
					}
				}
			}()

			next.ServeHTTP(rec, r)

			if rec.status >= http.StatusInternalServerError && log != nil {
				log.Error("request failed",
					"method", r.Method,
					"path", r.URL.Path,
					"status", rec.status,
				)
			}
		})
	}
}
