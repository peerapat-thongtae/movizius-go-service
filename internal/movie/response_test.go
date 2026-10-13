package movie

import (
	"encoding/json"
	"testing"
)

func TestMovieResponseUnmarshalCreditsAsCasts(t *testing.T) {
	var response MovieResponse
	err := json.Unmarshal([]byte(`{
		"id": 1,
		"credits": {
			"cast": [{"id": 10, "name": "On-screen actor", "character": "Hero", "order": 0}],
			"crew": [{"id": 20, "name": "Director", "job": "Director"}]
		}
	}`), &response)
	if err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if response.Casts == nil || len(response.Casts.Cast) != 1 || response.Casts.Cast[0].ID != 10 {
		t.Fatalf("Casts = %#v, want TMDB credits mapped to casts", response.Casts)
	}
	if len(response.Casts.Crew) != 1 || response.Casts.Crew[0].ID != 20 {
		t.Errorf("crew = %#v, want decoded TMDB crew", response.Casts.Crew)
	}
}
