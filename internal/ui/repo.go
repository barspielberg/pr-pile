package ui

import (
	"fmt"
	"strings"

	"github.com/barspielberg/pr-pile/internal/config"
	"github.com/barspielberg/pr-pile/internal/github"
)

// repoOf finds the declared repo a PR came from, ignoring case the way GitHub
// does. Every search is scoped to declared repos, so a miss is not expected.
func (m Model) repoOf(pr github.PR) (config.Repo, bool) {
	for _, r := range m.cfg.Repos {
		if strings.EqualFold(r.Name, pr.Repo) {
			return r, true
		}
	}
	return config.Repo{}, false
}

// multiRepo is when rows name their repo. With one repo every row would carry
// the same word, so the column and the label#N form are left out.
func (m Model) multiRepo() bool { return len(m.cfg.Repos) > 1 }

func (m Model) repoTag(pr github.PR) string {
	if r, ok := m.repoOf(pr); ok {
		return r.Tag()
	}
	return terminalText(pr.Repo)
}

// repoWidth is the repo column, sized to the longest label so the columns
// after it line up, and 0 when the column is not drawn.
func (m Model) repoWidth() int {
	if !m.multiRepo() {
		return 0
	}
	w := 0
	for _, r := range m.cfg.Repos {
		w = max(w, len([]rune(r.Tag())))
	}
	return min(w, maxRepoWidth)
}

// maxRepoWidth keeps a long label from eating the title. Labels are short by
// intent; one that runs past this is clipped like any other cell.
const maxRepoWidth = 12

// prRef names a PR in prose: label#12 on a board with several repos, #12 on
// one with a single repo.
func (m Model) prRef(pr github.PR) string {
	if !m.multiRepo() {
		return fmt.Sprintf("#%d", pr.Number)
	}
	return fmt.Sprintf("%s#%d", m.repoTag(pr), pr.Number)
}

// reposText is what the board covers, for the footer: the repo's full name when
// there is one, the labels when there are several, since the rows use those.
func (m Model) reposText() string {
	if !m.multiRepo() {
		return m.cfg.Repos[0].Name
	}
	tags := make([]string, len(m.cfg.Repos))
	for i, r := range m.cfg.Repos {
		tags[i] = r.Tag()
	}
	return strings.Join(tags, ", ")
}
