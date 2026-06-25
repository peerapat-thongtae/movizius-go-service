package movie

import (
	"net/http"

	"github.com/peera/movizius-go-service/internal/shared/middleware"
	"github.com/peera/movizius-go-service/internal/shared/response"
)

// Handler exposes movie endpoints over HTTP.
type Handler struct {
	service *MovieService
}

// NewHandler constructs a movie Handler.
func NewHandler(service *MovieService) *Handler {
	return &Handler{service: service}
}

// RegisterRoutes binds movie routes onto the given mux.
// auth is applied to every protected route in this feature.
func (h *Handler) RegisterRoutes(mux *http.ServeMux, auth func(http.Handler) http.Handler) {
	mux.Handle("GET /movie/states", auth(http.HandlerFunc(h.GetStates)))
}

// GetStates returns all movie tracking records for the authenticated user.
func (h *Handler) GetStates(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	states, err := h.service.GetStates(r.Context(), userID)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to fetch movie states")
		return
	}

	response.Success(w, http.StatusOK, states)
}
