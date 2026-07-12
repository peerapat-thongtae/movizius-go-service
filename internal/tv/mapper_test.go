package tv

import "testing"

func intPtr(v int) *int { return &v }

func TestTopCastIDs(t *testing.T) {
	credits := &Credits{
		Cast: []CastMember{
			{ID: 3, Order: intPtr(2)},
			{ID: 1, Order: intPtr(0)},
			{ID: 4, Order: nil},
			{ID: 2, Order: intPtr(1)},
			{ID: 5, Order: intPtr(3)},
			{ID: 6, Order: intPtr(4)},
		},
	}
	got := topCastIDs(credits, 5)
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
	if got := topCastIDs(&Credits{}, 5); got != nil {
		t.Errorf("topCastIDs(empty, 5) = %v, want nil", got)
	}
}

func TestCreatorIDs(t *testing.T) {
	cb := []CreatedBy{{ID: 1}, {ID: 2}}
	got := creatorIDs(cb)
	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Errorf("creatorIDs() = %v, want [1 2]", got)
	}
	if got := creatorIDs(nil); got != nil {
		t.Errorf("creatorIDs(nil) = %v, want nil", got)
	}
}

func TestTVKeywordIDs(t *testing.T) {
	k := &TVKeywordsWrapper{Results: []Keyword{{ID: 10}, {ID: 20}}}
	got := tvKeywordIDs(k)
	want := []int64{10, 20}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("tvKeywordIDs() = %v, want %v", got, want)
	}
	if got := tvKeywordIDs(nil); got != nil {
		t.Errorf("tvKeywordIDs(nil) = %v, want nil", got)
	}
}
