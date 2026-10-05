package ui

import (
	"context"
	"strings"
	"testing"

	"github.com/barspielberg/pr-pile/internal/board"
	"github.com/barspielberg/pr-pile/internal/github"
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

// RepoPath is configured for one repo, so a PR from another gets it empty
// rather than a checkout of the wrong repo.
func TestActionGetsThePRsOwnRepo(t *testing.T) {
	m := multiRepoBoard(t)
	for _, c := range []struct {
		pr   github.PR
		want string
	}{
		{github.PR{Repo: "o/r", Number: 1}, "echo o/r /src/r 1"},
		{github.PR{Repo: "o/api", Number: 1}, "echo o/api  1"},
		{github.PR{Number: 1}, "echo o/r /src/r 1"},
	} {
		got, err := m.renderAction("echo {{.Repo}} {{.RepoPath}} {{.Number}}", c.pr)
		if err != nil || got != c.want {
			t.Errorf("%+v: want %q, got %q (%v)", c.pr, c.want, got, err)
		}
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
