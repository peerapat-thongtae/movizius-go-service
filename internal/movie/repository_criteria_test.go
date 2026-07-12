package movie

import "testing"

func TestRecommendationCriteriaEmpty(t *testing.T) {
	if !(RecommendationCriteria{}).empty() {
		t.Errorf("empty criteria should report empty() == true")
	}
	nonEmpty := RecommendationCriteria{GenreIDs: []int64{1}}
	if nonEmpty.empty() {
		t.Errorf("criteria with GenreIDs set should report empty() == false")
	}
}
