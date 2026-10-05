package board

import (
	"errors"
	"testing"
	"time"

	"github.com/barspielberg/pr-pile/internal/config"
	"github.com/barspielberg/pr-pile/internal/github"
)

func cfg(names ...string) config.Config {
	c := config.Config{Repos: []config.Repo{{Name: "o/r"}}}
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

	b := New(config.Config{Repos: []config.Repo{{Name: "o/r"}}, Rules: []config.Rule{{Name: "mine", Query: "x", Tree: true}}})
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
		b := New(config.Config{Repos: []config.Repo{{Name: "o/r"}}, Rules: []config.Rule{{Name: "m", Query: "x", Tree: true}}})
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
	b := New(config.Config{Repos: []config.Repo{{Name: "o/r"}}, Rules: []config.Rule{{Name: "mine", Query: "x", Tree: true}}})
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

// A stack split across two sections is the normal case, not an edge case:
// first-match-wins claims part of a chain for an earlier rule. Each section
// must close the sub-chain it actually holds rather than emitting a dangling
// glyph or dropping the orphaned children.
func TestSplitStackClosesEachSubChain(t *testing.T) {
	c := []github.PR{
		{Number: 1, HeadRefName: "a", BaseRefName: "main", UpdatedAt: time.Unix(1, 0)},
		{Number: 2, HeadRefName: "b", BaseRefName: "a", UpdatedAt: time.Unix(2, 0)},
		{Number: 3, HeadRefName: "c", BaseRefName: "b", UpdatedAt: time.Unix(3, 0)},
		{Number: 4, HeadRefName: "d", BaseRefName: "c", UpdatedAt: time.Unix(4, 0)},
	}
	b := New(config.Config{Repos: []config.Repo{{Name: "o/r"}}, Rules: []config.Rule{
		{Name: "first", Query: "x", Tree: true},
		{Name: "rest", Query: "x", Tree: true},
	}})
	b.Apply(Result{Index: 0, PRs: []github.PR{c[0], c[1]}})
	b.Apply(Result{Index: 1, PRs: c})

	for i, want := range [][]string{{"╭╴", "╰╴"}, {"╭╴", "╰╴"}} {
		rows := b.Sections()[i].Rows
		if len(rows) != len(want) {
			t.Fatalf("section %d: want %d rows, got %d", i, len(want), len(rows))
		}
		for j, g := range want {
			if rows[j].Prefix != g {
				t.Errorf("section %d row %d: want %q, got %q", i, j, g, rows[j].Prefix)
			}
		}
	}
	if got := nums(b.Sections()[1]); got[0] != 3 || got[1] != 4 {
		t.Errorf("leftover chain should be 3,4; got %v", got)
	}
}

// A PR whose base is claimed by another section has no parent here, so it is a
// root of its own chain -- and a chain of one draws no glyph at all. The glyph
// means "stacked on something in this section", and must not claim otherwise.
func TestLoneStackTopDrawsNoGlyph(t *testing.T) {
	top := github.PR{Number: 2, HeadRefName: "b", BaseRefName: "a", UpdatedAt: time.Unix(2, 0)}
	b := New(config.Config{Repos: []config.Repo{{Name: "o/r"}}, Rules: []config.Rule{{Name: "team", Query: "x", Tree: true}}})
	b.Apply(Result{Index: 0, PRs: []github.PR{top}})

	rows := b.Sections()[0].Rows
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	if rows[0].Prefix != "" {
		t.Errorf("a stack top with no parent here should draw no glyph, got %q", rows[0].Prefix)
	}
}

// A hidden PR stays off while a lagging search still returns it, and comes
// back once the hold expires, so one closed by mistake and reopened returns.
func TestHideExpires(t *testing.T) {
	clock := time.Now()
	orig := now
	now = func() time.Time { return clock }
	t.Cleanup(func() { now = orig })

	b := New(cfg("a"))
	b.Apply(Result{Index: 0, PRs: []github.PR{pr(1), pr(2)}})
	b.Hide(pr(1).Key())
	if got := nums(b.Sections()[0]); len(got) != 1 || got[0] != 2 {
		t.Fatalf("after Hide: %v", got)
	}
	b.Apply(Result{Index: 0, PRs: []github.PR{pr(1), pr(2)}})
	if got := nums(b.Sections()[0]); len(got) != 1 {
		t.Errorf("a fetch inside the hold brought it back: %v", got)
	}
	clock = clock.Add(hideFor)
	b.Apply(Result{Index: 0, PRs: []github.PR{pr(1), pr(2)}})
	if got := nums(b.Sections()[0]); len(got) != 2 {
		t.Errorf("still hidden after the hold: %v", got)
	}
}

// A rule that searches two repos can return the same number from each, and
// they are two PRs: neither claims the other, and hiding one leaves the other.
func TestSameNumberInTwoReposIsTwoPRs(t *testing.T) {
	a := github.PR{Repo: "o/a", Number: 7, UpdatedAt: time.Unix(2, 0)}
	other := github.PR{Repo: "o/b", Number: 7, UpdatedAt: time.Unix(1, 0)}

	b := New(cfg("mine", "review"))
	b.Apply(Result{Index: 0, PRs: []github.PR{a}})
	b.Apply(Result{Index: 1, PRs: []github.PR{a, other}})
	if got := b.Sections()[1].Rows; len(got) != 1 || got[0].PR.Repo != "o/b" {
		t.Fatalf("section 2: want only o/b#7, got %v", got)
	}

	b.Hide(a.Key())
	if got := b.Sections()[0].Rows; len(got) != 0 {
		t.Errorf("section 1 after hiding o/a#7: want empty, got %v", got)
	}
	if got := b.Sections()[1].Rows; len(got) != 1 || got[0].PR.Repo != "o/b" {
		t.Errorf("section 2 after hiding o/a#7: want only o/b#7, got %v", got)
	}
}

// Branch names repeat across repos, so a PR stacks only on one in its own repo.
func TestStacksStayInsideOneRepo(t *testing.T) {
	base := github.PR{Repo: "o/a", Number: 1, HeadRefName: "fix", BaseRefName: "main", UpdatedAt: time.Unix(2, 0)}
	elsewhere := github.PR{Repo: "o/b", Number: 2, HeadRefName: "next", BaseRefName: "fix", UpdatedAt: time.Unix(1, 0)}

	b := New(config.Config{Repos: []config.Repo{{Name: "o/a"}}, Rules: []config.Rule{{Name: "mine", Query: "x", Tree: true}}})
	b.Apply(Result{Index: 0, PRs: []github.PR{base, elsewhere}})

	for _, r := range b.Sections()[0].Rows {
		if r.Prefix != "" {
			t.Errorf("%s#%d drew stack glyph %q across repos", r.PR.Repo, r.PR.Number, r.Prefix)
		}
	}
}
