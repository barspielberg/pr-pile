package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/barspielberg/pr-pile/internal/board"
	"github.com/barspielberg/pr-pile/internal/config"
	"github.com/barspielberg/pr-pile/internal/github"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// testRepo is the configured repo of every test board, and the repo of every
// test PR unless a test is about another one.
const testRepo = "o/r"

func testCfg() config.Config {
	return config.Config{
		Repos: []config.Repo{{Name: testRepo}},
		Rules: []config.Rule{
			{Name: "Mine", Query: "author:@me", Tree: true},
			{Name: "Review requested", Query: "review-requested:@me", Author: true},
		},
	}
}

func TestViewShowsLoadingUntilFirstSectionResolves(t *testing.T) {
	m := New(testCfg(), nil)
	m.width = 120

	// Second rule arrives first; it must not render yet.
	m.board.Apply(board.Result{Index: 1, PRs: []github.PR{
		{Repo: testRepo, Number: 99, Title: "should not be visible yet", UpdatedAt: time.Now()},
	}})
	if out := m.View(); strings.Contains(out, "should not be visible yet") {
		t.Error("section 2 rendered before section 1 resolved:\n" + out)
	}
	// The section still announces itself while pending: the header is always
	// drawn, so a section that has not resolved still has a name on screen.
	if !strings.Contains(stripANSI(m.View()), "Review requested") {
		t.Error("expected the pending section's header")
	}

	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{
		{Repo: testRepo, Number: 1, Title: "my pr", CIState: "SUCCESS", UpdatedAt: time.Now()},
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
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{Repo: testRepo,
		Number: 7, Title: "red pr", CIState: "FAILURE", Mergeable: "CONFLICTING",
		Review: "CHANGES_REQUESTED", FailedGates: []string{"webapp_e2e"}, UpdatedAt: time.Now(),
	}}})
	m.board.Apply(board.Result{Index: 1})

	out := m.View()
	// Glyphs, per the design spec: ✗1 = one failing check, ✗ = changes
	// requested, ! = conflicts.
	for _, want := range []string{"#7", "✗1", "!"} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing %q:\n%s", want, out)
		}
	}
	// The gate NAME is not in the list: it would cost the row a second line,
	// which is what made the board scroll unevenly. It lives behind `d`.
	if strings.Contains(out, "webapp_e2e") {
		t.Errorf("gate name should not be in the row:\n%s", out)
	}
	m = onRow(t, m, 0)
	m.showChecks = true
	if !strings.Contains(m.View(), "webapp_e2e") {
		t.Errorf("gate name missing from the checks overlay:\n%s", m.View())
	}
}

// A long title must not wrap the row; it gets clipped to the frame instead.
func TestLongTitleDoesNotOverflowWidth(t *testing.T) {
	m := New(testCfg(), nil)
	m.width = 100
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{Repo: testRepo,
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
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{Repo: testRepo, Number: 1, UpdatedAt: time.Now()}}})
	m.board.Apply(board.Result{Index: 1})
	m.clampCursor()

	// The cursor must land inside the address space. It may land on a header:
	// the last slot of a board whose final section resolved empty is that
	// section's header, and that is a legitimate place to be.
	sl := m.slots()
	if m.cursor < 0 || m.cursor >= len(sl) {
		t.Fatalf("cursor %d is outside the %d slots", m.cursor, len(sl))
	}
	// A row cursor must have a PR behind it; a header cursor must not.
	_, ok := m.selected()
	if got := sl[m.cursor]; got.isRow() != ok {
		t.Errorf("slot kind and selected() disagree: isRow=%v selected=%v", got.isRow(), ok)
	}
}

func TestActionTemplateRenders(t *testing.T) {
	cfg := testCfg()
	cfg.Repos[0].Path = "~/Repos/acme/monorepo"
	m := New(cfg, nil)
	got, err := m.renderAction("wt switch -x nvim pr:{{.Number}} # {{.Repo}} {{.RepoPath}} {{.Branch}}",
		github.PR{Repo: testRepo, Number: 42, HeadRefName: "feat/x"})
	if err != nil {
		t.Fatal(err)
	}
	if want := "wt switch -x nvim pr:42 # o/r ~/Repos/acme/monorepo 'feat/x'"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A draft PR still has a review decision; the draft marker must not hide it.
func TestDraftAndReviewAreSeparateColumns(t *testing.T) {
	m := New(testCfg(), nil)
	m.width = 150
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{Repo: testRepo,
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
		prs = append(prs, github.PR{Repo: testRepo,
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
		m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{Repo: testRepo,
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
		// Display cells, not bytes: the ▌ mark is multi-byte. The cluster sits
		// 11 cells into the row, which now starts at column 0.
		want := 11
		if got := lipgloss.Width(row[:strings.Index(row, "✓")]); got != want {
			t.Errorf("width %d: CI glyph at column %d, want %d\n%q", w, got, want, row)
		}
	}
}

func TestNarrowWidthsDropFieldsInOrder(t *testing.T) {
	pr := github.PR{Repo: testRepo,
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
		m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{Repo: testRepo,
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
		{Repo: testRepo, Number: 1, Title: "selected", CIState: "FAILURE", FailedGates: []string{"g"}, UpdatedAt: time.Now()},
		{Repo: testRepo, Number: 2, Title: "not selected", UpdatedAt: time.Now().Add(-time.Hour)},
	}})
	m.board.Apply(board.Result{Index: 1})
	m = onRow(t, m, 0)

	var sel, unsel string
	for _, l := range strings.Split(m.View(), "\n") {
		if strings.Contains(l, "#1") {
			sel = l
		}
		if strings.Contains(l, "#2") {
			unsel = l
		}
	}
	// selBg is ANSI 8, which lipgloss emits as the SGR bright-black background
	// 100 rather than a 48;5;N cube index -- that is the point: it resolves
	// through the user's theme.
	if !strings.Contains(sel, "100m") {
		t.Errorf("selected row has no background fill:\n%q", sel)
	}
	if strings.Contains(unsel, "100m") {
		t.Errorf("unselected row should not be filled:\n%q", unsel)
	}
	// And no fixed cube background anywhere on the board.
	if strings.Contains(sel, "48;5;") {
		t.Errorf("selected row uses a fixed cube background:\n%q", sel)
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

// The accent is the SAME on a selected row as on an unselected one, and it is
// ANSI 4 on both. It used to brighten to cube 75, justified by a comment
// claiming ANSI 4 measured 1.21 against selBg -- a figure computed against a
// nominal ANSI 4 rather than any real theme's palette. Against Catppuccin
// Mocha's actual #89b4fa, ANSI 4 beats 75 on every fill (3.17:1 vs 2.88:1 on
// an ANSI 8 selection), so 75 was costing a fixed value and buying nothing.
//
// This guards the regression in both directions: no cube value may reappear,
// and the accent must not diverge between the two row states.
func TestAccentIsThemedAndDoesNotChangeOnSelection(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	m := New(testCfg(), nil)
	m.width = 60
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{
		{Repo: testRepo, Number: 1, Title: "selected", UpdatedAt: time.Now()},
		{Repo: testRepo, Number: 2, Title: "not selected", UpdatedAt: time.Now().Add(-time.Hour)},
	}})
	m.board.Apply(board.Result{Index: 1})
	m = onRow(t, m, 0)

	var sel, unsel string
	for _, l := range strings.Split(m.View(), "\n") {
		if strings.Contains(l, "#1") {
			sel = l
		}
		if strings.Contains(l, "#2") {
			unsel = l
		}
	}

	// ANSI 4 as a foreground is SGR 34, whether or not a background follows.
	if !hasSGRParam(segmentAround(t, sel, "#1"), "34") {
		t.Errorf("selected row's PR number is not ANSI 4:\n%q", sel)
	}
	if !hasSGRParam(segmentAround(t, unsel, "#2"), "34") {
		t.Errorf("unselected row's PR number is not ANSI 4:\n%q", unsel)
	}
	// No fixed cube foreground on either row.
	for _, l := range []string{sel, unsel} {
		if strings.Contains(l, "38;5;") {
			t.Errorf("a row uses a fixed cube foreground:\n%q", l)
		}
	}
}

func TestRefreshDoesNotChangeLayout(t *testing.T) {
	m := New(testCfg(), nil)
	m.width, m.height = 120, 60

	mk := func(n, base int) []github.PR {
		var out []github.PR
		for i := 0; i < n; i++ {
			out = append(out, github.PR{Repo: testRepo,
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

// A short board must not let the footer float up the screen: the prompt and
// footer belong on the bottom edge whether there are two rows or fifty.
func TestFooterStaysPinnedToTheBottom(t *testing.T) {
	for _, n := range []int{1, 3, 40} {
		m := New(testCfg(), nil)
		m.width, m.height = 120, 24

		var prs []github.PR
		for i := 0; i < n; i++ {
			prs = append(prs, github.PR{Repo: testRepo,
				Number: 3000 + i, Title: fmt.Sprintf("pr %d", i),
				CIState: "SUCCESS", UpdatedAt: time.Unix(int64(9000-i), 0),
			})
		}
		m.board.Apply(board.Result{Index: 0, PRs: prs})
		m.board.Apply(board.Result{Index: 1})

		lines := strings.Split(m.View(), "\n")
		if len(lines) != m.height {
			t.Errorf("%d rows: view is %d lines, want exactly %d", n, len(lines), m.height)
		}
		if last := stripANSI(lines[len(lines)-1]); !strings.Contains(last, "quit") {
			t.Errorf("%d rows: last line is not the footer: %q", n, last)
		}
		// The footer is the only chrome, so the list owns every line above it
		// and the first one is the first section's header.
		if first := stripANSI(lines[0]); !strings.Contains(first, "Mine") {
			t.Errorf("%d rows: first line is not the first section's header: %q", n, first)
		}

		// And the same while searching, where the prompt is a second chrome row.
		m.searching = true
		m.query = "pr"
		flines := strings.Split(m.View(), "\n")
		if len(flines) != m.height {
			t.Errorf("%d rows searching: view is %d lines, want %d", n, len(flines), m.height)
		}
		if got := stripANSI(flines[len(flines)-2]); !strings.Contains(got, "/") {
			t.Errorf("%d rows searching: prompt not directly above the footer: %q", n, got)
		}
	}
}

// The author column is per rule: a rule like author:@me is all one person, so
// the column would be dead weight there.
func TestAuthorColumnIsPerRule(t *testing.T) {
	m := New(testCfg(), nil)
	m.width, m.height = 120, 30
	pr := github.PR{Repo: testRepo, Number: 1, Title: "a title", Author: "octocat",
		CIState: "SUCCESS", UpdatedAt: time.Now()}
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{pr}})
	m.board.Apply(board.Result{Index: 1, PRs: []github.PR{{Repo: testRepo,
		Number: 2, Title: "another", Author: "octocat",
		CIState: "SUCCESS", UpdatedAt: time.Now(),
	}}})

	var mine, review string
	for _, l := range strings.Split(m.View(), "\n") {
		if strings.Contains(l, "#1") {
			mine = stripANSI(l)
		}
		if strings.Contains(l, "#2") {
			review = stripANSI(l)
		}
	}
	// Rule 1 (Mine) has author off; rule 2 has it on.
	if strings.Contains(mine, "oct") {
		t.Errorf("Mine should not show an author: %q", mine)
	}
	if !strings.Contains(review, "oct") {
		t.Errorf("Review requested should show initials: %q", review)
	}
}

// The author column is muted, like age: one quiet tier rather than two. It
// used to carry a 15-entry hue palette keyed on the login, which spent 15 fixed
// cube values overriding the user's theme to decorate what the initials already
// said. Two different authors must now render identically apart from their
// letters -- that is the property a palette would break.
func TestAuthorColumnIsMutedLikeAge(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	cfg := config.Config{Repos: []config.Repo{{Name: "o/r"}}, Rules: []config.Rule{
		{Name: "Mine", Query: "a", Author: true},
	}}
	seg := func(login string) string {
		t.Helper()
		m := New(cfg, nil)
		m.width, m.height = 120, 20
		m.cursor = -1
		m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{Repo: testRepo,
			Number: 1, Title: "a title", Author: login,
			CIState: "SUCCESS", UpdatedAt: time.Now(),
		}}})
		for _, l := range strings.Split(m.View(), "\n") {
			if strings.Contains(stripANSI(l), "#1") {
				return segmentAround(t, l, initials(login))
			}
		}
		t.Fatalf("no row for %q", login)
		return ""
	}

	// Faint, and no foreground of its own.
	a := seg("octocat")
	if !hasSGRParam(a, "2") {
		t.Errorf("author is not faint: %q", a)
	}
	if strings.Contains(a, "38;5;") {
		t.Errorf("author still carries a palette colour: %q", a)
	}
	// Two logins, same styling: only the letters differ.
	b := seg("zebra")
	norm := func(seg, login string) string {
		return strings.ReplaceAll(seg, initials(login), "")
	}
	if norm(a, "octocat") != norm(b, "zebra") {
		t.Errorf("authors render differently:\n %q\n %q", a, b)
	}
}

func TestAuthorColumnDoesNotOverflow(t *testing.T) {
	cfg := testCfg()
	for i := range cfg.Rules {
		cfg.Rules[i].Author = true
	}
	for w := minWidth; w <= 200; w++ {
		m := New(cfg, nil)
		m.width, m.height = w, 30
		m.cursor = -1
		m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{Repo: testRepo,
			Number: 3248, Title: strings.Repeat("long title ", 20),
			Author: "verylongusername", CIState: "SUCCESS", UpdatedAt: time.Now(),
		}}})
		m.board.Apply(board.Result{Index: 1})
		for _, l := range strings.Split(m.View(), "\n") {
			if got := lipgloss.Width(stripANSI(l)); got > w {
				t.Fatalf("width %d: line is %d cells: %q", w, got, stripANSI(l))
			}
		}
	}
}

// The author cell is three initials, and those three are exactly what is
// searchable: a match always highlights characters the user can see. Typing a
// whole login finds nothing, because the rest of it is not on the row.
func TestSearchMatchesAuthorInitials(t *testing.T) {
	m := loaded(t, 120, 24, []github.PR{
		{Repo: testRepo, Number: 1, Title: "fix the thing", Author: "immanuel", UpdatedAt: time.Unix(900, 0)},
		{Repo: testRepo, Number: 2, Title: "unrelated work", Author: "someoneelse", UpdatedAt: time.Unix(800, 0)},
	}, nil)
	rows := m.board.Sections()[0].Rows

	if !m.rowMatches(rows[0], true, "imm") {
		txt, _ := m.searchText(rows[0], true)
		t.Errorf("author initials should match: %q", txt)
	}
	if m.rowMatches(rows[1], true, "imm") {
		t.Error("the other author should not match imm")
	}
	// Characters 4+ of the login are not drawn, so they are not searchable.
	if m.rowMatches(rows[0], true, "immanuel") {
		t.Error("a full login matched though only three characters are on screen")
	}

	// And the hit lands inside the author cell, not the title.
	_, cells := m.searchText(rows[0], true)
	hits := cellHits(m.matchSpans(rows[0], true, "imm"), cells.author)
	if len(hits) != 3 {
		t.Errorf("want all three initials marked, got %v", hits)
	}
}

// Sections above a resolving one must not move. A section that resolves empty
// collapses (that is its value changing), but everything already drawn above
// it stays put -- the jump the user sees is rows shifting under the cursor.
func TestResolvingASectionDoesNotMoveTheOnesAboveIt(t *testing.T) {
	cfg := testCfg()
	cfg.Rules = append(cfg.Rules, config.Rule{Name: "Third", Query: "x"})
	m := New(cfg, nil)
	m.width, m.height = 120, 40

	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{
		{Repo: testRepo, Number: 1, Title: "first section row", CIState: "SUCCESS", UpdatedAt: time.Now()},
	}})

	lineOf := func(needle string) int {
		for i, l := range strings.Split(m.View(), "\n") {
			if strings.Contains(stripANSI(l), needle) {
				return i
			}
		}
		return -1
	}
	before := lineOf("first section row")
	if before < 0 {
		t.Fatal("row not rendered")
	}

	// Sections 2 and 3 resolve, one empty, one with rows.
	m.board.Apply(board.Result{Index: 1})
	if got := lineOf("first section row"); got != before {
		t.Errorf("an empty section landing below moved the row above it: %d -> %d", before, got)
	}
	m.board.Apply(board.Result{Index: 2, PRs: []github.PR{
		{Repo: testRepo, Number: 2, Title: "third section row", CIState: "SUCCESS", UpdatedAt: time.Now()},
	}})
	if got := lineOf("first section row"); got != before {
		t.Errorf("a later section landing moved the row above it: %d -> %d", before, got)
	}
}

// An empty section is one line once resolved, not a reserved block: six blank
// rows for a section with nothing in it wastes most of a short pane.
func TestResolvedEmptySectionIsOneLine(t *testing.T) {
	cfg := testCfg()
	// A third rule below, so the blanks measured are the empty section's own
	// reservation rather than the padding that pins the footer.
	cfg.Rules = append(cfg.Rules, config.Rule{Name: "Third", Query: "x"})
	m := New(cfg, nil)
	m.width, m.height = 120, 40
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{
		{Repo: testRepo, Number: 1, Title: "a", CIState: "SUCCESS", UpdatedAt: time.Now()},
	}})
	m.board.Apply(board.Result{Index: 1})
	m.board.Apply(board.Result{Index: 2, PRs: []github.PR{
		{Repo: testRepo, Number: 2, Title: "b", CIState: "SUCCESS", UpdatedAt: time.Now()},
	}})

	lines := strings.Split(m.View(), "\n")
	for i, l := range lines {
		plain := stripANSI(l)
		if !strings.Contains(plain, "Review requested") {
			continue
		}
		// The header names the section and the dash is the one row beneath it,
		// so an empty section costs a header plus exactly one line.
		if i+1 >= len(lines) {
			t.Fatal("no line after the empty section's header")
		}
		if got := strings.TrimSpace(stripANSI(lines[i+1])); got != "\u2014" {
			t.Fatalf("expected the dash under the header, got %q", got)
		}
		// And no blanks after it: there is no separator row in this layout.
		if got := strings.TrimSpace(stripANSI(lines[i+2])); got == "" {
			t.Errorf("empty section is followed by a blank row")
		}
		return
	}
	t.Fatal("section header not found")
}

// esc and q close the help overlay rather than quitting: opening help must
// never cost the user their session by reflex. `j` no longer closes it -- it
// scrolls -- which is exactly why the closing keys are printed on the page.
func TestHelpOverlayClosesWithoutQuitting(t *testing.T) {
	for _, key := range []string{"esc", "q", "?"} {
		m := New(testCfg(), nil)
		m.width, m.height = 100, 30
		m.showHelp = true

		got, cmd := m.handleKey(keyOf(key))
		if cmd != nil {
			t.Errorf("%q from help should not issue a command (quit?)", key)
		}
		if got.(Model).showHelp {
			t.Errorf("%q should close the help overlay", key)
		}
	}

	// ctrl+c still quits from anywhere.
	m := New(testCfg(), nil)
	m.showHelp = true
	if _, cmd := m.handleKey(keyOf("ctrl+c")); cmd == nil {
		t.Error("ctrl+c should still quit from the help overlay")
	}
}

// Everything that is not a scroll or search key closes the page. A reader who
// guesses wrong still gets out, which is what keeps a scrolling overlay from
// being somewhere you can be trapped.
func TestAnyUnknownKeyStillClosesTheHelp(t *testing.T) {
	for _, key := range []string{"x", "z", "1", "enter", "r", "o"} {
		m := New(testCfg(), nil)
		m.width, m.height = 100, 14
		m.showHelp = true
		got, _ := m.handleKey(keyOf(key))
		if got.(Model).showHelp {
			t.Errorf("%q left the reader stuck in the help page", key)
		}
	}
}

// The page scrolls, reaches its end, and cannot be scrolled off either edge.
func TestHelpScrolls(t *testing.T) {
	m := New(testCfg(), nil)
	m.width, m.height = 120, 14
	m.showHelp = true

	total := len(m.helpLines())
	if total <= m.height {
		t.Fatalf("help fits at h=%d, this test proves nothing", m.height)
	}

	first := func(mm Model) string {
		return stripANSI(strings.Split(mm.helpOverlay(), "\n")[0])
	}
	top := first(m)
	if !strings.Contains(top, "KEYS") {
		t.Fatalf("help does not start at the top: %q", top)
	}

	// j moves by one and k brings it back.
	m = press(m, runeKey('j'))
	if first(m) == top {
		t.Error("j did not scroll the help page")
	}
	m = press(m, runeKey('k'))
	if first(m) != top {
		t.Errorf("k did not scroll back: %q, want %q", first(m), top)
	}
	if m.showHelp == false {
		t.Fatal("scrolling closed the page")
	}

	// It cannot be scrolled above the first line.
	for i := 0; i < 5; i++ {
		m = press(m, runeKey('k'))
	}
	if first(m) != top {
		t.Errorf("k ran off the top: %q", first(m))
	}

	// G reaches the end and the last legend line is on screen.
	m = press(m, runeKey('G'))
	out := stripANSI(m.helpOverlay())
	if !strings.Contains(out, "config:") {
		t.Errorf("G did not reach the bottom:\n%s", out)
	}
	if !strings.Contains(out, "end") {
		t.Errorf("the page did not say it was at the end:\n%s", out)
	}

	// Holding G cannot push past it.
	last := m.helpOverlay()
	m = press(m, runeKey('G'))
	m = press(m, runeKey('j'))
	if m.helpOverlay() != last {
		t.Error("the page scrolled past its own end")
	}

	// g returns to the top, and ctrl+d/ctrl+u move by half a page.
	m = press(m, runeKey('g'))
	if first(m) != top {
		t.Errorf("g did not return to the top: %q", first(m))
	}
	m = press(m, keyOf("ctrl+d"))
	half := m.helpScroll
	if half != halfPage(m.height) {
		t.Errorf("ctrl+d moved %d lines, want half a page (%d)", half, halfPage(m.height))
	}
	m = press(m, keyOf("ctrl+u"))
	if first(m) != top {
		t.Errorf("ctrl+u did not come back: %q", first(m))
	}

	// pgdn/pgup are the full page, and are no longer the same key as ctrl+d/u.
	// The separation is the whole point: half that equalled full would be a
	// rename, not a feature.
	m = press(m, keyOf("pgdown"))
	if m.helpScroll != fullPage(m.height) {
		t.Errorf("pgdn moved %d lines, want a full page (%d)", m.helpScroll, fullPage(m.height))
	}
	if m.helpScroll == half {
		t.Errorf("pgdn and ctrl+d both moved %d lines; they should differ at h=%d", half, m.height)
	}
	m = press(m, keyOf("pgup"))
	if first(m) != top {
		t.Errorf("pgup did not come back: %q", first(m))
	}
}

// Half a page floors at one line. The arithmetic rounds to zero below four
// rows, and a key that moves nothing is indistinguishable from a broken one.
func TestHalfPageNeverRoundsToZero(t *testing.T) {
	for h := 0; h <= 6; h++ {
		if got := halfPage(h); got < 1 {
			t.Errorf("halfPage(%d) = %d, want at least 1", h, got)
		}
		if got := fullPage(h); got < 1 {
			t.Errorf("fullPage(%d) = %d, want at least 1", h, got)
		}
	}
	if got := halfPage(42); got != 20 {
		t.Errorf("halfPage(42) = %d, want 20", got)
	}
}

// Re-opening starts at the top: the scroll position is a property of the
// reading, not of the session.
func TestHelpReopensAtTheTop(t *testing.T) {
	mine, review := samplePRs()
	m := loaded(t, 120, 14, mine, review)
	m = press(m, runeKey('?'))
	m = press(m, runeKey('G'))
	m = press(m, runeKey('q'))
	if m.showHelp {
		t.Fatal("q should have closed the page")
	}
	m = press(m, runeKey('?'))
	if got := stripANSI(strings.Split(m.helpOverlay(), "\n")[0]); !strings.Contains(got, "KEYS") {
		t.Errorf("help reopened mid-page: %q", got)
	}
}

// The board has no viewport to scroll, so a page key moves the cursor -- by
// half for ctrl+d/u and by a whole one for pgdn/pgup, the same split the help
// page uses. Both ends stop rather than wrap, like j and k.
func TestBoardPageScrolling(t *testing.T) {
	m := New(testCfg(), nil)
	m.width, m.height = 120, 24

	var prs []github.PR
	for i := 0; i < 60; i++ {
		prs = append(prs, github.PR{Repo: testRepo, Number: 100 + i, Title: "t", UpdatedAt: time.Unix(int64(9000-i), 0)})
	}
	m.board.Apply(board.Result{Index: 0, PRs: prs})
	m.board.Apply(board.Result{Index: 1, PRs: nil})

	m.cursor = 0
	m = press(m, keyOf("ctrl+d"))
	if m.cursor != halfPage(m.height) {
		t.Errorf("ctrl+d moved to %d, want half a page (%d)", m.cursor, halfPage(m.height))
	}
	m = press(m, keyOf("ctrl+u"))
	if m.cursor != 0 {
		t.Errorf("ctrl+u moved to %d, want back at the top", m.cursor)
	}

	m = press(m, keyOf("pgdown"))
	if m.cursor != fullPage(m.height) {
		t.Errorf("pgdn moved to %d, want a full page (%d)", m.cursor, fullPage(m.height))
	}
	if m.cursor == halfPage(m.height) {
		t.Error("pgdn and ctrl+d moved the same distance; they should differ")
	}
	m = press(m, keyOf("pgup"))
	if m.cursor != 0 {
		t.Errorf("pgup moved to %d, want back at the top", m.cursor)
	}

	// Neither end runs off: holding a page key stops at the last slot and at 0
	// rather than wrapping or going out of range.
	for i := 0; i < 20; i++ {
		m = press(m, keyOf("ctrl+d"))
	}
	if want := len(m.slots()) - 1; m.cursor != want {
		t.Errorf("ctrl+d ran to %d, want it stopped at the last slot %d", m.cursor, want)
	}
	for i := 0; i < 20; i++ {
		m = press(m, keyOf("pgup"))
	}
	if m.cursor != 0 {
		t.Errorf("pgup ran to %d, want it stopped at 0", m.cursor)
	}
}

// The chords are spelled as their own key types rather than as runes. A runes
// message happens to stringify to the same "ctrl+d", so the wrong one passes a
// test while nothing like it can arrive from a real terminal.
func keyOf(s string) tea.KeyMsg {
	switch s {
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	case "ctrl+d":
		return tea.KeyMsg{Type: tea.KeyCtrlD}
	case "ctrl+u":
		return tea.KeyMsg{Type: tea.KeyCtrlU}
	case "pgdown":
		return tea.KeyMsg{Type: tea.KeyPgDown}
	case "pgup":
		return tea.KeyMsg{Type: tea.KeyPgUp}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func TestSectionJumpNavigation(t *testing.T) {
	cfg := testCfg()
	cfg.Rules = append(cfg.Rules, config.Rule{Name: "Third", Query: "x"})
	m := New(cfg, nil)
	m.width, m.height = 120, 40

	mk := func(base, n int) []github.PR {
		var out []github.PR
		for i := 0; i < n; i++ {
			out = append(out, github.PR{Repo: testRepo, Number: base + i, Title: "t", UpdatedAt: time.Unix(int64(9000-i), 0)})
		}
		return out
	}
	m.board.Apply(board.Result{Index: 0, PRs: mk(100, 3)})
	m.board.Apply(board.Result{Index: 1, PRs: mk(200, 4)})
	m.board.Apply(board.Result{Index: 2, PRs: mk(300, 2)})

	// Section boundaries in cursor space. Starts are the section headers now,
	// and each header occupies a slot of its own: 0, 1+3=4, 4+1+4=9.
	// Starts are the section headers. Asked for by name rather than counted:
	// the offsets shift whenever a section gains or loses rows.
	starts := m.sectionStarts()
	if len(starts) != 3 {
		t.Fatalf("section starts = %v, want one per section", starts)
	}
	for i, got := range starts {
		if want := m.headerSlot(i); got != want {
			t.Fatalf("section %d starts at %d, want its header slot %d", i, got, want)
		}
	}

	press := func(key string) {
		mm, _ := m.handleKey(keyOf(key))
		m = mm.(Model)
	}

	press("l")
	if want := m.headerSlot(1); m.cursor != want {
		t.Errorf("l should land on the second header %d, got %d", want, m.cursor)
	}
	press("l")
	if want := m.headerSlot(2); m.cursor != want {
		t.Errorf("l should land on the third header %d, got %d", want, m.cursor)
	}
	// Past the last section, l stops at the final slot rather than wrapping.
	press("l")
	if want := len(m.slots()) - 1; m.cursor != want {
		t.Errorf("l at the end should stop at %d, got %d", want, m.cursor)
	}

	// h returns to the current section's header, then steps back.
	m = onRow(t, m, len(m.visibleRows())-1)
	press("h")
	if want := m.headerSlot(2); m.cursor != want {
		t.Errorf("h should land on the current section's header %d, got %d", want, m.cursor)
	}
	press("h")
	if want := m.headerSlot(1); m.cursor != want {
		t.Errorf("h should step back to %d, got %d", want, m.cursor)
	}
	press("h")
	press("h")
	if m.cursor != 0 {
		t.Errorf("h at the top should stay at 0, got %d", m.cursor)
	}
}

// An empty section now has somewhere to land: its header. The old gutter had
// nothing to point at when a section had no rows, so l/h had to skip it; with
// a header row the section is on the board whether or not it has contents, and
// jumping to it tells you it resolved empty rather than silently passing over
// it. That is strictly more information for the same keypress.
func TestSectionJumpLandsOnEveryHeaderIncludingEmptyOnes(t *testing.T) {
	cfg := testCfg()
	cfg.Rules = append(cfg.Rules, config.Rule{Name: "Third", Query: "x"})
	m := New(cfg, nil)
	m.width, m.height = 120, 40

	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{Repo: testRepo, Number: 1, UpdatedAt: time.Now()}}})
	m.board.Apply(board.Result{Index: 1})
	m.board.Apply(board.Result{Index: 2, PRs: []github.PR{{Repo: testRepo, Number: 2, UpdatedAt: time.Now()}}})

	starts := m.sectionStarts()
	if len(starts) != 3 {
		t.Fatalf("every section has a header to land on, got %v", starts)
	}
	for i, got := range starts {
		want := m.headerSlot(i)
		if got != want {
			t.Errorf("section %d starts at %d, want its header slot %d", i, got, want)
		}
	}

	// l walks the headers in order, including the empty section's.
	for i := 1; i < 3; i++ {
		mm, _ := m.handleKey(keyOf("l"))
		m = mm.(Model)
		if want := m.headerSlot(i); m.cursor != want {
			t.Fatalf("l %d: cursor %d, want header %d at %d", i, m.cursor, i, want)
		}
		sl, _ := m.slotAt(m.cursor)
		if !sl.isHeader() {
			t.Errorf("l should land on a header, landed on a row")
		}
	}
}

func TestCursorKeepsContextBelowIt(t *testing.T) {
	m := New(testCfg(), nil)
	m.width, m.height = 120, 14

	var prs []github.PR
	for i := 1; i <= 40; i++ {
		prs = append(prs, github.PR{Repo: testRepo,
			Number: 3000 + i, Title: fmt.Sprintf("pr %d", i),
			CIState: "SUCCESS", UpdatedAt: time.Unix(int64(9000-i), 0),
		})
	}
	m.board.Apply(board.Result{Index: 0, PRs: prs})
	m.board.Apply(board.Result{Index: 1})

	// Somewhere in the middle, away from either end of the list.
	m.cursor = 20
	lines := strings.Split(m.View(), "\n")

	cursorAt := -1
	for i, l := range lines {
		if strings.Contains(l, "▌") {
			cursorAt = i
		}
	}
	if cursorAt < 0 {
		t.Fatal("cursor not visible")
	}
	// Rows below the cursor, excluding the footer.
	below := len(lines) - 1 - cursorAt - 1
	if below < scrollOff {
		t.Errorf("only %d rows below the cursor, want at least %d", below, scrollOff)
	}
}

// The margin must collapse at the ends, or the last row could never be
// selected.
func TestCursorReachesBothEndsOfTheList(t *testing.T) {
	m := New(testCfg(), nil)
	m.width, m.height = 120, 12
	var prs []github.PR
	for i := 1; i <= 30; i++ {
		prs = append(prs, github.PR{Repo: testRepo,
			Number: 3000 + i, Title: fmt.Sprintf("pr %d", i),
			CIState: "SUCCESS", UpdatedAt: time.Unix(int64(9000-i), 0),
		})
	}
	m.board.Apply(board.Result{Index: 0, PRs: prs})
	m.board.Apply(board.Result{Index: 1})

	for _, cursor := range []int{0, 29} {
		m.cursor = cursor
		out := m.View()
		if !strings.Contains(out, "▌") {
			t.Errorf("cursor %d is not visible:\n%s", cursor, out)
		}
		want := fmt.Sprintf("pr %d", cursor+1)
		if !strings.Contains(stripANSI(out), want) {
			t.Errorf("cursor %d: %q not on screen", cursor, want)
		}
	}
}

// Every row is one line, so a failing PR can never be half-scrolled: its gate
// names are in the `d` overlay, which is not subject to the fold at all.
func TestFailingRowIsOneLineAndItsGatesAreInTheOverlay(t *testing.T) {
	m := New(testCfg(), nil)
	m.width, m.height = 120, 10

	var prs []github.PR
	for i := 1; i <= 20; i++ {
		pr := github.PR{Repo: testRepo,
			Number: 3000 + i, Title: fmt.Sprintf("pr %d", i),
			CIState: "SUCCESS", UpdatedAt: time.Unix(int64(9000-i), 0),
		}
		if i == 15 {
			pr.CIState = "FAILURE"
			pr.FailedGates = []string{"the-failing-gate", "and-another"}
		}
		prs = append(prs, pr)
	}
	m.board.Apply(board.Result{Index: 0, PRs: prs})
	m.board.Apply(board.Result{Index: 1})
	// The failing PR is the 15th row (i == 15 above, 0-based row 14).
	m = onRow(t, m, 14)

	out := stripANSI(m.View())
	if !strings.Contains(out, "pr 15") {
		t.Fatalf("selected row missing:\n%s", out)
	}
	if strings.Contains(out, "the-failing-gate") {
		t.Errorf("gate names must not take a line in the list:\n%s", out)
	}

	m.showChecks = true
	over := stripANSI(m.View())
	// The overlay shows the COMPLETE list, which the old one-line version
	// could not: it clipped everything past the first gate or two.
	for _, want := range []string{"the-failing-gate", "and-another"} {
		if !strings.Contains(over, want) {
			t.Errorf("checks overlay missing %q:\n%s", want, over)
		}
	}
}

// The cursor must stay on screen and the view must never open on a blank line,
// at every viewport height and row-height mix.
func TestScrollKeepsCursorVisible(t *testing.T) {
	for _, height := range []int{8, 10, 16, 24, 40} {
		for _, failEvery := range []int{0, 2, 3} {
			m := New(testCfg(), nil)
			m.width, m.height = 120, height

			mk := func(base, n int) []github.PR {
				var out []github.PR
				for i := 0; i < n; i++ {
					pr := github.PR{Repo: testRepo,
						Number: base + i, Title: fmt.Sprintf("pr %d", base+i),
						CIState: "SUCCESS", UpdatedAt: time.Unix(int64(9000-i), 0),
					}
					if failEvery > 0 && i%failEvery == 0 {
						pr.CIState, pr.FailedGates = "FAILURE", []string{"a-failing-gate"}
					}
					out = append(out, pr)
				}
				return out
			}
			m.board.Apply(board.Result{Index: 0, PRs: mk(100, 12)})
			m.board.Apply(board.Result{Index: 1, PRs: mk(200, 12)})

			for cursor := 0; cursor < 24; cursor++ {
				m.cursor = cursor
				view := m.View()

				// The top row may be clipped: that is the degree of freedom
				// that lets the cursor hold a constant gap from the bottom, so
				// a detail line at the top is expected, not a defect.
				if first := strings.Split(view, "\n")[0]; strings.TrimSpace(stripANSI(first)) == "" {
					t.Errorf("h=%d fail=%d cursor=%d: viewport starts on a blank line",
						height, failEvery, cursor)
				}
				if !strings.Contains(view, "▌") {
					t.Errorf("h=%d fail=%d cursor=%d: cursor is off screen", height, failEvery, cursor)
				}
			}
		}
	}
}

// Walking back up from the end must not scroll until the cursor nears the top
// edge, and walking down from the start not until it nears the bottom. The top
// line used to be derived from the cursor alone, which pinned the cursor
// scrollOff lines from the bottom, so every k from the end scrolled the board.
func TestBoardHoldsStillUntilTheCursorNearsAnEdge(t *testing.T) {
	const height = 12
	m := New(testCfg(), nil)
	var prs []github.PR
	for i := 0; i < 30; i++ {
		prs = append(prs, github.PR{Repo: testRepo,
			Number: 100 + i, Title: fmt.Sprintf("pr %d", 100+i),
			CIState: "SUCCESS", UpdatedAt: time.Unix(int64(9000-i), 0),
		})
	}
	m.board.Apply(board.Result{Index: 0, PRs: prs})
	m.board.Apply(board.Result{Index: 1})
	m, _ = drive(m, tea.WindowSizeMsg{Width: 120, Height: height}, keyRune('G'))
	body := height - 1

	for range len(m.slots()) - 1 {
		before := m.top
		m, _ = drive(m, keyRune('k'))
		if line := m.cursor - before; line >= scrollOff && m.top != before {
			t.Fatalf("k to slot %d (line %d of %d) scrolled the board from %d to %d",
				m.cursor, line, body, before, m.top)
		}
		if m.cursor-m.top < min(scrollOff, m.cursor) {
			t.Fatalf("k to slot %d left only %d lines above the cursor", m.cursor, m.cursor-m.top)
		}
	}
	for range len(m.slots()) - 1 {
		before := m.top
		m, _ = drive(m, keyRune('j'))
		if line := m.cursor - before; line <= body-1-scrollOff && m.top != before {
			t.Fatalf("j to slot %d (line %d of %d) scrolled the board from %d to %d",
				m.cursor, line, body, before, m.top)
		}
	}
}

// The author column costs 4 cells, so a rule that shows one needs FULL to
// start 4 columns later -- otherwise the title falls below the readable floor
// that the breakpoint exists to guarantee.
func TestAuthorColumnDoesNotStarveTheTitle(t *testing.T) {
	const floor = 53
	for w := 70; w <= 90; w++ {
		for _, author := range []bool{false, true} {
			tier := widthTierFor(w, author)
			if tier != tierFull {
				continue
			}
			got := titleWidth(w, tier)
			if author {
				got -= authorWidth + 1
			}
			if got < floor {
				t.Errorf("w=%d author=%v: title is %d cells, below the %d floor",
					w, author, got, floor)
			}
		}
	}
}

// The checks overlay is a look, not a mode: a movement key closes it and moves
// in one keypress, so inspecting a PR does not interrupt scanning the list.
func TestChecksOverlayClosesOnMovement(t *testing.T) {
	m := New(testCfg(), nil)
	m.width, m.height = 120, 20
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{
		{Repo: testRepo, Number: 1, Title: "a", CIState: "FAILURE", FailedGates: []string{"gate-one"}, UpdatedAt: time.Unix(9000, 0)},
		{Repo: testRepo, Number: 2, Title: "b", CIState: "SUCCESS", UpdatedAt: time.Unix(8000, 0)},
	}})
	m.board.Apply(board.Result{Index: 1})
	m = onRow(t, m, 0)

	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if !m.showChecks {
		t.Fatal("d should open the checks overlay")
	}
	if !strings.Contains(m.View(), "gate-one") {
		t.Errorf("overlay missing the gate name:\n%s", m.View())
	}

	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if m.showChecks {
		t.Error("a movement key should close the overlay")
	}
	if want := m.rowSlot(1); m.cursor != want {
		t.Errorf("the same keypress should also move: cursor %d, want %d", m.cursor, want)
	}

	// esc closes without moving, and without quitting.
	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.showChecks {
		t.Error("esc should close the overlay")
	}
	if want := m.rowSlot(1); m.cursor != want {
		t.Errorf("esc should not move: cursor %d, want %d", m.cursor, want)
	}
}

// The overlay names what is wrong and what is running, and collapses what
// passed. Listing passing checks would bury the signal: on the live board 45%
// of contexts pass and 47% are skipped, against 4% failing.
// See docs/checks-page.md.
func TestChecksOverlayNamesFailuresAndPendingButCountsPasses(t *testing.T) {
	m := New(testCfg(), nil)
	m.width, m.height = 120, 24
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{Repo: testRepo,
		Number: 7, Title: "t", CIState: "FAILURE", UpdatedAt: time.Now(),
		FailedGates:  []string{"build-push-image webapp"},
		PendingGates: []string{"webapp_e2e"},
		PassedCount:  22, SkippedCount: 10,
	}}})
	m.board.Apply(board.Result{Index: 1})
	m = onRow(t, m, 0)
	m.showChecks = true

	out := stripANSI(m.View())
	for _, want := range []string{
		"✗ build-push-image webapp", // failing, named
		"◐ webapp_e2e",              // pending, named
		"22 passing, 10 skipped",    // the rest, counted
	} {
		if !strings.Contains(out, want) {
			t.Errorf("checks overlay missing %q:\n%s", want, out)
		}
	}
	// A passing check's NAME must never appear: the count is the whole point.
	if strings.Contains(out, "lint-typecheck-test") {
		t.Errorf("passing check names must not be listed:\n%s", out)
	}
}

// The row's count is of failures that block the merge. With only optional ones
// the cell drops the count, which is what tells it apart from red ✗n without
// colour, and a nil Required keeps the old every-failure count.
func TestCICellCountsOnlyRequiredFailures(t *testing.T) {
	for _, tc := range []struct {
		name     string
		required map[string]bool
		want     string
	}{
		{"unknown counts all", nil, "✗2"},
		{"one required", map[string]bool{"run-e2e": true}, "✗1"},
		{"none required", map[string]bool{}, "✗ "},
	} {
		pr := github.PR{CIState: "FAILURE", FailedGates: []string{"claude-review", "run-e2e"}, Required: tc.required}
		got, style := ciCell(pr)
		if got != tc.want {
			t.Errorf("%s: cell %q, want %q", tc.name, got, tc.want)
		}
		if tc.want == "✗ " && style.GetForeground() != mutedStyle.GetForeground() {
			t.Errorf("%s: only-optional failures must be muted, not red", tc.name)
		}
	}
}

// Optional failures are still named, below what blocks and what is running,
// so the page accounts for every check without ranking them as equals.
func TestChecksOverlayMarksOptionalFailures(t *testing.T) {
	m := New(testCfg(), nil)
	m.width, m.height = 120, 24
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{Repo: testRepo,
		Number: 7, Title: "t", CIState: "FAILURE", UpdatedAt: time.Now(),
		FailedGates:  []string{"claude-review", "run-e2e"},
		PendingGates: []string{"env-setup"},
		Required:     map[string]bool{"run-e2e": true},
	}}})
	m.board.Apply(board.Result{Index: 1})
	m = onRow(t, m, 0)
	m.showChecks = true

	out := stripANSI(m.View())
	req := strings.Index(out, "✗ run-e2e")
	pend := strings.Index(out, "◐ env-setup")
	opt := strings.Index(out, "✗ claude-review (optional)")
	if req < 0 || pend < 0 || opt < 0 || !(req < pend && pend < opt) {
		t.Errorf("want required, then running, then optional:\n%s", out)
	}
}

// A cancelled check is named so a blocked PR never reads as green, but in the
// muted glyph rather than the failure one: it wants a re-run, not a fix.
func TestChecksOverlayNamesCancelledChecksApartFromFailures(t *testing.T) {
	m := New(testCfg(), nil)
	m.width, m.height = 120, 24
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{Repo: testRepo,
		Number: 7, Title: "t", CIState: "CANCELLED", UpdatedAt: time.Now(),
		CancelledGates: []string{"Build"}, PassedCount: 3,
	}}})
	m.board.Apply(board.Result{Index: 1})
	m = onRow(t, m, 0)
	m.showChecks = true

	out := stripANSI(m.View())
	if !strings.Contains(out, "⊘ Build (cancelled)") {
		t.Errorf("cancelled check not named:\n%s", out)
	}
	if strings.Contains(out, "✗ Build") || strings.Contains(out, "all 3 checks passing") {
		t.Errorf("a cancelled check read as failing or as all green:\n%s", out)
	}
}

// Nearly half the live board sits in PENDING rollup state, where there is
// nothing failing at all. Before pending gates were carried, `d` answered
// "no failing checks" on all of them, which is a dead end.
func TestChecksOverlayOnPendingPRNamesWhatIsRunning(t *testing.T) {
	m := New(testCfg(), nil)
	m.width, m.height = 120, 24
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{Repo: testRepo,
		Number: 8, Title: "t", CIState: "PENDING", UpdatedAt: time.Now(),
		PendingGates: []string{"run platform e2e"},
		PassedCount:  9, SkippedCount: 12,
	}}})
	m.board.Apply(board.Result{Index: 1})
	m = onRow(t, m, 0)
	m.showChecks = true

	out := stripANSI(m.View())
	if !strings.Contains(out, "run platform e2e") {
		t.Errorf("pending gate not named:\n%s", out)
	}
	if strings.Contains(out, "no failing checks") {
		t.Errorf("a pending PR should say what is running, not report a dead end:\n%s", out)
	}
}

// An all-green PR gets the count as the whole answer, not an empty list with a
// footnote under it.
func TestChecksOverlayOnGreenPRIsOneLine(t *testing.T) {
	m := New(testCfg(), nil)
	m.width, m.height = 120, 24
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{Repo: testRepo,
		Number: 9, Title: "t", CIState: "SUCCESS", UpdatedAt: time.Now(),
		PassedCount: 22, SkippedCount: 10,
	}}})
	m.board.Apply(board.Result{Index: 1})
	m = onRow(t, m, 0)
	m.showChecks = true

	if out := stripANSI(m.View()); !strings.Contains(out, "all 22 checks passing") {
		t.Errorf("green PR should report its count:\n%s", out)
	}
}

// The overlay grew a list that can outgrow the pane, so it must clip rather
// than push its own footer off screen -- and say how much it hid.
func TestChecksOverlayFitsThePane(t *testing.T) {
	var gates []string
	for i := range 40 {
		gates = append(gates, fmt.Sprintf("failing-gate-%02d", i))
	}
	for _, height := range []int{8, 12, 20, 24, 50} {
		m := New(testCfg(), nil)
		m.width, m.height = 120, height
		m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{Repo: testRepo,
			Number: 1, Title: "t", CIState: "FAILURE", FailedGates: gates, UpdatedAt: time.Now(),
		}}})
		m.board.Apply(board.Result{Index: 1})
		m = onRow(t, m, 0)
		m.showChecks = true

		out := stripANSI(m.View())
		if n := len(strings.Split(strings.TrimRight(out, "\n"), "\n")); n > height {
			t.Errorf("height %d: overlay is %d lines:\n%s", height, n, out)
		}
		// 40 gates genuinely fit a 50-row pane, so the notice is only owed
		// when something was actually dropped.
		clipped := !strings.Contains(out, "failing-gate-39")
		if clipped != strings.Contains(out, "not shown") {
			t.Errorf("height %d: clipped=%v but the notice disagrees:\n%s", height, clipped, out)
		}
		// The footer has to survive the clip, or the overlay stops telling the
		// user how to leave it.
		if !strings.Contains(out, "any key closes") {
			t.Errorf("height %d: clip ate the footer:\n%s", height, out)
		}
	}
}

// The tally is what makes the overlay's numbers reconcile against GitHub, so
// it has to survive a clip rather than be dropped as ordinary list content.
func TestChecksOverlayTallySurvivesClipping(t *testing.T) {
	var gates []string
	for i := range 40 {
		gates = append(gates, fmt.Sprintf("failing-gate-%02d", i))
	}
	m := New(testCfg(), nil)
	m.width, m.height = 120, 12
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{Repo: testRepo,
		Number: 1, Title: "t", CIState: "FAILURE", FailedGates: gates,
		PassedCount: 9, SkippedCount: 12, UpdatedAt: time.Now(),
	}}})
	m.board.Apply(board.Result{Index: 1})
	m = onRow(t, m, 0)
	m.showChecks = true

	out := stripANSI(m.View())
	if !strings.Contains(out, "9 passing, 12 skipped") {
		t.Errorf("the tally was clipped away:\n%s", out)
	}
	if !strings.Contains(out, "not shown") {
		t.Errorf("clipped without saying so:\n%s", out)
	}
	if n := len(strings.Split(strings.TrimRight(out, "\n"), "\n")); n > m.height {
		t.Errorf("overlay is %d lines in a %d-row pane:\n%s", n, m.height, out)
	}
}

// The board's first line is a row, not a section header. A pinned header was
// tried and removed: bound to the top visible row it froze on the first section
// on any pane tall enough to show the whole board, which is the common case.
// See docs/section-layout.md §13.
func TestNoChromeLineAboveTheList(t *testing.T) {
	m := New(testCfg(), nil)
	m.width, m.height = 120, 40

	var mine []github.PR
	for i := 1; i <= 5; i++ {
		mine = append(mine, github.PR{Repo: testRepo,
			Number: 3000 + i, Title: fmt.Sprintf("mine %d", i),
			CIState: "SUCCESS", UpdatedAt: time.Unix(int64(9000-i), 0),
		})
	}
	m.board.Apply(board.Result{Index: 0, PRs: mine})
	m.board.Apply(board.Result{Index: 1, PRs: []github.PR{
		{Repo: testRepo, Number: 4001, Title: "theirs", CIState: "SUCCESS", UpdatedAt: time.Unix(8000, 0)},
	}})

	lines := strings.Split(m.View(), "\n")
	// The board opens on the first section's header, with its first row
	// directly beneath. The header is section furniture, not a chrome line: it
	// scrolls with the content and the cursor can sit on it, which is what the
	// reverted sticky line could not do.
	first := stripANSI(lines[0])
	if !strings.Contains(first, "Mine") {
		t.Errorf("first line is not the first section's header: %q", first)
	}
	if !strings.Contains(stripANSI(lines[1]), "#3001") {
		t.Errorf("second line is not the first row: %q", stripANSI(lines[1]))
	}
	// A count like "1 of 5" read as a cursor position and was never one. The
	// header's count is a section size, which is why it is a bare number.
	if strings.Contains(first, " of ") {
		t.Errorf("header carries a position count: %q", first)
	}
}

// A header is a row on the board rather than a chrome line above it, so it
// scrolls out of view with the section it names. The old gutter had to
// re-label the top visible row precisely because the label was pinned
// per-row; an inline header is simply above its rows or it is gone.
func TestSectionHeaderScrollsWithItsSection(t *testing.T) {
	m := New(testCfg(), nil)
	m.width, m.height = 120, 14

	var prs []github.PR
	for i := 1; i <= 30; i++ {
		prs = append(prs, github.PR{Repo: testRepo,
			Number: 3000 + i, Title: fmt.Sprintf("pr %d", i),
			CIState: "SUCCESS", UpdatedAt: time.Unix(int64(9000-i), 0),
		})
	}
	m.board.Apply(board.Result{Index: 0, PRs: prs})
	m.board.Apply(board.Result{Index: 1})

	// At the top the header is on screen, above its first row.
	m = onHeader(t, m, 0)
	if top := stripANSI(strings.Split(m.View(), "\n")[0]); !strings.Contains(top, "Mine") {
		t.Errorf("header missing at the top of the board: %q", top)
	}

	// Scrolled deep into the same section it has left the viewport rather than
	// following the cursor down it.
	m = onRow(t, m, 25)
	body := strings.Join(strings.Split(stripANSI(m.View()), "\n")[:m.height-1], "\n")
	if strings.Contains(body, "  Mine") {
		t.Errorf("header stuck to the viewport instead of scrolling:\n%s", body)
	}
	if strings.Contains(body, "pr 1 ") {
		t.Fatalf("board did not scroll, test proves nothing:\n%s", body)
	}
}

// A header announces its section's size -- the count the 8-cell gutter could
// never carry. A section still loading has no count to show yet.
func TestSectionHeaderCarriesTheRowCount(t *testing.T) {
	m := New(testCfg(), nil)
	m.width, m.height = 120, 40
	var mine []github.PR
	for i := 1; i <= 4; i++ {
		mine = append(mine, github.PR{Repo: testRepo,
			Number: 3000 + i, Title: fmt.Sprintf("pr %d", i),
			CIState: "SUCCESS", UpdatedAt: time.Unix(int64(9000-i), 0),
		})
	}
	m.board.Apply(board.Result{Index: 0, PRs: mine})

	header := func(name string) string {
		t.Helper()
		for _, l := range strings.Split(stripANSI(m.View()), "\n") {
			if strings.Contains(l, name) {
				return strings.TrimSpace(l)
			}
		}
		t.Fatalf("no header for %q", name)
		return ""
	}
	if got := header("Mine"); !strings.HasSuffix(got, "4") {
		t.Errorf("resolved section header lacks its count: %q", got)
	}
	// Still pending: a number here would be about to change under the reader.
	if got := header("Review requested"); got != "Review requested" {
		t.Errorf("pending header should carry no count: %q", got)
	}
}

// Sections are separated by the header's own background, not by a blank row.
// A blank is a line the cursor cannot occupy, and every such line costs one to
// the worst-case scroll delta -- docs/uniform-rows.md §4.1.
func TestNoBlankRowBetweenSections(t *testing.T) {
	m := New(testCfg(), nil)
	m.width, m.height = 120, 20
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{
		{Repo: testRepo, Number: 1, Title: "a", CIState: "SUCCESS", UpdatedAt: time.Unix(9000, 0)},
	}})
	m.board.Apply(board.Result{Index: 1, PRs: []github.PR{
		{Repo: testRepo, Number: 2, Title: "b", CIState: "SUCCESS", UpdatedAt: time.Unix(8999, 0)},
	}})

	lines, slotStarts := m.body("")
	// Every line in the list is a slot: that is the invariant, stated as code.
	if len(lines) != len(slotStarts) {
		t.Errorf("%d lines for %d slots: some line is not addressable", len(lines), len(slotStarts))
	}
	for i, l := range lines {
		if strings.TrimSpace(stripANSI(l)) == "" {
			t.Errorf("line %d is blank: %q", i, stripANSI(l))
		}
	}
}

// The footer names the cursor's section and where the cursor sits inside it.
// It is bound to the cursor and not to the top visible row: at the user's 40
// rows the board does not scroll, so a top-row binding is frozen at the first
// section forever. That is how the sticky line died -- docs/section-layout.md
// §13.1, §14.
func TestFooterNamesTheCursorSection(t *testing.T) {
	m := New(testCfg(), nil)
	m.width, m.height = 120, 40

	var mine, review []github.PR
	for i := 1; i <= 5; i++ {
		mine = append(mine, github.PR{Repo: testRepo, Number: 3000 + i, Title: fmt.Sprintf("mine %d", i),
			CIState: "SUCCESS", UpdatedAt: time.Unix(int64(9000-i), 0)})
	}
	for i := 1; i <= 7; i++ {
		review = append(review, github.PR{Repo: testRepo, Number: 4000 + i, Title: fmt.Sprintf("theirs %d", i),
			CIState: "SUCCESS", UpdatedAt: time.Unix(int64(8000-i), 0)})
	}
	m.board.Apply(board.Result{Index: 0, PRs: mine})
	m.board.Apply(board.Result{Index: 1, PRs: review})

	// The board must fit, or this proves nothing about the frozen case.
	if len(m.visibleRows()) >= m.height-1 {
		t.Fatalf("board scrolls at h=%d, the case under test is the one that does not", m.height)
	}

	foot := func() string {
		lines := strings.Split(stripANSI(m.View()), "\n")
		return lines[len(lines)-1]
	}

	want := []string{}
	for i := 1; i <= 5; i++ {
		want = append(want, fmt.Sprintf("MINE · %d of 5", i))
	}
	for i := 1; i <= 7; i++ {
		want = append(want, fmt.Sprintf("REVIEW REQUESTED · %d of 7", i))
	}

	// Walked by PR row rather than by raw index: the indices now interleave
	// headers, and a header's footer reads "MINE · 5" -- the section's size,
	// with no position, because the cursor is at the section and not in it.
	for i, w := range want {
		m = onRow(t, m, i)
		if got := foot(); !strings.Contains(got, w) {
			t.Fatalf("row %d: footer %q, want it to carry %q", i, got, w)
		}
	}

	// On a header the footer names the section and its size, and claims no
	// position inside it.
	m = onHeader(t, m, 0)
	if got := foot(); !strings.Contains(got, "MINE · 5") || strings.Contains(got, " of ") {
		t.Errorf("header footer %q, want the section size and no position", got)
	}

	// The repo yielded the slot, so it is not also drawn there.
	if strings.Contains(foot(), "o/r") {
		t.Errorf("the repo still holds the footer's right field: %q", foot())
	}
}

// The value has to change on every keypress, which is the property the sticky
// line failed: it read `MINE · 1 of 5` on all 34 frames of a walk down the
// board. Asserting the strings differ catches a regression to any binding that
// is constant while the cursor moves.
func TestFooterSectionCountChangesOnEveryKeypress(t *testing.T) {
	m := New(testCfg(), nil)
	m.width, m.height = 120, 40

	var mine, review []github.PR
	for i := 1; i <= 6; i++ {
		mine = append(mine, github.PR{Repo: testRepo, Number: 3000 + i, Title: fmt.Sprintf("mine %d", i),
			CIState: "SUCCESS", UpdatedAt: time.Unix(int64(9000-i), 0)})
	}
	for i := 1; i <= 6; i++ {
		review = append(review, github.PR{Repo: testRepo, Number: 4000 + i, Title: fmt.Sprintf("theirs %d", i),
			CIState: "SUCCESS", UpdatedAt: time.Unix(int64(8000-i), 0)})
	}
	m.board.Apply(board.Result{Index: 0, PRs: mine})
	m.board.Apply(board.Result{Index: 1, PRs: review})

	field := func() string {
		lines := strings.Split(stripANSI(m.View()), "\n")
		last := lines[len(lines)-1]
		f := strings.Fields(last)
		// The field is the trailing `NAME · N of M`, four fields from the end.
		if len(f) < 5 {
			t.Fatalf("footer has no section field: %q", last)
		}
		return strings.Join(f[len(f)-5:], " ")
	}

	prev := ""
	for c := 0; c < len(m.visibleRows()); c++ {
		m.cursor = c
		now := field()
		if now == prev {
			t.Fatalf("cursor %d: footer field did not change, still %q", c, now)
		}
		prev = now
	}
}

// At 80 columns the two fields do not both fit. The keys clip and the section
// name survives whole, because the keys are a reminder of what the user knows
// and the name is the only place the full name and count exist.
func TestFooterKeepsTheSectionNameWhenItClips(t *testing.T) {
	mine, review := samplePRs()
	for _, w := range []int{80, 86, 100, 120} {
		m := loaded(t, w, 24, mine, review)
		m = onRow(t, m, 1)
		lines := strings.Split(stripANSI(m.View()), "\n")
		foot := lines[len(lines)-1]
		if !strings.Contains(foot, "MINE · 2 of 2") {
			t.Errorf("w=%d: the section name did not survive the clip: %q", w, foot)
		}
		if lipgloss.Width(foot) > w {
			t.Errorf("w=%d: footer overflows at %d cells: %q", w, lipgloss.Width(foot), foot)
		}
	}
}

// `gg` is the vim chord. It works because bare `g` is already top, so the
// second press repeats a move that is its own fixed point -- no pending-key
// mode, and a stray `g` cannot leave the board waiting for a key.
func TestGGGoesToTheTop(t *testing.T) {
	mine, review := samplePRs()
	m := loaded(t, 120, 20, mine, review)

	m.cursor = 2
	m = press(m, runeKey('g'))
	m = press(m, runeKey('g'))
	if m.cursor != 0 {
		t.Errorf("gg left the cursor at %d, want 0", m.cursor)
	}

	// A stray single `g` is a complete move, not half a chord: the next key is
	// read as itself.
	m.cursor = 2
	m = press(m, runeKey('g'))
	if m.cursor != 0 {
		t.Errorf("a bare g left the cursor at %d, want 0", m.cursor)
	}
	m = press(m, runeKey('j'))
	if m.cursor != 1 {
		t.Errorf("j after g moved to %d, want 1: g swallowed the next key", m.cursor)
	}

	// G is still the bottom, and gG is not a chord that wedges anything. The
	// bottom is the last slot, which is the last PR row on a board whose final
	// section has any.
	m = press(m, runeKey('g'))
	m = press(m, runeKey('G'))
	if want := len(m.slots()) - 1; m.cursor != want {
		t.Errorf("G left the cursor at %d, want the last slot %d", m.cursor, want)
	}
}

// The help page lists every key the board and the search actually handle. It
// drifted once already: the arrows, home/end, o and the two extra quit keys
// were all live and undocumented.
func TestHelpListsEveryKeyThatIsHandled(t *testing.T) {
	m := New(testCfg(), nil)
	m.width, m.height = 120, 40
	out := stripANSI(m.helpOverlay())

	for _, k := range []string{
		"j / k", "↓ ↑", "l / h", "→ ←", "g / G", "home", "end",
		"enter", "o ", "d ", "/ ", "r ", "? ", "q ", "esc", "ctrl+c",
		"ctrl+n/p", "ctrl+j/k", "backspace", "n / N",
		"ctrl+d/u", "pgdn/pgup",
	} {
		if !strings.Contains(out, k) {
			t.Errorf("help does not mention %q:\n%s", k, out)
		}
	}

	// The legend advertised ctrl+u as clearing the query long after the binding
	// existed to do it. A key that means half a page must not be documented as
	// meaning something else on the one page that explains the keys.
	if strings.Contains(out, "clear the query") {
		t.Errorf("help still advertises a query-clearing key that no longer exists:\n%s", out)
	}

	// This page's own keys are on its bottom row rather than in the list, so
	// they are checked where they actually live -- on a pane short enough that
	// the row is drawn.
	short := New(testCfg(), nil)
	short.width, short.height = 120, 14
	short.showHelp = true
	hint := stripANSI(short.helpOverlay())
	for _, k := range []string{"esc", "q", "?", "close", "scroll"} {
		if !strings.Contains(hint, k) {
			t.Errorf("the help page does not say %q on its own bottom row:\n%s", k, hint)
		}
	}
}

// The help page never overflows its pane and never clips its top: it scrolls
// instead. Clipping from the top is the bug this replaces -- it lost KEYS, the
// section anyone opening `?` is looking for. A two-column fold was tried and
// reverted; it still clipped on a short pane, so it paid layout complexity
// without buying the fix.
func TestHelpFitsEveryPane(t *testing.T) {
	for _, h := range []int{14, 20, 24, 30, 40, 58, 80} {
		for _, w := range []int{80, 120, 147} {
			m := New(testCfg(), nil)
			m.width, m.height = w, h
			m.showHelp = true
			lines := strings.Split(m.helpOverlay(), "\n")
			if len(lines) > h {
				t.Errorf("%dx%d: help is %d lines, pane is %d", w, h, len(lines), h)
			}
			for _, l := range lines {
				if lipgloss.Width(l) > w {
					t.Errorf("%dx%d: help line overflows at %d cells: %q",
						w, h, lipgloss.Width(l), stripANSI(l))
				}
			}

			out := stripANSI(m.helpOverlay())
			// The page always opens on KEYS, at every size.
			if !strings.Contains(out, "KEYS") || !strings.Contains(out, "j / k") {
				t.Errorf("%dx%d: help does not open on the key list:\n%s", w, h, out)
			}

			full := len(m.helpLines())
			if full <= h {
				// It fits: no hint row, no indicator fuss.
				if strings.Contains(out, fmt.Sprintf("of %d", full)) {
					t.Errorf("%dx%d: a page that fits still drew a position:\n%s", w, h, out)
				}
				for _, want := range []string{"SEARCH", "CI", "REVIEW", "BLOCKERS", "ROWS", "config:"} {
					if !strings.Contains(out, want) {
						t.Errorf("%dx%d: help lost %s though it fits:\n%s", w, h, want, out)
					}
				}
				continue
			}
			// It does not fit, so it says where you are and how to leave.
			if !strings.Contains(out, fmt.Sprintf("of %d", full)) {
				t.Errorf("%dx%d: scrolling page did not show its position:\n%s", w, h, out)
			}
			for _, want := range []string{"esc", "q", "?", "scroll"} {
				if !strings.Contains(out, want) {
					t.Errorf("%dx%d: hint row does not mention %q:\n%s", w, h, want, out)
				}
			}
		}
	}
}

// Every line of the legend is reachable by scrolling, at the shortest pane the
// board supports. A reference you cannot reach the bottom of is not one.
func TestHelpScrollReachesEveryLine(t *testing.T) {
	m := New(testCfg(), nil)
	m.width, m.height = 120, 14
	m.showHelp = true

	seen := map[string]bool{}
	for i := 0; i < len(m.helpLines())+5; i++ {
		for _, l := range strings.Split(stripANSI(m.helpOverlay()), "\n") {
			seen[strings.TrimRight(l, " ")] = true
		}
		m = press(m, runeKey('j'))
	}
	for _, l := range m.helpLines() {
		if want := strings.TrimRight(stripANSI(l), " "); !seen[want] {
			t.Errorf("line never became visible while scrolling: %q", want)
		}
	}
}

// drive runs messages through Update the way the runtime does, returning the
// rendered frame. Every test above this one pokes the Model directly, and three
// bugs in a row shipped green because of it: the path the terminal actually
// uses -- tea.KeyMsg through Update, after a WindowSizeMsg -- was never
// exercised.
func drive(m Model, msgs ...tea.Msg) (Model, string) {
	var mm tea.Model = m
	for _, msg := range msgs {
		mm, _ = mm.Update(msg)
	}
	return mm.(Model), mm.(Model).View()
}

func keyRune(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

// The help scrolls through the real Update path, at every height -- including
// the ones where the legend nearly fits, which is where it broke. `j` must
// change the frame and must not close the page.
func TestHelpScrollsThroughUpdate(t *testing.T) {
	m := New(testCfg(), nil)
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{
		{Repo: testRepo, Number: 1, Title: "a", CIState: "SUCCESS", UpdatedAt: time.Unix(9000, 0)},
	}})
	m.board.Apply(board.Result{Index: 1})

	total := len(m.helpLines())
	// Heights either side of the legend's own length: total-1 and total are the
	// boundary where "fits" and "scrolls" disagree about the hint row.
	for _, h := range []int{12, 14, 20, 30, 40, total - 2, total - 1, total, total + 1, 58} {
		if h < 2 {
			continue
		}
		start, _ := drive(m, tea.WindowSizeMsg{Width: 120, Height: h}, keyRune('?'))
		if !start.showHelp {
			t.Fatalf("h=%d: ? did not open the help", h)
		}
		before := start.View()

		if len(strings.Split(before, "\n")) > h {
			t.Errorf("h=%d: help renders %d lines into a %d-row pane",
				h, len(strings.Split(before, "\n")), h)
		}

		next, after := drive(start, keyRune('j'))
		if !next.showHelp {
			t.Errorf("h=%d: j closed the help instead of scrolling it", h)
			continue
		}
		fits := total <= h-1
		switch {
		case fits && after != before:
			t.Errorf("h=%d: the whole legend fits, so j should not have moved it", h)
		case !fits && after == before:
			t.Errorf("h=%d: j did not scroll the page:\n%s", h, stripANSI(after))
		}
	}
}

// The hint row is on the page at EVERY height, and j is never a silent no-op.
// Both used to be false in a band of heights around the legend's own length:
// the row was drawn only when the page overflowed, so at h == total the page
// showed no affordance and j did nothing while still eating the key. This walks
// every height rather than sampling, because the defect was two rows wide.
func TestHelpHintIsOnEveryHeight(t *testing.T) {
	m := New(testCfg(), nil)
	total := len(m.helpLines())

	for h := 4; h <= total+6; h++ {
		mm, view := drive(m, tea.WindowSizeMsg{Width: 120, Height: h}, keyRune('?'))
		out := stripANSI(view)
		lines := strings.Split(view, "\n")

		if len(lines) > h {
			t.Errorf("h=%d: %d lines rendered into %d rows", h, len(lines), h)
		}
		// The closing keys are stated at every height: scrolling took `any key
		// closes` away everywhere, not only where the page overflows.
		if !strings.Contains(out, "esc q ? close") {
			t.Errorf("h=%d (total %d): no closing hint on the page:\n%s", h, total, out)
		}

		fits := total <= h-1
		moved, movedView := drive(mm, keyRune('j'))
		if !moved.showHelp {
			t.Fatalf("h=%d: j closed the page", h)
		}
		switch {
		case fits && !strings.Contains(out, fmt.Sprintf("all %d", total)):
			t.Errorf("h=%d: whole legend shown but the row does not say so:\n%s", h, out)
		case fits && movedView != view:
			t.Errorf("h=%d: the legend all fits, so j should not move it", h)
		case !fits && !strings.Contains(out, fmt.Sprintf("of %d", total)):
			t.Errorf("h=%d: no position on a page that does not fit:\n%s", h, out)
		case !fits && movedView == view:
			t.Errorf("h=%d: j did not scroll a page that does not fit:\n%s", h, out)
		}
	}
}

// Every closing key works through Update, and so does a key with no meaning.
// Scrolling took `any key closes` away from j/k, so this is the property that
// keeps the page from being somewhere a reader can be trapped.
func TestHelpClosesThroughUpdate(t *testing.T) {
	m := New(testCfg(), nil)
	for _, h := range []int{14, 45, 58} {
		for _, k := range []tea.KeyMsg{
			{Type: tea.KeyEsc}, keyRune('q'), keyRune('?'), keyRune('z'), keyRune('1'),
		} {
			open, _ := drive(m, tea.WindowSizeMsg{Width: 120, Height: h}, keyRune('?'))
			if !open.showHelp {
				t.Fatalf("h=%d: ? did not open the help", h)
			}
			closed, _ := drive(open, k)
			if closed.showHelp {
				t.Errorf("h=%d: %q left the reader stuck in the help page", h, k.String())
			}
		}
	}
}

// One k after holding j to the bottom must move the page. The offset used to be
// clamped only at render, so holding j banked an invisible surplus and the
// first several k presses spent it without the page moving -- which reads as k
// being broken, and only ever showed up after a real hold.
func TestHelpScrollBackIsImmediateAfterHoldingJ(t *testing.T) {
	m := New(testCfg(), nil)
	const h = 20
	mm, _ := drive(m, tea.WindowSizeMsg{Width: 120, Height: h}, keyRune('?'))
	if len(m.helpLines()) <= h {
		t.Skip("legend fits; nothing to overshoot")
	}
	for i := 0; i < 40; i++ {
		mm, _ = drive(mm, keyRune('j'))
	}
	atEnd := mm.View()

	back, afterK := drive(mm, keyRune('k'))
	if !back.showHelp {
		t.Fatal("k closed the page")
	}
	if afterK == atEnd {
		t.Error("k did not move the page after j was held to the bottom")
	}
}

// runCmd runs whatever a keypress returned and reports the message it produced,
// so a test can assert on the status line the user actually sees.
func runCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		return nil
	}
	return cmd()
}

// withClipboard swaps the pbcopy shell-out for a recorder, so the suite never
// writes to the developer's real clipboard.
func withClipboard(t *testing.T, err error) *string {
	t.Helper()
	var got string
	prev := copyToClipboard
	copyToClipboard = func(s string) error {
		got = s
		return err
	}
	t.Cleanup(func() { copyToClipboard = prev })
	return &got
}

// `y` is the vim yank verb, on a board that already answers to j/k, g/G and
// l/h. It copies the selected PR's URL and says so on the status line, because
// a clipboard write is invisible otherwise.
func TestYCopiesTheSelectedURL(t *testing.T) {
	got := withClipboard(t, nil)

	mine, review := samplePRs()
	mine[0].URL = "https://github.com/o/r/pull/3248"
	m := loaded(t, 120, 20, mine, review)
	m = onRow(t, m, 0)

	next, cmd := m.handleKey(runeKey('y'))
	m = next.(Model)
	msg := runCmd(t, cmd)

	if *got != "https://github.com/o/r/pull/3248" {
		t.Errorf("copied %q, want the selected PR's url", *got)
	}
	if s, ok := msg.(asyncStatusMsg); !ok || !strings.Contains(s.text, "3248") {
		t.Errorf("status was %v, want it to name the PR that was copied", msg)
	}
}

// The cursor is what decides, not the board order: y must follow the selection.
func TestYCopiesTheRowUnderTheCursor(t *testing.T) {
	got := withClipboard(t, nil)

	mine, review := samplePRs()
	mine[0].URL = "https://github.com/o/r/pull/3248"
	mine[1].URL = "https://github.com/o/r/pull/3100"
	m := loaded(t, 120, 20, mine, review)
	m = press(m, runeKey('j'))

	_, cmd := m.handleKey(runeKey('y'))
	runCmd(t, cmd)
	if *got != "https://github.com/o/r/pull/3100" {
		t.Errorf("copied %q, want the second row's url", *got)
	}
}

// Nothing selected is a real state -- an empty board -- and it must not reach
// for a PR that is not there. A search cannot produce it: nothing is hidden,
// so a query matching nothing still leaves a row under the cursor.
func TestYWithNothingSelectedDoesNotCrash(t *testing.T) {
	got := withClipboard(t, nil)

	m := loaded(t, 120, 20, nil, nil)
	next, cmd := m.handleKey(runeKey('y'))
	m = next.(Model)
	msg := runCmd(t, cmd)

	if *got != "" {
		t.Errorf("copied %q from an empty board, want nothing", *got)
	}
	if msg != nil || m.status == "" {
		t.Errorf("status=%q msg=%v, want a synchronous no-selection status", m.status, msg)
	}
}

// A query that matches nothing hides nothing, so the cursor is still on a real
// row and y copies it. The empty-board case above is the only no-selection
// state left.
func TestYWithASearchMatchingNothingStillCopies(t *testing.T) {
	got := withClipboard(t, nil)

	mine, review := samplePRs()
	mine[0].URL = "https://github.com/o/r/pull/3248"
	m := typeQuery(loaded(t, 120, 20, mine, review), "zzzznotathing")
	if n := len(m.matchIndexes()); n != 0 {
		t.Fatalf("expected the query to match nothing, got %d", n)
	}
	if n := len(m.visibleRows()); n != 3 {
		t.Fatalf("the board lost rows to a query: %d of 3 left", n)
	}

	// y is query text while searching, so the copy is reached the way the user
	// would: leave the search first.
	m = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	_, cmd := m.handleKey(runeKey('y'))
	runCmd(t, cmd)

	if *got != mine[0].URL {
		t.Errorf("copied %q, want the cursor row's url", *got)
	}
}

// A failed pbcopy is reported rather than silently looking like it worked.
func TestYReportsAFailedCopy(t *testing.T) {
	withClipboard(t, fmt.Errorf("pbcopy: not found"))

	mine, review := samplePRs()
	mine[0].URL = "https://github.com/o/r/pull/3248"
	m := loaded(t, 120, 20, mine, review)

	_, cmd := m.handleKey(runeKey('y'))
	msg := runCmd(t, cmd)
	s, ok := msg.(asyncStatusMsg)
	if !ok || !strings.Contains(s.text, "copy failed") {
		t.Errorf("status was %v, want it to report the failure", msg)
	}
}

func TestRefreshInvalidatesPendingCopyStatus(t *testing.T) {
	withClipboard(t, nil)
	mine, review := samplePRs()
	mine[0].URL = "https://github.com/o/r/pull/3248"
	m := onRow(t, loaded(t, 120, 20, mine, review), 0)

	next, cmd := m.handleKey(runeKey('y'))
	m = next.(Model)
	msg := runCmd(t, cmd)
	next, _ = m.refresh()
	m = next.(Model)
	next, _ = m.Update(msg)
	if got := next.(Model).status; got != "" {
		t.Fatalf("copy completion from before refresh set status %q", got)
	}
}

// The footer hint and the help page both have to name y, or the key is
// undiscoverable -- which is how o, home/end and the arrows drifted before.
func TestCopyKeyIsDocumented(t *testing.T) {
	m := New(testCfg(), nil)
	m.width, m.height = 140, 40

	if foot := stripANSI(m.footer("")); !strings.Contains(foot, "y copy") {
		t.Errorf("the footer does not offer the copy key: %q", foot)
	}
	if out := stripANSI(m.helpOverlay()); !strings.Contains(out, "copy") {
		t.Errorf("the help page does not mention copying:\n%s", out)
	}
}

// Guards the tier breakpoints and the title arithmetic against drifting from
// DESIGN.md §3.8's table. The minWidth constant itself is not asserted here --
// four tests below use it as a loop bound, which exercises it against real
// rendering rather than restating its value.
func TestDocumentedTiersMatchTheCode(t *testing.T) {
	for _, c := range []struct {
		w    int
		want tier
	}{
		{40, tierNarrow}, {59, tierNarrow},
		{60, tierMid}, {75, tierMid},
		{76, tierFull}, {400, tierFull},
	} {
		if got := widthTier(c.w); got != c.want {
			t.Errorf("width %d: tier %v, want %v", c.w, got, c.want)
		}
	}
	for _, c := range []struct{ w, want int }{
		{76, 76 - 23}, {60, 60 - 19}, {40, 40 - 15},
	} {
		if got := titleWidth(c.w, widthTier(c.w)); got != c.want {
			t.Errorf("width %d: title %d, want %d", c.w, got, c.want)
		}
	}
}

// slots() and body() must agree line for line, because every slot index the
// model uses -- the cursor, the footer's position, l/h -- indexes slots() while
// what the user sees comes from body(). They desynced once: body() gained a
// note slot that slots() did not know about, which silently shifted every slot
// after an empty section by one, so the footer named the wrong section and
// selected() returned the wrong PR. Counting lines does not catch that; this
// checks the kinds line up.
func TestSlotsAgreeWithBodyLineForLine(t *testing.T) {
	states := []func(m *Model, i int){
		func(m *Model, i int) { // ready with rows
			m.board.Apply(board.Result{Index: i, PRs: []github.PR{
				{Repo: testRepo, Number: 100 + i, Title: "t", CIState: "SUCCESS", UpdatedAt: time.Unix(int64(9000-i), 0)},
			}})
		},
		func(m *Model, i int) { m.board.Apply(board.Result{Index: i}) },               // empty
		func(m *Model, i int) { m.board.Apply(board.Result{Index: i, Err: errTest}) }, // failed
		func(m *Model, i int) {}, // pending
	}
	for a := range states {
		for b := range states {
			for c := range states {
				cfg := config.Config{Repos: []config.Repo{{Name: "o/r"}}, Rules: []config.Rule{
					{Name: "One", Query: "a"}, {Name: "Two", Query: "b"}, {Name: "Three", Query: "c"},
				}}
				m := New(cfg, nil)
				m.width, m.height = 147, 20
				for i, pick := range []int{a, b, c} {
					states[pick](&m, i)
				}

				lines, slotStarts := m.body("")
				sl := m.slots()
				if len(sl) != len(slotStarts) {
					t.Fatalf("%d/%d/%d: slots()=%d but body drew %d slots",
						a, b, c, len(sl), len(slotStarts))
				}
				// Every line is a slot, so slotStarts[i] is slot i's line and
				// each one must draw the kind slots() claims.
				if len(lines) != len(slotStarts) {
					t.Fatalf("%d/%d/%d: %d lines for %d slots -- some line is not addressable",
						a, b, c, len(lines), len(slotStarts))
				}
				for i, start := range slotStarts {
					got := stripANSI(lines[start])
					switch {
					case sl[i].isRow():
						if !strings.Contains(got, "#") {
							t.Errorf("%d/%d/%d: slot %d claims a row, drew %q", a, b, c, i, got)
						}
					case sl[i].isHeader():
						if !strings.Contains(got, sl[i].section) {
							t.Errorf("%d/%d/%d: slot %d claims header %q, drew %q",
								a, b, c, i, sl[i].section, got)
						}
					case sl[i].isNote():
						if strings.Contains(got, "#") {
							t.Errorf("%d/%d/%d: slot %d claims a note, drew a row %q", a, b, c, i, got)
						}
					}
				}
			}
		}
	}
}

// The footer's section and position are read off the cursor's slot, so they
// have to be right for every slot kind and in the presence of a note line --
// the case that silently shifted them by one section before notes became
// slots.
func TestFooterSectionIsCorrectForEverySlot(t *testing.T) {
	cfg := config.Config{Repos: []config.Repo{{Name: "o/r"}}, Rules: []config.Rule{
		{Name: "One", Query: "a"}, {Name: "Empty", Query: "b"}, {Name: "Three", Query: "c"},
	}}
	m := New(cfg, nil)
	m.width, m.height = 147, 30
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{
		{Repo: testRepo, Number: 11, Title: "a", CIState: "SUCCESS", UpdatedAt: time.Unix(9002, 0)},
		{Repo: testRepo, Number: 12, Title: "b", CIState: "SUCCESS", UpdatedAt: time.Unix(9001, 0)},
	}})
	m.board.Apply(board.Result{Index: 1}) // resolved empty -> header + note
	m.board.Apply(board.Result{Index: 2, PRs: []github.PR{
		{Repo: testRepo, Number: 31, Title: "c", CIState: "SUCCESS", UpdatedAt: time.Unix(9000, 0)},
	}})

	for i, sl := range m.slots() {
		m.cursor = i
		name, pos, total := m.cursorSection()
		if name != sl.section {
			t.Errorf("slot %d: footer says %q, slot belongs to %q", i, name, sl.section)
		}
		switch {
		case sl.isRow():
			if pos < 1 || pos > total {
				t.Errorf("slot %d (%s): position %d of %d is out of range", i, name, pos, total)
			}
		default:
			if pos != 0 {
				t.Errorf("slot %d (%s) is not a row but claims position %d", i, name, pos)
			}
		}
	}

	// The last section's single row must read "1 of 1", not "3 of 1": rowIdx
	// counts across the board and has to be offset by the sections above.
	m.cursor = m.rowSlot(2)
	if name, pos, total := m.cursorSection(); name != "Three" || pos != 1 || total != 1 {
		t.Errorf("last section's row: %q %d of %d, want Three 1 of 1", name, pos, total)
	}
}

// errTest is a section fetch failure, for the Failed state in the matrix below.
var errTest = errors.New("rule query failed")

// The scroll invariant across section STATE, not just row counts.
//
// This test exists because the first version of it did not vary state: it
// applied rows to every rule, so every section was Ready and non-empty, and it
// passed while three kinds of unselectable line were live in body(). Each cost
// a line of delta by the rule in docs/uniform-rows.md §4.1, and the pending
// case cost more as the pane grew, because the placeholder block scaled with
// height. Cold start is every launch, so that was the common case.
//
// Row counts are the easy axis and state is the one that bites, so the matrix
// walks every combination of Ready-with-rows / Ready-empty / Failed / Pending
// across three sections, at every height the rest of the suite uses.
func TestOneKeypressScrollsAtMostOneLineAcrossSectionStates(t *testing.T) {
	type state int
	const (
		ready state = iota
		empty
		failed
		pending
	)
	name := map[state]string{ready: "ready", empty: "empty", failed: "failed", pending: "pending"}

	mk := func(base, n int) []github.PR {
		var prs []github.PR
		for i := 0; i < n; i++ {
			prs = append(prs, github.PR{Repo: testRepo,
				Number: base + i, Title: fmt.Sprintf("pr %d", base+i),
				Author: "someone", CIState: "SUCCESS", UpdatedAt: time.Unix(int64(9000-i), 0),
			})
		}
		return prs
	}

	// Every ordered triple of states. Order matters: a pending section above a
	// populated one is a different layout from one below it.
	all := []state{ready, empty, failed, pending}
	for _, a := range all {
		for _, b := range all {
			for _, c := range all {
				states := [3]state{a, b, c}
				label := name[a] + "/" + name[b] + "/" + name[c]
				for _, h := range []int{5, 8, 10, 12, 16, 20, 24} {
					cfg := config.Config{Repos: []config.Repo{{Name: "o/r"}}, Rules: []config.Rule{
						{Name: "Mine", Query: "a"},
						{Name: "Needs my review", Query: "b"},
						{Name: "All open", Query: "c"},
					}}
					m := New(cfg, nil)
					m.width, m.height = 147, h

					// Pending is the absence of a result, so those rules get
					// no Apply at all. Frontier means a pending rule holds
					// every rule below it pending too, which is itself one of
					// the shapes worth covering.
					for i, st := range states {
						switch st {
						case ready:
							m.board.Apply(board.Result{Index: i, PRs: mk(3200+i*100, 4)})
						case empty:
							m.board.Apply(board.Result{Index: i})
						case failed:
							m.board.Apply(board.Result{Index: i, Err: errTest})
						case pending:
							// no result
						}
					}

					lines, slotStarts := m.body("")
					// The invariant, stated directly: every line in the list
					// is a slot. This is the cheap check that catches a new
					// unselectable line before the walk below has to.
					if len(lines) != len(slotStarts) {
						t.Errorf("%s h=%d: %d lines for %d slots -- some line is not addressable",
							label, h, len(lines), len(slotStarts))
						continue
					}
					if len(slotStarts) < 2 || len(lines) <= h-1 {
						continue // nothing to scroll
					}

					prev := -1
					for _, dir := range []int{1, -1} {
						start, stop := 0, len(slotStarts)-1
						if dir < 0 {
							start, stop = len(slotStarts)-1, 0
						}
						m.cursor = start
						_, prev = window(lines, m.cursor, h-1, slotStarts, 0)
						for cur := start; cur != stop; cur += dir {
							m.cursor = cur + dir
							_, now := window(lines, m.cursor, h-1, slotStarts, prev)
							if d := now - prev; d < -1 || d > 1 {
								t.Fatalf("%s h=%d: slot %d -> %d scrolled %d lines, want at most 1",
									label, h, cur, m.cursor, d)
							}
							prev = now
						}
					}
				}
			}
		}
	}
}

// Every colour the board draws must resolve through the user's theme. This is
// the guard on the whole arc of colour work on this branch: an author palette,
// five commit-type tints, a header band and a brightened selection accent were
// all fixed 256-cube values, and all of them are gone.
//
// A fixed value is not banned in principle -- DESIGN-GUIDE.md §3 allows one
// where a measured contrast requirement has no themed answer -- but there is
// currently no such case, so any cube index appearing here is a regression
// until DESIGN.md §3.3's table says otherwise.
func TestNoFixedCubeColoursAnywhereOnTheBoard(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	cfg := config.Config{Repos: []config.Repo{{Name: "o/r"}}, Rules: []config.Rule{
		{Name: "Mine", Query: "a", Tree: true},
		{Name: "Needs my review", Query: "b", Author: true},
		{Name: "Empty", Query: "c"},
		{Name: "Broken", Query: "d"},
	}}
	m := New(cfg, nil)
	m.width, m.height = 147, 24
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{
		{Repo: testRepo, Number: 1, Title: "fix(api): AF-1 a failing one", CIState: "FAILURE",
			FailedGates: []string{"g1", "g2"}, Mergeable: "CONFLICTING", UpdatedAt: time.Now()},
		{Repo: testRepo, Number: 2, Title: "feat(web): a draft", CIState: "PENDING",
			IsDraft: true, Author: "someone", UpdatedAt: time.Now()},
		{Repo: testRepo, Number: 3, Title: "chore: an approved one", CIState: "SUCCESS",
			Review: "APPROVED", Author: "other", UpdatedAt: time.Now()},
	}})
	m.board.Apply(board.Result{Index: 1, PRs: []github.PR{
		{Repo: testRepo, Number: 4, Title: "plain prose title", CIState: "SUCCESS",
			Review: "REVIEW_REQUIRED", Author: "third", UpdatedAt: time.Now()},
	}})
	m.board.Apply(board.Result{Index: 2})               // resolved empty -> note
	m.board.Apply(board.Result{Index: 3, Err: errTest}) // failed -> error note

	// Walk every slot so the selected variant of every line kind is rendered.
	for i := range m.slots() {
		m.cursor = i
		for _, line := range strings.Split(m.View(), "\n") {
			if strings.Contains(line, "38;5;") || strings.Contains(line, "48;5;") {
				t.Fatalf("cursor %d: a fixed cube colour is on the board:\n%q", i, line)
			}
		}
	}
}
