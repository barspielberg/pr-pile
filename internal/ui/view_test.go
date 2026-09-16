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
		if !strings.Contains(stripANSI(l), "REVIEW REQUESTED") {
			continue
		}
		if got := strings.TrimSpace(stripANSI(lines[i+1])); got != "—" {
			t.Fatalf("expected the dash directly under the header, got %q", got)
		}
		// The next line must be the following section or the padding, not
		// more reserved blanks for this one.
		blank := 0
		for j := i + 2; j < len(lines) && strings.TrimSpace(stripANSI(lines[j])) == ""; j++ {
			blank++
		}
		if blank > 1 {
			t.Errorf("empty section holds %d blank rows after its dash", blank)
		}
		return
	}
	t.Fatal("section header not found")
}

// esc and q close the help overlay rather than quitting: opening help must
// never cost the user their session by reflex.
func TestHelpOverlayClosesWithoutQuitting(t *testing.T) {
	for _, key := range []string{"esc", "q", "?", "j"} {
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

// A PR with failing checks is two lines. Scrolling to only its first line
// leaves the detail line below the fold.
func TestSelectedRowDetailLineStaysVisible(t *testing.T) {
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
			pr.FailedGates = []string{"the-failing-gate"}
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
	if !strings.Contains(out, "the-failing-gate") {
		t.Errorf("the selected row's detail line is below the fold:\n%s", out)
	}
}

// Rows are one or two lines tall and sections add blank lines, so a viewport
// offset measured in lines made every keypress scroll a different distance and
// could leave an orphaned detail line at the top. The viewport must always
// start on a whole unit.
func TestScrollAlwaysStartsOnAWholeRow(t *testing.T) {
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

				var top string
				for _, l := range strings.Split(view, "\n") {
					if s := strings.TrimSpace(stripANSI(l)); s != "" {
						top = s
						break
					}
				}
				if strings.HasPrefix(top, "└") {
					t.Errorf("h=%d fail=%d cursor=%d: viewport starts on an orphaned detail line: %q",
						height, failEvery, cursor, top)
				}
				if !strings.Contains(view, "▌") {
					t.Errorf("h=%d fail=%d cursor=%d: cursor is off screen", height, failEvery, cursor)
				}
			}
		}
	}
}
