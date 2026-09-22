package ui

import (
	"github.com/barspielberg/pr-pile/internal/board"
	"github.com/barspielberg/pr-pile/internal/config"
	"github.com/barspielberg/pr-pile/internal/github"
	tea "github.com/charmbracelet/bubbletea"
	"time"
)

type Model struct {
	cfg    config.Config
	client *github.Client
	board  *board.Board

	width, height   int
	cursor          int // index into the flattened visible rows
	spinner         int
	status          string
	fetching        bool
	fetchGeneration uint64
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
	// The help page carries its own search rather than sharing the board's:
	// the two pages are open one at a time, and a query typed at the legend
	// has nothing to say about the PRs behind it.
	helpSearching bool
	helpQuery     string
	// helpOrigin is the scroll position when / was pressed, so esc can put the
	// page back after incsearch has walked it.
	helpOrigin int
	// helpMatch is the line the page is currently sitting on, in helpLines
	// space, or -1 when the query matches nothing. It is what n and N walk and
	// what the prompt counts from.
	helpMatch int
	// helpStatus is the legend's own message line -- the wrap announcement and
	// "no matches". It is separate from status rather than sharing it: status
	// belongs to the board, and routing the legend's messages through it put
	// every action failure and copy confirmation on the legend's bottom row,
	// displacing the closing keys that row exists to carry at every height.
	helpStatus string

	// detail holds what the on-demand request returned, keyed by PR number.
	// The key is what makes stale responses harmless: one that lands after the
	// cursor has moved on files itself under the PR it describes rather than
	// overwriting whatever is selected now, so nothing has to be cancelled.
	detail map[int]github.Detail
	// inflight is the set of PRs already asked about, so holding `j` cannot
	// fire the same request twice while the first is still out.
	inflight map[int]bool
}

type resultMsg struct {
	generation uint64
	result     board.Result
}
type detailMsg struct {
	detail github.Detail
	err    error
}
type tickMsg time.Time
type spinMsg time.Time
type refreshMsg struct{}
type statusMsg string

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
		fetchGeneration: 1, detail: map[int]github.Detail{}, inflight: map[int]bool{}}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(append(m.fetchAll(), spinTick())...)
}
