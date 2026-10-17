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

const (
	minimumMoviePopularity           = 7
	minimumPopularityForVoteOverride = 2
	// A substantial vote history is a more stable signal than TMDB popularity,
	// which naturally falls after a title leaves the current conversation.
	minimumVotesToRetainLowPopularityMovie = 500
)

func isAcceptableMovie(detail MovieResponse) bool {
	genreIDs := make([]int64, 0, len(detail.Genres))
	for _, g := range detail.Genres {
		genreIDs = append(genreIDs, g.ID)
	}
	return acceptableMovie(detail.Adult, detail.Popularity, detail.VoteCount, detail.PosterPath, detail.OriginalLanguage, detail.Status, genreIDs)
}

// isAcceptableMovieDoc evaluates acceptability against a stored movie document. The adult flag is not
// persisted, so it is treated as false (adult titles are already filtered out on insert).
func isAcceptableMovieDoc(m Movie) bool {
	var popularity float64
	if m.Popularity != nil {
		popularity = *m.Popularity
	}
	var voteCount int64
	if m.VoteCount != nil {
		voteCount = *m.VoteCount
	}
	return acceptableMovie(false, popularity, voteCount, m.PosterPath, m.OriginalLanguage, m.Status, m.Genres)
}

// acceptableMovie holds the shared movie acceptability rules over primitive fields.
func acceptableMovie(adult bool, popularity float64, voteCount int64, posterPath, lang, status string, genreIDs []int64) bool {
	if adult {
		return false
	}
	// Keep Thai titles available even when TMDB metadata is sparse. They still
	// need a minimal popularity signal, while adult titles remain excluded.
	if lang == "th" {
		return popularity > 2
	}
	if posterPath == "" && !isUnreleasedMovieStatus(status) {
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
	// Popularity is useful for new catalog candidates but temporary. Keep titles
	// with a substantial vote history when their popularity later cools down.
	if popularity < minimumMoviePopularity && (popularity < minimumPopularityForVoteOverride || voteCount < minimumVotesToRetainLowPopularityMovie) {
		return false
	}
	return true
}

func isUnreleasedMovieStatus(status string) bool {
	switch status {
	case "Planned", "In Production", "Post Production":
		return true
	default:
		return false
	}
}
