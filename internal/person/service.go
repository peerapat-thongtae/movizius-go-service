package person

import (
	"context"
	"fmt"

	"github.com/peera/movizius-go-service/internal/movie"
	"github.com/peera/movizius-go-service/internal/shared/response"
	"github.com/peera/movizius-go-service/internal/tv"
	"github.com/peera/movizius-go-service/pkg/tmdb"
)

// Service loads person metadata from TMDB and limits credits to the local catalog.
type Service struct {
	movies movie.MovieRepository
	tv     tv.TVRepository
	tmdb   *tmdb.Client
}

// NewService constructs a person service.
func NewService(movies movie.MovieRepository, tvRepo tv.TVRepository, tmdbClient *tmdb.Client) *Service {
	return &Service{movies: movies, tv: tvRepo, tmdb: tmdbClient}
}

// Search returns matching people from TMDB without requiring them to exist in
// the local movie or TV catalog.
func (s *Service) Search(ctx context.Context, query string, page int) (*response.Page[SearchResult], error) {
	var result tmdb.SearchPage[SearchResult]
	if err := s.tmdb.SearchPerson(ctx, query, page, &result); err != nil {
		return nil, fmt.Errorf("person service: search tmdb: %w", err)
	}

	return &response.Page[SearchResult]{
		Page:         result.Page,
		TotalPages:   result.TotalPages,
		TotalResults: result.TotalResults,
		Results:      result.Results,
	}, nil
}

// GetByID fetches the TMDB person and only returns filmography credits which
// already exist in the local movie or TV catalog.
func (s *Service) GetByID(ctx context.Context, id int64) (*Response, error) {
	var result Response
	if err := s.tmdb.GetPersonDetail(ctx, id, &result.Person); err != nil {
		return nil, fmt.Errorf("person service: get person %d: %w", id, err)
	}
	if err := s.tmdb.GetPersonCombinedCredits(ctx, id, &result.Credits); err != nil {
		return nil, fmt.Errorf("person service: get credits for %d: %w", id, err)
	}

	movieIDs, tvIDs := creditIDs(result.Credits)
	movies, err := s.movies.FindByTMDBIDs(ctx, movieIDs)
	if err != nil {
		return nil, fmt.Errorf("person service: find movie credits: %w", err)
	}
	series, err := s.tv.FindByTMDBIDs(ctx, tvIDs)
	if err != nil {
		return nil, fmt.Errorf("person service: find tv credits: %w", err)
	}

	result.Credits.Cast = filterCredits(result.Credits.Cast, movies, series)
	result.Credits.Crew = filterCredits(result.Credits.Crew, movies, series)
	return &result, nil
}

func creditIDs(credits Credits) ([]int64, []int64) {
	movieIDs := make(map[int64]struct{})
	tvIDs := make(map[int64]struct{})
	collect := func(items []Credit) {
		for _, credit := range items {
			switch credit.MediaType {
			case "movie":
				movieIDs[credit.ID] = struct{}{}
			case "tv":
				tvIDs[credit.ID] = struct{}{}
			}
		}
	}
	collect(credits.Cast)
	collect(credits.Crew)
	return mapKeys(movieIDs), mapKeys(tvIDs)
}

func mapKeys(ids map[int64]struct{}) []int64 {
	keys := make([]int64, 0, len(ids))
	for id := range ids {
		keys = append(keys, id)
	}
	return keys
}

func filterCredits(credits []Credit, movies map[int64]movie.Movie, series map[int64]tv.TV) []Credit {
	filtered := make([]Credit, 0, len(credits))
	for _, credit := range credits {
		switch credit.MediaType {
		case "movie":
			if _, ok := movies[credit.ID]; ok {
				filtered = append(filtered, credit)
			}
		case "tv":
			if _, ok := series[credit.ID]; ok {
				filtered = append(filtered, credit)
			}
		}
	}
	return filtered
}
