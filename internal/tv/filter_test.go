package tv

import "testing"

func TestHasDeniedKeyword(t *testing.T) {
	tests := []struct {
		name     string
		keywords *TVKeywordsWrapper
		want     bool
	}{
		{name: "nil", keywords: nil, want: false},
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
