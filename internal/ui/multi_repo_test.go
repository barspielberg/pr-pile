package ui

import (
	"context"
	"strings"
	"testing"

	"github.com/barspielberg/pr-pile/internal/board"
	"github.com/barspielberg/pr-pile/internal/github"
	"github.com/charmbracelet/lipgloss"
)

// multiRepoBoard is a board whose first rule searched o/r, o/api and x/lib,
// with #1 in both o/r and o/api.
func multiRepoBoard(t *testing.T) Model {
	t.Helper()
	cfg := testCfg()
	cfg.RepoPath = "/src/r"
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

// The configured repo stays untagged, an org sibling is named by its repo
// alone, and another owner's repo is named in full.
func TestRowFromAnotherRepoIsTagged(t *testing.T) {
	m := multiRepoBoard(t)
	if line := boardLine(t, m, "here"); strings.Contains(line, "r fix: here") {
		t.Errorf("configured repo's row is tagged: %q", line)
	}
	if line := boardLine(t, m, "there"); !strings.Contains(line, "api feat(auth): there") {
		t.Errorf("o/api row: want tag api before the title, got %q", line)
	}
	if line := boardLine(t, m, "elsewhere"); !strings.Contains(line, "x/lib elsewhere") {
		t.Errorf("x/lib row: want tag x/lib before the title, got %q", line)
	}
}

// The tag is muted and the commit convention after it still parses.
func TestRepoTagKeepsTheTitleConvention(t *testing.T) {
	parts := titleParts("api feat(auth): there", 4)
	want := []titlePart{partRepo, partRepo, partRepo, partRepo, partType, partType, partType, partType}
	for i, p := range want {
		if parts[i] != p {
			t.Fatalf("rune %d: want part %v, got %v (all: %v)", i, p, parts[i], parts)
		}
	}
	if got := titleParts("api plain words", 4); got[4] != partSubject {
		t.Errorf("a free-form title after a tag should stay subject, got %v", got)
	}
}

func TestSearchMatchesTheRepoTag(t *testing.T) {
	m := multiRepoBoard(t)
	text, _ := m.searchText(board.Row{PR: github.PR{Repo: "o/api", Number: 1, Title: "t"}}, false)
	if !strings.Contains(text, "api t") {
		t.Errorf("search text should carry the tag: %q", text)
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

func TestDetailHeaderNamesTheRepo(t *testing.T) {
	m := multiRepoBoard(t)
	m = pressKey(m, "j")
	if out := stripANSI(m.detailOverlay()); !strings.HasPrefix(out, "  api#1 ") {
		t.Errorf("detail header: want api#1, got %q", strings.SplitN(out, "\n", 2)[0])
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
	} {
		got, err := m.renderAction(c.tmpl, c.pr)
		if err != nil || got != c.want {
			t.Errorf("%+v: want %q, got %q (%v)", c.pr, c.want, got, err)
		}
	}
}

// RepoPath is configured for one repo. Empty and unquoted it would shift the
// arguments after it, or cd into $HOME, so an action using it refuses a PR
// from another repo, alone or in a selection.
func TestRepoPathActionRefusesAnotherRepo(t *testing.T) {
	m := multiRepoBoard(t)
	other := github.PR{Repo: "o/api", Number: 1}
	if _, err := m.renderAction("pr-workspace {{.RepoPath}} {{.Number}}", other); err == nil {
		t.Error("single: want an error for .RepoPath on o/api")
	}
	if _, err := m.renderMultiAction("cd {{.RepoPath}} && echo {{.Numbers}}", []github.PR{other}); err == nil {
		t.Error("multi: want an error for .RepoPath on o/api")
	}
}

// Repo goes into the command unquoted, so one from GitHub has to look like a
// repo name first.
func TestActionRefusesARepoThatIsNotARepoName(t *testing.T) {
	m := multiRepoBoard(t)
	if _, err := m.renderAction("echo {{.Repo}}", github.PR{Repo: "o/a;rm -rf ~", Number: 1}); err == nil {
		t.Error("want an error for a repo that is not owner/name")
	}
}

func TestMultiActionAcrossReposRefusesOnlyWhenItNamesTheRepo(t *testing.T) {
	m := multiRepoBoard(t)
	prs := []github.PR{{Repo: "o/r", Number: 1}, {Repo: "o/api", Number: 1}}
	if _, err := m.renderMultiAction("echo {{.Repo}} {{.Numbers}}", prs); err == nil {
		t.Error("a .Repo action over two repos should refuse")
	}
	if got, err := m.renderMultiAction("echo {{.Numbers}}", prs); err != nil || got != "echo 1 1" {
		t.Errorf("an action without .Repo should run: %q, %v", got, err)
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

func TestConfiguredRepoMatchesWhateverItsCase(t *testing.T) {
	m := multiRepoBoard(t)
	m.cfg.Repo = "O/R"
	if tag := m.repoTag(github.PR{Repo: "o/r", Number: 1}); tag != "" {
		t.Errorf("o/r against a configured O/R should be untagged, got %q", tag)
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

func TestDetailHeaderFitsWithARepoTag(t *testing.T) {
	m := multiRepoBoard(t)
	m.width = 60
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{
		{Repo: "x/a-much-longer-repo-name", Number: 1234, Title: strings.Repeat("long title ", 10)},
	}})
	m.cursor = m.firstRowSlot()
	head := strings.SplitN(m.detailOverlay(), "\n", 2)[0]
	if w := lipgloss.Width(head); w > m.width {
		t.Errorf("header is %d wide on a %d-wide pane: %q", w, m.width, stripANSI(head))
	}
}
