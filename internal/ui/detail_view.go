package ui

import (
	"fmt"
	"github.com/barspielberg/pr-pile/internal/github"
	"strings"
)

// detailOverlay answers "what do I do about this PR" for the selected row. It
// began as the checks page, and the checks block is still the top of it: the
// first thing the page answers is what CI says.
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
		headerStyle.Render("  "+m.prRef(pr)) + " " +
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
// top-down and the top is what you pressed `d` for. Within a bucket the API's
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
