package tv

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/peera/movizius-go-service/internal/shared/middleware"
	"github.com/peera/movizius-go-service/internal/shared/response"
	"github.com/peera/movizius-go-service/pkg/tmdb"
)

// Handler exposes TV endpoints over HTTP.
type Handler struct {
	service *TVService
	log     *slog.Logger
}

// NewHandler constructs a TV Handler.
func NewHandler(service *TVService, log *slog.Logger) *Handler {
	return &Handler{service: service, log: log}
}

// RegisterRoutes binds TV routes onto the given mux.
// auth is applied to every protected route in this feature.
func (h *Handler) RegisterRoutes(mux *http.ServeMux, auth func(http.Handler) http.Handler) {
	mux.Handle("POST /tv", auth(http.HandlerFunc(h.UpsertState)))
	mux.Handle("POST /tv/episodes", auth(http.HandlerFunc(h.UpsertEpisodes)))
	mux.Handle("GET /tv/states", auth(http.HandlerFunc(h.GetStates)))
	mux.Handle("GET /tv/discover", auth(http.HandlerFunc(h.Discover)))
	mux.Handle("GET /tv/search", auth(http.HandlerFunc(h.Search)))
	mux.Handle("GET /tv/random", auth(http.HandlerFunc(h.Random)))
	mux.Handle("GET /tv/trending", auth(http.HandlerFunc(h.Trending)))
	mux.Handle("GET /tv/{id}", auth(http.HandlerFunc(h.GetByID)))
}

// Search searches TMDB for TV series matching a query and enriches results with cached DB data.
//
//	@Summary		Search TV series
//	@Description	Search TMDB for TV series. Results are enriched with genres and metadata from the local cache when available.
//	@Tags			tv
//	@Produce		json
//	@Security		BearerAuth
//	@Param			q		query		string	true	"Search query"
//	@Param			page	query		int		false	"Page number (default 1)"
//	@Success		200		{object}	response.Page[tv.TVResponse]
//	@Failure		400		{object}	map[string]string
//	@Failure		401		{object}	map[string]string
//	@Failure		500		{object}	map[string]string
//	@Router			/tv/search [get]
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
		h.log.Error("failed to search tv series", "error", err, "query", q, "path", r.URL.Path)
		response.Error(w, http.StatusInternalServerError, "failed to search tv series")
		return
	}

	response.Success(w, http.StatusOK, result)
}

// UpsertState creates or updates the authenticated user's TV tracking record.
// For status="watched" all episodes are fetched from TMDB and marked as watched.
//
//	@Summary		Upsert TV state
//	@Description	Set watchlist or watched status for a TV series.
//	@Tags			tv
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			body	body		tv.UpsertStateRequest	true	"TV state"
//	@Success		204		"No Content"
//	@Failure		400		{object}	map[string]string
//	@Failure		401		{object}	map[string]string
//	@Failure		500		{object}	map[string]string
//	@Router			/tv [post]
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

	if err := h.service.UpsertTVState(r.Context(), userID, req); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// UpsertEpisodes adds watched episodes to the authenticated user's TV tracking record.
//
//	@Summary		Add watched episodes
//	@Description	Mark specific episodes as watched for a TV series.
//	@Tags			tv
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			body	body		tv.UpsertEpisodesRequest	true	"Episodes to mark as watched"
//	@Success		204		"No Content"
//	@Failure		400		{object}	map[string]string
//	@Failure		401		{object}	map[string]string
//	@Failure		500		{object}	map[string]string
//	@Router			/tv/episodes [post]
func (h *Handler) UpsertEpisodes(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req UpsertEpisodesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.service.UpsertEpisodes(r.Context(), userID, req); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Random returns a page of TV series the user hasn't watchlisted/watched yet,
// sampled from TMDB's trending pool.
//
//	@Summary		Random TV suggestions
//	@Description	Returns TV series the user has no watchlist/watched record for, sampled by shuffling TMDB's trending pool (already filtered through the same acceptability rules as /trending) and excluding recently-served titles. Results are randomized on every call.
//	@Tags			tv
//	@Produce		json
//	@Security		BearerAuth
//	@Param			page			query		int		false	"Page number (echoed back, does not affect sampling; default 1)"
//	@Param			without_status	query		string	false	"Comma-separated account statuses to exclude, e.g. watchlist,watched,watching,waiting_next_ep"
//	@Param			total			query		int		false	"Number of results (default 20)"
//	@Success		200				{object}	response.Page[tv.TVResponse]
//	@Failure		401				{object}	map[string]string
//	@Failure		500				{object}	map[string]string
//	@Router			/tv/random [get]
func (h *Handler) Random(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	page := intParam(r.URL.Query().Get("page"), 1)
	if page < 1 {
		page = 1
	}
	total := intParam(r.URL.Query().Get("total"), 20)
	if total < 1 {
		total = 20
	}
	withoutStatus := stringListParam(strings.ToLower(r.URL.Query().Get("without_status")))

	results, err := h.service.Random(r.Context(), userID, total, withoutStatus)
	if err != nil {
		h.log.Error("failed to get random tv series", "error", err, "user", userID, "path", r.URL.Path)
		response.Error(w, http.StatusInternalServerError, "failed to get random tv series")
		return
	}

	response.Success(w, http.StatusOK, response.Page[TVResponse]{
		Page:         page,
		TotalResults: len(results),
		TotalPages:   1,
		Results:      results,
	})
}

// Trending returns TMDB's trending TV series for a time window, filtered to acceptable titles.
//
//	@Summary		Trending TV series
//	@Description	Returns TMDB's trending TV series for the given time window. Results are filtered through the same acceptability rules used at sync time (no adult content, unwanted genres, unsupported languages, or non-scripted types).
//	@Tags			tv
//	@Produce		json
//	@Security		BearerAuth
//	@Param			time_window	query		string	false	"day | week (default day)"
//	@Param			page		query		int		false	"Page number (default 1)"
//	@Success		200			{object}	response.Page[tv.TVResponse]
//	@Failure		400			{object}	map[string]string
//	@Failure		401			{object}	map[string]string
//	@Failure		500			{object}	map[string]string
//	@Router			/tv/trending [get]
func (h *Handler) Trending(w http.ResponseWriter, r *http.Request) {
	_, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	timeWindow := r.URL.Query().Get("time_window")
	if timeWindow == "" {
		timeWindow = "day"
	}
	if timeWindow != "day" && timeWindow != "week" {
		response.Error(w, http.StatusBadRequest, "time_window must be day or week")
		return
	}

	page := intParam(r.URL.Query().Get("page"), 1)
	if page < 1 {
		page = 1
	}

	result, err := h.service.Trending(r.Context(), timeWindow, page)
	if err != nil {
		h.log.Error("failed to fetch trending tv series", "error", err, "path", r.URL.Path)
		response.Error(w, http.StatusInternalServerError, "failed to fetch trending tv series")
		return
	}

	response.Success(w, http.StatusOK, result)
}

// GetByID returns full TMDB detail for a single TV series.
//
//	@Summary		Get TV detail
//	@Description	Returns full TMDB detail for a TV series, with vote_average/vote_count and next_episode_to_air.air_date from the local cache.
//	@Tags			tv
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"TMDB TV id"
//	@Success		200	{object}	tv.TVResponse
//	@Failure		400	{object}	map[string]string
//	@Failure		401	{object}	map[string]string
//	@Failure		404	{object}	map[string]string
//	@Failure		500	{object}	map[string]string
//	@Router			/tv/{id} [get]
func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	_, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	result, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, tmdb.ErrNotFound) {
			response.Error(w, http.StatusNotFound, "tv series not found")
			return
		}
		h.log.Error("failed to fetch tv series", "error", err, "id", id, "path", r.URL.Path)
		response.Error(w, http.StatusInternalServerError, "failed to fetch tv series")
		return
	}

	response.Success(w, http.StatusOK, result)
}

// Discover returns a paginated list of TV series from the local cache, enriched
// with full TMDB detail (credits, videos, watch providers, external IDs).
//
//	@Summary		Discover TV series
//	@Description	Browse the local TV cache with TMDB-style filters and sort. Each result is enriched with full TMDB detail via append_to_response.
//	@Tags			tv
//	@Produce		json
//	@Security		BearerAuth
//	@Param			page					query		int		false	"Page number (default 1)"
//	@Param			sort_by					query		string	false	"popularity.desc | first_air_date.desc | vote_average.desc | name.asc | max_watched_ep.desc | watchlisted_at.desc | watched_at.desc | ..."
//	@Param			with_genres				query		string	false	"Comma-separated genre IDs (AND logic)"
//	@Param			without_genres			query		string	false	"Comma-separated genre IDs to exclude"
//	@Param			first_air_date_year		query		int		false	"Filter by first air date year"
//	@Param			first_air_date.gte		query		string	false	"First air date >= (YYYY-MM-DD)"
//	@Param			first_air_date.lte		query		string	false	"First air date <= (YYYY-MM-DD)"
//	@Param			vote_average.gte		query		number	false	"Vote average >="
//	@Param			vote_average.lte		query		number	false	"Vote average <="
//	@Param			vote_count.gte			query		integer	false	"Vote count >="
//	@Param			with_original_language	query		string	false	"ISO 639-1 language code (e.g. en, th)"
//	@Param			include_adult			query		bool	false	"Include adult content (default false)"
//	@Param			softcore				query		bool	false	"Filter by softcore flag"
//	@Param			with_watch_providers	query		string	false	"Comma-separated provider IDs"
//	@Param			watch_region			query		string	false	"ISO 3166-1 country code for watch provider filter"
//	@Param			with_account_status		query		string	false	"watchlist | watching | waiting_next_ep | watched"
//	@Param			with_networks			query		string	false	"Comma-separated network IDs"
//	@Param			is_anime				query		bool	false	"Filter by anime flag"
//	@Param			with_status						query		string	false	"TV series status (e.g. Returning Series, Ended)"
//	@Param			with_type						query		string	false	"TV series type (e.g. Scripted, Documentary)"
//	@Param			with_next_episode_air_date.gte	query		string	false	"Next episode air date >= (YYYY-MM-DD or RFC3339)"
//	@Param			with_next_episode_air_date.lte	query		string	false	"Next episode air date <= (YYYY-MM-DD or RFC3339)"
//	@Success		200								{object}	response.Page[tv.TVResponse]
//	@Failure		401						{object}	map[string]string
//	@Failure		500						{object}	map[string]string
//	@Router			/tv/discover [get]
func (h *Handler) Discover(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	q := discoverQueryFromRequest(r)

	results, total, err := h.service.Discover(r.Context(), userID, q)
	if err != nil {
		h.log.Error("failed to discover tv series", "error", err, "user", userID, "path", r.URL.Path)
		response.Error(w, http.StatusInternalServerError, "failed to discover tv series")
		return
	}

	const pageSize = 20
	totalPages := max(1, (total+pageSize-1)/pageSize)

	response.Success(w, http.StatusOK, response.Page[TVResponse]{
		Page:         q.Page,
		TotalResults: total,
		TotalPages:   totalPages,
		Results:      results,
	})
}

// GetStates returns all TV tracking records for the authenticated user.
//
//	@Summary		List TV states
//	@Description	Returns all TV series watchlist/history records for the authenticated user.
//	@Tags			tv
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	response.Page[tv.TVStateResponse]
//	@Failure		401	{object}	map[string]string
//	@Failure		500	{object}	map[string]string
//	@Router			/tv/states [get]
func (h *Handler) GetStates(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	states, err := h.service.GetStates(r.Context(), userID)
	if err != nil {
		h.log.Error("failed to fetch tv states", "error", err, "user", userID, "path", r.URL.Path)
		response.Error(w, http.StatusInternalServerError, "failed to fetch tv states")
		return
	}

	response.Paginated(w, http.StatusOK, states, 1, 1)
}
