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

// Index 8 is unsafe as a foreground but safe as a background: in mainstream
// schemes it sits between 0 and 7 in luminance, so it contrasts with the
// terminal's own background whichever end that sits at. No foreground is forced
// on the selected row, so it inherits a colour guaranteed to contrast.
var selBg = lipgloss.Color("8")

// authorPalette colours the author column so the same person is the same
// colour on every row, the way lazygit colours its authors.
//
// These are 256-cube indices rather than the 0-15 the rest of the file sticks
// to, and that is the point: every colour the board already uses carries a
// meaning -- 1 is CI failing, 2 approved, 3 pending, 4 the accent, 6 the
// header -- and an author colour means nothing at all. Landing an author on
// red or green would read as a status. There is no room left in 0-15 for
// eight arbitrary colours once those and their bright variants are out, so
// the palette comes from the cube instead: pale tints in the blue-violet,
// magenta-pink and peach-tan families, none of them in the red or green the
// status colours own.
//
// Every entry was checked against selBg as well as the default background,
// since the selected row keeps its foreground and only gains a background.
// Anything that washed out on colour 8 was dropped.
var authorPalette = []lipgloss.Color{
	"117", // sky
	"189", // pale periwinkle
	"183", // lavender
	"213", // orchid
	"211", // pink
	"216", // peach
	"180", // tan
	"152", // pale teal
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

// authorWidth is deliberately narrow: lazygit, tig and neomutt all collapse to
// initials in their dense views rather than showing a name, and the point of
// this column is to be readable without being loud.
const authorWidth = 3

// initials shortens a GitHub login to fit authorWidth. Collisions are possible
// and tolerable -- the column answers "is this mine or someone else's", and the
// fuzzy filter matches the full login for anything more precise.
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

// clipLeft drops from the front, keeping the tail visible. The filter query
// grows at its end, so that is the end worth keeping on screen.
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
	tw := titleWidth(m.width, t)
	if showAuthor && t == tierFull {
		tw -= authorWidth + 1
	}

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
	b.WriteString(paint(accentStyle).Render(mark))
	b.WriteString(paint(fgStyle).Render(" "))
	b.WriteString(paint(mutedStyle).Render(pad(r.Prefix, 2)))
	b.WriteString(paint(accentStyle).Render(pad("#"+fmt.Sprint(r.PR.Number), 6)))
	b.WriteString(paint(fgStyle).Render(" "))
	b.WriteString(paint(ciStyle).Render(ci))
	if t > tierNarrow {
		b.WriteString(paint(fgStyle).Render(" "))
		b.WriteString(paint(revStyle).Render(rev))
		b.WriteString(paint(fgStyle).Render(" "))
	}
	b.WriteString(paint(blockerStyle).Render(blocker))
	b.WriteString(paint(fgStyle).Render(" "))
	b.WriteString(m.renderTitle(r, titleStyle, paint, tw))
	if t == tierFull {
		if showAuthor {
			b.WriteString(paint(fgStyle).Render(" "))
			b.WriteString(paint(authorStyle(r.PR.Author)).Render(padLeft(initials(r.PR.Author), authorWidth)))
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

// renderTitle draws the title: the conventional-commit prefix coloured by
// part, and the characters the active filter matched underlined.
//
// The two are independent channels on purpose. Hue says what kind of change
// this is, underline says where the query hit, and a query that lands inside a
// scope has to keep both -- losing either one would make filtering and reading
// fight over the same column. Runs are therefore coalesced on the pair
// (part, hit) rather than on hit alone.
//
// Parsing runs on the clipped-and-padded string rather than the raw title, so
// the part boundaries are the ones actually on screen. A narrow terminal that
// cuts through a scope degrades to a half-coloured prefix, which is honest
// about the clipping rather than drifting out of alignment with it.
func (m Model) renderTitle(r board.Row, st lipgloss.Style, paint func(lipgloss.Style) lipgloss.Style, tw int) string {
	text := pad(clip(r.PR.Title, tw), tw)
	hits := matchedTitleIndexes(r, m.query())
	parts := parseTitle(text)
	if len(hits) == 0 && parts == nil {
		return paint(st).Render(text)
	}

	typeWord := leadingType(text, parts)
	styleFor := func(p titlePart, hit bool) lipgloss.Style {
		s := titlePartStyle(st, p, typeWord)
		if hit {
			s = s.Underline(true)
		}
		return paint(s)
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
