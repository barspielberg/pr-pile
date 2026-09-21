package ui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/barspielberg/prs-mng/internal/config"
	"github.com/barspielberg/prs-mng/internal/github"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// detail.go carries the state block that sits under the check list.

// helpBlock is one titled group of rows. The page is assembled from blocks
// rather than written out as a string so the renderer can window it: the legend
// is longer than a short pane and the reader needs all of it, so it scrolls.
type helpBlock struct {
	title string
	rows  []helpRow
}

// helpRow is one legend entry: the key, what it does, and the key's own style
// where it has one. The glyph rows carry a colour because the colour is what
// they are the legend for -- the CI block's green check explains a green check
// on the board.
//
// The style travels beside the key rather than baked into it. Rendering it
// early and recovering it later would mean stripping the escape codes back off
// to search and highlight the text, and the search has to see plain characters:
// a query of "x" must not match the `m` in an SGR sequence.
type helpRow struct {
	key, desc string
	style     lipgloss.Style
}

// row is a legend row in the page's default style, which is most of them.
func row(key, desc string) helpRow {
	return helpRow{key: key, desc: desc, style: mutedStyle}
}

// glyph is a legend row whose key is a board glyph, shown in the colour it
// carries on the board.
func glyph(key, desc string, st lipgloss.Style) helpRow {
	return helpRow{key: key, desc: desc, style: st}
}

// The glyph key is rendered from the same helpers the rows use, so a legend can
// never drift from what is actually on screen.
func (m Model) helpBlocks() []helpBlock {
	// Aliases are grouped onto the key's own line rather than listed
	// separately: the reader is asking "how do I move", and four rows saying
	// "move" answer it worse than one.
	keys := helpBlock{"KEYS", []helpRow{
		row("j / k", "move ( ↓ ↑ )"),
		row("ctrl+d/u", "half a page down / up"),
		row("pgdn/pgup", "a full page down / up"),
		row("l / h", "next / previous section ( → ← )"),
		row("g / G", "top / bottom ( home / end )"),
		row("enter", "open in browser ( o )"),
		row("d", "detail for this PR"),
		row("y", "copy the PR url"),
		row("/", "search"),
		row("n / N", "next / previous match"),
		row("r", "reload"),
		row("?", "help, and close it again"),
		row("q", "quit ( esc, ctrl+c )"),
	}}
	for _, a := range m.cfg.Actions {
		if a.Run != "" {
			keys.rows = append(keys.rows, row(a.Key, a.Name))
		}
	}

	return []helpBlock{
		keys,
		// This page's own scroll and close keys are on its bottom row, live,
		// so listing them here too would be the one redundancy a legend cannot
		// justify -- it is the only section the reader can already see.
		{"OVERLAYS", []helpRow{
			row("d page", "any key closes it; j k l h close it and move"),
			row("? page", "scrolls and searches; see its bottom row"),
		}},
		{"SEARCH", []helpRow{
			row("/", "search; the board does not move"),
			row("n / N", "next / previous match, wrapping"),
			row("ctrl+n/p", "next / previous while typing ( ctrl+j/k, ↓ ↑ )"),
			row("enter", "keep the query and the highlights"),
			row("esc", "cancel, or clear the highlights from the board"),
			row("backspace", "edit the query"),
			row("text", "matches what you can see: number, title, author initials"),
			row("", "the author cell is 3 letters, so type those three"),
			row("", "this page searches the same way, over its own lines"),
		}},
		{"CI", []helpRow{
			glyph("✓", "passing", okStyle),
			glyph("✗2", "2 checks failing", errorStyle),
			glyph("◐", "running", attentionStyle),
			glyph("·", "no checks", mutedStyle),
		}},
		{"REVIEW", []helpRow{
			glyph("✓", "approved", okStyle),
			glyph("✗", "changes requested", errorStyle),
			glyph("○", "review required", attentionStyle),
		}},
		{"BLOCKERS", []helpRow{
			glyph("!", "merge conflicts", errorStyle),
			glyph("~", "draft", mutedStyle),
		}},
		{"ROWS", []helpRow{
			glyph("╭╴│╰╴", "a stack: each PR targets the one above", mutedStyle),
			row("abc", "author initials, on rules with author: true"),
			row("2h", "last updated"),
		}},
	}
}

// helpSegment is one styled run of a legend line. A line is a list of them
// because the glyph keys carry their own colour -- the ✗ in the CI block is
// the legend for a red ✗ on the board -- while the text around them is muted.
type helpSegment struct {
	text  string
	style lipgloss.Style
}

// helpLine is one line of the page: its segments in draw order, and the plain
// text they spell. The two are built together so the search can match what is
// on screen and the highlight can land on the right runes -- the same contract
// searchText gives the board.
type helpLine struct {
	segs []helpSegment
	text string
}

// helpPage is the whole legend, top to bottom, with the config path as its
// last line.
func (m Model) helpPage() []helpLine {
	var out []helpLine
	line := func(segs ...helpSegment) {
		var b strings.Builder
		for _, sg := range segs {
			b.WriteString(sg.text)
		}
		out = append(out, helpLine{segs: segs, text: b.String()})
	}
	for i, blk := range m.helpBlocks() {
		if i > 0 {
			line()
		}
		line(helpSegment{"  " + blk.title, headerStyle})
		for _, r := range blk.rows {
			line(
				// The indent is its own segment rather than part of the key's:
				// it is searchable either way, but a glyph key's colour is the
				// legend for that glyph, and stretching it over two leading
				// spaces makes it the legend for the margin as well.
				helpSegment{"  ", mutedStyle},
				helpSegment{r.key, r.style},
				// pad, not a width-based repeat, because it is what the rest of
				// the board pads with -- a key whose rune count and display
				// width differ must land in the same column here as everywhere
				// else.
				helpSegment{strings.TrimPrefix(pad(r.key, 10), r.key), mutedStyle},
				helpSegment{r.desc, mutedStyle},
			)
		}
	}
	line()
	line(helpSegment{fmt.Sprintf("  config: %s", config.Path()), mutedStyle})
	return out
}

// helpLines is the page as drawn: each line's segments rendered, with the runes
// the query matched filled by the same hitStyle the board uses. It is rendered
// in full and windowed afterwards, so the scroll position is an index into a
// list that does not depend on it.
func (m Model) helpLines() []string {
	page := m.helpPage()
	out := make([]string, len(page))
	// The band belongs to a live search, so it is gated on the query rather
	// than on helpMatch alone. A zero helpMatch is indistinguishable from
	// "line 0 is current", and New() leaves it at Go's zero value -- which put
	// a cursor on KEYS on a page nobody had searched.
	searching := strings.TrimSpace(m.helpQuery) != ""
	for i, l := range page {
		out[i] = l.render(textSpans(l.text, m.helpQuery), searching && i == m.helpMatch, m.width)
	}
	return out
}

// render draws one line, filling the runes the spans cover. Each segment is
// given the slice of the spans that falls inside it, so a match spanning the
// key and its description highlights across both.
//
// The current match takes the board's selected-row fill. Every match is filled
// the same, so the fill alone cannot say which one `n` is on -- the page did
// not change by a single byte between "1 of 4" and "2 of 4", which made the
// count assert a position the page then refused to show. The band is the same
// answer vim reaches for with hl-CurSearch and fzf with `hl+`: the current
// match gets a second channel, not a louder version of the first.
func (l helpLine) render(spans [][2]int, current bool, width int) string {
	paint := keepStyle
	if current {
		paint = func(st lipgloss.Style) lipgloss.Style { return st.Background(selBg) }
	}

	var b strings.Builder
	at := 0
	for _, sg := range l.segs {
		n := utf8.RuneCountInString(sg.text)
		b.WriteString(hitRuns(sg.text, cellHits(spans, [2]int{at, at + n}), sg.style, paint))
		at += n
	}

	line := b.String()
	if current {
		// Fill to the right edge so the band reads as one row, the way the
		// board's selected row does. A legend line is as long as its text, so
		// without this the band stops mid-pane and reads as a ragged stub
		// rather than a cursor.
		if gap := width - lipgloss.Width(line); gap > 0 {
			line += paint(fgStyle).Render(strings.Repeat(" ", gap))
		}
	}
	return line
}

// keepStyle is hitRuns' paint hook for a line the cursor is not on: the segment
// keeps the style it was given. The current match composes selBg over it
// instead, so the legend does have a cursor row now -- it just is not this one.
func keepStyle(st lipgloss.Style) lipgloss.Style { return st }

// helpMatches is every line the query matches, as indexes into helpPage. Like
// the board's matchIndexes it is recomputed rather than cached: the legend
// grows with the configured actions and the page is rebuilt every frame, so a
// stored match set could go stale under a highlight that says otherwise.
func (m Model) helpMatches() []int {
	if strings.TrimSpace(m.helpQuery) == "" {
		return nil
	}
	var out []int
	for i, l := range m.helpPage() {
		if textMatches(l.text, m.helpQuery) {
			out = append(out, i)
		}
	}
	return out
}

// helpPreview is the legend's incsearch: the page scrolls to the first match at
// or below where the search opened, as the query is typed.
//
// It stays put when the line it is already on still matches, so typing the
// middle of a word does not walk the page off a hit it had already found, and
// it leaves the page alone when nothing matches -- a query on its way to
// matching should not throw away where the reader was.
func (m *Model) helpPreview() {
	if m.helpQuery == "" {
		m.helpMatch, m.helpScroll = -1, m.helpOrigin
		return
	}
	matches := m.helpMatches()
	if len(matches) == 0 {
		m.helpMatch = -1
		return
	}
	for _, i := range matches {
		if i == m.helpMatch {
			return
		}
	}
	// From where the search opened rather than from the current match:
	// backspacing to a wider query has to be able to walk back up the page,
	// not only further down.
	m.helpMatch = matches[0]
	for _, i := range matches {
		if i >= m.helpOrigin {
			m.helpMatch = i
			break
		}
	}
	m.scrollToHelpMatch()
}

// scrollToHelpMatch brings the current match into the window, and does nothing
// when it is already there. A match the reader can see should not jump the page
// under them; one they cannot see is the whole reason they typed.
func (m *Model) scrollToHelpMatch() {
	if m.helpMatch < 0 {
		return
	}
	body := m.helpBody(len(m.helpPage()))
	switch {
	case m.helpMatch < m.helpScroll:
		m.helpScroll = m.helpMatch
	case m.helpMatch >= m.helpScroll+body:
		m.helpScroll = m.helpMatch - body + 1
	}
}

// helpBody is how many legend lines the pane shows: everything but the hint
// row, and the prompt row too while the search is open.
//
// The floor is 0, not 1. A 2-row pane with the prompt open has no room for a
// legend line at all, and floored at 1 the overlay came out three rows tall in
// a two-row pane -- which scrolls its own top off, the failure the height
// arithmetic exists to prevent. At that size the prompt and the hint are the
// whole page, which is the honest answer: they are what the reader is typing
// into and how they leave.
func (m Model) helpBody(total int) int {
	if m.height <= 1 {
		return total
	}
	body := m.height - 1
	if m.helpSearching {
		body--
	}
	if body > total {
		body = total
	}
	return max(0, body)
}

// stepHelpMatch is n and N on the legend: the next or previous match, wrapping
// like the board's. The wrap is announced in the same words, on the page's own
// hint row -- a silent wrap is indistinguishable from being stuck.
func (m Model) stepHelpMatch(forward bool) (tea.Model, tea.Cmd) {
	matches := m.helpMatches()
	if len(matches) == 0 {
		if strings.TrimSpace(m.helpQuery) == "" {
			return m, nil
		}
		m.helpStatus = "no matches"
		return m, nil
	}

	next, wrapped := matches[0], true
	if forward {
		for _, i := range matches {
			if i > m.helpMatch {
				next, wrapped = i, false
				break
			}
		}
	} else {
		next, wrapped = matches[len(matches)-1], true
		for j := len(matches) - 1; j >= 0; j-- {
			if matches[j] < m.helpMatch {
				next, wrapped = matches[j], false
				break
			}
		}
	}
	m.helpMatch = next
	m.scrollToHelpMatch()
	if !wrapped {
		m.helpStatus = ""
		return m, nil
	}
	m.helpStatus = "search hit BOTTOM, continuing at TOP"
	if !forward {
		m.helpStatus = "search hit TOP, continuing at BOTTOM"
	}
	return m, nil
}

// handleHelpSearchKey is the legend's prompt. It is the board's prompt with the
// cursor swapped for the scroll position: the same keys edit the query, the
// same chords step matches while typing, enter keeps the query and its
// highlights, and esc abandons the search and puts the page back.
func (m Model) handleHelpSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.helpSearching, m.helpQuery, m.helpMatch = false, "", -1
		m.helpScroll = m.helpOrigin
		return m, nil
	// An empty query is not a search, so these scroll the page instead --
	// which is what j and k do here, so the two motions agree.
	case "ctrl+n", "ctrl+j", "down":
		if strings.TrimSpace(m.helpQuery) == "" {
			m.helpScroll++
			break
		}
		return m.stepHelpMatch(true)
	case "ctrl+p", "ctrl+k", "up":
		if strings.TrimSpace(m.helpQuery) == "" {
			m.helpScroll--
			break
		}
		return m.stepHelpMatch(false)
	case "enter":
		m.helpSearching = false
		return m, nil
	default:
		q, ok := editQuery(m.helpQuery, msg)
		if !ok {
			return m, nil
		}
		m.helpQuery = q
		m.helpPreview()
	}
	m.clampHelpScroll()
	return m, nil
}

// helpPrompt is the legend's prompt row, drawn above the hint row. It is the
// board's prompt, counting lines instead of rows.
func (m Model) helpPrompt() string {
	matches := m.helpMatches()
	at := -1
	for i, idx := range matches {
		if idx == m.helpMatch {
			at = i
			break
		}
	}
	return m.renderPrompt(m.helpQuery, len(matches), at)
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
// The prompt is a second chrome row while the search is open, so the legend
// gives up one line for it -- the same trade the board's view makes.
func (m Model) helpOverlay() string {
	lines := m.helpLines()
	if m.height <= 1 {
		return strings.Join(lines, "\n")
	}
	body := m.helpBody(len(lines))
	top := m.helpTop(len(lines), body)
	out := append(lines[top:top+body:top+body], m.helpHint(top, len(lines), body))
	if m.helpSearching {
		out = append(out[:body:body], m.helpPrompt(), out[body])
	}
	return strings.Join(out, "\n")
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

// clampHelpScroll pins the stored offset to what the page can actually show.
// It is clamped on the keypress as well as at render: render-time clamping
// alone lets the offset drift past the end while j is held, and then the first
// k only walks that invisible surplus back down -- the page sits still for as
// many presses as it overshot, which reads as k being broken.
func (m *Model) clampHelpScroll() {
	total := len(m.helpPage())
	m.helpScroll = m.helpTop(total, m.helpBody(total))
}

// helpHint is the page's bottom row: which keys close it, and where you are.
// Both halves are load-bearing -- the keys because scrolling took `any key`
// away, the position because the whole point of this change is that the page no
// longer pretends it is showing everything.
func (m Model) helpHint(top, total, body int) string {
	left := "  esc q ? close · j/k scroll · / search"
	right := fmt.Sprintf("%d-%d of %d  ", top+1, top+body, total)
	switch {
	case body >= total:
		// The whole legend is on screen, so a range would be noise. The closing
		// keys still are not: they are why this row exists at every height.
		left = "  esc q ? close · / search"
		right = fmt.Sprintf("all %d  ", total)
	case top+body >= total:
		right = fmt.Sprintf("%d-%d of %d · end  ", top+1, total, total)
	}
	// The query outlives the prompt here too, so the row has to say what the
	// keys that only work now actually do.
	switch {
	case m.helpSearching:
		left = "  ctrl+n/p next · enter keep · esc cancel"
	case m.helpQuery != "":
		left = "  j/k scroll · n/N next match · esc clear · q ? close"
	}
	// The legend's own message, and only ever its own: the board's status line
	// is about the board, and routing these through it put every action failure
	// on this row.
	//
	// The message takes the row but the closing keys stay pinned to the front
	// of it. Substituting the whole row is what broke this before -- a page
	// whose only stated way out has been replaced by a transient reads as
	// stuck, which is the failure the comment above this function is about.
	// Appending instead just pushed the message off the right edge at 100
	// columns, so the verbose middle of the legend is what yields.
	if m.helpStatus != "" {
		left = "  esc q ? close · " + m.helpStatus
	}
	left = clip(left, max(0, m.width-lipgloss.Width(right)))
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
