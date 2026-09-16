package board

import (
	"errors"
	"testing"
	"time"

	"github.com/barspielberg/prs-mng/internal/config"
	"github.com/barspielberg/prs-mng/internal/github"
)

func cfg(names ...string) config.Config {
	c := config.Config{Repo: "o/r"}
	for _, n := range names {
		c.Rules = append(c.Rules, config.Rule{Name: n, Query: "x"})
	}
	return c
}

func pr(n int) github.PR {
	return github.PR{Number: n, UpdatedAt: time.Unix(int64(n), 0)}
}

func nums(s Section) []int {
	out := []int{}
	for _, r := range s.Rows {
		out = append(out, r.PR.Number)
	}
	return out
}

// A later rule must not render before an earlier one, even when its own
// request finished first: the earlier rule may still claim its PRs.
func TestLaterSectionWaitsForEarlier(t *testing.T) {
	b := New(cfg("mine", "review"))
	b.Apply(Result{Index: 1, PRs: []github.PR{pr(2)}})

	if got := b.Sections()[1].State; got != Pending {
		t.Errorf("section 2 should wait for section 1, got state %v", got)
	}
	if !b.Loading() {
		t.Error("board should still be loading")
	}

	b.Apply(Result{Index: 0, PRs: []github.PR{pr(1)}})
	for i, s := range b.Sections() {
		if s.State != Ready {
			t.Errorf("section %d: want Ready, got %v", i, s.State)
		}
	}
	if b.Loading() {
		t.Error("board should be done")
	}
}

func TestFirstMatchWins(t *testing.T) {
	b := New(cfg("mine", "involved"))
	// #1 matches both rules; the first one must keep it.
	b.Apply(Result{Index: 1, PRs: []github.PR{pr(1), pr(2)}})
	b.Apply(Result{Index: 0, PRs: []github.PR{pr(1)}})

	if got := nums(b.Sections()[0]); len(got) != 1 || got[0] != 1 {
		t.Errorf("section 1: want [1], got %v", got)
	}
	if got := nums(b.Sections()[1]); len(got) != 1 || got[0] != 2 {
		t.Errorf("section 2: want [2] (1 claimed above), got %v", got)
	}
}

// Order of arrival must not change the outcome, only when it becomes visible.
func TestClaimFollowsRuleOrderNotArrivalOrder(t *testing.T) {
	forward := New(cfg("a", "b"))
	forward.Apply(Result{Index: 0, PRs: []github.PR{pr(1)}})
	forward.Apply(Result{Index: 1, PRs: []github.PR{pr(1)}})

	reverse := New(cfg("a", "b"))
	reverse.Apply(Result{Index: 1, PRs: []github.PR{pr(1)}})
	reverse.Apply(Result{Index: 0, PRs: []github.PR{pr(1)}})

	for i := range forward.Sections() {
		f, r := nums(forward.Sections()[i]), nums(reverse.Sections()[i])
		if len(f) != len(r) {
			t.Fatalf("section %d differs by arrival order: %v vs %v", i, f, r)
		}
	}
	if got := nums(reverse.Sections()[1]); len(got) != 0 {
		t.Errorf("section 2 should be empty, got %v", got)
	}
}

// A failed rule claims nothing, so the board keeps going instead of stalling.
func TestFailedRuleDoesNotBlockBoard(t *testing.T) {
	b := New(cfg("mine", "review"))
	b.Apply(Result{Index: 0, Err: errors.New("boom")})
	b.Apply(Result{Index: 1, PRs: []github.PR{pr(1)}})

	if s := b.Sections()[0]; s.State != Failed {
		t.Errorf("section 1: want Failed, got %v", s.State)
	}
	if got := nums(b.Sections()[1]); len(got) != 1 {
		t.Errorf("section 2 should still render, got %v", got)
	}
}

func TestStackedPRsGroupIntoChain(t *testing.T) {
	base := github.PR{Number: 1, HeadRefName: "a", BaseRefName: "main", UpdatedAt: time.Unix(3, 0)}
	mid := github.PR{Number: 2, HeadRefName: "b", BaseRefName: "a", UpdatedAt: time.Unix(2, 0)}
	top := github.PR{Number: 3, HeadRefName: "c", BaseRefName: "b", UpdatedAt: time.Unix(1, 0)}

	b := New(config.Config{Repo: "o/r", Rules: []config.Rule{{Name: "mine", Query: "x", Tree: true}}})
	b.Apply(Result{Index: 0, PRs: []github.PR{top, base, mid}})

	rows := b.Sections()[0].Rows
	if got := nums(b.Sections()[0]); len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Fatalf("chain should read bottom-up as 1,2,3; got %v", got)
	}
	if rows[0].Prefix != "╭╴" || rows[1].Prefix != "│ " || rows[2].Prefix != "╰╴" {
		t.Errorf("unexpected tree glyphs: %q %q %q", rows[0].Prefix, rows[1].Prefix, rows[2].Prefix)
	}
}

// A base/head cycle is malformed but must not hang the UI.
func TestCyclicStackTerminates(t *testing.T) {
	x := github.PR{Number: 1, HeadRefName: "a", BaseRefName: "b"}
	y := github.PR{Number: 2, HeadRefName: "b", BaseRefName: "a"}

	done := make(chan []Row, 1)
	go func() {
		b := New(config.Config{Repo: "o/r", Rules: []config.Rule{{Name: "m", Query: "x", Tree: true}}})
		b.Apply(Result{Index: 0, PRs: []github.PR{x, y}})
		done <- b.Sections()[0].Rows
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cyclic stack did not terminate")
	}
}

// An unset base must not read as "stacked on" an unset head, which would drop
// every PR from a tree section.
func TestUnstackedPRsSurviveTreeLayout(t *testing.T) {
	b := New(config.Config{Repo: "o/r", Rules: []config.Rule{{Name: "mine", Query: "x", Tree: true}}})
	b.Apply(Result{Index: 0, PRs: []github.PR{pr(1), pr(2)}})

	if got := nums(b.Sections()[0]); len(got) != 2 {
		t.Errorf("want both PRs, got %v", got)
	}
	for _, r := range b.Sections()[0].Rows {
		if r.Prefix != "" {
			t.Errorf("#%d should have no tree glyph, got %q", r.PR.Number, r.Prefix)
		}
	}
}
