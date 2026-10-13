package stats

import (
	"context"
	"fmt"
	"sort"
)

// topN bounds how many entries each breakdown list (genres/actors/directors)
// returns, keeping the response small for users with large watch histories.
const topN = 10

// Service computes watch-history summaries from a user's tracked movies/TV.
type Service struct {
	repo Repository
}

// NewService constructs a Service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// GetSummary computes the watch-history Summary for a user over the period
// described by q.
func (s *Service) GetSummary(ctx context.Context, userID string, q SummaryQuery) (Summary, error) {
	from, to := q.dateRange()

	var movies []movieWatchRecord
	if q.MediaType == "all" || q.MediaType == "movie" {
		var err error
		movies, err = s.repo.FindWatchedMovies(ctx, userID, from, to)
		if err != nil {
			return Summary{}, fmt.Errorf("stats: get summary: %w", err)
		}
	}

	var shows []tvWatchRecord
	if q.MediaType == "all" || q.MediaType == "tv" {
		var err error
		shows, err = s.repo.FindWatchedTV(ctx, userID, from, to)
		if err != nil {
			return Summary{}, fmt.Errorf("stats: get summary: %w", err)
		}
	}

	genreCounts := make(map[int64]int)
	actorCounts := make(map[int64]int)
	directorCounts := make(map[int64]int)
	langCounts := make(map[string]int)

	var episodesWatched, watchMinutes int
	var myRatingSum, othersRatingSum float64
	var myRatingCount, othersRatingCount int

	for _, m := range movies {
		addCounts(genreCounts, m.Genres)
		addCounts(actorCounts, m.CastIDs)
		if m.DirectorID != nil {
			directorCounts[*m.DirectorID]++
		}
		if m.OriginalLanguage != "" {
			langCounts[m.OriginalLanguage]++
		}
		if m.Runtime != nil {
			watchMinutes += *m.Runtime
		}
		if m.Rating != nil {
			myRatingSum += *m.Rating
			myRatingCount++
		}
		if m.VoteAverage != nil {
			othersRatingSum += *m.VoteAverage
			othersRatingCount++
		}
	}

	for _, tv := range shows {
		addCounts(genreCounts, tv.Genres)
		addCounts(actorCounts, tv.CastIDs)
		addCounts(directorCounts, tv.CreatorIDs)
		if tv.OriginalLanguage != "" {
			langCounts[tv.OriginalLanguage]++
		}
		episodesWatched += tv.EpisodesInPeriod
		watchMinutes += avgEpisodeRunTime(tv.EpisodeRunTime) * tv.EpisodesInPeriod
		if tv.Rating != nil {
			myRatingSum += *tv.Rating
			myRatingCount++
		}
		if tv.VoteAverage != nil {
			othersRatingSum += *tv.VoteAverage
			othersRatingCount++
		}
	}

	summary := Summary{
		Period: Period{Type: q.Period, Year: q.Year, Month: q.Month, MediaType: q.MediaType},
		Totals: Totals{
			MoviesWatched:         len(movies),
			TVShowsWatched:        len(shows),
			EpisodesWatched:       episodesWatched,
			TotalWatchTimeMinutes: watchMinutes,
		},
		Ratings: Ratings{
			MyRatedCount: myRatingCount,
		},
		TopGenres:    topIDCounts(genreCounts, topN),
		TopActors:    topIDCounts(actorCounts, topN),
		TopDirectors: topIDCounts(directorCounts, topN),
		Languages:    topLanguageCounts(langCounts),
	}
	if myRatingCount > 0 {
		avg := myRatingSum / float64(myRatingCount)
		summary.Ratings.MyAverageRating = &avg
	}
	if othersRatingCount > 0 {
		avg := othersRatingSum / float64(othersRatingCount)
		summary.Ratings.OthersAverageRating = &avg
	}

	return summary, nil
}

func addCounts(counts map[int64]int, ids []int64) {
	for _, id := range ids {
		counts[id]++
	}
}

// avgEpisodeRunTime returns the mean of TMDB's episode_run_time (TMDB
// reports it as a small set of typical runtimes for the whole series, not
// per-episode), or 0 when unavailable.
func avgEpisodeRunTime(runtimes []int) int {
	if len(runtimes) == 0 {
		return 0
	}
	sum := 0
	for _, r := range runtimes {
		sum += r
	}
	return sum / len(runtimes)
}

func topIDCounts(counts map[int64]int, limit int) []IDCount {
	out := make([]IDCount, 0, len(counts))
	for id, count := range counts {
		out = append(out, IDCount{ID: id, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].ID < out[j].ID
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func topLanguageCounts(counts map[string]int) []LanguageCount {
	out := make([]LanguageCount, 0, len(counts))
	for lang, count := range counts {
		out = append(out, LanguageCount{Language: lang, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Language < out[j].Language
	})
	return out
}
