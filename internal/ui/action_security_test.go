package ui

import (
	"os"
	"path/filepath"
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

func TestActionTemplateStringsAreShellQuoted(t *testing.T) {
	values := []string{
		"plain",
		"with spaces",
		"semi;colon",
		"$(printf substitution)",
		"`printf backtick`",
		"line one\nline two",
		`back\slash`,
		"single'quote",
		"",
	}
	for _, value := range values {
		t.Run(strconv.Quote(value), func(t *testing.T) {
			m := New(testCfg(), nil)
			line, err := m.renderAction("printf '%s' {{.Title}}", github.PR{Repo: testRepo, Title: value})
			if err != nil {
				t.Fatal(err)
			}
			out, err := runShell(t, line)
			if err != nil {
				t.Fatalf("run %q: %v", line, err)
			}
			if got := string(out); got != value {
				t.Fatalf("output = %q, want literal %q (command %q)", got, value, line)
			}
		})
	}
}

func TestEveryRemoteStringTemplateFieldCannotInjectShell(t *testing.T) {
	for _, field := range []string{"Branch", "Base", "URL", "Author", "Title"} {
		t.Run(field, func(t *testing.T) {
			dir := t.TempDir()
			marker := filepath.Join(dir, "injected")
			output := filepath.Join(dir, "output")
			value := "literal; touch " + marker
			m := New(testCfg(), nil)
			pr := github.PR{Repo: testRepo,
				HeadRefName: value,
				BaseRefName: value,
				URL:         value,
				Author:      value,
				Title:       value,
			}
			line, err := m.renderAction("printf '%s' {{."+field+"}} > "+strconv.Quote(output), pr)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := runShellIn(t, dir, line); err != nil {
				t.Fatalf("run %q: %v", line, err)
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("%s executed injected shell syntax; marker error=%v", field, err)
			}
			got, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(string(got)) != value {
				t.Fatalf("output = %q, want %q", got, value)
			}
		})
	}
}

func TestRemoteTemplateFieldsRejectUnsafeShellContexts(t *testing.T) {
	for _, tmpl := range []string{
		`printf '%s' "{{.Title}}"`,
		`printf '%s' '{{.Title}}'`,
		`printf '%s' prefix{{.Title}}`,
		`printf '%s' {{.Title}}suffix`,
		`printf '%s' $(printf '%s' {{.Title}})`,
		`printf '%s' $[ {{.Title}} ]`,
		"cat <<EOF\n{{.Title}}\nEOF",
		`printf '%s' {{index . "Title"}}`,
		`sh -c {{.Title}}`,
		`sh -ce {{.Title}}`,
		`sh -e -c {{.Title}}`,
		`/bin/bash -ec {{.Title}}`,
		`bash --noprofile -c {{.Title}}`,
		`sh -o errexit -c {{.Title}}`,
		`eval {{.Title}}`,
		`"eval" {{.Title}}`,
		`\eval {{.Title}}`,
	} {
		t.Run(strconv.Quote(tmpl), func(t *testing.T) {
			m := New(testCfg(), nil)
			if _, err := m.renderAction(tmpl, github.PR{Repo: testRepo, Title: "remote"}); err == nil {
				t.Fatalf("renderAction accepted unsafe remote-field context %q", tmpl)
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
