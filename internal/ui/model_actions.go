package ui

import (
	"fmt"
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
			m.status = running + " still running"
			m.clearSeq = 0
			return m, nil, true
		}
		line, err := m.renderAction(a.Run, pr)
		if err != nil {
			return m.setStatus("action: " + err.Error()), nil, true
		}
		m.runSeq++
		m.statusSeq++
		m.clearSeq = 0
		cmd := exec.Command("sh", "-c", line)
		if a.Mode == "suspend" {
			// Hand the terminal over for TUI commands (a diff pager, a review
			// session), then repaint when they exit.
			name := a.Name
			statusSeq := m.statusSeq
			return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
				if err != nil {
					return asyncStatusMsg{seq: statusSeq, text: name + " failed: " + err.Error()}
				}
				return asyncStatusMsg{seq: statusSeq}
			}), true
		}
		m.running, m.status = a.Name, ""
		seq := m.runSeq
		work := runAction(cmd, a.Name, seq, m.statusSeq)
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
func runAction(cmd *exec.Cmd, name string, seq int, statusSeq uint64) tea.Cmd {
	// Bounded, because a command that streams to stderr should not be able to
	// grow the board's memory; the tail is the part that says what failed.
	var errBuf tailWriter
	errBuf.limit = 4096
	cmd.Stderr = &errBuf
	return func() tea.Msg {
		err := cmd.Run()
		return actionDoneMsg{name: name, seq: seq, statusSeq: statusSeq, err: err, detail: errBuf.lastLine()}
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
	if err := validateRemoteActionFields(tmpl); err != nil {
		return "", err
	}
	t, err := template.New("action").Parse(tmpl)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	err = t.Execute(&b, actionTemplateData{
		Number: pr.Number, Repo: m.cfg.Repo, RepoPath: m.cfg.RepoPath,
		Branch: shellQuote(pr.HeadRefName), Base: shellQuote(pr.BaseRefName),
		URL: shellQuote(pr.URL), Author: shellQuote(pr.Author), Title: shellQuote(pr.Title),
	})
	return b.String(), err
}

type actionTemplateData struct {
	Number            int
	Repo, RepoPath    string
	Branch, Base, URL string
	Author, Title     string
}

var remoteActionFields = []string{"Branch", "Base", "URL", "Author", "Title"}

func validateRemoteActionFields(tmpl string) error {
	for offset := 0; ; {
		rel := strings.Index(tmpl[offset:], "{{")
		if rel < 0 {
			return nil
		}
		start := offset + rel
		closeRel := strings.Index(tmpl[start+2:], "}}")
		if closeRel < 0 {
			return nil
		}
		end := start + 2 + closeRel
		action := strings.TrimSpace(tmpl[start+2 : end])
		for _, field := range remoteActionFields {
			token := "." + field
			if !strings.Contains(action, token) {
				continue
			}
			if action != token || !standaloneActionField(tmpl, start, end+2) ||
				strings.Contains(tmpl, "<<") || !topLevelShellContext(tmpl[:start]) {
				return fmt.Errorf("remote field %s must be an unquoted standalone placeholder", token)
			}
		}
		offset = end + 2
	}
}

func standaloneActionField(tmpl string, start, end int) bool {
	return (start == 0 || shellSpace(tmpl[start-1])) &&
		(end == len(tmpl) || shellSpace(tmpl[end]))
}

func shellSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

func topLevelShellContext(prefix string) bool {
	var quote byte
	parenDepth, braceDepth, bracketDepth := 0, 0, 0
	for i := 0; i < len(prefix); i++ {
		if i+1 < len(prefix) && prefix[i:i+2] == "{{" {
			if end := strings.Index(prefix[i+2:], "}}"); end >= 0 {
				i += end + 3
				continue
			}
		}
		c := prefix[i]
		if c == '\\' && quote != '\'' {
			i++
			continue
		}
		switch quote {
		case '\'':
			if c == '\'' {
				quote = 0
			}
			continue
		case '"':
			if c == '"' {
				quote = 0
			}
			continue
		case '`':
			if c == '`' {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"', '`':
			quote = c
		case '$':
			if i+1 < len(prefix) && prefix[i+1] == '(' {
				parenDepth++
				i++
			} else if i+1 < len(prefix) && prefix[i+1] == '{' {
				braceDepth++
				i++
			} else if i+1 < len(prefix) && prefix[i+1] == '[' {
				bracketDepth++
				i++
			}
		case '(':
			if parenDepth > 0 {
				parenDepth++
			}
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '}':
			if braceDepth > 0 {
				braceDepth--
			}
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		}
	}
	return quote == 0 && parenDepth == 0 && braceDepth == 0 && bracketDepth == 0
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
