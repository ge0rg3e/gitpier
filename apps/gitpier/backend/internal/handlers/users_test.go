package handlers

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"gitpier/internal/models"
)

func TestContributionAggregateJSONRoundTrip(t *testing.T) {
	src := contributionAggregate{
		Counts: map[string]int{
			"2026-06-01": 4,
			"2026-06-02": 2,
		},
		Total:         6,
		CurrentStreak: 2,
		LongestStreak: 5,
	}

	b, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshal contribution aggregate: %v", err)
	}

	var dst contributionAggregate
	if err := json.Unmarshal(b, &dst); err != nil {
		t.Fatalf("unmarshal contribution aggregate: %v", err)
	}

	if !reflect.DeepEqual(dst.Counts, src.Counts) {
		t.Fatalf("counts mismatch after round trip: got %#v want %#v", dst.Counts, src.Counts)
	}
	if dst.Total != src.Total {
		t.Fatalf("total mismatch after round trip: got %d want %d", dst.Total, src.Total)
	}
	if dst.CurrentStreak != src.CurrentStreak {
		t.Fatalf("current streak mismatch after round trip: got %d want %d", dst.CurrentStreak, src.CurrentStreak)
	}
	if dst.LongestStreak != src.LongestStreak {
		t.Fatalf("longest streak mismatch after round trip: got %d want %d", dst.LongestStreak, src.LongestStreak)
	}
}

func TestBuildDashboardRecentActivityRepos(t *testing.T) {
	repos := []models.Repository{
		{
			Name:      "personal",
			UpdatedAt: time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC),
			Owner:     models.User{Username: "alice"},
		},
		{
			Name:      "org-repo",
			UpdatedAt: time.Date(2026, 6, 3, 8, 0, 0, 0, time.UTC),
			Owner:     models.User{Username: "alice"},
			Org:       &models.Organization{Login: "acme"},
		},
		{
			Name:      "older",
			UpdatedAt: time.Date(2026, 5, 28, 8, 0, 0, 0, time.UTC),
			Owner:     models.User{Username: "alice"},
		},
	}

	got := buildDashboardRecentActivityRepos(repos, 2)
	if len(got) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(got))
	}
	if got[0].Owner != "acme" || got[0].Name != "org-repo" {
		t.Fatalf("expected newest org repo first, got %#v", got[0])
	}
	if got[1].Owner != "alice" || got[1].Name != "personal" {
		t.Fatalf("expected personal repo second, got %#v", got[1])
	}
}

func TestFilterGitHubCountedReposSkipsForks(t *testing.T) {
	forkedFrom := "upstream"
	repos := []models.Repository{
		{Name: "kept"},
		{Name: "fork", ForkedFromRepoID: &forkedFrom},
	}

	got := filterGitHubCountedRepos(repos)
	if len(got) != 1 {
		t.Fatalf("expected 1 repo after filtering forks, got %d", len(got))
	}
	if got[0].Name != "kept" {
		t.Fatalf("expected non-fork repo to remain, got %#v", got[0])
	}
}

func TestContributionRefNames(t *testing.T) {
	got := contributionRefNames(models.Repository{DefaultBranch: "main"})
	want := []string{"main", "gh-pages"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected refs: got %#v want %#v", got, want)
	}

	ghPagesOnly := contributionRefNames(models.Repository{DefaultBranch: "gh-pages"})
	if !reflect.DeepEqual(ghPagesOnly, []string{"gh-pages"}) {
		t.Fatalf("unexpected gh-pages refs: got %#v", ghPagesOnly)
	}
}
