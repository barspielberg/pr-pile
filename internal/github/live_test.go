package github_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/barspielberg/pr-pile/internal/config"
	"github.com/barspielberg/pr-pile/internal/github"
)

// Hits the real API; run with -run TestLive when you want it.
func TestLive(t *testing.T) {
	if testing.Short() {
		t.Skip("live API test")
	}
	cfg := config.Default()
	cfg.Repo = os.Getenv("PILE_REPO")
	if cfg.Repo == "" {
		t.Skip("set PILE_REPO to the repo to test against")
	}
	c, err := github.New()
	if err != nil {
		t.Skip("no gh token:", err)
	}
	// Some orgs make every search return zero rather than failing (an IP allow
	// list will do it), which would look like a passing test against an empty
	// board.
	if err := c.CheckRepo(context.Background(), cfg.Repo); err != nil {
		t.Skip("repo unreachable:", err)
	}
	for _, r := range cfg.Rules {
		start := time.Now()
		prs, err := c.Search(context.Background(), cfg.SearchQuery(r), r.PageSize())
		if err != nil {
			t.Errorf("%s: %v", r.Name, err)
			continue
		}
		t.Logf("%-18s %5.2fs  %d rows", r.Name, time.Since(start).Seconds(), len(prs))
		for i, p := range prs {
			if i >= 2 {
				break
			}
			t.Logf("    #%d ci=%-8s rev=%-16s mrg=%-11s gates=%v  %.50s",
				p.Number, p.CIState, p.Review, p.Mergeable, p.FailedGates, p.Title)
		}
	}
}

// Polls a handful of real PRs the way `m` does. Run with -run TestLiveWatch.
func TestLiveWatch(t *testing.T) {
	if testing.Short() {
		t.Skip("live API test")
	}
	repo := os.Getenv("PILE_REPO")
	if repo == "" {
		t.Skip("set PILE_REPO to the repo to test against")
	}
	c, err := github.New()
	if err != nil {
		t.Skip("no gh token:", err)
	}
	prs, err := c.Search(context.Background(), "repo:"+repo+" is:pr sort:updated-desc", 10)
	if err != nil {
		t.Fatal(err)
	}
	numbers := make([]int, 0, len(prs))
	for _, pr := range prs {
		numbers = append(numbers, pr.Number)
	}
	start := time.Now()
	got, err := c.Watch(context.Background(), repo, numbers)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d PRs in %.2fs", len(numbers), time.Since(start).Seconds())
	for _, n := range numbers {
		w, ok := got[n]
		if !ok {
			t.Errorf("#%d missing from the poll", n)
			continue
		}
		t.Logf("#%d %-6s ci=%-7s review=%-17s mergeable=%s", n, w.State, w.CIState, w.Review, w.Mergeable)
	}
}
