package ui

import (
	"fmt"
	"os/exec"
	"strings"
	"text/template"
	"time"

	"github.com/barspielberg/pr-pile/internal/github"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) actionFor(key string) (Model, tea.Cmd, bool) {
	prs := m.actionPRs()
	if len(prs) == 0 {
		return m, nil, false
	}
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
		m.runSeq++
		m.statusSeq++
		m.clearSeq = 0
		cmd := exec.Command("sh", "-c", line)
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
	if err := validateRemoteActionFields(tmpl); err != nil {
		return "", err
	}
	if err := rejectFields(tmpl, pluralActionFields); err != nil {
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

// renderMultiAction renders a `multi: true` action once for the whole
// selection.
//
// The plural fields carry exactly the same attacker-controlled GitHub data as
// the singular ones -- a branch name or a PR title is written by whoever opened
// the PR -- so they go through the SAME validation, and each element is quoted
// by the same shellQuote. A list is a bigger surface, not a safer one.
func (m Model) renderMultiAction(tmpl string, prs []github.PR) (string, error) {
	if err := validateRemoteActionFields(tmpl); err != nil {
		return "", err
	}
	if err := rejectFields(tmpl, singularActionFields); err != nil {
		return "", err
	}
	t, err := template.New("action").Parse(tmpl)
	if err != nil {
		return "", err
	}
	data := multiActionTemplateData{Repo: m.cfg.Repo, RepoPath: m.cfg.RepoPath}
	var numbers, branches, bases, urls, authors, titles []string
	for _, pr := range prs {
		numbers = append(numbers, fmt.Sprint(pr.Number))
		branches = append(branches, shellQuote(pr.HeadRefName))
		bases = append(bases, shellQuote(pr.BaseRefName))
		urls = append(urls, shellQuote(pr.URL))
		authors = append(authors, shellQuote(pr.Author))
		titles = append(titles, shellQuote(pr.Title))
	}
	data.Numbers = strings.Join(numbers, " ")
	data.Branches = strings.Join(branches, " ")
	data.Bases = strings.Join(bases, " ")
	data.URLs = strings.Join(urls, " ")
	data.Authors = strings.Join(authors, " ")
	data.Titles = strings.Join(titles, " ")

	var b strings.Builder
	err = t.Execute(&b, data)
	return b.String(), err
}

type actionTemplateData struct {
	Number            int
	Repo, RepoPath    string
	Branch, Base, URL string
	Author, Title     string
}

type multiActionTemplateData struct {
	Numbers         string
	Repo, RepoPath  string
	Branches, Bases string
	URLs            string
	Authors, Titles string
}

// remoteActionFields is every template field whose value comes from GitHub and
// is therefore attacker-controlled. Each must appear as a bare, standalone
// placeholder at top-level shell context.
//
// The plural forms are here for the same reason the singular ones are. Adding a
// field to the template data without adding it here is the mistake this list
// exists to prevent -- see TestRemoteFieldListCoversEveryStringField, which
// walks the structs by reflection so the list cannot silently fall behind.
var remoteActionFields = []string{
	"Branch", "Base", "URL", "Author", "Title",
	"Branches", "Bases", "URLs", "Authors", "Titles",
}

// A singular template in a multi action (or the reverse) is a config error
// rather than an empty expansion: `{{.URL}}` in a multi action would render
// nothing at all, and an action that silently does the wrong thing is worse
// than one that refuses at startup.
var singularActionFields = []string{"Number", "Branch", "Base", "URL", "Author", "Title"}
var pluralActionFields = []string{"Numbers", "Branches", "Bases", "URLs", "Authors", "Titles"}

// actionUsesToken reports whether a template action references exactly this
// field, splitting on the characters that can delimit a field in a template
// action so a longer field name cannot answer for a shorter one.
func actionUsesToken(action, token string) bool {
	for _, word := range strings.FieldsFunc(action, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '(' || r == ')' || r == '|'
	}) {
		if word == token {
			return true
		}
	}
	return false
}

// rejectFields fails if the template mentions any of the given fields.
func rejectFields(tmpl string, fields []string) error {
	for _, field := range fields {
		if templateMentions(tmpl, field) {
			return fmt.Errorf("field %s does not belong in this action", "."+field)
		}
	}
	return nil
}

// templateMentions reports whether a template uses a given field, matching the
// whole name so .URL does not answer for .URLs.
func templateMentions(tmpl, field string) bool {
	for offset := 0; ; {
		rel := strings.Index(tmpl[offset:], "{{")
		if rel < 0 {
			return false
		}
		start := offset + rel
		closeRel := strings.Index(tmpl[start+2:], "}}")
		if closeRel < 0 {
			return false
		}
		end := start + 2 + closeRel
		for _, token := range strings.FieldsFunc(tmpl[start+2:end], func(r rune) bool {
			return r == ' ' || r == '\t' || r == '(' || r == ')' || r == '|'
		}) {
			if token == "."+field {
				return true
			}
		}
		offset = end + 2
	}
}

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
			// Whole-token, not substring: `.URL` is a prefix of `.URLs`, so a
			// Contains check reports the wrong field and rejects a valid
			// plural template outright.
			if !actionUsesToken(action, token) {
				continue
			}
			if action != token || !standaloneActionField(tmpl, start, end+2) ||
				strings.Contains(tmpl, "<<") || usesIndirectEvaluator(tmpl) ||
				!topLevelShellContext(tmpl[:start]) {
				return fmt.Errorf("remote field %s must be an unquoted standalone placeholder", token)
			}
		}
		offset = end + 2
	}
}

func usesIndirectEvaluator(tmpl string) bool {
	normalized := strings.NewReplacer("'", "", `"`, "", `\`, "").Replace(tmpl)
	words := strings.FieldsFunc(normalized, func(r rune) bool {
		return strings.ContainsRune(" \t\r\n;&|()", r)
	})
	for i, word := range words {
		if slash := strings.LastIndexByte(word, '/'); slash >= 0 {
			word = word[slash+1:]
		}
		if word == "eval" {
			return true
		}
		if word != "sh" && word != "bash" && word != "dash" && word != "zsh" && word != "ksh" {
			continue
		}
		for _, option := range words[i+1:] {
			if strings.HasPrefix(option, "-") && strings.Contains(strings.TrimLeft(option, "-"), "c") {
				return true
			}
		}
	}
	return false
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
