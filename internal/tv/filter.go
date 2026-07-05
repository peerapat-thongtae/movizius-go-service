package tv

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

var earlyStageStatuses = map[string]bool{
	"Planned":     true,
	"In Production": true,
}

func isAcceptableTV(detail TVResponse) bool {
	genreIDs := make([]int64, 0, len(detail.Genres))
	for _, g := range detail.Genres {
		genreIDs = append(genreIDs, g.ID)
	}
	return acceptableTV(detail.Adult, detail.Type, detail.OriginalLanguage, detail.Status, detail.Popularity, genreIDs)
}

// isAcceptableTVDoc evaluates acceptability against a stored tv document. The adult flag is not
// persisted, so it is treated as false (adult titles are already filtered out on insert).
func isAcceptableTVDoc(t TV) bool {
	var (
		typ        string
		popularity float64
	)
	if t.Type != nil {
		typ = *t.Type
	}
	if t.Popularity != nil {
		popularity = *t.Popularity
	}
	return acceptableTV(false, typ, t.OriginalLanguage, t.Status, popularity, t.Genres)
}

// acceptableTV holds the shared tv acceptability rules over primitive fields.
func acceptableTV(adult bool, typ, lang, status string, popularity float64, genreIDs []int64) bool {
	if adult {
		return false
	}
	if typ != "" && typ != "Miniseries" && typ != "Scripted" {
		return false
	}
	if !availableLanguages[lang] {
		return false
	}
	if len(genreIDs) == 0 {
		return false
	}
	for _, id := range genreIDs {
		if unwantedGenreIDs[id] {
			return false
		}
	}
	if popularity <= 5 && !earlyStageStatuses[status] {
		return false
	}
	return true
}
