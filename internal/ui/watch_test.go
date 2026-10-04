package ui

import (
	"context"
	"io"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/barspielberg/pr-pile/internal/board"
	"github.com/barspielberg/pr-pile/internal/github"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func watchBoard(t *testing.T) Model {
	t.Helper()
	cfg := testCfg()
	m := New(cfg, nil)
	m.width, m.height = 120, 20
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{
		{Number: 1, Title: "first", CIState: "PENDING", Mergeable: "MERGEABLE", Review: "REVIEW_REQUIRED"},
		{Number: 2, Title: "second", CIState: "PENDING"},
		{Number: 3, Title: "third"},
	}})
	m.board.Apply(board.Result{Index: 1})
	m.fetching = false
	m.cursor = m.firstRowSlot()
	return m
}

func open(pr github.PR) github.Watched { return github.Watched{PR: pr, State: "OPEN"} }

func TestWatchEvents(t *testing.T) {
	base := github.PR{CIState: "PENDING", Review: "REVIEW_REQUIRED", Mergeable: "MERGEABLE"}
	with := func(f func(*github.PR)) github.PR { p := base; f(&p); return p }

	for _, tc := range []struct {
		name string
		now  github.Watched
		want string
	}{
		{"nothing changed", open(base), ""},
		{"ci passed", open(with(func(p *github.PR) { p.CIState = "SUCCESS" })), "CI passed"},
		// The first failing check is the news, while the rest still run.
		{"first failure while running", open(with(func(p *github.PR) { p.FailedGates = []string{"lint"} })), "CI failed: lint"},
		{"approved", open(with(func(p *github.PR) { p.Review = "APPROVED" })), "approved"},
		{"changes requested", open(with(func(p *github.PR) { p.Review = "CHANGES_REQUESTED" })), "changes requested"},
		{"conflicts", open(with(func(p *github.PR) { p.Mergeable = "CONFLICTING" })), "conflicts"},
		{"unknown is not a change", open(with(func(p *github.PR) { p.Mergeable = "UNKNOWN" })), ""},
		{"merged wins over the rest", github.Watched{PR: with(func(p *github.PR) { p.CIState = "SUCCESS" }), State: "MERGED"}, "merged"},
		{"several at once", open(with(func(p *github.PR) { p.CIState = "SUCCESS"; p.Review = "APPROVED" })), "CI passed, approved"},
	} {
		if got := strings.Join(watchEvents(base, tc.now), ", "); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

// A new push after a failure goes back to running without a word, and only the
// next outcome is reported.
func TestWatchEventsStaySilentWhileANewRunStarts(t *testing.T) {
	failed := github.PR{CIState: "FAILURE", FailedGates: []string{"lint"}}
	running := github.PR{CIState: "PENDING"}
	if got := watchEvents(failed, open(running)); len(got) != 0 {
		t.Errorf("failed -> running should be silent, got %q", got)
	}
}

func TestToggleWatchOnAndOff(t *testing.T) {
	m := watchBoard(t)
	m = press(m, runeKey('m'))
	if m.watched[1] == nil {
		t.Fatalf("expected #1 watched, got %v", m.watched)
	}
	if m.status != "watching #1" {
		t.Errorf("status %q", m.status)
	}
	m = press(m, runeKey('m'))
	if m.watched[1] != nil {
		t.Error("a second m should stop the watch, not add another")
	}
}

// A selection where some PRs are watched is watched whole, so one press never
// leaves part of it unwatched.
func TestToggleWatchMixedSelectionWatchesAll(t *testing.T) {
	m := watchBoard(t)
	m = press(m, runeKey('m'))
	m.selection = map[int]bool{1: true, 2: true}
	m = press(m, runeKey('m'))
	if m.watched[1] == nil || m.watched[2] == nil {
		t.Fatalf("expected both watched, got %v", m.watched)
	}
	if len(m.selection) != 0 {
		t.Error("the selection should clear once acted on")
	}
}

func TestWatchSurvivesRefresh(t *testing.T) {
	m := watchBoard(t)
	m = press(m, runeKey('m'))
	next, _ := m.refresh()
	if next.(Model).watched[1] == nil {
		t.Error("a refresh dropped the watch")
	}
}

func TestApplyWatchReportsAndMarksUnseen(t *testing.T) {
	m := watchBoard(t)
	m.selection = map[int]bool{1: true, 2: true}
	m = press(m, runeKey('m'))

	pr1 := m.watched[1].pr
	pr1.CIState = "SUCCESS"
	pr2 := m.watched[2].pr
	pr2.CIState = "PENDING"
	next, cmd := m.applyWatch(polled(map[int]github.Watched{1: open(pr1), 2: open(pr2)}))

	if next.status != "#1 CI passed" {
		t.Errorf("status %q", next.status)
	}
	if !next.watched[1].unseen || next.watched[2].unseen {
		t.Error("only the PR that changed should be unseen")
	}
	if cmd == nil {
		t.Error("a change should send a notification")
	}
	if got, _ := next.watchCell(1); got != newsGlyph {
		t.Errorf("unseen cell %q", got)
	}
	if got, _ := next.watchCell(2); got != watchGlyph {
		t.Errorf("watched cell %q", got)
	}
}

func TestApplyWatchNotifiesEachChangedPR(t *testing.T) {
	var sent []string
	stubNotice(t, func(how, _ string, n notice) error {
		sent = append(sent, how+" "+n.text)
		return nil
	})
	m := watchBoard(t)
	m.selection = map[int]bool{1: true, 2: true}
	m = press(m, runeKey('m'))
	pr1, pr2 := m.watched[1].pr, m.watched[2].pr
	pr1.CIState, pr2.CIState = "SUCCESS", "FAILURE"
	_, cmd := m.applyWatch(polled(map[int]github.Watched{1: open(pr1), 2: open(pr2)}))
	runAll(cmd)
	if strings.Join(sent, "|") != "osc #1 CI passed|osc #2 CI failed" {
		t.Errorf("sent %q", sent)
	}
}

func TestNotifyNoneSendsNothing(t *testing.T) {
	stubNotice(t, func(string, string, notice) error {
		t.Error("notify: none sent a notification")
		return nil
	})
	m := watchBoard(t)
	m.cfg.Watch.Notify = "none"
	runAll(m.notify([]notice{{number: 1, text: "#1 merged"}}))
}

// The command gets the change in its environment, never in its text.
func TestNotifyCommandGetsTheChangeInEnv(t *testing.T) {
	out := t.TempDir() + "/out"
	n := notice{number: 7, url: "https://x/7", title: "$(touch pwned) title", text: "#7 merged"}
	if err := sendNotice("command", `printf '%s|%s|%s|%s' "$PILE_TITLE" "$PILE_MESSAGE" "$PILE_URL" "$PILE_NUMBER" > `+out, n); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(out)
	if string(got) != "pile: #7 merged|$(touch pwned) title|https://x/7|7" {
		t.Errorf("env %q", got)
	}
}

func stubNotice(t *testing.T, f func(how, command string, n notice) error) {
	t.Helper()
	orig := sendNotice
	sendNotice = f
	t.Cleanup(func() { sendNotice = orig })
}

func runAll(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	if batch, ok := cmd().(tea.BatchMsg); ok {
		for _, c := range batch {
			runAll(c)
		}
	}
}

func TestApplyWatchEndsOnMergeAndLeavesTheBoard(t *testing.T) {
	stubNotice(t, func(string, string, notice) error { return nil })
	m := watchBoard(t)
	m = press(m, runeKey('m'))
	next, _ := m.applyWatch(polled(map[int]github.Watched{
		1: {PR: m.watched[1].pr, State: "MERGED"},
	}))
	if next.status != "#1 merged" {
		t.Errorf("status %q", next.status)
	}
	if next.watched[1] != nil {
		t.Error("a merged PR should stop being watched")
	}
	for _, r := range next.visibleRows() {
		if r.PR.Number == 1 {
			t.Error("a merged PR should leave the board without waiting for a refresh")
		}
	}
	// A fetch that still returns it, as a lagging search index can, does not
	// bring it back.
	next.board.Apply(board.Result{Index: 0, PRs: []github.PR{{Number: 1}, {Number: 2}}})
	for _, r := range next.visibleRows() {
		if r.PR.Number == 1 {
			t.Error("a later fetch brought the merged PR back")
		}
	}
}

func TestRefreshKeepsWatchNews(t *testing.T) {
	stubNotice(t, func(string, string, notice) error { return nil })
	m := watchBoard(t)
	m = press(m, runeKey('m'))
	pr := m.watched[1].pr
	pr.CIState = "SUCCESS"
	m, _ = m.applyWatch(polled(map[int]github.Watched{1: open(pr)}))
	next, _ := m.refresh()
	if got := next.(Model).status; got != "#1 CI passed" {
		t.Errorf("refresh wiped the news: %q", got)
	}
}

// Watched PRs are checked on every refresh, timed or `r`, and only then: there
// is one clock, and `r` always answers for them.
func TestRefreshPollsWatchedPRs(t *testing.T) {
	var polls [][]int
	stubWatch(t, func(numbers []int) (map[int]github.Watched, error) {
		polls = append(polls, numbers)
		return nil, nil
	})
	m := watchBoard(t)
	m.client = &github.Client{}
	m.selection = map[int]bool{1: true, 2: true}
	m = press(m, runeKey('m'))
	m.watchInflight = false // the poll the watch started has landed

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if !next.(Model).watchInflight {
		t.Fatal("r did not poll the watched PRs")
	}
	// A second refresh while that poll is out lets it answer rather than
	// sending another, which would report the same change twice.
	if next2, _ := next.(Model).refresh(); !next2.(Model).watchInflight {
		t.Error("the in-flight poll was dropped")
	}

	next, _ = m.Update(tickMsg{seq: m.refreshSeq})
	if !next.(Model).watchInflight {
		t.Error("the timed refresh did not poll the watched PRs")
	}
}

func stubWatch(t *testing.T, f func(numbers []int) (map[int]github.Watched, error)) {
	t.Helper()
	orig := fetchWatched
	fetchWatched = func(_ context.Context, _ *github.Client, _ string, numbers []int) (map[int]github.Watched, error) {
		return f(numbers)
	}
	t.Cleanup(func() { fetchWatched = orig })
}

// UNKNOWN is GitHub still computing, so the baseline keeps the last real
// answer and the conflict that follows is still reported against it.
func TestApplyWatchKeepsMergeableThroughUnknown(t *testing.T) {
	m := watchBoard(t)
	m = press(m, runeKey('m'))
	pr := m.watched[1].pr
	pr.Mergeable = "UNKNOWN"
	m, _ = m.applyWatch(polled(map[int]github.Watched{1: open(pr)}))
	if got := m.watched[1].pr.Mergeable; got != "MERGEABLE" {
		t.Fatalf("baseline mergeable %q", got)
	}
	pr.Mergeable = "CONFLICTING"
	m, _ = m.applyWatch(polled(map[int]github.Watched{1: open(pr)}))
	if m.status != "#1 conflicts" {
		t.Errorf("status %q", m.status)
	}
}

func TestApplyWatchFailureKeepsTheStatus(t *testing.T) {
	m := watchBoard(t)
	m = press(m, runeKey('m'))
	m = m.setStatus("worktree ✓")
	next, _ := m.applyWatch(watchMsg{err: errTest})
	if next.watchInflight {
		t.Error("a failed poll should free the slot for the next refresh")
	}
	if next.status != "worktree ✓" {
		t.Errorf("a failed poll took over the status line: %q", next.status)
	}
}

// The row draws the poll's state once it is newer than the board, and the
// board's own once a refresh lands after it.
func TestRowDrawsTheFresherCopy(t *testing.T) {
	m := watchBoard(t)
	m = press(m, runeKey('m'))
	pr := m.watched[1].pr
	pr.CIState = "SUCCESS"
	m, _ = m.applyWatch(polled(map[int]github.Watched{1: open(pr)}))
	if got := m.statusPR(github.PR{Number: 1, CIState: "PENDING"}).CIState; got != "SUCCESS" {
		t.Errorf("row drew %q, want the poll's SUCCESS", got)
	}
	m.boardAt = time.Now().Add(time.Second)
	if got := m.statusPR(github.PR{Number: 1, CIState: "PENDING"}).CIState; got != "PENDING" {
		t.Errorf("row drew %q, want the newer board's PENDING", got)
	}
}

func TestOpeningClearsUnseen(t *testing.T) {
	m := watchBoard(t)
	m = press(m, runeKey('m'))
	m.watched[1].unseen = true
	m = press(m, runeKey('d'))
	if m.watched[1].unseen {
		t.Error("d should clear the unseen mark")
	}
}

// The flag takes the pick column, so watching moves nothing else on the row.
func TestWatchFlagSitsInThePickColumn(t *testing.T) {
	m := watchBoard(t)
	row := m.board.Sections()[0].Rows[0]
	before := stripANSI(m.renderRow(row, false, false))
	m.watched[row.PR.Number] = &watchEntry{pr: row.PR}
	after := stripANSI(m.renderRow(row, false, false))
	if want := " " + watchGlyph + "  #1"; !strings.HasPrefix(after, want) {
		t.Errorf("row starts %q, want %q", after, want)
	}
	if strings.Replace(after, watchGlyph, " ", 1) != before {
		t.Errorf("watching changed more than the pick cell:\n%q\n%q", before, after)
	}
	if w := lipgloss.Width(after); w != lipgloss.Width(before) {
		t.Errorf("row is %d cells, was %d", w, lipgloss.Width(before))
	}
}

// A pick is short-lived and acted on next, so it wins the shared cell; the
// flag comes back once the selection clears.
func TestPickWinsTheSharedCell(t *testing.T) {
	m := watchBoard(t)
	row := m.board.Sections()[0].Rows[0]
	m.watched[row.PR.Number] = &watchEntry{pr: row.PR, unseen: true}
	m.selection = map[int]bool{row.PR.Number: true}
	if got := stripANSI(m.renderRow(row, false, false)); !strings.HasPrefix(got, " •") {
		t.Errorf("picked row starts %q, want the pick dot", got)
	}
	m.selection = map[int]bool{}
	if got := stripANSI(m.renderRow(row, true, false)); !strings.HasPrefix(got, "▌"+newsGlyph) {
		t.Errorf("selected row starts %q, want the cursor then the flag", got)
	}
}

func TestQuitAsksWhileWatching(t *testing.T) {
	m := watchBoard(t)
	if _, cmd := m.handleKey(runeKey('q')); !isQuit(cmd) {
		t.Fatal("q with nothing running should quit at once")
	}

	m = press(m, runeKey('m'))
	m = press(m, runeKey('q'))
	if !m.confirmQuit {
		t.Fatal("q while watching should ask first")
	}
	if foot := stripANSI(m.footer("")); !strings.Contains(foot, "1 watched. quit anyway?") {
		t.Errorf("footer %q", foot)
	}
	m = press(m, runeKey('x'))
	if m.confirmQuit || m.watched[1] == nil {
		t.Error("any other key should cancel and keep the watch")
	}

	m = press(m, runeKey('q'))
	if _, cmd := m.handleKey(runeKey('y')); !isQuit(cmd) {
		t.Error("y should quit")
	}
	if _, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC}); !isQuit(cmd) {
		t.Error("ctrl+c should quit from the prompt")
	}
}

func TestCtrlCQuitsWithoutAsking(t *testing.T) {
	m := watchBoard(t)
	m = press(m, runeKey('m'))
	if _, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC}); !isQuit(cmd) {
		t.Error("ctrl+c should always quit")
	}
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// The prompt shows over a running action's spinner, since the next key
// answers it; and once what it named has finished it still asks plainly.
func TestQuitPromptStaysOnScreen(t *testing.T) {
	m := watchBoard(t)
	m.running = "worktree"
	m = press(m, runeKey('q'))
	if foot := stripANSI(m.footer("")); !strings.Contains(foot, "worktree running. quit anyway?") {
		t.Errorf("running action hid the prompt: %q", foot)
	}
	m.running = ""
	if foot := stripANSI(m.footer("")); !strings.Contains(foot, "  quit?  y / enter") {
		t.Errorf("prompt with nothing left to name: %q", foot)
	}
}

func TestNotifyKeepsGoingAfterAFailure(t *testing.T) {
	var sent []int
	stubNotice(t, func(_, _ string, n notice) error {
		sent = append(sent, n.number)
		if n.number == 1 {
			return errTest
		}
		return nil
	})
	m := watchBoard(t)
	msg := m.notify([]notice{{number: 1}, {number: 2}, {number: 3}})()
	if len(sent) != 3 {
		t.Errorf("sent %v, want all three", sent)
	}
	if _, ok := msg.(notifyFailedMsg); !ok {
		t.Errorf("the failure was not reported: %#v", msg)
	}
}

// Bubble Tea only sizes and restores a terminal it can see through the
// output, so the locked writer must still be one.
func TestTerminalIsStillATerminalFile(t *testing.T) {
	var out interface {
		io.ReadWriteCloser
		Fd() uintptr
	} = Terminal
	if out.Fd() != os.Stdout.Fd() {
		t.Error("Terminal does not wrap stdout")
	}
	if _, ok := any(Terminal).(io.StringWriter); !ok {
		t.Error("io.WriteString would bypass the lock")
	}
}

// Quitting asks in the same box as the 3+ confirm, listing what it would cut off.
func TestQuitAsksInABoxOverTheBoard(t *testing.T) {
	m := watchBoard(t)
	m.selection = map[int]bool{1: true, 2: true}
	m = press(m, runeKey('m'))
	m.running = "worktree"
	m = press(m, runeKey('q'))
	lines := strings.Split(m.View(), "\n")
	if len(lines) != m.height {
		t.Fatalf("frame is %d lines, want %d", len(lines), m.height)
	}
	at := -1
	for i, l := range lines {
		if at < 0 && strings.Contains(stripANSI(l), "2 watched, worktree running. quit anyway?") {
			at = i
		}
		if w := lipgloss.Width(l); w > m.width {
			t.Fatalf("line %d is %d wide, over the pane's %d", i, w, m.width)
		}
	}
	if at <= 0 || at >= m.height-2 {
		t.Fatalf("question on line %d, want it in a box mid-board:\n%s", at, stripANSI(m.View()))
	}
	plain := stripANSI(m.View())
	for _, want := range []string{"worktree is still running", "#1  first", "#2  second", "y / enter quit · any other key cancels"} {
		if !strings.Contains(plain, want) {
			t.Errorf("frame missing %q:\n%s", want, plain)
		}
	}
}

// Too short for the box, the footer still asks the whole question.
func TestQuitFitsAShortPane(t *testing.T) {
	for h := 2; h <= 8; h++ {
		m := watchBoard(t)
		m = press(m, runeKey('m'))
		m = press(m, runeKey('q'))
		m.height = h
		lines := strings.Split(m.View(), "\n")
		if len(lines) != h {
			t.Fatalf("height %d: frame is %d lines", h, len(lines))
		}
		if !strings.Contains(stripANSI(lines[h-1]), "1 watched. quit anyway?") {
			t.Fatalf("height %d: question not on the footer:\n%s", h, stripANSI(m.View()))
		}
	}
}

// "watching #1" only confirms the keypress, so it goes after statusHold; news
// that took the line in the meantime stays.
func TestWatchConfirmationExpires(t *testing.T) {
	m := watchBoard(t)
	m = press(m, runeKey('m'))
	seq := m.statusSeq
	next, _ := m.Update(expireStatusMsg{seq: seq})
	if got := next.(Model).status; got != "" {
		t.Errorf("status still %q after it expired", got)
	}

	m = m.setStatus("#1 CI passed")
	next, _ = m.Update(expireStatusMsg{seq: seq})
	if got := next.(Model).status; got != "#1 CI passed" {
		t.Errorf("a stale expiry cleared newer news: %q", got)
	}
}

// polled is a poll that asked for exactly the PRs it got back.
func polled(got map[int]github.Watched) watchMsg {
	asked := make([]int, 0, len(got))
	for n := range got {
		asked = append(asked, n)
	}
	sort.Ints(asked)
	return watchMsg{sent: time.Now(), asked: asked, got: got}
}

func TestWatchStartedDuringAPollIsPolledWhenItLands(t *testing.T) {
	stubWatch(t, func([]int) (map[int]github.Watched, error) { return nil, nil })
	m := watchBoard(t)
	m.client = &github.Client{}
	m = press(m, runeKey('m'))
	pr1 := m.watched[1].pr
	m = press(m, runeKey('j'))
	m = press(m, runeKey('m'))
	if m.watched[2] == nil {
		t.Fatal("#2 is not watched")
	}

	next, cmd := m.applyWatch(polled(map[int]github.Watched{1: open(pr1)}))
	if !next.watchInflight || cmd == nil {
		t.Error("#2 was left for the next refresh")
	}
	next.watchInflight = false
	next, _ = next.applyWatch(polled(map[int]github.Watched{1: open(pr1), 2: open(next.watched[2].pr)}))
	if next.watchInflight {
		t.Error("a poll that covered every watch polled again")
	}
}

func TestApplyWatchDropsAPRThatNoLongerResolves(t *testing.T) {
	m := watchBoard(t)
	m.selection = map[int]bool{1: true, 2: true}
	m = press(m, runeKey('m'))
	pr1 := m.watched[1].pr
	next, _ := m.applyWatch(watchMsg{sent: time.Now(), asked: []int{1, 2}, got: map[int]github.Watched{1: open(pr1)}})
	if next.watched[2] != nil {
		t.Error("a PR GitHub no longer returns is still watched")
	}
	if next.watched[1] == nil {
		t.Error("the PR that came back was dropped too")
	}
	if next.status != "#2 not found, stopped watching" {
		t.Errorf("status %q", next.status)
	}
}

func TestMergedPRLeavesTheSelection(t *testing.T) {
	stubNotice(t, func(string, string, notice) error { return nil })
	m := watchBoard(t)
	m = press(m, runeKey('m'))
	m.selection = map[int]bool{1: true}
	next, _ := m.applyWatch(polled(map[int]github.Watched{1: {PR: m.watched[1].pr, State: "MERGED"}}))
	if len(next.selection) != 0 {
		t.Errorf("selection %v still holds the merged PR", next.selection)
	}
}

func TestNotifyFailureKeepsTheNews(t *testing.T) {
	stubNotice(t, func(string, string, notice) error { return nil })
	m := watchBoard(t)
	m = press(m, runeKey('m'))
	pr := m.watched[1].pr
	pr.CIState = "SUCCESS"
	m, _ = m.applyWatch(polled(map[int]github.Watched{1: open(pr)}))
	next, _ := m.Update(notifyFailedMsg{err: errTest})
	got := next.(Model).status
	if !strings.HasPrefix(got, "#1 CI passed · notify failed: ") {
		t.Errorf("status %q", got)
	}
	if next2, _ := next.(Model).refresh(); next2.(Model).status != got {
		t.Error("a refresh wiped the news and its notify failure")
	}
}
