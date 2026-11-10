package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/barspielberg/pr-pile/internal/badge"
	"github.com/barspielberg/pr-pile/internal/board"
	"github.com/barspielberg/pr-pile/internal/config"
	"github.com/barspielberg/pr-pile/internal/github"
	"github.com/barspielberg/pr-pile/internal/shelltmpl"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// badges is everything the board needs to run badge scripts. It sits behind a
// pointer so the Model's value receivers share one cache and one runner, and
// is nil when the config has no badges.
type badges struct {
	defs  []config.Badge // with defaults filled in, in config order
	cache *badge.Cache
	run   *badge.Runner
	// timerSeq numbers the expiry timer, so only the newest one dispatches.
	timerSeq uint64
	// Manual refresh replacements must use the refetched PRs, including
	// when an old job finishes before the board request does.
	waitForBoard bool
}

type badgeDoneMsg struct{ result badge.Result }
type badgeTimerMsg struct{ seq uint64 }

// titleFloor is how far a title is cut to make room for its badges before any
// badge is dropped: past this the title stops being recognisable, and the
// title is what the row is about.
const titleFloor = 24

func newBadges(cfg config.Config) *badges {
	if len(cfg.Badges) == 0 {
		return nil
	}
	b := &badges{cache: badge.NewCache(cfg.Workers())}
	for _, def := range cfg.Badges {
		b.defs = append(b.defs, def.WithDefaults())
	}
	b.run = badge.NewRunner()
	return b
}

// Close kills every badge script still running. Each runs in its own session,
// so the terminal closing does not reach them.
func (m Model) Close() {
	if m.badges != nil {
		m.badges.run.Stop()
	}
}

func (b *badges) def(name string) (config.Badge, bool) {
	for _, d := range b.defs {
		if d.Name == name {
			return d, true
		}
	}
	return config.Badge{}, false
}

type badgeWant struct {
	key badge.Key
	pr  github.PR
	def config.Badge
}

// badgeWants is every badge the board shows, in the order they should be
// filled: rule order, then row order, then config order within a row.
func (m Model) badgeWants() []badgeWant {
	var out []badgeWant
	for _, s := range m.sections() {
		for _, r := range s.Rows {
			for _, def := range m.badges.defs {
				if s.Rule.ShowsBadge(def.Name) {
					out = append(out, badgeWant{badge.KeyFor(def.Name, r.PR.Repo, r.PR.Number), r.PR, def})
				}
			}
		}
	}
	return out
}

// dispatchBadges starts whatever is due, up to the worker cap, and arms one
// timer for the next entry to fall due. It is called after anything that can
// free a slot or add a row, so the timer is re-armed every time: a timer that
// fired while every worker was busy would otherwise be the last one.
func (m Model) dispatchBadges(now time.Time) tea.Cmd {
	if m.badges == nil {
		return nil
	}
	if m.badges.waitForBoard {
		if m.board.Loading() {
			return nil
		}
		m.badges.waitForBoard = false
	}
	wants := m.badgeWants()
	keys := make([]badge.Key, len(wants))
	byKey := make(map[badge.Key]badgeWant, len(wants))
	for i, w := range wants {
		keys[i] = w.key
		byKey[w.key] = w
	}
	var cmds []tea.Cmd
	for {
		taken := m.badges.cache.Take(keys, now)
		if len(taken) == 0 {
			break
		}
		failed := false
		for _, k := range taken {
			w := byKey[k]
			job, err := m.badgeJob(w)
			if err != nil {
				// Refused before a process started, such as a branch name
				// that fails its check. It still holds a slot until Done.
				m.badges.cache.Done(badge.Result{Key: k, Err: err}, w.def.TTL, now)
				failed = true
				continue
			}
			run := m.badges.run
			cmds = append(cmds, func() tea.Msg { return badgeDoneMsg{run.Run(job)} })
		}
		if !failed {
			break
		}
	}
	if due, ok := m.badges.cache.NextDue(keys); ok && due.After(now) {
		m.badges.timerSeq++
		seq := m.badges.timerSeq
		cmds = append(cmds, tea.Tick(due.Sub(now), func(time.Time) tea.Msg { return badgeTimerMsg{seq} }))
	}
	return tea.Batch(cmds...)
}

func (m Model) badgeJob(w badgeWant) (badge.Job, error) {
	repo, repoPath, err := m.actionRepo(w.def.Run, w.pr)
	if err != nil {
		return badge.Job{}, err
	}
	line, err := shelltmpl.Render(w.def.Run, false, shelltmpl.NewData(w.pr, repo, repoPath))
	if err != nil {
		return badge.Job{}, err
	}
	dir, err := m.actionDir(w.def.Run, []github.PR{w.pr})
	if err != nil {
		return badge.Job{}, err
	}
	return badge.Job{Key: w.key, Line: line, Dir: dir, Timeout: w.def.Timeout}, nil
}

func (m Model) applyBadge(msg badgeDoneMsg) (Model, tea.Cmd) {
	if m.badges == nil {
		return m, nil
	}
	ttl := time.Minute
	if def, ok := m.badges.def(msg.result.Key.Badge); ok {
		ttl = def.TTL
	}
	now := time.Now()
	m.badges.cache.Done(msg.result, ttl, now)
	return m, m.dispatchBadges(now)
}

type rowBadge struct {
	text  string
	style lipgloss.Style
	right bool
}

// rowBadges is what a row shows, in config order. Only text a script has
// answered with is drawn; a badge not yet fetched is blank, never a
// placeholder, since · already means "none" on the board.
func (m Model) rowBadges(pr github.PR, rule config.Rule) []rowBadge {
	if m.badges == nil {
		return nil
	}
	var out []rowBadge
	for _, def := range m.badges.defs {
		if !rule.ShowsBadge(def.Name) {
			continue
		}
		e, ok := m.badges.cache.Get(badge.KeyFor(def.Name, pr.Repo, pr.Number))
		if !ok || e.Text == "" {
			continue
		}
		out = append(out, rowBadge{text: clipBadge(e.Text, def.MaxWidth), style: badgeStyle(def.Color), right: def.Side == "right"})
	}
	return out
}

func badgeStyle(color string) lipgloss.Style {
	switch color {
	case "fg":
		return fgStyle
	case "ok":
		return okStyle
	case "attention":
		return attentionStyle
	case "error":
		return errorStyle
	case "accent":
		return accentStyle
	}
	return mutedStyle
}

// fitBadges shares the title cell between the title and its badges. Badges sit
// in the slack after the title, so the title never moves. When both do not
// fit, the title is cut down to titleFloor first, then badges drop from the
// last in config order. titleW is the title's share: the whole cell when no
// badge is drawn, so a row without badges draws exactly as it always has.
func fitBadges(title string, tw int, bs []rowBadge) (titleW int, kept []rowBadge) {
	tlen := lipgloss.Width(terminalText(title))
	floor := min(tlen, titleFloor)
	kept = bs
	for len(kept) > 0 {
		need := 0
		for _, b := range kept {
			need += lipgloss.Width(b.text) + 1
		}
		if floor+need <= tw {
			return min(tlen, tw-need), kept
		}
		kept = kept[:len(kept)-1]
	}
	return tw, nil
}

// ruleOf is the rule whose section holds pr. A PR sits under the first rule
// that matches it, so there is one.
func (m Model) ruleOf(pr github.PR) config.Rule {
	for _, s := range m.sections() {
		for _, r := range s.Rows {
			if r.PR.Key() == pr.Key() {
				return s.Rule
			}
		}
	}
	return config.Rule{}
}

// badgeLayout is how a row's title cell is split: the title's share, then the
// left badges, the slack, and the right badges, which together fill the cell.
type badgeLayout struct {
	titleW      int
	left, right []rowBadge
	gap         int
}

// layoutTitleCell is the one place the split is decided, so the searchable
// text and the drawn row cannot disagree about where a badge sits. Badges drop
// in the narrow tier, with age and author: they are extras the user added, not
// the board.
func (m Model) layoutTitleCell(r board.Row, tw int, showAuthor bool) badgeLayout {
	if m.badges == nil || widthTierFor(m.width, showAuthor) == tierNarrow {
		return badgeLayout{titleW: tw}
	}
	titleW, kept := fitBadges(r.PR.Title, tw, m.rowBadges(r.PR, m.ruleOf(r.PR)))
	l := badgeLayout{titleW: titleW, gap: tw - titleW}
	for _, b := range kept {
		l.gap -= lipgloss.Width(b.text) + 1
		if b.right {
			l.right = append(l.right, b)
		} else {
			l.left = append(l.left, b)
		}
	}
	return l
}

func (l badgeLayout) badges() []rowBadge {
	return append(append([]rowBadge(nil), l.left...), l.right...)
}

// badgeLines is the badges block of the detail page: the full text the row
// clips, how old it is, and the error the row cannot show.
func (m Model) badgeLines(pr github.PR) []string {
	if m.badges == nil {
		return nil
	}
	rule := m.ruleOf(pr)
	nameW := 0
	for _, def := range m.badges.defs {
		nameW = max(nameW, lipgloss.Width(def.Name))
	}
	var out []string
	label := "badges"
	for _, def := range m.badges.defs {
		if !rule.ShowsBadge(def.Name) {
			continue
		}
		e, _ := m.badges.cache.Get(badge.KeyFor(def.Name, pr.Repo, pr.Number))
		var parts []string
		switch {
		case e.Text != "":
			parts = append(parts, badgeStyle(def.Color).Render(e.Text)+mutedStyle.Render(" · "+fetchedAgo(e.Fetched)))
		case e.Err == nil && e.Fetched.IsZero():
			parts = append(parts, mutedStyle.Render("not run yet"))
		case e.Err == nil:
			parts = append(parts, mutedStyle.Render("empty"))
		}
		if e.Running {
			parts = append(parts, mutedStyle.Render(string(spinFrames[m.spinner%len(spinFrames)])+" running"))
		}
		if e.Err != nil {
			parts = append(parts, errorStyle.Render("failed: "+terminalText(e.Err.Error())))
		}
		line := "  " + mutedStyle.Render(pad(label, 10)) + pad(terminalText(def.Name), nameW) + "  " + strings.Join(parts, mutedStyle.Render(" · "))
		out = append(out, line)
		label = ""
	}
	return out
}

// badgeHelp lists the configured badges, since a badge has no column header to
// say what it is.
func (m Model) badgeHelp() []helpRow {
	if m.badges == nil {
		return nil
	}
	var rows []helpRow
	for _, def := range m.badges.defs {
		where := "after the title"
		if def.Side == "right" {
			where = "before the author"
		}
		rows = append(rows, glyph(def.Name, where+", refreshed every "+shortDuration(def.TTL), badgeStyle(def.Color)))
	}
	return append(rows, row("r", "reloads the board and reruns every badge"))
}

// fetchedAgo is finer than a row's age cell, because a badge's TTL is minutes.
func fetchedAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	return age(t) + " ago"
}

// clipBadge is clip, except that a badge only a few cells wide is cut bare
// instead of blanked: clip gives up below 4 cells, and a blank badge reads
// as a script that had nothing to say.
func clipBadge(s string, w int) string {
	if w > 3 || lipgloss.Width(s) <= w {
		return clip(s, w)
	}
	var b strings.Builder
	for _, r := range s {
		if lipgloss.Width(b.String()+string(r)) > w {
			break
		}
		b.WriteRune(r)
	}
	return b.String()
}

// shortDuration drops the zero units Duration.String spells out: 5m, not 5m0s.
func shortDuration(d time.Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}
	return s
}

// refreshBadges requests one new run per badge after the board refetch, so
// even a pending replacement uses current PR data. Old answers stay visible.
func (m Model) refreshBadges() {
	if m.badges == nil {
		return
	}
	wants := m.badgeWants()
	keys := make([]badge.Key, len(wants))
	for i, w := range wants {
		keys[i] = w.key
	}
	m.badges.cache.Expire(keys)
	m.badges.waitForBoard = true
	m.badges.timerSeq++
}

// badgeFailure is the footer's word on a badge that has failed on every row it
// ran on. One failing row is the script's business and shows in `d`; failing
// everywhere usually means the script itself is broken, and a blank column
// would otherwise look like it had nothing to say.
func (m Model) badgeFailure() string {
	if m.badges == nil {
		return ""
	}
	type tally struct {
		failed, answered int
		err              error
	}
	tallies := map[string]*tally{}
	for _, w := range m.badgeWants() {
		e, ok := m.badges.cache.Get(w.key)
		if !ok {
			continue
		}
		t := tallies[w.def.Name]
		if t == nil {
			t = &tally{}
			tallies[w.def.Name] = t
		}
		switch {
		case e.Err != nil:
			t.failed++
			t.err = e.Err
		case !e.Fetched.IsZero():
			t.answered++
		}
	}
	var names []string
	var first error
	for _, def := range m.badges.defs {
		if t := tallies[def.Name]; t != nil && t.failed > 0 && t.answered == 0 {
			names = append(names, terminalText(def.Name))
			if first == nil {
				first = t.err
			}
		}
	}
	if len(names) == 0 {
		return ""
	}
	return "badge " + strings.Join(names, ", ") + " failing: " + terminalText(first.Error())
}
