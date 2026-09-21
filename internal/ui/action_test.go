package ui

import (
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/barspielberg/prs-mng/internal/board"
	"github.com/barspielberg/prs-mng/internal/config"
	"github.com/barspielberg/prs-mng/internal/github"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// actionCfg is a board with one PR and one action bound to `w`, so a test can
// press the key the way the user does rather than calling runAction directly.
func actionCfg(run string) config.Config {
	cfg := testCfg()
	cfg.Actions = []config.Action{{Key: "w", Name: "worktree", Run: run}}
	return cfg
}

func actionBoard(t *testing.T, run string) Model {
	t.Helper()
	m := New(actionCfg(run), nil)
	m.width, m.height = 120, 20
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{
		{Number: 42, Title: "a pr", HeadRefName: "feat/x"},
	}})
	m.board.Apply(board.Result{Index: 1})
	m.fetching = false
	// Slot 0 is the section header, so land on the PR itself.
	m.cursor = m.firstRowSlot()
	return m
}

// pressAction sends a key and drains the returned command, feeding every message it
// produces back through Update -- which is what the Bubble Tea runtime does.
func pressAction(t *testing.T, m Model, key string) Model {
	t.Helper()
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	m = next.(Model)
	return drain(t, m, cmd)
}

func drain(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	for _, msg := range flatten(cmd) {
		// The spin tick would recurse forever, and the auto-clear tick would
		// make every test wait out statusHold. Neither says anything about an
		// action's outcome; the clear is driven explicitly where it is tested.
		switch msg.(type) {
		case spinMsg, clearStatusMsg:
			continue
		}
		next, sub := m.Update(msg)
		m = next.(Model)
		m = drain(t, m, sub)
	}
	return m
}

// flatten runs a command, unwrapping a Batch into the messages of its parts.
func flatten(cmd tea.Cmd) []tea.Msg {
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	var out []tea.Msg
	for _, c := range batch {
		if c != nil {
			out = append(out, flatten(c)...)
		}
	}
	return out
}

// The bug this feature fixes: a background action used to print its name the
// instant it was started, whether it took 7 seconds or failed outright.
func TestActionReportsRunningThenSuccess(t *testing.T) {
	m := actionBoard(t, "true")

	// Start alone, so the footer can be read mid-run.
	next, _, ok := m.actionFor("w")
	if !ok {
		t.Fatal("w did not match the configured action")
	}
	m = next
	started, _ := m.Update(actionStartMsg{name: "worktree", seq: m.runSeq})
	m = started.(Model)

	if m.running != "worktree" {
		t.Fatalf("running = %q, want the action name", m.running)
	}
	out := stripANSI(m.View())
	if !strings.Contains(out, "worktree") {
		t.Error("footer did not name the running action:\n" + out)
	}
	if !strings.ContainsAny(out, string(spinFrames)) {
		t.Error("footer showed no running glyph:\n" + out)
	}

	done, _ := m.Update(actionDoneMsg{name: "worktree", seq: m.runSeq})
	m = done.(Model)
	if m.running != "" {
		t.Errorf("still running after the result landed: %q", m.running)
	}
	if !strings.Contains(m.status, "worktree") {
		t.Errorf("status = %q, want it to name the action", m.status)
	}
}

// Failure persists; success clears itself so the footer stops claiming an
// action is current.
func TestSucceededActionClearsAndFailedActionPersists(t *testing.T) {
	m := actionBoard(t, "true")
	m = pressAction(t, m, "w")
	cleared, _ := m.Update(clearStatusMsg(m.runSeq))
	if s := cleared.(Model).status; s != "" {
		t.Errorf("success did not clear: %q", s)
	}

	m = actionBoard(t, "exit 1")
	m = pressAction(t, m, "w")
	before := m.status
	kept, _ := m.Update(clearStatusMsg(m.runSeq))
	if got := kept.(Model).status; got != before {
		t.Errorf("failure was cleared: %q became %q", before, got)
	}
}

// Pressing the key again while one is out must not start a second process, and
// must not leave the first one's "running" state stranded.
func TestSecondPressWhileRunningIsRefused(t *testing.T) {
	m := actionBoard(t, "true")
	next, _, _ := m.actionFor("w")
	started, _ := next.Update(actionStartMsg{name: "worktree", seq: next.runSeq})
	m = started.(Model)
	seq := m.runSeq

	again, cmd, ok := m.actionFor("w")
	if !ok {
		t.Fatal("the key stopped matching while running")
	}
	if again.runSeq != seq {
		t.Errorf("a refused press started a new run: seq %d -> %d", seq, again.runSeq)
	}
	m = drain(t, again, cmd)
	if !strings.Contains(m.status, "still running") {
		t.Errorf("status = %q, want it to say the action is still running", m.status)
	}

	// The first result still lands and still clears the running state.
	done, _ := m.Update(actionDoneMsg{name: "worktree", seq: seq})
	if r := done.(Model).running; r != "" {
		t.Errorf("running state stranded after the result: %q", r)
	}
}

// A result from a superseded run must not overwrite the newer run's status.
func TestStaleResultIsIgnored(t *testing.T) {
	m := actionBoard(t, "true")
	m.runSeq = 7
	m.running = "worktree"

	done, _ := m.Update(actionDoneMsg{name: "worktree", seq: 3, err: errors.New("stale")})
	got := done.(Model)
	if got.running != "worktree" {
		t.Errorf("a stale result cleared the live run: %q", got.running)
	}
	if got.status != "" {
		t.Errorf("a stale result wrote the status: %q", got.status)
	}
}

// A refresh takes the footer back; a result that lands afterwards must not
// reinstate a running glyph for a board that has moved on.
func TestRefreshDropsTheRunningIndicator(t *testing.T) {
	m := actionBoard(t, "true")
	next, _, _ := m.actionFor("w")
	started, _ := next.Update(actionStartMsg{name: "worktree", seq: next.runSeq})
	m = started.(Model)

	refreshed, _ := m.refresh()
	if r := refreshed.(Model).running; r != "" {
		t.Errorf("refresh kept the running state: %q", r)
	}
}

// The footer is one line whatever the command wrote to stderr; the board's
// one-line-per-row invariant does not get an exception for an error message.
func TestLongFailureStaysOneLine(t *testing.T) {
	m := actionBoard(t, "echo '"+strings.Repeat("boom ", 200)+"' >&2; exit 1")
	m = pressAction(t, m, "w")

	foot := m.footer("")
	if strings.Contains(foot, "\n") {
		t.Error("the footer wrapped:\n" + foot)
	}
	if w := lipgloss.Width(foot); w > m.width {
		t.Errorf("footer width %d exceeds the terminal's %d", w, m.width)
	}
}

// runAction is where a non-zero exit becomes a reported failure, and these
// exercise it directly rather than through a hand-built actionDoneMsg. A test
// that only ever asserts on messages it constructed itself cannot tell a
// working wait from one that never looks at the exit status -- which is how a
// stale binary reporting every action as a success went unnoticed.
func runActionMsg(t *testing.T, line string) actionDoneMsg {
	t.Helper()
	msg := runAction(exec.Command("sh", "-c", line), "worktree", 1)()
	done, ok := msg.(actionDoneMsg)
	if !ok {
		t.Fatalf("runAction returned %T, want actionDoneMsg", msg)
	}
	return done
}

// The exit status has to survive `sh -c`, the stderr capture and the wait, and
// arrive as a non-nil err. This is the bug the user hit: reported as success.
func TestNonZeroExitIsReportedAsFailure(t *testing.T) {
	done := runActionMsg(t, "exit 1")
	if done.err == nil {
		t.Fatal("a command that exited 1 reported no error")
	}
	if got := actionResult(done); !strings.Contains(got, "failed") {
		t.Errorf("footer = %q, want a failure", got)
	}
	if got := actionResult(done); strings.Contains(got, "✓") {
		t.Errorf("footer = %q, a failing command must not show a tick", got)
	}
}

// The tick must mean the command succeeded, not be what the code always emits.
// Asserted against the same function that produces the failure text, so the two
// cannot silently converge.
func TestTickMeansExitZero(t *testing.T) {
	ok := runActionMsg(t, "exit 0")
	if ok.err != nil {
		t.Fatalf("a command that exited 0 reported an error: %v", ok.err)
	}
	if got := actionResult(ok); !strings.Contains(got, "✓") {
		t.Errorf("footer = %q, want a tick", got)
	}

	// Same name, same code path, opposite outcome: the only difference is the
	// exit status, so a tick cannot be unconditional.
	bad := runActionMsg(t, "exit 3")
	if actionResult(ok) == actionResult(bad) {
		t.Errorf("success and failure produced the same footer: %q", actionResult(ok))
	}
}

// The reason `detail` exists: the script's own diagnostic beats "exit status 1".
func TestStderrLastLineReachesTheFooter(t *testing.T) {
	done := runActionMsg(t,
		"echo 'noise on an earlier line' >&2; "+
			"echo 'failed to resolve worktree for pr:999999' >&2; exit 1")
	if done.detail != "failed to resolve worktree for pr:999999" {
		t.Errorf("detail = %q, want the last stderr line", done.detail)
	}
	got := actionResult(done)
	if !strings.Contains(got, "failed to resolve worktree for pr:999999") {
		t.Errorf("footer = %q, want the script's own message", got)
	}
	if strings.Contains(got, "exit status") {
		t.Errorf("footer = %q, want the script's message instead of the exit status", got)
	}
}

// The inverse of the case above, and the other half of why `detail` is a
// separate field: with nothing on stderr there is no diagnostic to prefer, so
// the footer has to fall back to the exit status rather than say only "failed".
func TestSilentFailureFallsBackToTheExitStatus(t *testing.T) {
	done := runActionMsg(t, "exit 7")
	if done.detail != "" {
		t.Errorf("detail = %q, want empty for a command that wrote no stderr", done.detail)
	}
	got := actionResult(done)
	if !strings.Contains(got, "failed") || !strings.Contains(got, "exit status 7") {
		t.Errorf("footer = %q, want the exit status as the fallback", got)
	}
}

// A command that does not exist is distinct from one that exits non-zero: `sh`
// starts fine and writes its own diagnostic, so the exec itself never fails.
func TestMissingCommandIsADistinctFailure(t *testing.T) {
	done := runActionMsg(t, "definitely-not-a-real-command-xyz")
	if done.err == nil {
		t.Fatal("a missing command reported no error")
	}
	got := actionResult(done)
	if !strings.Contains(got, "not found") {
		t.Errorf("footer = %q, want sh's own diagnostic", got)
	}
	if strings.Contains(got, "✓") {
		t.Errorf("footer = %q, a missing command must not show a tick", got)
	}
}

// End to end through Update, so the failure reaches m.status and not just
// actionResult: a correct message that the model drops is still a silent
// failure on screen.
func TestFailureReachesTheFooterThroughUpdate(t *testing.T) {
	m := actionBoard(t, "echo 'herdr not running' >&2; exit 1")
	next, cmd, ok := m.actionFor("w")
	if !ok {
		t.Fatal("no action matched")
	}
	m = drain(t, next, cmd)

	if strings.Contains(m.status, "✓") {
		t.Errorf("status = %q, a failing action showed a tick", m.status)
	}
	if !strings.Contains(m.status, "herdr not running") {
		t.Errorf("status = %q, want the command's stderr", m.status)
	}
	foot := stripANSI(m.footer(""))
	if !strings.Contains(foot, "herdr not running") {
		t.Error("the footer did not carry the failure:\n" + foot)
	}
}
