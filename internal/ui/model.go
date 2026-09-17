// Package ui is the Bubble Tea board.
package ui

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"text/template"
	"time"

	"github.com/barspielberg/prs-mng/internal/board"
	"github.com/barspielberg/prs-mng/internal/browser"
	"github.com/barspielberg/prs-mng/internal/config"
	"github.com/barspielberg/prs-mng/internal/github"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Model struct {
	cfg    config.Config
	client *github.Client
	board  *board.Board

	width, height int
	cursor        int // index into the flattened visible rows
	spinner       int
	status        string
	fetching      bool

	filtering  bool
	filter     string
	showHelp   bool
	showChecks bool
}

type resultMsg board.Result
type tickMsg time.Time
type spinMsg time.Time
type refreshMsg struct{}
type statusMsg string

var spinFrames = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")

func New(cfg config.Config, client *github.Client) Model {
	return Model{cfg: cfg, client: client, board: board.New(cfg), width: 100, fetching: true}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(append(m.fetchAll(), spinTick())...)
}

// Every rule is fetched concurrently; only the reveal is ordered, which the
// board handles.
func (m Model) fetchAll() []tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(m.cfg.Rules))
	for i, r := range m.cfg.Rules {
		cmds = append(cmds, m.fetchRule(i, r))
	}
	return cmds
}

func (m Model) fetchRule(i int, r config.Rule) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		prs, err := m.client.Search(ctx, m.cfg.SearchQuery(r), r.PageSize())
		return resultMsg{Index: i, PRs: prs, Err: err}
	}
}

func spinTick() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg { return spinMsg(t) })
}

func (m Model) refreshTick() tea.Cmd {
	d := m.cfg.Refresh
	if d <= 0 {
		return nil
	}
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case spinMsg:
		if !m.fetching {
			return m, nil
		}
		m.spinner++
		return m, spinTick()

	case resultMsg:
		m.board.Apply(board.Result(msg))
		if !m.board.Loading() {
			m.fetching = false
			m.clampCursor()
			return m, m.refreshTick()
		}
		return m, nil

	case tickMsg, refreshMsg:
		return m.refresh()

	case statusMsg:
		m.status = string(msg)
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) refresh() (tea.Model, tea.Cmd) {
	// Keep the current rows on screen while refetching, so the board does not
	// collapse and re-expand under the cursor.
	m.board.Refetch()
	m.fetching = true
	m.status = ""
	return m, tea.Batch(append(m.fetchAll(), spinTick())...)
}

// In filter mode every printable key belongs to the query, so navigation has to
// move to chords. ctrl+n/p is what the user asked for; ctrl+j/k and the arrows
// are the same motions under the other two conventions.
func (m Model) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		return m.exitFilter(), nil
	case "ctrl+n", "ctrl+j", "down":
		m.cursor++
		m.clampCursor()
		return m, nil
	case "ctrl+p", "ctrl+k", "up":
		m.cursor--
		m.clampCursor()
		return m, nil
	case "enter":
		cmd := m.openSelected()
		return m.exitFilter(), cmd
	case "ctrl+u":
		m.filter = ""
	case "backspace":
		if r := []rune(m.filter); len(r) > 0 {
			m.filter = string(r[:len(r)-1])
		}
	default:
		// Space arrives as its own key type with no runes attached, so it has
		// to be spelled out or multi-word queries would silently drop it.
		switch {
		case msg.Type == tea.KeySpace:
			m.filter += " "
		case msg.Type == tea.KeyRunes && len(msg.Runes) > 0:
			m.filter += string(msg.Runes)
		default:
			return m, nil
		}
	}
	// The match set shrinks as the query grows, so the cursor can fall off the
	// end between keystrokes.
	m.clampCursor()
	return m, nil
}

func (m Model) exitFilter() Model {
	m.filtering = false
	m.filter = ""
	m.clampCursor()
	return m
}

func (m Model) openSelected() tea.Cmd {
	pr, ok := m.selected()
	if !ok {
		return nil
	}
	url := pr.URL
	return func() tea.Msg {
		if err := browser.Open(url); err != nil {
			return statusMsg("open failed: " + err.Error())
		}
		return statusMsg("")
	}
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.showHelp || m.showChecks {
		// Only ctrl+c quits from here: esc and q mean "back to the board", so
		// opening an overlay can never cost the user their session by reflex.
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		// A movement key closes the overlay AND moves, so checking a PR then
		// carrying on down the list is one keypress, not two.
		if m.showChecks {
			m.showChecks = false
			switch msg.String() {
			case "j", "down", "k", "up", "l", "right", "h", "left", "g", "home", "G", "end":
				return m.handleKey(msg)
			}
			return m, nil
		}
		m.showHelp = false
		return m, nil
	}
	if m.filtering {
		return m.handleFilterKey(msg)
	}
	switch msg.String() {
	case "q", "ctrl+c", "esc":
		return m, tea.Quit
	case "/":
		m.filtering = true
		return m, nil
	case "?":
		m.showHelp = !m.showHelp
		return m, nil
	case "c":
		if _, ok := m.selected(); ok {
			m.showChecks = true
		}
		return m, nil
	case "j", "down":
		m.cursor++
		m.clampCursor()
	case "k", "up":
		m.cursor--
		m.clampCursor()
	case "l", "right":
		m.cursor = m.nextSection()
		m.clampCursor()
	case "h", "left":
		m.cursor = m.prevSection()
		m.clampCursor()
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		m.cursor = len(m.visibleRows()) - 1
		m.clampCursor()
	case "r":
		return m.refresh()
	case "enter", "o":
		if cmd := m.openSelected(); cmd != nil {
			return m, cmd
		}
	default:
		// User-configured actions are matched last so they cannot shadow
		// navigation keys.
		if cmd, ok := m.actionFor(msg.String()); ok {
			return m, cmd
		}
	}
	return m, nil
}

func (m Model) actionFor(key string) (tea.Cmd, bool) {
	pr, ok := m.selected()
	if !ok {
		return nil, false
	}
	for _, a := range m.cfg.Actions {
		if a.Key != key || strings.TrimSpace(a.Run) == "" {
			continue
		}
		line, err := m.renderAction(a.Run, pr)
		if err != nil {
			return func() tea.Msg { return statusMsg("action: " + err.Error()) }, true
		}
		cmd := exec.Command("sh", "-c", line)
		if a.Mode == "suspend" {
			// Hand the terminal over for TUI commands (a diff pager, a review
			// session), then repaint when they exit.
			return tea.ExecProcess(cmd, func(err error) tea.Msg {
				if err != nil {
					return statusMsg("action failed: " + err.Error())
				}
				return statusMsg("")
			}), true
		}
		return func() tea.Msg {
			if err := cmd.Start(); err != nil {
				return statusMsg("action failed: " + err.Error())
			}
			go cmd.Wait() // reap, so a background action does not become a zombie
			return statusMsg(a.Name)
		}, true
	}
	return nil, false
}

func (m Model) renderAction(tmpl string, pr github.PR) (string, error) {
	t, err := template.New("action").Parse(tmpl)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	err = t.Execute(&b, map[string]any{
		"Number": pr.Number, "Repo": m.cfg.Repo, "RepoPath": m.cfg.RepoPath,
		"Branch": pr.HeadRefName, "Base": pr.BaseRefName,
		"URL": pr.URL, "Author": pr.Author, "Title": pr.Title,
	})
	return b.String(), err
}

// query is the active filter, empty when not filtering. Reading it in one place
// keeps visibleRows and body from ever disagreeing about what is on screen.
func (m Model) query() string {
	if !m.filtering {
		return ""
	}
	return m.filter
}

// sections applies the filter to the board's own sections. A section whose rows
// all fail the query is dropped entirely, header included: while filtering the
// point is to narrow, and an empty header is noise.
func (m Model) sections() []board.Section {
	q := m.query()
	if q == "" {
		return m.board.Sections()
	}
	var out []board.Section
	for _, s := range m.board.Sections() {
		s.Rows = filterSection(s.Rows, q)
		if len(s.Rows) == 0 {
			continue
		}
		out = append(out, s)
	}
	return out
}

// sectionStarts gives the cursor index of each non-empty section's first row,
// so l/h can jump between them without the cursor knowing about sections.
func (m Model) sectionStarts() []int {
	var starts []int
	idx := 0
	for _, s := range m.sections() {
		if len(s.Rows) > 0 {
			starts = append(starts, idx)
			idx += len(s.Rows)
		}
	}
	return starts
}

// nextSection moves to the first row of the following section, or the last row
// when there is none -- the same end-stop behaviour as j.
func (m Model) nextSection() int {
	for _, start := range m.sectionStarts() {
		if start > m.cursor {
			return start
		}
	}
	if n := len(m.visibleRows()); n > 0 {
		return n - 1
	}
	return 0
}

// prevSection moves to the start of the current section, or to the previous
// one when already there, which is how a "back" key is expected to feel.
func (m Model) prevSection() int {
	starts := m.sectionStarts()
	for i := len(starts) - 1; i >= 0; i-- {
		if starts[i] < m.cursor {
			return starts[i]
		}
	}
	return 0
}

// visibleRows flattens the drawable sections so the cursor can move across
// section boundaries without knowing about them.
func (m Model) visibleRows() []board.Row {
	var rows []board.Row
	for _, s := range m.sections() {
		// Stale rows are drawn, so they must be navigable too.
		rows = append(rows, s.Rows...)
	}
	return rows
}

func (m Model) selected() (github.PR, bool) {
	rows := m.visibleRows()
	if m.cursor < 0 || m.cursor >= len(rows) {
		return github.PR{}, false
	}
	return rows[m.cursor].PR, true
}

// Sections resolve progressively, so the row under the cursor can disappear
// between frames.
func (m *Model) clampCursor() {
	n := len(m.visibleRows())
	if n == 0 {
		m.cursor = 0
		return
	}
	if m.cursor >= n {
		m.cursor = n - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

// lineMeta says which section a rendered line belongs to and where the line
// sits within it, so the top row's gutter can be resolved after window() has
// chosen a start. Without it the label would have to be decided before the
// window is known, which is why every row above a scrolled-off boundary used
// to go unnamed.
type lineMeta struct {
	section  string
	author   bool // the section's rule shows an author column
	row      int  // index within the section, -1 for notes and blanks
	isRow    bool
	rowIndex int // index into the flattened visible rows, -1 when not a row
}

// body renders every section and reports the line each row starts on, so the
// viewport can scroll to the cursor. Every row is exactly one line, but section
// notes and placeholder blanks sit between them, so the line is not the row
// index.
func (m Model) body(spin string) (lines []string, rowStarts []int, meta []lineMeta) {
	idx := 0
	filtering := m.query() != ""
	add := func(line string, mt lineMeta) {
		lines = append(lines, line)
		meta = append(meta, mt)
	}
	addRow := func(row board.Row, s board.Section, i int) {
		rowStarts = append(rowStarts, len(lines))
		gs := sectionContinues
		if i == 0 {
			gs = sectionStarts
		}
		add(m.renderRow(row, idx == m.cursor, s.Rule.Author, s.Rule.Name, gs),
			lineMeta{section: s.Rule.Name, author: s.Rule.Author, row: i,
				isRow: true, rowIndex: idx})
		idx++
	}
	note := func(s board.Section, text string) {
		add(m.renderSectionNote(s.Rule.Name, text),
			lineMeta{section: s.Rule.Name, row: -1, rowIndex: -1})
	}
	for _, s := range m.sections() {
		switch s.State {
		case board.Pending:
			// A section that already has rows keeps them, so only the spinner
			// in the gutter changes while the refetch is in flight.
			for i, row := range s.Rows {
				addRow(row, s, i)
			}
			// On a cold start there is nothing to keep, so hold a placeholder
			// block instead: without it each section that lands pushes every
			// section below it down the screen. While filtering the board is
			// deliberately narrowing, so reserving space fights the point.
			if len(s.Rows) == 0 && !filtering {
				note(s, spin)
				for _, b := range blanks(m.placeholderRows(s.Rule) - 1) {
					add(b, lineMeta{section: s.Rule.Name, row: -1, rowIndex: -1})
				}
			}
		case board.Failed:
			note(s, errorStyle.Render(clip(s.Err.Error(), max(0, m.width-sectionWidth-3))))
		case board.Ready:
			if len(s.Rows) == 0 {
				// A resolved empty section collapses to one line: it knows it
				// has nothing, so holding six blank rows for it would waste
				// most of a short pane. The shrink is the value changing,
				// which is the one reason a row is allowed to move.
				note(s, mutedStyle.Render("—"))
			}
			for i, row := range s.Rows {
				addRow(row, s, i)
			}
		}
	}
	return lines, rowStarts, meta
}

func blanks(n int) []string {
	if n < 1 {
		return nil
	}
	return make([]string, n)
}

// placeholderRows reserves roughly the space a pending section will occupy, so
// the skeleton is close to its final height from the first frame. Capped well
// below the rule's limit: overshooting would scroll real rows off the bottom.
func (m Model) placeholderRows(r config.Rule) int {
	const cap = 6
	n := r.PageSize()
	if n > cap {
		n = cap
	}
	if m.height > 0 {
		if budget := (m.height - 2) / max(1, len(m.cfg.Rules)); n > budget {
			n = budget
		}
	}
	if n < 1 {
		return 1
	}
	return n
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// scrollOff is how many lines of context are kept beyond the cursor, so moving
// down shows what is coming rather than pinning the cursor to the bottom edge.
// lazygit ships 2 and fzf 3; 2 is enough here to always reveal the first line
// of the next PR while costing little of a 20-row pane.
const scrollOff = 2

// window scrolls the body so the cursor keeps scrollOff rows of context on
// whichever edge it is approaching.
//
// The invariant is that ONE keypress scrolls the board by at most ONE line.
// Five earlier attempts could not hold it, because a row's height depended on
// its data and a section header took a line of its own: a single `j` moved the
// world by 0 to 3 lines depending on what happened to be nearby, which is what
// read as jumping. Now every line on the board is a row -- gate names live in
// the `c` overlay and the section name lives in each row's left gutter -- so
// the clamp below is the whole of it. See docs/uniform-rows.md.
//
// Clamping both edges rather than pinning one means the board does not move at
// all while the cursor crosses the middle, which is the conventional behaviour
// (vim's scrolloff, less, fzf).
// It also reports the index of the top visible line, which the sticky header
// and the top row's gutter both need: both name the section of the row you are
// actually looking at, and neither can know that before the slice is chosen.
func window(lines []string, cursorRow, height int, rowStarts []int) ([]string, int) {
	if height <= 0 || len(lines) <= height {
		return lines, 0
	}
	if len(rowStarts) == 0 || cursorRow < 0 {
		return lines[:height], 0
	}
	if cursorRow >= len(rowStarts) {
		cursorRow = len(rowStarts) - 1
	}
	cur := rowStarts[cursorRow]

	// The margin has to fit above and below the cursor or the two clamps fight
	// and the viewport oscillates; a very short pane centres instead.
	off := scrollOff
	if 2*off+1 > height {
		off = (height - 1) / 2
	}

	// Two bounds on the top line, each shifting by exactly one when the cursor
	// does. Between them the top is free, so the board holds still through the
	// middle of the viewport and only moves at the edges.
	start := cur - (height - 1 - off)
	if lo := cur - off; start > lo {
		start = lo
	}
	if last := len(lines) - height; start > last {
		start = last
	}
	if start < 0 {
		start = 0
	}
	return lines[start : start+height], start
}

// The footer carries the repo and the spinner, so no global header row is
// needed: in a 20-row pane every chrome row costs a PR.
// promptLine is the filter's own row, drawn directly above the footer. The
// match count sits on the right where the footer already puts its right-hand
// field, so the two chrome rows share one alignment.
func (m Model) promptLine() string {
	n := len(m.visibleRows())
	right := fmt.Sprintf("%d matches", n)
	if n == 1 {
		right = "1 match"
	}

	const prefix = "  / "
	// The query keeps the tail rather than the head: while typing, the end of
	// what you just entered is the part you are looking at.
	field := m.filter + "▏"
	budget := m.width - lipgloss.Width(prefix) - lipgloss.Width(right) - 2
	if budget < 1 {
		// No honest room for the count at this width, so drop it.
		return accentStyle.Render(prefix) +
			fgStyle.Render(clipLeft(field, max(0, m.width-lipgloss.Width(prefix))))
	}
	field = clipLeft(field, budget)

	gap := budget - lipgloss.Width(field)
	return accentStyle.Render(prefix) + fgStyle.Render(field) +
		strings.Repeat(" ", gap) + mutedStyle.Render(right+"  ")
}

func (m Model) footer(spin string) string {
	left := "  j/k move · l/h section · enter open · c checks · / filter · ? help · q quit"
	if m.filtering {
		left = "  ctrl+n/p move · enter open · esc clear"
	}
	if m.status != "" {
		left = "  " + m.status
	}
	right := m.cfg.Repo
	if spin != "" {
		right += " " + spin
	}
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 1 {
		return mutedStyle.Render(clip(left, m.width))
	}
	return mutedStyle.Render(left + strings.Repeat(" ", gap) + right + "  ")
}

func (m Model) View() string {
	if m.width > 0 && m.width < minWidth {
		return mutedStyle.Render(fmt.Sprintf("  terminal too narrow\n  (need %d cols)", minWidth))
	}

	if m.showHelp {
		return m.helpOverlay()
	}
	if m.showChecks {
		return m.checksOverlay()
	}

	spin := ""
	if m.fetching {
		spin = string(spinFrames[m.spinner%len(spinFrames)])
	}

	foot := m.footer(spin)
	chrome := 1
	if m.filtering {
		// The prompt is a second chrome row, so the body has one line less.
		foot = m.promptLine() + "\n" + foot
		chrome = 2
	}

	lines, rowStarts, meta := m.body(spin)
	start := 0
	if m.height > 0 {
		avail := m.height - chrome
		lines, start = window(lines, m.cursor, avail, rowStarts)
		// Pad to the full height so the prompt and footer stay pinned to the
		// bottom edge instead of floating under a short result set.
		for len(lines) < avail {
			lines = append(lines, "")
		}
	}
	lines = m.nameTopSection(lines, meta, start)
	return strings.Join(lines, "\n") + "\n" + foot
}

// nameTopSection re-renders the top visible line so it carries its section's
// name even when the section began above the window. It keeps the rule at `│`:
// `╷` claims a section starts on this row, which is false here, and the two
// facts are worth keeping apart.
func (m Model) nameTopSection(lines []string, meta []lineMeta, start int) []string {
	if len(lines) == 0 || start >= len(meta) {
		return lines
	}
	mt := meta[start]
	if !mt.isRow || mt.row == 0 {
		return lines
	}
	rows := m.visibleRows()
	if mt.rowIndex < 0 || mt.rowIndex >= len(rows) {
		return lines
	}
	out := append([]string(nil), lines...)
	out[0] = m.renderRow(rows[mt.rowIndex], mt.rowIndex == m.cursor,
		mt.author, mt.section, sectionAbove)
	return out
}
