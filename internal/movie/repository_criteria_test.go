package movie

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestRecommendationCriteriaEmpty(t *testing.T) {
	if !(RecommendationCriteria{}).empty() {
		t.Errorf("empty criteria should report empty() == true")
	}
	nonEmpty := RecommendationCriteria{GenreIDs: []int64{1}}
	if nonEmpty.empty() {
		t.Errorf("criteria with GenreIDs set should report empty() == false")
	}
}

func TestReleaseDateSortUsesEffectiveThailandDate(t *testing.T) {
	pipeline := buildDiscoverPipeline("", DiscoverQuery{SortBy: "release_date.asc"})

	foundEffectiveDate := false
	for _, rawStage := range pipeline {
		stage := rawStage.(bson.D)
		if stage[0].Key == "$addFields" {
			foundEffectiveDate = true
			break
		}
	}
	if !foundEffectiveDate {
		t.Fatal("release-date sorting should add the effective Thailand release date")
	}

	sort := sortStage("release_date.asc")
	if sort[0].Key != "_effective_release_date" || sort[0].Value != 1 {
		t.Fatalf("unexpected release-date sort: %#v", sort)
	}
}
