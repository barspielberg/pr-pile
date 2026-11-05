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
	hidden map[github.Key]time.Time
}

// hideFor outlasts the search index's lag behind a merge, which is seconds to
// a minute, by a wide margin.
const hideFor = 10 * time.Minute

// now is a var so a test can move the clock past hideFor.
var now = time.Now

func New(cfg config.Config) *Board {
	b := &Board{cfg: cfg, results: make([]*Result, len(cfg.Rules)), hidden: map[github.Key]time.Time{}}
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
func (b *Board) Hide(key github.Key) {
	b.hidden[key] = now()
	b.rebuild()
	// A section still waiting on its fetch keeps last time's rows, which
	// rebuild leaves alone, so the PR comes out of those here.
	for i := range b.sections {
		s := &b.sections[i]
		rows := s.Rows[:0:0]
		for _, r := range s.Rows {
			if r.PR.Key() != key {
				rows = append(rows, r)
			}
		}
		s.Rows = rows
	}
}

func (b *Board) isHidden(key github.Key) bool {
	at, ok := b.hidden[key]
	if ok && now().Sub(at) >= hideFor {
		delete(b.hidden, key)
		return false
	}
	return ok
}

func (b *Board) Apply(res Result) {
	if res.Index < 0 || res.Index >= len(b.results) {
		return
	}
	res.PRs = b.keepKnownMergeState(res.PRs)
	b.results[res.Index] = &res
	b.rebuild()
}

// keepKnownMergeState carries the last real answer through UNKNOWN. GitHub
// computes both fields lazily and a refresh often lands mid-recompute, which
// would blank a row's ! or ↓ for one refresh and bring it back the next.
func (b *Board) keepKnownMergeState(prs []github.PR) []github.PR {
	shown := map[github.Key]github.PR{}
	for _, s := range b.sections {
		for _, r := range s.Rows {
			shown[r.PR.Key()] = r.PR
		}
	}
	out := make([]github.PR, len(prs))
	for i, pr := range prs {
		if was, ok := shown[pr.Key()]; ok {
			if pr.Mergeable == "UNKNOWN" {
				pr.Mergeable = was.Mergeable
			}
			if pr.MergeState == "UNKNOWN" {
				pr.MergeState = was.MergeState
			}
		}
		out[i] = pr
	}
	return out
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
	claimed := map[github.Key]bool{}
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
			if claimed[pr.Key()] || b.isHidden(pr.Key()) {
				continue
			}
			claimed[pr.Key()] = true
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

// branch is a ref within its repo: two repos can both have a fix/login, and
// one PR must not stack on the other's.
type branch struct{ repo, name string }

// stack is a GitHub native stack within one repo.
type stack struct {
	repo   string
	number int
}

// stacks emits each chain contiguously, bottom first, where its bottom PR
// would sort. A native stack is kept together and ordered by position; then
// one PR, or one native stack, chains onto another whose head branch it
// targets, so a plain PR on a native stack still draws with it.
func stacks(prs []github.PR) []Row {
	order := map[github.Key]int{}
	for i, pr := range prs {
		order[pr.Key()] = i
	}
	var units [][]github.PR
	at := map[stack]int{}
	for _, pr := range prs {
		if pr.Stack.Number == 0 {
			units = append(units, []github.PR{pr})
			continue
		}
		s := stack{pr.Repo, pr.Stack.Number}
		if i, ok := at[s]; ok {
			units[i] = append(units[i], pr)
			continue
		}
		at[s] = len(units)
		units = append(units, []github.PR{pr})
	}
	for _, u := range units {
		sort.SliceStable(u, func(i, j int) bool { return u[i].Stack.Position < u[j].Stack.Position })
	}
	sort.SliceStable(units, func(i, j int) bool { return order[units[i][0].Key()] < order[units[j][0].Key()] })

	owner := map[branch]int{}
	for i, u := range units {
		for _, pr := range u {
			// An empty ref would make every PR look stacked on every other,
			// since an unset base would "match" an unset head.
			if pr.HeadRefName != "" {
				owner[branch{pr.Repo, pr.HeadRefName}] = i
			}
		}
	}
	parent := func(i int) bool {
		bottom := units[i][0]
		p, ok := owner[branch{bottom.Repo, bottom.BaseRefName}]
		return bottom.BaseRefName != "" && ok && p != i
	}
	kids := map[int][]int{}
	for i, u := range units {
		if parent(i) {
			p := owner[branch{u[0].Repo, u[0].BaseRefName}]
			kids[p] = append(kids[p], i)
		}
	}

	var rows []Row
	for i := range units {
		// Roots only: a unit stacked on another is emitted by its parent's chain.
		if parent(i) {
			continue
		}
		rows = append(rows, chainRows(walk(i, units, kids, map[int]bool{}))...)
	}
	return rows
}

// seen guards against a base/head cycle, which would otherwise recurse forever.
func walk(i int, units [][]github.PR, kids map[int][]int, seen map[int]bool) []github.PR {
	if seen[i] {
		return nil
	}
	seen[i] = true
	out := append([]github.PR(nil), units[i]...)
	for _, k := range kids[i] {
		out = append(out, walk(k, units, kids, seen)...)
	}
	return out
}

// chainRows draws one chain. A native stack knows what is missing, unlike a
// branch chain: a ╎ marks each end or gap where it goes on outside this
// section, whether another section claimed that PR or the limit cut it.
// Merged PRs keep their positions, so "below" is read off the base branch
// rather than off position 1.
func chainRows(chain []github.PR) []Row {
	n := len(chain)
	heads := map[string]bool{}
	for _, pr := range chain {
		heads[pr.HeadRefName] = true
	}
	cut := func(pr github.PR) bool {
		return pr.Stack.Number != 0 && pr.BaseRefName != pr.Stack.Base && !heads[pr.BaseRefName]
	}
	end := chain[n-1]
	top := end.Stack.Number != 0 && end.Stack.Position < end.Stack.Size
	rows := make([]Row, n)
	for i, pr := range chain {
		first, last := i == 0, i == n-1
		g := glyph(i, n)
		switch {
		case (first && cut(pr)) || (last && (cut(pr) || top)):
			g = "╎╴"
		case n == 1:
			g = ""
		case !first && !last && cut(pr):
			g = "╎ "
		}
		rows[i] = Row{PR: pr, Prefix: g, Last: last}
	}
	return rows
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
