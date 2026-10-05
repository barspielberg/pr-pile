package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/barspielberg/pr-pile/internal/github"
)

// stateLines is the state block: what is true about the PR rather than about
// its CI. Order is the drop order reversed -- the last line here is the first
// to be clipped -- so the block degrades by truncation alone.
//
// A line with nothing to say is not drawn. On an all-green, freshly-opened PR
// this is three lines; docs/pr-detail.md §8.3 is the case it is tuned for.
func (m Model) stateLines(pr github.PR) []string {
	d, loaded := m.detail[pr.Key()]
	body := max(0, m.width-4)
	var out []string

	label := func(k, v string) string {
		return "  " + mutedStyle.Render(pad(k, 10)) + clip(v, max(0, body-12))
	}

	// Conflicted and how far behind are one line: `!` already says there is a
	// problem, and behindBy says how big it is. Either half can be absent.
	if s := m.mergeLine(pr, d, loaded); s != "" {
		out = append(out, s)
	}
	if loaded && d.Unresolved > 0 {
		out = append(out, "  "+attentionStyle.Render("●")+" "+
			mutedStyle.Render(plural(d.Unresolved, "unresolved comment")))
	}
	if s := m.pendingLine(pr); s != "" {
		out = append(out, s)
	}
	// Only with several repos: with one it would be a constant.
	if m.multiRepo() {
		out = append(out, label("repo", terminalText(pr.Repo)))
	}
	if pr.Author != "" {
		out = append(out, label("author", person(pr.AuthorName, pr.Author)))
	}
	if s := m.reviewerLine(pr, d, loaded); s != "" {
		out = append(out, s)
	}
	if pr.ChangedFiles > 0 {
		out = append(out, label("size", fmt.Sprintf("+%d −%d · %s",
			pr.Additions, pr.Deletions, plural(pr.ChangedFiles, "file"))))
	}
	// Only when it is not the default branch: on a stacked PR this is the only
	// place the tool says what the stack is built on, and on the ~90% that
	// target the default it would be a constant. Until the request lands the
	// default is unknown, so this waits rather than guessing "master".
	if base := pr.BaseRefName; base != "" && loaded && base != d.DefaultBranch {
		out = append(out, label("base", base))
	}
	if pr.HeadRefName != "" {
		out = append(out, label("branch", pr.HeadRefName))
	}
	if s := ageLine(pr); s != "" {
		out = append(out, label("opened", s))
	}
	return out
}

// pendingLine is the page's loading state: one skeleton line standing where the
// on-demand lines will land, so the common case resolves in place instead of
// pushing the rest of the block down. docs/pr-detail.md §7 chose to draw
// nothing and let the lines appear, on the grounds that a line which appears
// disturbs less than one that changes. The user reported not seeing a loading
// state at all, which is the evidence against that: the appearing line reflows
// everything below it, and there was nothing on screen to say why.
//
// It is one line and not one per field. `behind` and `unresolved` are each
// conditional -- unresolved threads exist on 6 of 50 PRs (§5.4) -- so a
// placeholder per field would draw two rows on most PRs that then vanish,
// trading the reflow on arrival for a worse one on resolution. One line is what
// the group usually resolves to, so usually nothing moves at all.
func (m Model) pendingLine(pr github.PR) string {
	if _, loaded := m.detail[pr.Key()]; loaded {
		return ""
	}
	if _, failed := m.detailFailed[pr.Key()]; failed {
		return ""
	}
	// Nothing is coming when there is no client to ask, so a board rendered
	// offline or in a test must not sit on a loader forever.
	if m.client == nil {
		return ""
	}
	frame := string(spinFrames[m.spinner%len(spinFrames)])
	return "  " + mutedStyle.Render(frame+" checking for conflicts and open conversations")
}

// mergeLine states the merge problem and its size together. `26 commits behind`
// is the honest answer to "which files conflict": GitHub exposes no per-file
// conflict list, and the only computable proxy over-reports by 3x in the median
// case and 8x at worst against a real merge, so it fabricates rather than
// merely omits. See docs/pr-detail.md §4.
func (m Model) mergeLine(pr github.PR, d github.Detail, loaded bool) string {
	behind := ""
	if loaded && d.BehindBy > 0 {
		// Named against the PR's own base, not the repo default: a stacked PR
		// is behind the branch it targets, and saying "behind master" there
		// would be a different and wrong number.
		base := pr.BaseRefName
		if base == "" {
			base = d.DefaultBranch
		}
		base = terminalText(base)
		behind = fmt.Sprintf("%s behind %s", plural(d.BehindBy, "commit"), base)
	}
	switch {
	case pr.Mergeable == "CONFLICTING" && behind != "":
		return "  " + errorStyle.Render("!") + " " + mutedStyle.Render("conflicted · "+behind)
	case pr.Mergeable == "CONFLICTING":
		return "  " + errorStyle.Render("!") + " " + mutedStyle.Render("conflicted")
	case behind != "":
		return "  " + mutedStyle.Render("· "+behind)
	}
	return ""
}

// reviewerLine names who formed an opinion. The row's glyph says an opinion
// exists; the name says who to go talk to about it.
//
// It is the only line that holds its place with a placeholder rather than
// appearing when it arrives. The rest are absent until they land because a line
// that appears disturbs less than one that changes -- but this one was asked
// for specifically, and a row that silently gains a name a second later reads
// as the page having been wrong rather than incomplete.
func (m Model) reviewerLine(pr github.PR, d github.Detail, loaded bool) string {
	head := "  " + mutedStyle.Render(pad("review", 10))
	if !loaded {
		// An unreviewed PR has no name coming, so promising one would be a
		// placeholder for nothing.
		if pr.Review == "" || pr.Review == "REVIEW_REQUIRED" {
			return ""
		}
		if _, failed := m.detailFailed[pr.Key()]; failed {
			return ""
		}
		return head + mutedStyle.Render("…")
	}
	if len(d.Reviewers) == 0 {
		return ""
	}
	var parts []string
	for _, r := range d.Reviewers {
		glyph := okStyle.Render("✓")
		if r.State == "CHANGES_REQUESTED" {
			glyph = errorStyle.Render("✗")
		}
		parts = append(parts, glyph+" "+person(r.Name, r.Login))
	}
	return head + clip(strings.Join(parts, " · "), max(0, m.width-14))
}

// person renders a human the same way everywhere: the display name is null for
// 36% of this board's authors and for some reviewers too, so the login is
// always carried -- it is also the string you would @-mention or search for.
func person(name, login string) string {
	name, login = terminalText(name), terminalText(login)
	if name == "" {
		return login
	}
	return fmt.Sprintf("%s (%s)", name, login)
}

// ageLine says how long this has been open, and how long it has sat. The row's
// age column is updatedAt, which is a different question from either. Nothing
// is said about a PR opened today, since the row already covers that, and idle
// is only worth naming once it has outlasted a working day.
func ageLine(pr github.PR) string {
	if pr.CreatedAt.IsZero() || time.Since(pr.CreatedAt) < 24*time.Hour {
		return ""
	}
	s := fmt.Sprintf("%s ago", age(pr.CreatedAt))
	if !pr.UpdatedAt.IsZero() && time.Since(pr.UpdatedAt) > 48*time.Hour {
		s += " · idle " + age(pr.UpdatedAt)
	}
	return s
}
