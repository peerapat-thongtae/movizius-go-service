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
	if detail.Adult {
		return false
	}
	if detail.Type != "" && detail.Type != "Miniseries" && detail.Type != "Scripted" {
		return false
	}
	if !availableLanguages[detail.OriginalLanguage] {
		return false
	}
	if len(detail.Genres) == 0 {
		return false
	}
	for _, g := range detail.Genres {
		if unwantedGenreIDs[g.ID] {
			return false
		}
	}
	if detail.Popularity <= 5 && !earlyStageStatuses[detail.Status] {
		return false
	}
	return true
}
