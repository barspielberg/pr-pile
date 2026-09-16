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

// Index 8 is unsafe as a foreground but safe as a background: in mainstream
// schemes it sits between 0 and 7 in luminance, so it contrasts with the
// terminal's own background whichever end that sits at. No foreground is forced
// on the selected row, so it inherits a colour guaranteed to contrast.
var selBg = lipgloss.Color("8")

// Width tiers. Exactly one column flexes (title), so the status cluster stays
// pinned at the same screen offset at every width.
const (
	fixedFull   = 23 // mark+gut+tree+number+gut+status(7)+gut+gut+age
	fixedMid    = 19 // age dropped
	fixedNarrow = 15 // review glyph dropped, status compressed to 3

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
func (m Model) renderRow(r board.Row, selected, showAuthor bool) string {
	t := widthTier(m.width)
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
			b.WriteString(paint(mutedStyle).Render(padLeft(initials(r.PR.Author), authorWidth)))
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

	// Failing gate names are secondary detail: muted, indented under the row,
	// never competing with an actionable title.
	if len(r.PR.FailedGates) > 0 {
		cont := "  "
		if r.Prefix == "╭╴" || r.Prefix == "│ " {
			cont = "│ "
		}
		gates := clip(strings.Join(r.PR.FailedGates, ", "), tw)
		line += "\n  " + mutedStyle.Render(cont) + "       " + mutedStyle.Render("└ "+gates)
	}
	return line
}

// renderTitle draws the title, underlining the characters the active filter
// matched. Underline rather than a colour: the title column already encodes
// draft as muted and the row may sit on the selection fill, so the one free
// channel left is weight, not hue.
//
// The clipped-and-padded string is built first and styled per-rune after, so
// the column is exactly tw cells wide whether or not anything matched.
func (m Model) renderTitle(r board.Row, st lipgloss.Style, paint func(lipgloss.Style) lipgloss.Style, tw int) string {
	text := pad(clip(r.PR.Title, tw), tw)
	hits := matchedTitleIndexes(r, m.query())
	if len(hits) == 0 {
		return paint(st).Render(text)
	}

	hl := paint(st.Underline(true))
	plain := paint(st)
	var b strings.Builder
	// Runs are coalesced so a matched span emits one SGR pair, not one per rune.
	var run strings.Builder
	runHit := false
	flush := func() {
		if run.Len() == 0 {
			return
		}
		if runHit {
			b.WriteString(hl.Render(run.String()))
		} else {
			b.WriteString(plain.Render(run.String()))
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

// renderSectionHeader draws a rule as a full-width band so it can never be
// mistaken for a PR row.
func (m Model) renderSectionHeader(name string, count string) string {
	label := "━━ " + strings.ToUpper(name) + " "
	tail := m.width - lipgloss.Width(label) - lipgloss.Width(count) - 1
	if tail < 0 {
		tail = 0
	}
	return headerStyle.Render(label+strings.Repeat("━", tail)) + " " + mutedStyle.Render(count)
}
