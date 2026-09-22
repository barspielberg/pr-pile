package ui

import (
	"testing"

	"github.com/barspielberg/pr-pile/internal/board"
	"github.com/barspielberg/pr-pile/internal/github"
)

func TestDetailResponseFromBeforeRefreshIsIgnored(t *testing.T) {
	m := New(testCfg(), nil)
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{Number: 7, HeadRefName: "old"}}})
	m.board.Apply(board.Result{Index: 1})
	old := detailMsg{
		generation: m.fetchGeneration,
		head:       "old",
		detail:     github.Detail{Number: 7, BehindBy: 99},
	}

	next, _ := m.refresh()
	m = next.(Model)
	if m.inflight[7] {
		t.Fatal("refresh retained the old detail request")
	}
	m.inflight[7] = true
	next, _ = m.Update(old)
	m = next.(Model)

	if _, ok := m.detail[7]; ok {
		t.Fatal("detail response from before refresh was cached")
	}
	if !m.inflight[7] {
		t.Fatal("stale response cleared the current detail request")
	}
}

func TestDetailResponseForOldHeadIsIgnored(t *testing.T) {
	m := New(testCfg(), nil)
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{Number: 7, HeadRefName: "old"}}})
	m.board.Apply(board.Result{Index: 1})
	old := detailMsg{
		generation: m.fetchGeneration,
		head:       "old",
		detail:     github.Detail{Number: 7, BehindBy: 99},
	}

	m.board.Refetch()
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{Number: 7, HeadRefName: "new"}}})
	m.board.Apply(board.Result{Index: 1})
	m.inflight[7] = true
	next, _ := m.Update(old)
	m = next.(Model)

	if _, ok := m.detail[7]; ok {
		t.Fatal("detail response for old head was cached")
	}
	if !m.inflight[7] {
		t.Fatal("old-head response cleared the current request")
	}
}

func TestCurrentDetailResponseIsAccepted(t *testing.T) {
	m := New(testCfg(), nil)
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{Number: 7, HeadRefName: "head"}}})
	m.board.Apply(board.Result{Index: 1})
	m.inflight[7] = true

	next, _ := m.Update(detailMsg{
		generation: m.fetchGeneration,
		head:       "head",
		detail:     github.Detail{Number: 7, BehindBy: 3},
	})
	m = next.(Model)

	if got := m.detail[7].BehindBy; got != 3 {
		t.Fatalf("behind = %d, want 3", got)
	}
	if m.inflight[7] {
		t.Fatal("accepted response left request in flight")
	}
}
