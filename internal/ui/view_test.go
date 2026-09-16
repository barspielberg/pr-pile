package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/barspielberg/prs-mng/internal/board"
	"github.com/barspielberg/prs-mng/internal/config"
	"github.com/barspielberg/prs-mng/internal/github"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
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
	if !strings.Contains(m.View(), "REVIEW REQUESTED") {
		t.Error("expected the pending section header")
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
	// Glyphs, per the design spec: ✗1 = one failing check, ✗ = changes
	// requested, ! = conflicts.
	for _, want := range []string{"#7", "✗1", "!", "webapp_e2e"} {
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
	if !strings.Contains(out, "~") {
		t.Error("expected the draft blocker glyph:\n" + out)
	}
	if !strings.Contains(out, "✓") {
		t.Error("draft hid the approved review glyph:\n" + out)
	}
}

// The board is taller than the terminal in the normal case, so the selected
// row must stay on screen as the cursor moves.
func TestCursorStaysVisibleInShortTerminal(t *testing.T) {
	m := New(testCfg(), nil)
	m.width, m.height = 120, 20

	var prs []github.PR
	for i := 1; i <= 40; i++ {
		prs = append(prs, github.PR{
			Number: i, Title: fmt.Sprintf("pr number %d", i),
			CIState: "SUCCESS", UpdatedAt: time.Unix(int64(100-i), 0),
		})
	}
	m.board.Apply(board.Result{Index: 0, PRs: prs})
	m.board.Apply(board.Result{Index: 1})

	for _, cursor := range []int{0, 5, 20, 39} {
		m.cursor = cursor
		out := m.View()
		if got := strings.Count(out, "\n") + 1; got > m.height {
			t.Errorf("cursor %d: view is %d lines, terminal is %d", cursor, got, m.height)
		}
		if !strings.Contains(out, "▌") {
			t.Errorf("cursor %d: selected row is off screen:\n%s", cursor, out)
		}
		want := fmt.Sprintf("pr number %d", cursor+1)
		if !strings.Contains(out, want) {
			t.Errorf("cursor %d: expected %q on screen:\n%s", cursor, want, out)
		}
	}
}

// Status must sit at the same screen offset at every width: exactly one column
// (title) flexes, so the eye learns one position.
func TestStatusClusterIsPinnedAcrossWidths(t *testing.T) {
	for _, w := range []int{80, 100, 120, 200} {
		m := New(testCfg(), nil)
		m.width = w
		m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{
			Number: 3248, Title: "feat(api-service): PROJ-2037 refuse order plan writes",
			CIState: "SUCCESS", Review: "REVIEW_REQUIRED", UpdatedAt: time.Now(),
		}}})
		m.board.Apply(board.Result{Index: 1})

		row := ""
		for _, l := range strings.Split(m.View(), "\n") {
			if strings.Contains(l, "#3248") {
				row = stripANSI(l)
			}
		}
		// Display cells, not bytes: the ▌ mark is multi-byte.
		if got := lipgloss.Width(row[:strings.Index(row, "✓")]); got != 11 {
			t.Errorf("width %d: CI glyph at column %d, want 11\n%q", w, got, row)
		}
	}
}

func TestNarrowWidthsDropFieldsInOrder(t *testing.T) {
	pr := github.PR{
		Number: 3248, Title: "feat(api-service): refuse order plan writes",
		CIState: "SUCCESS", Review: "REVIEW_REQUIRED", UpdatedAt: time.Now().Add(-2 * time.Hour),
	}
	rowAt := func(w int) string {
		m := New(testCfg(), nil)
		m.width = w
		m.board.Apply(board.Result{Index: 0, PRs: []github.PR{pr}})
		m.board.Apply(board.Result{Index: 1})
		for _, l := range strings.Split(m.View(), "\n") {
			if strings.Contains(l, "#3248") {
				return stripANSI(l)
			}
		}
		return ""
	}

	if got := rowAt(100); !strings.Contains(got, "2h") || !strings.Contains(got, "○") {
		t.Errorf("FULL tier should keep age and review glyph: %q", got)
	}
	if got := rowAt(70); strings.Contains(got, "2h") {
		t.Errorf("MID tier should drop age: %q", got)
	} else if !strings.Contains(got, "○") {
		t.Errorf("MID tier should keep the review glyph: %q", got)
	}
	if got := rowAt(50); strings.Contains(got, "○") {
		t.Errorf("NARROW tier should drop the review glyph: %q", got)
	} else if !strings.Contains(got, "✓") {
		t.Errorf("NARROW tier must keep CI: %q", got)
	}
}

// Below the minimum there is no honest layout, so say so rather than misalign.
func TestBelowMinimumWidthSaysSo(t *testing.T) {
	m := New(testCfg(), nil)
	m.width = 30
	if out := m.View(); !strings.Contains(out, "too narrow") {
		t.Errorf("want a too-narrow message, got:\n%s", out)
	}
}

// No row may overflow the terminal at any supported width. Stepped by 1, and
// with a cursor that is not on the row: a selected row is filled to the right
// edge, which hides an over-wide row behind the fill.
func TestNoRowOverflowsAtAnyWidth(t *testing.T) {
	for w := minWidth; w <= 200; w++ {
		m := New(testCfg(), nil)
		m.width = w
		m.cursor = -1
		m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{
			Number: 3248, Title: strings.Repeat("long title ", 30),
			CIState: "FAILURE", FailedGates: []string{"a", "b"},
			Mergeable: "CONFLICTING", IsDraft: true, UpdatedAt: time.Now(),
		}}})
		m.board.Apply(board.Result{Index: 1})
		for _, l := range strings.Split(m.View(), "\n") {
			if got := lipgloss.Width(stripANSI(l)); got > w {
				t.Errorf("width %d: line is %d cells: %q", w, got, stripANSI(l))
			}
		}
	}
}

// The age column is budgeted at 3 cells. A very old PR (or a skewed clock) must
// saturate rather than widen the row and push it past the frame.
func TestAgeColumnNeverExceedsItsBudget(t *testing.T) {
	for _, ts := range []time.Time{
		time.Now().Add(-30 * time.Minute),
		time.Now().Add(-5 * time.Hour),
		time.Now().Add(-3 * 24 * time.Hour),
		time.Now().Add(-40 * 7 * 24 * time.Hour),
		time.Unix(1, 0), // epoch: ~2900 weeks
	} {
		if got := lipgloss.Width(age(ts)); got > 3 {
			t.Errorf("age(%v) is %d cells, want <= 3: %q", ts, got, age(ts))
		}
	}
}

// The selection background is the primary selection signal; the ▌ bar is the
// second channel. A row built from per-cell styles loses an outer background
// to the cells' own resets, so this asserts the fill actually survives.
func TestSelectedRowIsFilledEdgeToEdge(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	m := New(testCfg(), nil)
	m.width = 60
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{
		{Number: 1, Title: "selected", CIState: "FAILURE", FailedGates: []string{"g"}, UpdatedAt: time.Now()},
		{Number: 2, Title: "not selected", UpdatedAt: time.Now().Add(-time.Hour)},
	}})
	m.board.Apply(board.Result{Index: 1})
	m.cursor = 0

	var sel, unsel string
	for _, l := range strings.Split(m.View(), "\n") {
		if strings.Contains(l, "#1") {
			sel = l
		}
		if strings.Contains(l, "#2") {
			unsel = l
		}
	}
	// 100 is "bright black background" (ANSI 8 as bg).
	if !strings.Contains(sel, "\x1b[100m") && !strings.Contains(sel, "48;5;8") {
		t.Errorf("selected row has no background fill:\n%q", sel)
	}
	if strings.Contains(unsel, "\x1b[100m") {
		t.Errorf("unselected row should not be filled:\n%q", unsel)
	}
	if !strings.Contains(sel, "▌") {
		t.Error("selected row is missing the mark bar")
	}
	// The fill must reach the right edge, or the band looks ragged.
	if got := lipgloss.Width(stripANSI(sel)); got != m.width {
		t.Errorf("selected row is %d cells, want %d", got, m.width)
	}
	// Failure colour must survive on top of the selection fill.
	if !strings.Contains(sel, "✗") {
		t.Error("selected+failing lost its CI glyph")
	}
}

// A refresh must not collapse the board and re-expand it: that shoves every
// row below each resolving section down the screen.
func TestRefreshDoesNotChangeLayout(t *testing.T) {
	m := New(testCfg(), nil)
	m.width, m.height = 120, 60

	mk := func(n, base int) []github.PR {
		var out []github.PR
		for i := 0; i < n; i++ {
			out = append(out, github.PR{
				Number: base + i, Title: fmt.Sprintf("pr %d", base+i),
				CIState: "SUCCESS", UpdatedAt: time.Unix(int64(1000-i), 0),
			})
		}
		return out
	}
	m.board.Apply(board.Result{Index: 0, PRs: mk(5, 100)})
	m.board.Apply(board.Result{Index: 1, PRs: mk(12, 200)})

	settled := m.View()
	height := strings.Count(settled, "\n")

	m.board.Refetch()
	if got := strings.Count(m.View(), "\n"); got != height {
		t.Errorf("board changed height on refetch: %d -> %d", height, got)
	}
	if !strings.Contains(m.View(), "#100") {
		t.Error("rows vanished during refetch")
	}

	// Sections landing one at a time must not move anything either.
	m.board.Apply(board.Result{Index: 0, PRs: mk(5, 100)})
	if got := strings.Count(m.View(), "\n"); got != height {
		t.Errorf("height changed when section 1 landed: %d -> %d", height, got)
	}
	m.board.Apply(board.Result{Index: 1, PRs: mk(12, 200)})
	if got := strings.Count(m.View(), "\n"); got != height {
		t.Errorf("height changed when section 2 landed: %d -> %d", height, got)
	}
	if m.View() != settled {
		t.Error("board did not return to the same frame after an identical refetch")
	}
}
