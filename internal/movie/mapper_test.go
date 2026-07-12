package movie

import "testing"

func intPtr(v int) *int       { return &v }
func strPtr(v string) *string { return &v }

func TestTopCastIDs(t *testing.T) {
	casts := &Casts{
		Cast: []Cast{
			{ID: 3, Order: intPtr(2)},
			{ID: 1, Order: intPtr(0)},
			{ID: 4, Order: nil},
			{ID: 2, Order: intPtr(1)},
			{ID: 5, Order: intPtr(3)},
			{ID: 6, Order: intPtr(4)},
			{ID: 7, Order: intPtr(5)},
		},
	}
	got := topCastIDs(casts, 5)
	want := []int64{1, 2, 3, 5, 6}
	if len(got) != len(want) {
		t.Fatalf("topCastIDs() = %v, want %v", got, want)
	}
	for i, id := range want {
		if got[i] != id {
			t.Errorf("topCastIDs()[%d] = %v, want %v (full: %v)", i, got[i], id, got)
		}
	}
}

func TestTopCastIDsNil(t *testing.T) {
	if got := topCastIDs(nil, 5); got != nil {
		t.Errorf("topCastIDs(nil, 5) = %v, want nil", got)
	}
	if got := topCastIDs(&Casts{}, 5); got != nil {
		t.Errorf("topCastIDs(empty, 5) = %v, want nil", got)
	}
}

func TestFindDirectorID(t *testing.T) {
	casts := &Casts{
		Crew: []Cast{
			{ID: 1, Job: strPtr("Producer")},
			{ID: 2, Job: strPtr("Director")},
			{ID: 3, Job: strPtr("Director of Photography")},
		},
	}
	got := findDirectorID(casts)
	if got == nil || *got != 2 {
		t.Errorf("findDirectorID() = %v, want pointer to 2", got)
	}
}

func TestFindDirectorIDAbsent(t *testing.T) {
	casts := &Casts{Crew: []Cast{{ID: 1, Job: strPtr("Producer")}}}
	if got := findDirectorID(casts); got != nil {
		t.Errorf("findDirectorID() = %v, want nil", got)
	}
	if got := findDirectorID(nil); got != nil {
		t.Errorf("findDirectorID(nil) = %v, want nil", got)
	}
}

func TestKeywordIDs(t *testing.T) {
	k := &KeywordsWrapper{Keywords: []Keyword{{ID: 10}, {ID: 20}}}
	got := keywordIDs(k)
	want := []int64{10, 20}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("keywordIDs() = %v, want %v", got, want)
	}
	if got := keywordIDs(nil); got != nil {
		t.Errorf("keywordIDs(nil) = %v, want nil", got)
	}
}
