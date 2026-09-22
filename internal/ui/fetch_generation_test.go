package ui

import (
	"testing"

	"github.com/barspielberg/pr-pile/internal/board"
	"github.com/barspielberg/pr-pile/internal/github"
)

func TestStaleBoardResultFromPreviousRefreshIsIgnored(t *testing.T) {
	m := New(testCfg(), nil)
	old := resultMsg{generation: m.fetchGeneration, result: board.Result{
		Index: 0, PRs: []github.PR{{Number: 1, Title: "old"}},
	}}

	next, _ := m.refresh()
	m = next.(Model)
	next, _ = m.Update(old)
	m = next.(Model)

	if got := m.board.Frontier(); got != 0 {
		t.Fatalf("stale result advanced refresh frontier to %d", got)
	}
	if rows := m.board.Sections()[0].Rows; len(rows) != 0 {
		t.Fatalf("stale result replaced current rows: %+v", rows)
	}

	current := resultMsg{generation: m.fetchGeneration, result: board.Result{
		Index: 0, PRs: []github.PR{{Number: 2, Title: "current"}},
	}}
	next, _ = m.Update(current)
	m = next.(Model)
	if got := m.board.Frontier(); got != 1 {
		t.Fatalf("current result left frontier at %d", got)
	}
}

func TestStaleRefreshTimerIsIgnored(t *testing.T) {
	m := New(testCfg(), nil)
	old := tickMsg{seq: m.refreshSeq}
	next, _ := m.refresh()
	m = next.(Model)
	generation := m.fetchGeneration

	next, _ = m.Update(old)
	m = next.(Model)
	if m.fetchGeneration != generation {
		t.Fatalf("stale timer started another refresh: generation %d -> %d", generation, m.fetchGeneration)
	}
}

func TestCurrentRefreshTimerStartsOneRefresh(t *testing.T) {
	m := New(testCfg(), nil)
	generation := m.fetchGeneration
	next, _ := m.Update(tickMsg{seq: m.refreshSeq})
	m = next.(Model)
	if m.fetchGeneration != generation+1 {
		t.Fatalf("current timer moved generation %d -> %d", generation, m.fetchGeneration)
	}
}
