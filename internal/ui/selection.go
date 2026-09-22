package ui

import "github.com/barspielberg/pr-pile/internal/github"

// The selection is a set of PR NUMBERS, not slot or row indexes.
//
// The board refetches on a timer and its sections resolve one at a time, so a
// row's position is not stable between frames -- clampCursor exists for exactly
// that reason. A selection held by position would silently come to mean
// different PRs after a refresh reordered a section, and the user would find
// out by copying the wrong urls. A number is the one handle on a PR that the
// board cannot change under us.
//
// A selected PR that leaves the board entirely is dropped when the selection is
// read (selectedPRs walks what is on screen now), so nothing has to be
// reconciled on every Apply.
func (m *Model) toggleSelect() {
	pr, ok := m.selected()
	if !ok {
		// A header or a note. Not an error: the cursor is allowed to rest
		// there, and a status barking about it would be noise on a keypress
		// the user will repeat two rows down.
		return
	}
	if m.selection[pr.Number] {
		delete(m.selection, pr.Number)
		return
	}
	m.selection[pr.Number] = true
}

// isSelected reports whether a PR number is in the set. Nil-safe so a
// zero-value Model -- which several tests build -- can still be rendered.
func (m Model) isSelected(number int) bool { return m.selection[number] }

// selectedPRs is the selection in BOARD order, which is not the map's order.
//
// Go randomizes map iteration, so reading the set directly would put the copied
// urls in a different order on every press. Walking the slots instead gives the
// order the user is looking at, which is the only one they can predict.
func (m Model) selectedPRs() []github.PR {
	if len(m.selection) == 0 {
		return nil
	}
	var out []github.PR
	for _, s := range m.slots() {
		if s.isRow() && m.selection[s.row.PR.Number] {
			out = append(out, s.row.PR)
		}
	}
	return out
}

// actionPRs is what a key acts on: the selection when there is one, and the row
// under the cursor when there is not. Every action key goes through this, so a
// user who never presses space sees no change in behaviour anywhere.
func (m Model) actionPRs() []github.PR {
	if prs := m.selectedPRs(); len(prs) > 0 {
		return prs
	}
	if pr, ok := m.selected(); ok {
		return []github.PR{pr}
	}
	return nil
}

// clearSelection drops the set and leaves range mode. Used by esc, by a
// completed action, and by a refresh.
func (m *Model) clearSelection() {
	if len(m.selection) > 0 {
		m.selection = map[int]bool{}
	}
	m.rangeOwned = map[int]bool{}
	m.ranging, m.anchor = false, -1
}

// startRange anchors a range at the cursor, or ends one that is already open.
// `v` is its own toggle so there are two ways out of the mode -- `v` again and
// `esc` -- which is what lazygit landed on after finding a mode with one exit
// is a mode people get stuck in.
//
// Ending a range KEEPS what it selected. The mode is a way of marking rows, not
// a container for them: leaving it should not throw away the work. esc is the
// key that discards.
func (m *Model) startRange() {
	if m.ranging {
		m.ranging, m.anchor = false, -1
		return
	}
	m.ranging, m.anchor = true, m.cursor
	m.applyRange()
}

// applyRange selects every PR row between the anchor and the cursor, in either
// direction. Called on every move while ranging.
//
// It ADDS to the selection rather than replacing it, so `space` on a few
// scattered rows and then `v` over a run gives both -- the documented route to
// a non-contiguous selection.
//
// Headers and notes inside the span contribute nothing and are not an error:
// the range is drawn over slots because that is what the cursor moves through,
// but only the PR rows inside it are collected.
func (m *Model) applyRange() {
	if !m.ranging || m.anchor < 0 {
		return
	}
	lo, hi := m.anchor, m.cursor
	if lo > hi {
		lo, hi = hi, lo
	}
	// Rows the range previously covered but no longer does have to come back
	// out, or walking the cursor back over a range would leave a trail behind
	// it. Everything selected outside the span is left alone, which is what
	// keeps an earlier `space` from being undone.
	for _, pr := range m.rangeDrop(lo, hi) {
		delete(m.selection, pr)
	}
	sl := m.slots()
	for i := lo; i <= hi && i < len(sl); i++ {
		if !sl[i].isRow() {
			continue
		}
		number := sl[i].row.PR.Number
		// A row the user had already marked with `space` is NOT claimed by the
		// range, even though the span covers it. Claiming it would mean
		// shrinking the range back off that row released a mark the range
		// never made -- the user's own work, undone by a cursor move.
		if !m.selection[number] {
			m.rangeOwned[number] = true
		}
		m.selection[number] = true
	}
}

// rangeDrop is the PRs this range had selected that its current span no longer
// covers. Only rows the RANGE selected are eligible: one marked with `space`
// before the range started is the user's and stays.
func (m *Model) rangeDrop(lo, hi int) []int {
	sl := m.slots()
	inSpan := map[int]bool{}
	for i := lo; i <= hi && i < len(sl); i++ {
		if sl[i].isRow() {
			inSpan[sl[i].row.PR.Number] = true
		}
	}
	var out []int
	for number := range m.rangeOwned {
		if !inSpan[number] {
			out = append(out, number)
			delete(m.rangeOwned, number)
		}
	}
	return out
}
