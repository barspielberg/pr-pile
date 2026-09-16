package ui

import (
	"fmt"
	"strings"

	"github.com/barspielberg/prs-mng/internal/board"
	"github.com/barspielberg/prs-mng/internal/github"
	"github.com/charmbracelet/lipgloss"
)

var (
	headerStyle  = lipgloss.NewStyle().Bold(true)
	dimStyle     = lipgloss.NewStyle().Faint(true)
	sectionStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	numStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	treeStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	selStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	greenStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	redStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	yellowStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
)

// Glyphs need a Nerd Font, same as the script this replaces.
func ciCell(pr github.PR) (string, lipgloss.Style) {
	switch pr.CIState {
	case "SUCCESS":
		return " passing", greenStyle
	case "FAILURE", "ERROR":
		if n := len(pr.FailedGates); n > 0 {
			return fmt.Sprintf(" %d failing", n), redStyle
		}
		return " failing", redStyle
	case "PENDING", "EXPECTED":
		return " running", yellowStyle
	default:
		return " none", dimStyle
	}
}

func reviewCell(pr github.PR) string {
	if pr.IsDraft {
		return " draft"
	}
	switch pr.Review {
	case "APPROVED":
		return " approved"
	case "CHANGES_REQUESTED":
		return " changes req"
	case "REVIEW_REQUIRED":
		return " review"
	default:
		return " -"
	}
}

func mergeCell(pr github.PR) string {
	if pr.Mergeable == "CONFLICTING" {
		return " conflicts"
	}
	return ""
}

// clip truncates on display width, not bytes, so a title with wide glyphs
// still fits the row instead of wrapping.
func clip(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
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

func pad(s string, w int) string {
	if d := w - lipgloss.Width(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}

func (m Model) renderRow(r board.Row, selected bool) string {
	ci, ciColor := ciCell(r.PR)
	merge := mergeCell(r.PR)

	// Fixed columns: cursor(2) + tree(2) + number(6) + ci(11) + review(13)
	// + gutters, with the title taking whatever is left.
	used := 2 + 2 + 6 + 11 + 13 + 4
	if merge != "" {
		used += 11
	}
	titleWidth := m.width - used
	if titleWidth < 12 {
		titleWidth = 12
	}

	cursor := "  "
	if selected {
		cursor = selStyle.Render("▸ ")
	}
	title := clip(r.PR.Title, titleWidth)
	if selected {
		title = selStyle.Render(title)
	}

	var b strings.Builder
	b.WriteString(cursor)
	b.WriteString(treeStyle.Render(pad(r.Prefix, 2)))
	b.WriteString(numStyle.Render(pad("#"+fmt.Sprint(r.PR.Number), 6)))
	b.WriteString(" ")
	b.WriteString(ciColor.Render(pad(ci, 11)))
	b.WriteString(" ")
	b.WriteString(dimStyle.Render(pad(reviewCell(r.PR), 13)))
	if merge != "" {
		b.WriteString(yellowStyle.Render(pad(merge, 11)))
	}
	b.WriteString(" ")
	b.WriteString(title)

	// Failing gate names go on their own line: too long for a column, and
	// only present on the few rows that are actually red.
	if len(r.PR.FailedGates) > 0 {
		cont := "  "
		if r.Prefix == "╭╴" || r.Prefix == "│ " {
			cont = "│ "
		}
		gates := clip(strings.Join(r.PR.FailedGates, ", "), titleWidth+20)
		b.WriteString("\n    " + treeStyle.Render(cont) + "   " + redStyle.Render("└ "+gates))
	}
	return b.String()
}
