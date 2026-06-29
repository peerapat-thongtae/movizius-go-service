package movie

import (
	"encoding/json"
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
	mux.Handle("POST /movie", auth(http.HandlerFunc(h.UpsertState)))
	mux.Handle("GET /movie/states", auth(http.HandlerFunc(h.GetStates)))
	mux.Handle("GET /movie/discover", auth(http.HandlerFunc(h.Discover)))
	mux.Handle("GET /movie/search", auth(http.HandlerFunc(h.Search)))
}

// Search searches TMDB for movies matching a query and enriches results with cached DB data.
//
//	@Summary		Search movies
//	@Description	Search TMDB for movies. Results are enriched with genres and metadata from the local cache when available.
//	@Tags			movies
//	@Produce		json
//	@Security		BearerAuth
//	@Param			q		query		string	true	"Search query"
//	@Param			page	query		int		false	"Page number (default 1)"
//	@Success		200		{object}	response.Page[movie.MovieResponse]
//	@Failure		400		{object}	map[string]string
//	@Failure		401		{object}	map[string]string
//	@Failure		500		{object}	map[string]string
//	@Router			/movie/search [get]
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	_, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	q := r.URL.Query().Get("q")
	if q == "" {
		response.Error(w, http.StatusBadRequest, "q is required")
		return
	}

	page := intParam(r.URL.Query().Get("page"), 1)
	if page < 1 {
		page = 1
	}

	result, err := h.service.Search(r.Context(), q, page)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to search movies")
		return
	}

	response.Success(w, http.StatusOK, result)
}

// UpsertState creates or updates the authenticated user's movie tracking record.
//
//	@Summary		Upsert movie state
//	@Description	Set watchlist or watched status for a movie.
//	@Tags			movies
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			body	body		movie.UpsertStateRequest	true	"Movie state"
//	@Success		204		"No Content"
//	@Failure		400		{object}	map[string]string
//	@Failure		401		{object}	map[string]string
//	@Failure		500		{object}	map[string]string
//	@Router			/movie [post]
func (h *Handler) UpsertState(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req UpsertStateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.service.UpsertState(r.Context(), userID, req); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetStates returns all movie tracking records for the authenticated user.
//
//	@Summary		List movie states
//	@Description	Returns all movie watchlist/history records for the authenticated user.
//	@Tags			movies
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	response.Page[movie.MovieUser]
//	@Failure		401	{object}	map[string]string
//	@Failure		500	{object}	map[string]string
//	@Router			/movie/states [get]
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

	response.Paginated(w, http.StatusOK, states, 1, 1)
}

// Discover returns a paginated list of movies from the local cache, enriched
// with full TMDB detail (casts, videos, watch providers, release dates).
//
//	@Summary		Discover movies
//	@Description	Browse the local movie cache with TMDB-style filters and sort. Each result is enriched with full TMDB detail via append_to_response.
//	@Tags			movies
//	@Produce		json
//	@Security		BearerAuth
//	@Param			page					query		int		false	"Page number (default 1)"
//	@Param			sort_by					query		string	false	"popularity.desc | release_date.desc | vote_average.desc | title.asc | watched_at.desc | watchlisted_at.desc | ..."
//	@Param			with_genres				query		string	false	"Comma-separated genre IDs (AND logic)"
//	@Param			without_genres			query		string	false	"Comma-separated genre IDs to exclude"
//	@Param			primary_release_year	query		int		false	"Filter by release year"
//	@Param			release_date.gte		query		string	false	"Release date >= (YYYY-MM-DD)"
//	@Param			release_date.lte		query		string	false	"Release date <= (YYYY-MM-DD)"
//	@Param			vote_average.gte		query		number	false	"Vote average >="
//	@Param			vote_average.lte		query		number	false	"Vote average <="
//	@Param			vote_count.gte			query		integer	false	"Vote count >="
//	@Param			with_original_language	query		string	false	"ISO 639-1 language code (e.g. en, th)"
//	@Param			include_adult			query		bool	false	"Include adult content (default false)"
//	@Param			softcore				query		bool	false	"Filter by softcore flag"
//	@Param			with_watch_providers	query		string	false	"Comma-separated provider IDs"
//	@Param			watch_region			query		string	false	"ISO 3166-1 country code for watch provider filter"
//	@Param			with_account_status		query		string	false	"watchlist | watched"
//	@Success		200						{object}	response.Page[movie.MovieResponse]
//	@Failure		401						{object}	map[string]string
//	@Failure		500						{object}	map[string]string
//	@Router			/movie/discover [get]
func (h *Handler) Discover(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	q := discoverQueryFromRequest(r)

	results, total, err := h.service.Discover(r.Context(), userID, q)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to discover movies")
		return
	}

	const pageSize = 20
	totalPages := max(1, (total+pageSize-1)/pageSize)

	response.Success(w, http.StatusOK, response.Page[MovieResponse]{
		Page:         q.Page,
		TotalResults: total,
		TotalPages:   totalPages,
		Results:      results,
	})
}
