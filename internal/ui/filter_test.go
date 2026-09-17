package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/barspielberg/prs-mng/internal/board"
	"github.com/barspielberg/prs-mng/internal/github"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func runeKey(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func press(m Model, msg tea.KeyMsg) Model {
	next, _ := m.handleKey(msg)
	return next.(Model)
}

// typeQuery enters filter mode and types the query one key at a time, the way
// the user would: the board narrows on every keystroke, not only the last.
func typeQuery(m Model, q string) Model {
	m = press(m, runeKey('/'))
	for _, r := range q {
		m = press(m, runeKey(r))
	}
	return m
}

// loaded is a two-section board with both rules resolved, so nothing is pending
// and every row below is drawable.
func loaded(t *testing.T, width, height int, mine, review []github.PR) Model {
	t.Helper()
	m := New(testCfg(), nil)
	m.width, m.height = width, height
	m.board.Apply(board.Result{Index: 0, PRs: mine})
	m.board.Apply(board.Result{Index: 1, PRs: review})
	return m
}

func samplePRs() ([]github.PR, []github.PR) {
	mine := []github.PR{
		{Number: 3248, Title: "feat(api-service): PROJ-2037 refuse order plan writes",
			CIState: "SUCCESS", UpdatedAt: time.Unix(300, 0)},
		{Number: 3100, Title: "fix(pricing): rounding on invoice totals",
			CIState: "SUCCESS", UpdatedAt: time.Unix(200, 0)},
	}
	review := []github.PR{
		{Number: 4001, Title: "chore(deps): bump lipgloss",
			CIState: "SUCCESS", UpdatedAt: time.Unix(100, 0)},
	}
	return mine, review
}

func TestQueryNarrowsToMatches(t *testing.T) {
	mine, review := samplePRs()
	m := typeQuery(loaded(t, 120, 40, mine, review), "apisvc")

	out := m.View()
	if !strings.Contains(out, "#3248") {
		t.Errorf("fuzzy query %q should match the api-service PR:\n%s", "apisvc", out)
	}
	// "apisvc" has no subsequence in this title at all, so it must be gone.
	// (A title that *does* contain the letters in order stays: that is fuzzy
	// matching working, not a bug -- ranking is what separates them.)
	if strings.Contains(out, "#3100") {
		t.Errorf("#3100 should be filtered out by %q:\n%s", "apisvc", out)
	}
	if got := len(m.visibleRows()); got == len(loaded(t, 120, 40, mine, review).visibleRows()) {
		t.Errorf("query did not narrow the board at all (%d rows)", got)
	}
}

// Fuzzy matching is deliberately permissive, so the ranking of matches is what
// makes it usable: the PR the query obviously means must come first.
func TestBetterMatchesRankFirst(t *testing.T) {
	mine := []github.PR{
		{Number: 4001, Title: "chore(deps): bump lipgloss", CIState: "SUCCESS", UpdatedAt: time.Unix(300, 0)},
		{Number: 3248, Title: "feat(api-service): refuse order plan writes", CIState: "SUCCESS", UpdatedAt: time.Unix(200, 0)},
	}
	m := typeQuery(loaded(t, 120, 40, mine, nil), "refuse")

	rows := m.visibleRows()
	if len(rows) == 0 {
		t.Fatal("no matches")
	}
	if rows[0].PR.Number != 3248 {
		t.Errorf("best match is #%d, want #3248 first", rows[0].PR.Number)
	}
}

// Typing a number must find the PR by its number, not only by title text.
func TestQueryMatchesPRNumber(t *testing.T) {
	mine, review := samplePRs()
	m := typeQuery(loaded(t, 120, 40, mine, review), "3248")

	rows := m.visibleRows()
	if len(rows) != 1 {
		t.Fatalf("want 1 row matching \"3248\", got %d", len(rows))
	}
	if rows[0].PR.Number != 3248 {
		t.Errorf("matched #%d, want #3248", rows[0].PR.Number)
	}
}

func TestCtrlNAndCtrlPMoveWithinFilteredSet(t *testing.T) {
	var mine []github.PR
	for i := 0; i < 5; i++ {
		mine = append(mine, github.PR{
			Number: 100 + i, Title: fmt.Sprintf("alpha match %d", i),
			CIState: "SUCCESS", UpdatedAt: time.Unix(int64(500-i), 0),
		})
	}
	mine = append(mine, github.PR{
		Number: 999, Title: "zeta unrelated", CIState: "SUCCESS", UpdatedAt: time.Unix(1, 0),
	})
	m := typeQuery(loaded(t, 120, 40, mine, nil), "alpha")

	if got := len(m.visibleRows()); got != 5 {
		t.Fatalf("want 5 matches, got %d", got)
	}

	down := tea.KeyMsg{Type: tea.KeyCtrlN}
	up := tea.KeyMsg{Type: tea.KeyCtrlP}

	for want := 1; want <= 4; want++ {
		m = press(m, down)
		if m.cursor != want {
			t.Fatalf("ctrl+n: cursor %d, want %d", m.cursor, want)
		}
	}
	// Past the end it must stop on the last match, not run off it.
	m = press(m, down)
	if m.cursor != 4 {
		t.Errorf("ctrl+n past the end: cursor %d, want it clamped to 4", m.cursor)
	}

	for want := 3; want >= 0; want-- {
		m = press(m, up)
		if m.cursor != want {
			t.Fatalf("ctrl+p: cursor %d, want %d", m.cursor, want)
		}
	}
	m = press(m, up)
	if m.cursor != 0 {
		t.Errorf("ctrl+p past the top: cursor %d, want it clamped to 0", m.cursor)
	}

	// The cursor must still address a row inside the filtered set.
	if pr, ok := m.selected(); !ok || !strings.HasPrefix(pr.Title, "alpha") {
		t.Errorf("selection left the filtered set: %+v", pr)
	}
}

// ctrl+j/k are the same motions under the other convention.
func TestCtrlJAndCtrlKAlsoMove(t *testing.T) {
	mine, review := samplePRs()
	m := typeQuery(loaded(t, 120, 40, mine, review), "")

	before := len(m.visibleRows())
	m = press(m, tea.KeyMsg{Type: tea.KeyCtrlJ})
	if m.cursor != 1 {
		t.Errorf("ctrl+j: cursor %d, want 1 (of %d rows)", m.cursor, before)
	}
	m = press(m, tea.KeyMsg{Type: tea.KeyCtrlK})
	if m.cursor != 0 {
		t.Errorf("ctrl+k: cursor %d, want 0", m.cursor)
	}
}

func TestEscRestoresTheFullBoard(t *testing.T) {
	mine, review := samplePRs()
	full := loaded(t, 120, 40, mine, review)
	want := full.View()

	m := typeQuery(full, "apisvc")
	if len(m.visibleRows()) == len(full.visibleRows()) {
		t.Fatal("query did not narrow anything, so this test proves nothing")
	}

	m = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.filtering {
		t.Error("esc should leave filter mode")
	}
	if m.filter != "" {
		t.Errorf("esc should clear the query, got %q", m.filter)
	}
	if got := m.View(); got != want {
		t.Errorf("board did not return to its unfiltered frame after esc:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// Narrowing is the point, so a section that matches nothing must take up no
// space at all -- not an empty header, not a placeholder.
func TestEmptySectionsAreHiddenWhileFiltering(t *testing.T) {
	mine, review := samplePRs()
	m := typeQuery(loaded(t, 120, 40, mine, review), "refuse")

	// The gutter clips the rule name to sectionWidth, so match on its stem.
	out := stripANSI(m.View())
	if strings.Contains(out, "REVIEW") {
		t.Errorf("a section with no matches should be hidden, gutter included:\n%s", out)
	}
	if !strings.Contains(out, "MINE") {
		t.Errorf("the matching section should keep its gutter label:\n%s", out)
	}

	// And the section comes back when the query stops excluding it.
	m = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if !strings.Contains(stripANSI(m.View()), "REVIEW") {
		t.Error("the hidden section did not come back after esc")
	}
}

// The filter prompt is a second chrome row, so the body budget shrinks by one.
// The frame must still fit the terminal at every supported width.
func TestFilteredBoardFitsTerminalAtAnyWidth(t *testing.T) {
	var mine []github.PR
	for i := 0; i < 40; i++ {
		mine = append(mine, github.PR{
			Number: 3000 + i, Title: fmt.Sprintf("feat(api-service): %s %d", strings.Repeat("long ", 10), i),
			CIState: "FAILURE", FailedGates: []string{"a", "b"}, UpdatedAt: time.Unix(int64(900-i), 0),
		})
	}

	for w := minWidth; w <= 200; w += 7 {
		for _, h := range []int{6, 12, 24} {
			m := typeQuery(loaded(t, w, h, mine, nil), "api")
			if len(m.visibleRows()) == 0 {
				t.Fatalf("width %d height %d: query matched nothing", w, h)
			}
			for _, cursor := range []int{0, 5, len(m.visibleRows()) - 1} {
				m.cursor = cursor
				m.clampCursor()
				out := m.View()

				if got := strings.Count(out, "\n") + 1; got > h {
					t.Errorf("w=%d h=%d cursor=%d: view is %d lines, terminal is %d",
						w, h, cursor, got, h)
				}
				for _, l := range strings.Split(out, "\n") {
					if got := lipgloss.Width(stripANSI(l)); got > w {
						t.Errorf("w=%d h=%d: line is %d cells: %q", w, h, got, stripANSI(l))
					}
				}
			}
		}
	}
}

// The selected row has to stay on screen while filtering, exactly as it does on
// the full board.
func TestCursorStaysVisibleWhileFiltering(t *testing.T) {
	var mine []github.PR
	for i := 0; i < 30; i++ {
		mine = append(mine, github.PR{
			Number: 200 + i, Title: fmt.Sprintf("refuse item %d", i),
			CIState: "SUCCESS", UpdatedAt: time.Unix(int64(900-i), 0),
		})
	}
	m := typeQuery(loaded(t, 120, 12, mine, nil), "refuse")

	n := len(m.visibleRows())
	for _, cursor := range []int{0, 7, n - 1} {
		m.cursor = cursor
		out := m.View()
		if !strings.Contains(out, "▌") {
			t.Errorf("cursor %d: selected row scrolled off screen:\n%s", cursor, out)
		}
		if got := strings.Count(out, "\n") + 1; got > m.height {
			t.Errorf("cursor %d: view is %d lines, terminal is %d", cursor, got, m.height)
		}
	}
}

// Backspace edits the query rather than exiting, so widening a too-narrow query
// brings rows back.
func TestBackspaceWidensTheQuery(t *testing.T) {
	mine, review := samplePRs()
	m := typeQuery(loaded(t, 120, 40, mine, review), "refusez")
	if got := len(m.visibleRows()); got != 0 {
		t.Fatalf("query %q should match nothing, got %d rows", "refusez", got)
	}

	m = press(m, tea.KeyMsg{Type: tea.KeyBackspace})
	if m.filter != "refuse" {
		t.Errorf("query is %q, want %q", m.filter, "refuse")
	}
	if got := len(m.visibleRows()); got != 1 {
		t.Errorf("want 1 row back after backspace, got %d", got)
	}
	if !m.filtering {
		t.Error("backspace should not leave filter mode")
	}
}

// With no matches the board is empty but the chrome must still render, so the
// user can see the query that produced nothing.
func TestNoMatchesStillRendersPromptAndCount(t *testing.T) {
	mine, review := samplePRs()
	m := typeQuery(loaded(t, 120, 20, mine, review), "zzzqqq")

	out := m.View()
	if !strings.Contains(out, "zzzqqq") {
		t.Errorf("prompt should show the query:\n%s", out)
	}
	if !strings.Contains(out, "0 matches") {
		t.Errorf("prompt should report zero matches:\n%s", out)
	}
	if got := strings.Count(out, "\n") + 1; got > m.height {
		t.Errorf("empty filtered view is %d lines, terminal is %d", got, m.height)
	}
}

// The count on the prompt must agree with what is actually navigable.
func TestPromptCountMatchesVisibleRows(t *testing.T) {
	mine, review := samplePRs()
	for _, q := range []string{"", "o", "apisvc", "3248", "zzz"} {
		m := typeQuery(loaded(t, 120, 40, mine, review), q)
		n := len(m.visibleRows())
		want := fmt.Sprintf("%d matches", n)
		if n == 1 {
			want = "1 match"
		}
		if got := stripANSI(m.promptLine()); !strings.Contains(got, want) {
			t.Errorf("query %q: prompt %q should contain %q", q, got, want)
		}
	}
}

// ctrl+c still quits from inside filter mode; esc must not.
func TestCtrlCQuitsFromFilterMode(t *testing.T) {
	mine, review := samplePRs()
	m := typeQuery(loaded(t, 120, 40, mine, review), "ref")

	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("ctrl+c produced no command, expected tea.Quit")
	}
	if msg := cmd(); msg != tea.Quit() {
		t.Errorf("ctrl+c should quit, got %T", msg)
	}
}

// Keys that are navigation on the board are query text while filtering,
// otherwise "j" could never be typed into a search.
func TestPrintableKeysGoToTheQueryNotNavigation(t *testing.T) {
	mine, review := samplePRs()
	m := press(loaded(t, 120, 40, mine, review), runeKey('/'))

	for _, r := range []rune{'j', 'k', 'g', 'q', 'r'} {
		m = press(m, runeKey(r))
	}
	if m.filter != "jkgqr" {
		t.Errorf("query is %q, want %q", m.filter, "jkgqr")
	}
	if !m.filtering {
		t.Error("typing should not have exited filter mode")
	}
}

// Matched characters are underlined. Underline is the only channel left: the
// title already uses colour for draft and may sit on the selection fill.
func TestMatchedCharactersAreUnderlined(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	mine := []github.PR{{
		Number: 3248, Title: "feat(api-service): refuse order plan writes",
		CIState: "SUCCESS", UpdatedAt: time.Now(),
	}}
	m := typeQuery(loaded(t, 100, 20, mine, nil), "apisvc")

	var row string
	for _, l := range strings.Split(m.View(), "\n") {
		if strings.Contains(l, "#3248") {
			row = l
		}
	}
	if !strings.Contains(row, "\x1b[") || !strings.Contains(row, "4m") {
		t.Errorf("no underline in the matched title:\n%q", row)
	}
	// Highlighting must not change the row's width, or the age column shifts.
	if got := lipgloss.Width(stripANSI(row)); got != 100 {
		t.Errorf("highlighted row is %d cells, want 100", got)
	}

	// Without a filter the same title carries no underline.
	plain := loaded(t, 100, 20, mine, nil)
	for _, l := range strings.Split(plain.View(), "\n") {
		if strings.Contains(l, "#3248") && strings.Contains(l, ";4m") {
			t.Errorf("unfiltered row should not be underlined:\n%q", l)
		}
	}
}

// Space arrives as its own key type rather than as a rune, so a multi-word
// query would silently lose its spaces if that were not handled.
func TestSpaceIsTypedIntoTheQuery(t *testing.T) {
	mine, review := samplePRs()
	m := press(loaded(t, 120, 40, mine, review), runeKey('/'))
	for _, r := range "order" {
		m = press(m, runeKey(r))
	}
	m = press(m, tea.KeyMsg{Type: tea.KeySpace})
	for _, r := range "plan" {
		m = press(m, runeKey(r))
	}

	if m.filter != "order plan" {
		t.Fatalf("query is %q, want %q", m.filter, "order plan")
	}
	rows := m.visibleRows()
	if len(rows) != 1 || rows[0].PR.Number != 3248 {
		t.Errorf("query %q should find #3248, got %v rows", m.filter, len(rows))
	}
}

// Filtering is a view concern: it must not disturb the board's own bucketing.
func TestFilterDoesNotMutateTheBoard(t *testing.T) {
	mine, review := samplePRs()
	m := loaded(t, 120, 40, mine, review)

	before := make([][]int, 0)
	for _, s := range m.board.Sections() {
		var nums []int
		for _, r := range s.Rows {
			nums = append(nums, r.PR.Number)
		}
		before = append(before, nums)
	}

	typeQuery(m, "apisvc").View()

	for i, s := range m.board.Sections() {
		if len(s.Rows) != len(before[i]) {
			t.Fatalf("section %d changed size: %d -> %d", i, len(before[i]), len(s.Rows))
		}
		for j, r := range s.Rows {
			if r.PR.Number != before[i][j] {
				t.Errorf("section %d row %d changed: #%d -> #%d", i, j, before[i][j], r.PR.Number)
			}
		}
	}
}

// A bare number is a lookup, not a fuzzy search: "3248" must not also match a
// title that happens to contain 3...2...4...8 in order.
func TestNumericQueryMatchesNumbersOnly(t *testing.T) {
	rows := []board.Row{
		{PR: github.PR{Number: 3248, Title: "fix(webapp): reach the dependency popup"}},
		{PR: github.PR{Number: 3134, Title: "feat: 3 of 2 with 4 and 8 scattered"}},
		{PR: github.PR{Number: 3249, Title: "another one"}},
	}

	got := filterSection(rows, "3248")
	if len(got) != 1 || got[0].PR.Number != 3248 {
		t.Errorf("want only #3248, got %v", numbersOf(got))
	}

	// A partial number narrows by prefix as it is typed.
	if got := filterSection(rows, "324"); len(got) != 2 {
		t.Errorf("want #3248 and #3249, got %v", numbersOf(got))
	}

	// A leading # is accepted.
	if got := filterSection(rows, "#3248"); len(got) != 1 || got[0].PR.Number != 3248 {
		t.Errorf("want only #3248 for \"#3248\", got %v", numbersOf(got))
	}

	// Mixed queries stay fuzzy.
	if got := filterSection(rows, "reach"); len(got) != 1 || got[0].PR.Number != 3248 {
		t.Errorf("text query should still be fuzzy, got %v", numbersOf(got))
	}

	// Nothing to underline when the query is numeric.
	if hits := matchedTitleIndexes(rows[0], "3248"); len(hits) != 0 {
		t.Errorf("numeric query should not highlight the title, got %v", hits)
	}
}

func numbersOf(rows []board.Row) []int {
	out := []int{}
	for _, r := range rows {
		out = append(out, r.PR.Number)
	}
	return out
}
