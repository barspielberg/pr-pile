package ui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/barspielberg/pr-pile/internal/board"
	"github.com/barspielberg/pr-pile/internal/config"
	"github.com/barspielberg/pr-pile/internal/github"
)

// multiBoard is selectBoard with one action bound to `x`, multi on or off.
func multiBoard(t *testing.T, run string, multi bool) Model {
	t.Helper()
	cfg := testCfg()
	cfg.Actions = []config.Action{{Key: "x", Name: "act", Run: run, Multi: multi}}
	m := New(cfg, nil)
	m.width, m.height = 120, 20
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{
		{Number: 1, Title: "first", URL: "https://x/1", HeadRefName: "a"},
		{Number: 2, Title: "second", URL: "https://x/2", HeadRefName: "b"},
		{Number: 3, Title: "third", URL: "https://x/3", HeadRefName: "c"},
	}})
	m.board.Apply(board.Result{Index: 1})
	m.fetching = false
	m.cursor = m.firstRowSlot()
	return m
}

// An action that did not opt in must refuse rather than pick one silently.
func TestSingleActionRefusesWithSeveralSelected(t *testing.T) {
	m := multiBoard(t, "true", false)
	m = pressKey(m, "v")
	m = pressKey(m, "j")
	next, _, ok := m.actionFor("x")
	if !ok {
		t.Fatal("the action key should have matched")
	}
	if next.running != "" {
		t.Fatal("no process may start when the action refuses")
	}
	if !strings.Contains(next.status, "one PR at a time") {
		t.Fatalf("status = %q, want a refusal", next.status)
	}
}

func TestSingleActionStillWorksWithOneSelected(t *testing.T) {
	m := pressKey(multiBoard(t, "true", false), " ")
	next, _, ok := m.actionFor("x")
	if !ok || next.running != "act" {
		t.Fatalf("running = %q, ok = %v; one selected should behave like none", next.running, ok)
	}
}

func TestMultiActionRunsOnceForTheWholeSelection(t *testing.T) {
	dir := t.TempDir()
	counter := filepath.Join(dir, "runs")
	m := multiBoard(t, "echo x >> "+counter, true)
	m = pressKey(m, "v")
	m = pressKey(m, "j") // two PRs: under the confirm threshold
	m = pressAction(t, m, "x")
	data, err := os.ReadFile(counter)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(data), "x"); got != 1 {
		t.Fatalf("ran %d times, want exactly 1 for the whole selection", got)
	}
}

func TestMultiActionExpandsPluralFields(t *testing.T) {
	m := multiBoard(t, "printf '%s\\n' {{.URLs}}", true)
	m = pressKey(m, "v")
	m = pressKey(m, "j")
	line, err := m.renderMultiAction("echo {{.Numbers}}", m.actionPRs())
	if err != nil {
		t.Fatal(err)
	}
	if line != "echo 1 2" {
		t.Fatalf("rendered %q, want \"echo 1 2\"", line)
	}
}

// Each element quoted separately, proven by running it through a real shell.
func TestMultiActionPluralFieldsAreQuotedPerElement(t *testing.T) {
	m := New(testCfg(), nil)
	prs := []github.PR{
		{Title: "plain"},
		{Title: "with spaces"},
		{Title: "semi;colon"},
		{Title: "$(printf substitution)"},
	}
	line, err := m.renderMultiAction("printf '%s\\n' {{.Titles}}", prs)
	if err != nil {
		t.Fatal(err)
	}
	out, err := runShell(t, line)
	if err != nil {
		t.Fatalf("run %q: %v", line, err)
	}
	got := strings.Split(strings.TrimRight(out, "\n"), "\n")
	want := []string{"plain", "with spaces", "semi;colon", "$(printf substitution)"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q (command %q)", got, want, line)
	}
}

func TestMultiActionWithNothingSelectedUsesTheCursorRow(t *testing.T) {
	m := multiBoard(t, "true", true)
	line, err := m.renderMultiAction("echo {{.Numbers}}", m.actionPRs())
	if err != nil {
		t.Fatal(err)
	}
	if line != "echo 1" {
		t.Fatalf("rendered %q, want the cursor row as a list of one", line)
	}
}

func TestSingularFieldInMultiTemplateIsAnError(t *testing.T) {
	m := New(testCfg(), nil)
	for _, tmpl := range []string{"echo {{.Number}}", "echo {{.URL}}", "echo {{.Title}}"} {
		if _, err := m.renderMultiAction(tmpl, []github.PR{{Number: 1}}); err == nil {
			t.Errorf("%q was accepted in a multi action", tmpl)
		}
	}
}

func TestPluralFieldInSingleTemplateIsAnError(t *testing.T) {
	m := New(testCfg(), nil)
	for _, tmpl := range []string{"echo {{.Numbers}}", "echo {{.URLs}}"} {
		if _, err := m.renderAction(tmpl, github.PR{Number: 1}); err == nil {
			t.Errorf("%q was accepted in a single action", tmpl)
		}
	}
}

// The direct analogue of TestEveryRemoteStringTemplateFieldCannotInjectShell,
// over the plural fields.
func TestEveryRemotePluralTemplateFieldCannotInjectShell(t *testing.T) {
	for _, field := range []string{"Branches", "Bases", "URLs", "Authors", "Titles"} {
		t.Run(field, func(t *testing.T) {
			dir := t.TempDir()
			marker := filepath.Join(dir, "injected")
			value := "literal; touch " + marker
			pr := github.PR{
				HeadRefName: value, BaseRefName: value, URL: value,
				Author: value, Title: value,
			}
			m := New(testCfg(), nil)
			line, err := m.renderMultiAction("printf '%s' {{."+field+"}}", []github.PR{pr, pr})
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if _, err := runShellIn(t, dir, line); err != nil {
				t.Fatalf("run %q: %v", line, err)
			}
			if _, err := os.Stat(marker); err == nil {
				t.Fatalf("injection succeeded via %s (command %q)", field, line)
			}
		})
	}
}

// A validator that only checked the first element would pass every other test
// in this file.
func TestInjectionFromASecondSelectedPR(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "injected")
	m := New(testCfg(), nil)
	prs := []github.PR{
		{Title: "benign"},
		{Title: "literal; touch " + marker},
	}
	line, err := m.renderMultiAction("printf '%s' {{.Titles}}", prs)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if _, err := runShellIn(t, dir, line); err != nil {
		t.Fatalf("run %q: %v", line, err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatalf("injection succeeded from the SECOND PR (command %q)", line)
	}
}

// A plural placeholder has to be bare and at top-level shell context, exactly
// as the singular ones do. Quoting it, feeding it to another shell, or burying
// it in a substitution all defeat the per-element quoting.
func TestPluralFieldsRejectUnsafeShellContexts(t *testing.T) {
	m := New(testCfg(), nil)
	for _, tmpl := range []string{
		`open "{{.URLs}}"`, "open '{{.URLs}}'",
		"eval {{.URLs}}", "sh -c {{.URLs}}", "bash -c {{.Titles}}",
		"echo $( {{.URLs}} )", "echo `{{.URLs}}`", "cat <<EOF\n{{.URLs}}\nEOF",
	} {
		if _, err := m.renderMultiAction(tmpl, []github.PR{{URL: "u"}}); err == nil {
			t.Errorf("%q was accepted", tmpl)
		}
	}
}

// The guard against someone adding a field to the template data and forgetting
// to validate it. DESIGN-GUIDE.md section 12: an audit nothing re-checks goes stale.
func TestRemoteFieldListCoversEveryStringField(t *testing.T) {
	covered := map[string]bool{}
	for _, f := range remoteActionFields {
		covered[f] = true
	}
	// Repo and RepoPath come from the user's own config, not from GitHub, and
	// Number/Numbers are integer-derived.
	local := map[string]bool{"Repo": true, "RepoPath": true, "Number": true, "Numbers": true}

	for _, data := range []any{actionTemplateData{}, multiActionTemplateData{}} {
		ty := reflect.TypeOf(data)
		for i := 0; i < ty.NumField(); i++ {
			f := ty.Field(i)
			if f.Type.Kind() != reflect.String || local[f.Name] {
				continue
			}
			if !covered[f.Name] {
				t.Errorf("%s.%s is a remote string field but is not in remoteActionFields",
					ty.Name(), f.Name)
			}
		}
	}
}

func TestNumbersFieldNeedsNoQuoting(t *testing.T) {
	m := New(testCfg(), nil)
	line, err := m.renderMultiAction("echo {{.Numbers}}", []github.PR{{Number: 7}, {Number: 9}})
	if err != nil {
		t.Fatal(err)
	}
	if line != "echo 7 9" {
		t.Fatalf("rendered %q, want unquoted integers", line)
	}
}

func TestSecondMultiActionRefusedWhileOneRuns(t *testing.T) {
	m := multiBoard(t, "true", true)
	m = pressKey(m, "v")
	m = pressKey(m, "j")
	next, _, _ := m.actionFor("x")
	if next.running != "act" {
		t.Fatalf("running = %q, want the first run in flight", next.running)
	}
	again, _, _ := next.actionFor("x")
	if !strings.Contains(again.status, "still running") {
		t.Fatalf("status = %q, want the second press refused", again.status)
	}
}

func TestMultiActionClearsTheSelection(t *testing.T) {
	m := multiBoard(t, "true", true)
	m = pressKey(m, "v")
	m = pressKey(m, "j")
	next, _, _ := m.actionFor("x")
	if got := selectedNumbers(next); len(got) != 0 {
		t.Fatalf("selection = %v, want cleared once the action started", got)
	}
}

// shellTimeout bounds a payload that blocks -- a test command that waits on
// stdin or sleeps would otherwise hold the whole runner.
const shellTimeout = 10 * time.Second

// runShell runs a rendered action line the way the board does, contained: a
// per-test temp dir so an injection that ever DOES fire leaves its debris
// somewhere `go test` cleans up, rather than in the working tree or in the
// shared system temp that nothing ever empties.
func runShell(t *testing.T, line string) (string, error) {
	t.Helper()
	return runShellIn(t, t.TempDir(), line)
}

func runShellIn(t *testing.T, dir, line string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), shellTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", line)
	cmd.Dir = dir
	out, err := cmd.Output()
	if ctx.Err() != nil {
		t.Fatalf("command %q did not finish within %s", line, shellTimeout)
	}
	return string(out), err
}

// A multi action runs one command over the whole selection, so it gets the same
// "this many at once" guard `enter` has. `open {{.URLs}}` over a big selection
// is the exact case the prompt exists for.
func TestMultiActionAsksBeforeRunningOnManyPRs(t *testing.T) {
	dir := t.TempDir()
	counter := filepath.Join(dir, "runs")
	m := multiBoard(t, "echo x >> "+counter, true)
	m = pressKey(m, "v")
	m = pressKey(pressKey(m, "j"), "j") // three PRs

	m = pressAction(t, m, "x")
	if len(m.confirmOpen) != 3 {
		t.Fatalf("confirmOpen = %d, want the prompt up for 3 PRs", len(m.confirmOpen))
	}
	if _, err := os.Stat(counter); err == nil {
		t.Fatal("the action ran before the prompt was answered -- the prompt must gate it")
	}
	if got := stripANSI(m.View()); !strings.Contains(got, "act on 3 PRs?") {
		t.Fatalf("prompt not on screen:\n%s", got)
	}

	m = pressAction(t, m, "y") // answer it
	data, err := os.ReadFile(counter)
	if err != nil {
		t.Fatalf("the action should have run once confirmed: %v", err)
	}
	if got := strings.Count(string(data), "x"); got != 1 {
		t.Fatalf("ran %d times, want exactly 1 for the whole selection", got)
	}
}

// A non-multi action is refused outright over a selection, so it must never
// raise a prompt -- there is nothing to confirm.
func TestSingleActionDoesNotAskItRefuses(t *testing.T) {
	m := multiBoard(t, "true", false)
	m = pressKey(m, "v")
	m = pressKey(pressKey(m, "j"), "j")
	next, _, _ := m.actionFor("x")
	if len(next.confirmOpen) != 0 {
		t.Fatalf("confirmOpen = %d, want no prompt for a non-multi action", len(next.confirmOpen))
	}
	if !strings.Contains(next.status, "one PR at a time") {
		t.Fatalf("status = %q, want the refusal", next.status)
	}
}

// The prompt runs the set it asked about, not whatever the selection holds when
// the answer arrives.
func TestMultiActionConfirmUsesTheCapturedSet(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "numbers")
	m := multiBoard(t, "echo {{.Numbers}} > "+out, true)
	m = pressKey(m, "v")
	m = pressKey(pressKey(m, "j"), "j")
	m = pressAction(t, m, "x")

	m.clearSelection()
	m.cursor = m.rowSlot(2)
	m = pressAction(t, m, "y")

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(data)); got != "1 2 3" {
		t.Fatalf("ran on %q, want \"1 2 3\" -- the captured set", got)
	}
}

// A prompt that appears is a prompt that will be honoured. Raising one while an
// action is already in flight meant the confirmed `y` hit runAction's
// still-running guard with the prompt already cleared -- the action the user
// explicitly agreed to never ran, and they had to raise the prompt again.
func TestMultiActionRefusesWhileOneRunsInsteadOfPrompting(t *testing.T) {
	dir := t.TempDir()
	counter := filepath.Join(dir, "runs")
	m := multiBoard(t, "echo x >> "+counter, true)
	m = pressKey(m, "v")
	m = pressKey(pressKey(m, "j"), "j") // three PRs

	m.running = "other"

	next, _, ok := m.actionFor("x")
	if !ok {
		t.Fatal("the action key should have matched")
	}
	if len(next.confirmOpen) != 0 {
		t.Fatalf("confirmOpen = %d, want no prompt while an action is in flight", len(next.confirmOpen))
	}
	if !strings.Contains(next.status, "still running") {
		t.Fatalf("status = %q, want the refusal", next.status)
	}
}

// The end-to-end shape of the same bug: raise the prompt, answer it, and the
// action must either have run or still be offered -- never silently dropped.
func TestConfirmedMultiActionIsNeverSilentlyDropped(t *testing.T) {
	dir := t.TempDir()
	counter := filepath.Join(dir, "runs")
	m := multiBoard(t, "echo x >> "+counter, true)
	m = pressKey(m, "v")
	m = pressKey(pressKey(m, "j"), "j")

	// Busy when the key is pressed AND still busy when the answer arrives --
	// the sequence where confirming used to drop the action on the floor.
	m.running = "other"
	m = pressAction(t, m, "x")

	if len(m.confirmOpen) > 0 {
		m = pressAction(t, m, "y") // answering must not silently lose it
		if _, err := os.Stat(counter); err != nil {
			t.Fatal("the confirmed action never ran: the prompt was cleared and runAction refused")
		}
		return
	}

	// Refused up front instead. The selection has to survive so the retry is
	// one keypress once the other action finishes.
	if !strings.Contains(m.status, "still running") {
		t.Fatalf("status = %q, want a refusal when no prompt was raised", m.status)
	}
	if got := selectedNumbers(m); len(got) != 3 {
		t.Fatalf("selection = %v, want it kept for the retry", got)
	}
	m.running = ""
	m = pressAction(t, m, "x")
	if len(m.confirmOpen) == 0 {
		t.Fatal("no prompt on retry -- the action is unreachable")
	}
	m = pressAction(t, m, "y")
	if _, err := os.Stat(counter); err != nil {
		t.Fatalf("the confirmed action never ran: %v", err)
	}
}

// The overlay is what a user reads mid-session, so a multi action has to
// advertise the prompt there -- otherwise the guard arrives unannounced.
func TestHelpOverlayAdvertisesTheMultiPrompt(t *testing.T) {
	cfg := testCfg()
	cfg.Actions = []config.Action{
		{Key: "b", Name: "browse all", Run: "open {{.URLs}}", Multi: true},
		{Key: "w", Name: "worktree", Run: "true"},
	}
	m := New(cfg, nil)
	m.width, m.height = 120, 40
	m.showHelp = true
	view := stripANSI(m.View())

	if !strings.Contains(view, "browse all ( whole selection; 3+ asks first )") {
		t.Fatalf("multi action does not advertise the prompt:\n%s", view)
	}
	// A plain action keeps its bare row.
	if strings.Contains(view, "worktree (") {
		t.Fatalf("a non-multi action should carry no hint:\n%s", view)
	}
}
