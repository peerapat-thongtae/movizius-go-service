package recommendation

import (
	"log/slog"
	"net/http"

	"github.com/peera/movizius-go-service/internal/shared/middleware"
	"github.com/peera/movizius-go-service/internal/shared/response"
)

// Handler exposes recommendation profile endpoints over HTTP.
type Handler struct {
	service *Service
	log     *slog.Logger
}

// NewHandler constructs a recommendation Handler.
func NewHandler(service *Service, log *slog.Logger) *Handler {
	return &Handler{service: service, log: log}
}

// RegisterRoutes binds recommendation routes onto the given mux.
// auth is applied to every protected route in this feature.
func (h *Handler) RegisterRoutes(mux *http.ServeMux, auth func(http.Handler) http.Handler) {
	mux.Handle("GET /recommendation/profile", auth(http.HandlerFunc(h.GetProfile)))
	mux.Handle("POST /recommendation/admin/recompute", auth(http.HandlerFunc(h.RecomputeAll)))
}

// GetProfile returns the authenticated user's recommendation profile.
//
//	@Summary		Get current user's recommendation profile
//	@Description	Returns the authenticated user's computed recommendation profile (genre/keyword/actor/director/collection/company/creator scores), built incrementally from watch history.
//	@Tags			recommendation
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	recommendation.Profile
//	@Failure		401	{object}	map[string]string
//	@Failure		500	{object}	map[string]string
//	@Router			/recommendation/profile [get]
func (h *Handler) GetProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	profile, err := h.service.GetProfile(r.Context(), userID)
	if err != nil {
		h.log.Error("failed to get recommendation profile", "error", err, "user", userID, "path", r.URL.Path)
		response.Error(w, http.StatusInternalServerError, "failed to get recommendation profile")
		return
	}
	response.Success(w, http.StatusOK, profile)
}

// RecomputeAll triggers a full synchronous recompute of every user's
// recommendation profile from their movie_user/tv_user tracking docs plus
// cached metadata. Admin/repair operation for backfill and post-formula-change
// reconciliation; blocking and may be slow on large user bases.
//
//	@Summary		Recompute all recommendation profiles
//	@Description	Full recompute of every user's recommendation profile from scratch. Synchronous/blocking admin operation — use after backfilling movie/tv metadata or bumping the scoring version.
//	@Tags			recommendation
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	recommendation.RecomputeResult
//	@Failure		401	{object}	map[string]string
//	@Failure		500	{object}	map[string]string
//	@Router			/recommendation/admin/recompute [post]
func (h *Handler) RecomputeAll(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	result, err := h.service.RecomputeAll(r.Context())
	if err != nil {
		h.log.Error("failed to recompute recommendation profiles", "error", err, "user", userID, "path", r.URL.Path)
		response.Error(w, http.StatusInternalServerError, "failed to recompute recommendation profiles")
		return
	}
	response.Success(w, http.StatusOK, result)
}
