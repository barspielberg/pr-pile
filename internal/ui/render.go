package ui

import (
	"fmt"
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

// selBg is the selected-row fill: ANSI 8, so it resolves through the user's
// own theme like every other colour on the board.
//
// It used to be cube 237, chosen because index 8 is nominally "bright black" --
// a mid grey, and mid grey is the worst possible backdrop for foreground
// colours (only 39 of the 216 cube colours clear 3:1 against it). That
// reasoning is sound for a theme where 8 really is mid grey, and wrong for the
// many themes where it is not: Catppuccin Mocha renders 8 as #585b70, a tinted
// dark, on a #1e1e2e background.
//
// Measured in that theme, every foreground on a selected row over ANSI 8:
//
//	title (default fg)        4.62:1
//	type + number (ANSI 4)    3.17:1
//	passing / approved (2)    4.49:1
//	running / required (3)    5.25:1
//	failing / conflict (1)    2.88:1   <- the floor
//	muted author / age       ~2.54:1   <- approximate, see below
//
// The two below 3:1 are the accepted cost. Both clear 2:1, and status is glyph
// shape first with colour as reinforcement (DESIGN.md §3.2), so neither is the
// sole carrier of anything. The muted figure is an estimate: SGR 2 has no
// specified blend ratio, modelled here as ~55% foreground over the fill.
//
// This is theme-dependent by construction, which is the trade. DESIGN.md §3.4
// has the cross-theme table -- it measures well in Mocha and Tokyo Night and
// poorly in Gruvbox and Solarized. Check that table before concluding a
// contrast complaint is a bug in this line.
//
// No foreground is forced on the selected row, so it keeps its own.
var selBg = lipgloss.Color("8")

// hitStyle fills the runes a search matched, the way vim's Search group does --
// a background fill, not an underline.
//
// A fill rather than an underline because a hit has to read the same wherever
// it lands, and this board spends foreground colour everywhere: an underline
// under a faint scope, a dim ticket key or a muted draft title is missable,
// which is the one thing a search highlight may not be.
//
// **It sets BOTH ground and figure, and that is the whole point.** It was
// `Background(5)` alone, which inherits whatever foreground the run already
// had -- and a dark theme's ANSI 5 is a light pink BY DESIGN, so every
// foreground landed light-on-light. Measured in Catppuccin Mocha:
//
//	type / PR number (ANSI 4)   1.38:1
//	title (default fg)          1.06:1
//	muted author                1.03:1
//
// against a fill that was itself 10.74:1 on the board. A loud block with
// invisible contents, which is worse than no highlight: it draws the eye to
// the one place it cannot read. A highlight that inherits its foreground is
// broken by construction, not by palette choice.
//
// Reverse rather than a chosen pair of slots. The fill takes the theme's
// foreground and the text takes its background, so the contrast is whatever
// the theme already guarantees for ordinary text -- by definition its best
// pairing, and correct on light and dark without measuring either. Across
// eight themes it is the only candidate clearing 4.5:1 everywhere (worst 4.75
// on Solarized Dark); every fixed-slot pair collapses to about 2.4:1 on a
// light theme, where ANSI 5's luminance flips.
//
// It also cannot clash with anything. There is no hue to collide with the
// commit type's ANSI 4 (DESIGN.md §3.3.1) because reverse chooses no colour at
// all, so hue and hit stay the independent channels §3.3 requires.
//
// DESIGN.md §2 rejects reverse for the SELECTED ROW, because swapping a whole
// row's foreground into its background destroys the status colour on exactly
// the failing rows that need it. That objection does not reach a hit run, and
// the reason is structural rather than a judgement call: hitRuns is applied to
// three cells only -- number, title and author -- and the status glyphs never
// pass through it, so a hit cannot recolour a status. Those three cells are
// also already giving up their own styling deliberately (see styleFor below: a
// hit takes hitStyle whole), so there is no hue left for reverse to destroy.
//
// On a selected row the hit does not inherit selBg either, so the fill stays
// the theme foreground and reads at 4.62:1 against selBg in Mocha -- the
// highlight is still visible on the cursor line, which is the case most likely
// to break.
var hitStyle = lipgloss.NewStyle().Reverse(true)

// The section header has NO background fill, which is why there is no headerBg.
//
// It had one, and the fill was the problem rather than the fix. A band has to
// contrast with the board ground, which is the terminal's own background and
// therefore unknown, so it was a fixed cube value: 235, then 234. Against
// Catppuccin Mocha's #1e1e2e those measure 1.08:1 and 1.04:1 -- invisible. The
// band was simultaneously too faint to separate anything and the only neutral
// grey on a blue-tinted board, so it read as grafted on.
//
// The label carries the header without it, on three channels that need no
// fill: indentation (the name starts at column 2, PR rows at column 4), weight
// (bold, and nothing else on the board is bold), and hue (headerStyle's ANSI 6,
// the only bold teal on the board, 11.01:1 against Mocha's background).
//
// A selected header still takes selBg, so the cursor is never invisible. That
// also removes the old problem of a header band and a selection fill being two
// near-identical greys -- 234 against 237 was 1.50:1 -- because there is now
// only one fill on the board.

// sectionHeader draws a section's full-width header row: the rule name at the
// left, the row count right-aligned two cells from the edge.
//
// A header is a real entry in the cursor's address space, not a line the
// cursor skips. That is the whole point of this layout: if every line on the
// board is selectable, one keypress moves the cursor one line by construction,
// and the scroll deltas that killed the earlier inline-header attempts cannot
// arise. See docs/uniform-rows.md §4.1.
//
// It is deliberately NOT sticky -- a pinned line was built and reverted
// (DESIGN.md §3.5.1); scrolling with the content is what avoids that.
func (m Model) sectionHeader(name, count string, selected bool) string {
	w := max(0, m.width)
	label := "  " + name
	right := ""
	if count != "" {
		right = count + "  "
	}
	label = clip(label, max(0, w-lipgloss.Width(right)))
	gap := w - lipgloss.Width(label) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}
	body := label + strings.Repeat(" ", gap) + right
	if !selected {
		// No fill: the label separates the header on its own (see selBg).
		return headerStyle.Render(body)
	}
	// Selected: the mark goes in column 0 where nothing else ever draws, so a
	// header carries the same cursor affordance a row does.
	r := []rune(body)
	return accentStyle.Background(selBg).Render("▌") +
		headerStyle.Background(selBg).Render(string(r[1:]))
}

// Width tiers. Exactly one column flexes (title), so the status cluster stays
// pinned at the same screen offset at every width.
const (
	fixedFull   = 23 // mark+gut+tree+number+gut+status(7)+gut+gut+age
	fixedMid    = 19 // age dropped
	fixedNarrow = 15 // review glyph dropped, status compressed to 3

	// The section name moved out of a per-row gutter and into its own header
	// row, so the 10 cells it took are back in the title at every tier and
	// every breakpoint drops by the same 10.
	minWidth    = 40
	narrowUntil = 48
	midUntil    = 60
	fullFrom    = 76
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
//
// The column is muted, like age. It carried a 15-entry hue palette so the same
// person was the same colour on every row; that was a grouping hint costing 15
// fixed cube values that overrode the user's theme, to decorate a fact the
// initials already carried. Colour is a budget (DESIGN-GUIDE.md §1) and who
// wrote a PR does not compete with whether it is broken.
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
func (m Model) renderRow(r board.Row, selected, showAuthor bool) string {
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

	var b strings.Builder
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
