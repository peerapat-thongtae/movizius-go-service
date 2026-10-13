package person

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

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
	mux.Handle("GET /person/search", auth(http.HandlerFunc(h.Search)))
	mux.Handle("GET /person/{id}", auth(http.HandlerFunc(h.GetByID)))
}

// Search returns people matching a TMDB query.
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	if _, ok := middleware.UserIDFromContext(r.Context()); !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		response.Error(w, http.StatusBadRequest, "q is required")
		return
	}

	page := 1
	if rawPage := r.URL.Query().Get("page"); rawPage != "" {
		if parsedPage, err := strconv.Atoi(rawPage); err == nil && parsedPage > 0 {
			page = parsedPage
		}
	}

	result, err := h.service.Search(r.Context(), query, page)
	if err != nil {
		h.log.Error("failed to search people", "error", err, "query", query, "path", r.URL.Path)
		response.Error(w, http.StatusInternalServerError, "failed to search people")
		return
	}

	response.Success(w, http.StatusOK, result)
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
