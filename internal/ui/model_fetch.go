package ui

import (
	"context"
	"time"

	"github.com/barspielberg/pr-pile/internal/board"
	"github.com/barspielberg/pr-pile/internal/config"
	"github.com/barspielberg/pr-pile/internal/github"
	tea "github.com/charmbracelet/bubbletea"
)

// Every rule is fetched concurrently; only the reveal is ordered, which the
// board handles.
func (m Model) fetchAll() []tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(m.cfg.Rules))
	for i, r := range m.cfg.Rules {
		cmds = append(cmds, m.fetchRule(i, r))
	}
	return cmds
}

func (m Model) fetchRule(i int, r config.Rule) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		prs, err := m.client.Search(ctx, m.cfg.SearchQuery(r), r.PageSize())
		return resultMsg{generation: m.fetchGeneration, result: board.Result{Index: i, PRs: prs, Err: err}}
	}
}

// fetchDetail asks for the on-demand half of the overlay: how far behind its
// base the PR is, its unresolved conversations, who reviewed it, and what the
// repo's default branch is. Measured at 1.2-1.9s, so the overlay draws without
// it and these lines arrive late.
//
// Nothing is cancelled. Responses carry their repo and number and file themselves
// under it, so holding `j` with `d` at each row leaves requests that answer a
// question nobody is asking any more -- harmless, and cheaper than threading
// cancellation through Bubble Tea's command model. What is guarded is asking
// twice: a PR already answered or already out is not requested again.
func (m Model) fetchDetail(pr github.PR) tea.Cmd {
	if m.client == nil {
		return nil
	}
	// It does not go stale within a session: behindBy and unresolved threads
	// move on the scale of a working day, and `r` refetches the board anyway.
	if _, done := m.detail[pr.Key()]; done {
		return nil
	}
	if _, pending := m.inflight[pr.Key()]; pending {
		return nil
	}
	if _, failed := m.detailFailed[pr.Key()]; failed {
		return nil
	}

	key, head, generation := pr.Key(), pr.HeadRefName, m.fetchGeneration
	m.inflight[key] = detailRequest{generation: generation, head: head}
	repo, compare := pr.Repo, pr.CompareRef()
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		d, err := client.Detail(ctx, repo, key.Number, compare)
		d.Repo, d.Number = key.Repo, key.Number // so a failed response is still attributable
		return detailMsg{generation: generation, head: head, detail: d, err: err}
	}
}

func (m *Model) reconcileDetailIdentity() {
	for key, request := range m.inflight {
		head, ok := m.currentHead(key)
		if request.generation != m.fetchGeneration || !ok || head != request.head {
			delete(m.inflight, key)
		}
	}
	for key, identity := range m.detailIdentity {
		head, ok := m.currentHead(key)
		if identity.generation != m.fetchGeneration || !ok || head != identity.head {
			delete(m.detail, key)
			delete(m.detailIdentity, key)
		}
	}
	for key, request := range m.detailFailed {
		head, ok := m.currentHead(key)
		if request.generation != m.fetchGeneration || !ok || head != request.head {
			delete(m.detailFailed, key)
		}
	}
}

func (m Model) currentHead(key github.Key) (string, bool) {
	for _, section := range m.board.Sections() {
		for _, row := range section.Rows {
			if row.PR.Key() == key {
				return row.PR.HeadRefName, true
			}
		}
	}
	return "", false
}

func spinTick() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg { return spinMsg(t) })
}

func (m Model) refreshTick() tea.Cmd {
	d := m.cfg.Refresh
	if d <= 0 {
		return nil
	}
	seq := m.refreshSeq
	return tea.Tick(d, func(time.Time) tea.Msg { return tickMsg{seq: seq} })
}
