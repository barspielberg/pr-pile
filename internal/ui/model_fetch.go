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
// Nothing is cancelled. Responses carry their PR number and file themselves
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
	if _, done := m.detail[pr.Number]; done {
		return nil
	}
	if _, pending := m.inflight[pr.Number]; pending {
		return nil
	}

	repo, number, head, generation := m.cfg.Repo, pr.Number, pr.HeadRefName, m.fetchGeneration
	m.inflight[number] = detailRequest{generation: generation, head: head}
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		d, err := client.Detail(ctx, repo, number, head)
		d.Number = number // so a failed response is still attributable
		return detailMsg{generation: generation, head: head, detail: d, err: err}
	}
}

func (m *Model) reconcileDetailIdentity() {
	for number, request := range m.inflight {
		head, ok := m.currentHead(number)
		if request.generation != m.fetchGeneration || !ok || head != request.head {
			delete(m.inflight, number)
		}
	}
	for number, identity := range m.detailIdentity {
		head, ok := m.currentHead(number)
		if identity.generation != m.fetchGeneration || !ok || head != identity.head {
			delete(m.detail, number)
			delete(m.detailIdentity, number)
		}
	}
}

func (m Model) currentHead(number int) (string, bool) {
	for _, section := range m.board.Sections() {
		for _, row := range section.Rows {
			if row.PR.Number == number {
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
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}
