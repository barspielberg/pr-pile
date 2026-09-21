package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/barspielberg/prs-mng/internal/board"
	"github.com/barspielberg/prs-mng/internal/github"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func runeKey(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func press(m Model, msg tea.KeyMsg) Model {
	next, _ := m.handleKey(msg)
	return next.(Model)
}

// typeQuery opens the search and types the query one key at a time, the way
// the user would: the match set is re-evaluated on every keystroke, not only
// the last.
func typeQuery(m Model, q string) Model {
	m = press(m, runeKey('/'))
	for _, r := range q {
		m = press(m, runeKey(r))
	}
	return m
}

// loaded is a two-section board with both rules resolved, so nothing is pending
// and every row below is drawable.
func loaded(t *testing.T, width, height int, mine, review []github.PR) Model {
	t.Helper()
	m := New(testCfg(), nil)
	m.width, m.height = width, height
	m.board.Apply(board.Result{Index: 0, PRs: mine})
	m.board.Apply(board.Result{Index: 1, PRs: review})
	return m
}

func samplePRs() ([]github.PR, []github.PR) {
	mine := []github.PR{
		{Number: 3248, Title: "feat(api-service): PROJ-2037 refuse order plan writes",
			CIState: "SUCCESS", UpdatedAt: time.Unix(300, 0)},
		{Number: 3100, Title: "fix(pricing): rounding on invoice totals",
			CIState: "SUCCESS", UpdatedAt: time.Unix(200, 0)},
	}
	review := []github.PR{
		{Number: 4001, Title: "chore(deps): bump lipgloss",
			CIState: "SUCCESS", UpdatedAt: time.Unix(100, 0)},
	}
	return mine, review
}

func TestQueryDoesNotNarrowTheBoard(t *testing.T) {
	mine, review := samplePRs()
	full := loaded(t, 120, 40, mine, review)
	m := typeQuery(loaded(t, 120, 40, mine, review), "api-serv")

	if got, want := len(m.visibleRows()), len(full.visibleRows()); got != want {
		t.Errorf("the board lost %d rows to a query", want-got)
	}
	out := stripANSI(m.View())
	for _, n := range []string{"#3248", "#3100", "#4001"} {
		if !strings.Contains(out, n) {
			t.Errorf("%s left the board while searching:\n%s", n, out)
		}
	}
	if got := len(m.matchIndexes()); got != 1 {
		t.Errorf("want 1 match for %q, got %d", "api-serv", got)
	}
}

// The number is text like any other text on the row: it matches by substring,
// and so does a title that happens to carry the same digits. Both are
// highlighted, so the user can see which is which.
func TestNumberIsOrdinarySearchableText(t *testing.T) {
	mine := []github.PR{
		{Number: 3248, Title: "feat(api-service): refuse order plan writes",
			CIState: "SUCCESS", UpdatedAt: time.Unix(300, 0)},
		{Number: 3100, Title: "fix(pricing): round the 324 cent remainder",
			CIState: "SUCCESS", UpdatedAt: time.Unix(200, 0)},
	}
	m := loaded(t, 120, 40, mine, nil)
	rows := m.board.Sections()[0].Rows

	if !m.rowMatches(rows[0], false, "3248") {
		t.Error("the PR number did not match its own digits")
	}
	// The hit lands in the number cell, before the title starts.
	_, cells := m.searchText(rows[0], false)
	for _, sp := range m.matchSpans(rows[0], false, "3248") {
		if sp[1] > cells.title[0] {
			t.Errorf("span %v runs past the number cell", sp)
		}
	}

	// And the same query prefix finds it in a title too.
	if !m.rowMatches(rows[1], false, "324") {
		t.Error("a title containing the digits did not match")
	}
	if got := len(typeQuery(m, "324").matchIndexes()); got != 2 {
		t.Errorf("want both the number and the title matched, got %d", got)
	}
}

func TestCtrlNAndCtrlPMoveWithinMatches(t *testing.T) {
	var mine []github.PR
	for i := 0; i < 5; i++ {
		mine = append(mine, github.PR{
			Number: 100 + i, Title: fmt.Sprintf("alpha match %d", i),
			CIState: "SUCCESS", UpdatedAt: time.Unix(int64(500-i), 0),
		})
	}
	mine = append(mine, github.PR{
		Number: 999, Title: "zeta unrelated", CIState: "SUCCESS", UpdatedAt: time.Unix(1, 0),
	})
	m := typeQuery(loaded(t, 120, 40, mine, nil), "alpha")

	if got := len(m.matchIndexes()); got != 5 {
		t.Fatalf("want 5 matches, got %d", got)
	}

	matches := m.matchIndexes()
	down := tea.KeyMsg{Type: tea.KeyCtrlN}
	up := tea.KeyMsg{Type: tea.KeyCtrlP}

	for i := 1; i < len(matches); i++ {
		m = press(m, down)
		if m.cursor != matches[i] {
			t.Fatalf("ctrl+n: cursor %d, want match %d at row %d", m.cursor, i, matches[i])
		}
	}
	for i := len(matches) - 2; i >= 0; i-- {
		m = press(m, up)
		if m.cursor != matches[i] {
			t.Fatalf("ctrl+p: cursor %d, want match %d at row %d", m.cursor, i, matches[i])
		}
	}

	// The cursor must still address a match.
	if pr, ok := m.selected(); !ok || !strings.HasPrefix(pr.Title, "alpha") {
		t.Errorf("selection left the match set: %+v", pr)
	}
}

// ctrl+j/k are the same motions under the other convention.
func TestCtrlJAndCtrlKAlsoMove(t *testing.T) {
	mine, review := samplePRs()
	m := typeQuery(loaded(t, 120, 40, mine, review), "")

	before := len(m.visibleRows())
	m = press(m, tea.KeyMsg{Type: tea.KeyCtrlJ})
	if m.cursor != 1 {
		t.Errorf("ctrl+j: cursor %d, want 1 (of %d rows)", m.cursor, before)
	}
	m = press(m, tea.KeyMsg{Type: tea.KeyCtrlK})
	if m.cursor != 0 {
		t.Errorf("ctrl+k: cursor %d, want 0", m.cursor)
	}
}

func TestEscRestoresTheCursor(t *testing.T) {
	mine, review := samplePRs()
	m := loaded(t, 120, 40, mine, review)
	m.cursor = 1
	want := m.View()

	m = typeQuery(m, "lipgloss")

	m = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.searching {
		t.Error("esc should leave the search")
	}
	if m.query != "" {
		t.Errorf("esc should clear the query, got %q", m.query)
	}
	if m.cursor != 1 {
		t.Errorf("cursor is %d, want it back at 1 where the search opened", m.cursor)
	}
	if got := m.View(); got != want {
		t.Errorf("board did not return to its pre-search frame after esc:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// The board does not move while searching, so a section that matches nothing
// keeps its gutter and its rows: the user is looking for one PR among the ones
// around it, not at a shortlist.
func TestEmptySectionsStayVisibleWhileSearching(t *testing.T) {
	mine, review := samplePRs()
	m := typeQuery(loaded(t, 120, 40, mine, review), "refuse")

	out := stripANSI(m.View())
	for _, want := range []string{"MINE", "REVIEW", "#3248", "#3100", "#4001"} {
		if !strings.Contains(out, want) {
			t.Errorf("searching hid %s:\n%s", want, out)
		}
	}
	if got := len(m.matchIndexes()); got != 1 {
		t.Errorf("want exactly one match, got %d", got)
	}
}

// The search prompt is a second chrome row, so the body budget shrinks by one.
// The frame must still fit the terminal at every supported width.
func TestSearchedBoardFitsTerminalAtAnyWidth(t *testing.T) {
	var mine []github.PR
	for i := 0; i < 40; i++ {
		mine = append(mine, github.PR{
			Number: 3000 + i, Title: fmt.Sprintf("feat(api-service): %s %d", strings.Repeat("long ", 10), i),
			CIState: "FAILURE", FailedGates: []string{"a", "b"}, UpdatedAt: time.Unix(int64(900-i), 0),
		})
	}

	for w := minWidth; w <= 200; w += 7 {
		for _, h := range []int{6, 12, 24} {
			m := typeQuery(loaded(t, w, h, mine, nil), "api")
			if len(m.visibleRows()) == 0 {
				t.Fatalf("width %d height %d: query matched nothing", w, h)
			}
			for _, cursor := range []int{0, 5, len(m.visibleRows()) - 1} {
				m.cursor = cursor
				m.clampCursor()
				out := m.View()

				if got := strings.Count(out, "\n") + 1; got > h {
					t.Errorf("w=%d h=%d cursor=%d: view is %d lines, terminal is %d",
						w, h, cursor, got, h)
				}
				for _, l := range strings.Split(out, "\n") {
					if got := lipgloss.Width(stripANSI(l)); got > w {
						t.Errorf("w=%d h=%d: line is %d cells: %q", w, h, got, stripANSI(l))
					}
				}
			}
		}
	}
}

// The selected row has to stay on screen while searching, exactly as it does
// with no query.
func TestCursorStaysVisibleWhileSearching(t *testing.T) {
	var mine []github.PR
	for i := 0; i < 30; i++ {
		mine = append(mine, github.PR{
			Number: 200 + i, Title: fmt.Sprintf("refuse item %d", i),
			CIState: "SUCCESS", UpdatedAt: time.Unix(int64(900-i), 0),
		})
	}
	m := typeQuery(loaded(t, 120, 12, mine, nil), "refuse")

	n := len(m.visibleRows())
	for _, cursor := range []int{0, 7, n - 1} {
		m.cursor = cursor
		out := m.View()
		if !strings.Contains(out, "▌") {
			t.Errorf("cursor %d: selected row scrolled off screen:\n%s", cursor, out)
		}
		if got := strings.Count(out, "\n") + 1; got > m.height {
			t.Errorf("cursor %d: view is %d lines, terminal is %d", cursor, got, m.height)
		}
	}
}

// Backspace edits the query rather than exiting, so widening a too-narrow query
// brings matches back.
func TestBackspaceWidensTheQuery(t *testing.T) {
	mine, review := samplePRs()
	m := typeQuery(loaded(t, 120, 40, mine, review), "refusez")
	if got := len(m.matchIndexes()); got != 0 {
		t.Fatalf("query %q should match nothing, got %d", "refusez", got)
	}

	m = press(m, tea.KeyMsg{Type: tea.KeyBackspace})
	if m.query != "refuse" {
		t.Errorf("query is %q, want %q", m.query, "refuse")
	}
	if got := len(m.matchIndexes()); got != 1 {
		t.Errorf("want 1 match back after backspace, got %d", got)
	}
	if !m.searching {
		t.Error("backspace should not leave the search")
	}
}

// Deleting the whole query puts the board back where the prompt opened, the
// same as esc: an empty query is the state / started in, so the cursor has no
// match left to be sitting on.
func TestEmptyingTheQueryReturnsTheCursor(t *testing.T) {
	mine, review := samplePRs()

	for _, clear := range []string{"backspace", "ctrl+u"} {
		m := loaded(t, 120, 40, mine, review)
		m.cursor = 1
		m = typeQuery(m, "lipgloss")
		if m.cursor == 1 {
			t.Fatalf("%s: incsearch did not move the cursor, so this test proves nothing", clear)
		}

		if clear == "ctrl+u" {
			m = press(m, tea.KeyMsg{Type: tea.KeyCtrlU})
		} else {
			for range "lipgloss" {
				m = press(m, tea.KeyMsg{Type: tea.KeyBackspace})
			}
		}

		if m.query != "" {
			t.Errorf("%s: query is %q, want it empty", clear, m.query)
		}
		if !m.searching {
			t.Errorf("%s: should not leave the search", clear)
		}
		if m.cursor != 1 {
			t.Errorf("%s: cursor is %d, want it back at 1 where the search opened", clear, m.cursor)
		}
	}
}

// A query that matches nothing changes nothing on the board, so the chrome
// carries the whole signal: the position says so, and the query text itself
// turns red.
func TestNoMatchesStillRendersPromptAndCount(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	mine, review := samplePRs()
	m := typeQuery(loaded(t, 120, 20, mine, review), "zzzqqq")

	prompt := m.promptLine()
	if got := stripANSI(prompt); !strings.Contains(got, "zzzqqq") {
		t.Errorf("prompt should show the query: %q", got)
	}
	if got := stripANSI(prompt); !strings.Contains(got, "no matches") {
		t.Errorf("prompt should say no matches: %q", got)
	}
	if !strings.Contains(prompt, "\x1b[31m") {
		t.Errorf("a query matching nothing should be red: %q", prompt)
	}

	// And every row is still on the board.
	out := m.View()
	for _, n := range []string{"#3248", "#3100", "#4001"} {
		if !strings.Contains(stripANSI(out), n) {
			t.Errorf("%s left the board:\n%s", n, stripANSI(out))
		}
	}
	if got := strings.Count(out, "\n") + 1; got > m.height {
		t.Errorf("the view is %d lines, terminal is %d", got, m.height)
	}
}

// The moment a match exists the red goes away, which is what makes backspacing
// back to a match readable.
func TestPromptStopsBeingRedWhenAMatchAppears(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	mine, review := samplePRs()
	m := typeQuery(loaded(t, 120, 20, mine, review), "refusez")
	if !strings.Contains(m.promptLine(), "\x1b[31m") {
		t.Fatal("the no-match query is not red, so this test proves nothing")
	}

	m = press(m, tea.KeyMsg{Type: tea.KeyBackspace})
	if strings.Contains(m.promptLine(), "\x1b[31m") {
		t.Errorf("the prompt stayed red with a match: %q", m.promptLine())
	}
}

// The count on the prompt must agree with what n and N will actually visit.
func TestPromptCountMatchesMatchingRows(t *testing.T) {
	mine, review := samplePRs()
	for _, q := range []string{"", "o", "api-serv", "3248", "zzz"} {
		m := typeQuery(loaded(t, 120, 40, mine, review), q)
		matches := m.matchIndexes()

		want := fmt.Sprintf("%d matches", len(matches))
		switch {
		case strings.TrimSpace(q) == "":
			want = ""
		case len(matches) == 0:
			want = "no matches"
		default:
			for i, idx := range matches {
				if idx == m.cursor {
					want = fmt.Sprintf("%d of %d", i+1, len(matches))
				}
			}
		}
		if got := stripANSI(m.promptLine()); !strings.Contains(got, want) {
			t.Errorf("query %q: prompt %q should contain %q", q, got, want)
		}
	}
}

// ctrl+c still quits from inside the search; esc must not.
func TestCtrlCQuitsFromTheSearch(t *testing.T) {
	mine, review := samplePRs()
	m := typeQuery(loaded(t, 120, 40, mine, review), "ref")

	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("ctrl+c produced no command, expected tea.Quit")
	}
	if msg := cmd(); msg != tea.Quit() {
		t.Errorf("ctrl+c should quit, got %T", msg)
	}
}

// Keys that are navigation on the board are query text while searching. It is
// load-bearing now that n is a board key as well as a letter.
func TestPrintableKeysGoToTheQueryNotNavigation(t *testing.T) {
	mine, review := samplePRs()
	m := press(loaded(t, 120, 40, mine, review), runeKey('/'))

	for _, r := range []rune{'j', 'k', 'g', 'q', 'r'} {
		m = press(m, runeKey(r))
	}
	if m.query != "jkgqr" {
		t.Errorf("query is %q, want %q", m.query, "jkgqr")
	}
	if !m.searching {
		t.Error("typing should not have left the search")
	}
}

// Matched characters are filled, the way vim's hlsearch fills them: the board
// already spends foreground colour on status, author and title part, so the
// background is the only channel a hit can own outright.
func TestMatchedCharactersAreHighlighted(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	mine := []github.PR{{
		Number: 3248, Title: "feat(api-service): refuse order plan writes",
		CIState: "SUCCESS", UpdatedAt: time.Now(),
	}}
	m := typeQuery(loaded(t, 100, 20, mine, nil), "api-serv")

	var row string
	for _, l := range strings.Split(m.View(), "\n") {
		if strings.Contains(l, "#3248") {
			row = l
		}
	}
	if len(hitRunsOf(row)) == 0 {
		t.Errorf("no highlight in the matched title:\n%q", row)
	}
	// Highlighting must not change the row's width, or the age column shifts.
	if got := lipgloss.Width(stripANSI(row)); got != 100 {
		t.Errorf("highlighted row is %d cells, want 100", got)
	}

	// With no query the same title carries no highlight.
	plain := loaded(t, 100, 20, mine, nil)
	for _, l := range strings.Split(plain.View(), "\n") {
		if strings.Contains(l, "#3248") && strings.Contains(l, ";4m") {
			t.Errorf("a row with no query should not be highlighted:\n%q", l)
		}
	}
}

// Space arrives as its own key type rather than as a rune, so a multi-word
// query would silently lose its spaces if that were not handled.
func TestSpaceIsTypedIntoTheQuery(t *testing.T) {
	mine, review := samplePRs()
	m := press(loaded(t, 120, 40, mine, review), runeKey('/'))
	for _, r := range "order" {
		m = press(m, runeKey(r))
	}
	m = press(m, tea.KeyMsg{Type: tea.KeySpace})
	for _, r := range "plan" {
		m = press(m, runeKey(r))
	}

	if m.query != "order plan" {
		t.Fatalf("query is %q, want %q", m.query, "order plan")
	}
	matches := m.matchIndexes()
	rows := m.visibleRows()
	if len(matches) != 1 || rows[matches[0]].PR.Number != 3248 {
		t.Errorf("query %q should find #3248, got %d matches", m.query, len(matches))
	}
}

// Searching is a view concern: it must not disturb the board's own bucketing.
func TestSearchDoesNotMutateTheBoard(t *testing.T) {
	mine, review := samplePRs()
	m := loaded(t, 120, 40, mine, review)

	before := make([][]int, 0)
	for _, s := range m.board.Sections() {
		var nums []int
		for _, r := range s.Rows {
			nums = append(nums, r.PR.Number)
		}
		before = append(before, nums)
	}

	typeQuery(m, "api-serv").View()

	for i, s := range m.board.Sections() {
		if len(s.Rows) != len(before[i]) {
			t.Fatalf("section %d changed size: %d -> %d", i, len(before[i]), len(s.Rows))
		}
		for j, r := range s.Rows {
			if r.PR.Number != before[i][j] {
				t.Errorf("section %d row %d changed: #%d -> #%d", i, j, before[i][j], r.PR.Number)
			}
		}
	}
}

func numbersOf(rows []board.Row) []int {
	out := []int{}
	for _, r := range rows {
		out = append(out, r.PR.Number)
	}
	return out
}

// The one duplication in this design is searchText reproducing renderRow's
// width arithmetic. If the layout changes and the search does not, matches
// stop lining up with what is drawn -- the exact failure invariant A forbids.
// So: every cell of searchText must appear verbatim in the rendered row, at
// every width tier and with the author column both ways.
//
// The wide-rune titles are the ones that matter most here: a cell's rune count
// and its display width part company at the first CJK or emoji rune, and an
// offset measured in the wrong one of those two lands the author's highlight
// under the title.
func TestSearchTextMatchesWhatIsRendered(t *testing.T) {
	prs := []github.PR{
		{Number: 3248, Title: "feat(api-service): PROJ-2037 refuse order plan writes",
			Author: "immanuel", CIState: "SUCCESS", UpdatedAt: time.Unix(300, 0)},
		{Number: 7, Title: "fix: tiny", Author: "ab", CIState: "FAILURE", UpdatedAt: time.Unix(200, 0)},
		{Number: 41234, Title: "chore(deps): bump lipgloss to the version with the fix",
			Author: "dependabot", CIState: "PENDING", UpdatedAt: time.Unix(100, 0)},
		{Number: 4001, Title: "修复订单计划写入被拒绝的问题",
			Author: "immanuel", CIState: "SUCCESS", UpdatedAt: time.Unix(400, 0)},
		{Number: 4002, Title: "🚀🚀 fix the ordering thing",
			Author: "immanuel", CIState: "SUCCESS", UpdatedAt: time.Unix(500, 0)},
		{Number: 4003, Title: "修复 fix(api): 订单 plan writes 被拒绝",
			Author: "immanuel", CIState: "FAILURE", UpdatedAt: time.Unix(600, 0)},
	}

	for _, w := range []int{minWidth, 60, 75, 86, 100, 147, 200} {
		for _, showAuthor := range []bool{false, true} {
			m := loaded(t, w, 40, prs, nil)
			for _, r := range m.board.Sections()[0].Rows {
				row := stripANSI(m.renderRow(r, false, showAuthor, "mine", sectionStarts))
				txt, cells := m.searchText(r, showAuthor)
				runes := []rune(txt)

				number := string(runes[cells.number[0]:cells.number[1]])
				if !strings.Contains(row, number) {
					t.Errorf("w=%d author=%v: number cell %q is not in the row:\n%q",
						w, showAuthor, number, row)
				}
				if cells.title[1] > cells.title[0] {
					cell := string(runes[cells.title[0]:cells.title[1]])
					if !strings.Contains(row, cell) {
						t.Errorf("w=%d author=%v: title cell %q is not in the row:\n%q",
							w, showAuthor, cell, row)
					}
					// The cell has to be the full title column on screen, or a
					// hit in its tail would be projected outside it.
					if got := lipgloss.Width(cell); got != m.searchTitleWidth(showAuthor) {
						t.Errorf("w=%d author=%v: title cell %q draws %d columns, not %d",
							w, showAuthor, cell, got, m.searchTitleWidth(showAuthor))
					}
				}
				if m.showsAuthor(showAuthor) {
					cell := string(runes[cells.author[0]:cells.author[1]])
					if !strings.Contains(row, cell) {
						t.Errorf("w=%d author=%v: author cell %q is not in the row:\n%q",
							w, showAuthor, cell, row)
					}
					if cells.author[1] != len(runes) {
						t.Errorf("w=%d author=%v: author cell ends at %d, not the end of %q",
							w, showAuthor, cells.author[1], txt)
					}
				} else if len(runes) > cells.title[1] {
					t.Errorf("w=%d author=%v: searchText carries an author cell the row does not draw: %q",
						w, showAuthor, txt)
				}
			}
		}
	}
}

// The cells are accumulated as searchText appends its segments, but tw moves
// with the width tier, so they are asserted rather than trusted. A wide-rune
// title is the case the arithmetic they replaced got wrong.
func TestSearchTextOffsetsLandOnTheCells(t *testing.T) {
	for _, title := range []string{
		"feat(api): refuse order plan writes",
		"修复订单计划写入被拒绝的问题",
		"🚀🚀 refuse order plan writes",
	} {
		pr := github.PR{Number: 3248, Title: title,
			Author: "immanuel", CIState: "SUCCESS", UpdatedAt: time.Unix(300, 0)}

		for _, w := range []int{80, 100, 147} {
			m := loaded(t, w, 40, []github.PR{pr}, nil)
			r := m.board.Sections()[0].Rows[0]
			raw, cells := m.searchText(r, true)
			txt := []rune(raw)

			head := []rune(clip(title, m.searchTitleWidth(true)))
			if n := cells.title[0] + len(head); n <= len(txt) {
				if got := string(txt[cells.title[0]:n]); got != string(head) {
					t.Errorf("w=%d title=%q: title cell starts on %q", w, title, got)
				}
			}
			// Below tierFull the cell is not drawn, so the offset addresses
			// nothing -- which is the point, not an off-by-one.
			if !m.showsAuthor(true) {
				continue
			}
			if got := string(txt[cells.author[0]:cells.author[1]]); got != "imm" {
				t.Errorf("w=%d title=%q: author cell %v lands on %q", w, title, cells.author, got)
			}
		}
	}
}

// The bug this guards: the author offset used to be a display width added to a
// rune index. One wide rune in the title and they diverge by a column each, so
// the author's hit slid out of its own cell -- first painting runes inside the
// title that do not contain the query, then, with a fully CJK title, vanishing
// altogether while the row still counted as a match.
func TestAuthorHighlightSurvivesWideRuneTitles(t *testing.T) {
	for _, title := range []string{
		"fix the ordering thing",
		"🚀 fix the ordering thing",
		"🚀🚀 fix the ordering thing",
		"修复 fix the ordering thing",
		"修复订单计划写入被拒绝的问题",
		"修复订单计划写入被拒绝的问题，以及其他若干问题需要处理",
	} {
		pr := github.PR{Number: 4001, Title: title, Author: "immanuel",
			CIState: "SUCCESS", UpdatedAt: time.Unix(300, 0)}
		m := loaded(t, 147, 40, []github.PR{pr}, nil)
		r := m.board.Sections()[0].Rows[0]
		if !m.showsAuthor(true) {
			t.Fatal("fixture width does not draw the author cell")
		}

		spans := m.matchSpans(r, true, "imm")
		_, cells := m.searchText(r, true)

		// The whole query lands in the author cell, every rune of it.
		if got := cellHits(spans, cells.author); len(got) != len("imm") {
			t.Errorf("title=%q: author hits %v, want all 3 runes of the query", title, got)
		}
		// And none of it leaks into the title, which does not contain it.
		if got := cellHits(spans, cells.title); got != nil {
			t.Errorf("title=%q: query is not in the title, but it highlighted %v", title, got)
		}
	}
}

// A clipped title stops at the clip point, so a word past it is not on screen
// and must not match. Widen the pane and the same word is found.
func TestClippedTitleTextIsNotSearchable(t *testing.T) {
	pr := github.PR{Number: 3248, CIState: "SUCCESS", UpdatedAt: time.Unix(300, 0),
		Title: "feat(api-service): refuse order plan writes while the fleet is offline"}

	narrow := loaded(t, 70, 40, []github.PR{pr}, nil)
	if narrow.rowMatches(narrow.board.Sections()[0].Rows[0], false, "offline") {
		txt, _ := narrow.searchText(narrow.board.Sections()[0].Rows[0], false)
		t.Errorf("a word past the clip point matched: %q", txt)
	}

	wide := loaded(t, 147, 40, []github.PR{pr}, nil)
	if !wide.rowMatches(wide.board.Sections()[0].Rows[0], false, "offline") {
		txt, _ := wide.searchText(wide.board.Sections()[0].Rows[0], false)
		t.Errorf("the same word did not match at a width that draws it: %q", txt)
	}
}

// The author column is not drawn below tierFull, so it is not searchable there.
func TestAuthorIsNotSearchableBelowFullWidth(t *testing.T) {
	pr := github.PR{Number: 3248, Title: "refuse order plan writes",
		Author: "immanuel", CIState: "SUCCESS", UpdatedAt: time.Unix(300, 0)}

	mid := loaded(t, midUntil, 40, []github.PR{pr}, nil)
	if got := widthTierFor(mid.width, true); got == tierFull {
		t.Fatalf("width %d is tierFull, so this test proves nothing", mid.width)
	}
	if mid.rowMatches(mid.board.Sections()[0].Rows[0], true, "imm") {
		t.Error("author matched at a width that does not draw the column")
	}
}

// Age is text and it is drawn, but it is excluded: a match on it would expire
// on its own as the clock moves, and a bare digit would collide with the PR
// number on the way to every numeric search.
func TestAgeIsNotSearchable(t *testing.T) {
	pr := github.PR{Number: 3248, Title: "refuse order plan writes",
		Author: "immanuel", CIState: "SUCCESS", UpdatedAt: time.Now().Add(-48 * time.Hour)}
	m := loaded(t, 147, 40, []github.PR{pr}, nil)
	r := m.board.Sections()[0].Rows[0]

	if got := age(pr.UpdatedAt); got != "2d" {
		t.Fatalf("fixture is %q, not the 2d this test needs", got)
	}
	if txt, _ := m.searchText(r, true); strings.Contains(txt, "2d") {
		t.Errorf("the age cell is in the searchable text: %q", txt)
	}
	if m.rowMatches(r, true, "2d") {
		t.Error("a query matched the age cell")
	}
}

// Every occurrence is a hit, not only the first: a query can appear twice on
// one row and both have to be highlighted.
func TestMatchSpansFindsEveryOccurrence(t *testing.T) {
	pr := github.PR{Number: 3248, Title: "fix: fix the fix", CIState: "SUCCESS",
		UpdatedAt: time.Unix(300, 0)}
	m := loaded(t, 147, 40, []github.PR{pr}, nil)

	if got := m.matchSpans(m.board.Sections()[0].Rows[0], false, "fix"); len(got) != 3 {
		t.Errorf("want 3 spans for three occurrences, got %v", got)
	}
}

// strings.Index gives byte offsets and every consumer indexes by rune, so a
// title with multi-byte characters -- and the multi-byte ellipsis clip itself
// adds -- would misplace every highlight without the conversion.
func TestClippedNonASCIITitleHighlightsCorrectly(t *testing.T) {
	pr := github.PR{Number: 3248, CIState: "SUCCESS", UpdatedAt: time.Unix(300, 0),
		Title: "feat(café): refuse — órder plán writes for the whole fleet at once"}
	m := loaded(t, 90, 40, []github.PR{pr}, nil)
	r := m.board.Sections()[0].Rows[0]

	raw, _ := m.searchText(r, false)
	txt := []rune(raw)
	if !strings.ContainsRune(string(txt), '…') {
		t.Fatalf("fixture is not clipped at this width: %q", string(txt))
	}

	for _, q := range []string{"café", "órder", "—"} {
		spans := m.matchSpans(r, false, q)
		if len(spans) == 0 {
			t.Errorf("%q did not match: %q", q, string(txt))
			continue
		}
		s := spans[0]
		if got := strings.ToLower(string(txt[s[0]:s[1]])); got != strings.ToLower(q) {
			t.Errorf("span for %q covers %q, not the query", q, got)
		}
	}
}

// Padding is part of the searchable string so the offsets line up with the
// screen, which would otherwise make a query of spaces match every row.
func TestWhitespaceQueryMatchesNothing(t *testing.T) {
	mine, review := samplePRs()
	m := loaded(t, 147, 40, mine, review)
	r := m.board.Sections()[0].Rows[0]

	for _, q := range []string{"", " ", "   "} {
		if m.rowMatches(r, true, q) {
			t.Errorf("query %q matched", q)
		}
		if got := m.matchSpans(r, true, q); got != nil {
			t.Errorf("query %q produced spans %v", q, got)
		}
	}
}

// Case never matters, in either direction: initials() lower-cases the author,
// so a login typed the way GitHub spells it would never find its own cell.
func TestMatchingIsCaseInsensitive(t *testing.T) {
	pr := github.PR{Number: 3248, Title: "feat(api): PROJ-2037 refuse writes",
		Author: "Immanuel", CIState: "SUCCESS", UpdatedAt: time.Unix(300, 0)}
	m := loaded(t, 147, 40, []github.PR{pr}, nil)
	r := m.board.Sections()[0].Rows[0]

	for _, q := range []string{"PROJ", "proj", "Imm", "IMM", "REFUSE"} {
		if !m.rowMatches(r, true, q) {
			txt, _ := m.searchText(r, true)
			t.Errorf("%q did not match %q", q, txt)
		}
	}
}

// rowFor renders the row carrying a number and returns the line, so the
// highlight tests can name a row without counting lines.
func rowFor(t *testing.T, m Model, number string) string {
	t.Helper()
	for _, l := range strings.Split(m.View(), "\n") {
		if strings.Contains(stripANSI(l), number) {
			return l
		}
	}
	t.Fatalf("no row for %s on the board", number)
	return ""
}

// Invariant A, as an executable assertion: a row the search accepts always
// carries a filled run on screen. A matched row with nothing marked on it is
// the one state this design must never produce.
func TestEveryMatchIsVisiblyHighlighted(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	mine := []github.PR{
		{Number: 3248, Title: "feat(api-service): PROJ-2037 refuse order plan writes",
			Author: "immanuel", CIState: "SUCCESS", UpdatedAt: time.Unix(300, 0)},
		{Number: 3100, Title: "fix(pricing): rounding on invoice totals",
			Author: "cdiaz88", CIState: "SUCCESS", UpdatedAt: time.Unix(200, 0)},
		// Wide runes put the cells' rune counts and their column widths out of
		// step, which is where a match can be counted and still draw nothing.
		{Number: 3301, Title: "修复订单计划写入被拒绝的问题",
			Author: "immanuel", CIState: "SUCCESS", UpdatedAt: time.Unix(250, 0)},
		{Number: 3302, Title: "🚀🚀 refuse order plan writes",
			Author: "cdiaz88", CIState: "FAILURE", UpdatedAt: time.Unix(240, 0)},
		{Number: 3303, Title: "修复 fix(api): 订单 refuse PROJ-2037 被拒绝",
			Author: "dependabot", CIState: "SUCCESS", UpdatedAt: time.Unix(230, 0)},
	}
	review := []github.PR{
		{Number: 4001, Title: "chore(deps): bump lipgloss", Author: "dependabot",
			CIState: "SUCCESS", UpdatedAt: time.Unix(100, 0)},
		{Number: 4002, Title: "重构：订单计划的写入路径 lipgloss", Author: "immanuel",
			CIState: "PENDING", UpdatedAt: time.Unix(90, 0)},
	}

	for _, q := range []string{"refuse", "324", "imm", "o", "PROJ", "lipgloss", "3", "dep",
		"修复", "订单", "🚀", "被拒绝", "计划"} {
		for _, w := range []int{100, 147, 200} {
			m := typeQuery(loaded(t, w, 40, mine, review), q)
			rows := m.visibleRows()
			for _, idx := range m.matchIndexes() {
				line := rowFor(t, m, fmt.Sprintf("#%d", rows[idx].PR.Number))
				if len(hitRunsOf(line)) == 0 {
					t.Errorf("w=%d q=%q: #%d matches but draws no highlight:\n%q",
						w, q, rows[idx].PR.Number, line)
				}
			}
		}
	}
}

// The number fills the digits that were typed and nothing else -- not the
// trailing padding, which would read as a wider match than it is -- and the
// cell keeps its width so the status cluster does not shift.
func TestNumberCellHighlightsAndKeepsItsWidth(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	mine := []github.PR{{Number: 42, Title: "fix(pricing): round it",
		Author: "immanuel", CIState: "SUCCESS", UpdatedAt: time.Unix(300, 0)}}

	plain := loaded(t, 147, 20, mine, nil)
	before := stripANSI(rowFor(t, plain, "#42"))

	m := typeQuery(loaded(t, 147, 20, mine, nil), "42")
	line := rowFor(t, m, "#42")

	if got := stripANSI(line); got != before {
		t.Errorf("highlighting moved the row's characters:\n got %q\nwant %q", got, before)
	}
	if got := lipgloss.Width(stripANSI(line)); got != 147 {
		t.Errorf("highlighted row is %d cells, want 147", got)
	}

	// Only the two digits are filled: the cell is six wide, and the four
	// trailing spaces are not part of the match.
	if got := hitRunsOf(line); len(got) != 1 || got[0] != "42" {
		t.Errorf("highlighted %v, want just the digits", got)
	}
}

// Padding sits inside the searchable string so the offsets line up with the
// screen, but it must never be filled: a fill running under the padding reads as
// a wider match than the one that was found.
func TestPaddingIsNeverFilled(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	mine := []github.PR{{Number: 42, Title: "fix: round it", Author: "ab",
		CIState: "SUCCESS", UpdatedAt: time.Unix(300, 0)}}
	m := typeQuery(loaded(t, 147, 20, mine, nil), "42")
	line := rowFor(t, m, "#42")

	// Every filled run on the row is query text, never blank filler: a fill
	// that ran under the padding would read as a wider match than it is.
	for _, run := range hitRunsOf(line) {
		if strings.TrimSpace(run) == "" {
			t.Errorf("a highlighted run is pure padding: %q in %q", run, line)
		}
	}
}

// hitFill is the SGR parameter a matched run is filled with, which is what the
// tests below look for. It is read back out of what hitStyle actually renders
// rather than assembled by hand: lipgloss writes a themed background as the
// short form (45) and a cube one as 48;5;N, so spelling it out would silently
// stop these tests asserting anything the next time the colour moves.
func hitFill() string {
	rendered := hitStyle.Render("x")
	i := strings.Index(rendered, "\x1b[")
	j := strings.Index(rendered[i:], "m")
	return rendered[i+2 : i+j]
}

// hitRunsOf returns the plain text of every filled stretch of the line, with
// adjacent runs joined: the test cares where the highlight starts and stops,
// not how many escape sequences it took to draw.
func hitRunsOf(line string) []string {
	var out []string
	open := false
	for _, seg := range strings.Split(line, "\x1b[")[1:] {
		i := strings.Index(seg, "m")
		if i < 0 {
			continue
		}
		text := seg[i+1:]
		if !strings.Contains(seg[:i], hitFill()) {
			if text != "" {
				open = false
			}
			continue
		}
		if open {
			out[len(out)-1] += text
			continue
		}
		out = append(out, text)
		open = true
	}
	return out
}

// The author cell is three cells wide and stays three cells wide, and the
// initials are filled like every other hit rather than keeping their palette hue.
func TestAuthorCellHighlightsWithinItsThreeCells(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	// Only the review rule shows the author column, so that is where a row
	// with an author cell to fill has to live.
	review := []github.PR{{Number: 3248, Title: "fix(pricing): round it",
		Author: "immanuel", CIState: "SUCCESS", UpdatedAt: time.Unix(300, 0)}}

	plain := loaded(t, 147, 20, nil, review)
	before := stripANSI(rowFor(t, plain, "#3248"))

	m := typeQuery(loaded(t, 147, 20, nil, review), "imm")
	line := rowFor(t, m, "#3248")

	if got := stripANSI(line); got != before {
		t.Errorf("highlighting the author moved the row:\n got %q\nwant %q", got, before)
	}
	if got := hitRunsOf(line); len(got) != 1 || got[0] != "imm" {
		t.Errorf("highlighted %v, want just the initials", got)
	}
	// The fill replaces the palette colour rather than composing with it. That
	// is the point: an author hit has to look like every other hit, not like a
	// slightly different shade of that author.
	palette := fmt.Sprintf("38;5;%s", authorStyle("immanuel").GetForeground())
	for _, seg := range strings.Split(line, "\x1b[") {
		if strings.Contains(seg, hitFill()) && strings.Contains(seg, palette) {
			t.Errorf("the fill kept the author's own colour under it: %q", seg)
		}
	}
}

// The age cell is drawn but never searchable, so it is never highlighted --
// including when the query would have matched its text.
func TestAgeCellIsNeverHighlighted(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	mine := []github.PR{{Number: 3248, Title: "fix(pricing): round it",
		Author: "immanuel", CIState: "SUCCESS", UpdatedAt: time.Now().Add(-48 * time.Hour)}}
	m := typeQuery(loaded(t, 147, 20, mine, nil), "2d")
	line := rowFor(t, m, "#3248")

	if got := hitRunsOf(line); len(got) != 0 {
		t.Errorf("a query matching only the age highlighted %v: %q", got, line)
	}
}

// incsearch: the cursor walks to the match as the query is typed, so the
// answer is on screen before the user stops typing.
func TestTypingWalksTheCursorToTheFirstMatch(t *testing.T) {
	mine, review := samplePRs()
	m := loaded(t, 120, 40, mine, review)
	m.cursor = 0

	m = typeQuery(m, "lipgloss")
	matches := m.matchIndexes()
	if len(matches) != 1 {
		t.Fatalf("want 1 match, got %d", len(matches))
	}
	if m.cursor != matches[0] {
		t.Errorf("cursor is %d, want it on the match at %d", m.cursor, matches[0])
	}
}

// A keystroke that keeps the current row matching must not move the cursor, or
// typing the middle of a word jitters off a row it had already found.
func TestTypingOnAMatchingRowDoesNotMoveTheCursor(t *testing.T) {
	mine, review := samplePRs()
	m := typeQuery(loaded(t, 120, 40, mine, review), "refuse")
	landed := m.cursor

	for _, r := range " order" {
		m = press(m, runeKey(r))
		if m.cursor != landed {
			t.Fatalf("cursor moved to %d while the row still matched", m.cursor)
		}
	}
}

// A query that stops matching leaves the cursor where it is: it is on its way
// somewhere, and throwing away the position would lose the user's place.
func TestAQueryMatchingNothingLeavesTheCursorAlone(t *testing.T) {
	mine, review := samplePRs()
	m := typeQuery(loaded(t, 120, 40, mine, review), "refuse")
	landed := m.cursor

	m = typeQuery(m, "zzz")
	if len(m.matchIndexes()) != 0 {
		t.Fatal("the query still matches, so this test proves nothing")
	}
	if m.cursor != landed {
		t.Errorf("cursor moved to %d on a query that matches nothing", m.cursor)
	}
}

// The board itself never moves while searching: same rows, same order, same
// stack glyphs, only colour and the two chrome rows change.
func TestBoardDoesNotMoveWhileSearching(t *testing.T) {
	var mine []github.PR
	for i := 0; i < 12; i++ {
		mine = append(mine, github.PR{
			Number: 3000 + i, Title: fmt.Sprintf("feat(ordering): AF-1%d do the thing", i),
			CIState: "SUCCESS", UpdatedAt: time.Unix(int64(900-i), 0),
		})
	}
	_, review := samplePRs()

	before := loaded(t, 147, 40, mine, review)
	after := typeQuery(loaded(t, 147, 40, mine, review), "AF-13")

	// The cursor moves, so compare the rows rather than the whole frame.
	b, a := before.visibleRows(), after.visibleRows()
	if len(b) != len(a) {
		t.Fatalf("the board has %d rows under a query, %d without", len(a), len(b))
	}
	for i := range b {
		if b[i].PR.Number != a[i].PR.Number {
			t.Errorf("row %d is #%d searching, #%d not", i, a[i].PR.Number, b[i].PR.Number)
		}
		if b[i].Prefix != a[i].Prefix || b[i].Last != a[i].Last {
			t.Errorf("row %d lost its stack glyph: %q -> %q", i, b[i].Prefix, a[i].Prefix)
		}
	}
}

// The stack spine is drawn from Prefix, so it has to survive a query intact --
// a dangling ╰╴ would draw a line to a row that is not above it.
func TestStackGlyphsSurviveSearch(t *testing.T) {
	mine := []github.PR{
		{Number: 3001, Title: "feat: base", HeadRefName: "a", CIState: "SUCCESS", UpdatedAt: time.Unix(900, 0)},
		{Number: 3002, Title: "feat: stacked on it", BaseRefName: "a", HeadRefName: "b",
			CIState: "SUCCESS", UpdatedAt: time.Unix(800, 0)},
	}
	plain := stripANSI(loaded(t, 147, 40, mine, nil).View())
	if !strings.Contains(plain, "╭╴") || !strings.Contains(plain, "╰╴") {
		t.Fatalf("the fixture is not a stack, so this test proves nothing:\n%s", plain)
	}

	out := stripANSI(typeQuery(loaded(t, 147, 40, mine, nil), "stacked").View())
	for _, glyph := range []string{"╭╴", "╰╴"} {
		if !strings.Contains(out, glyph) {
			t.Errorf("searching dropped %q from the stack:\n%s", glyph, out)
		}
	}
}

// searched is a board with the query accepted: the prompt is closed, the query
// and its highlights are still live, and n/N work on them.
func searched(t *testing.T, m Model, q string) Model {
	t.Helper()
	m = typeQuery(m, q)
	return press(m, tea.KeyMsg{Type: tea.KeyEnter})
}

// n walks every match in order and N walks back, both crossing section
// boundaries -- the cursor indexes the flattened rows, so a boundary is not a
// thing it can stop at.
func TestNCrossesSectionBoundaries(t *testing.T) {
	mine := []github.PR{
		{Number: 3248, Title: "feat(ordering): refuse order plan writes",
			CIState: "SUCCESS", UpdatedAt: time.Unix(300, 0)},
		{Number: 3100, Title: "fix(pricing): rounding on invoice totals",
			CIState: "SUCCESS", UpdatedAt: time.Unix(200, 0)},
	}
	review := []github.PR{
		{Number: 4001, Title: "chore(ordering): bump lipgloss",
			CIState: "SUCCESS", UpdatedAt: time.Unix(100, 0)},
	}
	m := searched(t, loaded(t, 147, 40, mine, review), "ordering")

	matches := m.matchIndexes()
	if len(matches) != 2 {
		t.Fatalf("want 2 matches in different sections, got %d", len(matches))
	}
	if m.cursor != matches[0] {
		t.Fatalf("the search left the cursor at %d, want %d", m.cursor, matches[0])
	}

	m = press(m, runeKey('n'))
	if m.cursor != matches[1] {
		t.Errorf("n stopped at %d, want the next section's match at %d", m.cursor, matches[1])
	}
	m = press(m, runeKey('N'))
	if m.cursor != matches[0] {
		t.Errorf("N stopped at %d, want %d", m.cursor, matches[0])
	}
}

// Both ends wrap, and say so: a silent wrap is indistinguishable from being
// stuck on the last match.
func TestNWrapsAtTheEnds(t *testing.T) {
	mine, review := samplePRs()
	m := searched(t, loaded(t, 147, 40, mine, review), "o")
	matches := m.matchIndexes()
	if len(matches) < 2 {
		t.Fatalf("want several matches, got %d", len(matches))
	}

	// Walk to the last one, then one more.
	m.cursor = matches[len(matches)-1]
	next, cmd := m.handleKey(runeKey('n'))
	m = next.(Model)
	if m.cursor != matches[0] {
		t.Errorf("n at the end landed on %d, want the first match at %d", m.cursor, matches[0])
	}
	if got := runCmd(t, cmd); got != statusMsg("search hit BOTTOM, continuing at TOP") {
		t.Errorf("n did not announce the wrap, got %v", got)
	}

	next, cmd = m.handleKey(runeKey('N'))
	m = next.(Model)
	if m.cursor != matches[len(matches)-1] {
		t.Errorf("N at the top landed on %d, want %d", m.cursor, matches[len(matches)-1])
	}
	if got := runCmd(t, cmd); got != statusMsg("search hit TOP, continuing at BOTTOM") {
		t.Errorf("N did not announce the wrap, got %v", got)
	}
}

// n and N are the whole point of keeping the query after enter.
func TestNWorksAfterEnter(t *testing.T) {
	mine, review := samplePRs()
	m := searched(t, loaded(t, 147, 40, mine, review), "o")

	if m.searching {
		t.Error("enter should close the prompt")
	}
	if m.query == "" {
		t.Fatal("enter should keep the query")
	}
	before := m.cursor
	m = press(m, runeKey('n'))
	if m.cursor == before {
		t.Error("n did nothing after the search was accepted")
	}
}

// With no query at all n and N are not a mode the user can get stuck in: they
// do nothing, the same as any unbound key.
func TestNDoesNothingWithoutAQuery(t *testing.T) {
	mine, review := samplePRs()
	m := loaded(t, 147, 40, mine, review)
	m.cursor = 1

	for _, k := range []rune{'n', 'N'} {
		next, cmd := m.handleKey(runeKey(k))
		if got := next.(Model).cursor; got != 1 {
			t.Errorf("%c moved the cursor to %d with no query", k, got)
		}
		if cmd != nil {
			t.Errorf("%c produced a command with no query: %v", k, runCmd(t, cmd))
		}
	}
}

// esc on the board is this board's :noh -- it drops the query and with it the
// highlights, and leaves the cursor alone.
func TestEscOnBoardClearsHighlights(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	mine, review := samplePRs()
	m := searched(t, loaded(t, 147, 40, mine, review), "refuse")
	line := rowFor(t, m, "#3248")
	if len(hitRunsOf(line)) == 0 {
		t.Fatal("the accepted search is not highlighted, so this test proves nothing")
	}
	landed := m.cursor

	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if cmd != nil {
		t.Error("the first esc should clear the query, not quit")
	}
	if m.query != "" {
		t.Errorf("esc left the query %q", m.query)
	}
	if m.cursor != landed {
		t.Errorf("esc moved the cursor to %d, want it left at %d", m.cursor, landed)
	}
	if got := hitRunsOf(rowFor(t, m, "#3248")); len(got) != 0 {
		t.Errorf("highlights survived esc: %v", got)
	}

	// And with nothing left to clear it quits, as it always did.
	_, cmd = m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil || cmd() != tea.Quit() {
		t.Error("a second esc should quit")
	}
}

// q and ctrl+c quit from any state, so a query can never trap the user.
func TestQAndCtrlCQuitWithAQueryActive(t *testing.T) {
	mine, review := samplePRs()
	m := searched(t, loaded(t, 147, 40, mine, review), "refuse")

	for _, msg := range []tea.KeyMsg{runeKey('q'), {Type: tea.KeyCtrlC}} {
		_, cmd := m.handleKey(msg)
		if cmd == nil || cmd() != tea.Quit() {
			t.Errorf("%v did not quit with a query active", msg)
		}
	}
}

// enter accepts the search rather than opening the PR: <CR> is accept
// everywhere this model comes from, and enter or o is still one key away.
func TestEnterAcceptsRatherThanOpening(t *testing.T) {
	mine, review := samplePRs()
	mine[0].URL = "https://github.com/o/r/pull/3248"
	m := typeQuery(loaded(t, 147, 40, mine, review), "refuse")

	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	// Opening shells out to the browser, so the only safe assertion is that
	// enter asks for nothing to be done at all.
	if cmd != nil {
		t.Error("enter produced a command, want it only to accept the search")
	}
	if m.searching {
		t.Error("enter should close the prompt")
	}
	if m.query != "refuse" {
		t.Errorf("enter dropped the query: %q", m.query)
	}
}

// A background refresh must not disturb a live search: the query stays, the
// highlights come back on the refreshed rows, and the cursor is not re-homed
// -- re-homing because a fetch happened is the most disorienting thing a board
// can do.
func TestSearchSurvivesRefresh(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	mine, review := samplePRs()
	m := searched(t, loaded(t, 147, 40, mine, review), "refuse")
	landed, query := m.cursor, m.query

	next, _ := m.refresh()
	m = next.(Model)
	m.board.Apply(board.Result{Index: 0, PRs: mine})
	m.board.Apply(board.Result{Index: 1, PRs: review})

	if m.query != query {
		t.Errorf("the refresh dropped the query: %q", m.query)
	}
	if m.cursor != landed {
		t.Errorf("the refresh moved the cursor to %d, want it left at %d", m.cursor, landed)
	}
	if got := hitRunsOf(rowFor(t, m, "#3248")); len(got) == 0 {
		t.Error("the refreshed row lost its highlight")
	}
}

// Section 4.1: the searchable text is what is drawn, so the width decides the
// match set. Widening reveals matches a narrow pane clipped away.
func TestResizeRecomputesMatches(t *testing.T) {
	mine := []github.PR{{Number: 3248, CIState: "SUCCESS", UpdatedAt: time.Unix(300, 0),
		Title: "feat(api-service): refuse order plan writes while the fleet is offline"}}

	m := loaded(t, 70, 40, mine, nil)
	if got := len(typeQuery(m, "offline").matchIndexes()); got != 0 {
		t.Errorf("a word past the clip point matched at 70 cols: %d", got)
	}

	wide, _ := m.Update(tea.WindowSizeMsg{Width: 147, Height: 40})
	if got := len(typeQuery(wide.(Model), "offline").matchIndexes()); got != 1 {
		t.Errorf("the same word did not match at 147 cols: %d", got)
	}
}
