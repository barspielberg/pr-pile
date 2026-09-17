package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/barspielberg/prs-mng/internal/board"
	"github.com/barspielberg/prs-mng/internal/github"
	tea "github.com/charmbracelet/bubbletea"
)

// detailModel is a board with one PR selected, which is what every test here
// starts from: the overlay only ever renders the selected row.
func detailModel(t *testing.T, pr github.PR, h int) Model {
	t.Helper()
	m := New(testCfg(), nil)
	m.width, m.height = 120, h
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{pr}})
	m.board.Apply(board.Result{Index: 1})
	return m
}

// The state block says what is true about the PR rather than about its CI:
// the conflict and its size, the conversations still open, who it is from and
// how big. This is the case docs/pr-detail.md §8.2 is written against -- CI is
// green and the PR is still completely stuck.
func TestDetailOverlayRendersTheStateBlock(t *testing.T) {
	pr := github.PR{
		Number: 3186, Title: "free unit numbers when a plan is abandoned",
		CIState: "SUCCESS", PassedCount: 18, Mergeable: "CONFLICTING",
		Author: "cdiaz88", AuthorName: "Carol Diaz",
		Additions: 596, Deletions: 45, ChangedFiles: 7,
		HeadRefName: "PROJ-1951-unit-number-integrity", BaseRefName: "master",
		CreatedAt: time.Now().Add(-6 * 24 * time.Hour),
		UpdatedAt: time.Now().Add(-25 * time.Hour),
	}
	m := detailModel(t, pr, 24)
	m.detail[3186] = github.Detail{
		Number: 3186, BehindBy: 26, Unresolved: 9, DefaultBranch: "master",
	}

	out := stripANSI(m.detailOverlay())
	for _, want := range []string{
		"✓ all 18 checks passing",
		"! conflicted · 26 commits behind master",
		"● 9 unresolved comments",
		"author    Carol Diaz (cdiaz88)",
		"size      +596 −45 · 7 files",
		"branch    PROJ-1951-unit-number-integrity",
		"opened    6d ago",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("overlay missing %q:\n%s", want, out)
		}
	}
	// The base is the repo default, so naming it would be a constant.
	if strings.Contains(out, "base ") {
		t.Errorf("base line drawn for a PR targeting the default branch:\n%s", out)
	}
}

// A line with nothing to say is not drawn. An all-green PR opened this morning
// with no conversations earns almost nothing, which is the point: a green PR
// does not get a full page just because a full page exists. §8.3.
func TestDetailOverlayDrawsNothingItCannotSay(t *testing.T) {
	pr := github.PR{
		Number: 3253, Title: "enable ops reports templates", CIState: "SUCCESS",
		PassedCount: 8, SkippedCount: 13, Mergeable: "MERGEABLE",
		Author: "erin-w-74", AuthorName: "Erin Walsh",
		Additions: 3, Deletions: 3, ChangedFiles: 2,
		HeadRefName: "PROJ-2089", BaseRefName: "master",
		CreatedAt: time.Now().Add(-9 * time.Minute),
		UpdatedAt: time.Now().Add(-9 * time.Minute),
	}
	m := detailModel(t, pr, 24)
	m.detail[3253] = github.Detail{Number: 3253, DefaultBranch: "master"}

	out := stripANSI(m.detailOverlay())
	for _, unwanted := range []string{"conflicted", "unresolved", "opened", "behind", "base "} {
		if strings.Contains(out, unwanted) {
			t.Errorf("overlay drew %q with nothing to say:\n%s", unwanted, out)
		}
	}
	if !strings.Contains(out, "author    Erin Walsh (erin-w-74)") {
		t.Errorf("overlay missing the author:\n%s", out)
	}
}

// The base branch is the only place the tool says what a stacked PR is built
// on, so it is drawn whenever it is not the repo default -- and the default
// comes from the repo, not from assuming "master". §5.6.
func TestDetailOverlayNamesANonDefaultBase(t *testing.T) {
	pr := github.PR{
		Number: 3109, Title: "forbid only the move that re-opens a cycle",
		CIState: "SUCCESS", PassedCount: 16, Author: "barspielberg",
		HeadRefName: "fix-PROJ-1850-seat-mapping-error",
		BaseRefName: "fix-PROJ-1839-reject-legacy-option",
		UpdatedAt:   time.Now(),
	}
	m := detailModel(t, pr, 24)
	m.detail[3109] = github.Detail{Number: 3109, DefaultBranch: "master"}

	if out := stripANSI(m.detailOverlay()); !strings.Contains(out,
		"base      fix-PROJ-1839-reject-legacy-option") {
		t.Errorf("overlay missing the stacked base:\n%s", out)
	}

	// A repo whose default is `main` must not have `master` hardcoded against
	// it: the same base line would then be drawn for every PR on the board.
	pr.BaseRefName = "main"
	m2 := detailModel(t, pr, 24)
	m2.detail[3109] = github.Detail{Number: 3109, DefaultBranch: "main"}
	if out := stripANSI(m2.detailOverlay()); strings.Contains(out, "base ") {
		t.Errorf("base drawn for a repo whose default is main:\n%s", out)
	}
}

// The state block is clipped before the checks block, entirely, and never
// interleaved: a PR with 6 failing checks must not drop a failure to make room
// for its branch name. §6.2, at the 14 rows the spec sizes the drop order for.
func TestDetailOverlayClipsTheStateBlockBeforeTheChecks(t *testing.T) {
	pr := github.PR{
		Number: 3230, Title: "bump @types/send from 0.17.4 to 1.2.1",
		CIState: "FAILURE", Author: "dependabot",
		FailedGates: []string{
			"build-push-image customer-portal",
			"build-push-image billing-service",
			"build-push-image web-client",
			"build-push-image webapp",
		},
		PendingGates: []string{"webapp_e2e", "web_client_e2e"},
		PassedCount:  9, SkippedCount: 12,
		Additions: 194, Deletions: 143, ChangedFiles: 1,
		HeadRefName: "dependabot/npm_and_yarn/types/send-1.2.1",
		BaseRefName: "master",
		CreatedAt:   time.Now().Add(-30 * time.Hour),
		UpdatedAt:   time.Now().Add(-30 * time.Hour),
	}
	m := detailModel(t, pr, 14)
	m.detail[3230] = github.Detail{Number: 3230, DefaultBranch: "master"}

	out := stripANSI(m.detailOverlay())

	// Every failing and pending gate survives, and so does the tally that makes
	// the numbers reconcile.
	for _, g := range append(append([]string{}, pr.FailedGates...), pr.PendingGates...) {
		if !strings.Contains(out, g) {
			t.Errorf("a check line was dropped to fit a state line: %q\n%s", g, out)
		}
	}
	if !strings.Contains(out, "9 passing, 12 skipped") {
		t.Errorf("the tally did not survive:\n%s", out)
	}
	// The state block gave up its lowest-priority lines, in order, and said so.
	if !strings.Contains(out, "author") {
		t.Errorf("the highest-priority state line was dropped:\n%s", out)
	}
	for _, dropped := range []string{"branch", "opened", "size"} {
		if strings.Contains(out, dropped) {
			t.Errorf("%q survived a clip that should have dropped it:\n%s", dropped, out)
		}
	}
	if !strings.Contains(out, "not shown") {
		t.Errorf("the clip was silent about what it hid:\n%s", out)
	}
	// Counted as emitted, not trimmed: a trailing newline is a 15th line to the
	// terminal and scrolls the header off the top, which is how this was found.
	if lines := len(strings.Split(out, "\n")); lines > 14 {
		t.Errorf("overlay emits %d lines into a 14-row pane:\n%s", lines, out)
	}
}

// The overlay is usable before the on-demand request lands: `d` draws it from
// data already held, and the two late lines are simply absent. §7.
func TestDetailOverlayIsUsableBeforeTheRequestLands(t *testing.T) {
	pr := github.PR{
		Number: 3186, Title: "free unit numbers", CIState: "SUCCESS",
		PassedCount: 18, Mergeable: "CONFLICTING", Review: "REVIEW_REQUIRED",
		Author: "cdiaz88", AuthorName: "Carol Diaz",
		Additions: 596, Deletions: 45, ChangedFiles: 7,
		HeadRefName: "PROJ-1951-unit-number-integrity", BaseRefName: "master",
		UpdatedAt: time.Now(),
	}
	m := detailModel(t, pr, 24)

	// Nothing has arrived: the page still answers what it already knows.
	out := stripANSI(m.detailOverlay())
	for _, want := range []string{
		"✓ all 18 checks passing",
		"! conflicted",
		"author    Carol Diaz (cdiaz88)",
		"size      +596 −45 · 7 files",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("overlay unusable before the request landed, missing %q:\n%s", want, out)
		}
	}
	// The lines that need the request are absent rather than wrong: claiming
	// "0 commits behind" or "no unresolved comments" would be a lie, not a gap.
	for _, unwanted := range []string{"behind", "unresolved"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("overlay drew %q before the data existed:\n%s", unwanted, out)
		}
	}
	// The base is not claimed either, since the default branch is not yet known.
	if strings.Contains(out, "base ") {
		t.Errorf("base drawn before the default branch was known:\n%s", out)
	}

	// The response lands and the lines appear.
	next, _ := m.Update(detailMsg{detail: github.Detail{
		Number: 3186, BehindBy: 26, Unresolved: 9, DefaultBranch: "master",
	}})
	out = stripANSI(next.(Model).detailOverlay())
	if !strings.Contains(out, "26 commits behind master") ||
		!strings.Contains(out, "● 9 unresolved comments") {
		t.Errorf("the late lines did not arrive:\n%s", out)
	}
}

// The reviewer line holds its place with a placeholder rather than appearing
// after the fact: it was asked for specifically, and a name that materialises a
// second later reads as the page having been wrong rather than incomplete.
func TestReviewerLineShowsALoaderUntilItArrives(t *testing.T) {
	pr := github.PR{
		Number: 3246, Title: "add context tree", CIState: "SUCCESS",
		PassedCount: 4, Review: "CHANGES_REQUESTED", Author: "someone",
		HeadRefName: "PROJ-1827", UpdatedAt: time.Now(),
	}
	m := detailModel(t, pr, 24)

	out := stripANSI(m.detailOverlay())
	if !strings.Contains(out, "review    …") {
		t.Errorf("no loader held the reviewer line:\n%s", out)
	}

	next, _ := m.Update(detailMsg{detail: github.Detail{
		Number: 3246, DefaultBranch: "master",
		Reviewers: []github.Reviewer{
			{Login: "alicechen", Name: "Alice Chen", State: "CHANGES_REQUESTED"},
		},
	}})
	out = stripANSI(next.(Model).detailOverlay())
	if !strings.Contains(out, "review    ✗ Alice Chen (alicechen)") {
		t.Errorf("the reviewer did not replace the loader:\n%s", out)
	}
	if strings.Contains(out, "review    …") {
		t.Errorf("the loader outlived the response:\n%s", out)
	}
}

// A PR nobody has reviewed has no name coming, so promising one with a loader
// would be a placeholder for nothing.
func TestReviewerLineIsSilentWhenNoOneHasReviewed(t *testing.T) {
	pr := github.PR{
		Number: 3230, Title: "bump", CIState: "SUCCESS", PassedCount: 2,
		Review: "REVIEW_REQUIRED", Author: "dependabot", UpdatedAt: time.Now(),
	}
	m := detailModel(t, pr, 24)
	if out := stripANSI(m.detailOverlay()); strings.Contains(out, "review") {
		t.Errorf("a loader promised a reviewer that is not coming:\n%s", out)
	}
}

// A display name is null for 36% of this board's authors, so the login is
// always carried and stands alone when there is no name. §5.2.
func TestPersonFallsBackToTheLogin(t *testing.T) {
	for _, tc := range []struct{ name, login, want string }{
		{"Carol Diaz", "cdiaz88", "Carol Diaz (cdiaz88)"},
		{"", "dependabot", "dependabot"},
	} {
		if got := person(tc.name, tc.login); got != tc.want {
			t.Errorf("person(%q, %q) = %q, want %q", tc.name, tc.login, got, tc.want)
		}
	}
}

// A response that lands after the cursor has moved on files itself under the PR
// it describes, so it can never overwrite the overlay you are looking at. This
// is what makes holding `j` safe without cancelling anything.
func TestALateResponseIsFiledUnderItsOwnPR(t *testing.T) {
	m := New(testCfg(), nil)
	m.width, m.height = 120, 24
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{
		{Number: 1, Title: "a", CIState: "SUCCESS", PassedCount: 1, UpdatedAt: time.Unix(9000, 0)},
		{Number: 2, Title: "b", CIState: "SUCCESS", PassedCount: 1, UpdatedAt: time.Unix(8000, 0)},
	}})
	m.board.Apply(board.Result{Index: 1})

	// The cursor is on #2 when #1's request finally answers.
	m.cursor = 1
	next, _ := m.Update(detailMsg{detail: github.Detail{
		Number: 1, Unresolved: 4, DefaultBranch: "master",
	}})
	m = next.(Model)

	if out := stripANSI(m.detailOverlay()); strings.Contains(out, "unresolved") {
		t.Errorf("#1's response leaked onto #2's overlay:\n%s", out)
	}
	m.cursor = 0
	if out := stripANSI(m.detailOverlay()); !strings.Contains(out, "● 4 unresolved comments") {
		t.Errorf("#1's response was not kept for #1:\n%s", out)
	}
}

// Pressing `d` must not fire a second request for a PR already answered or
// already asked about: holding the key down would otherwise queue one per
// repeat.
func TestRepeatedPressesDoNotRefetch(t *testing.T) {
	m := detailModel(t, github.PR{
		Number: 7, Title: "t", CIState: "SUCCESS", PassedCount: 1,
		HeadRefName: "b", UpdatedAt: time.Now(),
	}, 24)
	pr, _ := m.selected()

	// No client, so the command is nil either way; what is asserted is the
	// bookkeeping that decides whether one would have been issued.
	m.inflight[pr.Number] = true
	if cmd := m.fetchDetail(pr); cmd != nil {
		t.Error("a second request was issued while the first was in flight")
	}
	delete(m.inflight, pr.Number)
	m.detail[pr.Number] = github.Detail{Number: pr.Number}
	if cmd := m.fetchDetail(pr); cmd != nil {
		t.Error("a request was issued for a PR already answered")
	}
}

// `d` still opens instantly and still closes on the next movement, with the
// state block present. The overlay grew a block; it did not grow a mode.
func TestDetailOverlayStaysAGlance(t *testing.T) {
	m := detailModel(t, github.PR{
		Number: 1, Title: "a", CIState: "FAILURE", FailedGates: []string{"gate-one"},
		Author: "someone", HeadRefName: "b", UpdatedAt: time.Unix(9000, 0),
	}, 20)
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{
		{Number: 1, Title: "a", CIState: "FAILURE", FailedGates: []string{"gate-one"},
			Author: "someone", HeadRefName: "b", UpdatedAt: time.Unix(9000, 0)},
		{Number: 2, Title: "b", CIState: "SUCCESS", UpdatedAt: time.Unix(8000, 0)},
	}})

	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if !m.showChecks {
		t.Fatal("d should open the detail overlay")
	}
	out := stripANSI(m.View())
	if !strings.Contains(out, "gate-one") || !strings.Contains(out, "author") {
		t.Errorf("overlay missing a block:\n%s", out)
	}
	if !strings.Contains(out, "any key closes") {
		t.Errorf("overlay lost its footer:\n%s", out)
	}

	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if m.showChecks || m.cursor != 1 {
		t.Errorf("one keypress should close and move: showChecks=%v cursor=%d", m.showChecks, m.cursor)
	}
}

// The overlay fits whatever pane it is given, at every height the board is
// usable at. A page that overruns would push its own footer off the screen.
func TestDetailOverlayFitsEveryPane(t *testing.T) {
	pr := github.PR{
		Number: 3186, Title: strings.Repeat("a long title ", 12),
		CIState: "FAILURE", Mergeable: "CONFLICTING",
		FailedGates:  []string{"g1", "g2", "g3", "g4", "g5", "g6", "g7", "g8"},
		PendingGates: []string{"p1", "p2", "p3"}, PassedCount: 9, SkippedCount: 12,
		Author: "cdiaz88", AuthorName: "Carol Diaz",
		Additions: 596, Deletions: 45, ChangedFiles: 7,
		HeadRefName: "PROJ-1951-unit-number-integrity", BaseRefName: "other-branch",
		CreatedAt: time.Now().Add(-6 * 24 * time.Hour),
		UpdatedAt: time.Now().Add(-72 * time.Hour),
	}
	for _, h := range []int{10, 12, 14, 20, 24, 40} {
		for _, w := range []int{80, 120, 200} {
			m := detailModel(t, pr, h)
			m.width = w
			m.detail[3186] = github.Detail{
				Number: 3186, BehindBy: 26, Unresolved: 9, DefaultBranch: "master",
				Reviewers: []github.Reviewer{{Login: "alicechen", Name: "Alice Chen", State: "APPROVED"}},
			}
			out := m.detailOverlay()
			if got := len(strings.Split(out, "\n")); got > h {
				t.Errorf("%dx%d: overlay emits %d lines:\n%s", w, h, got, stripANSI(out))
			}
			for _, line := range strings.Split(stripANSI(out), "\n") {
				if len([]rune(line)) > w {
					t.Errorf("%dx%d: line overruns the width: %q", w, h, line)
				}
			}
		}
	}
}

// The state block never appears above or inside the check list: the reader came
// for the checks and they stay at the top of the page.
func TestStateBlockStaysBelowTheChecks(t *testing.T) {
	pr := github.PR{
		Number: 9, Title: "t", CIState: "FAILURE",
		FailedGates: []string{"gate-one", "gate-two"}, PassedCount: 3,
		Author: "someone", HeadRefName: "b", UpdatedAt: time.Now(),
	}
	m := detailModel(t, pr, 24)
	m.detail[9] = github.Detail{Number: 9, Unresolved: 2, DefaultBranch: "master"}

	out := stripANSI(m.detailOverlay())
	lastCheck := strings.Index(out, "3 passing")
	firstState := strings.Index(out, "● 2 unresolved")
	if lastCheck < 0 || firstState < 0 {
		t.Fatalf("overlay missing a block:\n%s", out)
	}
	if firstState < lastCheck {
		t.Errorf("the state block rendered above the checks:\n%s", out)
	}
	if got := strings.Index(out, "gate-one"); got > lastCheck {
		t.Errorf("the blocks interleaved:\n%s", out)
	}
}

// plural carries the count, so a single unresolved comment does not read as
// "1 unresolved comments".
func TestUnresolvedLineAgreesWithItsCount(t *testing.T) {
	for n, want := range map[int]string{1: "● 1 unresolved comment", 9: "● 9 unresolved comments"} {
		m := detailModel(t, github.PR{
			Number: 5, Title: "t", CIState: "SUCCESS", PassedCount: 1,
			Author: "a", UpdatedAt: time.Now(),
		}, 24)
		m.detail[5] = github.Detail{Number: 5, Unresolved: n, DefaultBranch: "master"}
		if out := stripANSI(m.detailOverlay()); !strings.Contains(out, want) {
			t.Errorf("n=%d: missing %q:\n%s", n, want, fmt.Sprint(out))
		}
	}
}

// The display name is searchable, not just the login: the row shows three
// initials of a login like `cdiaz88`, so "Carol" is what a person would
// actually type to find it.
func TestFilterMatchesTheAuthorsDisplayName(t *testing.T) {
	m := New(testCfg(), nil)
	m.width, m.height = 120, 24
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{
		{Number: 1, Title: "unit numbers", Author: "cdiaz88",
			AuthorName: "Carol Diaz", UpdatedAt: time.Unix(9000, 0)},
		{Number: 2, Title: "something else", Author: "dependabot",
			UpdatedAt: time.Unix(8000, 0)},
	}})
	m.board.Apply(board.Result{Index: 1})

	m = typeQuery(m, "Carol")
	rows := m.visibleRows()
	if len(rows) != 1 || rows[0].PR.Number != 1 {
		t.Fatalf("display name did not match: got %d rows", len(rows))
	}

	// The login still matches, and so does an author with no display name.
	m = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	m = typeQuery(m, "dependabot")
	if rows := m.visibleRows(); len(rows) != 1 || rows[0].PR.Number != 2 {
		t.Fatalf("login-only author stopped matching: got %d rows", len(rows))
	}
}

// The page says it is still loading, and it says so where the answer will
// land: the skeleton line stands in the on-demand lines' own position, so the
// common case -- one of the two lines arriving -- resolves in place and nothing
// below it moves. docs/pr-detail.md §7 chose to draw nothing and let the lines
// appear; that reflows the whole block and shows no loading state at all.
func TestDetailOverlayShowsItIsStillLoading(t *testing.T) {
	pr := github.PR{
		Number: 3186, Title: "free unit numbers", CIState: "SUCCESS",
		PassedCount: 18, Author: "cdiaz88", AuthorName: "Carol Diaz",
		Additions: 596, Deletions: 45, ChangedFiles: 7,
		HeadRefName: "PROJ-1951", BaseRefName: "master", UpdatedAt: time.Now(),
	}
	m := detailModel(t, pr, 24)
	// The loader only promises what something is actually fetching, so it needs
	// a client to have asked.
	m.client = &github.Client{}

	before := strings.Split(stripANSI(m.detailOverlay()), "\n")
	if !strings.Contains(strings.Join(before, "\n"), "checking for conflicts and open conversations") {
		t.Fatalf("no loading state on an unresolved detail page:\n%s", strings.Join(before, "\n"))
	}
	loaderAt := -1
	for i, l := range before {
		if strings.Contains(l, "checking for conflicts") {
			loaderAt = i
		}
	}

	// One on-demand line arrives, which is the common shape, and it lands on
	// the loader's own row.
	next, _ := m.Update(detailMsg{detail: github.Detail{
		Number: 3186, BehindBy: 26, DefaultBranch: "master",
	}})
	after := strings.Split(stripANSI(next.(Model).detailOverlay()), "\n")

	if strings.Contains(strings.Join(after, "\n"), "checking for conflicts") {
		t.Errorf("the loader outlived the response:\n%s", strings.Join(after, "\n"))
	}
	if len(after) != len(before) {
		t.Errorf("the page changed height on resolution: %d lines, was %d", len(after), len(before))
	}
	if !strings.Contains(after[loaderAt], "26 commits behind master") {
		t.Errorf("the answer did not land on the loader's row %d: %q", loaderAt, after[loaderAt])
	}
	for i := range before {
		if i == loaderAt {
			continue
		}
		if i < len(after) && after[i] != before[i] {
			t.Errorf("line %d reflowed when the response landed:\n before %q\n  after %q",
				i, before[i], after[i])
		}
	}
}

// A board with no client is not waiting on anything, so it must not sit on a
// loader forever.
func TestDetailOverlayHasNoLoaderWithNothingInFlight(t *testing.T) {
	pr := github.PR{Number: 1, Title: "a", CIState: "SUCCESS", PassedCount: 1,
		Author: "someone", UpdatedAt: time.Now()}
	m := detailModel(t, pr, 24)
	if out := stripANSI(m.detailOverlay()); strings.Contains(out, "checking for") {
		t.Errorf("a loader with nothing to wait for:\n%s", out)
	}
}

// The spinner has to keep ticking while a detail request is out, or the loader
// is a frozen glyph -- which reads as stuck, not as working. The board may well
// have finished fetching by the time `d` is pressed.
func TestSpinnerKeepsTickingForTheDetailRequest(t *testing.T) {
	pr := github.PR{Number: 1, Title: "a", CIState: "SUCCESS", PassedCount: 1,
		Author: "someone", UpdatedAt: time.Now()}
	m := detailModel(t, pr, 24)
	m.fetching = false
	m.inflight[1] = true

	next, cmd := m.Update(spinMsg(time.Now()))
	if next.(Model).spinner == m.spinner {
		t.Error("the spinner did not advance while a detail request was in flight")
	}
	if cmd == nil {
		t.Error("the tick stopped while a detail request was in flight")
	}

	// And it stops once nothing is out, so an idle board is not spinning.
	m.inflight = map[int]bool{}
	if _, cmd := m.Update(spinMsg(time.Now())); cmd != nil {
		t.Error("the spinner kept ticking on an idle board")
	}
}
