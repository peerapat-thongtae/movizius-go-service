package main

import (
	"reflect"
	"testing"
)

func TestTopNewCandidates(t *testing.T) {
	export := map[int64]float64{
		1: 100.0, // new, highest
		2: 75.5,  // new
		3: 60.0,  // already in DB — excluded
		4: 50.0,  // not > 50 — excluded
		5: 49.9,  // not > 50 — excluded
		6: 80.0,  // new
	}
	existing := map[int64]struct{}{3: {}}

	got := topNewCandidates(export, existing, minInsertPopularity, maxInsertsPerRun)
	want := []int64{1, 6, 2} // sorted by popularity desc: 100, 80, 75.5
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("topNewCandidates = %v, want %v", got, want)
	}
}

func TestTopNewCandidatesCapAndTieBreak(t *testing.T) {
	export := map[int64]float64{
		10: 90.0, // tie pop, lower id first
		20: 90.0,
		30: 55.0,
	}
	got := topNewCandidates(export, map[int64]struct{}{}, minInsertPopularity, 2)
	want := []int64{10, 20} // capped to 2; equal pop broken by ascending id
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("topNewCandidates = %v, want %v", got, want)
	}
}

func TestTopNewCandidatesEmpty(t *testing.T) {
	got := topNewCandidates(map[int64]float64{1: 10.0}, map[int64]struct{}{}, minInsertPopularity, maxInsertsPerRun)
	if len(got) != 0 {
		t.Fatalf("expected no candidates, got %v", got)
	}
}
