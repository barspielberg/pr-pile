package ui

import (
	"github.com/barspielberg/pr-pile/internal/github"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case spinMsg:
		// The detail overlay's loader and a running action are the same
		// spinner, so the tick has to survive a board that has finished
		// fetching: otherwise the loader is a frozen glyph, which reads as
		// stuck rather than as working.
		if !m.fetching && len(m.inflight) == 0 && m.running == "" {
			return m, nil
		}
		m.spinner++
		return m, spinTick()

	case resultMsg:
		if msg.generation != m.fetchGeneration {
			return m, nil
		}
		m.board.Apply(msg.result)
		m.reconcileDetailIdentity()
		if !m.board.Loading() {
			m.fetching = false
			m.clampCursor()
			return m, m.refreshTick()
		}
		return m, nil

	case tickMsg:
		if msg.seq != m.refreshSeq {
			return m, nil
		}
		return m.refresh()

	case refreshMsg:
		return m.refresh()

	case detailMsg:
		if msg.generation != m.fetchGeneration {
			return m, nil
		}
		request, pending := m.inflight[msg.detail.Number]
		if !pending || request.generation != msg.generation || request.head != msg.head {
			return m, nil
		}
		head, ok := m.currentHead(msg.detail.Number)
		if !ok || head != msg.head {
			return m, nil
		}
		delete(m.inflight, msg.detail.Number)
		// A failed request costs the on-demand lines and nothing else: the
		// overlay is already on screen and already useful without them, and
		// saying so at the user would be noise about a page they are reading.
		if msg.err == nil && msg.detail.Number != 0 {
			m.detail[msg.detail.Number] = msg.detail
			m.detailIdentity[msg.detail.Number] = request
		}
		return m, nil

	case asyncStatusMsg:
		if msg.seq != m.statusSeq {
			return m, nil
		}
		m.status = msg.text
		m.clearSeq = 0
		return m, nil

	case actionDoneMsg:
		// A result from a run the user has already superseded says nothing
		// about what is on screen now.
		if msg.seq != m.runSeq {
			return m, nil
		}
		m.running = ""
		ownsStatus := msg.statusSeq == m.statusSeq || m.status == msg.name+" still running"
		if !ownsStatus {
			return m, nil
		}
		m.status = actionResult(msg)
		if msg.err == nil {
			// Success has said its piece; leaving it up would have the footer
			// claim an action is current long after it finished. Failure
			// persists, because it is the one the user has to act on.
			m.clearSeq = msg.seq
			return m, clearStatusIn(statusHold, msg.seq)
		}
		return m, nil

	case clearStatusMsg:
		// Only the run that set it may clear it: anything newer -- another
		// action, a refresh, a copy -- owns the line now.
		if int(msg) == m.clearSeq && m.running == "" {
			m.status = ""
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) setStatus(text string) Model {
	m.statusSeq++
	m.status = text
	m.clearSeq = 0
	return m
}

func (m Model) refresh() (tea.Model, tea.Cmd) {
	// Keep the current rows on screen while refetching, so the board does not
	// collapse and re-expand under the cursor.
	m.fetchGeneration++
	m.refreshSeq++
	m.statusSeq++
	m.board.Refetch()
	m.detail = make(map[int]github.Detail)
	m.inflight = make(map[int]detailRequest)
	m.detailIdentity = make(map[int]detailRequest)
	m.fetching = true
	m.status = ""
	m.clearSeq = 0
	// A refresh does not kill the process, but the board it was launched from
	// is gone; keeping its name on the footer would attribute the fetch
	// spinner to the action. Its result still lands, keyed by seq.
	if m.running != "" {
		m.runSeq++
		m.running = ""
	}
	return m, tea.Batch(append(m.fetchAll(), spinTick())...)
}
