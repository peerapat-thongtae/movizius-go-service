package person

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/peera/movizius-go-service/internal/shared/middleware"
	"github.com/peera/movizius-go-service/internal/shared/response"
	"github.com/peera/movizius-go-service/pkg/tmdb"
)

// Handler exposes person endpoints over HTTP.
type Handler struct {
	service *Service
	log     *slog.Logger
}

// NewHandler constructs a person Handler.
func NewHandler(service *Service, log *slog.Logger) *Handler {
	return &Handler{service: service, log: log}
}

// RegisterRoutes binds protected person routes onto mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux, auth func(http.Handler) http.Handler) {
	mux.Handle("GET /person/{id}", auth(http.HandlerFunc(h.GetByID)))
}

// GetByID returns a TMDB person and the subset of their credits in the local catalog.
func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	if _, ok := middleware.UserIDFromContext(r.Context()); !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		response.Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	result, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, tmdb.ErrNotFound) {
			response.Error(w, http.StatusNotFound, "person not found")
			return
		}
		h.log.Error("failed to fetch person", "error", err, "id", id, "path", r.URL.Path)
		response.Error(w, http.StatusInternalServerError, "failed to fetch person")
		return
	}

	response.Success(w, http.StatusOK, result)
}
