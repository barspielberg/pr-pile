package ui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/barspielberg/pr-pile/internal/github"
)

// isConfiguredRepo ignores case the way GitHub does, so a config that spells
// the repo differently from GitHub does not tag every row.
func (m Model) isConfiguredRepo(repo string) bool { return strings.EqualFold(repo, m.cfg.Repo) }

// repoTag names a PR's repo on its row, and is empty for the configured repo:
// that one is the board's default, so tagging it would put the same word on
// nearly every row. The owner is dropped when it matches the configured one,
// since the name alone is what tells two of an org's repos apart.
func (m Model) repoTag(pr github.PR) string {
	repo := pr.Repo
	if m.isConfiguredRepo(repo) {
		return ""
	}
	owner, name, _ := strings.Cut(repo, "/")
	if base, _, _ := strings.Cut(m.cfg.Repo, "/"); strings.EqualFold(owner, base) {
		return terminalText(name)
	}
	return terminalText(repo)
}

// titleText is the title cell's text before clipping: the repo tag, when the
// PR has one, then the title. The tag sits inside the title cell rather than in
// a column of its own so the board's fixed columns stay where they are, and so
// a search for the repo's name finds and highlights it like any other text.
// tagLen is how many leading runes the tag and its separating space take.
func (m Model) titleText(pr github.PR) (text string, tagLen int) {
	tag := m.repoTag(pr)
	if tag == "" {
		return pr.Title, 0
	}
	return tag + " " + pr.Title, utf8.RuneCountInString(tag) + 1
}

// titleParts is parseTitle for a title drawn after a repo tag: the tag is a
// part of its own, and the commit convention is read from what follows it.
func titleParts(text string, tagLen int) []titlePart {
	if tagLen == 0 {
		return parseTitle(text)
	}
	r := []rune(text)
	n := min(tagLen, len(r))
	parts := make([]titlePart, n, len(r))
	fill(parts, 0, n, partRepo)
	rest := parseTitle(string(r[n:]))
	if rest == nil {
		rest = make([]titlePart, len(r)-n)
	}
	return append(parts, rest...)
}

// prRef names a PR in prose: #12 in the configured repo, repo#12 elsewhere.
func (m Model) prRef(pr github.PR) string {
	return fmt.Sprintf("%s#%d", m.repoTag(pr), pr.Number)
}
