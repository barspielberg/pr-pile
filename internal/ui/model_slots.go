package ui

import (
	"fmt"
	"github.com/barspielberg/pr-pile/internal/board"
	"github.com/barspielberg/pr-pile/internal/github"
	"strings"
)

// sections is the board's own sections, unconditionally. A search marks rows
// where they are rather than collecting them: you usually care about the rows
// around the one you are looking for, and a board that reshuffles under the
// query cannot highlight what it hid.
func (m Model) sections() []board.Section {
	return m.board.Sections()
}

// matchIndexes is every match's index into the cursor's address space -- a
// SLOT index, not a row index. Both a PR row and a section header can match, so
// the match set is expressed in the one space that holds them both, in draw
// order.
//
// It used to be row space, which could not name a header at all and cost every
// caller a conversion through rowSlotClamped. Slot space removes the
// conversion: a match index is a cursor position already.
//
// It is recomputed rather than cached: the board refreshes under the query and
// the width changes what is drawn, so a stored match set would go stale
// silently -- and a stale highlight is the one thing this design cannot show.
func (m Model) matchIndexes() []int {
	if m.noQuery() {
		return nil
	}
	var out []int
	for i, s := range m.slots() {
		switch {
		case s.isRow() && m.rowMatches(s.row, s.author, m.query):
		case s.isHeader() && m.headerMatches(s.section, s.count, m.query):
		default:
			continue
		}
		out = append(out, i)
	}
	return out
}

// sectionStarts gives the cursor index of each section's header, so l/h can
// jump between them without the cursor knowing about sections.
//
// The target is the header, not the first PR row. The header is the section's
// own first slot, so a jump lands on the thing that names where you have
// arrived -- and `j` from there is the first PR, which is one extra keypress
// only if you did not want the orientation.
//
// Every section is listed, including empty and still-loading ones: they have a
// header now, so there is somewhere to land. The old gutter had nothing to
// point at in an empty section and had to skip it.
// Derived from slots() rather than counted independently: a second walk that
// has to stay in step with the first is how the note slot desynced the cursor
// once already.
func (m Model) sectionStarts() []int {
	var starts []int
	for i, s := range m.slots() {
		if s.isHeader() {
			starts = append(starts, i)
		}
	}
	return starts
}

// nextSection moves to the following section's header, or to the last slot
// when there is none -- the same end-stop behaviour as j.
func (m Model) nextSection() int {
	for _, start := range m.sectionStarts() {
		if start > m.cursor {
			return start
		}
	}
	if n := len(m.slots()); n > 0 {
		return n - 1
	}
	return 0
}

// prevSection moves to the current section's header, or to the previous one
// when already there, which is how a "back" key is expected to feel.
func (m Model) prevSection() int {
	starts := m.sectionStarts()
	for i := len(starts) - 1; i >= 0; i-- {
		if starts[i] < m.cursor {
			return starts[i]
		}
	}
	return 0
}

// visibleRows flattens the drawable sections so the cursor can move across
// section boundaries without knowing about them.
func (m Model) visibleRows() []board.Row {
	var rows []board.Row
	for _, s := range m.sections() {
		// Stale rows are drawn, so they must be navigable too.
		rows = append(rows, s.Rows...)
	}
	return rows
}

// slotKind is what the cursor is sitting on. A header is addressable but not
// actionable: the cursor can rest there, and every key that operates on a PR
// does nothing.
type slotKind int

const (
	slotRow slotKind = iota
	slotHeader
	// slotNote is a section's single no-rows line: the spinner while loading,
	// an em dash once resolved empty, the error text on failure. Addressable
	// so that it is not an unselectable line in the middle of the list, and
	// not actionable because there is no PR behind it.
	slotNote
)

// slot is one position in the cursor's address space. That space contains the
// section headers as well as the PR rows, which is what makes every line of
// the list reachable and the one-keypress-one-line invariant hold by
// construction rather than by viewport arithmetic: with nothing to skip, the
// cursor's line and its index move together. docs/uniform-rows.md §4.1.
//
// The kind is carried on the value rather than inferred from the index,
// because index arithmetic is exactly what goes wrong once the address space
// holds two things: "slot 3" tells you nothing about whether it is a header,
// and code that hand-counts past headers is code that breaks when a section
// empties. Ask the slot what it is.
type slot struct {
	kind    slotKind
	section string
	row     board.Row
	rowIdx  int // index into visibleRows(), -1 for a header
	// author is the section rule's author setting, which decides whether a row
	// draws its author cell and so whether the search can match it. It is a
	// property of the rule, so every slot in the section carries it.
	author bool
	// count is what a HEADER draws on its right, and is empty on every other
	// kind. It is here because it decides where the name is clipped, and so
	// what a query can match -- not because a row has a count of its own.
	//
	// Both are read off the slot rather than looked up again, which keeps the
	// match walk on the address space it reports indexes into: a second walk of
	// m.sections() that has to stay in step with slots() is how the note slot
	// desynced the cursor once already.
	count string
}

func (s slot) isHeader() bool { return s.kind == slotHeader }
func (s slot) isRow() bool    { return s.kind == slotRow }
func (s slot) isNote() bool   { return s.kind == slotNote }

// slotAt returns the slot the cursor is on, and whether there is one.
func (m Model) slotAt(i int) (slot, bool) {
	sl := m.slots()
	if i < 0 || i >= len(sl) {
		return slot{}, false
	}
	return sl[i], true
}

// rowSlot is the cursor index of the nth PR row, counting across sections and
// ignoring headers, or -1 if there is no such row. This is the accessor a
// caller wants when it means "the third PR on the board" -- notably every test
// that used to say `m.cursor = 3` and mean exactly that.
func (m Model) rowSlot(n int) int {
	for i, s := range m.slots() {
		if s.isRow() && s.rowIdx == n {
			return i
		}
	}
	return -1
}

// headerSlot is the cursor index of the nth section's header, or -1. Sections
// are counted as drawn, so an empty or still-loading section has one too.
func (m Model) headerSlot(n int) int {
	seen := 0
	for i, s := range m.slots() {
		if !s.isHeader() {
			continue
		}
		if seen == n {
			return i
		}
		seen++
	}
	return -1
}

// slots is the cursor's address space: headers interleaved with their rows, in
// draw order. The blank separator between sections is NOT a slot -- it carries
// no information and stopping on it twice per boundary would be a dead beat
// with nothing to read.
// It must agree with body() line for line, because every slot index the rest
// of the model uses -- the cursor, the footer's position, l/h -- is an index
// into this. They are kept in step by construction: both walk m.sections() in
// the same order and emit a slot for the same three things.
func (m Model) slots() []slot {
	var out []slot
	idx := 0
	for _, s := range m.sections() {
		base := slot{section: s.Rule.Name, rowIdx: -1, author: s.Rule.Author}
		header := base
		header.kind, header.count = slotHeader, headerCount(s)
		out = append(out, header)
		for _, r := range s.Rows {
			row := base
			row.kind, row.row, row.rowIdx = slotRow, r, idx
			out = append(out, row)
			idx++
		}
		if m.hasNote(s) {
			note := base
			note.kind = slotNote
			out = append(out, note)
		}
	}
	return out
}

// headerCount is the count a section's header draws, empty while the rule is
// still pending: the number is unknown until it resolves, so the header carries
// the name alone rather than a number about to change.
//
// The search needs it as well as body() does -- it decides where the name is
// clipped, and so what a query can match -- and a rule this small is exactly
// the kind that drifts when it is written twice.
func headerCount(s board.Section) string {
	if s.State != board.Ready {
		return ""
	}
	return fmt.Sprint(len(s.Rows))
}

// hasNote says whether a section draws its no-rows line. body() asks the same
// question, so the two cannot drift.
func (m Model) hasNote(s board.Section) bool {
	switch s.State {
	case board.Failed:
		return true
	case board.Ready:
		return len(s.Rows) == 0
	case board.Pending:
		return len(s.Rows) == 0
	}
	return false
}

// firstRowSlot is the index of the first PR row in the address space, or 0 if
// there is none. Filtering uses it: a query narrows the board to matches, so
// opening the cursor on a header -- a line that is by definition not a match --
// would make the first ctrl+n a wasted keypress.
func (m Model) firstRowSlot() int {
	if i := m.rowSlot(0); i >= 0 {
		return i
	}
	return 0
}

// nextRowSlot is the next PR row in direction dir, or the current slot when
// there is none -- the same end-stop behaviour j and k have on the board.
func (m Model) nextRowSlot(dir int) int {
	sl := m.slots()
	for i := m.cursor + dir; i >= 0 && i < len(sl); i += dir {
		if sl[i].isRow() {
			return i
		}
	}
	if m.cursor >= 0 && m.cursor < len(sl) {
		return m.cursor
	}
	return m.firstRowSlot()
}

// noQuery reports whether there is no search to step through. Whitespace does
// not count: a blank query matches nothing, row or header, so stepping it as a
// search would be a no-op the user reads as a broken key.
func (m Model) noQuery() bool { return strings.TrimSpace(m.query) == "" }

// selected is the PR under the cursor. A header has none, so every key that
// acts on a PR falls through to doing nothing there.
func (m Model) selected() (github.PR, bool) {
	s, ok := m.slotAt(m.cursor)
	if !ok || !s.isRow() {
		return github.PR{}, false
	}
	return s.row.PR, true
}

// Sections resolve progressively, so the slot under the cursor can disappear
// between frames. searchOrigin is clamped alongside it: a refresh can reorder
// the board while the prompt is open, and esc landing a slot or two off is
// acceptable where an out-of-range index is not.
func (m *Model) clampCursor() {
	n := len(m.slots())
	if n == 0 {
		m.cursor, m.searchOrigin = 0, 0
		return
	}
	m.cursor = clampIndex(m.cursor, n)
	m.searchOrigin = clampIndex(m.searchOrigin, n)
	// An open range follows the cursor. This lives here rather than at each
	// movement key because clampCursor is already the one place every move
	// funnels through -- nine call sites and counting -- and a range that had
	// to be re-applied by hand at each of them would stop tracking the moment
	// someone added a tenth.
	m.applyRange()
}

func clampIndex(i, n int) int {
	if i >= n {
		return n - 1
	}
	if i < 0 {
		return 0
	}
	return i
}
