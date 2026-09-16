package ui

import (
	"fmt"

	"github.com/barspielberg/prs-mng/internal/board"
	"github.com/sahilm/fuzzy"
)

// haystack is what a row is matched against: the number carries the "#" so a
// query of "#32" works, and the title follows so one query spans both fields.
func haystack(r board.Row) string {
	return fmt.Sprintf("#%d %s", r.PR.Number, r.PR.Title)
}

// titleOffset is where the title starts inside haystack, used to translate
// match positions back into title indexes for highlighting.
func titleOffset(r board.Row) int {
	return len(fmt.Sprintf("#%d ", r.PR.Number))
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
	matches := fuzzy.FindFrom(query, rowSource(rows))
	out := make([]board.Row, 0, len(matches))
	for _, match := range matches {
		r := rows[match.Index]
		r.Prefix, r.Last = "", false
		out = append(out, r)
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
