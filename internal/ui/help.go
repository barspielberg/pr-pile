package ui

import (
	"fmt"
	"strings"

	"github.com/barspielberg/prs-mng/internal/config"
)

// The glyph key is rendered from the same helpers the rows use, so a legend can
// never drift from what is actually on screen.
func (m Model) helpOverlay() string {
	line := func(label, body string) string {
		return "  " + mutedStyle.Render(pad(label, 10)) + body
	}

	var b strings.Builder
	b.WriteString(headerStyle.Render("  KEYS") + "\n")
	for _, k := range [][2]string{
		{"j / k", "move"},
		{"l / h", "next / previous section"},
		{"g / G", "top / bottom"},
		{"enter", "open in browser"},
		{"c", "failing checks for this PR"},
		{"/", "filter"},
		{"r", "reload"},
		{"?", "close this"},
		{"q", "quit"},
	} {
		b.WriteString(line(k[0], mutedStyle.Render(k[1])) + "\n")
	}
	for _, a := range m.cfg.Actions {
		if a.Run != "" {
			b.WriteString(line(a.Key, mutedStyle.Render(a.Name)) + "\n")
		}
	}

	b.WriteString("\n" + headerStyle.Render("  FILTER") + "\n")
	for _, k := range [][2]string{
		{"ctrl+n/p", "move within matches"},
		{"esc", "clear the filter"},
		{"123", "a bare number matches PR numbers"},
		{"text", "fuzzy over author and title"},
	} {
		b.WriteString(line(k[0], mutedStyle.Render(k[1])) + "\n")
	}

	b.WriteString("\n" + headerStyle.Render("  CI") + "\n")
	b.WriteString(line(okStyle.Render("✓"), mutedStyle.Render("passing")) + "\n")
	b.WriteString(line(errorStyle.Render("✗2"), mutedStyle.Render("2 checks failing")) + "\n")
	b.WriteString(line(attentionStyle.Render("◐"), mutedStyle.Render("running")) + "\n")
	b.WriteString(line(mutedStyle.Render("·"), mutedStyle.Render("no checks")) + "\n")

	b.WriteString("\n" + headerStyle.Render("  REVIEW") + "\n")
	b.WriteString(line(okStyle.Render("✓"), mutedStyle.Render("approved")) + "\n")
	b.WriteString(line(errorStyle.Render("✗"), mutedStyle.Render("changes requested")) + "\n")
	b.WriteString(line(attentionStyle.Render("○"), mutedStyle.Render("review required")) + "\n")

	b.WriteString("\n" + headerStyle.Render("  BLOCKERS") + "\n")
	b.WriteString(line(errorStyle.Render("!"), mutedStyle.Render("merge conflicts")) + "\n")
	b.WriteString(line(mutedStyle.Render("~"), mutedStyle.Render("draft")) + "\n")

	b.WriteString("\n" + headerStyle.Render("  ROWS") + "\n")
	b.WriteString(line(mutedStyle.Render("╭╴│╰╴"), mutedStyle.Render("a stack: each PR targets the one above")) + "\n")
	b.WriteString(line("abc", mutedStyle.Render("author initials, on rules with author: true")) + "\n")
	b.WriteString(line("2h", mutedStyle.Render("last updated")) + "\n")

	b.WriteString("\n" + mutedStyle.Render(fmt.Sprintf("  config: %s", config.Path())) + "\n")
	return b.String()
}

// checksOverlay lists the selected PR's failing gates in full. The row itself
// only carries the count (✗6): names are what you want after deciding a PR is
// worth investigating, not while scanning, and giving them a line in the list
// is what made the board scroll unevenly. See docs/uniform-rows.md.
//
// It closes on the next movement key, so it reads as a look rather than a mode.
func (m Model) checksOverlay() string {
	pr, ok := m.selected()
	if !ok {
		return ""
	}

	var b strings.Builder
	b.WriteString(headerStyle.Render(fmt.Sprintf("  #%d", pr.Number)) + " " +
		mutedStyle.Render(clip(pr.Title, max(0, m.width-12))) + "\n\n")

	if len(pr.FailedGates) == 0 {
		b.WriteString("  " + mutedStyle.Render("no failing checks") + "\n")
	} else {
		// Unclipped and one per line: this overlay exists to show the whole
		// list, which the old single-line version could not.
		for _, g := range pr.FailedGates {
			b.WriteString("  " + errorStyle.Render("✗") + " " + clip(g, max(0, m.width-4)) + "\n")
		}
	}

	b.WriteString("\n" + mutedStyle.Render("  any key closes") + "\n")
	return b.String()
}
