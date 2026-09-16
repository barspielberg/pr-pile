package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/barspielberg/prs-mng/internal/board"
	"github.com/barspielberg/prs-mng/internal/config"
	"github.com/barspielberg/prs-mng/internal/github"
)

func testCfg() config.Config {
	return config.Config{
		Repo: "o/r",
		Rules: []config.Rule{
			{Name: "Mine", Query: "author:@me", Tree: true},
			{Name: "Review requested", Query: "review-requested:@me"},
		},
	}
}

func TestViewShowsLoadingUntilFirstSectionResolves(t *testing.T) {
	m := New(testCfg(), nil)
	m.width = 120

	// Second rule arrives first; it must not render yet.
	m.board.Apply(board.Result{Index: 1, PRs: []github.PR{
		{Number: 99, Title: "should not be visible yet", UpdatedAt: time.Now()},
	}})
	if out := m.View(); strings.Contains(out, "should not be visible yet") {
		t.Error("section 2 rendered before section 1 resolved:\n" + out)
	}
	if !strings.Contains(m.View(), "loading") {
		t.Error("expected a loading indicator")
	}

	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{
		{Number: 1, Title: "my pr", CIState: "SUCCESS", UpdatedAt: time.Now()},
	}})
	out := m.View()
	for _, want := range []string{"my pr", "should not be visible yet", "#1", "#99"} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing %q:\n%s", want, out)
		}
	}
}

func TestViewRendersStatesAndGates(t *testing.T) {
	m := New(testCfg(), nil)
	m.width = 140
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{
		Number: 7, Title: "red pr", CIState: "FAILURE", Mergeable: "CONFLICTING",
		Review: "CHANGES_REQUESTED", FailedGates: []string{"webapp_e2e"}, UpdatedAt: time.Now(),
	}}})
	m.board.Apply(board.Result{Index: 1})

	out := m.View()
	for _, want := range []string{"#7", "failing", "conflicts", "changes req", "webapp_e2e"} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing %q:\n%s", want, out)
		}
	}
}

// A long title must not wrap the row; it gets clipped to the frame instead.
func TestLongTitleDoesNotOverflowWidth(t *testing.T) {
	m := New(testCfg(), nil)
	m.width = 100
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{
		Number: 1, Title: strings.Repeat("very long title ", 40), CIState: "SUCCESS", UpdatedAt: time.Now(),
	}}})
	m.board.Apply(board.Result{Index: 1})

	for _, line := range strings.Split(m.View(), "\n") {
		if w := len([]rune(stripANSI(line))); w > m.width {
			t.Errorf("line exceeds width %d: %d cells\n%q", m.width, w, line)
		}
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		switch {
		case r == 0x1b:
			inEsc = true
		case inEsc && r == 'm':
			inEsc = false
		case !inEsc:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func TestCursorSurvivesSectionsResolving(t *testing.T) {
	m := New(testCfg(), nil)
	m.width = 120
	m.cursor = 5 // stale cursor from a previous, longer board
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{Number: 1, UpdatedAt: time.Now()}}})
	m.board.Apply(board.Result{Index: 1})
	m.clampCursor()

	if m.cursor != 0 {
		t.Errorf("cursor should clamp to 0, got %d", m.cursor)
	}
	if _, ok := m.selected(); !ok {
		t.Error("expected a selected PR after clamping")
	}
}

func TestActionTemplateRenders(t *testing.T) {
	cfg := testCfg()
	cfg.RepoPath = "~/Repos/acme/monorepo"
	m := New(cfg, nil)
	got, err := m.renderAction("wt switch -x nvim pr:{{.Number}} # {{.Repo}} {{.Branch}}",
		github.PR{Number: 42, HeadRefName: "feat/x"})
	if err != nil {
		t.Fatal(err)
	}
	if want := "wt switch -x nvim pr:42 # o/r feat/x"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A draft PR still has a review decision; the draft marker must not hide it.
func TestDraftAndReviewAreSeparateColumns(t *testing.T) {
	m := New(testCfg(), nil)
	m.width = 150
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{
		Number: 1, Title: "draft but approved", IsDraft: true,
		Review: "APPROVED", CIState: "SUCCESS", UpdatedAt: time.Now(),
	}}})
	m.board.Apply(board.Result{Index: 1})

	out := m.View()
	if !strings.Contains(out, "draft") {
		t.Error("expected draft marker:\n" + out)
	}
	if !strings.Contains(out, "approved") {
		t.Error("draft hid the review state:\n" + out)
	}
}
