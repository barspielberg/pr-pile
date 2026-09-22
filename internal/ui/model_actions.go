package ui

import (
	"github.com/barspielberg/pr-pile/internal/github"
	tea "github.com/charmbracelet/bubbletea"
	"os/exec"
	"strings"
	"text/template"
	"time"
)

func (m Model) actionFor(key string) (Model, tea.Cmd, bool) {
	pr, ok := m.selected()
	if !ok {
		return m, nil, false
	}
	for _, a := range m.cfg.Actions {
		if a.Key != key || strings.TrimSpace(a.Run) == "" {
			continue
		}
		// A second press while one is still out would start a second process
		// and lose the first's result to the seq check. Saying so is more
		// useful than silently doing nothing.
		if m.running != "" {
			running := m.running
			return m, func() tea.Msg { return statusMsg(running + " still running") }, true
		}
		line, err := m.renderAction(a.Run, pr)
		if err != nil {
			return m, func() tea.Msg { return statusMsg("action: " + err.Error()) }, true
		}
		m.runSeq++
		cmd := exec.Command("sh", "-c", line)
		if a.Mode == "suspend" {
			// Hand the terminal over for TUI commands (a diff pager, a review
			// session), then repaint when they exit.
			name := a.Name
			return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
				if err != nil {
					return statusMsg(name + " failed: " + err.Error())
				}
				return statusMsg("")
			}), true
		}
		m.running, m.status = a.Name, ""
		seq := m.runSeq
		work := runAction(cmd, a.Name, seq)
		if m.fetching || len(m.inflight) > 0 {
			return m, work, true
		}
		return m, tea.Batch(work, spinTick()), true
	}
	return m, nil, false
}

// statusHold is how long a succeeded action holds the footer. Long enough to
// read after looking back from whatever the action opened, short enough that
// the line is not still claiming an action when the user next looks down.
const statusHold = 4 * time.Second

type clearStatusMsg int

func clearStatusIn(d time.Duration, seq int) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return clearStatusMsg(seq) })
}

// runAction waits for the process off the UI goroutine and reports how it went.
// It also reaps, which the fire-and-forget version did only as a side effect of
// throwing the result away.
func runAction(cmd *exec.Cmd, name string, seq int) tea.Cmd {
	// Bounded, because a command that streams to stderr should not be able to
	// grow the board's memory; the tail is the part that says what failed.
	var errBuf tailWriter
	errBuf.limit = 4096
	cmd.Stderr = &errBuf
	return func() tea.Msg {
		err := cmd.Run()
		return actionDoneMsg{name: name, seq: seq, err: err, detail: errBuf.lastLine()}
	}
}

// actionResult is the one line the footer shows when an action ends. A failure
// prefers what the command said on stderr to Go's exit-status wording, which
// names the mechanism and not the problem.
func actionResult(msg actionDoneMsg) string {
	if msg.err == nil {
		return msg.name + " ✓"
	}
	if d := msg.detail; d != "" {
		return msg.name + " failed: " + d
	}
	return msg.name + " failed: " + msg.err.Error()
}

func (m Model) renderAction(tmpl string, pr github.PR) (string, error) {
	t, err := template.New("action").Parse(tmpl)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	err = t.Execute(&b, map[string]any{
		"Number": pr.Number, "Repo": m.cfg.Repo, "RepoPath": m.cfg.RepoPath,
		"Branch": shellQuote(pr.HeadRefName), "Base": shellQuote(pr.BaseRefName),
		"URL": shellQuote(pr.URL), "Author": shellQuote(pr.Author), "Title": shellQuote(pr.Title),
	})
	return b.String(), err
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

type tailWriter struct {
	buf   []byte
	limit int
}

func (w *tailWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	if over := len(w.buf) - w.limit; over > 0 {
		w.buf = w.buf[over:]
	}
	return len(p), nil
}

// lastLine is the final non-blank line, which is where a shell script's
// diagnostic lands. Anything above it is usually a tool's own progress noise.
func (w *tailWriter) lastLine() string {
	lines := strings.Split(strings.TrimRight(string(w.buf), "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if s := strings.TrimSpace(lines[i]); s != "" {
			return s
		}
	}
	return ""
}
