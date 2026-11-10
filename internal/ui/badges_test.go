package ui

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/barspielberg/pr-pile/internal/badge"
	"github.com/barspielberg/pr-pile/internal/board"
	"github.com/barspielberg/pr-pile/internal/config"
	"github.com/barspielberg/pr-pile/internal/github"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var badgePRs = []github.PR{
	{Repo: testRepo, Number: 1, Title: "feat: short", HeadRefName: "a", Author: "alice", URL: "https://github.com/o/r/pull/1", UpdatedAt: time.Now()},
	{Repo: testRepo, Number: 2, Title: "feat: a much longer title that fills most of the row", HeadRefName: "b", Author: "bob", URL: "https://github.com/o/r/pull/2", UpdatedAt: time.Now()},
}

func badgeBoard(t *testing.T, width int, author bool, defs ...config.Badge) Model {
	t.Helper()
	cfg := config.Config{
		Repos:   []config.Repo{{Name: testRepo}},
		Rules:   []config.Rule{{Name: "Mine", Query: "author:@me", Author: author}},
		Badges:  defs,
		Actions: nil,
	}
	m := New(cfg, nil)
	m.width, m.height = width, 20
	m.board.Apply(board.Result{Index: 0, PRs: badgePRs})
	m.fetching = false
	m.cursor = m.firstRowSlot()
	return m
}

// answer files a script's answer as if it had run.
func answer(m Model, name string, pr github.PR, text string, err error) {
	k := badge.KeyFor(name, pr.Repo, pr.Number)
	m.badges.cache.Take([]badge.Key{k}, time.Now())
	m.badges.cache.Done(badge.Result{Key: k, Text: text, Err: err}, time.Minute, time.Now())
}

func rowLine(t *testing.T, m Model, number int) string {
	t.Helper()
	for _, l := range strings.Split(ansi.Strip(m.View()), "\n") {
		if strings.Contains(l, "#"+strconv.Itoa(number)+" ") {
			return l
		}
	}
	t.Fatalf("no row for #%d in:\n%s", number, ansi.Strip(m.View()))
	return ""
}

func TestLeftBadgeSitsRightAfterTheTitle(t *testing.T) {
	m := badgeBoard(t, 100, false, config.Badge{Name: "jira", Run: "true"})
	before := rowLine(t, m, 1)
	answer(m, "jira", badgePRs[0], "PROJ-1", nil)
	after := rowLine(t, m, 1)
	if !strings.Contains(after, "feat: short PROJ-1") {
		t.Fatalf("badge not after the title:\n%q", after)
	}
	if strings.Index(after, "feat:") != strings.Index(before, "feat:") {
		t.Fatalf("title moved:\n%q\n%q", before, after)
	}
	if lipgloss.Width(after) != lipgloss.Width(before) {
		t.Fatalf("row width changed from %d to %d", lipgloss.Width(before), lipgloss.Width(after))
	}
}

func TestRightBadgeSitsBeforeTheAuthor(t *testing.T) {
	m := badgeBoard(t, 100, true, config.Badge{Name: "ci", Run: "true", Side: "right"})
	before := rowLine(t, m, 1)
	answer(m, "ci", badgePRs[0], "green", nil)
	after := rowLine(t, m, 1)
	if !strings.Contains(after, "green "+initials("alice")) {
		t.Fatalf("badge not just before the author:\n%q", after)
	}
	if lipgloss.Width(after) != lipgloss.Width(before) {
		t.Fatalf("row width changed from %d to %d", lipgloss.Width(before), lipgloss.Width(after))
	}
}

func TestBadgeOutputIsClippedToMaxWidth(t *testing.T) {
	m := badgeBoard(t, 100, false, config.Badge{Name: "jira", Run: "true", MaxWidth: 6})
	answer(m, "jira", badgePRs[0], "In review by two people", nil)
	if line := rowLine(t, m, 1); !strings.Contains(line, "In re…") {
		t.Fatalf("badge not clipped to 6:\n%q", line)
	}
}

// A badge only two cells wide is cut bare, not blanked as clip would.
func TestNarrowBadgeIsCutNotBlanked(t *testing.T) {
	m := badgeBoard(t, 100, false, config.Badge{Name: "x", Run: "true", MaxWidth: 2})
	answer(m, "x", badgePRs[0], "OK!", nil)
	if line := rowLine(t, m, 1); !strings.Contains(line, "feat: short OK") {
		t.Fatalf("narrow badge missing:\n%q", line)
	}
}

// The title gives way down to its floor, then the last badge in config order
// goes.
func TestLongTitleKeepsItsFloorAndDropsTheLastBadge(t *testing.T) {
	m := badgeBoard(t, 76, false,
		config.Badge{Name: "one", Run: "true", MaxWidth: 12},
		config.Badge{Name: "two", Run: "true", MaxWidth: 12, Side: "right"},
		config.Badge{Name: "three", Run: "true", MaxWidth: 12})
	pr := badgePRs[1]
	answer(m, "one", pr, "first-badge", nil)
	answer(m, "two", pr, "second-badge", nil)
	answer(m, "three", pr, "third-badge", nil)
	line := rowLine(t, m, 2)
	if !strings.Contains(line, "first-badge") || !strings.Contains(line, "second-badge") {
		t.Fatalf("lost a badge that fits:\n%q", line)
	}
	if strings.Contains(line, "third-badge") {
		t.Fatalf("kept a badge that does not fit:\n%q", line)
	}
	if !strings.Contains(line, pr.Title[:titleFloor-1]) {
		t.Fatalf("title cut below its floor:\n%q", line)
	}
	if lipgloss.Width(line) > m.width {
		t.Fatalf("row is %d wide on a %d terminal", lipgloss.Width(line), m.width)
	}
}

func TestBadgesDropInTheNarrowTier(t *testing.T) {
	m := badgeBoard(t, 50, false, config.Badge{Name: "jira", Run: "true"})
	answer(m, "jira", badgePRs[0], "PROJ-1", nil)
	if line := rowLine(t, m, 1); strings.Contains(line, "PROJ-1") {
		t.Fatalf("badge drawn in the narrow tier:\n%q", line)
	}
}

func TestRuleWithEmptyBadgeListShowsNone(t *testing.T) {
	m := badgeBoard(t, 100, false, config.Badge{Name: "jira", Run: "true"})
	none := []string{}
	m.cfg.Rules[0].Badges = &none
	m.board = board.New(m.cfg)
	m.board.Apply(board.Result{Index: 0, PRs: badgePRs})
	answer(m, "jira", badgePRs[0], "PROJ-1", nil)
	if line := rowLine(t, m, 1); strings.Contains(line, "PROJ-1") {
		t.Fatalf("badge drawn on a rule with badges: []:\n%q", line)
	}
	if wants := m.badgeWants(); len(wants) != 0 {
		t.Fatalf("scheduled %d runs for a rule that shows no badges", len(wants))
	}
}

// A badge is drawn text, and the search matches what is drawn.
func TestSearchFindsBadgeText(t *testing.T) {
	m := badgeBoard(t, 100, false, config.Badge{Name: "jira", Run: "true"})
	answer(m, "jira", badgePRs[1], "PROJ-42", nil)
	m.query = "proj-42"
	matches := m.matchIndexes()
	if len(matches) != 1 {
		t.Fatalf("matches = %v, want only #2", matches)
	}
	if s, _ := m.slotAt(matches[0]); s.row.PR.Number != 2 {
		t.Fatalf("matched #%d, want #2", s.row.PR.Number)
	}
}

// One line per row, whatever the scripts answer.
func TestBadgesNeverWrapARow(t *testing.T) {
	for _, w := range []int{40, 59, 60, 75, 76, 80, 120} {
		m := badgeBoard(t, w, true,
			config.Badge{Name: "a", Run: "true", MaxWidth: 40},
			config.Badge{Name: "b", Run: "true", MaxWidth: 40, Side: "right"})
		for _, pr := range badgePRs {
			answer(m, "a", pr, strings.Repeat("x", 40), nil)
			answer(m, "b", pr, strings.Repeat("y", 40), nil)
		}
		for i, l := range strings.Split(ansi.Strip(m.View()), "\n") {
			if lipgloss.Width(l) > w {
				t.Fatalf("width %d: line %d is %d wide:\n%q", w, i, lipgloss.Width(l), l)
			}
		}
	}
}

// The scheduler end to end: a board that resolves starts the scripts, and
// their answers land on the rows.
func TestBoardRunsBadgeScriptsAndDrawsTheirAnswers(t *testing.T) {
	cfg := config.Config{
		Repos:  []config.Repo{{Name: testRepo}},
		Rules:  []config.Rule{{Name: "Mine", Query: "author:@me"}},
		Badges: []config.Badge{{Name: "n", Run: "echo pr-{{.Number}}"}},
	}
	m := New(cfg, nil)
	m.width, m.height = 100, 20
	defer m.Close()
	next, cmd := m.Update(resultMsg{generation: m.fetchGeneration, result: board.Result{Index: 0, PRs: badgePRs}})
	m = next.(Model)
	for _, msg := range drainBadges(cmd) {
		if done, ok := msg.(badgeDoneMsg); ok {
			next, _ = m.Update(done)
			m = next.(Model)
		}
	}
	for _, pr := range badgePRs {
		if line := rowLine(t, m, pr.Number); !strings.Contains(line, "pr-"+strconv.Itoa(pr.Number)) {
			t.Fatalf("#%d has no badge:\n%q", pr.Number, line)
		}
	}
}

// drain runs a command and every command it batches, returning the messages
// that are not timers, so a test does not wait out the refresh or the TTL.
func drainBadges(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-done:
	case <-time.After(2 * time.Second):
		return nil // a tea.Tick
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, drainBadges(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

// A field refused by its check fails the badge without starting a process,
// and frees its slot.
func TestRefusedFieldFailsTheBadgeWithoutAProcess(t *testing.T) {
	m := badgeBoard(t, 100, false, config.Badge{Name: "b", Run: "echo {{.Branch}}"})
	m.board = board.New(m.cfg)
	bad := badgePRs[0]
	bad.HeadRefName = "$(touch PWNED)"
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{bad}})
	m.dispatchBadges(time.Now())
	e, _ := m.badges.cache.Get(badge.KeyFor("b", bad.Repo, bad.Number))
	if e.Err == nil || e.Running || !strings.Contains(e.Err.Error(), "refusing branch") {
		t.Fatalf("entry = %+v, want a refusal", e)
	}
	if m.badges.cache.Busy() {
		t.Fatal("the refusal kept its worker slot")
	}
}

func TestDetailPageShowsTheFullBadgeAndItsError(t *testing.T) {
	m := badgeBoard(t, 100, false,
		config.Badge{Name: "jira", Run: "true", MaxWidth: 4},
		config.Badge{Name: "deploy", Run: "true"})
	answer(m, "jira", badgePRs[0], "In review", nil)
	answer(m, "deploy", badgePRs[0], "", errors.New("no such env"))
	got := ansi.Strip(strings.Join(m.badgeLines(badgePRs[0]), "\n"))
	for _, want := range []string{"badges", "jira", "In review · just now", "deploy", "failed: no such env"} {
		if !strings.Contains(got, want) {
			t.Fatalf("detail block lacks %q:\n%s", want, got)
		}
	}
}

func TestHelpListsTheConfiguredBadges(t *testing.T) {
	m := badgeBoard(t, 100, false, config.Badge{Name: "jira", Run: "true", Side: "right"})
	page := strings.Join(m.helpLines(), "\n")
	if !strings.Contains(page, "BADGES") || !strings.Contains(page, "jira") || !strings.Contains(page, "before the author") {
		t.Fatalf("help page does not list the badge:\n%s", ansi.Strip(page))
	}
}

func TestNoBadgesConfiguredDrawsTheRowAsBefore(t *testing.T) {
	m := badgeBoard(t, 100, true)
	if m.badges != nil {
		t.Fatal("a config without badges built a badge runner")
	}
	if line := rowLine(t, m, 1); !strings.Contains(line, "feat: short") {
		t.Fatalf("row = %q", line)
	}
}

func TestFooterNamesABadgeFailingOnEveryRow(t *testing.T) {
	m := badgeBoard(t, 120, false, config.Badge{Name: "jira", Run: "true"}, config.Badge{Name: "ci", Run: "true"})
	for _, pr := range badgePRs {
		answer(m, "jira", pr, "", errors.New("jira-status: not found"))
	}
	answer(m, "ci", badgePRs[0], "", errors.New("flaky"))
	answer(m, "ci", badgePRs[1], "green", nil)
	foot := ansi.Strip(m.footer(""))
	if !strings.Contains(foot, "badge jira failing: jira-status: not found") {
		t.Fatalf("footer = %q, want jira named", foot)
	}
	if strings.Contains(foot, "ci") {
		t.Fatalf("footer = %q, but ci answered on one row", foot)
	}
	m.status = "worktree ✓"
	if foot := ansi.Strip(m.footer("")); !strings.Contains(foot, "worktree ✓") {
		t.Fatalf("footer = %q, want the status to win", foot)
	}
}

// r bypasses the TTL, but waits for current PR data before launching badges.
func TestReloadRerunsBadgesAfterTheRefetch(t *testing.T) {
	m := badgeBoard(t, 100, false, config.Badge{Name: "jira", Run: "true"})
	defer m.Close()
	for _, pr := range badgePRs {
		answer(m, "jira", pr, "old", nil)
	}
	m = pressKey(m, "r")
	if !m.fetching {
		t.Fatal("r did not refetch the board")
	}
	for _, pr := range badgePRs {
		e, _ := m.badges.cache.Get(badge.KeyFor("jira", pr.Repo, pr.Number))
		if e.Running || e.Text != "old" {
			t.Fatalf("#%d entry = %+v, want it waiting with its old text up", pr.Number, e)
		}
	}
	next, _ := m.Update(resultMsg{generation: m.fetchGeneration, result: board.Result{Index: 0, PRs: badgePRs}})
	m = next.(Model)
	for _, pr := range badgePRs {
		e, _ := m.badges.cache.Get(badge.KeyFor("jira", pr.Repo, pr.Number))
		if !e.Running || e.Text != "old" {
			t.Fatalf("#%d entry = %+v, want it rerunning after refetch", pr.Number, e)
		}
	}
}

// The timed refresh is not r: badges keep to their TTL.
func TestTimedRefreshLeavesBadgesToTheirTTL(t *testing.T) {
	m := badgeBoard(t, 100, false, config.Badge{Name: "jira", Run: "true"})
	answer(m, "jira", badgePRs[0], "old", nil)
	next, _ := m.Update(tickMsg{seq: m.refreshSeq})
	m = next.(Model)
	if e, _ := m.badges.cache.Get(badge.KeyFor("jira", testRepo, 1)); e.Running {
		t.Fatal("a timed refresh reran a badge before its TTL")
	}
}

func TestReloadDuringBadgeRunUsesRefetchedPR(t *testing.T) {
	for _, boardFirst := range []bool{false, true} {
		t.Run("board first="+strconv.FormatBool(boardFirst), func(t *testing.T) {
			m := badgeBoard(t, 100, false, config.Badge{Name: "base", Run: "echo {{.Base}}", TTL: time.Hour})
			defer m.Close()
			pr := badgePRs[0]
			pr.BaseRefName = "old-base"
			m.board = board.New(m.cfg)
			m.board.Apply(board.Result{Index: 0, PRs: []github.PR{pr}})
			answer(m, "base", pr, "cached", nil)
			key := badge.KeyFor("base", pr.Repo, pr.Number)
			before, _ := m.badges.cache.Get(key)
			m.badges.cache.Expire([]badge.Key{key})
			oldRun := m.dispatchBadges(time.Now())
			m = pressKey(m, "r")
			m = pressKey(m, "r") // one pending replacement, using the latest fetch
			pr.BaseRefName = "new-base"
			boardResult := resultMsg{generation: m.fetchGeneration, result: board.Result{Index: 0, PRs: []github.PR{pr}}}
			var rerun tea.Cmd
			if boardFirst {
				next, _ := m.Update(boardResult)
				m = next.(Model)
				m, rerun = m.applyBadge(oldRun().(badgeDoneMsg))
			} else {
				var premature tea.Cmd
				m, premature = m.applyBadge(oldRun().(badgeDoneMsg))
				if premature != nil {
					t.Fatal("reran a badge before the board refetch supplied current PR data")
				}
				next, cmd := m.Update(boardResult)
				m, rerun = next.(Model), cmd
			}
			e, _ := m.badges.cache.Get(key)
			if !e.Running || e.Text != "cached" || !e.Fetched.Equal(before.Fetched) {
				t.Fatalf("old run replaced the cached answer or lost its refresh: %+v", e)
			}
			if rerun == nil {
				t.Fatal("no replacement run")
			}
			msg, ok := rerun().(badgeDoneMsg)
			if !ok || msg.result.Err != nil || msg.result.Text != "new-base" {
				t.Fatalf("replacement did not use current PR data: %+v", msg)
			}
			m, _ = m.applyBadge(msg)
			e, _ = m.badges.cache.Get(key)
			if e.Running || e.Text != "new-base" || e.Due.Before(time.Now().Add(59*time.Minute)) {
				t.Fatalf("replacement did not settle for its TTL: %+v", e)
			}
		})
	}
}

func TestBadgeDetailsWrapTextAndErrorsBeforePagination(t *testing.T) {
	for _, failed := range []bool{false, true} {
		m := badgeBoard(t, 40, false, config.Badge{Name: "b", Run: "true", MaxWidth: 4})
		defer m.Close()
		pr := badgePRs[0]
		text := strings.Repeat("界x ", 80) + "END"
		if failed {
			answer(m, "b", pr, "", errors.New(text))
		} else {
			answer(m, "b", pr, text, nil)
		}
		for i, s := range m.slots() {
			if s.isRow() && s.row.PR.Key() == pr.Key() {
				m.cursor = i
			}
		}
		lines := m.badgeLines(pr)
		if len(lines) <= 1 {
			t.Fatalf("badge details were not split into physical lines (failed=%v)", failed)
		}
		for _, line := range lines {
			if lipgloss.Width(line) > m.width {
				t.Fatalf("detail line exceeds %d cells: %q", m.width, line)
			}
		}
		if !strings.Contains(ansi.Strip(strings.Join(lines, "")), text) {
			t.Fatal("wrapping lost badge content")
		}
		m.height = 8
		body := m.overlayBody(pr)
		m.detailScroll = len(body)
		view := ansi.Strip(m.detailOverlay())
		if len(strings.Split(view, "\n")) > m.height || !strings.Contains(view, "END") || !strings.Contains(view, "esc q d close") || m.detailTop(len(body)) != len(body)-m.detailRows(len(body)) {
			t.Fatalf("last detail page does not show the complete answer and hint:\n%s", view)
		}
	}
}
