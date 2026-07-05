package datasync

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/peera/movizius-go-service/internal/shared/response"
)

type syncByIDsRequest struct {
	IDs []int64 `json:"ids"`
}

const defaultLimit = 20

// Handler handles HTTP requests for sync operations.
type Handler struct {
	service *SyncService
	log     *slog.Logger
}

// NewHandler constructs a Handler.
func NewHandler(service *SyncService, log *slog.Logger) *Handler {
	return &Handler{service: service, log: log}
}

// RegisterRoutes registers sync endpoints on the provided mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux, auth func(http.Handler) http.Handler) {
	mux.HandleFunc("POST /sync/movie/by-ids", h.SyncMovieByIDs)
	mux.HandleFunc("POST /sync/tv/by-ids", h.SyncTVByIDs)
	mux.HandleFunc("POST /sync/movie/tracked", h.SyncMovieUserTracked)
	mux.HandleFunc("POST /sync/tv/tracked", h.SyncTVUserTracked)
	mux.HandleFunc("POST /sync/movie/trending", h.SyncTrendingMovie)
	mux.HandleFunc("POST /sync/tv/trending", h.SyncTrendingTV)
	mux.HandleFunc("POST /sync/movie/changes", h.SyncChangesMovie)
	mux.HandleFunc("POST /sync/tv/changes", h.SyncChangesTV)
	mux.HandleFunc("POST /sync/tv/tvmaze-schedule", h.SyncTVMazeSchedule)
	mux.HandleFunc("POST /sync/movie/cleanup-fields", h.CleanupMovieFields)
	mux.HandleFunc("POST /sync/tv/cleanup-fields", h.CleanupTVFields)
}

// SyncMovieByIDs syncs TMDB metadata for the movie IDs provided in the request body.
func (h *Handler) SyncMovieByIDs(w http.ResponseWriter, r *http.Request) {
	var req syncByIDsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.IDs) == 0 {
		response.Error(w, http.StatusBadRequest, "ids is required")
		return
	}
	result, err := h.service.SyncByIDs(r.Context(), "movie", req.IDs, parseSkipAcceptable(r))
	if err != nil {
		h.log.Error("sync operation failed", "error", err, "path", r.URL.Path)
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(w, http.StatusOK, result)
}

// SyncTVByIDs syncs TMDB metadata for the TV IDs provided in the request body.
func (h *Handler) SyncTVByIDs(w http.ResponseWriter, r *http.Request) {
	var req syncByIDsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.IDs) == 0 {
		response.Error(w, http.StatusBadRequest, "ids is required")
		return
	}
	result, err := h.service.SyncByIDs(r.Context(), "tv", req.IDs, parseSkipAcceptable(r))
	if err != nil {
		h.log.Error("sync operation failed", "error", err, "path", r.URL.Path)
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(w, http.StatusOK, result)
}

// SyncMovieUserTracked syncs TMDB metadata for movie IDs tracked in movie_user.
func (h *Handler) SyncMovieUserTracked(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.SyncFromUserTracked(r.Context(), "sync_movie_user_tracked", "movie", parseFrequency(r), parseLimit(r), parseSkipAcceptable(r))
	if err != nil {
		h.log.Error("sync operation failed", "error", err, "path", r.URL.Path)
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(w, http.StatusOK, result)
}

// SyncTVUserTracked syncs TMDB metadata for TV IDs tracked in tv_user.
func (h *Handler) SyncTVUserTracked(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.SyncFromUserTracked(r.Context(), "sync_tv_user_tracked", "tv", parseFrequency(r), parseLimit(r), parseSkipAcceptable(r))
	if err != nil {
		h.log.Error("sync operation failed", "error", err, "path", r.URL.Path)
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
	result, err := h.service.SyncFromTMDBTrending(r.Context(), "sync_movie_trending", "movie", timeWindow, parseFrequency(r), parseLimit(r), parseSkipAcceptable(r))
	if err != nil {
		h.log.Error("sync operation failed", "error", err, "path", r.URL.Path)
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
	result, err := h.service.SyncFromTMDBTrending(r.Context(), "sync_trending_tv", "tv", timeWindow, parseFrequency(r), parseLimit(r), parseSkipAcceptable(r))
	if err != nil {
		h.log.Error("sync operation failed", "error", err, "path", r.URL.Path)
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(w, http.StatusOK, result)
}

// SyncChangesMovie syncs movie metadata from TMDB's changes feed (start_date = yesterday), one page at a time.
func (h *Handler) SyncChangesMovie(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.SyncFromTMDBChanges(r.Context(), "sync_movie_changes", "movie", parseFrequency(r), parseLimit(r), parseSkipAcceptable(r))
	if err != nil {
		h.log.Error("sync operation failed", "error", err, "path", r.URL.Path)
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(w, http.StatusOK, result)
}

// SyncChangesTV syncs TV metadata from TMDB's changes feed (start_date = yesterday), one page at a time.
func (h *Handler) SyncChangesTV(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.SyncFromTMDBChanges(r.Context(), "sync_tv_changes", "tv", parseFrequency(r), parseLimit(r), parseSkipAcceptable(r))
	if err != nil {
		h.log.Error("sync operation failed", "error", err, "path", r.URL.Path)
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(w, http.StatusOK, result)
}

// SyncTVMazeSchedule fetches the TVMaze full schedule and updates next_episode_to_air.air_date.
func (h *Handler) SyncTVMazeSchedule(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.SyncTVMazeSchedule(r.Context())
	if err != nil {
		h.log.Error("sync operation failed", "error", err, "path", r.URL.Path)
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(w, http.StatusOK, result)
}

// CleanupMovieFields removes stale keys from all movie documents that are no longer in the DB model.
func (h *Handler) CleanupMovieFields(w http.ResponseWriter, r *http.Request) {
	modified, err := h.service.CleanupMovieFields(r.Context())
	if err != nil {
		h.log.Error("sync operation failed", "error", err, "path", r.URL.Path)
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(w, http.StatusOK, map[string]int64{"modified": modified})
}

// CleanupTVFields removes stale keys from all tv documents that are no longer in the DB model.
func (h *Handler) CleanupTVFields(w http.ResponseWriter, r *http.Request) {
	modified, err := h.service.CleanupTVFields(r.Context())
	if err != nil {
		h.log.Error("sync operation failed", "error", err, "path", r.URL.Path)
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(w, http.StatusOK, map[string]int64{"modified": modified})
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

// parseSkipAcceptable reads the "skipAcceptable" query param (default false).
// When true, the acceptability filter is bypassed during sync.
func parseSkipAcceptable(r *http.Request) bool {
	b, _ := strconv.ParseBool(r.URL.Query().Get("skipAcceptable"))
	return b
}

func parseLimit(r *http.Request) int {
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 {
		return n
	}
	return defaultLimit
}
