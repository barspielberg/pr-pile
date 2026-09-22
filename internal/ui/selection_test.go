package ui

import (
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
