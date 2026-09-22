package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/barspielberg/pr-pile/internal/board"
	"github.com/barspielberg/pr-pile/internal/github"
	tea "github.com/charmbracelet/bubbletea"
)

// selectBoard is a two-section board -- 3 PRs under Mine, 2 under Review
// requested, both resolved -- so a range has somewhere to cross a boundary.
// The slot space is header, row, row, row, header, row, row.
func selectBoard(t *testing.T) Model {
	t.Helper()
	m := New(testCfg(), nil)
	m.width, m.height = 120, 20
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{
		{Number: 1, Title: "first", URL: "https://x/1"},
		{Number: 2, Title: "second", URL: "https://x/2"},
		{Number: 3, Title: "third", URL: "https://x/3"},
	}})
	m.board.Apply(board.Result{Index: 1, PRs: []github.PR{
		{Number: 4, Title: "fourth", URL: "https://x/4"},
		{Number: 5, Title: "fifth", URL: "https://x/5"},
	}})
	m.fetching = false
	m.cursor = m.firstRowSlot()
	return m
}

// selectedNumbers is the selection in board order, as numbers, for terse asserts.
func selectedNumbers(m Model) []int {
	var out []int
	for _, pr := range m.selectedPRs() {
		out = append(out, pr.Number)
	}
	return out
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func pressKey(m Model, key string) Model {
	if key == " " {
		return press(m, tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")})
	}
	return press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
}

func TestSpaceSelectsThePRUnderTheCursor(t *testing.T) {
	m := pressKey(selectBoard(t), " ")
	if got := selectedNumbers(m); !equalInts(got, []int{1}) {
		t.Fatalf("selection = %v, want [1]", got)
	}
}

func TestSpaceTwiceDeselects(t *testing.T) {
	m := pressKey(pressKey(selectBoard(t), " "), " ")
	if got := selectedNumbers(m); len(got) != 0 {
		t.Fatalf("selection = %v, want empty", got)
	}
}

func TestSpaceOnAHeaderSelectsNothing(t *testing.T) {
	m := selectBoard(t)
	m.cursor = m.headerSlot(0)
	m = pressKey(m, " ")
	if got := selectedNumbers(m); len(got) != 0 {
		t.Fatalf("selection = %v, want empty", got)
	}
	// A no-op, not a failure: the cursor is allowed to rest on a header.
	if m.status != "" {
		t.Fatalf("status = %q, want none", m.status)
	}
}

func TestSpaceOnANoteSelectsNothing(t *testing.T) {
	m := New(testCfg(), nil)
	m.width, m.height = 120, 20
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{Number: 1, Title: "a"}}})
	m.board.Apply(board.Result{Index: 1}) // resolves empty, so it draws a note
	m.fetching = false
	for i, s := range m.slots() {
		if s.isNote() {
			m.cursor = i
		}
	}
	m = pressKey(m, " ")
	if got := selectedNumbers(m); len(got) != 0 {
		t.Fatalf("selection = %v, want empty", got)
	}
}

// k9s closed "make mark advance to the next line" as not-planned: it saves a
// keypress going down and costs one going up. Without a test, someone will
// "improve" this.
func TestSpaceDoesNotMoveTheCursor(t *testing.T) {
	m := selectBoard(t)
	before := m.cursor
	if got := pressKey(m, " ").cursor; got != before {
		t.Fatalf("cursor = %d after space, want %d", got, before)
	}
}

func TestSelectionSpansSections(t *testing.T) {
	m := selectBoard(t)
	m = pressKey(m, " ")
	m.cursor = m.rowSlot(3) // first row of the second section
	m = pressKey(m, " ")
	if got := selectedNumbers(m); !equalInts(got, []int{1, 4}) {
		t.Fatalf("selection = %v, want [1 4]", got)
	}
}

// The selection is keyed by PR number precisely so a refresh that reorders a
// section cannot silently change what is selected.
func TestSelectionSurvivesRowsMovingOnRefresh(t *testing.T) {
	m := selectBoard(t)
	m.cursor = m.rowSlot(1) // PR 2
	m = pressKey(m, " ")

	m.board.Refetch()
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{
		{Number: 9, Title: "new arrival", URL: "https://x/9"},
		{Number: 1, Title: "first", URL: "https://x/1"},
		{Number: 2, Title: "second", URL: "https://x/2"},
	}})
	m.board.Apply(board.Result{Index: 1, PRs: []github.PR{{Number: 4, Title: "fourth"}}})

	if got := selectedNumbers(m); !equalInts(got, []int{2}) {
		t.Fatalf("selection = %v after reorder, want [2] -- selection must key on PR number", got)
	}
}

func TestSelectionDropsPRsThatLeftTheBoard(t *testing.T) {
	m := selectBoard(t)
	m = pressKey(m, " ") // PR 1
	m.board.Refetch()
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{Number: 2, Title: "second"}}})
	m.board.Apply(board.Result{Index: 1})
	if got := selectedNumbers(m); len(got) != 0 {
		t.Fatalf("selection = %v, want empty once the PR left the board", got)
	}
}

func TestSelectedRowDrawsItsMark(t *testing.T) {
	m := pressKey(selectBoard(t), " ")
	if !strings.Contains(stripANSI(m.View()), "•") {
		t.Fatal("expected the selection mark on the board:\n" + stripANSI(m.View()))
	}
}

// The design guide's "does it make a healthy board louder" check, as an assert.
func TestUnselectedBoardLooksExactlyAsBefore(t *testing.T) {
	plain := selectBoard(t).View()
	marked := pressKey(selectBoard(t), " ")
	cleared := pressKey(marked, " ") // toggled back off
	if cleared.View() != plain {
		t.Fatal("a board with nothing selected must render identically to before")
	}
}

func TestCursorAndSelectionAreSeparateMarks(t *testing.T) {
	m := pressKey(selectBoard(t), " ")
	row := stripANSI(m.View())
	if !strings.Contains(row, "▌•") {
		t.Fatal("expected cursor and selection marks together:\n" + row)
	}
	m.cursor = m.rowSlot(2)
	moved := stripANSI(m.View())
	if !strings.Contains(moved, "•") {
		t.Fatal("selection mark should survive the cursor moving away:\n" + moved)
	}
	if strings.Contains(moved, "▌•") {
		t.Fatal("cursor mark should have moved off the selected row:\n" + moved)
	}
}

func TestSelectionMarkSurvivesNoColor(t *testing.T) {
	m := pressKey(selectBoard(t), " ")
	if !strings.Contains(stripANSI(m.View()), "•") {
		t.Fatal("the mark must be a glyph, not a hue")
	}
}

func TestEscClearsSelectionBeforeSearch(t *testing.T) {
	m := selectBoard(t)
	m.query = "first"
	m = pressKey(m, " ")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if got := selectedNumbers(m); len(got) != 0 {
		t.Fatalf("selection = %v, want cleared first", got)
	}
	if m.query != "first" {
		t.Fatalf("query = %q, want it to survive the first esc", m.query)
	}
}

func TestEscThenClearsTheSearch(t *testing.T) {
	m := selectBoard(t)
	m.query = "first"
	m = pressKey(m, " ")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyEsc})
	if q := next.(Model).query; q != "" {
		t.Fatalf("query = %q, want cleared by the second esc", q)
	}
}

func TestEscThenQuits(t *testing.T) {
	m := selectBoard(t)
	m.query = "first"
	m = pressKey(m, " ")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyEsc})
	_, cmd := next.(Model).Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("third esc should quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("third esc should return tea.Quit")
	}
}

// The new rung must not cost a keypress to anyone who never selects anything.
func TestEscQuitsWhenNothingIsSelected(t *testing.T) {
	_, cmd := selectBoard(t).Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("esc on a clean board should quit on the first press")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("esc on a clean board should return tea.Quit")
	}
}

func TestRangeSelectsFromAnchorToCursor(t *testing.T) {
	m := pressKey(selectBoard(t), "v")
	m = pressKey(pressKey(m, "j"), "j")
	if got := selectedNumbers(m); !equalInts(got, []int{1, 2, 3}) {
		t.Fatalf("selection = %v, want [1 2 3]", got)
	}
}

// A range that only works downward is the obvious bug here.
func TestRangeExtendsBackwards(t *testing.T) {
	m := selectBoard(t)
	m.cursor = m.rowSlot(2) // PR 3
	m = pressKey(m, "v")
	m = pressKey(pressKey(m, "k"), "k")
	if got := selectedNumbers(m); !equalInts(got, []int{1, 2, 3}) {
		t.Fatalf("selection = %v, want [1 2 3]", got)
	}
}

// The range tracks the cursor; it does not accumulate.
func TestRangeShrinksWhenTheCursorComesBack(t *testing.T) {
	m := pressKey(selectBoard(t), "v")
	m = pressKey(pressKey(pressKey(m, "j"), "j"), "k")
	if got := selectedNumbers(m); !equalInts(got, []int{1, 2}) {
		t.Fatalf("selection = %v, want [1 2] -- the range must shrink, not trail", got)
	}
}

func TestRangeSkipsHeadersAndNotes(t *testing.T) {
	m := selectBoard(t)
	m.cursor = m.rowSlot(2) // last row of section one
	m = pressKey(m, "v")
	// j crosses the section-two header and lands on its first row.
	m = pressKey(pressKey(m, "j"), "j")
	got := selectedNumbers(m)
	if !equalInts(got, []int{3, 4}) {
		t.Fatalf("selection = %v, want [3 4] -- the header between them is not a PR", got)
	}
}

func TestRangeOverAHeaderOnlySelectsNothing(t *testing.T) {
	m := selectBoard(t)
	m.cursor = m.headerSlot(0)
	m = pressKey(m, "v")
	if got := selectedNumbers(m); len(got) != 0 {
		t.Fatalf("selection = %v, want empty", got)
	}
}

func TestVAgainEndsRangeAndKeepsSelection(t *testing.T) {
	m := pressKey(selectBoard(t), "v")
	m = pressKey(m, "j")
	m = pressKey(m, "v") // end the range
	if m.ranging {
		t.Fatal("v again should leave range mode")
	}
	if got := selectedNumbers(m); !equalInts(got, []int{1, 2}) {
		t.Fatalf("selection = %v, want [1 2] kept after leaving the mode", got)
	}
	// Moving now must not extend anything.
	m = pressKey(m, "j")
	if got := selectedNumbers(m); !equalInts(got, []int{1, 2}) {
		t.Fatalf("selection = %v after moving outside the mode, want [1 2]", got)
	}
}

func TestEscDuringRangeClearsIt(t *testing.T) {
	m := pressKey(selectBoard(t), "v")
	m = pressKey(m, "j")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if got := selectedNumbers(m); len(got) != 0 {
		t.Fatalf("selection = %v, want cleared", got)
	}
	if m.ranging {
		t.Fatal("esc should leave range mode")
	}
}

// The documented route to a scattered selection, so it has to actually work.
func TestRangeAddsToAnExistingSelection(t *testing.T) {
	m := selectBoard(t)
	m = pressKey(m, " ") // PR 1 on its own
	m.cursor = m.rowSlot(3)
	m = pressKey(m, "v") // range over PR 4..5
	m = pressKey(m, "j")
	if got := selectedNumbers(m); !equalInts(got, []int{1, 4, 5}) {
		t.Fatalf("selection = %v, want [1 4 5]", got)
	}
}

// Shrinking a range must release only what the range selected, never a mark the
// user made with space beforehand.
func TestShrinkingARangeKeepsEarlierSpaceMarks(t *testing.T) {
	m := selectBoard(t)
	m.cursor = m.rowSlot(1)
	m = pressKey(m, " ") // PR 2, by hand
	m.cursor = m.rowSlot(0)
	m = pressKey(m, "v")                // range from PR 1
	m = pressKey(pressKey(m, "j"), "j") // covers 1,2,3
	m = pressKey(pressKey(m, "k"), "k") // back to just 1
	if got := selectedNumbers(m); !equalInts(got, []int{1, 2}) {
		t.Fatalf("selection = %v, want [1 2] -- the hand-made mark on 2 must survive", got)
	}
}

func TestFooterShowsRangeKeysWhileRanging(t *testing.T) {
	m := pressKey(selectBoard(t), "v")
	if !strings.Contains(stripANSI(m.View()), "esc clear") {
		t.Fatal("footer should say how to leave range mode:\n" + stripANSI(m.View()))
	}
}

func TestFooterShowsTheSelectionCount(t *testing.T) {
	m := pressKey(selectBoard(t), "v")
	m = pressKey(pressKey(m, "j"), "j")
	if !strings.Contains(stripANSI(m.View()), "y copy 3") {
		t.Fatal("footer should show the count:\n" + stripANSI(m.View()))
	}
}

func TestFooterReturnsToNormalWhenSelectionCleared(t *testing.T) {
	m := pressKey(selectBoard(t), " ")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !strings.Contains(stripANSI(next.(Model).View()), "/ search") {
		t.Fatal("footer should return to the default legend")
	}
}

// copyCapture swaps the clipboard shell-out for the duration of a test and
// returns what would have been written.
func copyCapture(t *testing.T, fail error) *string {
	t.Helper()
	var got string
	prev := copyToClipboard
	copyToClipboard = func(s string) error {
		got = s
		return fail
	}
	t.Cleanup(func() { copyToClipboard = prev })
	return &got
}

func TestCopyWithNothingSelectedCopiesTheCursorRow(t *testing.T) {
	got := copyCapture(t, nil)
	m := drain(t, selectBoard(t), nil)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = drain(t, next.(Model), cmd)
	if *got != "https://x/1" {
		t.Fatalf("copied %q, want the cursor row's url", *got)
	}
	if m.status != "copied #1 url" {
		t.Fatalf("status = %q, want the single-PR wording unchanged", m.status)
	}
}

func TestCopyCopiesEverySelectedURL(t *testing.T) {
	got := copyCapture(t, nil)
	m := pressKey(selectBoard(t), "v")
	m = pressKey(pressKey(m, "j"), "j")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	drain(t, next.(Model), cmd)
	for _, want := range []string{"https://x/1", "https://x/2", "https://x/3"} {
		if !strings.Contains(*got, want) {
			t.Fatalf("copied %q, missing %s", *got, want)
		}
	}
}

// Decided: one url per line.
func TestCopyJoinsWithNewlines(t *testing.T) {
	got := copyCapture(t, nil)
	m := pressKey(selectBoard(t), "v")
	m = pressKey(m, "j")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	drain(t, next.(Model), cmd)
	if *got != "https://x/1\nhttps://x/2" {
		t.Fatalf("copied %q, want newline-joined with no trailing newline", *got)
	}
}

// A Go map has no iteration order, so without this the url order is random.
func TestCopyUsesBoardOrderNotSelectionOrder(t *testing.T) {
	got := copyCapture(t, nil)
	m := selectBoard(t)
	m.cursor = m.rowSlot(2) // select bottom-up
	m = pressKey(m, " ")
	m.cursor = m.rowSlot(0)
	m = pressKey(m, " ")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	drain(t, next.(Model), cmd)
	if *got != "https://x/1\nhttps://x/3" {
		t.Fatalf("copied %q, want board order regardless of selection order", *got)
	}
}

func TestCopyReportsTheCount(t *testing.T) {
	copyCapture(t, nil)
	m := pressKey(selectBoard(t), "v")
	m = pressKey(pressKey(m, "j"), "j")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = drain(t, next.(Model), cmd)
	if m.status != "copied 3 urls" {
		t.Fatalf("status = %q, want \"copied 3 urls\"", m.status)
	}
}

func TestCopySkipsSelectedPRsWithNoURL(t *testing.T) {
	got := copyCapture(t, nil)
	m := New(testCfg(), nil)
	m.width, m.height = 120, 20
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{
		{Number: 1, Title: "a", URL: "https://x/1"},
		{Number: 2, Title: "b"}, // no url
		{Number: 3, Title: "c", URL: "https://x/3"},
	}})
	m.board.Apply(board.Result{Index: 1})
	m.fetching = false
	m.cursor = m.firstRowSlot()
	m = pressKey(m, "v")
	m = pressKey(pressKey(m, "j"), "j")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = drain(t, next.(Model), cmd)
	if *got != "https://x/1\nhttps://x/3" {
		t.Fatalf("copied %q, want the two real urls only", *got)
	}
	if m.status != "copied 2 urls" {
		t.Fatalf("status = %q, want the count to match what was copied", m.status)
	}
}

func TestCopyClearsTheSelection(t *testing.T) {
	copyCapture(t, nil)
	m := pressKey(selectBoard(t), "v")
	m = pressKey(m, "j")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = drain(t, next.(Model), cmd)
	if got := selectedNumbers(m); len(got) != 0 {
		t.Fatalf("selection = %v, want cleared after a successful copy", got)
	}
}

// Failure is the one case where the user has to retry, so what they picked has
// to still be there.
func TestCopyFailureKeepsTheSelection(t *testing.T) {
	copyCapture(t, errors.New("pbcopy exploded"))
	m := pressKey(selectBoard(t), "v")
	m = pressKey(m, "j")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = drain(t, next.(Model), cmd)
	if !strings.Contains(m.status, "copy failed") {
		t.Fatalf("status = %q, want the failure reported", m.status)
	}
	if got := selectedNumbers(m); !equalInts(got, []int{1, 2}) {
		t.Fatalf("selection = %v, want [1 2] kept so the user can retry", got)
	}
}

// End-to-end on a full board: the thing the feature was asked for.
func TestSelectAllRowsThenCopy(t *testing.T) {
	got := copyCapture(t, nil)
	m := selectBoard(t)
	m.cursor = 0
	m = pressKey(m, "v")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	m = next.(Model)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = drain(t, next.(Model), cmd)
	want := "https://x/1\nhttps://x/2\nhttps://x/3\nhttps://x/4\nhttps://x/5"
	if *got != want {
		t.Fatalf("copied:\n%q\nwant:\n%q", *got, want)
	}
}
