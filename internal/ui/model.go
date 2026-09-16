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
	m.board = board.New(m.cfg)
	m.fetching = true
	m.status = ""
	return m, tea.Batch(append(m.fetchAll(), spinTick())...)
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c", "esc":
		return m, tea.Quit
	case "j", "down":
		m.cursor++
		m.clampCursor()
	case "k", "up":
		m.cursor--
		m.clampCursor()
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		m.cursor = len(m.visibleRows()) - 1
		m.clampCursor()
	case "r":
		return m.refresh()
	case "enter", "o":
		if pr, ok := m.selected(); ok {
			url := pr.URL
			return m, func() tea.Msg {
				if err := browser.Open(url); err != nil {
					return statusMsg("open failed: " + err.Error())
				}
				return statusMsg("")
			}
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

// visibleRows flattens the drawable sections so the cursor can move across
// section boundaries without knowing about them.
func (m Model) visibleRows() []board.Row {
	var rows []board.Row
	for _, s := range m.board.Sections() {
		if s.State == board.Ready {
			rows = append(rows, s.Rows...)
		}
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
func (m Model) body(spin string) (lines []string, cursorLine int) {
	cursorLine = -1
	idx := 0
	for _, s := range m.board.Sections() {
		header := sectionStyle.Render("  " + s.Rule.Name)
		switch s.State {
		case board.Pending:
			lines = append(lines, "", header+dimStyle.Render("  "+spin+" loading…"))
		case board.Failed:
			lines = append(lines, "", header, redStyle.Render("    "+s.Err.Error()))
		case board.Ready:
			lines = append(lines, "", header+dimStyle.Render(fmt.Sprintf("  %d", len(s.Rows))))
			if len(s.Rows) == 0 {
				lines = append(lines, dimStyle.Render("    —"))
			}
			for _, row := range s.Rows {
				if idx == m.cursor {
					cursorLine = len(lines)
				}
				lines = append(lines, strings.Split(m.renderRow(row, idx == m.cursor), "\n")...)
				idx++
			}
		}
	}
	return lines, cursorLine
}

// window scrolls the body just far enough to keep the cursor visible, rather
// than paging: the board is mostly read by scanning, so keeping neighbouring
// rows stable matters more than centring the selection.
func window(lines []string, cursorLine, height int) []string {
	if height <= 0 || len(lines) <= height {
		return lines
	}
	start := 0
	if cursorLine >= 0 {
		if cursorLine >= height {
			start = cursorLine - height + 1
		}
		if max := len(lines) - height; start > max {
			start = max
		}
	}
	return lines[start : start+height]
}

func (m Model) View() string {
	spin := ""
	if m.fetching {
		spin = string(spinFrames[m.spinner%len(spinFrames)])
	}

	head := headerStyle.Render("  PRs") + "  " + dimStyle.Render(m.cfg.Repo)
	if spin != "" {
		head += "  " + yellowStyle.Render(spin)
	}

	foot := dimStyle.Render("  j/k move · enter open · r reload · q quit")
	if m.status != "" {
		foot = dimStyle.Render("  "+m.status) + "\n" + foot
	}

	lines, cursorLine := m.body(spin)
	// Reserve the header, the blank line above the footer, and the footer
	// itself; whatever is left belongs to the list.
	if m.height > 0 {
		avail := m.height - 2 - strings.Count(foot, "\n") - 1
		lines = window(lines, cursorLine, avail)
	}

	return head + "\n" + strings.Join(lines, "\n") + "\n\n" + foot
}
