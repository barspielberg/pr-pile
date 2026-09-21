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
// under it, so holding `j` with `c` at each row leaves requests that answer a
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
	case "ctrl+n", "ctrl+j", "down":
		m.cursor = m.matchAfter(m.cursor)
		m.clampCursor()
		return m, nil
	case "ctrl+p", "ctrl+k", "up":
		m.cursor = m.matchBefore(m.cursor)
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
	for _, idx := range matches {
		if idx == m.cursor {
			return
		}
	}
	// From where the search opened rather than from the cursor: backspacing to
	// a wider query has to be able to walk back up, not only further down.
	for _, idx := range matches {
		if idx >= m.searchOrigin {
			m.cursor = idx
			return
		}
	}
	m.cursor = matches[0]
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
		m.cursor = len(m.visibleRows()) - 1
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

	var next int
	var wrapped bool
	if forward {
		next = m.matchAfter(m.cursor)
		wrapped = next <= m.cursor
	} else {
		next = m.matchBefore(m.cursor)
		wrapped = next >= m.cursor
	}
	m.cursor = next
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

// sectionStarts gives the cursor index of each non-empty section's first row,
// so l/h can jump between them without the cursor knowing about sections.
func (m Model) sectionStarts() []int {
	var starts []int
	idx := 0
	for _, s := range m.sections() {
		if len(s.Rows) > 0 {
			starts = append(starts, idx)
			idx += len(s.Rows)
		}
	}
	return starts
}

// nextSection moves to the first row of the following section, or the last row
// when there is none -- the same end-stop behaviour as j.
func (m Model) nextSection() int {
	for _, start := range m.sectionStarts() {
		if start > m.cursor {
			return start
		}
	}
	if n := len(m.visibleRows()); n > 0 {
		return n - 1
	}
	return 0
}

// prevSection moves to the start of the current section, or to the previous
// one when already there, which is how a "back" key is expected to feel.
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

func (m Model) selected() (github.PR, bool) {
	rows := m.visibleRows()
	if m.cursor < 0 || m.cursor >= len(rows) {
		return github.PR{}, false
	}
	return rows[m.cursor].PR, true
}

// Sections resolve progressively, so the row under the cursor can disappear
// between frames. searchOrigin is clamped alongside it: a refresh can reorder
// the board while the prompt is open, and esc landing a row or two off is
// acceptable where an out-of-range index is not.
func (m *Model) clampCursor() {
	n := len(m.visibleRows())
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

// lineMeta says which section a rendered line belongs to and where the line
// sits within it, so the top row's gutter can be resolved after window() has
// chosen a start. Without it the label would have to be decided before the
// window is known, which is why every row above a scrolled-off boundary used
// to go unnamed.
type lineMeta struct {
	section  string
	author   bool // the section's rule shows an author column
	row      int  // index within the section, -1 for notes and blanks
	isRow    bool
	rowIndex int // index into the flattened visible rows, -1 when not a row
}

// body renders every section and reports the line each row starts on, so the
// viewport can scroll to the cursor. Every row is exactly one line, but section
// notes and placeholder blanks sit between them, so the line is not the row
// index.
func (m Model) body(spin string) (lines []string, rowStarts []int, meta []lineMeta) {
	idx := 0
	add := func(line string, mt lineMeta) {
		lines = append(lines, line)
		meta = append(meta, mt)
	}
	addRow := func(row board.Row, s board.Section, i int) {
		rowStarts = append(rowStarts, len(lines))
		gs := sectionContinues
		if i == 0 {
			gs = sectionStarts
		}
		add(m.renderRow(row, idx == m.cursor, s.Rule.Author, s.Rule.Name, gs),
			lineMeta{section: s.Rule.Name, author: s.Rule.Author, row: i,
				isRow: true, rowIndex: idx})
		idx++
	}
	note := func(s board.Section, text string) {
		add(m.renderSectionNote(s.Rule.Name, text),
			lineMeta{section: s.Rule.Name, row: -1, rowIndex: -1})
	}
	for _, s := range m.sections() {
		switch s.State {
		case board.Pending:
			// A section that already has rows keeps them, so only the spinner
			// in the gutter changes while the refetch is in flight.
			for i, row := range s.Rows {
				addRow(row, s, i)
			}
			// On a cold start there is nothing to keep, so hold a placeholder
			// block instead: without it each section that lands pushes every
			// section below it down the screen.
			if len(s.Rows) == 0 {
				note(s, spin)
				for _, b := range blanks(m.placeholderRows(s.Rule) - 1) {
					add(b, lineMeta{section: s.Rule.Name, row: -1, rowIndex: -1})
				}
			}
		case board.Failed:
			note(s, errorStyle.Render(clip(s.Err.Error(), max(0, m.width-sectionWidth-3))))
		case board.Ready:
			if len(s.Rows) == 0 {
				// A resolved empty section collapses to one line: it knows it
				// has nothing, so holding six blank rows for it would waste
				// most of a short pane. The shrink is the value changing,
				// which is the one reason a row is allowed to move.
				note(s, mutedStyle.Render("—"))
			}
			for i, row := range s.Rows {
				addRow(row, s, i)
			}
		}
	}
	return lines, rowStarts, meta
}

func blanks(n int) []string {
	if n < 1 {
		return nil
	}
	return make([]string, n)
}

// placeholderRows reserves roughly the space a pending section will occupy, so
// the skeleton is close to its final height from the first frame. Capped well
// below the rule's limit: overshooting would scroll real rows off the bottom.
func (m Model) placeholderRows(r config.Rule) int {
	const cap = 6
	n := r.PageSize()
	if n > cap {
		n = cap
	}
	if m.height > 0 {
		if budget := (m.height - 2) / max(1, len(m.cfg.Rules)); n > budget {
			n = budget
		}
	}
	if n < 1 {
		return 1
	}
	return n
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
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
// read as jumping. Now every line on the board is a row -- gate names live in
// the `c` overlay and the section name lives in each row's left gutter -- so
// the clamp below is the whole of it. See docs/uniform-rows.md.
//
// Clamping both edges rather than pinning one means the board does not move at
// all while the cursor crosses the middle, which is the conventional behaviour
// (vim's scrolloff, less, fzf).
// It also reports the index of the top visible line, which the sticky header
// and the top row's gutter both need: both name the section of the row you are
// actually looking at, and neither can know that before the slice is chosen.
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
		for i, idx := range matches {
			if idx == m.cursor {
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
func (m Model) cursorSection() (name string, pos, total int) {
	idx := 0
	for _, s := range m.sections() {
		if m.cursor >= idx && m.cursor < idx+len(s.Rows) {
			return s.Rule.Name, m.cursor - idx + 1, len(s.Rows)
		}
		idx += len(s.Rows)
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
	// section changes under every keypress. The gutter only has 8 cells for it,
	// so this is the one place the full name and the count are legible.
	right := m.cfg.Repo
	if name, pos, total := m.cursorSection(); name != "" {
		right = fmt.Sprintf("%s · %d of %d", strings.ToUpper(name), pos, total)
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

	lines, rowStarts, meta := m.body(spin)
	start := 0
	if m.height > 0 {
		avail := m.height - chrome
		lines, start = window(lines, m.cursor, avail, rowStarts)
		// Pad to the full height so the prompt and footer stay pinned to the
		// bottom edge instead of floating under a short result set.
		for len(lines) < avail {
			lines = append(lines, "")
		}
	}
	lines = m.nameTopSection(lines, meta, start)
	return strings.Join(lines, "\n") + "\n" + foot
}

// nameTopSection re-renders the top visible line so it carries its section's
// name even when the section began above the window. It keeps the rule at `│`:
// `╷` claims a section starts on this row, which is false here, and the two
// facts are worth keeping apart.
func (m Model) nameTopSection(lines []string, meta []lineMeta, start int) []string {
	if len(lines) == 0 || start >= len(meta) {
		return lines
	}
	mt := meta[start]
	if !mt.isRow || mt.row == 0 {
		return lines
	}
	rows := m.visibleRows()
	if mt.rowIndex < 0 || mt.rowIndex >= len(rows) {
		return lines
	}
	out := append([]string(nil), lines...)
	out[0] = m.renderRow(rows[mt.rowIndex], mt.rowIndex == m.cursor,
		mt.author, mt.section, sectionAbove)
	return out
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
