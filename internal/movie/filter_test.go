package movie

import "testing"

func TestHasDeniedKeyword(t *testing.T) {
	tests := []struct {
		name     string
		keywords *KeywordsWrapper
		want     bool
	}{
		{name: "nil", keywords: nil, want: false},
		{name: "other keyword", keywords: &KeywordsWrapper{Keywords: []Keyword{{ID: 1}}}, want: false},
		{name: "denied keyword", keywords: &KeywordsWrapper{Keywords: []Keyword{{ID: deniedKeywordIDs[0]}}}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasDeniedKeyword(tt.keywords); got != tt.want {
				t.Errorf("hasDeniedKeyword() = %v, want %v", got, tt.want)
			}
		})
	}
}
