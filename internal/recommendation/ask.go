package recommendation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/peera/movizius-go-service/internal/movie"
	"github.com/peera/movizius-go-service/internal/shared/middleware"
	"github.com/peera/movizius-go-service/internal/shared/response"
	"github.com/peera/movizius-go-service/internal/tv"
)

type askRequest struct {
	Prompt string `json:"prompt"`
}
type askCatalogItem struct {
	ID          int64    `json:"id"`
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	PosterPath  string   `json:"poster_path"`
	Year        string   `json:"year"`
	Overview    string   `json:"overview"`
	VoteAverage float64  `json:"voteAverage"`
	Genres      []string `json:"genres"`
}
type askPick struct {
	Type   string `json:"type"`
	ID     int64  `json:"id"`
	Reason string `json:"reason"`
}
type askModelResult struct {
	Message string    `json:"message"`
	Picks   []askPick `json:"picks"`
}

func (h *Handler) Ask(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.xaiKey == "" {
		response.Error(w, http.StatusServiceUnavailable, "AI recommendations are not configured")
		return
	}
	var req askRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(strings.TrimSpace(req.Prompt)) == 0 || len(req.Prompt) > 1000 {
		response.Error(w, http.StatusBadRequest, "prompt must be between 1 and 1000 characters")
		return
	}
	movies, _, err := h.movies.Recommendations(r.Context(), userID, 1)
	if err != nil {
		h.askError(w, r, userID, err)
		return
	}
	shows, _, err := h.tv.Recommendations(r.Context(), userID, 1)
	if err != nil {
		h.askError(w, r, userID, err)
		return
	}
	catalog := make([]askCatalogItem, 0, 24)
	for _, item := range movies {
		catalog = append(catalog, movieCatalogItem(item))
		if len(catalog) == 12 {
			break
		}
	}
	for _, item := range shows {
		if len(catalog) >= 24 {
			break
		}
		catalog = append(catalog, tvCatalogItem(item))
	}
	if len(catalog) == 0 {
		response.Error(w, http.StatusUnprocessableEntity, "rate a few watched titles first so we can personalize suggestions")
		return
	}
	result, err := h.askXAI(r.Context(), req.Prompt, catalog)
	if err != nil {
		h.askError(w, r, userID, err)
		return
	}
	byKey := map[string]askCatalogItem{}
	for _, item := range catalog {
		byKey[item.Type+fmt.Sprint(item.ID)] = item
	}
	picks := make([]map[string]any, 0, 6)
	for _, pick := range result.Picks {
		if len(picks) == 6 {
			break
		}
		item, exists := byKey[pick.Type+fmt.Sprint(pick.ID)]
		if !exists || (pick.Type != "movie" && pick.Type != "tv") {
			continue
		}
		picks = append(picks, map[string]any{"id": item.ID, "type": item.Type, "title": item.Title, "poster_path": item.PosterPath, "year": item.Year, "overview": item.Overview, "voteAverage": item.VoteAverage, "reason": pick.Reason})
	}
	response.Success(w, http.StatusOK, map[string]any{"message": result.Message, "picks": picks})
}

func (h *Handler) askError(w http.ResponseWriter, r *http.Request, userID string, err error) {
	h.log.Error("AI recommendation failed", "error", err, "user", userID, "path", r.URL.Path)
	response.Error(w, http.StatusBadGateway, "AI suggestions are temporarily unavailable")
}
func movieCatalogItem(item movie.MovieResponse) askCatalogItem {
	genres := make([]string, 0, len(item.Genres))
	for _, g := range item.Genres {
		genres = append(genres, g.Name)
	}
	return askCatalogItem{ID: item.ID, Type: "movie", Title: item.Title, PosterPath: item.PosterPath, Year: dateYear(item.ReleaseDate), Overview: item.Overview, VoteAverage: item.VoteAverage, Genres: genres}
}
func tvCatalogItem(item tv.TVResponse) askCatalogItem {
	genres := make([]string, 0, len(item.Genres))
	for _, g := range item.Genres {
		genres = append(genres, g.Name)
	}
	return askCatalogItem{ID: item.ID, Type: "tv", Title: item.Name, PosterPath: item.PosterPath, Year: dateYear(item.FirstAirDate), Overview: item.Overview, VoteAverage: item.VoteAverage, Genres: genres}
}
func dateYear(date string) string {
	if len(date) >= 4 {
		return date[:4]
	}
	return ""
}

func (h *Handler) askXAI(ctx context.Context, prompt string, catalog []askCatalogItem) (askModelResult, error) {
	model := h.xaiModel
	if model == "" {
		model = "grok-4.20-0309-non-reasoning"
	}
	payload := map[string]any{"model": model, "temperature": 0.4, "max_tokens": 700, "messages": []map[string]string{{"role": "system", "content": "You are Movizius, a concise movie and TV recommendation assistant. Recommend only IDs from the supplied catalog. Return valid JSON only: {\\\"message\\\":string,\\\"picks\\\":[{\\\"type\\\":\\\"movie\\\"|\\\"tv\\\",\\\"id\\\":number,\\\"reason\\\":string}]}. Return 1 to 6 picks."}, {"role": "user", "content": mustJSON(map[string]any{"request": prompt, "catalog": catalog})}}}
	body, err := json.Marshal(payload)
	if err != nil {
		return askModelResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.x.ai/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return askModelResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+h.xaiKey)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 30 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return askModelResult{}, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 2_000))
		return askModelResult{}, fmt.Errorf("xAI status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	var completion struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(res.Body).Decode(&completion); err != nil {
		return askModelResult{}, err
	}
	if len(completion.Choices) == 0 {
		return askModelResult{}, fmt.Errorf("xAI returned no choices")
	}
	var result askModelResult
	content := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(completion.Choices[0].Message.Content, "```json"), "```"))
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return askModelResult{}, err
	}
	return result, nil
}
func mustJSON(value any) string { data, _ := json.Marshal(value); return string(data) }
