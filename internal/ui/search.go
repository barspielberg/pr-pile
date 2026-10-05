package ui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/barspielberg/pr-pile/internal/board"
	"github.com/charmbracelet/lipgloss"
)

// searchCells is where each cell landed in searchText, as [start,end) rune
// indexes. Spans are attributed to a cell by these, so they are measured off
// the very segments searchText concatenated rather than recomputed from the
// layout: a cell's rune count and its display width are two different numbers
// the moment a title carries a CJK or emoji rune, and every consumer of a span
// indexes by rune.
type searchCells struct {
	number, title, author [2]int
}

// searchText is exactly what the row draws, in draw order: the number, the
// title as clipped to this width, and the author initials when the column is
// shown. It is built from the same helpers renderRow uses, so it can never
// drift from what is on screen -- which is the whole contract: every match is
// visible, so every match can be highlighted.
//
// It returns the cell extents alongside the string because it is the only
// place that knows both: the offsets are accumulated as the segments are
// appended, so they cannot disagree with the string they index.
//
// The age cell is deliberately omitted: it is derived from the clock, so a
// match on it would expire without the user typing anything, and a bare digit
// would collide with the PR number on the way to every numeric search.
func (m Model) searchText(r board.Row, showAuthor bool) (string, searchCells) {
	var b strings.Builder
	var cells searchCells

	// Appending through one helper is what keeps the offsets honest: a segment
	// can only enter the string by reporting the span it occupies.
	add := func(s string) [2]int {
		start := utf8.RuneCountInString(b.String())
		b.WriteString(s)
		return [2]int{start, start + utf8.RuneCountInString(s)}
	}

	tw := m.searchTitleWidth(showAuthor)
	cells.number = add(pad("#"+fmt.Sprint(r.PR.Number), numberWidth))
	add(" ")
	title, _ := m.titleText(r.PR)
	cells.title = add(pad(clip(title, tw), tw))
	if m.showsAuthor(showAuthor) {
		add(" ")
		cells.author = add(padLeft(initials(r.PR.Author), authorWidth))
	}
	return b.String(), cells
}

// searchTitleWidth is the title budget, including the reservation the author
// column takes. renderRow draws to the same number by calling this, so the
// searchable text and the drawn text cannot disagree about where the title
// ends.
func (m Model) searchTitleWidth(showAuthor bool) int {
	t := widthTierFor(m.width, showAuthor)
	tw := titleWidth(m.width, t)
	if showAuthor && t == tierFull {
		tw -= authorWidth + 1
	}
	return tw
}

// showsAuthor reports whether the author cell is actually drawn: the rule may
// ask for it and the width tier still refuse it.
func (m Model) showsAuthor(showAuthor bool) bool {
	return showAuthor && widthTierFor(m.width, showAuthor) == tierFull
}

// headerText is the searchable text of a section header: the rule name clipped
// exactly as the header draws it. That clip is what makes a header match
// honest, the same way searchText clips a title -- every match is visible, so
// every match can be highlighted, and a name cut off by a wide count cannot
// match on runes that are not on screen.
//
// The count itself is deliberately omitted, for the reason searchText omits the
// age cell: it is derived state, so a match on it would appear and expire as
// the board refreshes without the user typing anything.
func (m Model) headerText(name, count string) string {
	_, _, w := m.headerParts(count)
	return clip(name, w)
}

// headerParts divides the width between a header's three segments: the indent,
// the right-hand count, and whatever budget is left for the name.
//
// Each segment is clipped against what the ones to its right have already
// taken, so the three can never sum past the width -- not even on a pane
// narrower than the margin itself, which the single "indent+name" clip this
// replaced handled implicitly and segment-wise drawing does not.
//
// sectionHeader draws from this and headerText searches from it, so the
// searchable name and the drawn name cannot disagree about where it ends. That
// agreement is the whole contract: every match is visible, so every match can
// be highlighted.
func (m Model) headerParts(count string) (indent, right string, nameWidth int) {
	right = clip(headerRight(count), m.width)
	indent = clip(headerIndent, m.width-lipgloss.Width(right))
	nameWidth = max(0, m.width-lipgloss.Width(indent)-lipgloss.Width(right))
	return indent, right, nameWidth
}

// rowMatches reports whether a row matches the query. One rule: a
// case-insensitive substring of what the row actually draws at this width.
//
// A whitespace-only query matches nothing. The searchable text carries each
// cell's padding so the offsets line up with the screen, which would otherwise
// make a query of spaces match every row and highlight nothing the user meant.
func (m Model) rowMatches(r board.Row, showAuthor bool, query string) bool {
	txt, _ := m.searchText(r, showAuthor)
	return textMatches(txt, query)
}

// textMatches is the one matching rule, for any drawn line: a
// case-insensitive substring of what is on screen.
func textMatches(text, query string) bool {
	if strings.TrimSpace(query) == "" {
		return false
	}
	return strings.Contains(strings.ToLower(text), strings.ToLower(query))
}

// headerMatches reports whether a section header matches the query, by the
// same rule rowMatches uses on a row and the help page uses on a legend line.
func (m Model) headerMatches(name, count, query string) bool {
	return textMatches(m.headerText(name, count), query)
}

// matchSpans returns every [start,end) rune span of the query within
// searchText, so each cell can highlight the part of it that is on screen.
func (m Model) matchSpans(r board.Row, showAuthor bool, query string) [][2]int {
	txt, _ := m.searchText(r, showAuthor)
	return textSpans(txt, query)
}

// headerHits are the rune indexes of the header name the query hit, ready for
// hitRuns. A header is one cell where a row is three, so the whole name is the
// cell and the projection is over the whole string.
func (m Model) headerHits(name, count, query string) map[int]bool {
	txt := m.headerText(name, count)
	return cellHits(textSpans(txt, query), [2]int{0, utf8.RuneCountInString(txt)})
}

// textSpans is every [start,end) rune span of the query within one drawn
// line. The board projects these onto its cells; the help page highlights the
// line directly.
//
// Byte offsets from strings.Index are converted to rune indexes here, at the
// boundary: every consumer indexes by rune, and the searched text carries both
// non-ASCII titles and clip's own multi-byte ellipsis.
func textSpans(text, query string) [][2]int {
	if strings.TrimSpace(query) == "" {
		return nil
	}
	hay := strings.ToLower(text)
	needle := strings.ToLower(query)

	var spans [][2]int
	for off := 0; ; {
		i := strings.Index(hay[off:], needle)
		if i < 0 {
			return spans
		}
		start := utf8.RuneCountInString(hay[:off+i])
		spans = append(spans, [2]int{start, start + utf8.RuneCountInString(needle)})
		// Overlapping occurrences are all real hits, so advance by one rune
		// rather than by the whole needle.
		_, w := utf8.DecodeRuneInString(hay[off+i:])
		off += i + w
	}
}

// cellHits projects the spans onto one cell, returning the local rune indexes
// it should highlight. Out-of-cell spans fall away, so a cell only ever
// marks runes it actually draws.
func cellHits(spans [][2]int, cell [2]int) map[int]bool {
	if len(spans) == 0 {
		return nil
	}
	hits := map[int]bool{}
	for _, s := range spans {
		for i := s[0]; i < s[1]; i++ {
			if i >= cell[0] && i < cell[1] {
				hits[i-cell[0]] = true
			}
		}
	}
	if len(hits) == 0 {
		return nil
	}
	return hits
}
