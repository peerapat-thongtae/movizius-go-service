package stats

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// SummaryQuery holds the parsed/validated query params for GET /stats/summary.
type SummaryQuery struct {
	Period    string // "month", "year", or "all"
	Year      int
	Month     int    // 1-12, only set when Period == "month"
	MediaType string // "movie", "tv", or "all"
}

// summaryQueryFromRequest parses SummaryQuery from the request's URL query
// params. period and media_type default to "all" when omitted.
func summaryQueryFromRequest(r *http.Request) (SummaryQuery, error) {
	q := r.URL.Query()
	period := q.Get("period")
	if period == "" {
		period = "all"
	}
	mediaType := q.Get("media_type")
	if mediaType == "" {
		mediaType = "all"
	}
	if mediaType != "all" && mediaType != "movie" && mediaType != "tv" {
		return SummaryQuery{}, fmt.Errorf("media_type must be one of: all, movie, tv")
	}

	sq := SummaryQuery{Period: period, MediaType: mediaType}

	switch period {
	case "all":
		return sq, nil
	case "year":
		year, err := strconv.Atoi(q.Get("year"))
		if err != nil || year < 1900 {
			return SummaryQuery{}, fmt.Errorf("year is required and must be a valid year when period=year")
		}
		sq.Year = year
		return sq, nil
	case "month":
		year, err := strconv.Atoi(q.Get("year"))
		if err != nil || year < 1900 {
			return SummaryQuery{}, fmt.Errorf("year is required and must be a valid year when period=month")
		}
		month, err := strconv.Atoi(q.Get("month"))
		if err != nil || month < 1 || month > 12 {
			return SummaryQuery{}, fmt.Errorf("month is required and must be 1-12 when period=month")
		}
		sq.Year = year
		sq.Month = month
		return sq, nil
	default:
		return SummaryQuery{}, fmt.Errorf("period must be one of: all, year, month")
	}
}

// dateRange computes the [from, to) bounds for the query. A nil bound means
// unbounded (used for period=all).
func (sq SummaryQuery) dateRange() (from, to *time.Time) {
	switch sq.Period {
	case "year":
		f := time.Date(sq.Year, 1, 1, 0, 0, 0, 0, time.UTC)
		t := f.AddDate(1, 0, 0)
		return &f, &t
	case "month":
		f := time.Date(sq.Year, time.Month(sq.Month), 1, 0, 0, 0, 0, time.UTC)
		t := f.AddDate(0, 1, 0)
		return &f, &t
	default:
		return nil, nil
	}
}
