package ui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func confirmBoard(t *testing.T) Model {
	t.Helper()
	openCapture(t, nil)
	m := pressKey(selectBoard(t), "v")
	return pressEnter(t, pressKey(pressKey(m, "j"), "j"))
}

func TestConfirmDrawsInTheMiddleOfTheBoard(t *testing.T) {
	m := confirmBoard(t)
	lines := strings.Split(m.View(), "\n")
	if len(lines) != m.height {
		t.Fatalf("frame is %d lines, want %d", len(lines), m.height)
	}
	at := -1
	for i, l := range lines {
		if at < 0 && strings.Contains(ansi.Strip(l), "open 3 PRs in the browser?") {
			at = i
		}
		if w := lipgloss.Width(l); w > m.width {
			t.Fatalf("line %d is %d wide, over the pane's %d", i, w, m.width)
		}
	}
	if at <= 0 || at >= m.height-2 {
		t.Fatalf("question on line %d, want it in a box mid-board:\n%s", at, ansi.Strip(m.View()))
	}
	plain := ansi.Strip(m.View())
	for _, want := range []string{"#1  first", "#3  third", "y / enter open · any other key cancels"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("frame missing %q:\n%s", want, plain)
		}
	}
}

// Board text running straight up to the box's border read as part of the box.
func TestConfirmKeepsAGapBetweenTheBoxAndTheBoard(t *testing.T) {
	m := confirmBoard(t)
	for _, l := range strings.Split(ansi.Strip(m.View()), "\n") {
		i := strings.IndexAny(l, "│╭╰")
		if i < 0 {
			continue
		}
		if i == 0 || l[i-1] != ' ' {
			t.Fatalf("no gap left of the box:\n%q", l)
		}
	}
}

func TestConfirmNamesTheActionInItsHint(t *testing.T) {
	m := confirmBoard(t)
	m.confirmVerb = "worktree"
	plain := ansi.Strip(m.View())
	for _, want := range []string{"worktree on 3 PRs?", "y / enter worktree · any other key cancels"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("frame missing %q:\n%s", want, plain)
		}
	}
}

func TestConfirmCountsPRsPastTheListLimit(t *testing.T) {
	m := pressKey(selectBoard(t), "v")
	m = pressKey(m, "G")
	m = pressEnter(t, m)
	m.confirmOpen = append(m.confirmOpen, m.confirmOpen...)
	plain := ansi.Strip(m.View())
	if !strings.Contains(plain, "… and 6 more") {
		t.Fatalf("want the overflow counted:\n%s", plain)
	}
}

// The footer asks the question at every height, so a pane too short for the
// box still says what y would do.
func TestConfirmFitsAShortPane(t *testing.T) {
	for h := 2; h <= 8; h++ {
		m := confirmBoard(t)
		m.height = h
		lines := strings.Split(m.View(), "\n")
		if len(lines) != h {
			t.Fatalf("height %d: frame is %d lines", h, len(lines))
		}
		if !strings.Contains(ansi.Strip(lines[h-1]), "open 3 PRs in the browser?") {
			t.Fatalf("height %d: question not on the footer:\n%s", h, ansi.Strip(m.View()))
		}
	}
}

// A hint wider than the box used to push the bottom border past the pane.
func TestDialogsFitANarrowPane(t *testing.T) {
	copyMenu := pressKey(copyMenuBoard(t), "Y")
	for _, m := range []Model{confirmBoard(t), copyMenu} {
		m.width = 40
		for i, l := range strings.Split(m.View(), "\n") {
			if w := lipgloss.Width(l); w > m.width {
				t.Fatalf("line %d is %d wide, over the pane's %d:\n%s", i, w, m.width, ansi.Strip(m.View()))
			}
		}
	}
}

func TestConfirmWithNoHeightDrawsOnlyTheBox(t *testing.T) {
	m := confirmBoard(t)
	m.height = 0
	m.confirmOpen = append(append(m.confirmOpen, m.confirmOpen...), m.confirmOpen...)
	lines := m.overlayConfirm(nil)
	if len(lines) != confirmListed+4 {
		t.Fatalf("drew %d lines, want the box's %d", len(lines), confirmListed+4)
	}
}
