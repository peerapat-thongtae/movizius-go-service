package tmdb

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// movieIDExportURLFmt is the daily TMDB movie-ID export. The date is formatted
// as MM_DD_YYYY. The file is newline-delimited JSON, one object per line, gzipped.
// It lives on files.tmdb.org and requires no authentication.
const movieIDExportURLFmt = "http://files.tmdb.org/p/exports/movie_ids_%s.json.gz"

// exportDateLayout matches the MM_DD_YYYY format in the export filename.
const exportDateLayout = "01_02_2006"

// exportScanBuffer is the max line length for the export scanner; some rows
// (long original titles) exceed bufio.Scanner's default 64KB.
const exportScanBuffer = 1024 * 1024

// exportHTTPTimeout is generous — the file is tens of MB.
const exportHTTPTimeout = 5 * time.Minute

// FetchMovieIDPopularity downloads the TMDB daily movie-ID export for the given
// date and returns a map of TMDB movie id -> popularity. If the export for the
// requested date is not yet published (404), it retries once with the previous
// day. The returned map is the source of truth for which movie ids still exist
// upstream and their current popularity.
func FetchMovieIDPopularity(ctx context.Context, date time.Time) (map[int64]float64, error) {
	result, err := fetchMovieIDPopularityFor(ctx, date)
	if err == ErrNotFound {
		// Today's export may not be published yet — fall back to yesterday.
		return fetchMovieIDPopularityFor(ctx, date.AddDate(0, 0, -1))
	}
	return result, err
}

func fetchMovieIDPopularityFor(ctx context.Context, date time.Time) (map[int64]float64, error) {
	url := fmt.Sprintf(movieIDExportURLFmt, date.Format(exportDateLayout))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("tmdb: build export request: %w", err)
	}

	client := &http.Client{Timeout: exportHTTPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tmdb: request export %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tmdb: export %s: unexpected status %d", url, resp.StatusCode)
	}

	return parseMovieIDExport(resp.Body)
}

// parseMovieIDExport reads a gzipped, newline-delimited JSON export and returns
// a map of id -> popularity. It is separated out for testability.
func parseMovieIDExport(r io.Reader) (map[int64]float64, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("tmdb: open gzip export: %w", err)
	}
	defer gz.Close()

	scanner := bufio.NewScanner(gz)
	scanner.Buffer(make([]byte, 0, 64*1024), exportScanBuffer)

	var row struct {
		ID         int64   `json:"id"`
		Popularity float64 `json:"popularity"`
	}

	result := make(map[int64]float64)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		if err := json.Unmarshal(line, &row); err != nil {
			return nil, fmt.Errorf("tmdb: decode export row: %w", err)
		}
		if row.ID != 0 {
			result[row.ID] = row.Popularity
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("tmdb: scan export: %w", err)
	}
	return result, nil
}
