package ui

import (
	"fmt"
	"github.com/barspielberg/pr-pile/internal/board"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"strconv"
	"strings"
)

func (m Model) dialogWidth() int { return min(72, m.width-4) }

func dialogTop(title string, inner int) string {
	fill := max(0, inner-1-lipgloss.Width(title))
	return mutedStyle.Render("╭─") + headerStyle.Render(title) +
		mutedStyle.Render(strings.Repeat("─", fill)+"╮")
}

func dialogBottom(hint string, inner int) string {
	fill := max(0, inner-1-lipgloss.Width(hint))
	return mutedStyle.Render("╰" + strings.Repeat("─", fill) + hint + "─╯")
}

func dialogRow(text string, inner int, st lipgloss.Style) string {
	return mutedStyle.Render("│") + st.Render(pad(text, inner)) + mutedStyle.Render("│")
}

// overlayDialog lays a box over the middle of the board. The board stays
// visible around it, so the box reads as a question about the board rather
// than a page of its own, but it is redrawn faint and a blank cell is kept on
// each side of the box: at full strength, board text running up to the box's
// faint border read as part of the box.
//
// box is handed the rows it may use and must not return more, so the frame
// never grows past the pane and pushes the footer off. natural is the box's
// full height, drawn whole when the pane's height is not known yet.
func (m Model) overlayDialog(lines []string, natural int, box func(rows int) []string) []string {
	rows := len(lines)
	if m.height <= 0 {
		rows = natural
		for len(lines) < rows {
			lines = append(lines, "")
		}
	}
	b := box(rows)
	if len(b) == 0 {
		return lines
	}
	top := (len(lines) - len(b)) / 2
	boxW := lipgloss.Width(b[0])
	left := max(1, (m.width-boxW)/2)
	out := make([]string, len(lines))
	for i, l := range lines {
		plain := ansi.Strip(l)
		if i < top || i >= top+len(b) {
			out[i] = mutedStyle.Render(plain)
			continue
		}
		before := pad(ansi.Truncate(plain, left-1, ""), left-1)
		after := ansi.TruncateLeft(plain, left+boxW+1, "")
		out[i] = mutedStyle.Render(before) + " " + b[i-top] + " " + mutedStyle.Render(after)
	}
	return out
}

// confirmListed is how many PRs the confirm names before it counts the rest.
const confirmListed = 5

func (m Model) confirmVerbOrOpen() string {
	if m.confirmVerb != "" {
		return m.confirmVerb
	}
	return "open"
}

func (m Model) confirmQuestion() string {
	if m.confirmVerb != "" {
		return fmt.Sprintf("%s on %d PRs?", m.confirmVerb, len(m.confirmOpen))
	}
	return fmt.Sprintf("open %d PRs in the browser?", len(m.confirmOpen))
}

// confirmLines lists the PRs the question is about, drawn the way the board
// draws them so each line can be matched to a marked row at a glance.
func (m Model) confirmLines(inner int) []string {
	prs := m.confirmOpen
	shown := prs
	if len(prs) > confirmListed {
		// One row goes to the count, so a list one over the limit still fits.
		shown = prs[:confirmListed-1]
	}
	numW := 0
	for _, pr := range shown {
		numW = max(numW, len(strconv.Itoa(pr.Number))+1)
	}
	lead := 2 + numW + 2
	tw := max(0, inner-lead-1)
	plain := func(st lipgloss.Style) lipgloss.Style { return st }
	lines := make([]string, 0, len(shown)+1)
	for _, pr := range shown {
		lines = append(lines, "  "+accentStyle.Render(pad("#"+strconv.Itoa(pr.Number), numW))+"  "+
			m.renderTitle(board.Row{PR: pr}, fgStyle, plain, tw, nil)+" ")
	}
	if len(shown) < len(prs) {
		lines = append(lines, mutedStyle.Render(pad(fmt.Sprintf("  … and %d more", len(prs)-len(shown)), inner)))
	}
	return lines
}

// confirmBox draws the do-this-to-this-many question with the PRs it is about,
// at most rows tall. The question has a row of its own rather than a place in
// the border: here it is the content, not a label. A short pane loses the list
// from the bottom, and below three rows there is no box at all -- the footer
// carries the question at every height.
func (m Model) confirmBox(rows int) []string {
	if rows < 3 {
		return nil
	}
	inner := m.dialogWidth() - 2
	body := append([]string{
		fgStyle.Bold(true).Render(pad(clip("  "+m.confirmQuestion(), inner), inner)),
		strings.Repeat(" ", inner),
	}, m.confirmLines(inner)...)
	body = body[:min(len(body), rows-2)]
	out := []string{dialogTop("", inner)}
	for _, l := range body {
		out = append(out, mutedStyle.Render("│")+l+mutedStyle.Render("│"))
	}
	hint := " y / enter " + m.confirmVerbOrOpen() + " · any other key cancels "
	return append(out, dialogBottom(clip(hint, inner-1), inner))
}

func (m Model) overlayConfirm(lines []string) []string {
	return m.overlayDialog(lines, min(len(m.confirmOpen), confirmListed)+4, m.confirmBox)
}
