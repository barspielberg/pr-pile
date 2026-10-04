// Package board turns per-rule search results into the ordered sections the
// UI draws.
package board

import (
	"sort"
	"time"

	"github.com/barspielberg/pr-pile/internal/config"
	"github.com/barspielberg/pr-pile/internal/github"
)

type State int

const (
	Pending State = iota // request in flight, or waiting on a rule above it
	Ready
	Failed
)

type Section struct {
	Rule  config.Rule
	State State
	Err   error
	Rows  []Row
	// Stale marks rows carried over from the previous fetch. They stay on
	// screen so the board does not collapse and re-expand while refreshing.
	Stale bool
}

// Row is a PR plus its place in a stack. Depth and tree glyphs are display
// concerns the UI reads off here rather than recomputing.
type Row struct {
	PR     github.PR
	Prefix string
	Last   bool
}

// Result is one rule's fetch outcome, delivered as it arrives.
type Result struct {
	Index int
	PRs   []github.PR
	Err   error
}

// Board holds every rule's result and decides what is safe to show.
type Board struct {
	cfg      config.Config
	results  []*Result
	sections []Section
	// hidden is PRs the watch saw merge or close, and when. The search index
	// can lag a merge, so a fetch soon after must not bring one back; the
	// hold expires so a PR closed by mistake and reopened does come back.
	hidden map[int]time.Time
}

// hideFor outlasts the search index's lag behind a merge, which is seconds to
// a minute, by a wide margin.
const hideFor = 10 * time.Minute

// now is a var so a test can move the clock past hideFor.
var now = time.Now

func New(cfg config.Config) *Board {
	b := &Board{cfg: cfg, results: make([]*Result, len(cfg.Rules)), hidden: map[int]time.Time{}}
	b.sections = make([]Section, len(cfg.Rules))
	for i, r := range cfg.Rules {
		b.sections[i] = Section{Rule: r, State: Pending}
	}
	return b
}

// Refetch clears the results but keeps the rendered rows in place, marked
// stale. Rebuilding from empty would collapse the board and then re-expand it
// section by section, jumping every row below each one that resolves.
func (b *Board) Refetch() {
	for i := range b.results {
		b.results[i] = nil
	}
	for i := range b.sections {
		s := &b.sections[i]
		s.State, s.Err = Pending, nil
		s.Stale = len(s.Rows) > 0
	}
}

// Hide takes a PR off the board now, without waiting for the next fetch to
// stop returning it.
func (b *Board) Hide(number int) {
	b.hidden[number] = now()
	b.rebuild()
	// A section still waiting on its fetch keeps last time's rows, which
	// rebuild leaves alone, so the PR comes out of those here.
	for i := range b.sections {
		s := &b.sections[i]
		rows := s.Rows[:0:0]
		for _, r := range s.Rows {
			if r.PR.Number != number {
				rows = append(rows, r)
			}
		}
		s.Rows = rows
	}
}

func (b *Board) isHidden(number int) bool {
	at, ok := b.hidden[number]
	if ok && now().Sub(at) >= hideFor {
		delete(b.hidden, number)
		return false
	}
	return ok
}

func (b *Board) Apply(res Result) {
	if res.Index < 0 || res.Index >= len(b.results) {
		return
	}
	b.results[res.Index] = &res
	b.rebuild()
}

// Frontier is how far down the board is drawable: the first rule that has not
// resolved yet. Everything below it is still Pending even if its own request
// has already come back.
func (b *Board) Frontier() int {
	for i, r := range b.results {
		if r == nil {
			return i
		}
	}
	return len(b.results)
}

func (b *Board) Sections() []Section { return b.sections }

func (b *Board) Loading() bool { return b.Frontier() < len(b.results) }

// rebuild recomputes the visible prefix. A PR belongs to the first rule that
// matched it, so a section's contents are only knowable once every section
// above it has resolved -- otherwise a row could be shown here and then
// claimed by an earlier rule a moment later.
func (b *Board) rebuild() {
	claimed := map[int]bool{}
	frontier := b.Frontier()

	for i := range b.sections {
		s := &b.sections[i]
		if i >= frontier {
			// Keep whatever is already drawn: blanking here is what makes the
			// board jump as later sections resolve.
			s.State, s.Err = Pending, nil
			s.Stale = len(s.Rows) > 0
			continue
		}
		res := b.results[i]
		if res.Err != nil {
			// A failed rule cannot claim anything, so later rules may show PRs
			// this one would have taken. Better than stalling the whole board.
			s.State, s.Err, s.Rows, s.Stale = Failed, res.Err, nil, false
			continue
		}
		var own []github.PR
		for _, pr := range res.PRs {
			if claimed[pr.Number] || b.isHidden(pr.Number) {
				continue
			}
			claimed[pr.Number] = true
			own = append(own, pr)
		}
		s.State, s.Err, s.Stale = Ready, nil, false
		s.Rows = layout(own, s.Rule.Tree)
	}
}

// layout orders PRs newest-first and, when the rule asks for it, groups
// stacked PRs so a chain reads bottom-up as one unit.
func layout(prs []github.PR, tree bool) []Row {
	sort.SliceStable(prs, func(i, j int) bool {
		return prs[i].UpdatedAt.After(prs[j].UpdatedAt)
	})
	if !tree {
		rows := make([]Row, 0, len(prs))
		for _, pr := range prs {
			rows = append(rows, Row{PR: pr})
		}
		return rows
	}
	return stacks(prs)
}

// stacks finds chains where one PR targets another's head branch and emits
// each chain contiguously, root first.
func stacks(prs []github.PR) []Row {
	byBase := map[string][]github.PR{}
	heads := map[string]bool{}
	for _, pr := range prs {
		// An empty ref would make every PR look stacked on every other, since
		// an unset base would "match" an unset head.
		if pr.HeadRefName != "" {
			heads[pr.HeadRefName] = true
		}
	}
	stacked := func(pr github.PR) bool {
		return pr.BaseRefName != "" && heads[pr.BaseRefName]
	}
	for _, pr := range prs {
		if stacked(pr) {
			byBase[pr.BaseRefName] = append(byBase[pr.BaseRefName], pr)
		}
	}

	var rows []Row
	for _, pr := range prs {
		// Roots only: a PR stacked on another is emitted by its parent's chain.
		if stacked(pr) {
			continue
		}
		chain := walk(pr, byBase, map[int]bool{})
		if len(chain) == 1 {
			rows = append(rows, Row{PR: chain[0]})
			continue
		}
		for i, p := range chain {
			rows = append(rows, Row{PR: p, Prefix: glyph(i, len(chain)), Last: i == len(chain)-1})
		}
	}
	return rows
}

// seen guards against a base/head cycle, which would otherwise recurse forever.
func walk(pr github.PR, byBase map[string][]github.PR, seen map[int]bool) []github.PR {
	if seen[pr.Number] {
		return nil
	}
	seen[pr.Number] = true
	out := []github.PR{pr}
	for _, child := range byBase[pr.HeadRefName] {
		out = append(out, walk(child, byBase, seen)...)
	}
	return out
}

func glyph(i, n int) string {
	switch i {
	case 0:
		return "╭╴"
	case n - 1:
		return "╰╴"
	default:
		return "│ "
	}
}
