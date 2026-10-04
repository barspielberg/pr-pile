package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/barspielberg/pr-pile/internal/github"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// watchEntry is one watched PR. pr is the baseline the next poll is diffed
// against: the board's copy when the watch starts, so a change that landed
// between the board's fetch and the keypress is still reported.
type watchEntry struct {
	pr github.PR
	// at is when the poll that produced pr was sent, zero while pr is still
	// the board's copy. A row draws its status from pr only when this is newer
	// than the board's own fetch.
	at time.Time
	// unseen is set when a poll found something and cleared by opening the
	// PR or its detail.
	unseen bool
}

type watchMsg struct {
	sent  time.Time
	asked []int
	got   map[int]github.Watched
	err   error
}

// A mixed set is watched rather than flipped PR by PR, so one press never
// leaves half of what was picked unwatched.
func (m Model) toggleWatch() (Model, tea.Cmd) {
	prs := m.actionPRs()
	if len(prs) == 0 {
		return m, nil
	}
	all := true
	for _, pr := range prs {
		if m.watched[pr.Number] == nil {
			all = false
		}
	}
	m.clearSelection()
	if all {
		for _, pr := range prs {
			delete(m.watched, pr.Number)
		}
		return m.setBriefStatus("stopped watching " + prCount(prs))
	}
	for _, pr := range prs {
		if m.watched[pr.Number] == nil {
			m.watched[pr.Number] = &watchEntry{pr: pr}
		}
	}
	m, expire := m.setBriefStatus("watching " + prCount(prs))
	m, poll := m.pollNow()
	return m, tea.Batch(expire, poll)
}

type expireStatusMsg struct{ seq uint64 }

// setBriefStatus is for a line that only confirms a keypress: the flag on the
// row already says it, so the line goes after statusHold, as an action's ✓
// does. Anything that takes the line in the meantime bumps statusSeq and keeps
// its own.
func (m Model) setBriefStatus(text string) (Model, tea.Cmd) {
	m = m.setStatus(text)
	seq := m.statusSeq
	return m, tea.Tick(statusHold, func(time.Time) tea.Msg { return expireStatusMsg{seq: seq} })
}

// A var so a test can see that a refresh polls without reaching GitHub.
var fetchWatched = func(ctx context.Context, c *github.Client, repo string, numbers []int) (map[int]github.Watched, error) {
	return c.Watch(ctx, repo, numbers)
}

func prCount(prs []github.PR) string {
	if len(prs) == 1 {
		return fmt.Sprintf("#%d", prs[0].Number)
	}
	return fmt.Sprintf("%d PRs", len(prs))
}

// pollNow checks every watched PR. It runs with each board refresh, timed or
// `r`, so there is one interval to configure and `r` always answers for
// watched PRs; and once when a watch starts, so its row is current at once.
// One poll at a time: a refresh landing while one is out lets that one answer.
func (m Model) pollNow() (Model, tea.Cmd) {
	if m.watchInflight || len(m.watched) == 0 || m.client == nil {
		return m, nil
	}
	m.watchInflight = true
	numbers := make([]int, 0, len(m.watched))
	for n := range m.watched {
		numbers = append(numbers, n)
	}
	sort.Ints(numbers)
	client, repo := m.client, m.cfg.Repo
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		sent := time.Now()
		got, err := fetchWatched(ctx, client, repo, numbers)
		return watchMsg{sent: sent, asked: numbers, got: got, err: err}
	}
}

// pollMissed polls again when a watch started while the last poll was out,
// so the new row does not wait for the next refresh.
func (m Model) pollMissed(asked []int) (Model, tea.Cmd) {
	was := make(map[int]bool, len(asked))
	for _, n := range asked {
		was[n] = true
	}
	for n := range m.watched {
		if !was[n] {
			return m.pollNow()
		}
	}
	return m, nil
}

type notice struct {
	number int
	url    string
	title  string
	text   string
}

func (m Model) applyWatch(msg watchMsg) (Model, tea.Cmd) {
	m.watchInflight = false
	if msg.err != nil {
		if m.status == "" {
			m = m.setStatus("watch failed: " + terminalText(msg.err.Error()))
		}
		return m.pollMissed(msg.asked)
	}

	var news []notice
	var gone []string
	for _, n := range msg.asked {
		e := m.watched[n]
		if e == nil {
			// Unwatched while the poll was out.
			continue
		}
		now, ok := msg.got[n]
		if !ok {
			// Deleted, transferred or no longer visible to this token. It may
			// already be off the board, where `m` cannot reach it to stop.
			delete(m.watched, n)
			gone = append(gone, fmt.Sprintf("#%d not found, stopped watching", n))
			continue
		}
		if events := watchEvents(e.pr, now); len(events) > 0 {
			e.unseen = true
			news = append(news, notice{number: n, url: now.URL, title: terminalText(now.Title),
				text: fmt.Sprintf("#%d %s", n, strings.Join(events, ", "))})
		}
		// mergeable is UNKNOWN while GitHub recomputes it, so a flip through
		// UNKNOWN is not a change; keep the last answer it actually gave.
		if !knownMergeable(now.Mergeable) {
			now.Mergeable = e.pr.Mergeable
		}
		e.pr, e.at = now.PR, msg.sent
		if now.State != "OPEN" {
			// Off the board now rather than at the next refresh: the search
			// only returns open PRs, so the row has nothing left to say.
			delete(m.watched, n)
			delete(m.selection, n)
			delete(m.rangeOwned, n)
			m.board.Hide(n)
			m.clampCursor()
		}
	}
	m, poll := m.pollMissed(msg.asked)
	if len(news) == 0 && len(gone) == 0 {
		return m, poll
	}
	var texts []string
	for _, n := range news {
		texts = append(texts, n.text)
	}
	m = m.setStatus(strings.Join(append(texts, gone...), " · "))
	m.watchNews = m.status
	return m, tea.Batch(poll, m.notify(news))
}

// watchEvents is what a person would want to be told about the move from was
// to now. CI reports its outcome, not each check: the first failure, or
// everything passing, the same moments gh pr checks --watch --fail-fast exits.
func watchEvents(was github.PR, now github.Watched) []string {
	switch now.State {
	case "MERGED":
		return []string{"merged"}
	case "CLOSED":
		return []string{"closed"}
	}
	var events []string
	if out := ciOutcome(now.PR); out != ciOutcome(was) {
		switch out {
		case "failed":
			text := "CI failed"
			if len(now.FailedGates) > 0 {
				text += ": " + terminalText(now.FailedGates[0])
			}
			events = append(events, text)
		case "passed":
			events = append(events, "CI passed")
		}
	}
	if now.Review != was.Review {
		switch now.Review {
		case "APPROVED":
			events = append(events, "approved")
		case "CHANGES_REQUESTED":
			events = append(events, "changes requested")
		}
	}
	if knownMergeable(was.Mergeable) && knownMergeable(now.Mergeable) && was.Mergeable != now.Mergeable {
		if now.Mergeable == "CONFLICTING" {
			events = append(events, "conflicts")
		} else {
			events = append(events, "conflicts cleared")
		}
	}
	return events
}

func knownMergeable(s string) bool { return s == "MERGEABLE" || s == "CONFLICTING" }

// ciOutcome counts a single failed check as failed while others still run,
// which is what lets a failure be reported without waiting for the slowest job.
func ciOutcome(pr github.PR) string {
	switch {
	case len(pr.FailedGates) > 0 || pr.CIState == "FAILURE" || pr.CIState == "ERROR":
		return "failed"
	case pr.CIState == "SUCCESS":
		return "passed"
	case pr.CIState == "PENDING" || pr.CIState == "EXPECTED":
		return "running"
	}
	return ""
}

// statusPR is the copy of a PR its status cells draw from: the watch poll's
// when that is fresher than the board, so a row does not say ◐ for minutes
// after the status line said CI passed.
func (m Model) statusPR(pr github.PR) github.PR {
	if e := m.watched[pr.Number]; e != nil && e.at.After(m.boardAt) {
		return e.pr
	}
	return pr
}

// Hollow against filled, so the pair reads with colour stripped the way ○
// does. Plain Unicode rather than Nerd Font: an icon glyph overhangs its cell.
const (
	watchGlyph = "⚐"
	newsGlyph  = "⚑"
)

func (m Model) watchCell(number int) (string, lipgloss.Style) {
	e := m.watched[number]
	switch {
	case e == nil:
		return " ", fgStyle
	case e.unseen:
		return newsGlyph, attentionStyle
	default:
		return watchGlyph, mutedStyle
	}
}

func (m Model) markSeen(prs ...github.PR) {
	for _, pr := range prs {
		if e := m.watched[pr.Number]; e != nil {
			e.unseen = false
		}
	}
}

func (m Model) busyText() string {
	var parts []string
	if n := len(m.watched); n > 0 {
		parts = append(parts, fmt.Sprintf("%d watched", n))
	}
	if m.running != "" {
		parts = append(parts, m.running+" running")
	}
	return strings.Join(parts, ", ")
}

func (m Model) quitQuestion() string {
	// What it named may have finished while it was up.
	if busy := m.busyText(); busy != "" {
		return busy + ". quit anyway?"
	}
	return "quit?"
}

// quitBox lists what quitting would cut off: the running action, then the
// watched PRs the way the board draws them.
func (m Model) quitBox(rows int) []string {
	inner := m.dialogWidth() - 2
	var list []string
	if m.running != "" {
		list = append(list, mutedStyle.Render(pad(clip("  "+m.running+" is still running", inner), inner)))
	}
	list = append(list, m.prLines(m.watchedPRs(), inner)...)
	return m.questionBox(rows, m.quitQuestion(), list, " y / enter quit · any other key cancels ")
}

func (m Model) overlayQuit(lines []string) []string {
	natural := min(len(m.watched), confirmListed) + 4
	if m.running != "" {
		natural++
	}
	return m.overlayDialog(lines, natural, m.quitBox)
}

func (m Model) watchedPRs() []github.PR {
	prs := make([]github.PR, 0, len(m.watched))
	for _, e := range m.watched {
		prs = append(prs, e.pr)
	}
	sort.Slice(prs, func(i, j int) bool { return prs[i].Number < prs[j].Number })
	return prs
}

// quit asks first when it would end watches or a running action, after yazi's
// "There are unfinished tasks, quit anyway?".
func (m Model) quit() (tea.Model, tea.Cmd) {
	if m.busyText() == "" {
		return m, tea.Quit
	}
	m.confirmQuit = true
	return m, nil
}

// The quit prompt defaults to no, like the open confirm: only an explicit yes
// quits, and ctrl+c, which always quits.
func (m Model) handleQuitKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "enter", "ctrl+c":
		return m, tea.Quit
	}
	m.confirmQuit = false
	return m, nil
}
