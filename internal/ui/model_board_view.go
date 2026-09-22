package ui

import (
	"github.com/barspielberg/pr-pile/internal/board"
)

// body renders every section and reports the line each cursor slot starts on.
// EVERY line is a slot -- headers, PR rows and the no-rows note alike -- so
// slotStarts is the identity, which is the property that makes the scroll
// rhythm even. See docs/uniform-rows.md §4.1.
func (m Model) body(spin string) (lines []string, slotStarts []int) {
	slotIdx := 0
	addSlot := func(line string) {
		slotStarts = append(slotStarts, len(lines))
		lines = append(lines, line)
		slotIdx++
	}
	// A section with no rows still occupies exactly one note line -- the
	// spinner while loading, an em dash once resolved empty, the error text on
	// failure -- and that line is a SLOT. It was not, and the cursor skipped
	// it: by the rule in docs/uniform-rows.md §4.1 every unselectable line in
	// the list costs one to the worst-case scroll delta, so an empty, failed
	// or pending section broke the very invariant this layout exists to hold.
	//
	// A note carries no PR, so it is addressable but not actionable, exactly
	// like a header: selected() returns nothing on it and every key that acts
	// on a PR is a silent no-op there.
	note := func(text string) {
		addSlot(renderSectionNote(text))
	}
	// No blank separator between sections. It was drawn at first and measured:
	// a blank is a line the cursor cannot occupy, and every such line costs
	// exactly one to the worst-case scroll delta -- with it the board moved 2
	// lines per keypress at a boundary, without it exactly 1 everywhere. The
	// header's own background is what separates the sections instead, and it
	// costs nothing because the cursor can sit on it.
	header := func(s board.Section) {
		addSlot(m.sectionHeader(s.Rule.Name, headerCount(s), slotIdx == m.cursor))
	}
	for _, s := range m.sections() {
		switch s.State {
		case board.Pending:
			header(s)
			for _, row := range s.Rows {
				addSlot(m.renderRow(row, slotIdx == m.cursor, s.Rule.Author))
			}
			// One spinner line, and no placeholder block. The block reserved
			// roughly the section's final height so sections below it were
			// not pushed down as it resolved -- but every line of it was
			// unselectable AND it scaled with the pane, so on a tall board a
			// single keypress moved the viewport several lines. Cold start is
			// every launch, which made that the common case, not an edge one.
			//
			// Layout stability while loading is a real concern and this does
			// give some of it up. The scroll invariant outranks it: a row that
			// moves once as its section resolves is a value changing, which
			// §3.9 already allows, while a board that scrolls five lines per
			// keypress is the defect five earlier attempts were chasing.
			if len(s.Rows) == 0 {
				note(spin)
			}
		case board.Failed:
			header(s)
			note(errorStyle.Render(clip(s.Err.Error(), max(0, m.width-3))))
		case board.Ready:
			header(s)
			if len(s.Rows) == 0 {
				// A resolved empty section collapses to one line: it knows it
				// has nothing, so holding six blank rows would waste most of a
				// short pane.
				note(mutedStyle.Render("—"))
			}
			for _, row := range s.Rows {
				addSlot(m.renderRow(row, slotIdx == m.cursor, s.Rule.Author))
			}
		}
	}
	return lines, slotStarts
}

// scrollOff is how many lines of context are kept beyond the cursor, so moving
// down shows what is coming rather than pinning the cursor to the bottom edge.
// lazygit ships 2 and fzf 3; 2 is enough here to always reveal the first line
// of the next PR while costing little of a 20-row pane.
const scrollOff = 2

// window scrolls the body so the cursor keeps scrollOff rows of context on
// whichever edge it is approaching.
//
// The invariant is that ONE keypress scrolls the board by at most ONE line.
// Five earlier attempts could not hold it, because a row's height depended on
// its data and a section header took a line of its own: a single `j` moved the
// world by 0 to 3 lines depending on what happened to be nearby, which is what
// read as jumping. Now every line on the board is addressable -- gate names
// live in the `d` overlay and the section name lives in its own header row the
// cursor can sit on -- so the clamp below is the whole of it. See
// docs/uniform-rows.md.
//
// Clamping both edges rather than pinning one means the board does not move at
// all while the cursor crosses the middle, which is the conventional behaviour
// (vim's scrolloff, less, fzf).
//
// It also reports the index of the top visible line, which the top row needs to
// name the section of the row you are actually looking at -- not knowable
// before the slice is chosen.
func window(lines []string, cursorRow, height int, rowStarts []int) ([]string, int) {
	if height <= 0 || len(lines) <= height {
		return lines, 0
	}
	if len(rowStarts) == 0 || cursorRow < 0 {
		return lines[:height], 0
	}
	if cursorRow >= len(rowStarts) {
		cursorRow = len(rowStarts) - 1
	}
	cur := rowStarts[cursorRow]

	// The margin has to fit above and below the cursor or the two clamps fight
	// and the viewport oscillates; a very short pane centres instead.
	off := scrollOff
	if 2*off+1 > height {
		off = (height - 1) / 2
	}

	// Two bounds on the top line, each shifting by exactly one when the cursor
	// does. Between them the top is free, so the board holds still through the
	// middle of the viewport and only moves at the edges.
	start := cur - (height - 1 - off)
	if lo := cur - off; start > lo {
		start = lo
	}
	if last := len(lines) - height; start > last {
		start = last
	}
	if start < 0 {
		start = 0
	}
	return lines[start : start+height], start
}
