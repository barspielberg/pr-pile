package ui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/barspielberg/pr-pile/internal/board"
	"github.com/barspielberg/pr-pile/internal/github"
)

func TestActionStartsSynchronously(t *testing.T) {
	m := actionBoard(t, "true")
	next, _, ok := m.actionFor("w")
	if !ok {
		t.Fatal("w did not match the configured action")
	}
	if next.running != "worktree" {
		t.Fatalf("running = %q immediately after actionFor, want worktree", next.running)
	}
	if next.runSeq != m.runSeq+1 {
		t.Fatalf("run sequence = %d, want %d", next.runSeq, m.runSeq+1)
	}
}

// A checked value is inert wherever the template puts it, so the author's own
// quotes and embeddings work and render what they say.
func TestCheckedFieldsRenderInAnyPosition(t *testing.T) {
	pr := github.PR{Repo: testRepo, Number: 7, HeadRefName: "feat/x_1+2", BaseRefName: "main",
		URL: "https://github.com/acme/web/pull/7", Author: "octo-cat"}
	for tmpl, want := range map[string]string{
		`printf '%s' {{.Branch}}`:                           "feat/x_1+2",
		`printf '%s' "{{.Branch}}"`:                         "feat/x_1+2",
		`printf '%s' pr-{{.Branch}}-on-{{.Base}}`:           "pr-feat/x_1+2-on-main",
		`printf '%s' "$(printf '%s' {{.URL}})"`:             "https://github.com/acme/web/pull/7",
		`printf '%s' {{if .Author}}"by {{.Author}}"{{end}}`: "by octo-cat",
	} {
		t.Run(tmpl, func(t *testing.T) {
			m := New(testCfg(), nil)
			line, err := m.renderAction(tmpl, pr)
			if err != nil {
				t.Fatal(err)
			}
			out, err := runShell(t, line)
			if err != nil {
				t.Fatalf("run %q: %v", line, err)
			}
			if out != want {
				t.Fatalf("output = %q, want %q (command %q)", out, want, line)
			}
		})
	}
}

func TestUnsafeGitHubValuesRefuseTheAction(t *testing.T) {
	values := []string{
		"$(touch PWNED)", "`touch PWNED`", "a;b", "a b", "a'b", `a"b`, `a\b`,
		"a\nb", "a#b", "a*b", "-rf", "", "fé",
	}
	for _, field := range []string{"Branch", "Base", "URL", "Author"} {
		for _, value := range values {
			t.Run(field+"/"+strconv.Quote(value), func(t *testing.T) {
				m := New(testCfg(), nil)
				pr := github.PR{Repo: testRepo,
					HeadRefName: value, BaseRefName: value, URL: value, Author: value}
				line, err := m.renderAction("echo {{."+field+"}}", pr)
				if err == nil {
					t.Fatalf("rendered %q from %q", line, value)
				}
				if !strings.HasPrefix(err.Error(), "refusing ") {
					t.Fatalf("err = %q, want the field's own refusal", err)
				}
			})
		}
	}
}

// The check sits on the value, so no spelling of the field reaches it unchecked.
// These all got past the old check, which read the template text.
func TestEveryTemplateSpellingIsChecked(t *testing.T) {
	for _, tmpl := range []string{
		`echo "{{$.Branch}}"`,
		`{{with $p := .}}echo "{{$p.Branch}}"{{end}}`,
		`{{"\""}} {{.Branch}} {{"\""}}`,
		`{{define "t"}} {{.Branch}} {{end}}echo "{{template "t" .}}"`,
		`echo {{.Branch}} {{- "x"}}`,
		`echo {{printf "%s" .Branch}}`,
		`echo {{call .Branch}}`,
		`echo {{.pr.HeadRefName}}`,
	} {
		t.Run(tmpl, func(t *testing.T) {
			m := New(testCfg(), nil)
			pr := github.PR{Repo: testRepo, HeadRefName: "$(touch PWNED)"}
			if line, err := m.renderAction(tmpl, pr); err == nil {
				t.Fatalf("rendered %q", line)
			}
		})
	}
}

// Printing the data whole must not reach the raw PR behind the checked methods.
func TestTemplateDataCannotBeDumped(t *testing.T) {
	payload := "$(touch PWNED)"
	pr := github.PR{Repo: testRepo, Title: payload, HeadRefName: payload,
		BaseRefName: payload, URL: payload, Author: payload}
	for _, tmpl := range []string{
		`echo {{.}}`, `echo {{$}}`, `echo {{print .}}`, `echo {{js .}}`,
		`echo {{printf "%v" .}}`, `echo {{printf "%+v" $}}`, `echo {{printf "%#v" .}}`,
	} {
		t.Run(tmpl, func(t *testing.T) {
			m := New(testCfg(), nil)
			single, err := m.renderAction(tmpl, pr)
			if err != nil {
				t.Fatal(err)
			}
			multi, err := m.renderMultiAction(tmpl, []github.PR{pr, pr})
			if err != nil {
				t.Fatal(err)
			}
			for _, line := range []string{single, multi} {
				if strings.Contains(line, "PWNED") {
					t.Fatalf("rendered %q, which carries the raw PR", line)
				}
			}
		})
	}
}
func TestTitleIsRefusedWithAWayToGetIt(t *testing.T) {
	for _, tmpl := range []string{`echo {{.Title}}`, `echo "{{$.Title}}"`, `{{with $p := .}}{{$p.Title}}{{end}}`} {
		t.Run(tmpl, func(t *testing.T) {
			m := New(testCfg(), nil)
			_, err := m.renderAction(tmpl, github.PR{Repo: testRepo, Title: "harmless"})
			if err == nil || !strings.Contains(err.Error(), "gh pr view") {
				t.Fatalf("err = %v, want a refusal that points at gh pr view", err)
			}
		})
	}
}

func actionBoardWithPR(t *testing.T, run string, pr github.PR) Model {
	t.Helper()
	m := New(actionCfg(run), nil)
	m.width, m.height = 120, 20
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{pr}})
	m.board.Apply(board.Result{Index: 1})
	m.fetching = false
	m.cursor = m.firstRowSlot()
	return m
}
