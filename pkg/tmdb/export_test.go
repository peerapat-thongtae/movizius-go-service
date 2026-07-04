package tmdb

import (
	"bytes"
	"compress/gzip"
	"testing"
)

func gzipLines(t *testing.T, lines string) *bytes.Reader {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write([]byte(lines)); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return bytes.NewReader(buf.Bytes())
}

func TestParseMovieIDExport(t *testing.T) {
	body := `{"adult":false,"id":3924,"original_title":"Blondie","popularity":0.4687,"video":false}
{"adult":false,"id":11,"original_title":"Star Wars","popularity":52.3,"video":false}

{"adult":true,"id":99,"original_title":"X","popularity":1.5,"video":false}
`
	got, err := parseMovieIDExport(gzipLines(t, body))
	if err != nil {
		t.Fatalf("parseMovieIDExport: %v", err)
	}

	want := map[int64]float64{3924: 0.4687, 11: 52.3, 99: 1.5}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d: %v", len(got), len(want), got)
	}
	for id, pop := range want {
		if got[id] != pop {
			t.Errorf("id %d: got popularity %v, want %v", id, got[id], pop)
		}
	}
}

func TestParseMovieIDExportBadRow(t *testing.T) {
	if _, err := parseMovieIDExport(gzipLines(t, "not json\n")); err == nil {
		t.Fatal("expected error for malformed row, got nil")
	}
}
