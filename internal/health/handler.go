package health

import (
	"net/http"

	"github.com/peera/movizius-go-service/internal/shared/response"
)

// Handler exposes the health endpoints over HTTP.
type Handler struct {
	service *Service
}

// NewHandler constructs a health Handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// RegisterRoutes binds the health routes onto the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", h.Check)
}

// Check returns the current service health.
//
//	@Summary		Health check
//	@Description	Returns service status.
//	@Tags			health
//	@Produce		json
//	@Success		200	{object}	health.Status
//	@Router			/health [get]
func (h *Handler) Check(w http.ResponseWriter, r *http.Request) {
	status := h.service.Status(r.Context())
	response.Success(w, http.StatusOK, status)
}
