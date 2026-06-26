// Package response provides helpers for writing API responses as JSON.
package response

import (
	"encoding/json"
	"net/http"
)

// Success writes data directly as JSON with the given status code.
func Success(w http.ResponseWriter, status int, data any) {
	write(w, status, data)
}

// Error writes {"message": "..."} as JSON with the given status code.
func Error(w http.ResponseWriter, status int, message string) {
	write(w, status, map[string]string{"message": message})
}

// Page is the standard paginated list shape used by all list endpoints.
type Page[T any] struct {
	Page         int `json:"page"`
	TotalResults int `json:"total_results"`
	TotalPages   int `json:"total_pages"`
	Results      []T `json:"results"`
}

// Paginated writes a Page[T] response as JSON.
// For endpoints returning all results without pagination: pass page=1, totalPages=1.
func Paginated[T any](w http.ResponseWriter, status int, items []T, page, totalPages int) {
	if items == nil {
		items = []T{}
	}
	Success(w, status, Page[T]{
		Page:         page,
		TotalResults: len(items),
		TotalPages:   totalPages,
		Results:      items,
	})
}

func write(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
