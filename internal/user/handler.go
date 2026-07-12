package user

import (
	"log/slog"
	"net/http"

	"github.com/peera/movizius-go-service/internal/shared/middleware"
	"github.com/peera/movizius-go-service/internal/shared/response"
)

// Handler exposes user endpoints over HTTP.
type Handler struct {
	service *UserService
	log     *slog.Logger
}

// NewHandler constructs a user Handler.
func NewHandler(service *UserService, log *slog.Logger) *Handler {
	return &Handler{service: service, log: log}
}

// RegisterRoutes binds user routes onto the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux, auth func(http.Handler) http.Handler) {
	mux.Handle("GET /user/me", auth(http.HandlerFunc(h.Me)))
	mux.Handle("POST /user/sync", auth(http.HandlerFunc(h.Sync)))
}

// Me returns the authenticated user's profile, creating the record on first login.
//
//	@Summary		Get current user
//	@Description	Returns the authenticated user's profile. Creates the record from Auth0 on first login.
//	@Tags			user
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	user.User
//	@Failure		401	{object}	map[string]string
//	@Failure		500	{object}	map[string]string
//	@Router			/user/me [get]
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	u, err := h.service.EnsureUser(r.Context(), userID)
	if err != nil {
		h.log.Error("failed to ensure user", "error", err, "user", userID, "path", r.URL.Path)
		response.Error(w, http.StatusInternalServerError, "failed to load user")
		return
	}

	response.Success(w, http.StatusOK, u)
}

// Sync force-refreshes the authenticated user's profile from Auth0.
//
//	@Summary		Sync current user from Auth0
//	@Description	Force-refreshes email/profile fields from the Auth0 Management API, creating the record if it doesn't exist yet.
//	@Tags			user
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	user.User
//	@Failure		401	{object}	map[string]string
//	@Failure		500	{object}	map[string]string
//	@Router			/user/sync [post]
func (h *Handler) Sync(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	u, err := h.service.SyncFromAuth0(r.Context(), userID)
	if err != nil {
		h.log.Error("failed to sync user from auth0", "error", err, "user", userID, "path", r.URL.Path)
		response.Error(w, http.StatusInternalServerError, "failed to sync user")
		return
	}

	response.Success(w, http.StatusOK, u)
}

// SyncMiddleware lazily upserts the authenticated user's record on every
// request. Failures are logged and swallowed so a sync hiccup never blocks
// the caller's actual request.
func SyncMiddleware(service *UserService, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if userID, ok := middleware.UserIDFromContext(r.Context()); ok {
				if _, err := service.EnsureUser(r.Context(), userID); err != nil {
					log.Error("failed to lazily sync user", "error", err, "user", userID, "path", r.URL.Path)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
