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
