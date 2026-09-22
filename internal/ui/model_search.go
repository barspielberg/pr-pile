package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"strings"
)

// While searching every printable key belongs to the query, so navigation has
// to move to chords. ctrl+n/p is what the user asked for; ctrl+j/k and the
// arrows are the same motions under the other two conventions.
func (m Model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		return m.cancelSearch(), nil
	// A matched header is a match like any other, so these land on one. That
	// reverses the old rule ("matches are rows, so these never stop on
	// furniture"), which was a consequence of the match set living in row
	// space -- a space with no way to name a header -- rather than a choice.
	// Now that a header can match, skipping it would have the footer count a
	// match the user cannot step to, which is the worse of the two. A header is
	// addressable but not actionable (docs/uniform-rows.md §4.1), so landing on
	// one is a position this layout already supports.
	//
	// An empty query is not a search, so these fall through to plain movement
	// instead -- which means they stop on headers exactly as j and k do. Two
	// motion keys disagreeing about the same board is the kind of thing that
	// gets noticed in use, and "the cursor sits on headers" is the whole design
	// this layout rests on, so the fallback honours it rather than quietly
	// skipping furniture.
	case "ctrl+n", "ctrl+j", "down":
		if m.noQuery() {
			m.cursor++
		} else {
			m.cursor = m.matchAfter(m.cursor)
		}
		m.clampCursor()
		return m, nil
	case "ctrl+p", "ctrl+k", "up":
		if m.noQuery() {
			m.cursor--
		} else {
			m.cursor = m.matchBefore(m.cursor)
		}
		m.clampCursor()
		return m, nil
	case "enter":
		return m.acceptSearch(), nil
	default:
		q, ok := editQuery(m.query, msg)
		if !ok {
			return m, nil
		}
		m.query = q
	}
	// No re-seating on every keystroke: a search does not narrow the board, so
	// the cursor's slot still means what it meant. previewMatch below walks it
	// to a match when one exists and deliberately leaves it alone when none
	// does.
	m.clampCursor()
	m.previewMatch()
	return m, nil
}

// previewMatch is incsearch: the cursor walks to the match as the query is
// typed, so the answer is on screen before the user stops typing.
//
// It stays put when the row it is on still matches -- otherwise typing the
// middle of a word would jitter the cursor off a row it had already found --
// and when nothing matches at all, since a query on its way to matching should
// not throw away where the user was.
func (m *Model) previewMatch() {
	// An empty query is the state the prompt opened in, so the cursor belongs
	// where it opened. Deleting back to nothing otherwise stranded it wherever
	// the last near-miss walked it -- a move the user never asked for, and one
	// esc would have undone.
	if m.query == "" {
		m.cursor = m.searchOrigin
		return
	}
	matches := m.matchIndexes()
	if len(matches) == 0 {
		return
	}
	for _, idx := range matches {
		if idx == m.cursor {
			return
		}
	}
	// From where the search opened rather than from the cursor: backspacing to
	// a wider query has to be able to walk back up, not only further down.
	for _, idx := range matches {
		if idx >= m.searchOrigin {
			m.cursor = idx
			return
		}
	}
	m.cursor = matches[0]
}

// matchAfter is the first match below slot i, wrapping to the top. With no
// query, or no match, it is the next slot -- so the chords still move on an
// empty prompt rather than doing nothing.
func (m Model) matchAfter(i int) int {
	matches := m.matchIndexes()
	if len(matches) == 0 {
		return i + 1
	}
	for _, idx := range matches {
		if idx > i {
			return idx
		}
	}
	return matches[0]
}

// matchBefore is the first match above i, wrapping to the bottom.
func (m Model) matchBefore(i int) int {
	matches := m.matchIndexes()
	if len(matches) == 0 {
		return i - 1
	}
	for j := len(matches) - 1; j >= 0; j-- {
		if matches[j] < i {
			return matches[j]
		}
	}
	return matches[len(matches)-1]
}

// editQuery applies one keystroke to the query, and reports whether the key
// belonged to it at all. Both search prompts type into it, so the editing
// rules cannot drift apart between the board and the help page.
//
// ctrl+u is not one of them. It means half a page everywhere else, and a chord
// that clears a query on two prompts and scrolls on two pages is the kind of
// split the reader has to hold in their head. Backspace already edits and esc
// already cancels, so the prompt lost nothing by giving it up.
func editQuery(query string, msg tea.KeyMsg) (string, bool) {
	switch msg.String() {
	case "backspace":
		if r := []rune(query); len(r) > 0 {
			return string(r[:len(r)-1]), true
		}
		return query, true
	}
	// Space arrives as its own key type with no runes attached, so it has to be
	// spelled out or multi-word queries would silently drop it.
	switch {
	case msg.Type == tea.KeySpace:
		return query + " ", true
	case msg.Type == tea.KeyRunes && len(msg.Runes) > 0:
		return query + string(msg.Runes), true
	}
	return query, false
}

// cancelSearch is esc in the prompt: the search is abandoned, so the cursor
// goes back to where / was pressed. incsearch walked it while the query was
// being typed, and leaving it wherever the last near-miss happened to be would
// be a move the user never asked for.
func (m Model) cancelSearch() Model {
	m.searching = false
	m.query = ""
	m.cursor = m.searchOrigin
	m.clampCursor()
	return m
}

// acceptSearch is enter: the prompt closes and everything else stays -- the
// cursor on its match, the query live, the highlights on the board. That is
// vim's hlsearch, and it is what gives n and N something to walk.
//
// It no longer opens the PR. <CR> accepts a search everywhere else this model
// comes from, and enter or o is still one keypress away.
func (m Model) acceptSearch() Model {
	m.searching = false
	m.clampCursor()
	return m
}

// stepMatch is n and N: the next or previous match, wrapping like vim's
// wrapscan. There is no opening direction to be relative to -- ? is the help
// key, so there is no backwards-open -- which makes n always forward and N
// always backward, vim's own post-/ rule with the unreachable half removed.
//
// The wrap is announced, in less's wording. A silent wrap is indistinguishable
// from being stuck on the last match.
func (m Model) stepMatch(forward bool) (tea.Model, tea.Cmd) {
	matches := m.matchIndexes()
	if len(matches) == 0 {
		if strings.TrimSpace(m.query) == "" {
			return m, nil
		}
		return m, func() tea.Msg { return statusMsg("no matches") }
	}

	cur := m.cursor
	var next int
	var wrapped bool
	if forward {
		next = m.matchAfter(cur)
		wrapped = next <= cur
	} else {
		next = m.matchBefore(cur)
		wrapped = next >= cur
	}
	m.cursor = next
	m.clampCursor()
	if !wrapped {
		m.status = ""
		return m, nil
	}
	msg := "search hit BOTTOM, continuing at TOP"
	if !forward {
		msg = "search hit TOP, continuing at BOTTOM"
	}
	return m, func() tea.Msg { return statusMsg(msg) }
}
