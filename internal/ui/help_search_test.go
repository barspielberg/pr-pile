package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// openHelp puts the reader on the legend at a height that makes it scroll, so
// the tests below are exercising the same page a real terminal shows.
func openHelp(t *testing.T) Model {
	t.Helper()
	m := New(testCfg(), nil)
	m.width, m.height = 100, 14
	m.showHelp = true
	m.helpMatch = -1
	if len(m.helpPage()) <= m.height {
		t.Fatalf("the legend fits at h=%d, these tests would prove nothing", m.height)
	}
	return m
}

// typeHelpQuery opens the legend's search and types the query one key at a
// time, the way the user would: the match set is re-evaluated on every
// keystroke, not only the last.
func typeHelpQuery(t *testing.T, m Model, q string) Model {
	t.Helper()
	m = press(m, keyOf("/"))
	if !m.helpSearching {
		t.Fatal("/ did not open the legend's search")
	}
	for _, r := range q {
		m = press(m, runeKey(r))
	}
	return m
}

// The same key opens both searches, and it opens a prompt rather than closing
// the page -- which is the contract `/` had to take from "any key closes".
func TestSlashOpensTheHelpSearch(t *testing.T) {
	m := typeHelpQuery(t, openHelp(t), "reload")
	if !m.showHelp {
		t.Fatal("/ closed the help page instead of opening its search")
	}
	if m.helpQuery != "reload" {
		t.Errorf("query is %q, want %q", m.helpQuery, "reload")
	}
	if !strings.Contains(stripANSI(m.helpOverlay()), "/ reload") {
		t.Errorf("the prompt does not show the query:\n%s", stripANSI(m.helpOverlay()))
	}
}

// The board's prompt and the legend's are one renderer, so they count the same
// way: a match set, a position within it, and "no matches" in red when there is
// nothing to count.
func TestHelpPromptCountsMatches(t *testing.T) {
	m := typeHelpQuery(t, openHelp(t), "reload")
	if got := stripANSI(m.helpOverlay()); !strings.Contains(got, "1 of 1") {
		t.Errorf("want the position on the prompt row, got:\n%s", got)
	}

	m = typeHelpQuery(t, openHelp(t), "zzzznope")
	if got := stripANSI(m.helpOverlay()); !strings.Contains(got, "no matches") {
		t.Errorf("want 'no matches', got:\n%s", got)
	}
}

// Incsearch: the page scrolls to the match as the query is typed, so the answer
// is on screen before the reader stops typing.
func TestHelpIncsearchScrollsToTheMatch(t *testing.T) {
	m := openHelp(t)
	// A line far enough down the legend that it is off the first screen.
	target := -1
	for i, l := range m.helpPage() {
		if strings.Contains(l.text, "merge conflicts") {
			target = i
		}
	}
	if target < m.height {
		t.Fatalf("the target line is at %d, on the first screen already", target)
	}

	m = typeHelpQuery(t, m, "merge conflicts")
	if m.helpMatch != target {
		t.Errorf("the search landed on line %d, want %d", m.helpMatch, target)
	}
	if !strings.Contains(stripANSI(m.helpOverlay()), "merge conflicts") {
		t.Errorf("the match is not on screen:\n%s", stripANSI(m.helpOverlay()))
	}
}

// Backspacing to an empty query puts the page back where the search opened,
// rather than stranding it wherever the last near-miss scrolled it.
func TestHelpEmptyQueryReturnsToOrigin(t *testing.T) {
	m := openHelp(t)
	m.helpScroll = 3
	m = typeHelpQuery(t, m, "merge conflicts")
	if m.helpScroll == 3 {
		t.Fatal("the search never scrolled the page, so this test proves nothing")
	}
	for range "merge conflicts" {
		m = press(m, keyOf("backspace"))
	}
	if m.helpScroll != 3 {
		t.Errorf("an empty query left the page at %d, want the origin 3", m.helpScroll)
	}
}

// Both prompts type through editQuery, so dropping ctrl+u from it has to leave
// the legend's search alone as well -- a chord that still cleared here would be
// the same split, moved one page over.
func TestCtrlUDoesNotClearTheHelpQuery(t *testing.T) {
	m := openHelp(t)
	m = typeHelpQuery(t, m, "merge conflicts")
	m = press(m, keyOf("ctrl+u"))

	if m.helpQuery != "merge conflicts" {
		t.Errorf("query is %q, want it untouched at %q", m.helpQuery, "merge conflicts")
	}
	if !m.helpSearching {
		t.Error("ctrl+u closed the legend's prompt")
	}
}

// esc in the prompt abandons the search: the query goes, and so does the
// scrolling incsearch did on the way.
func TestEscInHelpPromptRestoresThePage(t *testing.T) {
	m := openHelp(t)
	m.helpScroll = 2
	m = typeHelpQuery(t, m, "merge conflicts")
	m = press(m, keyOf("esc"))

	if m.helpSearching || m.helpQuery != "" {
		t.Errorf("esc left the search open: searching=%v query=%q", m.helpSearching, m.helpQuery)
	}
	if m.helpScroll != 2 {
		t.Errorf("esc left the page at %d, want the origin 2", m.helpScroll)
	}
	if !m.showHelp {
		t.Error("esc in the prompt closed the whole page; it should only cancel the search")
	}
}

// enter keeps everything: the prompt closes, the query stays live, and the
// highlights stay on the page for n and N to walk. That is the board's
// hlsearch, on the legend.
func TestEnterKeepsTheHelpQuery(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	m := typeHelpQuery(t, openHelp(t), "search")
	m = press(m, keyOf("enter"))
	if m.helpSearching {
		t.Error("enter left the prompt open")
	}
	if m.helpQuery != "search" {
		t.Errorf("enter dropped the query: %q", m.helpQuery)
	}
	if !m.showHelp {
		t.Error("enter closed the page")
	}
	if !hasHighlight(m.helpLines()) {
		t.Error("the accepted query left no highlights on the page")
	}
}

// esc on the page with a live query is the legend's :noh -- the highlights go
// and the page stays. A second esc, with nothing left to clear, closes it.
func TestEscOnHelpPageClearsThenCloses(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	m := typeHelpQuery(t, openHelp(t), "search")
	m = press(m, keyOf("enter"))

	m = press(m, keyOf("esc"))
	if !m.showHelp {
		t.Fatal("the first esc closed the page instead of clearing the highlights")
	}
	if m.helpQuery != "" {
		t.Errorf("the query survived esc: %q", m.helpQuery)
	}
	if hasHighlight(m.helpLines()) {
		t.Error("the highlights survived esc")
	}

	if press(m, keyOf("esc")).showHelp {
		t.Error("the second esc, with nothing left to clear, should close the page")
	}
}

// n and N walk the matches and wrap, and the wrap is announced in the board's
// own wording -- a silent wrap is indistinguishable from being stuck.
func TestHelpNextAndPreviousMatchWrap(t *testing.T) {
	m := typeHelpQuery(t, openHelp(t), "search")
	m = press(m, keyOf("enter"))

	matches := m.helpMatches()
	if len(matches) < 2 {
		t.Fatalf("%q matches %d lines, this test needs at least 2", "search", len(matches))
	}
	if m.helpMatch != matches[0] {
		t.Fatalf("the search sits on line %d, want the first match %d", m.helpMatch, matches[0])
	}

	m = press(m, keyOf("n"))
	if m.helpMatch != matches[1] {
		t.Errorf("n went to %d, want %d", m.helpMatch, matches[1])
	}

	for i := 1; i < len(matches); i++ {
		m = press(m, keyOf("n"))
	}
	if m.helpMatch != matches[0] {
		t.Errorf("n did not wrap to %d, it is on %d", matches[0], m.helpMatch)
	}

	m = press(m, keyOf("N"))
	if m.helpMatch != matches[len(matches)-1] {
		t.Errorf("N did not wrap to the last match %d, it is on %d",
			matches[len(matches)-1], m.helpMatch)
	}
}

// The wrap says so, in the same words the board uses, and it says it on the
// legend's own line rather than the board's.
func TestHelpWrapIsAnnounced(t *testing.T) {
	m := typeHelpQuery(t, openHelp(t), "search")
	m = press(m, keyOf("enter"))
	for range m.helpMatches() {
		m = press(m, keyOf("n"))
	}
	if m.helpStatus != "search hit BOTTOM, continuing at TOP" {
		t.Errorf("the wrap was announced as %q", m.helpStatus)
	}
	if m.status != "" {
		t.Errorf("the legend's wrap message reached the board's status line: %q", m.status)
	}
	if !strings.Contains(stripANSI(m.helpOverlay()), "continuing at TOP") {
		t.Errorf("the announcement is not on the page:\n%s", stripANSI(m.helpOverlay()))
	}
}

// The matched characters are filled, the same way the board fills them: the
// highlight is a real style change on the runes the query hit.
func TestHelpMatchesAreHighlighted(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	m := typeHelpQuery(t, openHelp(t), "reload")
	line := ""
	for _, l := range m.helpLines() {
		if strings.Contains(stripANSI(l), "reload") {
			line = l
		}
	}
	if line == "" {
		t.Fatal("no line on the page says 'reload'")
	}
	if !strings.Contains(line, hitStyle.Render("reload")) {
		t.Errorf("the match is not filled:\n%q", line)
	}
	// With no query the same line carries no fill.
	plain := openHelp(t)
	for _, l := range plain.helpLines() {
		if strings.Contains(stripANSI(l), "reload") && strings.Contains(l, hitStyle.Render("reload")) {
			t.Errorf("a page with no query is highlighted:\n%q", l)
		}
	}
}

// Highlighting must not move a character: the key column is padded to a fixed
// width, and a fill that changed the width would shear the descriptions.
//
// Trailing spaces are excluded, the way the board's own tests exclude them: the
// current match is padded to the pane's right edge so its band reads as one
// row, and blank cells after the text move nothing.
func TestHelpHighlightKeepsTheLineIntact(t *testing.T) {
	plain := openHelp(t)
	searched := typeHelpQuery(t, openHelp(t), "reload")
	before, after := plain.helpLines(), searched.helpLines()
	if len(before) != len(after) {
		t.Fatalf("the query changed the page length: %d -> %d", len(before), len(after))
	}
	for i := range before {
		b := strings.TrimRight(stripANSI(before[i]), " ")
		a := strings.TrimRight(stripANSI(after[i]), " ")
		if b != a {
			t.Errorf("line %d moved under the highlight:\n got %q\nwant %q", i, a, b)
		}
	}
}

// The glyph keys keep the colour they are the legend for. They are the reason
// the page is drawn from styled segments rather than one muted string.
func TestHelpGlyphKeysKeepTheirColour(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	m := openHelp(t)
	want := okStyle.Render("✓")
	found := false
	for _, l := range m.helpLines() {
		if strings.Contains(l, want) {
			found = true
		}
	}
	if !found {
		t.Error("the CI legend lost the colour on its ✓")
	}
}

// A whitespace-only query matches nothing, exactly as it does on the board --
// otherwise every indented line on the page would light up.
func TestHelpBlankQueryMatchesNothing(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	m := typeHelpQuery(t, openHelp(t), "  ")
	if got := m.helpMatches(); got != nil {
		t.Errorf("a blank query matched %d lines", len(got))
	}
	if hasHighlight(m.helpLines()) {
		t.Error("a blank query highlighted the page")
	}
}

// The prompt is a chrome row like the board's, so the legend gives up a line
// for it rather than drawing over its own last line or the hint.
func TestHelpPromptTakesItsOwnRow(t *testing.T) {
	plain := openHelp(t)
	searching := typeHelpQuery(t, openHelp(t), "r")
	if got, want := len(strings.Split(searching.helpOverlay(), "\n")), plain.height; got != want {
		t.Errorf("the searching page is %d rows, want %d", got, want)
	}
	rows := strings.Split(stripANSI(searching.helpOverlay()), "\n")
	if !strings.Contains(rows[len(rows)-2], "/ r") {
		t.Errorf("the prompt is not the row above the hint:\n%s", strings.Join(rows, "\n"))
	}
	if !strings.Contains(rows[len(rows)-1], "esc cancel") {
		t.Errorf("the hint row does not say how to leave the prompt:\n%s", rows[len(rows)-1])
	}
}

// hasHighlight reports whether any line carries the search fill. hitStyle is
// reverse video, so the fill is the one SGR the legend never sets otherwise.
func hasHighlight(lines []string) bool {
	for _, l := range lines {
		if strings.Contains(l, "\x1b[7m") {
			return true
		}
	}
	return false
}

// Closing the legend takes its search with it. The wrap announcement is about
// a page that is no longer on screen, and the board's footer is where it would
// otherwise be read.
func TestClosingHelpClearsItsSearch(t *testing.T) {
	m := typeHelpQuery(t, openHelp(t), "search")
	m = press(m, keyOf("enter"))
	for range m.helpMatches() {
		m = press(m, keyOf("n"))
	}
	if m.helpStatus == "" {
		t.Fatal("n never announced a wrap, so this test proves nothing")
	}

	m = press(m, keyOf("x"))
	if m.showHelp {
		t.Fatal("x should have closed the page")
	}
	if m.helpStatus != "" {
		t.Errorf("the legend's message survived the close: %q", m.helpStatus)
	}
	if m.helpQuery != "" || m.helpMatch != -1 || m.helpSearching {
		t.Errorf("the search survived the close: query=%q match=%d searching=%v",
			m.helpQuery, m.helpMatch, m.helpSearching)
	}
}

// The board's status line belongs to the board. It used to take over the
// legend's hint row, displacing the keys that row exists to carry at every
// height -- press y to copy a url, then ?, and there was no way off the page
// on screen.
func TestBoardStatusStaysOffTheLegendHint(t *testing.T) {
	m := openHelp(t)
	m.status = "action failed: herdr not running"

	rows := strings.Split(stripANSI(m.helpOverlay()), "\n")
	hint := rows[len(rows)-1]
	if strings.Contains(hint, "herdr not running") {
		t.Errorf("a board status reached the legend's hint row:\n%q", hint)
	}
	if !strings.Contains(hint, "close") {
		t.Errorf("the hint row lost its closing keys:\n%q", hint)
	}
}

// The legend's own message shares the row with the closing keys rather than
// replacing them, for the same reason.
func TestHelpStatusKeepsTheClosingKeys(t *testing.T) {
	m := typeHelpQuery(t, openHelp(t), "search")
	m = press(m, keyOf("enter"))
	for range m.helpMatches() {
		m = press(m, keyOf("n"))
	}

	rows := strings.Split(stripANSI(m.helpOverlay()), "\n")
	hint := rows[len(rows)-1]
	if !strings.Contains(hint, "continuing at TOP") {
		t.Errorf("the wrap announcement is not on the hint row:\n%q", hint)
	}
	if !strings.Contains(hint, "close") {
		t.Errorf("the announcement displaced the closing keys:\n%q", hint)
	}
}

// An overlay taller than its pane scrolls its own top off. The prompt is a
// second chrome row, so the arithmetic has to hold in both modes at every
// height -- h=2 with the prompt open came out three rows in a two-row pane.
func TestHelpOverlayNeverExceedsThePane(t *testing.T) {
	for _, searching := range []bool{false, true} {
		for h := 2; h <= 60; h++ {
			m := New(testCfg(), nil)
			m.width, m.height = 100, h
			m.showHelp, m.helpSearching, m.helpMatch = true, searching, -1
			if searching {
				m.helpQuery = "e"
			}
			if got := len(strings.Split(m.helpOverlay(), "\n")); got > h {
				t.Errorf("searching=%v h=%d: overlay is %d rows, taller than its pane",
					searching, h, got)
			}
		}
	}
}

// The legend's plain text is pinned to the layout the page had before it was
// rebuilt from styled segments. This is the test that was missing: the three
// highlight tests above compare the page against itself, so a uniform shift
// passes all of them -- and one did. Every row lost its two-space indent and
// nothing caught it.
//
// It also guards the search contract, which is what makes the shift more than
// cosmetic: the page matches what is on screen, so a query of "  j" has to keep
// matching the row it matches on a rendered page.
func TestLegendLayoutIsPinned(t *testing.T) {
	m := New(testCfg(), nil)
	m.width, m.height = 100, 14

	want := []string{
		"  KEYS",
		"  j / k     move ( ↓ ↑ )",
		"  ctrl+d/u  half a page down / up",
		"  pgdn/pgup a full page down / up",
		"  l / h     next / previous section ( → ← )",
	}
	got := m.helpLines()
	for i, w := range want {
		if g := stripANSI(got[i]); g != w {
			t.Errorf("line %d:\n got %q\nwant %q", i, g, w)
		}
	}

	// Every row is indented and every key column ends at the same place.
	for i, l := range m.helpLines() {
		plain := stripANSI(l)
		if plain == "" || strings.HasPrefix(plain, "  config:") {
			continue
		}
		if !strings.HasPrefix(plain, "  ") {
			t.Errorf("line %d is not indented: %q", i, plain)
		}
	}

	// The indent is on screen, so it is searchable.
	m.helpQuery = "  j / k"
	if len(m.helpMatches()) != 1 {
		t.Errorf("the indent is not searchable: %q matched %d lines",
			m.helpQuery, len(m.helpMatches()))
	}
}

// bandedLine is the index of the line carrying the selected-row fill, or -1.
// selBg is ANSI 8, which lipgloss emits as the SGR bright-black background
// 100 rather than a cube index -- the same marker view_test.go looks for.
func bandedLine(t *testing.T, m Model) int {
	t.Helper()
	at := -1
	for i, l := range m.helpLines() {
		if !strings.Contains(l, "100m") {
			continue
		}
		if at >= 0 {
			t.Fatalf("two lines are banded at once: %d and %d", at, i)
		}
		at = i
	}
	return at
}

// The bug this fixes: every match was filled identically, so the page did not
// change by a single byte between "1 of 4" and "2 of 4" -- the count asserted a
// position the page then refused to show. Stepping must move something visible.
func TestHelpBandMovesWithTheMatch(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	m := typeHelpQuery(t, openHelp(t), "next")
	if len(m.helpMatches()) < 3 {
		t.Fatalf("want >=3 matches to step through, got %d", len(m.helpMatches()))
	}

	first := bandedLine(t, m)
	if first < 0 {
		t.Fatal("no line carries the band while a search is live")
	}
	if first != m.helpMatch {
		t.Errorf("the band is on line %d but the current match is %d", first, m.helpMatch)
	}

	m = press(m, keyOf("enter"))
	next, _ := m.stepHelpMatch(true)
	m = next.(Model)
	second := bandedLine(t, m)
	if second == first {
		t.Errorf("n did not move the band: still on line %d", first)
	}
	if second != m.helpMatch {
		t.Errorf("the band is on line %d but the current match is %d", second, m.helpMatch)
	}

	back, _ := m.stepHelpMatch(false)
	if got := bandedLine(t, back.(Model)); got != first {
		t.Errorf("N did not return the band to line %d, got %d", first, got)
	}
}

// Only the current match is banded. The fill says where the query landed; the
// band says which one you are on, and they have to stay separate channels.
func TestHelpBandMarksOnlyTheCurrentMatch(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	m := typeHelpQuery(t, openHelp(t), "next")
	matches := m.helpMatches()
	if len(matches) < 2 {
		t.Fatalf("want >=2 matches, got %d", len(matches))
	}
	lines := m.helpLines()
	for _, i := range matches {
		if i == m.helpMatch {
			continue
		}
		if strings.Contains(lines[i], "100m") {
			t.Errorf("a match that is not current is banded, line %d:\n%q", i, lines[i])
		}
		// It is still a match, so it keeps the fill.
		if !strings.Contains(lines[i], hitStyle.Render("next")) {
			t.Errorf("a non-current match lost its fill, line %d:\n%q", i, lines[i])
		}
	}
}

// The hit must stay readable on the band. render.go measures the reverse fill
// at 4.62:1 against selBg by having the hit bypass the paint hook entirely --
// if it ever composed selBg in, the highlight would wash out on exactly the
// line the reader is on.
func TestHelpHitDoesNotInheritTheBand(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	m := typeHelpQuery(t, openHelp(t), "next")
	line := m.helpLines()[m.helpMatch]
	if !strings.Contains(line, "100m") {
		t.Fatalf("the current match is not banded:\n%q", line)
	}
	if !strings.Contains(line, hitStyle.Render("next")) {
		t.Errorf("the hit is not rendered with the plain fill on a banded line:\n%q", line)
	}
}

// A page nobody has searched has no cursor. helpMatch is an int whose zero
// value is a valid line index, so an ungated band put a cursor on KEYS.
func TestHelpHasNoBandWithoutAQuery(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	fresh := New(testCfg(), nil)
	fresh.width, fresh.height = 100, 14
	if got := bandedLine(t, fresh); got >= 0 {
		t.Errorf("an unsearched page bands line %d", got)
	}

	// And the band goes when the query is cleared rather than lingering.
	m := typeHelpQuery(t, openHelp(t), "next")
	m = press(m, keyOf("enter"))
	m = press(m, keyOf("esc"))
	if got := bandedLine(t, m); got >= 0 {
		t.Errorf("the band survived esc on line %d", got)
	}
}

// The band is the pane's full width, so it reads as one row rather than
// stopping at the end of a short legend line.
func TestHelpBandReachesTheRightEdge(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	m := typeHelpQuery(t, openHelp(t), "reload")
	line := m.helpLines()[m.helpMatch]
	if got := lipgloss.Width(line); got != m.width {
		t.Errorf("the banded line is %d wide, want the full %d", got, m.width)
	}
	if stripANSI(line) == strings.TrimRight(stripANSI(line), " ") {
		t.Error("the banded line was not padded at all")
	}
}
