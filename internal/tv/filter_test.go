package tv

import "testing"

func TestHasDeniedKeyword(t *testing.T) {
	tests := []struct {
		name     string
		keywords *TVKeywordsWrapper
		want     bool
	}{
		{name: "nil", keywords: nil, want: true},
		{name: "empty", keywords: &TVKeywordsWrapper{}, want: true},
		{name: "other keyword", keywords: &TVKeywordsWrapper{Results: []Keyword{{ID: 1}}}, want: false},
		{name: "denied keyword", keywords: &TVKeywordsWrapper{Results: []Keyword{{ID: deniedKeywordIDs[0]}}}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasDeniedKeyword(tt.keywords); got != tt.want {
				t.Errorf("hasDeniedKeyword() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAcceptableTVEarlyStage(t *testing.T) {
	baseGenres := []int64{18}
	tests := []struct {
		name       string
		lang       string
		status     string
		popularity float64
		posterPath string
		want       bool
	}{
		{name: "planned below threshold", lang: "en", status: "Planned", popularity: 9.9, posterPath: "/poster.jpg", want: false},
		{name: "planned at threshold", lang: "en", status: "Planned", popularity: 10, posterPath: "/poster.jpg", want: true},
		{name: "in production below threshold", lang: "en", status: "In Production", popularity: 3.9, posterPath: "/poster.jpg", want: false},
		{name: "in production at threshold", lang: "en", status: "In Production", popularity: 4, posterPath: "/poster.jpg", want: true},
		{name: "early stage missing poster", lang: "en", status: "In Production", popularity: 30, want: false},
		{name: "thai exception", lang: "th", status: "Planned", popularity: 0.8, posterPath: "/poster.jpg", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := acceptableTV(false, "Scripted", tt.lang, tt.status, tt.popularity, 0, tt.posterPath, baseGenres)
			if got != tt.want {
				t.Errorf("acceptableTV() = %v, want %v", got, tt.want)
			}
		})
	}
}
