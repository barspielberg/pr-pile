package ui

import (
	"fmt"
	"github.com/barspielberg/pr-pile/internal/browser"
	"github.com/barspielberg/pr-pile/internal/github"
	tea "github.com/charmbracelet/bubbletea"
	"os/exec"
	"strings"
)

// copyToClipboard shells out to pbcopy rather than taking a clipboard
// dependency, which is the same trade browser.Open makes with `open`. DESIGN.md
// turned down bubbles/textinput partly because it drags in a clipboard
// shell-out for a binding we did not want -- the objection was to the
// dependency, not to the pipe, and this is the pipe on its own.
//
// It is a var so a test can watch what would be copied without writing to the
// developer's real clipboard.
var copyToClipboard = func(s string) error {
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(s)
	return cmd.Run()
}

// copySelected yanks the url of every selected PR, or of the row under the
// cursor when nothing is selected.
func (m Model) copySelected() (Model, tea.Cmd) {
	prs := m.actionPRs()
	if len(prs) == 0 {
		return m.setStatus("no PR selected"), nil
	}
	return m.copyField(prs, urlField)
}

// A var so a test can watch what would be opened.
var openURL = browser.Open

// Two tabs is a normal thing to want; three is where a mistyped key stops
// being recoverable, since nothing closes the tabs it would spray. See §4.5.
const confirmThreshold = 3

// openSelected opens every selected PR, or the row under the cursor when
// nothing is selected.
func (m Model) openSelected() (Model, tea.Cmd) {
	prs := m.actionPRs()
	if len(prs) == 0 {
		return m, nil
	}
	if len(prs) >= confirmThreshold {
		// Ask first, and open NOTHING until the answer comes back. The prompt
		// is the whole safeguard; opening optimistically behind it would make
		// it decoration.
		//
		// The set is captured here, not re-read when the answer comes: the
		// board refetches on a timer and a refresh landing mid-prompt clears
		// the selection underneath it.
		m.confirmOpen = prs
		return m, nil
	}
	return m.doOpen(prs)
}

// doOpen fires the opens and clears the selection. Split out so the confirm
// path and the straight-through path cannot drift.
func (m Model) doOpen(prs []github.PR) (Model, tea.Cmd) {
	var urls []string
	for _, pr := range prs {
		if pr.URL != "" {
			urls = append(urls, pr.URL)
		}
	}
	if len(urls) == 0 {
		return m.setStatus("no URL for this PR"), nil
	}
	m.statusSeq++
	seq := m.statusSeq
	text := ""
	if len(urls) > 1 {
		text = fmt.Sprintf("opened %d PRs", len(urls))
	}
	next := m
	next.clearSelection()
	next.clearConfirm()
	return next, func() tea.Msg {
		for _, url := range urls {
			if err := openURL(url); err != nil {
				return asyncStatusMsg{seq: seq, text: "open failed: " + err.Error()}
			}
		}
		return asyncStatusMsg{seq: seq, text: text}
	}
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.showHelp || m.showChecks {
		// Only ctrl+c quits from here: esc and q mean "back to the board", so
		// opening an overlay can never cost the user their session by reflex.
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		// A movement key closes the overlay AND moves, so checking a PR then
		// carrying on down the list is one keypress, not two.
		if m.showChecks {
			m.showChecks = false
			switch msg.String() {
			case "j", "down", "k", "up", "l", "right", "h", "left", "g", "home", "G", "end":
				return m.handleKey(msg)
			}
			return m, nil
		}
		return m.handleHelpKey(msg)
	}
	if m.searching {
		return m.handleSearchKey(msg)
	}
	if len(m.confirmOpen) > 0 {
		return m.handleConfirmKey(msg)
	}
	if len(m.copyMenu) > 0 {
		return m.handleCopyMenuKey(msg)
	}
	switch msg.String() {
	case "esc":
		// Three rungs, highest-priority first: selection, then search, then
		// quit. The selection goes first because it is the state that costs
		// most to be wrong about -- an action fires on it -- and because a
		// board with marks still on it is visibly holding something.
		//
		// A user who never selects anything still quits on the first press:
		// each rung only claims esc when it has something to clear.
		if len(m.selection) > 0 || m.ranging {
			m.clearSelection()
			return m, nil
		}
		// The :noh of this board. Strictly vim keeps the pattern for a later
		// n; here it goes entirely, because a board with no visible highlights
		// where n still jumps would be a mode with nothing on screen to say so.
		if m.query != "" {
			m.query = ""
			return m, nil
		}
		return m, tea.Quit
	case "q", "ctrl+c":
		return m, tea.Quit
	case "/":
		m.searching = true
		// The cursor stays where it is: a search does not narrow the board, so
		// there is nothing to jump to yet, and searchOrigin is what esc
		// restores after incsearch has walked it away.
		m.searchOrigin = m.cursor
		return m, nil
	case "?":
		m.showHelp = true
		m.helpScroll = 0
		// The page opens clean. A query left over from the last time `?` was
		// pressed would highlight lines the reader never asked about, and n
		// would jump a page they have only just opened.
		m.helpQuery, m.helpMatch, m.helpSearching = "", -1, false
		m.helpStatus = ""
		return m, nil
	case "d":
		pr, ok := m.selected()
		if !ok {
			return m, nil
		}
		m.showChecks = true
		cmd := m.fetchDetail(pr)
		if cmd == nil {
			return m, nil
		}
		// A board that has finished fetching has no live tick, so the loader
		// would sit on one frozen frame. Restarting it here is safe because
		// fetchDetail returns nil unless it actually started a request, and a
		// still-fetching board already has its own tick -- adding a second
		// would run the spinner at double speed.
		if m.fetching {
			return m, cmd
		}
		return m, tea.Batch(cmd, spinTick())
	case "v":
		// Range mode. Contiguous only, following lazygit: a scattered range has
		// no clear meaning for some actions, and `space` already covers the
		// scattered case.
		m.startRange()
		return m, nil
	case " ":
		// Toggle the PR under the cursor. The cursor deliberately does NOT
		// advance: it saves a keypress going down the board and costs one
		// going up, and k9s closed that request as not-planned for exactly
		// that reason.
		m.toggleSelect()
		return m, nil
	case "n":
		return m.stepMatch(true)
	case "N":
		return m.stepMatch(false)
	case "j", "down":
		m.cursor++
		m.clampCursor()
	case "k", "up":
		m.cursor--
		m.clampCursor()
	// The board has no viewport of its own -- the cursor walks the slots and
	// the window follows it -- so a page key is the same move as j, taken a
	// page at a time.
	case "ctrl+d":
		m.cursor += halfPage(m.height)
		m.clampCursor()
	case "ctrl+u":
		m.cursor -= halfPage(m.height)
		m.clampCursor()
	case "pgdown":
		m.cursor += fullPage(m.height)
		m.clampCursor()
	case "pgup":
		m.cursor -= fullPage(m.height)
		m.clampCursor()
	case "l", "right":
		m.cursor = m.nextSection()
		m.clampCursor()
	case "h", "left":
		m.cursor = m.prevSection()
		m.clampCursor()
	// Documented as `g`, not `gg`: bare `g` is already the whole move, so
	// advertising a chord that is really one key repeated was confusing. A
	// second `g` still lands in the same place -- top is its own fixed point --
	// so vim fingers typing `gg` cost nothing and there is no pending-key mode
	// a stray `g` could wedge.
	case "g", "home":
		m.cursor = 0
		m.clampCursor()
	case "G", "end":
		m.cursor = len(m.slots()) - 1
		m.clampCursor()
	case "r":
		return m.refresh()
	case "enter", "o":
		next, cmd := m.openSelected()
		return next, cmd
	case "y":
		next, cmd := m.copySelected()
		return next, cmd
	case "Y":
		return m.openCopyMenu(), nil
	default:
		// User-configured actions are matched last so they cannot shadow
		// navigation keys.
		if next, cmd, ok := m.actionFor(msg.String()); ok {
			return next, cmd
		}
	}
	return m, nil
}

// handleHelpKey scrolls the help page or closes it. The page used to close on
// any key; scrolling took j/k away from that, so the rule is now the inverse of
// a mode: the scroll keys scroll, and **everything else closes**. A key the
// reader guesses at still leaves the page, which is what keeps this from being
// somewhere you can get stuck -- and the page says so on its bottom row, since
// the old contract is no longer true.
//
// The search keys are the fourth exception to that rule, after the scroll
// keys. `/` opens the prompt, `n` and `N` walk the matches, and `esc` clears
// the highlights before it closes the page -- the board's own bindings, doing
// the board's own thing, so the legend is searched the way the list is.
func (m Model) handleHelpKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.helpSearching {
		return m.handleHelpSearchKey(msg)
	}
	switch msg.String() {
	case "j", "down":
		m.helpScroll++
	case "k", "up":
		m.helpScroll--
	case "ctrl+d":
		m.helpScroll += halfPage(m.height)
	case "ctrl+u":
		m.helpScroll -= halfPage(m.height)
	case "pgdown":
		m.helpScroll += fullPage(m.height)
	case "pgup":
		m.helpScroll -= fullPage(m.height)
	case "g", "home":
		m.helpScroll = 0
	case "G", "end":
		m.helpScroll = len(m.helpLines())
	case "/":
		m.helpSearching, m.helpQuery, m.helpMatch = true, "", -1
		// The page stays where it is: a search does not reflow the legend, so
		// there is nothing to jump to yet, and helpOrigin is what esc restores
		// after incsearch has scrolled it away.
		m.helpOrigin = m.helpScroll
		return m, nil
	case "n":
		return m.stepHelpMatch(true)
	case "N":
		return m.stepHelpMatch(false)
	case "esc":
		// The :noh of the legend, same as the board's: a page with no visible
		// highlights where n still jumps would be a mode with nothing on
		// screen to say so. With no query there is nothing to clear, so esc
		// means what it always did and closes the page.
		if m.helpQuery != "" {
			m.helpQuery, m.helpMatch, m.helpStatus = "", -1, ""
			return m, nil
		}
		return m.closeHelp(), nil
	default:
		return m.closeHelp(), nil
	}
	m.clampHelpScroll()
	return m, nil
}

// closeHelp leaves the legend and takes its search with it. The wrap
// announcement in particular is about a page that is no longer on screen, and
// the board's own footer is where it would otherwise be read.
func (m Model) closeHelp() Model {
	m.showHelp, m.helpSearching = false, false
	m.helpQuery, m.helpMatch, m.helpStatus = "", -1, ""
	return m
}

// fullPage and halfPage are what pgdn/pgup and ctrl+d/ctrl+u move by. The board
// and the help page share them so the two surfaces cannot drift into meaning
// different things by the same key.
//
// Both floor at 1, which is deliberately not helpBody's floor of 0. That one
// answers how many lines fit, where zero is the honest answer for a pane with
// no room; these answer how far a keypress moves, where zero is a dead key. A
// viewport showing nothing is a real state, a motion key moving nothing is not.
func fullPage(height int) int { return max(1, height-2) }

func halfPage(height int) int { return max(1, (height-2)/2) }

// handleConfirmKey answers the do-this-to-this-many prompt. `y` and `enter` go
// ahead; anything else does not.
//
// The default is NO: every key that is not an explicit yes cancels, rather than
// only esc cancelling and stray keys falling through to the board. A prompt
// guarding an unrecoverable action should not be dismissable by a keypress
// aimed at something else.
func (m Model) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "y", "enter":
		prs, key := m.confirmOpen, m.confirmAction
		m.clearConfirm()
		if key != "" {
			next, cmd, _ := m.runAction(key, prs)
			return next, cmd
		}
		return m.doOpen(prs)
	default:
		// The selection survives a cancel, so the user can adjust it rather
		// than rebuild it.
		m.clearConfirm()
		return m, nil
	}
}

func (m *Model) clearConfirm() {
	m.confirmOpen, m.confirmAction, m.confirmVerb = nil, "", ""
}
