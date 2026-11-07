package ui

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"strings"
)

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
