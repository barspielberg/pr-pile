package ui

import (
	"fmt"
	"github.com/charmbracelet/lipgloss"
	"strings"
)

// The footer carries the repo and the spinner, so no global header row is
// needed: in a 20-row pane every chrome row costs a PR.
// promptLine is the search's own row, drawn directly above the footer. The
// position sits on the right where the footer already puts its right-hand
// field, so the two chrome rows share one alignment.
//
// The board cannot change to say a query found nothing, so the query text
// itself turns red -- vim's own answer, and the only affordance left when
// nothing on screen is allowed to move. It reverts the moment a match exists,
// which is what makes backspacing back to a match legible.
func (m Model) promptLine() string {
	matches := m.matchIndexes()
	at := -1
	for i, idx := range matches {
		if idx == m.cursor {
			at = i
			break
		}
	}
	return m.renderPrompt(m.query, len(matches), at)
}

// renderPrompt draws the prompt row for any searchable page: the query, and
// where the cursor sits in the match set. at is the cursor's index into the
// matches, or -1 when it is not on one.
func (m Model) renderPrompt(query string, total, at int) string {
	right := ""
	queryStyle := fgStyle
	switch {
	case strings.TrimSpace(query) == "":
	case total == 0:
		right, queryStyle = "no matches", errorStyle
	case at >= 0:
		right = fmt.Sprintf("%d of %d", at+1, total)
	case total == 1:
		right = "1 match"
	default:
		right = fmt.Sprintf("%d matches", total)
	}

	const prefix = "  / "
	// The query keeps the tail rather than the head: while typing, the end of
	// what you just entered is the part you are looking at.
	field := query + "▏"
	budget := m.width - lipgloss.Width(prefix) - lipgloss.Width(right) - 2
	if budget < 1 {
		// No honest room for the position at this width, so drop it.
		return accentStyle.Render(prefix) +
			queryStyle.Render(clipLeft(field, max(0, m.width-lipgloss.Width(prefix))))
	}
	field = clipLeft(field, budget)

	gap := budget - lipgloss.Width(field)
	return accentStyle.Render(prefix) + queryStyle.Render(field) +
		strings.Repeat(" ", gap) + mutedStyle.Render(right+"  ")
}

// cursorSection names the section the cursor is in and where it sits within it.
// It is bound to the cursor, not to the top visible row: a board that fits the
// pane never scrolls, so a top-row-bound field is frozen at its first section
// forever -- which is exactly how the reverted sticky line failed. See
// docs/section-layout.md §14.
//
// Read off the slot the cursor is actually on, not recomputed by walking the
// sections: the section a slot belongs to is recorded on the slot, and a second
// independent walk is what put the footer one section out when the note slot
// was added.
//
// pos is 0 on a header or a note -- the cursor is at the section rather than at
// a row inside it, and claiming "1 of 12" there would be a position it does not
// have.
func (m Model) cursorSection() (name string, pos, total int) {
	cur, ok := m.slotAt(m.cursor)
	if !ok {
		return "", 0, 0
	}
	for _, s := range m.sections() {
		if s.Rule.Name != cur.section {
			continue
		}
		if !cur.isRow() {
			return s.Rule.Name, 0, len(s.Rows)
		}
		// rowIdx counts across the whole board, so offset by the rows in the
		// sections above this one.
		before := 0
		for _, up := range m.sections() {
			if up.Rule.Name == s.Rule.Name {
				break
			}
			before += len(up.Rows)
		}
		return s.Rule.Name, cur.rowIdx - before + 1, len(s.Rows)
	}
	return "", 0, 0
}

func (m Model) footer(spin string) string {
	left := "  j/k move · l/h section · enter open · d detail · y copy · / search · ? help · q quit"
	switch {
	case m.ranging || len(m.selection) > 0:
		// A selection is a mode the board is holding, so the footer has to say
		// so and say how to leave. The count is the part that matters: it is
		// the only confirmation that `v` picked up what the user thinks it did
		// before they press `y`.
		left = fmt.Sprintf("  space mark · v range · y copy %d · esc clear", len(m.selection))
	case m.searching:
		left = "  ctrl+n/p next · enter keep · esc cancel"
	case m.query != "":
		// The query outlives the prompt, so the legend has to say what the two
		// keys that only work now actually do.
		left = "  j/k move · l/h section · enter open · d detail · y copy · n/N next match · esc clear"
	}
	if m.status != "" {
		left = "  " + m.status
	}
	// The confirm outranks the status and the selection legend: it is a
	// question the board is waiting on, and nothing else on the line matters
	// until it is answered.
	if m.confirmOpen > 0 {
		left = fmt.Sprintf("  open %d PRs in the browser?  y / enter to confirm · any other key cancels", m.confirmOpen)
	}
	// A running action outranks a status: the status line is history and this
	// is happening now. The glyph is the board's own spinner rather than a
	// static marker -- the tick is kept alive for the duration (see the
	// spinMsg case), so it animates, and an animated glyph is the difference
	// between "working" and "wedged" on a command that takes seconds.
	if m.running != "" {
		left = "  " + string(spinFrames[m.spinner%len(spinFrames)]) + " " + m.running
	}
	// One line, clipped not wrapped: a second row would break the board's
	// one-line-per-row invariant, and stderr from a failing script is
	// arbitrarily long.
	left = clip(left, max(0, m.width-2))
	// The section name takes the right field and the repo yields it: the repo
	// is a constant the user chose and can read in the window title, while the
	// section changes under every keypress. A header scrolls away with its
	// section, so this is the only place the cursor's section is named once you
	// are past the top of it.
	right := m.cfg.Repo
	if name, pos, total := m.cursorSection(); name != "" {
		if pos == 0 {
			// On the header: the section's size, with no false position.
			right = fmt.Sprintf("%s · %d", strings.ToUpper(name), total)
		} else {
			right = fmt.Sprintf("%s · %d of %d", strings.ToUpper(name), pos, total)
		}
	}
	right = terminalText(right)
	if spin != "" {
		right += " " + spin
	}
	// When the two fields do not both fit, the keys clip and the right field
	// survives whole: the keys are a reminder of things the user already knows,
	// while the section name is the only place the full name and the count are
	// on screen at all. Below that the right field goes too, rather than being
	// clipped into a half-truth like `NEEDS MY REVI`.
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 1 {
		keys := m.width - lipgloss.Width(right) - 3
		if keys < 8 {
			return mutedStyle.Render(clip(left, m.width))
		}
		return mutedStyle.Render(clip(left, keys) + " " + right + "  ")
	}
	return mutedStyle.Render(left + strings.Repeat(" ", gap) + right + "  ")
}

func (m Model) View() string {
	if m.width > 0 && m.width < minWidth {
		return mutedStyle.Render(fmt.Sprintf("  terminal too narrow\n  (need %d cols)", minWidth))
	}

	if m.showHelp {
		return m.helpOverlay()
	}
	if m.showChecks {
		return m.detailOverlay()
	}

	spin := ""
	if m.fetching {
		spin = string(spinFrames[m.spinner%len(spinFrames)])
	}

	foot := m.footer(spin)
	chrome := 1
	if m.searching {
		// The prompt is a second chrome row, so the body has one line less.
		foot = m.promptLine() + "\n" + foot
		chrome = 2
	}

	lines, slotStarts := m.body(spin)
	if m.height > 0 {
		avail := m.height - chrome
		lines, _ = window(lines, m.cursor, avail, slotStarts)
		// Pad to the full height so the prompt and footer stay pinned to the
		// bottom edge instead of floating under a short result set.
		for len(lines) < avail {
			lines = append(lines, "")
		}
	}
	return strings.Join(lines, "\n") + "\n" + foot
}

// tailWriter keeps the last `limit` bytes written to it. A command's stderr is
// unbounded and only its end is wanted, so this drops from the front rather
// than refusing to record once full.
