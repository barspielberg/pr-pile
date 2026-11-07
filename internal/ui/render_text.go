package ui

import (
	"fmt"
	"github.com/barspielberg/pr-pile/internal/github"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"time"
	"unicode"
)

// ciCell returns the 2-cell CI glyph plus count. Five distinct silhouettes so
// the state survives losing colour.
func ciCell(pr github.PR) (string, lipgloss.Style) {
	switch pr.CIState {
	case "SUCCESS":
		return "✓ ", okStyle
	case "FAILURE", "ERROR":
		n := len(pr.FailedGates)
		switch {
		case n == 0:
			return "✗ ", errorStyle
		case n >= 10:
			return "✗+", errorStyle
		default:
			return fmt.Sprintf("✗%d", n), errorStyle
		}
	case "PENDING", "EXPECTED":
		return "◐ ", attentionStyle
	case "CANCELLED":
		return "⊘ ", mutedStyle
	default:
		return "· ", mutedStyle
	}
}

func reviewCell(pr github.PR) (string, lipgloss.Style) {
	switch pr.Review {
	case "APPROVED":
		return "✓", okStyle
	case "CHANGES_REQUESTED":
		return "✗", errorStyle
	case "REVIEW_REQUIRED":
		return "○", attentionStyle
	default:
		return "·", mutedStyle
	}
}

// One cell for what stops a merge: a conflicted draft is primarily conflicted,
// since the conflict is the thing that will bite, and a draft cannot merge
// whether or not it is behind. ↓ is the cell's one non-ASCII glyph, no riskier
// than the ◐ and ○ already beside it.
func blockerCell(pr github.PR) (string, lipgloss.Style) {
	switch {
	case pr.Mergeable == "CONFLICTING":
		return "!", errorStyle
	case pr.IsDraft:
		return "~", mutedStyle
	case pr.MergeState == "BEHIND" && waitsOnlyOnUpdate(pr):
		return "↓", attentionStyle
	case pr.MergeState == "BEHIND":
		return "↓", mutedStyle
	default:
		return " ", fgStyle
	}
}

// waitsOnlyOnUpdate is whether being behind is the last thing left. Only then
// does ↓ ask for attention: on a PR still waiting for review or fixes, updating
// the branch now just means updating it again later.
func waitsOnlyOnUpdate(pr github.PR) bool {
	switch {
	case pr.Review == "REVIEW_REQUIRED", pr.Review == "CHANGES_REQUESTED":
		return false
	case pr.CIState == "FAILURE", pr.CIState == "ERROR":
		return false
	}
	return true
}

// numberWidth fits "#99999" and is named because the search has to reproduce
// the cell exactly to attribute a match back to it.
const numberWidth = 6

// authorWidth is deliberately narrow: lazygit, tig and neomutt all collapse to
// initials in their dense views rather than showing a name, and the point of
// this column is to be readable without being loud.
//
// The column is muted, like age. It carried a 15-entry hue palette so the same
// person was the same colour on every row; that was a grouping hint costing 15
// fixed cube values that overrode the user's theme, to decorate a fact the
// initials already carried. Colour is a budget (DESIGN-GUIDE.md §1) and who
// wrote a PR does not compete with whether it is broken.
const authorWidth = 3

// initials shortens a GitHub login to fit authorWidth. Collisions are possible
// and tolerable -- the column answers "is this mine or someone else's".
//
// These three characters are also the whole of what an author search matches,
// since they are the whole of what is drawn: two logins sharing a prefix share
// a cell and so share a match, which the row shows rather than hides.
func initials(login string) string {
	login = terminalText(login)
	if login == "" {
		return ""
	}
	r := []rune(strings.ToLower(login))
	if len(r) > authorWidth {
		r = r[:authorWidth]
	}
	return string(r)
}

// age renders the largest single unit, right-aligned so digits line up. The
// column is budgeted at 3 cells, so a week count that needs more than two
// digits saturates rather than widening the row and pushing it past the frame.
func age(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return "now"
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	default:
		if w := int(d.Hours() / (24 * 7)); w < 100 {
			return fmt.Sprintf("%dw", w)
		}
		return "99+"
	}
}

func clip(s string, w int) string {
	s = terminalText(s)
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	if w <= 3 {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		if lipgloss.Width(b.String()+string(r)) > w-1 {
			break
		}
		b.WriteRune(r)
	}
	return b.String() + "…"
}

// clipLeft drops from the front, keeping the tail visible. The query grows at
// its end, so that is the end worth keeping on screen.
func clipLeft(s string, w int) string {
	s = terminalText(s)
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	r := []rune(s)
	for i := range r {
		if lipgloss.Width(string(r[i:])) <= w {
			return string(r[i:])
		}
	}
	return ""
}

// terminalText makes remote text safe to draw. Escape sequences go first and
// whole: neutralising just the ESC rune would leave the payload behind, so a
// title carrying "\x1b[31m" would render as a literal "[31m" and spend the
// column's width on it. What survives is real text, and any control rune left
// in it becomes visible rather than reaching the terminal.
func terminalText(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '\uFFFD'
		}
		return r
	}, ansi.Strip(s))
}

func pad(s string, w int) string {
	if d := w - lipgloss.Width(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}

func padLeft(s string, w int) string {
	if d := w - lipgloss.Width(s); d > 0 {
		return strings.Repeat(" ", d) + s
	}
	return s
}

// renderRow draws one PR. Every sub-slot is always emitted, blank when absent,
// so no field ever shifts as state changes.
