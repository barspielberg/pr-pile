package ui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
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
	if err := rejectFields(tmpl, pluralActionFields); err != nil {
		return "", err
	}
	t, err := template.New("action").Parse(tmpl)
	if err != nil {
		return "", err
	}
	repo, repoPath, err := m.actionRepo(tmpl, pr)
	if err != nil {
		return "", err
	}
	return executeAction(t, actionTemplateData{Number: pr.Number, Repo: repo, RepoPath: repoPath, pr: &pr})
}

// renderMultiAction renders a `multi: true` action once for the whole
// selection. The plural fields carry the same GitHub data as the singular
// ones, so every element passes the same check.
func (m Model) renderMultiAction(tmpl string, prs []github.PR) (string, error) {
	if err := rejectFields(tmpl, singularActionFields); err != nil {
		return "", err
	}
	t, err := template.New("action").Parse(tmpl)
	if err != nil {
		return "", err
	}
	repo, repoPath, err := m.actionRepo(tmpl, prs[0])
	if err != nil {
		return "", err
	}
	if templateMentionsAny(tmpl, repoBoundFields) {
		for _, pr := range prs[1:] {
			if !strings.EqualFold(pr.Repo, repo) {
				return "", fmt.Errorf("the selection spans repos, and this action works in one")
			}
		}
	}
	var numbers []string
	for _, pr := range prs {
		numbers = append(numbers, fmt.Sprint(pr.Number))
	}
	return executeAction(t, multiActionTemplateData{
		Numbers: strings.Join(numbers, " "), Repo: repo, RepoPath: repoPath, prs: &prs,
	})
}

// executeAction surfaces a refused field as its own message, not wrapped in
// the template's "executing action at <.Branch>: error calling Branch:".
func executeAction(t *template.Template, data any) (string, error) {
	var b strings.Builder
	if err := t.Execute(&b, data); err != nil {
		var refused fieldError
		if errors.As(err, &refused) {
			return "", refused
		}
		return "", err
	}
	return b.String(), nil
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
	if r.Path == "" && templateMentions(tmpl, "RepoPath") {
		return "", "", fmt.Errorf(".RepoPath needs a path for %s in repos", r.Tag())
	}
	return r.Name, r.Path, nil
}

// The GitHub fields are methods, not fields: the template calls them however
// it spells the access ($.Branch, a with block, a define), so the check runs on
// the value itself instead of on the template text. Only an action that uses a
// field can be refused by it. The raw PR sits behind a pointer because fmt
// prints a nested pointer as an address, so {{.}} or printf "%v" cannot dump it.
type actionTemplateData struct {
	Number         int
	Repo, RepoPath string
	pr             *github.PR
}

func (d actionTemplateData) Branch() (string, error) { return checkRef("branch", d.pr.HeadRefName) }
func (d actionTemplateData) Base() (string, error)   { return checkRef("base", d.pr.BaseRefName) }
func (d actionTemplateData) URL() (string, error)    { return checkURL(d.pr.URL) }
func (d actionTemplateData) Author() (string, error) { return checkLogin(d.pr.Author) }
func (d actionTemplateData) Title() (string, error) {
	return "", errTitleRemoved(".Title", "gh pr view {{.Number}} --repo {{.Repo}} --json title --jq .title")
}

type multiActionTemplateData struct {
	Numbers        string
	Repo, RepoPath string
	prs            *[]github.PR
}

func (d multiActionTemplateData) Branches() (string, error) {
	return d.each(func(pr github.PR) (string, error) { return checkRef("branch", pr.HeadRefName) })
}

func (d multiActionTemplateData) Bases() (string, error) {
	return d.each(func(pr github.PR) (string, error) { return checkRef("base", pr.BaseRefName) })
}

func (d multiActionTemplateData) URLs() (string, error) {
	return d.each(func(pr github.PR) (string, error) { return checkURL(pr.URL) })
}

func (d multiActionTemplateData) Authors() (string, error) {
	return d.each(func(pr github.PR) (string, error) { return checkLogin(pr.Author) })
}

func (d multiActionTemplateData) Titles() (string, error) {
	return "", errTitleRemoved(".Titles", "gh pr view on each of {{.Numbers}}")
}

func (d multiActionTemplateData) each(field func(github.PR) (string, error)) (string, error) {
	values := make([]string, 0, len(*d.prs))
	for _, pr := range *d.prs {
		v, err := field(pr)
		if err != nil {
			return "", err
		}
		values = append(values, v)
	}
	return strings.Join(values, " "), nil
}

// The GitHub fields are written by whoever opened the PR, and the rendered
// line runs through sh -c. Rather than quote them, which only holds where the
// placeholder sits outside any quotes and nothing re-parses the line, each must
// match a pattern with no shell syntax in it. A value that passes is inert in
// any position. The first character is never "-", so none reads as an option.
var (
	safeRef   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/+@,-]*$`)
	safeLogin = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*$`)
	safePRURL = regexp.MustCompile(`^https://github\.com/[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9._-]+/pull/[0-9]+$`)
)

type fieldError string

func (e fieldError) Error() string { return string(e) }

func checkRef(what, ref string) (string, error) {
	if !safeRef.MatchString(ref) {
		return "", fieldError(fmt.Sprintf("refusing %s name %s: only letters, digits and . _ / + @ , - are passed to actions", what, terminalText(strconv.Quote(ref))))
	}
	return ref, nil
}

func checkLogin(login string) (string, error) {
	if !safeLogin.MatchString(login) {
		return "", fieldError("refusing author " + terminalText(strconv.Quote(login)) + ": not a GitHub login")
	}
	return login, nil
}

func checkURL(url string) (string, error) {
	if !safePRURL.MatchString(url) {
		return "", fieldError("refusing URL " + terminalText(strconv.Quote(url)) + ": not a GitHub PR URL")
	}
	return url, nil
}

// The title is free text, so no pattern can make it safe to splice into a
// command. A script that needs it can ask gh for it.
func errTitleRemoved(field, instead string) error {
	return fieldError(field + " is no longer available to actions; use " + instead)
}

// A singular template in a multi action (or the reverse) is an error rather
// than an empty expansion: `{{.URL}}` in a multi action would render nothing at
// all, and an action that silently does the wrong thing is worse than one that
// refuses. Checked when the action runs, not at startup -- these validators
// live here and config cannot import ui.
var singularActionFields = []string{"Number", "Branch", "Base", "URL", "Author", "Title"}
var pluralActionFields = []string{"Numbers", "Branches", "Bases", "URLs", "Authors", "Titles"}

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
// repoBoundFields only mean something inside one repo: #12 or a branch name
// names a different PR, or nothing, in another. URLs, authors and titles do not
// depend on where the command runs.
var repoBoundFields = []string{"Repo", "RepoPath", "Number", "Numbers", "Branch", "Branches", "Base", "Bases"}

func templateMentionsAny(tmpl string, fields []string) bool {
	for _, f := range fields {
		if templateMentions(tmpl, f) {
			return true
		}
	}
	return false
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
	if m.multiRepo() && templateMentionsAny(tmpl, repoBoundFields) &&
		!templateMentions(tmpl, "Repo") && !templateMentions(tmpl, "RepoPath") {
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
