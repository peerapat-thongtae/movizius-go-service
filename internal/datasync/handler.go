package datasync

import (
	"net/http"
	"strconv"

	"github.com/peera/movizius-go-service/internal/shared/response"
)

const defaultLimit = 20

// Handler handles HTTP requests for sync operations.
type Handler struct {
	service *SyncService
}

// NewHandler constructs a Handler.
func NewHandler(service *SyncService) *Handler {
	return &Handler{service: service}
}

// RegisterRoutes registers sync endpoints on the provided mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux, auth func(http.Handler) http.Handler) {
	mux.Handle("POST /sync/movie/tracked", auth(http.HandlerFunc(h.SyncMovieUserTracked)))
	mux.Handle("POST /sync/tv/tracked", auth(http.HandlerFunc(h.SyncTVUserTracked)))
	mux.Handle("POST /sync/movie/trending", auth(http.HandlerFunc(h.SyncTrendingMovie)))
	mux.Handle("POST /sync/tv/trending", auth(http.HandlerFunc(h.SyncTrendingTV)))
}

// SyncMovieUserTracked syncs TMDB metadata for movie IDs tracked in movie_user.
func (h *Handler) SyncMovieUserTracked(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.SyncFromUserTracked(r.Context(), "sync_movie_user_tracked", "movie", parseFrequency(r), parseLimit(r))
	if err != nil {
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(w, http.StatusOK, result)
}

// SyncTVUserTracked syncs TMDB metadata for TV IDs tracked in tv_user.
func (h *Handler) SyncTVUserTracked(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.SyncFromUserTracked(r.Context(), "sync_tv_user_tracked", "tv", parseFrequency(r), parseLimit(r))
	if err != nil {
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(w, http.StatusOK, result)
}

// SyncTrendingMovie syncs movie metadata from TMDB trending, one page at a time.
func (h *Handler) SyncTrendingMovie(w http.ResponseWriter, r *http.Request) {
	timeWindow := r.URL.Query().Get("time_window")
	if timeWindow == "" {
		timeWindow = "week"
	}
	result, err := h.service.SyncFromTMDBTrending(r.Context(), "sync_movie_trending", "movie", timeWindow, parseFrequency(r), parseLimit(r))
	if err != nil {
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(w, http.StatusOK, result)
}

// SyncTrendingTV syncs TV metadata from TMDB trending, one page at a time.
func (h *Handler) SyncTrendingTV(w http.ResponseWriter, r *http.Request) {
	timeWindow := r.URL.Query().Get("time_window")
	if timeWindow == "" {
		timeWindow = "week"
	}
	result, err := h.service.SyncFromTMDBTrending(r.Context(), "sync_trending_tv", "tv", timeWindow, parseFrequency(r), parseLimit(r))
	if err != nil {
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(w, http.StatusOK, result)
}

func parseFrequency(r *http.Request) string {
	f := r.URL.Query().Get("frequency")
	switch f {
	case FrequencyDaily, FrequencyWeekly, FrequencyMonthly:
		return f
	default:
		return FrequencyWeekly
	}
}

func parseLimit(r *http.Request) int {
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 {
		return n
	}
	return defaultLimit
}
