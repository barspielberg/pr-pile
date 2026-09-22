package ui

import (
	"fmt"
	"strings"

	"github.com/barspielberg/pr-pile/internal/board"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) renderRow(r board.Row, selected, showAuthor bool) string {
	picked := m.isSelected(r.PR.Number)
	t := widthTierFor(m.width, showAuthor)
	tw := m.searchTitleWidth(showAuthor)

	// One set of spans feeds every cell: the search ran on the row's own drawn
	// text, so each cell fills the part of the hit that falls inside it, at the
	// rune extents searchText recorded as it laid them down.
	spans := m.matchSpans(r, showAuthor, m.query)
	_, cells := m.searchText(r, showAuthor)

	ci, ciStyle := ciCell(r.PR)
	rev, revStyle := reviewCell(r.PR)
	blocker, blockerStyle := blockerCell(r.PR)

	titleStyle := fgStyle
	if r.PR.IsDraft {
		// A draft is by definition not actionable, so it recedes.
		titleStyle = mutedStyle
	}

	// The background has to be set on every segment rather than wrapped around
	// the finished line: each segment's own style emits a reset, which would
	// terminate an outer background part-way along the row.
	paint := func(st lipgloss.Style) lipgloss.Style {
		if selected {
			return st.Background(selBg)
		}
		return st
	}
	if selected {
		titleStyle = titleStyle.Bold(true)
	}

	// The accent does NOT change on a selected row. It used to brighten to cube
	// 75, justified by a comment claiming "ANSI 4 measures 1.21 against selBg".
	// That figure was computed against a nominal ANSI 4 rather than any real
	// theme's, and it is wrong everywhere it matters. Measured against
	// Catppuccin Mocha's actual palette (ANSI 4 #89b4fa):
	//
	//	                       ANSI 4    cube 75
	//	on old selBg 237        5.40:1    4.91:1
	//	on selBg ANSI 8         3.17:1    2.88:1
	//	unselected, on bg       7.79:1    7.08:1
	//
	// ANSI 4 beats 75 on every fill, so 75 was buying nothing and costing a
	// fixed value that ignored the user's palette.
	//
	// These ratios are THEME-DEPENDENT: both ANSI 4 and ANSI 8 resolve through
	// the user's theme, so the numbers above describe Mocha and not a universal
	// truth. Do not recompute them against a nominal ANSI 4 and "fix" this back
	// to a cube colour -- that is the mistake the old comment enshrined.
	// DESIGN.md §3.4 has the cross-theme table.
	accent := accentStyle

	mark := " "
	if selected {
		mark = "▌"
	}

	// Column 1 was a blank spacer and now carries the multi-select mark. The
	// cursor (column 0) and the selection are separate channels on purpose: a
	// row can be under the cursor, selected, both or neither, and all four have
	// to be tellable apart. Reusing an existing column means nothing shifts and
	// the one-line-per-row invariant is untouched, and a board with nothing
	// selected renders exactly as it did before the feature existed.
	//
	// A glyph rather than a hue, so it survives NO_COLOR and a light theme --
	// the design guide's rule that colour is never the sole carrier.
	pick := " "
	if picked {
		pick = "•"
	}

	var b strings.Builder
	b.WriteString(paint(accent).Render(mark))
	b.WriteString(paint(accent).Render(pick))
	b.WriteString(paint(mutedStyle).Render(pad(r.Prefix, 2)))
	b.WriteString(hitRuns(pad("#"+fmt.Sprint(r.PR.Number), numberWidth),
		cellHits(spans, cells.number), accent, paint))
	b.WriteString(paint(fgStyle).Render(" "))
	b.WriteString(paint(ciStyle).Render(ci))
	if t > tierNarrow {
		b.WriteString(paint(fgStyle).Render(" "))
		b.WriteString(paint(revStyle).Render(rev))
		b.WriteString(paint(fgStyle).Render(" "))
	}
	b.WriteString(paint(blockerStyle).Render(blocker))
	b.WriteString(paint(fgStyle).Render(" "))
	b.WriteString(m.renderTitle(r, titleStyle, paint, tw, cellHits(spans, cells.title)))
	if t == tierFull {
		if showAuthor {
			b.WriteString(paint(fgStyle).Render(" "))
			// Muted like age, and still searchable: hitRuns fills the runes
			// a query matched, so losing the hue did not lose the highlight.
			b.WriteString(hitRuns(padLeft(initials(r.PR.Author), authorWidth),
				cellHits(spans, cells.author), mutedStyle, paint))
		}
		b.WriteString(paint(fgStyle).Render(" "))
		b.WriteString(paint(mutedStyle).Render(padLeft(clip(age(r.PR.UpdatedAt), 3), 3)))
	}

	line := b.String()
	if selected {
		// Fill to the right edge so the selected row reads as one band.
		if gap := m.width - lipgloss.Width(line); gap > 0 {
			line += paint(fgStyle).Render(strings.Repeat(" ", gap))
		}
	}

	// Gate names live in the `d` overlay, not here: a row whose height depends
	// on its data gives the list an uneven scroll rhythm, which is the bug five
	// attempts at the viewport math could not fix. See docs/uniform-rows.md.
	return line
}

// hitRuns renders text in the given style, filling the runes the query hit and
// coalescing adjacent runes of the same hit state into one run.
//
// It splits the *padded* cell rather than styling the value and padding after:
// a fill that ran on under the padding would read as a wider match than it is,
// and on a fixed-width cell it would look like a column of different sizes.
// Both cells it serves must stay exactly their width; the status cluster's
// screen offset depends on it.
//
// A hit run takes hitStyle whole and does not go through paint: the fill is
// what makes it visible, so letting the selection background compose over it
// would undo the point on exactly the row the cursor is on.
//
// renderTitle keeps its own richer loop: it coalesces on (part, hit), a second
// dimension neither of these cells has.
func hitRuns(text string, hits map[int]bool, st lipgloss.Style,
	paint func(lipgloss.Style) lipgloss.Style) string {
	if len(hits) == 0 {
		return paint(st).Render(text)
	}
	var b, run strings.Builder
	runHit := false
	flush := func() {
		if run.Len() == 0 {
			return
		}
		if runHit {
			b.WriteString(hitStyle.Render(run.String()))
		} else {
			b.WriteString(paint(st).Render(run.String()))
		}
		run.Reset()
	}
	for i, ch := range []rune(text) {
		if h := hits[i]; h != runHit {
			flush()
			runHit = h
		}
		run.WriteRune(ch)
	}
	flush()
	return b.String()
}

// renderTitle draws the title: the conventional-commit prefix coloured by
// part, and the characters the query matched filled.
//
// Hue says what kind of change this is; the fill says where the query hit, and
// it wins outright over the hue for the runes it covers. That is deliberate and
// it is a change from the underline this used to draw: a scope and a ticket key
// are faint, a draft title is faint, and an underline under any of them was
// missable -- which is the one thing a search highlight may not be. The hue is
// still there on every rune the query did not hit, which is almost all of them,
// so the column still reads as a conventional-commit title at a glance.
//
// Runs are still coalesced on the pair (part, hit): a hit run is one
// appearance, but the unhit runs on either side of it keep their own parts.
//
// Parsing runs on the clipped-and-padded string rather than the raw title, so
// the part boundaries are the ones actually on screen. A narrow terminal that
// cuts through a scope degrades to a half-coloured prefix, which is honest
// about the clipping rather than drifting out of alignment with it. The hits
// are indexes into that same string -- the search matched it -- so there is
// nothing to clamp and no ellipsis to guard against.
func (m Model) renderTitle(r board.Row, st lipgloss.Style, paint func(lipgloss.Style) lipgloss.Style, tw int, hits map[int]bool) string {
	text := pad(clip(r.PR.Title, tw), tw)
	parts := parseTitle(text)
	if len(hits) == 0 && parts == nil {
		return paint(st).Render(text)
	}

	styleFor := func(p titlePart, hit bool) lipgloss.Style {
		// A hit takes hitStyle whole: neither the part's hue nor the
		// selection background composes over it, or the fill would come out a
		// different colour in each of the four parts and on the cursor row.
		if hit {
			return hitStyle
		}
		return paint(titlePartStyle(st, p))
	}

	var b strings.Builder
	var run strings.Builder
	runPart, runHit := partSubject, false
	flush := func() {
		if run.Len() == 0 {
			return
		}
		b.WriteString(styleFor(runPart, runHit).Render(run.String()))
		run.Reset()
	}
	for i, ch := range []rune(text) {
		p := partSubject
		if i < len(parts) {
			p = parts[i]
		}
		if h := hits[i]; p != runPart || h != runHit {
			flush()
			runPart, runHit = p, h
		}
		run.WriteRune(ch)
	}
	flush()
	return b.String()
}

// renderSectionNote draws a section that has no rows to show -- still loading,
// resolved empty, or failed. It sits under the section's header and occupies
// exactly one line like everything else.
func renderSectionNote(note string) string {
	return "   " + note
}
