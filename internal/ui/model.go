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

	filtering bool
	filter    string
	showHelp  bool
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
	if m.showHelp {
		// Only ctrl+c quits from here: esc and q mean "back to the board", so
		// opening help can never cost the user their session by reflex.
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
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

// body renders every section and reports which line the cursor landed on, so
// the viewport can scroll to it. A row can occupy two lines (failing gates),
// so the cursor line is not derivable from the row index.
func (m Model) body(spin string) (lines []string, cursorLine, cursorHeight int, anchors, rowStarts []int) {
	cursorLine, cursorHeight = -1, 1
	idx := 0
	filtering := m.query() != ""
	for _, s := range m.sections() {
		switch s.State {
		case board.Pending:
			// A section that already has rows keeps them, so only the count in
			// the header changes while the refetch is in flight.
			lines = append(lines, "")
			anchors = append(anchors, len(lines))
			lines = append(lines, m.renderSectionHeader(s.Rule.Name, spin))
			for _, row := range s.Rows {
				rendered := strings.Split(m.renderRow(row, idx == m.cursor, s.Rule.Author), "\n")
				if idx == m.cursor {
					cursorLine, cursorHeight = len(lines), len(rendered)
				}
				anchors = append(anchors, len(lines))
				rowStarts = append(rowStarts, len(lines))
				lines = append(lines, rendered...)
				idx++
			}
			// On a cold start there is nothing to keep, so hold a placeholder
			// block instead: without it each section that lands pushes every
			// header below it down the screen. While filtering the board is
			// deliberately narrowing, so reserving space fights the point.
			if len(s.Rows) == 0 && !filtering {
				lines = append(lines, blanks(m.placeholderRows(s.Rule))...)
			}
		case board.Failed:
			lines = append(lines, "")
			anchors = append(anchors, len(lines))
			lines = append(lines, m.renderSectionHeader(s.Rule.Name, "!"),
				errorStyle.Render("    "+s.Err.Error()))
		case board.Ready:
			lines = append(lines, "")
			anchors = append(anchors, len(lines))
			lines = append(lines, m.renderSectionHeader(s.Rule.Name, fmt.Sprint(len(s.Rows))))
			if len(s.Rows) == 0 {
				// A resolved empty section collapses to one line: it knows it
				// has nothing, so holding six blank rows for it would waste
				// most of a short pane. The shrink is the value changing,
				// which is the one reason a row is allowed to move.
				lines = append(lines, mutedStyle.Render("    —"))
			}
			for _, row := range s.Rows {
				rendered := strings.Split(m.renderRow(row, idx == m.cursor, s.Rule.Author), "\n")
				if idx == m.cursor {
					cursorLine, cursorHeight = len(lines), len(rendered)
				}
				anchors = append(anchors, len(lines))
				rowStarts = append(rowStarts, len(lines))
				lines = append(lines, rendered...)
				idx++
			}
		}
	}
	// The leading blank before the first section is chrome in a short pane.
	// Everything indexing into lines shifts with it.
	if len(lines) > 0 && lines[0] == "" {
		lines = lines[1:]
		if cursorLine > 0 {
			cursorLine--
		}
		for i := range anchors {
			if anchors[i] > 0 {
				anchors[i]--
			}
		}
		for i := range rowStarts {
			if rowStarts[i] > 0 {
				rowStarts[i]--
			}
		}
	}
	return lines, cursorLine, cursorHeight, anchors, rowStarts
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

// window scrolls the body so the cursor keeps a constant number of whole rows
// beneath it.
//
// The constraint is on the BOTTOM edge: end is the line just past the last row
// that should follow the cursor, and a fixed viewport height then determines
// the top. start = end - height generally does not land on a row boundary, and
// that remainder has to go somewhere -- snapping it away is what made the gap
// under the cursor drift between 0 and 3 rows. So the top row is clipped
// instead. Clipping the top is the degree of freedom that makes the constraint
// satisfiable at all.
func window(lines []string, cursorRow, height int, rowStarts []int) []string {
	if height <= 0 || len(lines) <= height {
		return lines
	}
	if len(rowStarts) == 0 || cursorRow < 0 {
		return lines[:height]
	}
	if cursorRow >= len(rowStarts) {
		cursorRow = len(rowStarts) - 1
	}

	// The bottom edge: just past the LAST LINE of the scrollOff-th row after
	// the cursor. Measuring to the start of the row after it instead would
	// only half-guarantee a two-line row there, which is the residual wobble.
	end := len(lines)
	if last := cursorRow + scrollOff; last < len(rowStarts) {
		if after := last + 1; after < len(rowStarts) {
			end = rowStarts[after]
		}
	}
	start := end - height
	if start < 0 {
		start = 0
		end = height
	}
	// Never scroll past the cursor's own row, and never clip it off the
	// bottom: near either end of the list the margin simply collapses.
	cursorStart := rowStarts[cursorRow]
	cursorEnd := len(lines)
	if cursorRow+1 < len(rowStarts) {
		cursorEnd = rowStarts[cursorRow+1]
	}
	if start > cursorStart {
		start = cursorStart
		if end = start + height; end > len(lines) {
			end = len(lines)
		}
	}
	if cursorEnd > start+height {
		start = cursorEnd - height
		if start < 0 {
			start = 0
		}
		end = start + height
	}
	if end > len(lines) {
		end = len(lines)
	}
	// A clipped row at the top is fine; a blank separator line is not, so skip
	// it and show the header it belongs to.
	if start < len(lines) && strings.TrimSpace(stripSGR(lines[start])) == "" && start+1 < end {
		start++
	}
	return lines[start:end]
}

// snapToAnchor rounds a line offset down to the nearest line that can legally
// be the top of the viewport: a section header or a row's first line.
func snapToAnchor(start int, anchors []int) int {
	best := 0
	for _, a := range anchors {
		if a > start {
			break
		}
		best = a
	}
	return best
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
	left := "  j/k move · l/h section · enter open · / filter · ? help · q quit"
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

	lines, _, _, _, rowStarts := m.body(spin)
	if m.height > 0 {
		avail := m.height - chrome
		lines = window(lines, m.cursor, avail, rowStarts)
		// Pad to the full height so the prompt and footer stay pinned to the
		// bottom edge instead of floating under a short result set.
		for len(lines) < avail {
			lines = append(lines, "")
		}
	}
	return strings.Join(lines, "\n") + "\n" + foot
}
