package ui

import (
	"fmt"
	"github.com/barspielberg/pr-pile/internal/config"
	"github.com/charmbracelet/lipgloss"
	"strings"
	"unicode/utf8"
)

// detail.go carries the state block that sits under the check list.

// helpBlock is one titled group of rows. The page is assembled from blocks
// rather than written out as a string so the renderer can window it: the legend
// is longer than a short pane and the reader needs all of it, so it scrolls.
type helpBlock struct {
	title string
	rows  []helpRow
}

// helpRow is one legend entry: the key, what it does, and the key's own style
// where it has one. The glyph rows carry a colour because the colour is what
// they are the legend for -- the CI block's green check explains a green check
// on the board.
//
// The style travels beside the key rather than baked into it. Rendering it
// early and recovering it later would mean stripping the escape codes back off
// to search and highlight the text, and the search has to see plain characters:
// a query of "x" must not match the `m` in an SGR sequence.
type helpRow struct {
	key, desc string
	style     lipgloss.Style
}

// row is a legend row in the page's default style, which is most of them.
func row(key, desc string) helpRow {
	return helpRow{key: key, desc: desc, style: mutedStyle}
}

// glyph is a legend row whose key is a board glyph, shown in the colour it
// carries on the board.
func glyph(key, desc string, st lipgloss.Style) helpRow {
	return helpRow{key: key, desc: desc, style: st}
}

// The glyph key is rendered from the same helpers the rows use, so a legend can
// never drift from what is actually on screen.
func (m Model) helpBlocks() []helpBlock {
	// Aliases are grouped onto the key's own line rather than listed
	// separately: the reader is asking "how do I move", and four rows saying
	// "move" answer it worse than one.
	keys := helpBlock{"KEYS", []helpRow{
		row("j / k", "move ( ↓ ↑ )"),
		row("ctrl+d/u", "half a page down / up"),
		row("pgdn/pgup", "a full page down / up"),
		row("l / h", "next / previous section ( → ← )"),
		row("g / G", "top / bottom ( home / end )"),
		row("enter", "open in browser ( o ); 3+ asks first"),
		row("d", "detail for this PR"),
		row("space", "select this PR"),
		row("v", "select a range"),
		row("esc", "clear the selection"),
		row("y", "copy the url ( every selected one )"),
		row("Y", "copy menu: number, title, url, branch, author, markdown"),
		row("m", "watch, or stop watching ( every selected one )"),
		row("/", "search"),
		row("n / N", "next / previous match"),
		row("r", "reload"),
		row("?", "help, and close it again"),
		row("q", "quit ( esc, ctrl+c )"),
	}}
	for _, a := range m.cfg.Actions {
		if a.Run != "" {
			name := a.Name
			if a.Multi {
				name += " ( whole selection; 3+ asks first )"
			}
			keys.rows = append(keys.rows, row(a.Key, name))
		}
	}

	return []helpBlock{
		keys,
		// This page's own scroll and close keys are on its bottom row, live,
		// so listing them here too would be the one redundancy a legend cannot
		// justify -- it is the only section the reader can already see.
		{"OVERLAYS", []helpRow{
			row("d page", "any key closes it; j k l h close it and move"),
			row("? page", "scrolls and searches; see its bottom row"),
		}},
		{"SEARCH", []helpRow{
			row("/", "search; the board does not move"),
			row("n / N", "next / previous match, wrapping"),
			row("ctrl+n/p", "next / previous while typing ( ctrl+j/k, ↓ ↑ )"),
			row("enter", "keep the query and the highlights"),
			row("esc", "cancel, or clear the highlights from the board"),
			row("backspace", "edit the query"),
			row("text", "matches what you can see: number, title, author"),
			row("", "initials, and a section by its title"),
			row("", "the author cell is 3 letters, so type those three"),
			row("", "this page searches the same way, over its own lines"),
		}},
		{"CI", []helpRow{
			glyph("✓", "passing", okStyle),
			glyph("✗2", "2 checks failing", errorStyle),
			glyph("◐", "running", attentionStyle),
			glyph("⊘", "cancelled, nothing failing", mutedStyle),
			glyph("·", "no checks", mutedStyle),
		}},
		{"REVIEW", []helpRow{
			glyph("✓", "approved", okStyle),
			glyph("✗", "changes requested", errorStyle),
			glyph("○", "review required", attentionStyle),
		}},
		{"BLOCKERS", []helpRow{
			glyph("!", "merge conflicts", errorStyle),
			glyph("~", "draft", mutedStyle),
		}},
		{"ROWS", []helpRow{
			glyph("╭╴│╰╴", "a stack: each PR targets the one above", mutedStyle),
			row("abc", "author initials, on rules with author: true"),
			row("2h", "last updated"),
		}},
		{"WATCH", []helpRow{
			glyph(watchGlyph, "watched", mutedStyle),
			glyph(newsGlyph, "something changed; enter or d clears it", attentionStyle),
		}},
	}
}

// helpSegment is one styled run of a legend line. A line is a list of them
// because the glyph keys carry their own colour -- the ✗ in the CI block is
// the legend for a red ✗ on the board -- while the text around them is muted.
type helpSegment struct {
	text  string
	style lipgloss.Style
}

// helpLine is one line of the page: its segments in draw order, and the plain
// text they spell. The two are built together so the search can match what is
// on screen and the highlight can land on the right runes -- the same contract
// searchText gives the board.
type helpLine struct {
	segs []helpSegment
	text string
}

// helpPage is the whole legend, top to bottom, with the config path as its
// last line.
func (m Model) helpPage() []helpLine {
	var out []helpLine
	line := func(segs ...helpSegment) {
		var b strings.Builder
		for i := range segs {
			segs[i].text = terminalText(segs[i].text)
			b.WriteString(segs[i].text)
		}
		out = append(out, helpLine{segs: segs, text: b.String()})
	}
	for i, blk := range m.helpBlocks() {
		if i > 0 {
			line()
		}
		line(helpSegment{"  " + blk.title, headerStyle})
		for _, r := range blk.rows {
			key, desc := terminalText(r.key), terminalText(r.desc)
			line(
				// The indent is its own segment rather than part of the key's:
				// it is searchable either way, but a glyph key's colour is the
				// legend for that glyph, and stretching it over two leading
				// spaces makes it the legend for the margin as well.
				helpSegment{"  ", mutedStyle},
				helpSegment{key, r.style},
				// pad, not a width-based repeat, because it is what the rest of
				// the board pads with -- a key whose rune count and display
				// width differ must land in the same column here as everywhere
				// else.
				helpSegment{strings.TrimPrefix(pad(key, 10), key), mutedStyle},
				helpSegment{desc, mutedStyle},
			)
		}
	}
	line()
	line(helpSegment{fmt.Sprintf("  config: %s", config.Path()), mutedStyle})
	return out
}

// helpLines is the page as drawn: each line's segments rendered, with the runes
// the query matched filled by the same hitStyle the board uses. It is rendered
// in full and windowed afterwards, so the scroll position is an index into a
// list that does not depend on it.
func (m Model) helpLines() []string {
	page := m.helpPage()
	out := make([]string, len(page))
	// The band belongs to a live search, so it is gated on the query rather
	// than on helpMatch alone. A zero helpMatch is indistinguishable from
	// "line 0 is current", and New() leaves it at Go's zero value -- which put
	// a cursor on KEYS on a page nobody had searched.
	searching := strings.TrimSpace(m.helpQuery) != ""
	for i, l := range page {
		out[i] = l.render(textSpans(l.text, m.helpQuery), searching && i == m.helpMatch, m.width)
	}
	return out
}

// render draws one line, filling the runes the spans cover. Each segment is
// given the slice of the spans that falls inside it, so a match spanning the
// key and its description highlights across both.
//
// The current match takes the board's selected-row fill. Every match is filled
// the same, so the fill alone cannot say which one `n` is on -- the page did
// not change by a single byte between "1 of 4" and "2 of 4", which made the
// count assert a position the page then refused to show. The band is the same
// answer vim reaches for with hl-CurSearch and fzf with `hl+`: the current
// match gets a second channel, not a louder version of the first.
func (l helpLine) render(spans [][2]int, current bool, width int) string {
	paint := keepStyle
	if current {
		paint = func(st lipgloss.Style) lipgloss.Style { return st.Background(selBg) }
	}

	var b strings.Builder
	at := 0
	for _, sg := range l.segs {
		n := utf8.RuneCountInString(sg.text)
		b.WriteString(hitRuns(sg.text, cellHits(spans, [2]int{at, at + n}), sg.style, paint))
		at += n
	}

	line := b.String()
	if current {
		// Fill to the right edge so the band reads as one row, the way the
		// board's selected row does. A legend line is as long as its text, so
		// without this the band stops mid-pane and reads as a ragged stub
		// rather than a cursor.
		if gap := width - lipgloss.Width(line); gap > 0 {
			line += paint(fgStyle).Render(strings.Repeat(" ", gap))
		}
	}
	return line
}

// keepStyle is hitRuns' paint hook for a line the cursor is not on: the segment
// keeps the style it was given. The current match composes selBg over it
// instead, so the legend does have a cursor row now -- it just is not this one.
func keepStyle(st lipgloss.Style) lipgloss.Style { return st }
