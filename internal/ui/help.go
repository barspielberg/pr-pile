package ui

import (
	"fmt"
	"strings"

	"github.com/barspielberg/prs-mng/internal/config"
	"github.com/barspielberg/prs-mng/internal/github"
)

// pr-detail.go carries the state block that sits under the check list.

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
		{"c", "detail for this PR"},
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

// detailOverlay answers "what do I do about this PR" for the selected row. It
// began as the checks page and kept the key: the checks block is still the top
// of it, so `c` still means what it did.
//
// The page is two blocks. The checks block names what is failing or running and
// counts what passed -- 47% of contexts on this board are SKIPPED and 45%
// SUCCESS, so listing everything would bury the signal. The state block below
// it says what is true about the PR itself: conflicted and how far behind,
// unresolved conversations, who it is from, how big, what it targets.
//
// Two of those lines arrive on a second request and are simply absent until it
// lands (docs/pr-detail.md §7). The page never waits for the network.
//
// It closes on the next movement key, so it reads as a look rather than a mode.
func (m Model) detailOverlay() string {
	pr, ok := m.selected()
	if !ok {
		return ""
	}

	lines := []string{
		headerStyle.Render(fmt.Sprintf("  #%d", pr.Number)) + " " +
			mutedStyle.Render(clip(pr.Title, max(0, m.width-12))),
		"",
	}
	lines = append(lines, m.overlayBody(pr)...)
	lines = append(lines, "", mutedStyle.Render("  any key closes"))

	// No trailing newline: a pane is as many lines as it is tall, and one more
	// scrolls the header off the top edge -- which at 14 rows is exactly where
	// the page is already spending every line it has.
	return strings.Join(lines, "\n")
}

// overlayBody assembles both blocks and enforces the degradation order: the
// state block is clipped from its own bottom, entirely, before the checks block
// gives up a single line. A PR with 8 failing checks must not drop a failure to
// make room for its branch name -- that inverts what the page is for.
// See docs/pr-detail.md §6.2.
func (m Model) overlayBody(pr github.PR) []string {
	checks := m.checkLines(pr)
	state := m.stateLines(pr)
	if len(state) == 0 {
		return checks
	}

	// The checks block is clipped only once the state block is entirely gone,
	// so its budget is whatever is left after the state block has shrunk to
	// nothing -- which is the full body budget.
	body := m.bodyBudget()
	if body <= 0 {
		return append(checks, state...)
	}

	// +1 for the blank line between the blocks.
	if room := body - len(checks) - 1; room < len(state) {
		if room < 1 {
			// No honest room for any state line: the whole block goes, and the
			// checks block takes over the clipping exactly as it does today.
			return checks
		}
		hidden := len(state) - (room - 1)
		state = append(state[:room-1],
			"  "+mutedStyle.Render(fmt.Sprintf("… %s not shown", plural(hidden, "more line"))))
	}

	out := append([]string(nil), checks...)
	out = append(out, "")
	return append(out, state...)
}

// bodyBudget is how many lines both blocks together may occupy: the pane less
// the #-header and its blank line, and the blank line and footer below.
func (m Model) bodyBudget() int {
	const chrome = 4
	if m.height <= 0 {
		return 0
	}
	return m.height - chrome
}

// checkLines is the overlay's body, split out so a test can assert on the list
// without parsing the frame around it.
//
// Order is failing, then pending, then the passing count: the list is read
// top-down and the top is what you pressed `c` for. Within a bucket the API's
// own order is kept -- it groups a workflow's jobs together, which is more
// useful than an alphabetical sort that would interleave them.
func (m Model) checkLines(pr github.PR) []string {
	body := max(0, m.width-4)
	var out []string
	for _, g := range pr.FailedGates {
		out = append(out, "  "+errorStyle.Render("✗")+" "+clip(g, body))
	}
	for _, g := range pr.PendingGates {
		out = append(out, "  "+attentionStyle.Render("◐")+" "+mutedStyle.Render(clip(g, body)))
	}

	if len(out) == 0 {
		// Nothing is wrong and nothing is running, so the count is the whole
		// answer rather than a footnote to a list.
		switch {
		case pr.PassedCount > 0:
			return []string{"  " + okStyle.Render("✓") + " " +
				mutedStyle.Render(fmt.Sprintf("all %d checks passing", pr.PassedCount))}
		case pr.SkippedCount > 0:
			return []string{"  " + mutedStyle.Render("· every check skipped")}
		default:
			return []string{"  " + mutedStyle.Render("· no checks")}
		}
	}

	// The tally is pinned below the elision rather than passed through it: it
	// is what makes the numbers reconcile, so clipping it would leave the page
	// silently short of GitHub's total.
	out = m.fitChecks(out)
	if tally := checkTally(pr); tally != "" {
		out = append(out, "  "+okStyle.Render("✓")+" "+mutedStyle.Render(tally))
	}
	return out
}

// fitChecks keeps the overlay inside the pane. The list is already ranked by
// what you came to read, so dropping from the bottom loses the least: a scroll
// offset would add a second mode to something whose whole point is that any key
// dismisses it. The elision line is honest about what it hid.
//
// Budget: the #-header and its blank line, the blank line and "any key closes"
// below, the elision line itself, and the tally pinned under it.
func (m Model) fitChecks(lines []string) []string {
	const chrome = 6
	if m.height <= 0 || len(lines) <= m.height-chrome {
		return lines
	}
	keep := m.height - chrome
	if keep < 1 {
		keep = 1
	}
	hidden := len(lines) - keep
	return append(lines[:keep],
		"  "+mutedStyle.Render(fmt.Sprintf("… %s not shown", plural(hidden, "more line"))))
}

// checkTally closes the overlay's list the way `gh pr checks` closes its own:
// the named lines account for what is wrong, and one line accounts for
// everything else, so the total reconciles against GitHub instead of leaving
// the reader wondering what was omitted.
func checkTally(pr github.PR) string {
	var parts []string
	if pr.PassedCount > 0 {
		parts = append(parts, fmt.Sprintf("%d passing", pr.PassedCount))
	}
	if pr.SkippedCount > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", pr.SkippedCount))
	}
	return strings.Join(parts, ", ")
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
