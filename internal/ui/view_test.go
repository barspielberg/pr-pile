package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/barspielberg/prs-mng/internal/board"
	"github.com/barspielberg/prs-mng/internal/config"
	"github.com/barspielberg/prs-mng/internal/github"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func testCfg() config.Config {
	return config.Config{
		Repo: "o/r",
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
		{Number: 99, Title: "should not be visible yet", UpdatedAt: time.Now()},
	}})
	if out := m.View(); strings.Contains(out, "should not be visible yet") {
		t.Error("section 2 rendered before section 1 resolved:\n" + out)
	}
	// The section still announces itself while pending -- now via the row
	// gutter rather than a header band.
	if !strings.Contains(stripANSI(m.View()), "REVIEW") {
		t.Error("expected the pending section in the gutter")
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
	m.showChecks = true
	if !strings.Contains(m.View(), "webapp_e2e") {
		t.Errorf("gate name missing from the checks overlay:\n%s", m.View())
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
		// Display cells, not bytes: the ▌ mark is multi-byte. The cluster sits
		// 11 cells into the row body, which the section gutter offsets.
		want := sectionWidth + 2 + 11
		if got := lipgloss.Width(row[:strings.Index(row, "✓")]); got != want {
			t.Errorf("width %d: CI glyph at column %d, want %d\n%q", w, got, want, row)
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
	// selBg is 237, which the 256-colour profile emits as 48;5;237.
	if !strings.Contains(sel, "48;5;237") {
		t.Errorf("selected row has no background fill:\n%q", sel)
	}
	if strings.Contains(unsel, "48;5;237") {
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

// A short board must not let the footer float up the screen: the prompt and
// footer belong on the bottom edge whether there are two rows or fifty.
func TestFooterStaysPinnedToTheBottom(t *testing.T) {
	for _, n := range []int{1, 3, 40} {
		m := New(testCfg(), nil)
		m.width, m.height = 120, 24

		var prs []github.PR
		for i := 0; i < n; i++ {
			prs = append(prs, github.PR{
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
		// and the first one carries the first section's gutter name.
		if first := stripANSI(lines[0]); !strings.Contains(first, "MINE") {
			t.Errorf("%d rows: first line is not the first row: %q", n, first)
		}

		// And the same while filtering, where the prompt is a second chrome row.
		m.filtering = true
		m.filter = "pr"
		flines := strings.Split(m.View(), "\n")
		if len(flines) != m.height {
			t.Errorf("%d rows filtered: view is %d lines, want %d", n, len(flines), m.height)
		}
		if got := stripANSI(flines[len(flines)-2]); !strings.Contains(got, "/") {
			t.Errorf("%d rows filtered: prompt not directly above the footer: %q", n, got)
		}
	}
}

// The author column is per rule: a rule like author:@me is all one person, so
// the column would be dead weight there.
func TestAuthorColumnIsPerRule(t *testing.T) {
	m := New(testCfg(), nil)
	m.width, m.height = 120, 30
	pr := github.PR{Number: 1, Title: "a title", Author: "octocat",
		CIState: "SUCCESS", UpdatedAt: time.Now()}
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{pr}})
	m.board.Apply(board.Result{Index: 1, PRs: []github.PR{{
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

func TestAuthorColumnDoesNotOverflow(t *testing.T) {
	cfg := testCfg()
	for i := range cfg.Rules {
		cfg.Rules[i].Author = true
	}
	for w := minWidth; w <= 200; w++ {
		m := New(cfg, nil)
		m.width, m.height = w, 30
		m.cursor = -1
		m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{
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

// Author must be searchable, and a row surviving on an author match must not
// render with nothing marked.
func TestFilterMatchesAuthor(t *testing.T) {
	rows := []board.Row{
		{PR: github.PR{Number: 1, Title: "fix the thing", Author: "octocat"}},
		{PR: github.PR{Number: 2, Title: "unrelated work", Author: "someoneelse"}},
	}
	got := filterSection(rows, "octocat")
	if len(got) == 0 || got[0].PR.Number != 1 {
		t.Fatalf("author query should match #1, got %v", numbersOf(got))
	}
	// The highlighter maps matches back into title indexes; an author-only hit
	// must not claim positions inside the title.
	for i := range matchedTitleIndexes(rows[0], "octocat") {
		if i >= len([]rune(rows[0].PR.Title)) {
			t.Errorf("highlight index %d is outside the title", i)
		}
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
		{Number: 1, Title: "first section row", CIState: "SUCCESS", UpdatedAt: time.Now()},
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
		{Number: 2, Title: "third section row", CIState: "SUCCESS", UpdatedAt: time.Now()},
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
		{Number: 1, Title: "a", CIState: "SUCCESS", UpdatedAt: time.Now()},
	}})
	m.board.Apply(board.Result{Index: 1})
	m.board.Apply(board.Result{Index: 2, PRs: []github.PR{
		{Number: 2, Title: "b", CIState: "SUCCESS", UpdatedAt: time.Now()},
	}})

	lines := strings.Split(m.View(), "\n")
	for i, l := range lines {
		plain := stripANSI(l)
		if !strings.Contains(plain, "REVIEW") {
			continue
		}
		// The section name and its dash share one line now: the gutter carries
		// the name, so an empty section costs exactly one row.
		if !strings.HasSuffix(strings.TrimSpace(plain), "\u2014") {
			t.Fatalf("expected the dash on the section's own line, got %q", plain)
		}
		blank := 0
		for j := i + 1; j < len(lines) && strings.TrimSpace(stripANSI(lines[j])) == ""; j++ {
			blank++
		}
		if blank > 1 {
			t.Errorf("empty section holds %d blank rows after its dash", blank)
		}
		return
	}
	t.Fatal("section not found in the gutter")
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

// Everything that is not a scroll key closes the page. A reader who guesses
// wrong still gets out, which is what keeps a scrolling overlay from being
// somewhere you can be trapped.
func TestAnyUnknownKeyStillClosesTheHelp(t *testing.T) {
	for _, key := range []string{"x", "z", "1", "/", "enter", "r", "o"} {
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

	// g returns to the top, and ctrl+d/ctrl+u move by a page.
	m = press(m, runeKey('g'))
	if first(m) != top {
		t.Errorf("g did not return to the top: %q", first(m))
	}
	m = press(m, keyOf("ctrl+d"))
	paged := first(m)
	if paged == top {
		t.Error("ctrl+d did not page down")
	}
	m = press(m, keyOf("ctrl+u"))
	if first(m) != top {
		t.Errorf("ctrl+u did not page back: %q", first(m))
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

func keyOf(s string) tea.KeyMsg {
	switch s {
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
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
			out = append(out, github.PR{Number: base + i, Title: "t", UpdatedAt: time.Unix(int64(9000-i), 0)})
		}
		return out
	}
	m.board.Apply(board.Result{Index: 0, PRs: mk(100, 3)})
	m.board.Apply(board.Result{Index: 1, PRs: mk(200, 4)})
	m.board.Apply(board.Result{Index: 2, PRs: mk(300, 2)})

	// Section boundaries in cursor space: 0, 3, 7.
	if got := m.sectionStarts(); len(got) != 3 || got[0] != 0 || got[1] != 3 || got[2] != 7 {
		t.Fatalf("section starts = %v, want [0 3 7]", got)
	}

	press := func(key string) {
		mm, _ := m.handleKey(keyOf(key))
		m = mm.(Model)
	}

	press("l")
	if m.cursor != 3 {
		t.Errorf("l from 0 should land on 3, got %d", m.cursor)
	}
	press("l")
	if m.cursor != 7 {
		t.Errorf("l should land on 7, got %d", m.cursor)
	}
	// Past the last section, l stops at the final row rather than wrapping.
	press("l")
	if want := len(m.visibleRows()) - 1; m.cursor != want {
		t.Errorf("l at the end should stop at %d, got %d", want, m.cursor)
	}

	// h returns to the start of the current section, then steps back.
	m.cursor = 9
	press("h")
	if m.cursor != 7 {
		t.Errorf("h should land on the current section start 7, got %d", m.cursor)
	}
	press("h")
	if m.cursor != 3 {
		t.Errorf("h should step back to 3, got %d", m.cursor)
	}
	press("h")
	press("h")
	if m.cursor != 0 {
		t.Errorf("h at the top should stay at 0, got %d", m.cursor)
	}
}

// An empty section has no row to land on, so it must be skipped rather than
// leaving the cursor somewhere that renders nothing.
func TestSectionJumpSkipsEmptySections(t *testing.T) {
	cfg := testCfg()
	cfg.Rules = append(cfg.Rules, config.Rule{Name: "Third", Query: "x"})
	m := New(cfg, nil)
	m.width, m.height = 120, 40

	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{Number: 1, UpdatedAt: time.Now()}}})
	m.board.Apply(board.Result{Index: 1})
	m.board.Apply(board.Result{Index: 2, PRs: []github.PR{{Number: 2, UpdatedAt: time.Now()}}})

	if got := m.sectionStarts(); len(got) != 2 {
		t.Fatalf("empty section should not be a jump target: %v", got)
	}
	mm, _ := m.handleKey(keyOf("l"))
	if got := mm.(Model).cursor; got != 1 {
		t.Errorf("l should skip the empty section and land on 1, got %d", got)
	}
}

// Moving down should reveal what is coming, not pin the cursor to the bottom
// edge. lazygit treated the edge-pinned version as a defect (PR #2915) and
// fzf migrated from 0 to 3 in 2024; nobody migrated the other way.
func TestCursorKeepsContextBelowIt(t *testing.T) {
	m := New(testCfg(), nil)
	m.width, m.height = 120, 14

	var prs []github.PR
	for i := 1; i <= 40; i++ {
		prs = append(prs, github.PR{
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
		prs = append(prs, github.PR{
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
		pr := github.PR{
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
	m.cursor = 14

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
					pr := github.PR{
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

// Once the board is scrolling, the cursor must hold a steady screen line.
//
// A constant row margin and a constant screen position cannot both hold when
// rows differ in height: keeping N whole rows below the cursor moves the bottom
// edge by one or two lines depending on whether those rows carry a failing-check
// line, which makes the cursor bob. The screen position is what the eye tracks,
// so that is the invariant. It still steps when a section header scrolls past,
// because a header genuinely occupies lines.
// The board must never scroll faster than the cursor. This is the invariant
// that five earlier attempts at the viewport math could not hold: with
// variable-height rows a single keypress moved the board 0 to 3 lines
// depending on whether a neighbouring row carried a failing-check line, and
// that is what the user saw as jumping. See docs/uniform-rows.md.
func TestOneKeypressScrollsAtMostOneLine(t *testing.T) {
	cfg := config.Config{Repo: "o/r", Rules: []config.Rule{
		{Name: "Mine", Query: "a", Tree: true},
		{Name: "Needs my review", Query: "b", Author: true},
		{Name: "All open", Query: "c", Author: true},
	}}
	m := New(cfg, nil)
	m.width, m.height = 147, 24

	// Every third PR fails a gate: under the old layout those were the rows
	// that made the scroll uneven, so they are exactly what this must survive.
	mk := func(base int, n int) []github.PR {
		var prs []github.PR
		for i := 0; i < n; i++ {
			pr := github.PR{
				Number: base + i, Title: fmt.Sprintf("pr %d", base+i), Author: "someone",
				CIState: "SUCCESS", UpdatedAt: time.Unix(int64(9000-i), 0),
			}
			if i%3 == 0 {
				pr.CIState = "FAILURE"
				pr.FailedGates = []string{"a-failing-gate", "another-one", "and-a-third"}
			}
			prs = append(prs, pr)
		}
		return prs
	}
	m.board.Apply(board.Result{Index: 0, PRs: mk(3200, 12)})
	m.board.Apply(board.Result{Index: 1, PRs: mk(3300, 10)})
	m.board.Apply(board.Result{Index: 2, PRs: mk(3400, 14)})

	// The viewport is identified by its top line. Comparing whole rendered
	// frames would also catch the selection bar moving, which is not the point.
	topLine := func() string {
		return strings.Split(m.View(), "\n")[0]
	}
	cursorOnScreen := func() bool {
		for _, l := range strings.Split(m.View(), "\n") {
			if strings.Contains(l, "▌") {
				return true
			}
		}
		return false
	}

	rows := len(m.visibleRows())
	if rows < 30 {
		t.Fatalf("board too small to scroll: %d rows", rows)
	}

	// The top line is located by the PR it carries rather than by string
	// equality: when its section started above the window it is re-rendered
	// with the section's name in the gutter, so the bytes differ from body()'s
	// own copy while the line is the same line.
	number := func(line string) string {
		f := strings.Fields(stripANSI(line))
		for _, w := range f {
			if strings.HasPrefix(w, "#") {
				return w
			}
		}
		return ""
	}
	lineIndex := func(top string) int {
		want := number(top)
		if want == "" {
			return -1
		}
		lines, _, _ := m.body("")
		for i, l := range lines {
			if number(l) == want {
				return i
			}
		}
		return -1
	}

	for _, dir := range []int{1, -1} {
		start, stop := 0, rows-1
		if dir < 0 {
			start, stop = rows-1, 0
		}
		m.cursor = start
		prev := lineIndex(topLine())
		for c := start; c != stop; c += dir {
			m.cursor = c + dir
			if !cursorOnScreen() {
				t.Fatalf("cursor %d is off screen", m.cursor)
			}
			now := lineIndex(topLine())
			if now < 0 || prev < 0 {
				t.Fatalf("cursor %d: could not locate the viewport top", m.cursor)
			}
			if d := now - prev; d < -1 || d > 1 {
				t.Errorf("cursor %d -> %d: board scrolled %d lines, want at most 1",
					c, m.cursor, d)
				return
			}
			prev = now
		}
	}
}

// The checks overlay is a look, not a mode: a movement key closes it and moves
// in one keypress, so inspecting a PR does not interrupt scanning the list.
func TestChecksOverlayClosesOnMovement(t *testing.T) {
	m := New(testCfg(), nil)
	m.width, m.height = 120, 20
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{
		{Number: 1, Title: "a", CIState: "FAILURE", FailedGates: []string{"gate-one"}, UpdatedAt: time.Unix(9000, 0)},
		{Number: 2, Title: "b", CIState: "SUCCESS", UpdatedAt: time.Unix(8000, 0)},
	}})
	m.board.Apply(board.Result{Index: 1})

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
	if m.cursor != 1 {
		t.Errorf("the same keypress should also move: cursor %d, want 1", m.cursor)
	}

	// esc closes without moving, and without quitting.
	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.showChecks {
		t.Error("esc should close the overlay")
	}
	if m.cursor != 1 {
		t.Errorf("esc should not move: cursor %d, want 1", m.cursor)
	}
}

// The overlay names what is wrong and what is running, and collapses what
// passed. Listing passing checks would bury the signal: on the live board 45%
// of contexts pass and 47% are skipped, against 4% failing.
// See docs/checks-page.md.
func TestChecksOverlayNamesFailuresAndPendingButCountsPasses(t *testing.T) {
	m := New(testCfg(), nil)
	m.width, m.height = 120, 24
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{
		Number: 7, Title: "t", CIState: "FAILURE", UpdatedAt: time.Now(),
		FailedGates:  []string{"build-push-image webapp"},
		PendingGates: []string{"webapp_e2e"},
		PassedCount:  22, SkippedCount: 10,
	}}})
	m.board.Apply(board.Result{Index: 1})
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

// Nearly half the live board sits in PENDING rollup state, where there is
// nothing failing at all. Before pending gates were carried, `d` answered
// "no failing checks" on all of them, which is a dead end.
func TestChecksOverlayOnPendingPRNamesWhatIsRunning(t *testing.T) {
	m := New(testCfg(), nil)
	m.width, m.height = 120, 24
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{
		Number: 8, Title: "t", CIState: "PENDING", UpdatedAt: time.Now(),
		PendingGates: []string{"run platform e2e"},
		PassedCount:  9, SkippedCount: 12,
	}}})
	m.board.Apply(board.Result{Index: 1})
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
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{
		Number: 9, Title: "t", CIState: "SUCCESS", UpdatedAt: time.Now(),
		PassedCount: 22, SkippedCount: 10,
	}}})
	m.board.Apply(board.Result{Index: 1})
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
		m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{
			Number: 1, Title: "t", CIState: "FAILURE", FailedGates: gates, UpdatedAt: time.Now(),
		}}})
		m.board.Apply(board.Result{Index: 1})
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
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{
		Number: 1, Title: "t", CIState: "FAILURE", FailedGates: gates,
		PassedCount: 9, SkippedCount: 12, UpdatedAt: time.Now(),
	}}})
	m.board.Apply(board.Result{Index: 1})
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
		mine = append(mine, github.PR{
			Number: 3000 + i, Title: fmt.Sprintf("mine %d", i),
			CIState: "SUCCESS", UpdatedAt: time.Unix(int64(9000-i), 0),
		})
	}
	m.board.Apply(board.Result{Index: 0, PRs: mine})
	m.board.Apply(board.Result{Index: 1, PRs: []github.PR{
		{Number: 4001, Title: "theirs", CIState: "SUCCESS", UpdatedAt: time.Unix(8000, 0)},
	}})

	first := stripANSI(strings.Split(m.View(), "\n")[0])
	if !strings.Contains(first, "#3001") {
		t.Errorf("first line is not the first row: %q", first)
	}
	// A count like "1 of 5" read as a cursor position and was never one.
	if strings.Contains(first, " of ") {
		t.Errorf("first line carries a position count: %q", first)
	}
}

// The gutter name is computed against the visible window: the top row always
// carries its section's name, even when the section began above the fold. It
// keeps `│` there -- `╷` claims the section starts on that row, which is false.
func TestTopVisibleRowCarriesItsSectionName(t *testing.T) {
	m := New(testCfg(), nil)
	m.width, m.height = 120, 14

	var prs []github.PR
	for i := 1; i <= 30; i++ {
		prs = append(prs, github.PR{
			Number: 3000 + i, Title: fmt.Sprintf("pr %d", i),
			CIState: "SUCCESS", UpdatedAt: time.Unix(int64(9000-i), 0),
		})
	}
	m.board.Apply(board.Result{Index: 0, PRs: prs})
	m.board.Apply(board.Result{Index: 1})

	m.cursor = 20
	top := stripANSI(strings.Split(m.View(), "\n")[0])
	if !strings.HasPrefix(top, "MINE") {
		t.Errorf("top visible row is unnamed: %q", top)
	}
	if !strings.Contains(top, "│") || strings.Contains(top, "╷") {
		t.Errorf("top row should continue the rule, not start it: %q", top)
	}
	if strings.Contains(top, "pr 1 ") {
		t.Fatalf("board did not scroll, test proves nothing: %q", top)
	}
}

// The rule breaks at a section boundary, so a boundary reads even when the
// name fills all 8 cells and cannot signal it by shape.
func TestSectionBoundaryBreaksTheRule(t *testing.T) {
	m := New(testCfg(), nil)
	m.width, m.height = 120, 20
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{
		{Number: 1, Title: "a", CIState: "SUCCESS", UpdatedAt: time.Unix(9000, 0)},
		{Number: 2, Title: "b", CIState: "SUCCESS", UpdatedAt: time.Unix(8999, 0)},
	}})
	m.board.Apply(board.Result{Index: 1, PRs: []github.PR{
		{Number: 3, Title: "c", CIState: "SUCCESS", UpdatedAt: time.Unix(8998, 0)},
	}})

	var starts, continues int
	for _, l := range strings.Split(stripANSI(m.View()), "\n") {
		switch {
		case strings.Contains(l, "╷"):
			starts++
			if !strings.HasPrefix(strings.TrimSpace(l), "MINE") &&
				!strings.HasPrefix(strings.TrimSpace(l), "REVIEW") {
				t.Errorf("a broken rule without a name: %q", l)
			}
		case strings.Contains(l, "│"):
			continues++
		}
	}
	if starts != 2 {
		t.Errorf("got %d section starts, want 2", starts)
	}
	if continues != 1 {
		t.Errorf("got %d continuation rows, want 1", continues)
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
		mine = append(mine, github.PR{Number: 3000 + i, Title: fmt.Sprintf("mine %d", i),
			CIState: "SUCCESS", UpdatedAt: time.Unix(int64(9000-i), 0)})
	}
	for i := 1; i <= 7; i++ {
		review = append(review, github.PR{Number: 4000 + i, Title: fmt.Sprintf("theirs %d", i),
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

	m.cursor = 0
	for i, w := range want {
		m.cursor = i
		if got := foot(); !strings.Contains(got, w) {
			t.Fatalf("cursor %d: footer %q, want it to carry %q", i, got, w)
		}
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
		mine = append(mine, github.PR{Number: 3000 + i, Title: fmt.Sprintf("mine %d", i),
			CIState: "SUCCESS", UpdatedAt: time.Unix(int64(9000-i), 0)})
	}
	for i := 1; i <= 6; i++ {
		review = append(review, github.PR{Number: 4000 + i, Title: fmt.Sprintf("theirs %d", i),
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
		m.cursor = 1
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

	// G is still the bottom, and gG is not a chord that wedges anything.
	m = press(m, runeKey('g'))
	m = press(m, runeKey('G'))
	if want := len(m.visibleRows()) - 1; m.cursor != want {
		t.Errorf("G left the cursor at %d, want %d", m.cursor, want)
	}
}

// The help page lists every key the board and the filter actually handle. It
// drifted once already: the arrows, home/end, o and the two extra quit keys
// were all live and undocumented.
func TestHelpListsEveryKeyThatIsHandled(t *testing.T) {
	m := New(testCfg(), nil)
	m.width, m.height = 120, 40
	out := stripANSI(m.helpOverlay())

	for _, k := range []string{
		"j / k", "↓ ↑", "l / h", "→ ←", "g / G", "home", "end",
		"enter", "o ", "d ", "/ ", "r ", "? ", "q ", "esc", "ctrl+c",
		"ctrl+n/p", "ctrl+j/k", "backspace", "ctrl+u",
	} {
		if !strings.Contains(out, k) {
			t.Errorf("help does not mention %q:\n%s", k, out)
		}
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
				for _, want := range []string{"FILTER", "CI", "REVIEW", "BLOCKERS", "ROWS", "config:"} {
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
		{Number: 1, Title: "a", CIState: "SUCCESS", UpdatedAt: time.Unix(9000, 0)},
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
	m.cursor = 0

	_, cmd := m.handleKey(runeKey('y'))
	msg := runCmd(t, cmd)

	if *got != "https://github.com/o/r/pull/3248" {
		t.Errorf("copied %q, want the selected PR's url", *got)
	}
	if s, ok := msg.(statusMsg); !ok || !strings.Contains(string(s), "3248") {
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

// Nothing selected is a real state -- an empty board, or a filter that matched
// nothing -- and it must not reach for a PR that is not there.
func TestYWithNothingSelectedDoesNotCrash(t *testing.T) {
	got := withClipboard(t, nil)

	m := loaded(t, 120, 20, nil, nil)
	_, cmd := m.handleKey(runeKey('y'))
	msg := runCmd(t, cmd)

	if *got != "" {
		t.Errorf("copied %q from an empty board, want nothing", *got)
	}
	if s, ok := msg.(statusMsg); !ok || string(s) == "" {
		t.Errorf("status was %v, want it to say nothing is selected", msg)
	}
}

// Filtering to zero matches is the same no-selection state by another route.
func TestYWithAFilterMatchingNothingDoesNotCrash(t *testing.T) {
	got := withClipboard(t, nil)

	mine, review := samplePRs()
	mine[0].URL = "https://github.com/o/r/pull/3248"
	m := typeQuery(loaded(t, 120, 20, mine, review), "zzzznotathing")
	if n := len(m.visibleRows()); n != 0 {
		t.Fatalf("expected the query to match nothing, got %d rows", n)
	}

	// y is query text while filtering, so the copy is reached the way the user
	// would: leave the filter first.
	m = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	m.board.Apply(board.Result{Index: 0, PRs: nil})
	m.board.Apply(board.Result{Index: 1, PRs: nil})
	_, cmd := m.handleKey(runeKey('y'))
	runCmd(t, cmd)

	if *got != "" {
		t.Errorf("copied %q with nothing selected, want nothing", *got)
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
	s, ok := msg.(statusMsg)
	if !ok || !strings.Contains(string(s), "copy failed") {
		t.Errorf("status was %v, want it to report the failure", msg)
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
