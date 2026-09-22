package ui

import "github.com/barspielberg/pr-pile/internal/github"

// Keyed by PR number, not position: rows move between frames, so a selection
// held by position would come to mean different PRs after a refresh.
func (m *Model) toggleSelect() {
	pr, ok := m.selected()
	if !ok {
		return
	}
	if m.selection[pr.Number] {
		delete(m.selection, pr.Number)
		return
	}
	m.selection[pr.Number] = true
}

func (m Model) isSelected(number int) bool { return m.selection[number] }

// In board order: Go randomizes map iteration, so reading the set directly
// would reorder the copied urls on every press.
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

// actionPRs is what a key acts on: the selection when there is one, the row
// under the cursor when there is not.
func (m Model) actionPRs() []github.PR {
	if prs := m.selectedPRs(); len(prs) > 0 {
		return prs
	}
	if pr, ok := m.selected(); ok {
		return []github.PR{pr}
	}
	return nil
}

func (m *Model) clearSelection() {
	if len(m.selection) > 0 {
		m.selection = map[int]bool{}
	}
	m.rangeOwned = map[int]bool{}
	m.ranging, m.anchor = false, -1
}

// Ending a range keeps what it selected; esc is the key that discards.
func (m *Model) startRange() {
	if m.ranging {
		m.ranging, m.anchor = false, -1
		return
	}
	m.ranging, m.anchor = true, m.cursor
	m.applyRange()
}

// applyRange selects every PR row between the anchor and the cursor, adding to
// the selection rather than replacing it, so space then v gives both.
func (m *Model) applyRange() {
	if !m.ranging || m.anchor < 0 {
		return
	}
	lo, hi := m.anchor, m.cursor
	if lo > hi {
		lo, hi = hi, lo
	}
	for _, pr := range m.rangeDrop(lo, hi) {
		delete(m.selection, pr)
	}
	sl := m.slots()
	for i := lo; i <= hi && i < len(sl); i++ {
		if !sl[i].isRow() {
			continue
		}
		number := sl[i].row.PR.Number
		// A row already marked with space is not claimed by the range, or
		// shrinking the range back off it would undo the user's own mark.
		if !m.selection[number] {
			m.rangeOwned[number] = true
		}
		m.selection[number] = true
	}
}

// rangeDrop is the PRs this range selected that its current span no longer
// covers, so walking the cursor back does not leave a trail.
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
