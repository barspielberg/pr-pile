package ui

import (
	"errors"
	"github.com/barspielberg/pr-pile/internal/board"
	"github.com/barspielberg/pr-pile/internal/github"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func copyMenuBoard(t *testing.T) Model {
	t.Helper()
	m := New(testCfg(), nil)
	m.width, m.height = 120, 20
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{
		{Repo: testRepo, Number: 1, Title: "first", URL: "https://x/1", HeadRefName: "feat/one", Author: "ann"},
		{Repo: testRepo, Number: 2, Title: "second", URL: "https://x/2", HeadRefName: "feat/two", Author: "bob"},
		{Repo: testRepo, Number: 3, Title: "third"},
	}})
	m.board.Apply(board.Result{Index: 1})
	m.fetching = false
	m.cursor = m.firstRowSlot()
	return m
}

// pressDrain presses a key and runs whatever it returns, so an async copy has
// landed by the time the test looks.
func pressDrain(t *testing.T, m Model, key string) Model {
	t.Helper()
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	return drain(t, next.(Model), cmd)
}

func TestCopyMenuFieldKeysCopyTheCursorRow(t *testing.T) {
	for _, tc := range []struct{ key, want, status string }{
		{"n", "#1", "copied #1 number"},
		{"t", "first", "copied #1 title"},
		{"u", "https://x/1", "copied #1 url"},
		{"b", "feat/one", "copied #1 branch"},
		{"a", "@ann", "copied #1 author"},
		{"m", "[#1 first](https://x/1)", "copied #1 markdown"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			got := copyCapture(t, nil)
			m := pressDrain(t, pressKey(copyMenuBoard(t), "Y"), tc.key)
			if *got != tc.want {
				t.Fatalf("copied %q, want %q", *got, tc.want)
			}
			if m.status != tc.status {
				t.Fatalf("status = %q, want %q", m.status, tc.status)
			}
			if m.copyMenu != nil {
				t.Fatal("menu still open after a copy")
			}
		})
	}
}

func TestCopyMenuEnterCopiesTheHighlightedField(t *testing.T) {
	got := copyCapture(t, nil)
	m := pressKey(copyMenuBoard(t), "Y")
	m = pressKey(pressKey(m, "j"), "j")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	drain(t, next.(Model), cmd)
	if *got != "https://x/1" {
		t.Fatalf("copied %q, want the third field, the url", *got)
	}
}

func TestCopyMenuCopiesEverySelectedPROnePerLine(t *testing.T) {
	got := copyCapture(t, nil)
	m := pressKey(copyMenuBoard(t), "v")
	m = pressKey(pressKey(m, "j"), "j")
	m = pressDrain(t, pressKey(m, "Y"), "b")
	// #3 has no branch, so it adds no line and is not counted.
	if *got != "feat/one\nfeat/two" {
		t.Fatalf("copied %q, want both branches one per line", *got)
	}
	if m.status != "copied 2 branches" {
		t.Fatalf("status = %q, want the count of what was copied", m.status)
	}
	if got := selectedNumbers(m); len(got) != 0 {
		t.Fatalf("selection = %v, want cleared after a copy", got)
	}
}

func TestCopyMenuOtherKeysCloseWithoutCopying(t *testing.T) {
	got := copyCapture(t, nil)
	m := pressKey(pressKey(copyMenuBoard(t), " "), "Y")
	m = pressDrain(t, m, "x")
	if *got != "" {
		t.Fatalf("copied %q, want nothing", *got)
	}
	if m.copyMenu != nil {
		t.Fatal("menu still open")
	}
	if got := selectedNumbers(m); !equalInts(got, []int{1}) {
		t.Fatalf("selection = %v, want it kept after a cancel", got)
	}
}

func TestCopyMenuFailedCopyRestoresTheSelection(t *testing.T) {
	copyCapture(t, errors.New("no pbcopy"))
	m := pressKey(pressKey(copyMenuBoard(t), " "), "Y")
	m = pressDrain(t, m, "t")
	if !strings.HasPrefix(m.status, "copy failed") {
		t.Fatalf("status = %q, want the failure", m.status)
	}
	if got := selectedNumbers(m); !equalInts(got, []int{1}) {
		t.Fatalf("selection = %v, want it back after a failure", got)
	}
}

func TestCopyMenuFieldWithNoValueSaysSo(t *testing.T) {
	got := copyCapture(t, nil)
	m := copyMenuBoard(t)
	m.cursor = m.rowSlot(2)
	m = pressDrain(t, pressKey(m, "Y"), "b")
	if *got != "" {
		t.Fatalf("copied %q, want nothing", *got)
	}
	if m.status != "no branch for this PR" {
		t.Fatalf("status = %q", m.status)
	}
}

func TestCopyMenuOnAHeaderDoesNotOpen(t *testing.T) {
	m := copyMenuBoard(t)
	m.cursor = m.headerSlot(0)
	m = pressKey(m, "Y")
	if m.copyMenu != nil {
		t.Fatal("menu opened with no PR to copy from")
	}
}

// A timed refresh that closed the menu would send the next letter to the
// board, where `a` or `m` may be a configured action.
func TestTheCopyMenuSurvivesARefresh(t *testing.T) {
	got := copyCapture(t, nil)
	m := pressKey(copyMenuBoard(t), "Y")
	next, _ := m.refresh()
	m = pressDrain(t, next.(Model), "a")
	if *got != "@ann" {
		t.Fatalf("copied %q, want the menu to still own the key", *got)
	}
}

func TestCopyStatusNamesThePRTheValueCameFrom(t *testing.T) {
	copyCapture(t, nil)
	m := copyMenuBoard(t)
	m.cursor = m.rowSlot(1)
	m = pressKey(pressKey(m, "v"), "j") // #2 and #3; only #2 has a branch
	m = pressDrain(t, pressKey(m, "Y"), "b")
	if m.status != "copied #2 branch" {
		t.Fatalf("status = %q, want the PR whose branch was copied", m.status)
	}
}

func TestCopyMenuFitsAShortPane(t *testing.T) {
	for h := 2; h <= 10; h++ {
		m := copyMenuBoard(t)
		m.height = h
		m = pressKey(m, "Y")
		for range len(copyFields) - 1 {
			m = pressKey(m, "j")
		}
		frame := m.View()
		if n := len(strings.Split(frame, "\n")); n != h {
			t.Fatalf("height %d: frame is %d lines", h, n)
		}
		// The cursor sits on the last field, so the window has to follow it.
		if h > 2 && !strings.Contains(ansi.Strip(frame), "markdown") {
			t.Fatalf("height %d: highlighted field not on screen:\n%s", h, ansi.Strip(frame))
		}
	}
}

func TestCopyMenuDrawsOverTheBoardAtFullHeight(t *testing.T) {
	m := pressKey(pressKey(copyMenuBoard(t), "v"), "j")
	m = pressKey(m, "Y")
	frame := m.View()
	lines := strings.Split(frame, "\n")
	if len(lines) != m.height {
		t.Fatalf("frame is %d lines, want %d", len(lines), m.height)
	}
	plain := ansi.Strip(frame)
	for _, want := range []string{"copy 2 PRs", "feat/one (+1 more)", "@ann (+1 more)"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("frame missing %q:\n%s", want, plain)
		}
	}
	for i, l := range lines {
		if w := lipgloss.Width(l); w > m.width {
			t.Fatalf("line %d is %d wide, over the pane's %d", i, w, m.width)
		}
	}
}
