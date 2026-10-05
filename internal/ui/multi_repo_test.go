package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/barspielberg/pr-pile/internal/board"
	"github.com/barspielberg/pr-pile/internal/config"
	"github.com/barspielberg/pr-pile/internal/github"
)

// multiRepoBoard is a board over o/r, o/api and x/lib, with #1 in both o/r
// and o/api. Only o/r has a path.
func multiRepoBoard(t *testing.T) Model {
	t.Helper()
	cfg := testCfg()
	cfg.Repos = []config.Repo{{Name: "o/r", Path: "/src/r"}, {Name: "o/api"}, {Name: "x/lib"}}
	m := New(cfg, nil)
	m.width, m.height = 120, 20
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{
		{Repo: "o/r", Number: 1, Title: "fix: here"},
		{Repo: "o/api", Number: 1, Title: "feat(auth): there"},
		{Repo: "x/lib", Number: 2, Title: "elsewhere"},
	}})
	m.board.Apply(board.Result{Index: 1})
	m.fetching = false
	m.cursor = m.firstRowSlot()
	return m
}

func boardLine(t *testing.T, m Model, title string) string {
	t.Helper()
	for _, line := range strings.Split(stripANSI(m.View()), "\n") {
		if strings.Contains(line, title) {
			return line
		}
	}
	t.Fatalf("no line with %q in:\n%s", title, stripANSI(m.View()))
	return ""
}

// Every row names its repo in a column of its own, so no repo is implied by a
// blank, and the columns after it stay lined up.
func TestEveryRowNamesItsRepo(t *testing.T) {
	m := multiRepoBoard(t)
	at := -1
	for title, cell := range map[string]string{"here": "r     #1", "there": "api   #1", "elsewhere": "lib   #2"} {
		line := []rune(boardLine(t, m, title))
		i := strings.Index(string(line[2:]), cell)
		if i < 0 {
			t.Errorf("%s: want %q leading the row, got %q", title, cell, string(line))
			continue
		}
		if at == -1 {
			at = i
		} else if i != at {
			t.Errorf("%s: repo column at %d, want %d: %q", title, i, at, string(line))
		}
	}
	if line := boardLine(t, m, "there"); strings.Contains(line, "api feat") {
		t.Errorf("the label should not run into the title: %q", line)
	}
}

// With one repo every row would carry the same word, so there is no column.
func TestSingleRepoBoardHasNoRepoColumn(t *testing.T) {
	m := New(testCfg(), nil)
	text, _ := m.searchText(board.Row{PR: github.PR{Repo: testRepo, Number: 1, Title: "t"}}, false)
	if !strings.HasPrefix(text, "#1") {
		t.Errorf("a single-repo row should start at its number: %q", text)
	}
	if got := m.prRef(github.PR{Repo: testRepo, Number: 1}); got != "#1" {
		t.Errorf("prRef = %q, want #1", got)
	}
}

func TestSearchMatchesTheRepoColumn(t *testing.T) {
	m := multiRepoBoard(t)
	text, cells := m.searchText(board.Row{PR: github.PR{Repo: "o/api", Number: 1, Title: "t"}}, false)
	if got := string([]rune(text)[cells.repo[0]:cells.repo[1]]); got != "api" {
		t.Errorf("repo cell = %q in %q", got, text)
	}
	if !textMatches(text, "api") {
		t.Error("a search for the label should match the row")
	}
}

func TestFooterNamesTheRepos(t *testing.T) {
	m := multiRepoBoard(t)
	if got := m.reposText(); got != "r, api, lib" {
		t.Errorf("multi: got %q", got)
	}
	if got := New(testCfg(), nil).reposText(); got != testRepo {
		t.Errorf("single: got %q", got)
	}
}

func TestSameNumberFromTwoReposSelectsSeparately(t *testing.T) {
	m := multiRepoBoard(t)
	m = pressKey(m, " ")
	got := m.selectedPRs()
	if len(got) != 1 || got[0].Repo != "o/r" {
		t.Errorf("space on o/r#1 should select only it, got %v", got)
	}
}

// The header stays #N; the page names the repo in full.
func TestDetailListsTheRepo(t *testing.T) {
	m := multiRepoBoard(t)
	m = pressKey(m, "j")
	out := stripANSI(m.detailOverlay())
	if !strings.HasPrefix(out, "  #1 ") {
		t.Errorf("detail header: want #1, got %q", strings.SplitN(out, "\n", 2)[0])
	}
	if !strings.Contains(out, "repo      o/api") {
		t.Errorf("detail page should list o/api:\n%s", out)
	}
	single := New(testCfg(), nil)
	for _, line := range single.stateLines(github.PR{Repo: testRepo, Number: 1, Author: "a"}) {
		if strings.Contains(stripANSI(line), "repo") {
			t.Errorf("a single-repo board should not list the repo: %q", line)
		}
	}
}

func TestActionGetsThePRsOwnRepo(t *testing.T) {
	m := multiRepoBoard(t)
	for _, c := range []struct {
		tmpl string
		pr   github.PR
		want string
	}{
		{"echo {{.Repo}} {{.RepoPath}} {{.Number}}", github.PR{Repo: "o/r", Number: 1}, "echo o/r /src/r 1"},
		{"echo {{.Repo}} {{.Number}}", github.PR{Repo: "o/api", Number: 1}, "echo o/api 1"},
		// The name comes from repos:, not from how GitHub spelled it.
		{"echo {{.Repo}}", github.PR{Repo: "O/API", Number: 1}, "echo o/api"},
	} {
		got, err := m.renderAction(c.tmpl, c.pr)
		if err != nil || got != c.want {
			t.Errorf("%+v: want %q, got %q (%v)", c.pr, c.want, got, err)
		}
	}
}

// Empty and unquoted, RepoPath would shift the arguments after it, or cd into
// $HOME, so an action using it refuses a repo without a path, alone or in a
// selection.
func TestRepoPathActionRefusesARepoWithoutAPath(t *testing.T) {
	m := multiRepoBoard(t)
	other := github.PR{Repo: "o/api", Number: 1}
	if _, err := m.renderAction("pr-workspace {{.RepoPath}} {{.Number}}", other); err == nil {
		t.Error("single: want an error for .RepoPath on o/api")
	}
	if _, err := m.renderMultiAction("cd {{.RepoPath}} && echo {{.Numbers}}", []github.PR{other}); err == nil {
		t.Error("multi: want an error for .RepoPath on o/api")
	}
}

// Repo goes into the command unquoted, so it only ever comes from repos:. A
// PR from anywhere else has nothing trusted to put there.
func TestActionRefusesARepoNotInRepos(t *testing.T) {
	m := multiRepoBoard(t)
	if _, err := m.renderAction("echo {{.Repo}}", github.PR{Repo: "o/a;rm -rf ~", Number: 1}); err == nil {
		t.Error("want an error for a repo that is not declared")
	}
}

// Numbers and branches repeat across repos, so a selection that spans them
// only runs an action that does not depend on which repo it is in.
func TestMultiActionAcrossReposRefusesRepoBoundFields(t *testing.T) {
	m := multiRepoBoard(t)
	prs := []github.PR{{Repo: "o/r", Number: 1, URL: "u1"}, {Repo: "o/api", Number: 1, URL: "u2"}}
	for _, tmpl := range []string{"echo {{.Repo}} {{.Numbers}}", "gh pr merge {{.Numbers}}", "echo {{.Branches}}"} {
		if _, err := m.renderMultiAction(tmpl, prs); err == nil {
			t.Errorf("%q over two repos should refuse", tmpl)
		}
	}
	if got, err := m.renderMultiAction("open {{.URLs}}", prs); err != nil || got != "open 'u1' 'u2'" {
		t.Errorf("a URL-only action should run: %q, %v", got, err)
	}
}

// An action runs in its PR's checkout, so `tuicr pr {{.Number}}` finds the
// right #12 whichever repo pile was started from.
func TestActionRunsInThePRsCheckout(t *testing.T) {
	m := multiRepoBoard(t)
	home, _ := os.UserHomeDir()
	m.cfg.Repos[0].Path = "~/src/r"
	for _, c := range []struct {
		tmpl string
		prs  []github.PR
		dir  string
		ok   bool
	}{
		{"tuicr pr {{.Number}}", []github.PR{{Repo: "o/r", Number: 1}}, filepath.Join(home, "src/r"), true},
		// o/api has no path: #1 alone would be a guess at which repo.
		{"tuicr pr {{.Number}}", []github.PR{{Repo: "o/api", Number: 1}}, "", false},
		// Naming the repo makes the command say where it acts.
		{"gh pr view {{.Number}} -R {{.Repo}}", []github.PR{{Repo: "o/api", Number: 1}}, "", true},
		{"open {{.URL}}", []github.PR{{Repo: "o/api", Number: 1}}, "", true},
		{"open {{.URLs}}", []github.PR{{Repo: "o/r", Number: 1}, {Repo: "o/api", Number: 1}}, "", true},
	} {
		dir, err := m.actionDir(c.tmpl, c.prs)
		if (err == nil) != c.ok || dir != c.dir {
			t.Errorf("%q on %v: got dir %q, err %v; want dir %q, ok %v", c.tmpl, c.prs[0].Key(), dir, err, c.dir, c.ok)
		}
	}
}

// One repo is what every config was before repos:, so a repo without a path
// keeps running where pile was started.
func TestSingleRepoActionRunsWherePileStarted(t *testing.T) {
	m := New(testCfg(), nil)
	if dir, err := m.actionDir("tuicr pr {{.Number}}", []github.PR{{Repo: testRepo, Number: 1}}); err != nil || dir != "" {
		t.Errorf("got dir %q, err %v", dir, err)
	}
}

// Each repo is asked once for its own PRs, and the answers file under the PR
// they belong to even when both repos have a #1.
func TestWatchPollsEachRepo(t *testing.T) {
	asked := map[string][]int{}
	orig := fetchWatched
	fetchWatched = func(_ context.Context, _ *github.Client, repo string, numbers []int) (map[int]github.Watched, error) {
		asked[repo] = numbers
		got := map[int]github.Watched{}
		for _, n := range numbers {
			got[n] = open(github.PR{Repo: repo, Number: n, Title: repo})
		}
		return got, nil
	}
	t.Cleanup(func() { fetchWatched = orig })

	m := multiRepoBoard(t)
	m.client = &github.Client{}
	for _, pr := range []github.PR{{Repo: "o/r", Number: 1}, {Repo: "o/api", Number: 1}} {
		m.watched[pr.Key()] = &watchEntry{pr: pr}
	}
	m, cmd := m.pollNow()
	msg := cmd().(watchMsg)
	if len(asked) != 2 || len(asked["o/r"]) != 1 || len(asked["o/api"]) != 1 {
		t.Fatalf("want one poll per repo, got %v", asked)
	}
	m, _ = m.applyWatch(msg)
	for _, key := range []github.Key{{Repo: "o/r", Number: 1}, {Repo: "o/api", Number: 1}} {
		if e := m.watched[key]; e == nil || e.pr.Title != key.Repo {
			t.Errorf("%v: want it watched with its own repo's answer, got %+v", key, e)
		}
	}
}

func TestRepoMatchesWhateverItsCase(t *testing.T) {
	m := multiRepoBoard(t)
	if tag := m.repoTag(github.PR{Repo: "O/Api", Number: 1}); tag != "api" {
		t.Errorf("O/Api should resolve to the api label, got %q", tag)
	}
}

// A repo that stops answering costs its own watches one poll, not anyone
// else's: the healthy repo's change is still reported, and the failing repo's
// PRs stay watched rather than being dropped as not found.
func TestWatchKeepsHealthyReposWhenOneFails(t *testing.T) {
	orig := fetchWatched
	fetchWatched = func(_ context.Context, _ *github.Client, repo string, numbers []int) (map[int]github.Watched, error) {
		if repo == "o/api" {
			return nil, errTest
		}
		return map[int]github.Watched{1: open(github.PR{Repo: repo, Number: 1, CIState: "SUCCESS"})}, nil
	}
	t.Cleanup(func() { fetchWatched = orig })

	m := multiRepoBoard(t)
	m.client = &github.Client{}
	for _, pr := range []github.PR{{Repo: "o/r", Number: 1, CIState: "PENDING"}, {Repo: "o/api", Number: 1}} {
		m.watched[pr.Key()] = &watchEntry{pr: pr}
	}
	m, cmd := m.pollNow()
	m, _ = m.applyWatch(cmd().(watchMsg))

	if e := m.watched[github.Key{Repo: "o/r", Number: 1}]; e == nil || e.pr.CIState != "SUCCESS" {
		t.Errorf("o/r#1 should have taken the poll's answer, got %+v", e)
	}
	if m.watched[github.Key{Repo: "o/api", Number: 1}] == nil {
		t.Error("o/api#1 was dropped when its repo failed to answer")
	}
	if !strings.Contains(m.status, "CI passed") || !strings.Contains(m.status, "watch failed: o/api") {
		t.Errorf("status should carry both the news and the failure: %q", m.status)
	}
}
