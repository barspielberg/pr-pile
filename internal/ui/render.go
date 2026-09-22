package ui

import (
	"github.com/charmbracelet/lipgloss"
	"strings"
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
//
// The name is drawn through hitRuns so a query that matched this section fills
// the runes it hit, exactly as it does in a row's number and author cells. The
// indent and the count are outside that call: the hits index the name alone, so
// the name has to be its own segment for them to land on the right runes.
func (m Model) sectionHeader(name, count string, selected bool) string {
	indent, right, _ := m.headerParts(count)
	hits := m.headerHits(name, count, m.query)
	// Through headerText, not a clip of its own: the drawn name has to be the
	// searched name rune for rune, or a hit index lands on the wrong character.
	name = m.headerText(name, count)
	gap := max(0, m.width-lipgloss.Width(indent)-lipgloss.Width(name)-lipgloss.Width(right))

	// No fill when unselected: the label separates the header on its own (see
	// selBg).
	st, lead := headerStyle, headerStyle.Render(indent)
	paint := func(s lipgloss.Style) lipgloss.Style { return s }
	if selected && indent != "" {
		// Selected: the mark goes in column 0 where nothing else ever draws, so
		// a header carries the same cursor affordance a row does. It replaces
		// the first cell of the indent rather than adding one, or the name
		// would shift right by a column on the row the cursor is on.
		st = headerStyle.Background(selBg)
		paint = func(s lipgloss.Style) lipgloss.Style { return s.Background(selBg) }
		lead = accentStyle.Background(selBg).Render("▌") +
			st.Render(indent[1:])
	}
	return lead + hitRuns(name, hits, headerStyle, paint) +
		st.Render(strings.Repeat(" ", gap)+right)
}

// headerIndent is the header's left margin: the name starts at column 2, PR
// rows at column 4. It is one of the three channels that separate a header from
// a row without a fill.
const headerIndent = "  "

// headerRight is the header's right-hand field, the row count padded off the
// edge. Empty while the rule is still pending, when there is no count to show.
func headerRight(count string) string {
	if count == "" {
		return ""
	}
	return count + "  "
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
