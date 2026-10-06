package ui

import (
	"fmt"
	"github.com/barspielberg/pr-pile/internal/github"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
// It navigates like the `?` page: the scroll keys scroll, everything else
// closes, and the bottom row says so at every height. It used to clip on a
// short pane, and j closed it and moved the board -- so scrolling down to read
// the clipped lines threw the reader out instead.
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
	body := m.overlayBody(pr)
	rows := m.detailRows(len(body))
	top := m.detailTop(len(body))
	lines = append(lines, body[top:top+rows]...)
	lines = append(lines, m.detailHint(top, len(body), rows))

	// No trailing newline: a pane is as many lines as it is tall, and one more
	// scrolls the header off the top edge.
	if m.height > 0 {
		lines = lines[:min(len(lines), m.height)]
	}
	return strings.Join(lines, "\n")
}

// overlayBody is both blocks, unclipped. The checks block comes first so it is
// what a short pane shows before any scrolling. See docs/pr-detail.md §6.2.
func (m Model) overlayBody(pr github.PR) []string {
	checks := m.checkLines(pr)
	state := m.stateLines(pr)
	if len(state) == 0 {
		return checks
	}
	out := append([]string(nil), checks...)
	out = append(out, "")
	return append(out, state...)
}

// detailRows is how many body lines the page shows: the pane less the header,
// its blank line, and the hint row.
func (m Model) detailRows(total int) int {
	if m.height <= 0 {
		return total
	}
	return max(0, min(total, m.height-3))
}

// detailTop clamps the stored offset to what the page can show, at render as
// well as on the keypress so a resize cannot strand it past the end.
func (m Model) detailTop(total int) int {
	return max(0, min(m.detailScroll, total-m.detailRows(total)))
}

func (m *Model) clampDetailScroll() {
	pr, ok := m.selected()
	if !ok {
		m.detailScroll = 0
		return
	}
	m.detailScroll = m.detailTop(len(m.overlayBody(pr)))
}

// detailHint is the page's bottom row, worded like the `?` page's: which keys
// close it, and where you are.
func (m Model) detailHint(top, total, rows int) string {
	left := "  esc q d close · j/k scroll · ? help"
	right := fmt.Sprintf("%d-%d of %d  ", top+1, top+rows, total)
	switch {
	case rows >= total:
		left = "  esc q d close · ? help"
		right = fmt.Sprintf("all %d  ", total)
	case top+rows >= total:
		right = fmt.Sprintf("%d-%d of %d · end  ", top+1, total, total)
	}
	left = clip(left, max(0, m.width-lipgloss.Width(right)))
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return mutedStyle.Render(clip(left, m.width))
	}
	return mutedStyle.Render(left + strings.Repeat(" ", gap) + right)
}

// checkLines is the overlay's body, split out so a test can assert on the list
// without parsing the frame around it.
//
// Order is failing, pending, cancelled, then the passing count: the list is read
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
	// Muted rather than red: a cancelled check usually wants a re-run, not a fix.
	for _, g := range pr.CancelledGates {
		out = append(out, "  "+mutedStyle.Render("⊘ "+clip(g+" (cancelled)", body)))
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

	if tally := checkTally(pr); tally != "" {
		out = append(out, "  "+okStyle.Render("✓")+" "+mutedStyle.Render(tally))
	}
	return out
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

// scrollDetail moves the detail page on the `?` page's scroll keys, at every
// height. It reports false for any other key, which closes the page.
func (m Model) scrollDetail(msg tea.KeyMsg) (Model, bool) {
	pr, ok := m.selected()
	if !ok {
		return m, false
	}
	// Steps are sized to this page's own window, which is two rows shorter
	// than the legend's: the shared ones would skip a line on every pgdown.
	full := max(1, m.height-4)
	half := max(1, full/2)
	switch msg.String() {
	case "j", "down":
		m.detailScroll++
	case "k", "up":
		m.detailScroll--
	case "ctrl+d":
		m.detailScroll += half
	case "ctrl+u":
		m.detailScroll -= half
	case "pgdown":
		m.detailScroll += full
	case "pgup":
		m.detailScroll -= full
	case "g", "home":
		m.detailScroll = 0
	case "G", "end":
		m.detailScroll = len(m.overlayBody(pr))
	default:
		return m, false
	}
	m.clampDetailScroll()
	return m, true
}
