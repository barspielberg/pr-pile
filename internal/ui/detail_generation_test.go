package ui

import (
	"testing"

	"github.com/barspielberg/pr-pile/internal/board"
	"github.com/barspielberg/pr-pile/internal/github"
)

func TestDetailResponseFromBeforeRefreshIsIgnored(t *testing.T) {
	m := New(testCfg(), nil)
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{Repo: testRepo, Number: 7, HeadRefName: "old"}}})
	m.board.Apply(board.Result{Index: 1})
	old := detailMsg{
		generation: m.fetchGeneration,
		head:       "old",
		detail:     github.Detail{Repo: testRepo, Number: 7, BehindBy: 99},
	}

	next, _ := m.refresh()
	m = next.(Model)
	if _, ok := m.inflight[prKey(7)]; ok {
		t.Fatal("refresh retained the old detail request")
	}
	markDetailInflight(&m, 7)
	next, _ = m.Update(old)
	m = next.(Model)

	if _, ok := m.detail[prKey(7)]; ok {
		t.Fatal("detail response from before refresh was cached")
	}
	if _, ok := m.inflight[prKey(7)]; !ok {
		t.Fatal("stale response cleared the current detail request")
	}
}

func TestDetailResponseForOldHeadIsIgnored(t *testing.T) {
	m := New(testCfg(), nil)
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{Repo: testRepo, Number: 7, HeadRefName: "old"}}})
	m.board.Apply(board.Result{Index: 1})
	old := detailMsg{
		generation: m.fetchGeneration,
		head:       "old",
		detail:     github.Detail{Repo: testRepo, Number: 7, BehindBy: 99},
	}

	m.board.Refetch()
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{Repo: testRepo, Number: 7, HeadRefName: "new"}}})
	m.board.Apply(board.Result{Index: 1})
	m.inflight[prKey(7)] = detailRequest{generation: m.fetchGeneration, head: "new"}
	next, _ := m.Update(old)
	m = next.(Model)

	if _, ok := m.detail[prKey(7)]; ok {
		t.Fatal("detail response for old head was cached")
	}
	if _, ok := m.inflight[prKey(7)]; !ok {
		t.Fatal("old-head response cleared the current request")
	}
}

func TestCurrentDetailResponseIsAccepted(t *testing.T) {
	m := New(testCfg(), nil)
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{Repo: testRepo, Number: 7, HeadRefName: "head"}}})
	m.board.Apply(board.Result{Index: 1})
	markDetailInflight(&m, 7)

	next, _ := m.Update(detailMsg{
		generation: m.fetchGeneration,
		head:       "head",
		detail:     github.Detail{Repo: testRepo, Number: 7, BehindBy: 3},
	})
	m = next.(Model)

	if got := m.detail[prKey(7)].BehindBy; got != 3 {
		t.Fatalf("behind = %d, want 3", got)
	}
	if _, ok := m.inflight[prKey(7)]; ok {
		t.Fatal("accepted response left request in flight")
	}
}

func TestBoardResultInvalidatesDetailRequestForStaleRow(t *testing.T) {
	m := staleDetailRefreshModel(t)
	m.inflight[prKey(7)] = detailRequest{generation: m.fetchGeneration, head: "old"}

	next, _ := m.Update(resultMsg{generation: m.fetchGeneration, result: board.Result{
		Index: 0, PRs: []github.PR{{Repo: testRepo, Number: 7, HeadRefName: "new"}},
	}})
	m = next.(Model)

	if _, ok := m.inflight[prKey(7)]; ok {
		t.Fatal("new board head left the stale-head detail request in flight")
	}
}

func TestBoardResultInvalidatesDetailCachedFromStaleRow(t *testing.T) {
	m := staleDetailRefreshModel(t)
	m.inflight[prKey(7)] = detailRequest{generation: m.fetchGeneration, head: "old"}
	next, _ := m.Update(detailMsg{
		generation: m.fetchGeneration,
		head:       "old",
		detail:     github.Detail{Repo: testRepo, Number: 7, BehindBy: 99},
	})
	m = next.(Model)
	if _, ok := m.detail[prKey(7)]; !ok {
		t.Fatal("precondition: stale-row detail response was not cached")
	}

	next, _ = m.Update(resultMsg{generation: m.fetchGeneration, result: board.Result{
		Index: 0, PRs: []github.PR{{Repo: testRepo, Number: 7, HeadRefName: "new"}},
	}})
	m = next.(Model)

	if _, ok := m.detail[prKey(7)]; ok {
		t.Fatal("new board head retained detail cached from stale row")
	}
}

func staleDetailRefreshModel(t *testing.T) Model {
	t.Helper()
	m := New(testCfg(), nil)
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{Repo: testRepo, Number: 7, HeadRefName: "old"}}})
	m.board.Apply(board.Result{Index: 1})
	next, _ := m.refresh()
	return next.(Model)
}
