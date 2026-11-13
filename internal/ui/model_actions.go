package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/barspielberg/pr-pile/internal/github"
	"github.com/barspielberg/pr-pile/internal/shelltmpl"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) actionFor(key string) (Model, tea.Cmd, bool) {
	prs := m.actionPRs()
	if len(prs) == 0 {
		return m, nil, false
	}
	// A multi action over a big selection runs one command across every PR in
	// it, so it gets the same "this many at once" guard `enter` has: ask first,
	// and capture the set now rather than re-reading it when the answer comes.
	for _, a := range m.cfg.Actions {
		if a.Key == key && strings.TrimSpace(a.Run) != "" &&
			a.Multi && len(prs) >= confirmThreshold {
			// Refuse here rather than after the answer: runAction would turn
			// the confirmed `y` into "still running" with the prompt already
			// gone, losing the action the user just agreed to. A prompt that
			// appears is one that will be honoured.
			if m.running != "" {
				m.status = m.running + " still running"
				m.clearSeq = 0
				return m, nil, true
			}
			m.confirmOpen, m.confirmAction, m.confirmVerb = prs, key, a.Name
			return m, nil, true
		}
	}
	return m.runAction(key, prs)
}

// runAction is actionFor once the PR set is settled -- either straight from the
// board, or handed back by a confirm prompt.
func (m Model) runAction(key string, prs []github.PR) (Model, tea.Cmd, bool) {
	for _, a := range m.cfg.Actions {
		if a.Key != key || strings.TrimSpace(a.Run) == "" {
			continue
		}
		// An action that did not opt in refuses a selection rather than
		// running on whichever PR happens to be first. Silently acting on one
		// of several is the failure that takes longest to notice, and the
		// singular fields cannot mean a list without breaking every config
		// written before selections existed.
		if !a.Multi && len(prs) > 1 {
			return m.setStatus(a.Name + ": one PR at a time"), nil, true
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
		// A multi action with nothing selected gets the cursor row as a list of
		// one, so its template needs no special case for that.
		var line string
		var err error
		if a.Multi {
			line, err = m.renderMultiAction(a.Run, prs)
		} else {
			line, err = m.renderAction(a.Run, prs[0])
		}
		if err != nil {
			return m.setStatus("action: " + err.Error()), nil, true
		}
		dir, err := m.actionDir(a.Run, prs)
		if err != nil {
			return m.setStatus("action: " + err.Error()), nil, true
		}
		m.runSeq++
		m.statusSeq++
		m.clearSeq = 0
		cmd := exec.Command("sh", "-c", line)
		cmd.Dir = dir
		if a.Mode == "suspend" {
			// Hand the terminal over for TUI commands (a diff pager, a review
			// session), then repaint when they exit.
			name := a.Name
			statusSeq := m.statusSeq
			m.clearSelection()
			return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
				if err != nil {
					return asyncStatusMsg{seq: statusSeq, text: name + " failed: " + err.Error()}
				}
				return asyncStatusMsg{seq: statusSeq}
			}), true
		}
		m.running, m.status = a.Name, ""
		m.clearSelection()
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
	if err := shelltmpl.Check(tmpl, false); err != nil {
		return "", err
	}
	repo, repoPath, err := m.actionRepo(tmpl, pr)
	if err != nil {
		return "", err
	}
	return shelltmpl.Render(tmpl, false, shelltmpl.NewData(pr, repo, repoPath))
}

// renderMultiAction renders a `multi: true` action once for the whole
// selection. The plural fields carry the same GitHub data as the singular
// ones, so every element passes the same check.
func (m Model) renderMultiAction(tmpl string, prs []github.PR) (string, error) {
	if err := shelltmpl.Check(tmpl, true); err != nil {
		return "", err
	}
	repo, repoPath, err := m.actionRepo(tmpl, prs[0])
	if err != nil {
		return "", err
	}
	if shelltmpl.MentionsAny(tmpl, shelltmpl.RepoBoundFields) {
		for _, pr := range prs[1:] {
			if !strings.EqualFold(pr.Repo, repo) {
				return "", fmt.Errorf("the selection spans repos, and this action works in one")
			}
		}
	}
	return shelltmpl.Render(tmpl, true, shelltmpl.NewMultiData(prs, repo, repoPath))
}

// actionRepo is the Repo and RepoPath an action sees for a PR. Both come from
// the PR's entry in repos: rather than from GitHub, so they stay trusted config
// text. A template that uses RepoPath refuses a repo without a path: expanded
// empty and unquoted, it would shift every argument after it, or cd into $HOME.
func (m Model) actionRepo(tmpl string, pr github.PR) (repo, repoPath string, err error) {
	r, ok := m.repoOf(pr)
	if !ok {
		return "", "", fmt.Errorf("%s is not in repos", terminalText(pr.Repo))
	}
	if r.Path == "" && shelltmpl.Mentions(tmpl, "RepoPath") {
		return "", "", fmt.Errorf(".RepoPath needs a path for %s in repos", r.Tag())
	}
	return r.Name, r.Path, nil
}

// actionDir is where an action runs: the PR's checkout when its repo has a
// path, so a command like `tuicr pr {{.Number}}` finds the right #12. Without
// one it runs where pile was started, as it always has. On a board with
// several repos that is a guess, so an action that needs a repo and does not
// name one with .Repo or .RepoPath refuses instead.
func (m Model) actionDir(tmpl string, prs []github.PR) (string, error) {
	r, ok := m.repoOf(prs[0])
	if !ok {
		return "", nil
	}
	for _, pr := range prs[1:] {
		if !strings.EqualFold(pr.Repo, r.Name) {
			return "", nil
		}
	}
	if r.Path != "" {
		return expandHome(r.Path), nil
	}
	if m.multiRepo() && shelltmpl.MentionsAny(tmpl, shelltmpl.RepoBoundFields) &&
		!shelltmpl.Mentions(tmpl, "Repo") && !shelltmpl.Mentions(tmpl, "RepoPath") {
		return "", fmt.Errorf("set a path for %s in repos, so this runs in its checkout", r.Tag())
	}
	return "", nil
}

// expandHome resolves a leading ~, which the shell would have done for a path
// written into the command but nothing does for a working directory.
func expandHome(path string) string {
	rest, ok := strings.CutPrefix(path, "~")
	if !ok || (rest != "" && rest[0] != '/') {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, rest)
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
