package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/barspielberg/pr-pile/internal/board"
	"github.com/barspielberg/pr-pile/internal/github"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// shape renders a parse as one character per rune so a case reads as the
// alignment between the title and its parts.
func shape(s string) string {
	parts := parseTitle(s)
	if parts == nil {
		return ""
	}
	sym := map[titlePart]byte{partSubject: '.', partType: 'T', partScope: 'S', partTicket: 'J'}
	out := make([]byte, len(parts))
	for i, p := range parts {
		out[i] = sym[p]
	}
	return string(out)
}

// Real titles sampled off the board, including the free-form ones. A parse
// that is wrong here is wrong on every row the user actually sees.
func TestParseTitleOnRealTitles(t *testing.T) {
	cases := []struct{ title, want string }{
		{"fix(control-center): AF-12632 re-sync task status from the reloaded task",
			"TTTSSSSSSSSSSSSSSSSS.JJJJJJJJ..........................................."},
		{"refactor(ordering): AF-12418 drop transferPlate from the trade-in snapshot",
			"TTTTTTTTSSSSSSSSSSS.JJJJJJJJ.............................................."},
		{"fix(ordering,ordering-ms): AF-13043 CLO wizard Step 2 dependency modal",
			"TTTSSSSSSSSSSSSSSSSSSSSSSS.JJJJJJJJ..................................."},
		{"feat(ordering-ms,schemas): AF-12880 build the XLR8 order-submit payload",
			"TTTTSSSSSSSSSSSSSSSSSSSSSS.JJJJJJJJ...................................."},
		{"ci: bump the github-actions group with 2 updates",
			"TTS............................................."},
		{"chore: bump @types/send from 0.17.4 to 1.2.1",
			"TTTTTS......................................"},
		{"perf(control-center): halve lint-strict with eslint --concurrency 4",
			"TTTTSSSSSSSSSSSSSSSSS.............................................."},
		{"fix(sadot): DASC-1391 preserve custom field filter types",
			"TTTSSSSSSSS.JJJJJJJJJ..................................."},

		// A Jira key can be the whole prefix, with or without a separator.
		{"AF-12872 Order plan activity logs", "JJJJJJJJ........................."},
		{"AF-12835: raw ride export async when flag on", "JJJJJJJJJ..................................."},
		{"AF-12776 | Vehicle form breakdown picker sends contextId",
			"JJJJJJJJJJ.............................................."},

		// No prefix at all: "" means parseTitle returned nil and the renderer
		// takes its untouched path.
		{"Use mobile as brand name", ""},
		{"Fix location widget", ""},
	}
	for _, c := range cases {
		if got := shape(c.title); got != c.want {
			t.Errorf("%q\n got %q\nwant %q", c.title, got, c.want)
		}
	}
}

// The colon is the whole rule. Without it a title is prose that happens to
// open with a word we recognise, and colouring it would repaint the board.
func TestParseTitleRequiresTheColon(t *testing.T) {
	for _, s := range []string{
		"Fix location widget",
		"fix location widget",
		"feat something without punctuation",
		"fix(ordering) missing the colon",
		"refactor the ordering module",
		"perf improvements",
	} {
		if parseTitle(s) != nil {
			t.Errorf("%q has no colon and must not be styled: %q", s, shape(s))
		}
	}
}

// A word that is not a conventional type stays prose even with a colon, or
// every "Note:" and "WIP: whatever" would pick up a hue.
func TestParseTitleRejectsUnknownTypes(t *testing.T) {
	for _, s := range []string{
		"note: something",
		"wip: halfway there",
		"Fix: capitalised is not the convention",
		": leading colon",
		"(ordering): scope with no type",
	} {
		if parseTitle(s) != nil {
			t.Errorf("%q is not a conventional prefix: %q", s, shape(s))
		}
	}
}

// Clipping cuts wherever the width lands, including mid-scope and mid-key.
// Every offset must parse without panicking and without labelling past the end.
func TestParseTitleSurvivesClippingAtEveryOffset(t *testing.T) {
	full := "fix(ordering,ordering-ms): AF-13043 CLO wizard Step 2 dependency modal"
	for w := 0; w <= len(full)+5; w++ {
		text := pad(clip(full, w), w)
		parts := parseTitle(text)
		if parts != nil && len(parts) != len([]rune(text)) {
			t.Fatalf("w=%d: %d parts for %d runes", w, len(parts), len([]rune(text)))
		}
	}
}

// A title with no conventional prefix must render exactly as it did before
// prefix colouring existed -- one SGR pair around the whole padded column,
// nothing dimmed and nothing indented.
func TestTitlesWithoutAPrefixRenderUntouched(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	for _, title := range []string{"Use mobile as brand name", "Fix location widget"} {
		m := loaded(t, 100, 20, []github.PR{{
			Number: 3248, Title: title, CIState: "SUCCESS", UpdatedAt: time.Now(),
		}}, nil)
		m.cursor = -1
		r := board.Row{PR: m.board.Sections()[0].Rows[0].PR}

		got := m.renderTitle(r, fgStyle, func(s lipgloss.Style) lipgloss.Style { return s }, 40, nil)
		want := fgStyle.Render(pad(clip(title, 40), 40))
		if got != want {
			t.Errorf("%q rendered differently:\n got %q\nwant %q", title, got, want)
		}
	}
}

// The fill wins over the part hue on the runes it covers, and the hue is
// untouched everywhere else. Both halves matter: the first is what makes a hit
// readable inside a faint scope, and the second is what keeps the column
// readable as a conventional-commit title while a query is live.
func TestSearchFillWinsOverPrefixColour(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	title := "fix(ordering): AF-13043 wizard dependency modal"
	mine := []github.PR{{
		Number: 3248, Title: title, CIState: "SUCCESS", UpdatedAt: time.Now(),
	}}
	fill := hitFill()

	// "ordering" is entirely inside the scope, which is the faint part -- the
	// case an underline used to be missable in.
	m := typeQuery(loaded(t, 120, 20, mine, nil), "ordering")
	rows := m.board.Sections()[0].Rows
	if len(rows) == 0 {
		t.Fatal("query should have matched the row")
	}
	scopeRun := renderedTitle(m, rows[0])

	if got := stripANSI(scopeRun); !strings.Contains(got, title) {
		t.Fatalf("title did not render intact: %q", got)
	}
	seg := segmentAround(t, scopeRun, "ordering")
	if !strings.Contains(seg, fill) {
		t.Errorf("the matched scope is not filled: %q", seg)
	}
	if hasSGRParam(seg, "2") {
		t.Errorf("the fill kept the scope's faint under it: %q", seg)
	}

	// The type keeps its own colour when the match is elsewhere, so the hue is
	// only overridden where the query actually landed.
	m2 := typeQuery(loaded(t, 120, 20, mine, nil), "wizard")
	rows2 := m2.board.Sections()[0].Rows
	subjRun := renderedTitle(m2, rows2[0])

	fixSeg := segmentAround(t, subjRun, "fix")
	if !hasSGRParam(fixSeg, "34") {
		t.Errorf("type lost its colour while a query was active: %q", fixSeg)
	}
	if strings.Contains(fixSeg, fill) {
		t.Errorf("an unmatched part was filled: %q", fixSeg)
	}
	wizSeg := segmentAround(t, subjRun, "wizard")
	if !strings.Contains(wizSeg, fill) {
		t.Errorf("the matched subject is not filled: %q", wizSeg)
	}

	// Searching must not change the column's width.
	if want := m.searchTitleWidth(false); lipgloss.Width(stripANSI(scopeRun)) != want {
		t.Errorf("searched title is %d cells, want %d",
			lipgloss.Width(stripANSI(scopeRun)), want)
	}
}

// renderedTitle draws the title cell the way renderRow does, at the model's
// own width and with the row's own hits, so a test never has to keep the two
// in step by hand.
func renderedTitle(m Model, r board.Row) string {
	_, cells := m.searchText(r, false)
	tw := m.searchTitleWidth(false)
	return m.renderTitle(r, fgStyle, func(s lipgloss.Style) lipgloss.Style { return s }, tw,
		cellHits(m.matchSpans(r, false, m.query), cells.title))
}

// runAt returns the SGR-introduced run covering the rune at index i of the
// title, so a test names a position rather than a character that may recur.
func runAt(t *testing.T, rendered string, i int) string {
	t.Helper()
	if i < 0 {
		t.Fatal("index not found in the title")
	}
	at := 0
	for _, seg := range strings.SplitAfter(rendered, "\x1b[0m") {
		n := len([]rune(stripANSI(seg)))
		if at+n > i {
			return seg
		}
		at += n
	}
	t.Fatalf("index %d is past the end of %q", i, rendered)
	return ""
}

// hasSGRParam reports whether a rendered run sets one SGR parameter, matched
// as a whole field so "4" does not answer for the 4 inside "38;5;173".
func hasSGRParam(seg, param string) bool {
	i := strings.Index(seg, "\x1b[")
	if i < 0 {
		return false
	}
	j := strings.Index(seg[i:], "m")
	if j < 0 {
		return false
	}
	for _, f := range strings.Split(seg[i+2:i+j], ";") {
		if f == param {
			return true
		}
	}
	return false
}

// segmentAround returns the SGR-introduced run containing lit, so a test can
// assert on the parameters that actually apply to those characters.
func segmentAround(t *testing.T, rendered, lit string) string {
	t.Helper()
	for _, seg := range strings.Split(rendered, "\x1b[0m") {
		if strings.Contains(stripANSI(seg), lit) {
			return seg
		}
	}
	t.Fatalf("%q not found in any run of %q", lit, rendered)
	return ""
}

// Every recognised type takes the same hue. The eleven types used to carry
// five cube values between them, which put the board's largest block of fixed
// colour on the field that says least -- the word itself already distinguishes
// feat from fix. Asserting the rendered runs are byte-identical is what stops a
// per-type map growing back one exception at a time.
func TestEveryTypeSharesOneColour(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	seg := func(word string) string {
		t.Helper()
		m := loaded(t, 120, 20, []github.PR{{
			Number: 3248, Title: word + "(ordering): do the thing",
			CIState: "SUCCESS", UpdatedAt: time.Now(),
		}}, nil)
		m.cursor = -1
		r := m.board.Sections()[0].Rows[0]
		out := m.renderTitle(r, fgStyle, func(s lipgloss.Style) lipgloss.Style { return s }, 60, nil)
		return segmentAround(t, out, word)
	}

	types := []string{"fix", "feat", "refactor", "perf", "revert",
		"chore", "ci", "build", "docs", "style", "test"}

	// A themed ANSI slot, not a cube value: the type colour has to resolve
	// through the user's own theme like the status columns do.
	first := seg(types[0])
	if !hasSGRParam(first, "34") {
		t.Fatalf("type is not ANSI 4: %q", first)
	}
	if strings.Contains(first, "38;5;") {
		t.Fatalf("type still carries a 256-cube value: %q", first)
	}

	// Same SGR introducer for every type; only the letters differ.
	norm := func(seg, word string) string { return strings.ReplaceAll(seg, word, "") }
	want := norm(first, types[0])
	for _, w := range types[1:] {
		if got := norm(seg(w), w); got != want {
			t.Errorf("%q renders differently from %q:\n got %q\nwant %q", w, types[0], got, want)
		}
	}
}

// A draft arrives muted and a selected row arrives bold. Prefix colour layers
// onto that state rather than replacing it, or drafts stop looking like drafts.
func TestPrefixColourLayersOntoTheRowStyle(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	m := loaded(t, 120, 20, []github.PR{{
		Number: 3248, Title: "feat(ordering): add the thing",
		CIState: "SUCCESS", IsDraft: true, UpdatedAt: time.Now(),
	}}, nil)
	m.cursor = -1
	r := m.board.Sections()[0].Rows[0]

	out := m.renderTitle(r, mutedStyle, func(s lipgloss.Style) lipgloss.Style { return s }, 60, nil)
	// The subject of a draft carries no colour of its own, so it is the run
	// that proves the row's own faint survived.
	if seg := segmentAround(t, out, "add the thing"); !strings.Contains(seg, "2m") {
		t.Errorf("draft subject lost its faint: %q", seg)
	}
	if seg := segmentAround(t, out, "feat"); !hasSGRParam(seg, "34") {
		t.Errorf("draft type lost its colour: %q", seg)
	}
}
