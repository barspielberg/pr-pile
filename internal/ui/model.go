// Package ui is the Bubble Tea board.
package ui

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"text/template"
	"time"

	"github.com/barspielberg/prs-mng/internal/board"
	"github.com/barspielberg/prs-mng/internal/browser"
	"github.com/barspielberg/prs-mng/internal/config"
	"github.com/barspielberg/prs-mng/internal/github"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Model struct {
	cfg    config.Config
	client *github.Client
	board  *board.Board

	width, height int
	cursor        int // index into the flattened visible rows
	spinner       int
	status        string
	fetching      bool
	// running is the name of the background action in flight, empty when none.
	// It is what the footer animates on, and what makes a second press of the
	// same key a no-op rather than a second process.
	running string
	// runSeq numbers action runs so a result that lands after the user started
	// another one is dropped instead of overwriting the newer status.
	runSeq int
	// clearSeq is the run whose status may expire on its own. Only a success
	// sets it: a failure is the one message the user has to act on, so it
	// stays until something else takes the line.
	clearSeq int

	searching bool
	query     string
	// searchOrigin is where the cursor was when / was pressed, so esc can put
	// it back after incsearch has walked it across the board.
	searchOrigin int
	showHelp     bool
	showChecks   bool
	// helpScroll is the help page's top line. The legend outgrows a short pane
	// and the reader needs all of it, so that page scrolls rather than clips.
	helpScroll int

	// detail holds what the on-demand request returned, keyed by PR number.
	// The key is what makes stale responses harmless: one that lands after the
	// cursor has moved on files itself under the PR it describes rather than
	// overwriting whatever is selected now, so nothing has to be cancelled.
	detail map[int]github.Detail
	// inflight is the set of PRs already asked about, so holding `j` cannot
	// fire the same request twice while the first is still out.
	inflight map[int]bool
}

type resultMsg board.Result
type detailMsg struct {
	detail github.Detail
	err    error
}
type tickMsg time.Time
type spinMsg time.Time
type refreshMsg struct{}
type statusMsg string

// actionStartMsg and actionDoneMsg bracket a background action. Start carries
// the name so the footer can say what is running; done carries the same seq so
// a stale result cannot clobber a newer run's status.
type actionStartMsg struct {
	name string
	seq  int
}
type actionDoneMsg struct {
	name string
	seq  int
	err  error
	// detail is the command's last line of stderr, which is where a script
	// says what actually went wrong. "action failed: exit status 1" is not
	// worth showing when the script already said "herdr not running".
	detail string
}

var spinFrames = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")

func New(cfg config.Config, client *github.Client) Model {
	return Model{cfg: cfg, client: client, board: board.New(cfg), width: 100, fetching: true,
		detail: map[int]github.Detail{}, inflight: map[int]bool{}}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(append(m.fetchAll(), spinTick())...)
}

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
		return resultMsg{Index: i, PRs: prs, Err: err}
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
	if _, done := m.detail[pr.Number]; done || m.inflight[pr.Number] {
		return nil
	}
	m.inflight[pr.Number] = true

	repo, number, head := m.cfg.Repo, pr.Number, pr.HeadRefName
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		d, err := client.Detail(ctx, repo, number, head)
		d.Number = number // so a failed response is still attributable
		return detailMsg{detail: d, err: err}
	}
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
		m.board.Apply(board.Result(msg))
		if !m.board.Loading() {
			m.fetching = false
			m.clampCursor()
			return m, m.refreshTick()
		}
		return m, nil

	case tickMsg, refreshMsg:
		return m.refresh()

	case detailMsg:
		delete(m.inflight, msg.detail.Number)
		// A failed request costs the on-demand lines and nothing else: the
		// overlay is already on screen and already useful without them, and
		// saying so at the user would be noise about a page they are reading.
		if msg.err == nil && msg.detail.Number != 0 {
			m.detail[msg.detail.Number] = msg.detail
		}
		return m, nil

	case statusMsg:
		m.status = string(msg)
		return m, nil

	case actionStartMsg:
		m.running, m.status = msg.name, ""
		// A board that has finished fetching has no live tick, so without this
		// the running glyph would sit on one frame for the whole run. The
		// still-fetching board already has its own tick; a second would run
		// the spinner at double speed.
		if m.fetching || len(m.inflight) > 0 {
			return m, nil
		}
		return m, spinTick()

	case actionDoneMsg:
		// A result from a run the user has already superseded says nothing
		// about what is on screen now.
		if msg.seq != m.runSeq {
			return m, nil
		}
		m.running = ""
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

func (m Model) refresh() (tea.Model, tea.Cmd) {
	// Keep the current rows on screen while refetching, so the board does not
	// collapse and re-expand under the cursor.
	m.board.Refetch()
	m.fetching = true
	m.status = ""
	// A refresh does not kill the process, but the board it was launched from
	// is gone; keeping its name on the footer would attribute the fetch
	// spinner to the action. Its result still lands, keyed by seq.
	m.running = ""
	return m, tea.Batch(append(m.fetchAll(), spinTick())...)
}

// While searching every printable key belongs to the query, so navigation has
// to move to chords. ctrl+n/p is what the user asked for; ctrl+j/k and the
// arrows are the same motions under the other two conventions.
func (m Model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		return m.cancelSearch(), nil
	// Matches are rows, so these land on rows and never on a header or a note.
	// matchAfter/matchBefore work in row space; the cursor addresses slots, so
	// the index is converted at this boundary rather than teaching the search
	// about section furniture.
	//
	// An empty query is not a search, so these fall through to plain movement
	// instead -- which means they stop on headers exactly as j and k do. Two
	// motion keys disagreeing about the same board is the kind of thing that
	// gets noticed in use, and "the cursor sits on headers" is the whole design
	// this layout rests on (docs/uniform-rows.md §4.1), so the fallback honours
	// it rather than quietly skipping furniture.
	case "ctrl+n", "ctrl+j", "down":
		if m.noQuery() {
			m.cursor++
		} else {
			m.cursor = m.rowSlotClamped(m.matchAfter(m.cursorRow()))
		}
		m.clampCursor()
		return m, nil
	case "ctrl+p", "ctrl+k", "up":
		if m.noQuery() {
			m.cursor--
		} else {
			m.cursor = m.rowSlotClamped(m.matchBefore(m.cursorRow()))
		}
		m.clampCursor()
		return m, nil
	case "enter":
		return m.acceptSearch(), nil
	case "ctrl+u":
		m.query = ""
	case "backspace":
		if r := []rune(m.query); len(r) > 0 {
			m.query = string(r[:len(r)-1])
		}
	default:
		// Space arrives as its own key type with no runes attached, so it has
		// to be spelled out or multi-word queries would silently drop it.
		switch {
		case msg.Type == tea.KeySpace:
			m.query += " "
		case msg.Type == tea.KeyRunes && len(msg.Runes) > 0:
			m.query += string(msg.Runes)
		default:
			return m, nil
		}
	}
	// No re-seating on every keystroke: a search does not narrow the board, so
	// the cursor's slot still means what it meant. previewMatch below walks it
	// to a match when one exists and deliberately leaves it alone when none
	// does.
	m.clampCursor()
	m.previewMatch()
	return m, nil
}

// previewMatch is incsearch: the cursor walks to the match as the query is
// typed, so the answer is on screen before the user stops typing.
//
// It stays put when the row it is on still matches -- otherwise typing the
// middle of a word would jitter the cursor off a row it had already found --
// and when nothing matches at all, since a query on its way to matching should
// not throw away where the user was.
func (m *Model) previewMatch() {
	// An empty query is the state the prompt opened in, so the cursor belongs
	// where it opened. Deleting back to nothing otherwise stranded it wherever
	// the last near-miss walked it -- a move the user never asked for, and one
	// esc would have undone.
	if m.query == "" {
		m.cursor = m.searchOrigin
		return
	}
	matches := m.matchIndexes()
	if len(matches) == 0 {
		return
	}
	// matchIndexes is in row space and the cursor addresses slots, so the
	// comparison happens in row space and the assignment converts back. Doing
	// it the other way round parks the cursor on a section header, which is
	// never a match and costs the matched row its selection.
	cur := m.cursorRow()
	for _, idx := range matches {
		if idx == cur {
			return
		}
	}
	// From where the search opened rather than from the cursor: backspacing to
	// a wider query has to be able to walk back up, not only further down.
	origin := m.rowAt(m.searchOrigin)
	for _, idx := range matches {
		if idx >= origin {
			m.cursor = m.rowSlotClamped(idx)
			return
		}
	}
	m.cursor = m.rowSlotClamped(matches[0])
}

// matchAfter is the first match below i, wrapping to the top. With no query,
// or no match, it is the next row -- so the chords still move on an empty
// prompt rather than doing nothing.
func (m Model) matchAfter(i int) int {
	matches := m.matchIndexes()
	if len(matches) == 0 {
		return i + 1
	}
	for _, idx := range matches {
		if idx > i {
			return idx
		}
	}
	return matches[0]
}

// matchBefore is the first match above i, wrapping to the bottom.
func (m Model) matchBefore(i int) int {
	matches := m.matchIndexes()
	if len(matches) == 0 {
		return i - 1
	}
	for j := len(matches) - 1; j >= 0; j-- {
		if matches[j] < i {
			return matches[j]
		}
	}
	return matches[len(matches)-1]
}

// cancelSearch is esc in the prompt: the search is abandoned, so the cursor
// goes back to where / was pressed. incsearch walked it while the query was
// being typed, and leaving it wherever the last near-miss happened to be would
// be a move the user never asked for.
func (m Model) cancelSearch() Model {
	m.searching = false
	m.query = ""
	m.cursor = m.searchOrigin
	m.clampCursor()
	return m
}

// acceptSearch is enter: the prompt closes and everything else stays -- the
// cursor on its match, the query live, the highlights on the board. That is
// vim's hlsearch, and it is what gives n and N something to walk.
//
// It no longer opens the PR. <CR> accepts a search everywhere else this model
// comes from, and enter or o is still one keypress away.
func (m Model) acceptSearch() Model {
	m.searching = false
	m.clampCursor()
	return m
}

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

// copySelected yanks the selected PR's URL. With nothing selected -- an empty
// board -- there is no URL to copy and saying so is better than a silent
// no-op.
func (m Model) copySelected() tea.Cmd {
	pr, ok := m.selected()
	if !ok {
		return func() tea.Msg { return statusMsg("no PR selected") }
	}
	url, number := pr.URL, pr.Number
	if url == "" {
		return func() tea.Msg { return statusMsg("no URL for this PR") }
	}
	return func() tea.Msg {
		if err := copyToClipboard(url); err != nil {
			return statusMsg("copy failed: " + err.Error())
		}
		return statusMsg(fmt.Sprintf("copied #%d url", number))
	}
}

func (m Model) openSelected() tea.Cmd {
	pr, ok := m.selected()
	if !ok {
		return nil
	}
	url := pr.URL
	return func() tea.Msg {
		if err := browser.Open(url); err != nil {
			return statusMsg("open failed: " + err.Error())
		}
		return statusMsg("")
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
	switch msg.String() {
	case "esc":
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
	case "G", "end":
		m.cursor = len(m.slots()) - 1
		m.clampCursor()
	case "r":
		return m.refresh()
	case "enter", "o":
		if cmd := m.openSelected(); cmd != nil {
			return m, cmd
		}
	case "y":
		return m, m.copySelected()
	default:
		// User-configured actions are matched last so they cannot shadow
		// navigation keys.
		if next, cmd, ok := m.actionFor(msg.String()); ok {
			return next, cmd
		}
	}
	return m, nil
}

// stepMatch is n and N: the next or previous match, wrapping like vim's
// wrapscan. There is no opening direction to be relative to -- ? is the help
// key, so there is no backwards-open -- which makes n always forward and N
// always backward, vim's own post-/ rule with the unreachable half removed.
//
// The wrap is announced, in less's wording. A silent wrap is indistinguishable
// from being stuck on the last match.
func (m Model) stepMatch(forward bool) (tea.Model, tea.Cmd) {
	matches := m.matchIndexes()
	if len(matches) == 0 {
		if strings.TrimSpace(m.query) == "" {
			return m, nil
		}
		return m, func() tea.Msg { return statusMsg("no matches") }
	}

	// All of this is row space: matchAfter/matchBefore take and return row
	// indices, and the wrap is detected by comparing rows. The cursor, which
	// addresses slots, is converted once at the end.
	cur := m.cursorRow()
	var next int
	var wrapped bool
	if forward {
		next = m.matchAfter(cur)
		wrapped = next <= cur
	} else {
		next = m.matchBefore(cur)
		wrapped = next >= cur
	}
	m.cursor = m.rowSlotClamped(next)
	m.clampCursor()
	if !wrapped {
		m.status = ""
		return m, nil
	}
	msg := "search hit BOTTOM, continuing at TOP"
	if !forward {
		msg = "search hit TOP, continuing at BOTTOM"
	}
	return m, func() tea.Msg { return statusMsg(msg) }
}

// handleHelpKey scrolls the help page or closes it. The page used to close on
// any key; scrolling took j/k away from that, so the rule is now the inverse of
// a mode: the scroll keys scroll, and **everything else closes**. A key the
// reader guesses at still leaves the page, which is what keeps this from being
// somewhere you can get stuck -- and the page says so on its bottom row, since
// the old contract is no longer true.
func (m Model) handleHelpKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	page := max(1, m.height-2)
	switch msg.String() {
	case "j", "down":
		m.helpScroll++
	case "k", "up":
		m.helpScroll--
	case "ctrl+d", "pgdown":
		m.helpScroll += page
	case "ctrl+u", "pgup":
		m.helpScroll -= page
	case "g", "home":
		m.helpScroll = 0
	case "G", "end":
		m.helpScroll = len(m.helpLines())
	default:
		m.showHelp = false
		return m, nil
	}
	// Clamped here as well as at render. Render-time clamping alone lets the
	// stored offset drift past the end while j is held, and then the first k
	// only walks that invisible surplus back down -- the page sits still for as
	// many presses as it overshot, which reads as k being broken.
	m.helpScroll = m.helpTop(len(m.helpLines()), max(1, m.height-1))
	return m, nil
}

func (m Model) actionFor(key string) (Model, tea.Cmd, bool) {
	pr, ok := m.selected()
	if !ok {
		return m, nil, false
	}
	for _, a := range m.cfg.Actions {
		if a.Key != key || strings.TrimSpace(a.Run) == "" {
			continue
		}
		// A second press while one is still out would start a second process
		// and lose the first's result to the seq check. Saying so is more
		// useful than silently doing nothing.
		if m.running != "" {
			running := m.running
			return m, func() tea.Msg { return statusMsg(running + " still running") }, true
		}
		line, err := m.renderAction(a.Run, pr)
		if err != nil {
			return m, func() tea.Msg { return statusMsg("action: " + err.Error()) }, true
		}
		m.runSeq++
		cmd := exec.Command("sh", "-c", line)
		if a.Mode == "suspend" {
			// Hand the terminal over for TUI commands (a diff pager, a review
			// session), then repaint when they exit.
			name := a.Name
			return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
				if err != nil {
					return statusMsg(name + " failed: " + err.Error())
				}
				return statusMsg("")
			}), true
		}
		// Two messages, not one: the footer has to say "running" the moment
		// the key is pressed, and the process may take seconds to answer. The
		// wait happens inside a tea.Cmd so the board stays responsive.
		seq := m.runSeq
		return m, tea.Batch(
			func() tea.Msg { return actionStartMsg{name: a.Name, seq: seq} },
			runAction(cmd, a.Name, seq),
		), true
	}
	return m, nil, false
}

// statusHold is how long a succeeded action holds the footer. Long enough to
// read after looking back from whatever the action opened, short enough that
// the line is not still claiming an action when the user next looks down.
const statusHold = 4 * time.Second

type clearStatusMsg int

func clearStatusIn(d time.Duration, seq int) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return clearStatusMsg(seq) })
}

// runAction waits for the process off the UI goroutine and reports how it went.
// It also reaps, which the fire-and-forget version did only as a side effect of
// throwing the result away.
func runAction(cmd *exec.Cmd, name string, seq int) tea.Cmd {
	// Bounded, because a command that streams to stderr should not be able to
	// grow the board's memory; the tail is the part that says what failed.
	var errBuf tailWriter
	errBuf.limit = 4096
	cmd.Stderr = &errBuf
	return func() tea.Msg {
		err := cmd.Run()
		return actionDoneMsg{name: name, seq: seq, err: err, detail: errBuf.lastLine()}
	}
}

// actionResult is the one line the footer shows when an action ends. A failure
// prefers what the command said on stderr to Go's exit-status wording, which
// names the mechanism and not the problem.
func actionResult(msg actionDoneMsg) string {
	if msg.err == nil {
		return msg.name + " ✓"
	}
	if d := msg.detail; d != "" {
		return msg.name + " failed: " + d
	}
	return msg.name + " failed: " + msg.err.Error()
}

func (m Model) renderAction(tmpl string, pr github.PR) (string, error) {
	t, err := template.New("action").Parse(tmpl)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	err = t.Execute(&b, map[string]any{
		"Number": pr.Number, "Repo": m.cfg.Repo, "RepoPath": m.cfg.RepoPath,
		"Branch": pr.HeadRefName, "Base": pr.BaseRefName,
		"URL": pr.URL, "Author": pr.Author, "Title": pr.Title,
	})
	return b.String(), err
}

// sections is the board's own sections, unconditionally. A search marks rows
// where they are rather than collecting them: you usually care about the rows
// around the one you are looking for, and a board that reshuffles under the
// query cannot highlight what it hid.
func (m Model) sections() []board.Section {
	return m.board.Sections()
}

// matchIndexes is every matching row's index into the flattened visible rows.
// It is recomputed rather than cached: the board refreshes under the query and
// the width changes what is drawn, so a stored match set would go stale
// silently -- and a stale highlight is the one thing this design cannot show.
func (m Model) matchIndexes() []int {
	if strings.TrimSpace(m.query) == "" {
		return nil
	}
	var out []int
	idx := 0
	for _, s := range m.sections() {
		for _, r := range s.Rows {
			if m.rowMatches(r, s.Rule.Author, m.query) {
				out = append(out, idx)
			}
			idx++
		}
	}
	return out
}

// sectionStarts gives the cursor index of each section's header, so l/h can
// jump between them without the cursor knowing about sections.
//
// The target is the header, not the first PR row. The header is the section's
// own first slot, so a jump lands on the thing that names where you have
// arrived -- and `j` from there is the first PR, which is one extra keypress
// only if you did not want the orientation.
//
// Every section is listed, including empty and still-loading ones: they have a
// header now, so there is somewhere to land. The old gutter had nothing to
// point at in an empty section and had to skip it.
// Derived from slots() rather than counted independently: a second walk that
// has to stay in step with the first is how the note slot desynced the cursor
// once already.
func (m Model) sectionStarts() []int {
	var starts []int
	for i, s := range m.slots() {
		if s.isHeader() {
			starts = append(starts, i)
		}
	}
	return starts
}

// nextSection moves to the following section's header, or to the last slot
// when there is none -- the same end-stop behaviour as j.
func (m Model) nextSection() int {
	for _, start := range m.sectionStarts() {
		if start > m.cursor {
			return start
		}
	}
	if n := len(m.slots()); n > 0 {
		return n - 1
	}
	return 0
}

// prevSection moves to the current section's header, or to the previous one
// when already there, which is how a "back" key is expected to feel.
func (m Model) prevSection() int {
	starts := m.sectionStarts()
	for i := len(starts) - 1; i >= 0; i-- {
		if starts[i] < m.cursor {
			return starts[i]
		}
	}
	return 0
}

// visibleRows flattens the drawable sections so the cursor can move across
// section boundaries without knowing about them.
func (m Model) visibleRows() []board.Row {
	var rows []board.Row
	for _, s := range m.sections() {
		// Stale rows are drawn, so they must be navigable too.
		rows = append(rows, s.Rows...)
	}
	return rows
}

// slotKind is what the cursor is sitting on. A header is addressable but not
// actionable: the cursor can rest there, and every key that operates on a PR
// does nothing.
type slotKind int

const (
	slotRow slotKind = iota
	slotHeader
	// slotNote is a section's single no-rows line: the spinner while loading,
	// an em dash once resolved empty, the error text on failure. Addressable
	// so that it is not an unselectable line in the middle of the list, and
	// not actionable because there is no PR behind it.
	slotNote
)

// slot is one position in the cursor's address space. That space contains the
// section headers as well as the PR rows, which is what makes every line of
// the list reachable and the one-keypress-one-line invariant hold by
// construction rather than by viewport arithmetic: with nothing to skip, the
// cursor's line and its index move together. docs/uniform-rows.md §4.1.
//
// The kind is carried on the value rather than inferred from the index,
// because index arithmetic is exactly what goes wrong once the address space
// holds two things: "slot 3" tells you nothing about whether it is a header,
// and code that hand-counts past headers is code that breaks when a section
// empties. Ask the slot what it is.
type slot struct {
	kind    slotKind
	section string
	row     board.Row
	rowIdx  int // index into visibleRows(), -1 for a header
}

func (s slot) isHeader() bool { return s.kind == slotHeader }
func (s slot) isRow() bool    { return s.kind == slotRow }
func (s slot) isNote() bool   { return s.kind == slotNote }

// slotAt returns the slot the cursor is on, and whether there is one.
func (m Model) slotAt(i int) (slot, bool) {
	sl := m.slots()
	if i < 0 || i >= len(sl) {
		return slot{}, false
	}
	return sl[i], true
}

// rowSlot is the cursor index of the nth PR row, counting across sections and
// ignoring headers, or -1 if there is no such row. This is the accessor a
// caller wants when it means "the third PR on the board" -- notably every test
// that used to say `m.cursor = 3` and mean exactly that.
func (m Model) rowSlot(n int) int {
	for i, s := range m.slots() {
		if s.isRow() && s.rowIdx == n {
			return i
		}
	}
	return -1
}

// headerSlot is the cursor index of the nth section's header, or -1. Sections
// are counted as drawn, so an empty or still-loading section has one too.
func (m Model) headerSlot(n int) int {
	seen := 0
	for i, s := range m.slots() {
		if !s.isHeader() {
			continue
		}
		if seen == n {
			return i
		}
		seen++
	}
	return -1
}

// slots is the cursor's address space: headers interleaved with their rows, in
// draw order. The blank separator between sections is NOT a slot -- it carries
// no information and stopping on it twice per boundary would be a dead beat
// with nothing to read.
// It must agree with body() line for line, because every slot index the rest
// of the model uses -- the cursor, the footer's position, l/h -- is an index
// into this. They are kept in step by construction: both walk m.sections() in
// the same order and emit a slot for the same three things.
func (m Model) slots() []slot {
	var out []slot
	idx := 0
	for _, s := range m.sections() {
		out = append(out, slot{kind: slotHeader, section: s.Rule.Name, rowIdx: -1})
		for _, r := range s.Rows {
			out = append(out, slot{kind: slotRow, section: s.Rule.Name, row: r, rowIdx: idx})
			idx++
		}
		if m.hasNote(s) {
			out = append(out, slot{kind: slotNote, section: s.Rule.Name, rowIdx: -1})
		}
	}
	return out
}

// hasNote says whether a section draws its no-rows line. body() asks the same
// question, so the two cannot drift.
func (m Model) hasNote(s board.Section) bool {
	switch s.State {
	case board.Failed:
		return true
	case board.Ready:
		return len(s.Rows) == 0
	case board.Pending:
		return len(s.Rows) == 0
	}
	return false
}

// firstRowSlot is the index of the first PR row in the address space, or 0 if
// there is none. Filtering uses it: a query narrows the board to matches, so
// opening the cursor on a header -- a line that is by definition not a match --
// would make the first ctrl+n a wasted keypress.
func (m Model) firstRowSlot() int {
	if i := m.rowSlot(0); i >= 0 {
		return i
	}
	return 0
}

// nextRowSlot is the next PR row in direction dir, or the current slot when
// there is none -- the same end-stop behaviour j and k have on the board.
func (m Model) nextRowSlot(dir int) int {
	sl := m.slots()
	for i := m.cursor + dir; i >= 0 && i < len(sl); i += dir {
		if sl[i].isRow() {
			return i
		}
	}
	if m.cursor >= 0 && m.cursor < len(sl) {
		return m.cursor
	}
	return m.firstRowSlot()
}

// noQuery reports whether there is no search to step through. Whitespace does
// not count: rowMatches treats a blank query as matching nothing, so stepping
// it as a search would be a no-op the user reads as a broken key.
func (m Model) noQuery() bool { return strings.TrimSpace(m.query) == "" }

// cursorRow is the cursor's position in ROW space -- the index space the search
// works in, which counts only PR rows. On a header or a note the cursor has no
// row of its own, so the nearest row above it is used: that keeps "the next
// match after here" meaningful wherever the cursor is parked.
func (m Model) cursorRow() int { return m.rowAt(m.cursor) }

// rowAt is the row index at or above slot i, in row space. Unlike cursorRow it
// takes an explicit slot, so searchOrigin can be compared against matches.
func (m Model) rowAt(i int) int {
	sl := m.slots()
	row := -1
	for j := 0; j <= i && j < len(sl); j++ {
		if sl[j].isRow() {
			row = sl[j].rowIdx
		}
	}
	return row
}

// rowSlotClamped converts a row index back into a cursor slot, falling back to
// the first row when the index is out of range. It is the inverse of cursorRow
// and the only place row space and slot space meet.
func (m Model) rowSlotClamped(row int) int {
	if i := m.rowSlot(row); i >= 0 {
		return i
	}
	return m.firstRowSlot()
}

// selected is the PR under the cursor. A header has none, so every key that
// acts on a PR falls through to doing nothing there.
func (m Model) selected() (github.PR, bool) {
	s, ok := m.slotAt(m.cursor)
	if !ok || !s.isRow() {
		return github.PR{}, false
	}
	return s.row.PR, true
}

// Sections resolve progressively, so the slot under the cursor can disappear
// between frames. searchOrigin is clamped alongside it: a refresh can reorder
// the board while the prompt is open, and esc landing a slot or two off is
// acceptable where an out-of-range index is not.
func (m *Model) clampCursor() {
	n := len(m.slots())
	if n == 0 {
		m.cursor, m.searchOrigin = 0, 0
		return
	}
	m.cursor = clampIndex(m.cursor, n)
	m.searchOrigin = clampIndex(m.searchOrigin, n)
}

func clampIndex(i, n int) int {
	if i >= n {
		return n - 1
	}
	if i < 0 {
		return 0
	}
	return i
}

// body renders every section and reports the line each cursor slot starts on.
// EVERY line is a slot -- headers, PR rows and the no-rows note alike -- so
// slotStarts is the identity, which is the property that makes the scroll
// rhythm even. See docs/uniform-rows.md §4.1.
func (m Model) body(spin string) (lines []string, slotStarts []int) {
	slotIdx := 0
	addSlot := func(line string) {
		slotStarts = append(slotStarts, len(lines))
		lines = append(lines, line)
		slotIdx++
	}
	// A section with no rows still occupies exactly one note line -- the
	// spinner while loading, an em dash once resolved empty, the error text on
	// failure -- and that line is a SLOT. It was not, and the cursor skipped
	// it: by the rule in docs/uniform-rows.md §4.1 every unselectable line in
	// the list costs one to the worst-case scroll delta, so an empty, failed
	// or pending section broke the very invariant this layout exists to hold.
	//
	// A note carries no PR, so it is addressable but not actionable, exactly
	// like a header: selected() returns nothing on it and every key that acts
	// on a PR is a silent no-op there.
	note := func(text string) {
		addSlot(renderSectionNote(text))
	}
	// No blank separator between sections. It was drawn at first and measured:
	// a blank is a line the cursor cannot occupy, and every such line costs
	// exactly one to the worst-case scroll delta -- with it the board moved 2
	// lines per keypress at a boundary, without it exactly 1 everywhere. The
	// header's own background is what separates the sections instead, and it
	// costs nothing because the cursor can sit on it.
	header := func(s board.Section, count string) {
		addSlot(m.sectionHeader(s.Rule.Name, count, slotIdx == m.cursor))
	}
	for _, s := range m.sections() {
		switch s.State {
		case board.Pending:
			// The count is unknown until the rule resolves, so the header
			// carries the name alone rather than a number about to change.
			header(s, "")
			for _, row := range s.Rows {
				addSlot(m.renderRow(row, slotIdx == m.cursor, s.Rule.Author))
			}
			// One spinner line, and no placeholder block. The block reserved
			// roughly the section's final height so sections below it were
			// not pushed down as it resolved -- but every line of it was
			// unselectable AND it scaled with the pane, so on a tall board a
			// single keypress moved the viewport several lines. Cold start is
			// every launch, which made that the common case, not an edge one.
			//
			// Layout stability while loading is a real concern and this does
			// give some of it up. The scroll invariant outranks it: a row that
			// moves once as its section resolves is a value changing, which
			// §3.9 already allows, while a board that scrolls five lines per
			// keypress is the defect five earlier attempts were chasing.
			if len(s.Rows) == 0 {
				note(spin)
			}
		case board.Failed:
			header(s, "")
			note(errorStyle.Render(clip(s.Err.Error(), max(0, m.width-3))))
		case board.Ready:
			header(s, fmt.Sprint(len(s.Rows)))
			if len(s.Rows) == 0 {
				// A resolved empty section collapses to one line: it knows it
				// has nothing, so holding six blank rows would waste most of a
				// short pane.
				note(mutedStyle.Render("—"))
			}
			for _, row := range s.Rows {
				addSlot(m.renderRow(row, slotIdx == m.cursor, s.Rule.Author))
			}
		}
	}
	return lines, slotStarts
}

// scrollOff is how many lines of context are kept beyond the cursor, so moving
// down shows what is coming rather than pinning the cursor to the bottom edge.
// lazygit ships 2 and fzf 3; 2 is enough here to always reveal the first line
// of the next PR while costing little of a 20-row pane.
const scrollOff = 2

// window scrolls the body so the cursor keeps scrollOff rows of context on
// whichever edge it is approaching.
//
// The invariant is that ONE keypress scrolls the board by at most ONE line.
// Five earlier attempts could not hold it, because a row's height depended on
// its data and a section header took a line of its own: a single `j` moved the
// world by 0 to 3 lines depending on what happened to be nearby, which is what
// read as jumping. Now every line on the board is addressable -- gate names
// live in the `d` overlay and the section name lives in its own header row the
// cursor can sit on -- so the clamp below is the whole of it. See
// docs/uniform-rows.md.
//
// Clamping both edges rather than pinning one means the board does not move at
// all while the cursor crosses the middle, which is the conventional behaviour
// (vim's scrolloff, less, fzf).
//
// It also reports the index of the top visible line, which the top row needs to
// name the section of the row you are actually looking at -- not knowable
// before the slice is chosen.
func window(lines []string, cursorRow, height int, rowStarts []int) ([]string, int) {
	if height <= 0 || len(lines) <= height {
		return lines, 0
	}
	if len(rowStarts) == 0 || cursorRow < 0 {
		return lines[:height], 0
	}
	if cursorRow >= len(rowStarts) {
		cursorRow = len(rowStarts) - 1
	}
	cur := rowStarts[cursorRow]

	// The margin has to fit above and below the cursor or the two clamps fight
	// and the viewport oscillates; a very short pane centres instead.
	off := scrollOff
	if 2*off+1 > height {
		off = (height - 1) / 2
	}

	// Two bounds on the top line, each shifting by exactly one when the cursor
	// does. Between them the top is free, so the board holds still through the
	// middle of the viewport and only moves at the edges.
	start := cur - (height - 1 - off)
	if lo := cur - off; start > lo {
		start = lo
	}
	if last := len(lines) - height; start > last {
		start = last
	}
	if start < 0 {
		start = 0
	}
	return lines[start : start+height], start
}

// The footer carries the repo and the spinner, so no global header row is
// needed: in a 20-row pane every chrome row costs a PR.
// promptLine is the search's own row, drawn directly above the footer. The
// position sits on the right where the footer already puts its right-hand
// field, so the two chrome rows share one alignment.
//
// The board cannot change to say a query found nothing, so the query text
// itself turns red -- vim's own answer, and the only affordance left when
// nothing on screen is allowed to move. It reverts the moment a match exists,
// which is what makes backspacing back to a match legible.
func (m Model) promptLine() string {
	matches := m.matchIndexes()
	right := ""
	queryStyle := fgStyle
	switch {
	case strings.TrimSpace(m.query) == "":
	case len(matches) == 0:
		right, queryStyle = "no matches", errorStyle
	default:
		right = fmt.Sprintf("%d matches", len(matches))
		if len(matches) == 1 {
			right = "1 match"
		}
		// matchIndexes is in row space and the cursor addresses slots, so the
		// comparison is made in row space: comparing the two directly reports
		// "3 matches" while the cursor sits on one of them.
		cur := m.cursorRow()
		for i, idx := range matches {
			if idx == cur {
				right = fmt.Sprintf("%d of %d", i+1, len(matches))
				break
			}
		}
	}

	const prefix = "  / "
	// The query keeps the tail rather than the head: while typing, the end of
	// what you just entered is the part you are looking at.
	field := m.query + "▏"
	budget := m.width - lipgloss.Width(prefix) - lipgloss.Width(right) - 2
	if budget < 1 {
		// No honest room for the position at this width, so drop it.
		return accentStyle.Render(prefix) +
			queryStyle.Render(clipLeft(field, max(0, m.width-lipgloss.Width(prefix))))
	}
	field = clipLeft(field, budget)

	gap := budget - lipgloss.Width(field)
	return accentStyle.Render(prefix) + queryStyle.Render(field) +
		strings.Repeat(" ", gap) + mutedStyle.Render(right+"  ")
}

// cursorSection names the section the cursor is in and where it sits within it.
// It is bound to the cursor, not to the top visible row: a board that fits the
// pane never scrolls, so a top-row-bound field is frozen at its first section
// forever -- which is exactly how the reverted sticky line failed. See
// docs/section-layout.md §14.
//
// Read off the slot the cursor is actually on, not recomputed by walking the
// sections: the section a slot belongs to is recorded on the slot, and a second
// independent walk is what put the footer one section out when the note slot
// was added.
//
// pos is 0 on a header or a note -- the cursor is at the section rather than at
// a row inside it, and claiming "1 of 12" there would be a position it does not
// have.
func (m Model) cursorSection() (name string, pos, total int) {
	cur, ok := m.slotAt(m.cursor)
	if !ok {
		return "", 0, 0
	}
	for _, s := range m.sections() {
		if s.Rule.Name != cur.section {
			continue
		}
		if !cur.isRow() {
			return s.Rule.Name, 0, len(s.Rows)
		}
		// rowIdx counts across the whole board, so offset by the rows in the
		// sections above this one.
		before := 0
		for _, up := range m.sections() {
			if up.Rule.Name == s.Rule.Name {
				break
			}
			before += len(up.Rows)
		}
		return s.Rule.Name, cur.rowIdx - before + 1, len(s.Rows)
	}
	return "", 0, 0
}

func (m Model) footer(spin string) string {
	left := "  j/k move · l/h section · enter open · d detail · y copy · / search · ? help · q quit"
	switch {
	case m.searching:
		left = "  ctrl+n/p next · enter keep · esc cancel"
	case m.query != "":
		// The query outlives the prompt, so the legend has to say what the two
		// keys that only work now actually do.
		left = "  j/k move · l/h section · enter open · d detail · y copy · n/N next match · esc clear"
	}
	if m.status != "" {
		left = "  " + m.status
	}
	// A running action outranks a status: the status line is history and this
	// is happening now. The glyph is the board's own spinner rather than a
	// static marker -- the tick is kept alive for the duration (see the
	// spinMsg case), so it animates, and an animated glyph is the difference
	// between "working" and "wedged" on a command that takes seconds.
	if m.running != "" {
		left = "  " + string(spinFrames[m.spinner%len(spinFrames)]) + " " + m.running
	}
	// One line, clipped not wrapped: a second row would break the board's
	// one-line-per-row invariant, and stderr from a failing script is
	// arbitrarily long.
	left = clip(left, max(0, m.width-2))
	// The section name takes the right field and the repo yields it: the repo
	// is a constant the user chose and can read in the window title, while the
	// section changes under every keypress. A header scrolls away with its
	// section, so this is the only place the cursor's section is named once you
	// are past the top of it.
	right := m.cfg.Repo
	if name, pos, total := m.cursorSection(); name != "" {
		if pos == 0 {
			// On the header: the section's size, with no false position.
			right = fmt.Sprintf("%s · %d", strings.ToUpper(name), total)
		} else {
			right = fmt.Sprintf("%s · %d of %d", strings.ToUpper(name), pos, total)
		}
	}
	if spin != "" {
		right += " " + spin
	}
	// When the two fields do not both fit, the keys clip and the right field
	// survives whole: the keys are a reminder of things the user already knows,
	// while the section name is the only place the full name and the count are
	// on screen at all. Below that the right field goes too, rather than being
	// clipped into a half-truth like `NEEDS MY REVI`.
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 1 {
		keys := m.width - lipgloss.Width(right) - 3
		if keys < 8 {
			return mutedStyle.Render(clip(left, m.width))
		}
		return mutedStyle.Render(clip(left, keys) + " " + right + "  ")
	}
	return mutedStyle.Render(left + strings.Repeat(" ", gap) + right + "  ")
}

func (m Model) View() string {
	if m.width > 0 && m.width < minWidth {
		return mutedStyle.Render(fmt.Sprintf("  terminal too narrow\n  (need %d cols)", minWidth))
	}

	if m.showHelp {
		return m.helpOverlay()
	}
	if m.showChecks {
		return m.detailOverlay()
	}

	spin := ""
	if m.fetching {
		spin = string(spinFrames[m.spinner%len(spinFrames)])
	}

	foot := m.footer(spin)
	chrome := 1
	if m.searching {
		// The prompt is a second chrome row, so the body has one line less.
		foot = m.promptLine() + "\n" + foot
		chrome = 2
	}

	lines, slotStarts := m.body(spin)
	if m.height > 0 {
		avail := m.height - chrome
		lines, _ = window(lines, m.cursor, avail, slotStarts)
		// Pad to the full height so the prompt and footer stay pinned to the
		// bottom edge instead of floating under a short result set.
		for len(lines) < avail {
			lines = append(lines, "")
		}
	}
	return strings.Join(lines, "\n") + "\n" + foot
}

// tailWriter keeps the last `limit` bytes written to it. A command's stderr is
// unbounded and only its end is wanted, so this drops from the front rather
// than refusing to record once full.
type tailWriter struct {
	buf   []byte
	limit int
}

func (w *tailWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	if over := len(w.buf) - w.limit; over > 0 {
		w.buf = w.buf[over:]
	}
	return len(p), nil
}

// lastLine is the final non-blank line, which is where a shell script's
// diagnostic lands. Anything above it is usually a tool's own progress noise.
func (w *tailWriter) lastLine() string {
	lines := strings.Split(strings.TrimRight(string(w.buf), "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if s := strings.TrimSpace(lines[i]); s != "" {
			return s
		}
	}
	return ""
}
