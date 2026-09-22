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
	m.ranging, m.anchor = false, -1
}
