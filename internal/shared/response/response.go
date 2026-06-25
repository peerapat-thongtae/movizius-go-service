// Package response provides helpers for writing the API's standard JSON envelopes.
package response

import (
	"encoding/json"
	"net/http"
)

// envelope is the standard response shape used across the API.
type envelope struct {
	Success bool   `json:"success"`
	Data    any    `json:"data,omitempty"`
	Message string `json:"message,omitempty"`
}

// Success writes a success envelope: {"success": true, "data": ...}.
func Success(w http.ResponseWriter, status int, data any) {
	write(w, status, envelope{Success: true, Data: data})
}

// Error writes an error envelope: {"success": false, "message": ...}.
func Error(w http.ResponseWriter, status int, message string) {
	write(w, status, envelope{Success: false, Message: message})
}

func write(w http.ResponseWriter, status int, body envelope) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
