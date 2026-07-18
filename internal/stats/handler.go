package stats

import (
	"log/slog"
	"net/http"

	"github.com/peera/movizius-go-service/internal/shared/middleware"
	"github.com/peera/movizius-go-service/internal/shared/response"
)

// Handler exposes watch-history stats endpoints over HTTP.
type Handler struct {
	service *Service
	log     *slog.Logger
}

// NewHandler constructs a stats Handler.
func NewHandler(service *Service, log *slog.Logger) *Handler {
	return &Handler{service: service, log: log}
}

// RegisterRoutes binds stats routes onto the given mux.
// auth is applied to every protected route in this feature.
func (h *Handler) RegisterRoutes(mux *http.ServeMux, auth func(http.Handler) http.Handler) {
	mux.Handle("GET /stats/summary", auth(http.HandlerFunc(h.GetSummary)))
}

// GetSummary returns the authenticated user's watch-history summary.
//
//	@Summary		Get watch-history summary
//	@Description	Returns totals, ratings, and top genres/actors/directors/languages across the user's watched movies and TV episodes, optionally scoped to a month or year and/or a single media type. period=month requires year and month; period=year requires year; omitting period (or period=all) covers all-time.
//	@Tags			stats
//	@Produce		json
//	@Security		BearerAuth
//	@Param			period		query		string	false	"all (default), year, or month"
//	@Param			year		query		int		false	"required when period=year or period=month"
//	@Param			month		query		int		false	"1-12, required when period=month"
//	@Param			media_type	query		string	false	"all (default), movie, or tv"
//	@Success		200	{object}	stats.Summary
//	@Failure		400	{object}	map[string]string
//	@Failure		401	{object}	map[string]string
//	@Failure		500	{object}	map[string]string
//	@Router			/stats/summary [get]
func (h *Handler) GetSummary(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	q, err := summaryQueryFromRequest(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	summary, err := h.service.GetSummary(r.Context(), userID, q)
	if err != nil {
		h.log.Error("failed to get watch summary", "error", err, "user", userID, "path", r.URL.Path)
		response.Error(w, http.StatusInternalServerError, "failed to get watch summary")
		return
	}
	response.Success(w, http.StatusOK, summary)
}
