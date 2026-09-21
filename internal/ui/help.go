package ui

import (
	"fmt"
	"strings"

	"github.com/barspielberg/prs-mng/internal/config"
	"github.com/barspielberg/prs-mng/internal/github"
	"github.com/charmbracelet/lipgloss"
)

// detail.go carries the state block that sits under the check list.

// helpBlock is one titled group of rows. The page is assembled from blocks
// rather than written out as a string so the renderer can window it: the legend
// is longer than a short pane and the reader needs all of it, so it scrolls.
type helpBlock struct {
	title string
	rows  [][2]string
}

// The glyph key is rendered from the same helpers the rows use, so a legend can
// never drift from what is actually on screen.
func (m Model) helpBlocks() []helpBlock {
	// Aliases are grouped onto the key's own line rather than listed
	// separately: the reader is asking "how do I move", and four rows saying
	// "move" answer it worse than one.
	keys := helpBlock{"KEYS", [][2]string{
		{"j / k", "move ( ↓ ↑ )"},
		{"l / h", "next / previous section ( → ← )"},
		{"g / G", "top / bottom ( home / end )"},
		{"enter", "open in browser ( o )"},
		{"d", "detail for this PR"},
		{"y", "copy the PR url"},
		{"/", "search"},
		{"n / N", "next / previous match"},
		{"r", "reload"},
		{"?", "help, and close it again"},
		{"q", "quit ( esc, ctrl+c )"},
	}}
	for _, a := range m.cfg.Actions {
		if a.Run != "" {
			keys.rows = append(keys.rows, [2]string{a.Key, a.Name})
		}
	}

	return []helpBlock{
		keys,
		// This page's own scroll and close keys are on its bottom row, live,
		// so listing them here too would be the one redundancy a legend cannot
		// justify -- it is the only section the reader can already see.
		{"OVERLAYS", [][2]string{
			{"d page", "any key closes it; j k l h close it and move"},
			{"? page", "scrolls; see its bottom row"},
		}},
		{"SEARCH", [][2]string{
			{"/", "search; the board does not move"},
			{"n / N", "next / previous match, wrapping"},
			{"ctrl+n/p", "next / previous while typing ( ctrl+j/k, ↓ ↑ )"},
			{"enter", "keep the query and the highlights"},
			{"esc", "cancel, or clear the highlights from the board"},
			{"backspace", "edit the query"},
			{"ctrl+u", "clear the query"},
			{"text", "matches what you can see: number, title, author initials"},
			{"", "the author cell is 3 letters, so type those three"},
		}},
		{"CI", [][2]string{
			{okStyle.Render("✓"), "passing"},
			{errorStyle.Render("✗2"), "2 checks failing"},
			{attentionStyle.Render("◐"), "running"},
			{mutedStyle.Render("·"), "no checks"},
		}},
		{"REVIEW", [][2]string{
			{okStyle.Render("✓"), "approved"},
			{errorStyle.Render("✗"), "changes requested"},
			{attentionStyle.Render("○"), "review required"},
		}},
		{"BLOCKERS", [][2]string{
			{errorStyle.Render("!"), "merge conflicts"},
			{mutedStyle.Render("~"), "draft"},
		}},
		{"ROWS", [][2]string{
			{mutedStyle.Render("╭╴│╰╴"), "a stack: each PR targets the one above"},
			{"abc", "author initials, on rules with author: true"},
			{"2h", "last updated"},
		}},
	}
}

// helpLines is the whole page as one column, top to bottom, with the config
// path as its last line. It is rendered in full and windowed afterwards, so the
// scroll position is an index into a list that does not depend on it.
func (m Model) helpLines() []string {
	var out []string
	for i, blk := range m.helpBlocks() {
		if i > 0 {
			out = append(out, "")
		}
		out = append(out, headerStyle.Render("  "+blk.title))
		for _, r := range blk.rows {
			out = append(out, "  "+mutedStyle.Render(pad(r[0], 10))+mutedStyle.Render(r[1]))
		}
	}
	return append(out, "", mutedStyle.Render(fmt.Sprintf("  config: %s", config.Path())))
}

// helpOverlay draws the legend as one scrolling column. It clipped from the top
// before, which lost KEYS -- the section anyone opening `?` is looking for. A
// two-column fold was tried and reverted: it was more layout code and it still
// clipped on a short pane, so it paid complexity without buying the fix.
//
// The last row is always a hint rather than more legend. Always, not only when
// the page overflows: scrolling took the old "any key closes" contract away
// from j and k at every height, so a page that quietly omitted the row at some
// heights would be lying about how to leave it on exactly those screens.
//
// It cost a real bug to learn that. The row used to be drawn only when the
// legend did not fit, which left a band of pane heights -- 45 and 46 rows for a
// 45-line legend -- where `?` showed no affordance at all and `j` was a silent
// no-op that still swallowed the key. That is indistinguishable from "scrolling
// is broken", and it is at the heights a full-screen terminal actually reports.
// One shape at every height is worth the row.
func (m Model) helpOverlay() string {
	lines := m.helpLines()
	if m.height <= 1 {
		return strings.Join(lines, "\n")
	}
	body := m.height - 1
	if body > len(lines) {
		body = len(lines)
	}
	top := m.helpTop(len(lines), body)
	return strings.Join(append(lines[top:top+body], m.helpHint(top, len(lines), body)), "\n")
}

// helpTop clamps the stored scroll offset to what the page can actually show.
// It is clamped at render rather than on the keypress so a resize cannot strand
// the view past the end of a page that just got shorter.
func (m Model) helpTop(total, body int) int {
	top := m.helpScroll
	if last := total - body; top > last {
		top = last
	}
	if top < 0 {
		top = 0
	}
	return top
}

// helpHint is the page's bottom row: which keys close it, and where you are.
// Both halves are load-bearing -- the keys because scrolling took `any key`
// away, the position because the whole point of this change is that the page no
// longer pretends it is showing everything.
func (m Model) helpHint(top, total, body int) string {
	left := "  esc q ? close · j/k scroll"
	right := fmt.Sprintf("%d-%d of %d  ", top+1, top+body, total)
	switch {
	case body >= total:
		// The whole legend is on screen, so a range would be noise. The closing
		// keys still are not: they are why this row exists at every height.
		left = "  esc q ? close"
		right = fmt.Sprintf("all %d  ", total)
	case top+body >= total:
		right = fmt.Sprintf("%d-%d of %d · end  ", top+1, total, total)
	}
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return mutedStyle.Render(clip(left, m.width))
	}
	return mutedStyle.Render(left + strings.Repeat(" ", gap) + right)
}

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
