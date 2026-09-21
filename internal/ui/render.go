package ui

import (
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	"github.com/barspielberg/prs-mng/internal/board"
	"github.com/barspielberg/prs-mng/internal/github"
	"github.com/charmbracelet/lipgloss"
)

// Semantic tokens, all ANSI 0-15 so they resolve through the user's own
// terminal theme rather than fixed RGB.
//
// muted is Faint on the default foreground, not index 8: "bright black" is a
// light grey on light themes and can approach invisibility there, while Faint
// dims whatever the theme's foreground already is and so is correct in both
// directions.
var (
	fgStyle        = lipgloss.NewStyle()
	mutedStyle     = lipgloss.NewStyle().Faint(true)
	errorStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	attentionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	okStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	headerStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
	accentStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("4"))
)

// A dark fill rather than index 8's mid grey. Mid grey is the worst possible
// backdrop for foreground colours -- only 39 of the 216 cube colours clear 3:1
// against it -- and it was the real reason the author palette kept collapsing
// to a handful of entries. 237 is dark enough that colour choice is free again.
// No foreground is forced on the selected row, so it keeps its own.
var selBg = lipgloss.Color("237")

// hitStyle fills the runes a search matched, the way vim's Search group does
// (`ctermfg=0 ctermbg=14` -- a background fill, not an underline).
//
// A fill rather than an underline because a hit has to read the same wherever
// it lands, and this board spends foreground colour everywhere: an underline
// under a faint scope, a dim ticket key or a muted draft title is missable,
// which is the one thing a search highlight may not be. A fill wins over
// whatever the cell was already saying, so the hit stops depending on the row
// underneath it.
//
// ANSI 5 rather than a fixed cube shade. The cube is for a contrast floor the
// themed space cannot meet -- that is what buys selBg and the selected accent
// their exemption -- and a search hit has no such floor. A themed value is
// also correct on a light terminal by construction rather than by measurement,
// which is the whole reason the author palette and the commit-type tints came
// back out of the cube.
//
// 5 because it is the one semantic slot a row does not already use: 1 failing,
// 2 passing, 3 pending, 4 the number and the type, 6 the section gutter. A hit
// can therefore never be read as a status, which is the collision that matters
// -- the type tint sits in 4 precisely so `fix` never looks like a failure, and
// the same argument applies here.
//
// No foreground is set. The text keeps the terminal's own default, which is
// guaranteed to contrast with the terminal's background; forcing one would
// re-introduce the light-theme question a themed value exists to avoid, and
// 0/7/15 are not available as foregrounds anyway. It is applied standalone
// rather than layered, so a faint scope cannot leak its dimming onto the fill.
var hitStyle = lipgloss.NewStyle().Background(lipgloss.Color("5"))

// authorPalette colours the author column so the same person is the same
// colour on every row, the way lazygit colours its authors.
//
// 256-cube indices rather than the 0-15 the rest of the file uses, chosen for
// distinctness from each other: the binding constraint used to be selBg, not
// the hues, and now that the selected row is dark every entry clears 4.6:1 on
// it. The only rule left is to avoid the exact ANSI 1 and 2 slots, so an author
// is never literally the failing-red or approved-green pixel; sharing a family
// with them is fine, since the author cell is its own column and status has its
// own glyph.
//
// The length is load-bearing, and only the length: buckets are FNV-1a mod len,
// so the count alone decides who shares a colour, not which colours are in the
// list. 30-odd authors over 15 entries means sharing regardless -- colour is a
// grouping hint and the initials stay the identity -- but 15 spreads the real
// set better than 16, which piles six people onto one entry. Re-run the check
// over the live logins before changing the count.
var authorPalette = []lipgloss.Color{
	"44",  // teal
	"50",  // aqua
	"78",  // spring green
	"108", // sage
	"118", // lime
	"147", // periwinkle
	"148", // olive
	"153", // pale sky
	"178", // gold
	"186", // khaki
	"208", // orange
	"213", // orchid
	"217", // salmon
	"226", // yellow
	"229", // cream
}

// authorStyle maps a login to its palette entry. FNV-1a keeps it stable across
// runs and machines -- a map's iteration order or anything seeded would repaint
// people on every launch, which defeats the point of recognising them by colour.
func authorStyle(login string) lipgloss.Style {
	if login == "" {
		return mutedStyle
	}
	h := fnv.New32a()
	h.Write([]byte(login))
	return lipgloss.NewStyle().Foreground(authorPalette[h.Sum32()%uint32(len(authorPalette))])
}

// sectionWidth is the left gutter carrying the section name. Sections are a
// gutter rather than a header band so that every line on the board is a row:
// a header line makes one keypress scroll two lines whenever it crosses the
// top edge, which is the jump five attempts at the viewport math could not fix.
// See docs/uniform-rows.md.
//
// 8 cells fits MINE, REVIEW and ALL OPEN, and clips longer rule names the same
// way the title column clips.
const sectionWidth = 8

// gutterState is a row's relationship to its section, which the gutter and the
// rule glyph both read. It is three states rather than a bool because "the
// section starts here" and "the section started above the window" are different
// facts: the first draws the rule broken, the second draws it continuing and
// still names the section, so the top visible row is never anonymous.
type gutterState int

const (
	sectionContinues gutterState = iota
	sectionStarts
	sectionAbove
)

// sectionGutter draws the gutter: the name on a section's first visible row,
// blank on the rest.
func sectionGutter(name string, st gutterState) string {
	label := ""
	if st != sectionContinues {
		label = strings.ToUpper(name)
	}
	return pad(clip(label, sectionWidth), sectionWidth)
}

// sectionRule draws the one-cell rule beside the gutter. `╷` at a section's
// first row makes the rule visibly start rather than continue, so a boundary
// reads even when the name is clipped to all 8 cells and cannot signal it.
func sectionRule(st gutterState) string {
	if st == sectionStarts {
		return " ╷"
	}
	return " │"
}

// Width tiers. Exactly one column flexes (title), so the status cluster stays
// pinned at the same screen offset at every width.
const (
	fixedFull   = 23 + sectionWidth + 2 // mark+gut+tree+number+gut+status(7)+gut+gut+age
	fixedMid    = 19 + sectionWidth + 2 // age dropped
	fixedNarrow = 15 + sectionWidth + 2 // review glyph dropped, status compressed to 3

	// Each tier starts sectionWidth+2 later than it did before the gutter, so
	// the title keeps the same readable floor at every tier.
	minWidth    = 40 + sectionWidth + 2
	narrowUntil = 48 + sectionWidth + 2
	midUntil    = 60 + sectionWidth + 2
	fullFrom    = 76 + sectionWidth + 2
)

type tier int

const (
	tierNarrow tier = iota
	tierMid
	tierFull
)

// An author column costs 4 more cells, so FULL has to start 4 columns later
// for a rule that shows one -- otherwise the title drops below the readable
// floor the breakpoint exists to guarantee.
func widthTierFor(w int, showAuthor bool) tier {
	if showAuthor && w < fullFrom+authorWidth+1 {
		if w >= midUntil {
			return tierMid
		}
		return tierNarrow
	}
	return widthTier(w)
}

func widthTier(w int) tier {
	switch {
	case w >= fullFrom:
		return tierFull
	case w >= midUntil:
		return tierMid
	default:
		return tierNarrow
	}
}

func titleWidth(w int, t tier) int {
	var fixed int
	switch t {
	case tierFull:
		fixed = fixedFull
	case tierMid:
		fixed = fixedMid
	default:
		fixed = fixedNarrow
	}
	if n := w - fixed; n > 0 {
		return n
	}
	return 0
}

// ciCell returns the 2-cell CI glyph plus count. Four distinct silhouettes so
// the state survives losing colour.
func ciCell(pr github.PR) (string, lipgloss.Style) {
	switch pr.CIState {
	case "SUCCESS":
		return "✓ ", okStyle
	case "FAILURE", "ERROR":
		n := len(pr.FailedGates)
		switch {
		case n == 0:
			return "✗ ", errorStyle
		case n >= 10:
			return "✗+", errorStyle
		default:
			return fmt.Sprintf("✗%d", n), errorStyle
		}
	case "PENDING", "EXPECTED":
		return "◐ ", attentionStyle
	default:
		return "· ", mutedStyle
	}
}

func reviewCell(pr github.PR) (string, lipgloss.Style) {
	switch pr.Review {
	case "APPROVED":
		return "✓", okStyle
	case "CHANGES_REQUESTED":
		return "✗", errorStyle
	case "REVIEW_REQUIRED":
		return "○", attentionStyle
	default:
		return "·", mutedStyle
	}
}

// One cell for both booleans: a conflicted draft is primarily conflicted, since
// the conflict is the thing that will bite. Both glyphs are ASCII so no font can
// break this slot.
func blockerCell(pr github.PR) (string, lipgloss.Style) {
	switch {
	case pr.Mergeable == "CONFLICTING":
		return "!", errorStyle
	case pr.IsDraft:
		return "~", mutedStyle
	default:
		return " ", fgStyle
	}
}

// numberWidth fits "#99999" and is named because the search has to reproduce
// the cell exactly to attribute a match back to it.
const numberWidth = 6

// authorWidth is deliberately narrow: lazygit, tig and neomutt all collapse to
// initials in their dense views rather than showing a name, and the point of
// this column is to be readable without being loud.
const authorWidth = 3

// initials shortens a GitHub login to fit authorWidth. Collisions are possible
// and tolerable -- the column answers "is this mine or someone else's".
//
// These three characters are also the whole of what an author search matches,
// since they are the whole of what is drawn: two logins sharing a prefix share
// a cell and so share a match, which the row shows rather than hides.
func initials(login string) string {
	if login == "" {
		return ""
	}
	r := []rune(strings.ToLower(login))
	if len(r) > authorWidth {
		r = r[:authorWidth]
	}
	return string(r)
}

// age renders the largest single unit, right-aligned so digits line up. The
// column is budgeted at 3 cells, so a week count that needs more than two
// digits saturates rather than widening the row and pushing it past the frame.
func age(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return "now"
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	default:
		if w := int(d.Hours() / (24 * 7)); w < 100 {
			return fmt.Sprintf("%dw", w)
		}
		return "99+"
	}
}

func clip(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	if w <= 3 {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		if lipgloss.Width(b.String()+string(r)) > w-1 {
			break
		}
		b.WriteRune(r)
	}
	return b.String() + "…"
}

// clipLeft drops from the front, keeping the tail visible. The query grows at
// its end, so that is the end worth keeping on screen.
func clipLeft(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	r := []rune(s)
	for i := range r {
		if lipgloss.Width(string(r[i:])) <= w {
			return string(r[i:])
		}
	}
	return ""
}

func pad(s string, w int) string {
	if d := w - lipgloss.Width(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}

func padLeft(s string, w int) string {
	if d := w - lipgloss.Width(s); d > 0 {
		return strings.Repeat(" ", d) + s
	}
	return s
}

// renderRow draws one PR. Every sub-slot is always emitted, blank when absent,
// so no field ever shifts as state changes.
func (m Model) renderRow(r board.Row, selected, showAuthor bool, section string, gs gutterState) string {
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

	// ANSI 4 measures 1.21 against selBg, so on the selected row the cursor bar
	// and the PR number would be the least readable things on it. 75 rather
	// than bright-blue 12 because 4 and 12 are themeable slots with no fixed
	// relationship -- Catppuccin, Tokyo Night and Dracula all define 12 as 4,
	// so brightening to it would change nothing there. 75 is a fixed cube
	// colour: 4.91 on selBg, and outside the author palette.
	accent := accentStyle
	if selected {
		accent = accent.Foreground(lipgloss.Color("75"))
	}

	mark := " "
	if selected {
		mark = "▌"
	}

	var b strings.Builder
	// The section gutter sits outside the selection fill: it belongs to the
	// board, not to the row, and highlighting it would make the band look like
	// part of the selected PR.
	b.WriteString(headerStyle.Render(sectionGutter(section, gs)))
	b.WriteString(mutedStyle.Render(sectionRule(gs)))
	b.WriteString(paint(accent).Render(mark))
	b.WriteString(paint(fgStyle).Render(" "))
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
			b.WriteString(hitRuns(padLeft(initials(r.PR.Author), authorWidth),
				cellHits(spans, cells.author), authorStyle(r.PR.Author), paint))
		}
		b.WriteString(paint(fgStyle).Render(" "))
		b.WriteString(paint(mutedStyle).Render(padLeft(clip(age(r.PR.UpdatedAt), 3), 3)))
	}

	line := b.String()
	if selected {
		// Fill to the right edge so the selected row reads as one band.
		if gap := m.width - lipgloss.Width(stripSGR(line)); gap > 0 {
			line += paint(fgStyle).Render(strings.Repeat(" ", gap))
		}
	}

	// Gate names live in the `c` overlay, not here: a row whose height depends
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

	typeWord := leadingType(text, parts)
	styleFor := func(p titlePart, hit bool) lipgloss.Style {
		// A hit takes hitStyle whole: neither the part's hue nor the
		// selection background composes over it, or the fill would come out a
		// different colour in each of the four parts and on the cursor row.
		if hit {
			return hitStyle
		}
		return paint(titlePartStyle(st, p, typeWord))
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

// stripSGR measures a styled string's display width by removing SGR sequences.
func stripSGR(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		switch {
		case r == 0x1b:
			inEsc = true
		case inEsc && r == 'm':
			inEsc = false
		case !inEsc:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// renderSectionNote draws a section that has no rows to show -- still loading,
// resolved empty, or failed. It reuses the row's gutter so the board keeps one
// left edge, and occupies exactly one line like everything else.
func (m Model) renderSectionNote(name, note string) string {
	return headerStyle.Render(sectionGutter(name, sectionStarts)) +
		mutedStyle.Render(sectionRule(sectionStarts)+" ") + note
}
