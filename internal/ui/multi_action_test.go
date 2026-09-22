package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

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
	m = pressKey(pressKey(m, "j"), "j")
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
	out, err := runShell(line)
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
			if _, err := runShell(line); err != nil {
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
	if _, err := runShell(line); err != nil {
		t.Fatalf("run %q: %v", line, err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatalf("injection succeeded from the SECOND PR (command %q)", line)
	}
}

func TestPluralFieldsRejectQuotedPlaceholders(t *testing.T) {
	m := New(testCfg(), nil)
	for _, tmpl := range []string{`open "{{.URLs}}"`, "open '{{.URLs}}'"} {
		if _, err := m.renderMultiAction(tmpl, []github.PR{{URL: "u"}}); err == nil {
			t.Errorf("%q was accepted", tmpl)
		}
	}
}

func TestPluralFieldsRejectIndirectEvaluation(t *testing.T) {
	m := New(testCfg(), nil)
	for _, tmpl := range []string{"eval {{.URLs}}", "sh -c {{.URLs}}", "bash -c {{.Titles}}"} {
		if _, err := m.renderMultiAction(tmpl, []github.PR{{URL: "u"}}); err == nil {
			t.Errorf("%q was accepted", tmpl)
		}
	}
}

func TestPluralFieldsRejectNonTopLevelContext(t *testing.T) {
	m := New(testCfg(), nil)
	for _, tmpl := range []string{"echo $( {{.URLs}} )", "echo `{{.URLs}}`", "cat <<EOF\n{{.URLs}}\nEOF"} {
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

// runShell runs a rendered action line the way the board does, from a
// throwaway directory: an injection test that ever DOES inject should leave its
// debris in a temp dir rather than in the working tree.
func runShell(line string) (string, error) {
	cmd := exec.Command("sh", "-c", line)
	cmd.Dir = os.TempDir()
	out, err := cmd.Output()
	return string(out), err
}
