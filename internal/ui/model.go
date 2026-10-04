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
	top             int // the board's top visible line, kept across frames
	spinner         int
	status          string
	statusSeq       uint64
	fetching        bool
	fetchGeneration uint64
	refreshSeq      uint64
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

	// The selection, by PR number, and the open `v` range: anchor..cursor
	// inclusive, recomputed on every move so walking back shrinks it.
	// rangeOwned is what the range itself marked, kept apart from the user's
	// own space marks so shrinking cannot eat them. See selection.go.
	selection  map[int]bool
	anchor     int
	ranging    bool
	rangeOwned map[int]bool

	// The pending confirm: the PR set captured when the prompt opened (nil
	// when none is up), the action key it guards (empty for the builtin open),
	// and what to call it on the footer. The set is held rather than a count
	// so a refresh landing mid-prompt cannot change what the answer acts on.
	confirmOpen   []github.PR
	confirmAction string
	confirmVerb   string

	// The open `Y` menu: the PRs it copies from (nil when closed) and the
	// highlighted field.
	copyMenu   []github.PR
	copyCursor int

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
	inflight       map[int]detailRequest
	detailIdentity map[int]detailRequest

	// Watches outlive a refresh, unlike the selection: they are keyed by
	// number and nothing about a refetch makes them stale. See watch.go.
	watched map[int]*watchEntry
	// One poll at a time: two out at once would diff against the same
	// baseline and report the same change twice.
	watchInflight bool
	// watchNews is the last poll's status text, kept through a refresh that
	// would otherwise wipe it seconds after it appeared.
	watchNews string
	// boardAt is when the board's current fetch was sent, what a watch
	// poll's copy has to be newer than to be drawn.
	boardAt     time.Time
	confirmQuit bool
}

type detailRequest struct {
	generation uint64
	head       string
}

type resultMsg struct {
	generation uint64
	result     board.Result
}
type detailMsg struct {
	generation uint64
	head       string
	detail     github.Detail
	err        error
}
type tickMsg struct{ seq uint64 }
type spinMsg time.Time
type refreshMsg struct{}
type asyncStatusMsg struct {
	seq  uint64
	text string
	// restore is the selection to put back, set only when an async action
	// failed after the selection was optimistically cleared. Clearing on the
	// way out keeps the footer honest while the copy is in flight; a failure
	// hands the user back exactly what they picked, so retrying is one key.
	restore map[int]bool
}

type actionDoneMsg struct {
	name      string
	seq       int
	statusSeq uint64
	err       error
	// detail is the command's last line of stderr, which is where a script
	// says what actually went wrong. "action failed: exit status 1" is not
	// worth showing when the script already said "herdr not running".
	detail string
}

var spinFrames = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")

func New(cfg config.Config, client *github.Client) Model {
	return Model{cfg: cfg, client: client, board: board.New(cfg), width: 100, fetching: true,
		fetchGeneration: 1, refreshSeq: 1, detail: map[int]github.Detail{},
		inflight: map[int]detailRequest{}, detailIdentity: map[int]detailRequest{},
		selection: map[int]bool{}, rangeOwned: map[int]bool{}, anchor: -1,
		watched: map[int]*watchEntry{}, boardAt: time.Now()}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(append(m.fetchAll(), spinTick())...)
}
