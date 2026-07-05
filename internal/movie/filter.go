package movie

var availableLanguages = map[string]bool{
	"th": true, "en": true, "es": true, "fr": true, "ko": true,
	"us": true, "da": true, "kr": true, "jp": true, "ja": true,
	"no": true, "de": true, "pl": true, "nl": true, "ga": true,
}

var unwantedGenreIDs = map[int64]bool{
	10763: true, // News
	10764: true, // Reality
	10766: true, // Soap
	10767: true, // Talk
}

func isAcceptableMovie(detail MovieResponse) bool {
	genreIDs := make([]int64, 0, len(detail.Genres))
	for _, g := range detail.Genres {
		genreIDs = append(genreIDs, g.ID)
	}
	return acceptableMovie(detail.Adult, detail.Popularity, detail.OriginalLanguage, detail.Status, genreIDs)
}

// isAcceptableMovieDoc evaluates acceptability against a stored movie document. The adult flag is not
// persisted, so it is treated as false (adult titles are already filtered out on insert).
func isAcceptableMovieDoc(m Movie) bool {
	var popularity float64
	if m.Popularity != nil {
		popularity = *m.Popularity
	}
	return acceptableMovie(false, popularity, m.OriginalLanguage, m.Status, m.Genres)
}

// acceptableMovie holds the shared movie acceptability rules over primitive fields.
func acceptableMovie(adult bool, popularity float64, lang, status string, genreIDs []int64) bool {
	if adult {
		return false
	}
	if popularity < 2 {
		return false
	}
	if len(genreIDs) == 0 {
		return false
	}
	if !availableLanguages[lang] {
		return false
	}
	if status == "Canceled" || status == "Rumored" {
		return false
	}
	for _, id := range genreIDs {
		if unwantedGenreIDs[id] {
			return false
		}
	}
	return true
}
