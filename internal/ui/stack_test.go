package ui

import (
	"strings"
	"testing"

	"github.com/barspielberg/pr-pile/internal/board"
	"github.com/barspielberg/pr-pile/internal/github"
)

// stackBoard is #1 and #2 in stack 10, #3 in no stack, and #4, #5 in stacks
// 11 and 12, in that order on the board.
func stackBoard(t *testing.T) Model {
	t.Helper()
	m := New(testCfg(), nil)
	m.width, m.height = 120, 20
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{
		{Repo: testRepo, Number: 1, Title: "one", Stack: github.Stack{Number: 10, Size: 2, Position: 1}},
		{Repo: testRepo, Number: 2, Title: "two", Stack: github.Stack{Number: 10, Size: 2, Position: 2}},
		{Repo: testRepo, Number: 3, Title: "three"},
		{Repo: testRepo, Number: 4, Title: "four", Stack: github.Stack{Number: 11, Size: 2, Position: 1}},
		{Repo: testRepo, Number: 5, Title: "five", Stack: github.Stack{Number: 12, Size: 2, Position: 1}},
	}})
	m.board.Apply(board.Result{Index: 1})
	m.fetching = false
	m.cursor = m.firstRowSlot()
	return m
}

func TestStackKeyOpensTheStackPage(t *testing.T) {
	got := openCapture(t, nil)
	pressDrain(t, stackBoard(t), "S")
	if len(*got) != 1 || (*got)[0] != "https://github.com/o/r/stack/10" {
		t.Fatalf("opened %v", *got)
	}
}

func TestStackKeyOnAPROutsideAStackSaysSo(t *testing.T) {
	got := openCapture(t, nil)
	m := stackBoard(t)
	m.cursor = m.rowSlot(2)
	m = pressDrain(t, m, "S")
	if len(*got) != 0 || m.status != "not in a stack" {
		t.Fatalf("opened %v, status %q", *got, m.status)
	}
}

// Several PRs of one stack are one page, not a tab each.
func TestStackKeyOpensEachStackOnce(t *testing.T) {
	got := openCapture(t, nil)
	m := pressKey(pressKey(pressKey(stackBoard(t), "v"), "j"), "j") // #1, #2, #3
	m = pressDrain(t, m, "S")
	if len(*got) != 1 || len(m.confirmOpen) != 0 {
		t.Fatalf("opened %v, confirm %v", *got, m.confirmOpen)
	}
}

// Three stacks is three tabs, which gets the same question as three PRs.
func TestStackKeyAsksBeforeOpeningThreeStacks(t *testing.T) {
	got := openCapture(t, nil)
	m := stackBoard(t)
	for range 4 {
		m = pressKey(m, "j")
	}
	m = pressKey(m, "v")
	for range 4 {
		m = pressKey(m, "k")
	}
	m = pressDrain(t, m, "S")
	if len(*got) != 0 || m.confirmQuestion() != "open 3 stacks in the browser?" {
		t.Fatalf("opened %v, question %q", *got, m.confirmQuestion())
	}
	m = pressDrain(t, m, "y")
	want := []string{"https://github.com/o/r/stack/10", "https://github.com/o/r/stack/11", "https://github.com/o/r/stack/12"}
	if len(*got) != 3 || (*got)[0] != want[0] || (*got)[1] != want[1] || (*got)[2] != want[2] {
		t.Fatalf("opened %v, want %v", *got, want)
	}
	if m.status != "opened 3 stacks" {
		t.Errorf("status %q", m.status)
	}
}

func TestCopyStackLinkCopiesEachStackOnce(t *testing.T) {
	got := copyCapture(t, nil)
	m := pressKey(stackBoard(t), "v")
	for range 3 {
		m = pressKey(m, "j")
	}
	m = pressDrain(t, pressKey(m, "Y"), "s") // #1..#4: stacks 10, 10, none, 11
	if want := "https://github.com/o/r/stack/10\nhttps://github.com/o/r/stack/11"; *got != want {
		t.Fatalf("copied %q, want %q", *got, want)
	}
	if m.status != "copied 2 stack links" {
		t.Errorf("status %q", m.status)
	}
}

// A five-PR stack: three merged PRs under #2902 and #2906, with #2902 on the
// board and #2906 the one the page is about.
func stackDetailModel(t *testing.T) Model {
	t.Helper()
	pr := github.PR{Repo: testRepo, Number: 2906, Title: "rewrite the stub", CIState: "SUCCESS", PassedCount: 3,
		HeadRefName: "five",
		Stack:       github.Stack{Number: 2904, Size: 5, Base: "master", Position: 5}}
	m := detailModel(t, pr, 40)
	m.client = &github.Client{}
	return m
}

var stack2904 = []github.StackEntry{
	{Position: 1, Number: 2899, Title: "one", State: "MERGED"},
	{Position: 2, Number: 2900, Title: "two", State: "MERGED"},
	{Position: 3, Number: 2901, Title: "three", State: "CLOSED"},
	{Position: 4, Number: 2902, Title: "four", State: "OPEN", IsDraft: true},
	{Position: 5, Number: 2906, Title: "rewrite the stub", State: "OPEN"},
}

func TestDetailStackBlockLoadsUnderItsHeader(t *testing.T) {
	m := stackDetailModel(t)
	before := strings.Split(stripANSI(m.detailOverlay()), "\n")
	body := strings.Join(before, "\n")
	if !strings.Contains(body, "stack     #2904 · 5 PRs · on master") || !strings.Contains(body, "listing the stack") {
		t.Fatalf("want the header and a loader before the request lands:\n%s", body)
	}

	next, _ := m.Update(detailResponse(m, github.Detail{Repo: testRepo, Number: 2906, BehindBy: 3, DefaultBranch: "master", Stack: stack2904}))
	after := strings.Split(stripANSI(next.(Model).detailOverlay()), "\n")
	got := strings.Join(after, "\n")
	for _, want := range []string{
		"    1  #2899  merged  one\n",
		"    3  #2901  closed  three\n",
		"    4  #2902  draft   four  (not on board)\n",
		"  ▌ 5  #2906  open    rewrite the stub\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "listing the stack") {
		t.Error("the loader outlived the response")
	}
	// The entries only grow the bottom, so the header stays on its row.
	header := func(lines []string) int {
		for i, l := range lines {
			if strings.Contains(l, "#2904 · 5 PRs") {
				return i
			}
		}
		return -1
	}
	if b, a := header(before), header(after); b != a {
		t.Errorf("the stack header moved from line %d to %d", b, a)
	}
}

func TestDetailStackBlockKnowsWhatIsOnTheBoard(t *testing.T) {
	m := stackDetailModel(t)
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{
		m.board.Sections()[0].Rows[0].PR,
		{Repo: testRepo, Number: 2902, Title: "four"},
	}})
	m.detail[github.Key{Repo: testRepo, Number: 2906}] = github.Detail{Repo: testRepo, Number: 2906, Stack: stack2904}
	if got := stripANSI(m.detailOverlay()); strings.Contains(got, "not on board") {
		t.Errorf("#2902 is on the board:\n%s", got)
	}
}

// A failed request leaves the header, which came from the search, and no loader.
func TestDetailStackBlockWithoutItsEntries(t *testing.T) {
	m := stackDetailModel(t)
	m.detailFailed[github.Key{Repo: testRepo, Number: 2906}] = detailRequest{}
	got := stripANSI(m.detailOverlay())
	if !strings.Contains(got, "#2904 · 5 PRs") || strings.Contains(got, "listing the stack") {
		t.Errorf("got:\n%s", got)
	}
}

func TestDetailHasNoStackBlockOutsideAStack(t *testing.T) {
	m := detailModel(t, github.PR{Repo: testRepo, Number: 1, Title: "a"}, 40)
	if got := stripANSI(m.detailOverlay()); strings.Contains(got, "stack") {
		t.Errorf("got:\n%s", got)
	}
}
