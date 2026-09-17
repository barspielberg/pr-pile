package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/barspielberg/prs-mng/internal/board"
	"github.com/barspielberg/prs-mng/internal/github"
	"github.com/sahilm/fuzzy"
)

// haystack is what a row is matched against: number, author and title, so one
// query spans all three. The author sits before the title so its characters
// cannot be mistaken for title positions when translating match indexes back.
//
// The author is login and display name together, so "Carol" finds a PR
// authored by `cdiaz88`. The name is how you think of a person, the login
// is what GitHub calls them, and the row shows only three initials of either.
func haystack(r board.Row) string {
	return fmt.Sprintf("#%d %s %s", r.PR.Number, searchableAuthor(r.PR), r.PR.Title)
}

// titleOffset is where the title starts inside haystack, used to translate
// match positions back into title indexes for highlighting.
func titleOffset(r board.Row) int {
	return len(fmt.Sprintf("#%d %s ", r.PR.Number, searchableAuthor(r.PR)))
}

// searchableAuthor is a person in both the forms you might type. Null for 36%
// of this board's authors, which is why the login is never dropped.
func searchableAuthor(pr github.PR) string {
	if pr.AuthorName == "" {
		return pr.Author
	}
	return pr.Author + " " + pr.AuthorName
}

type rowSource []board.Row

func (s rowSource) String(i int) string { return haystack(s[i]) }
func (s rowSource) Len() int            { return len(s) }

// filterSection keeps the rows of one section that match the query, best match
// first. Fuzzy matching is permissive enough that a weak subsequence hit is
// common, so score order is what makes the result usable -- unranked, the PR
// the user meant can sit below noise.
//
// Rows that were part of a stack lose their tree glyphs: the chain is almost
// never contiguous once filtered, and a dangling "╰╴" would draw a spine to a
// row that is no longer above it.
func filterSection(rows []board.Row, query string) []board.Row {
	if query == "" {
		return rows
	}
	if digits, ok := numericQuery(query); ok {
		return matchNumber(rows, digits)
	}
	matches := fuzzy.FindFrom(query, rowSource(rows))
	out := make([]board.Row, 0, len(matches))
	for _, match := range matches {
		r := rows[match.Index]
		r.Prefix, r.Last = "", false
		out = append(out, r)
	}
	return out
}

// numericQuery reports whether the query is a bare PR number. Fuzzy matching
// treats digits as a subsequence, so "3248" also hits a title containing
// 3...2...4...8 scattered across it -- noise when the user is clearly after one
// PR. A leading "#" is accepted and ignored.
func numericQuery(query string) (string, bool) {
	digits := strings.TrimPrefix(strings.TrimSpace(query), "#")
	if digits == "" {
		return "", false
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			return "", false
		}
	}
	return digits, true
}

// matchNumber keeps PRs whose number starts with the digits typed, so a partial
// number narrows as it is typed and a complete one lands on a single row.
func matchNumber(rows []board.Row, digits string) []board.Row {
	var out []board.Row
	for _, r := range rows {
		if strings.HasPrefix(strconv.Itoa(r.PR.Number), digits) {
			r.Prefix, r.Last = "", false
			out = append(out, r)
		}
	}
	return out
}

// matchedTitleIndexes returns the positions inside the PR title that the query
// hit, so the renderer can highlight them. Positions that landed on the number
// are dropped: the number column is styled as a unit and splitting it would
// break the fixed-width cluster.
func matchedTitleIndexes(r board.Row, query string) map[int]bool {
	if query == "" {
		return nil
	}
	// A numeric query matches the number column, which is styled as a unit, so
	// there is nothing in the title to mark.
	if _, ok := numericQuery(query); ok {
		return nil
	}
	matches := fuzzy.FindFrom(query, rowSource([]board.Row{r}))
	if len(matches) == 0 {
		return nil
	}
	off := titleOffset(r)
	out := map[int]bool{}
	for _, i := range matches[0].MatchedIndexes {
		if i >= off {
			out[i-off] = true
		}
	}
	return out
}
