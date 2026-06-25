package tv

import (
	"net/http"

	"github.com/peera/movizius-go-service/internal/shared/middleware"
	"github.com/peera/movizius-go-service/internal/shared/response"
)

// Handler exposes TV endpoints over HTTP.
type Handler struct {
	service *TVService
}

// NewHandler constructs a TV Handler.
func NewHandler(service *TVService) *Handler {
	return &Handler{service: service}
}

// RegisterRoutes binds TV routes onto the given mux.
// auth is applied to every protected route in this feature.
func (h *Handler) RegisterRoutes(mux *http.ServeMux, auth func(http.Handler) http.Handler) {
	mux.Handle("GET /tv/states", auth(http.HandlerFunc(h.GetStates)))
}

// GetStates returns all TV tracking records for the authenticated user.
func (h *Handler) GetStates(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	states, err := h.service.GetStates(r.Context(), userID)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to fetch tv states")
		return
	}

	response.Success(w, http.StatusOK, states)
}
