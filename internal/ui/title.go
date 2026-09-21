package ui

import (
	"unicode"

	"github.com/charmbracelet/lipgloss"
)

// titlePart is what a run of title characters is, which decides its colour.
type titlePart int

const (
	partSubject titlePart = iota
	partType
	partScope
	partTicket
)

// commitTypes is the set of conventional-commit type words that parse as a
// type. It is a set, not a colour map: every recognised type now shares one
// hue, so the only thing a lookup answers is whether a leading word is a type
// at all.
var commitTypes = map[string]bool{
	"fix": true, "feat": true, "refactor": true, "perf": true, "revert": true,
	"chore": true, "ci": true, "build": true, "docs": true, "style": true,
	"test": true,
}

// typeStyle is the one colour every recognised type takes.
//
// A themed ANSI slot rather than a 256-cube tint. The eleven types used to
// carry five cube values between them, which put the board's largest block of
// fixed colour on the field that says least: the type is a category, and
// categories do not need telling apart by hue when the word itself is right
// there. Distinguishing feat from fix by colour was decoration on top of a
// word that already read, and it cost the user's theme five slots to do it.
//
// ANSI 4 is the accent the PR number already uses. Sharing the slot is the
// point rather than a collision: both are the row's structural furniture --
// the number identifies it, the type classifies it -- and they sit in
// different columns with different shapes, so position disambiguates them the
// way it does for the reused checkmarks in the status cluster.
//
// Checked on the live board before settling: the number sits at column 14 and
// the type at 29 with the status cluster's own colours between them, so the
// two never abut, and `#3265` against `fix` is a different shape as well as a
// different column. It reads as structure repeating, not as one field bleeding
// into another. ANSI 6 was the fallback if they had read as confusable.
var typeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("4"))

// parseTitle labels each rune of a title with the part it belongs to, or
// returns nil when the title has no conventional prefix at all. Nil rather
// than an all-subject slice so the renderer can take its original path
// byte-for-byte: roughly a quarter of this board's titles are free-form, and
// they must not pick up a single stray SGR pair.
//
// The colon is required. "Fix location widget" opens with a type word but is
// an ordinary sentence, and a parse loose enough to colour it would recolour
// the first word of every prose title on the board. Requiring the colon is
// what separates a convention from a coincidence.
func parseTitle(s string) []titlePart {
	r := []rune(s)
	parts := make([]titlePart, len(r))

	i := 0
	if n := scanTypePrefix(r); n > 0 {
		markTypePrefix(parts, r[:n])
		i = n
	} else if n := scanTicket(r, 0); n > 0 && ticketDelimited(r, n) {
		// A bare Jira key is its own prefix: "AF-12872 Order plan activity
		// logs" carries the same "this is the tag, that is the subject" shape,
		// and muting the key leaves the subject brightest either way.
		fill(parts, 0, n, partTicket)
		i = n
	} else {
		return nil
	}

	// A key that follows the prefix is still a tag, not subject.
	i = skipSpace(r, i)
	if n := scanTicket(r, i); n > i && ticketDelimited(r, n) {
		fill(parts, i, n, partTicket)
	}
	return parts
}

// scanTypePrefix returns the length of a leading `type:` or `type(scope):`,
// including the colon, or 0 if there is none.
func scanTypePrefix(r []rune) int {
	i := 0
	for i < len(r) && isLowerAlpha(r[i]) {
		i++
	}
	if i == 0 {
		return 0
	}
	if !commitTypes[string(r[:i])] {
		return 0
	}
	if i < len(r) && r[i] == '(' {
		j := i + 1
		for j < len(r) && r[j] != ')' && r[j] != ':' {
			j++
		}
		if j == len(r) || r[j] != ')' {
			return 0
		}
		i = j + 1
	}
	if i < len(r) && r[i] == '!' {
		i++
	}
	if i >= len(r) || r[i] != ':' {
		return 0
	}
	return i + 1
}

// markTypePrefix labels an already-validated `type(scope):` span. Only the
// bare type word takes the hue; the parens, the `!` and the colon are
// punctuation and recede with the scope, so "ci:" and "fix(ordering):" put the
// same amount of colour on the same kind of thing.
func markTypePrefix(parts []titlePart, prefix []rune) {
	word := 0
	for word < len(prefix) && isLowerAlpha(prefix[word]) {
		word++
	}
	fill(parts, 0, word, partType)
	fill(parts, word, len(prefix), partScope)
}

// scanTicket returns the end index of a Jira-style KEY-123 starting at i, or i
// if there is none. The trailing separator (":" or "|") is swallowed so it
// mutes with the key rather than hanging off the subject.
func scanTicket(r []rune, i int) int {
	start := i
	for i < len(r) && r[i] >= 'A' && r[i] <= 'Z' {
		i++
	}
	if i == start || i >= len(r) || r[i] != '-' {
		return start
	}
	i++
	digits := i
	for i < len(r) && unicode.IsDigit(r[i]) {
		i++
	}
	if i == digits {
		return start
	}
	// The separator some titles put between key and subject belongs to the
	// tag: left bright it reads as the subject's first character.
	if i < len(r) && (r[i] == ':' || r[i] == '|') {
		i++
	} else if i+1 < len(r) && r[i] == ' ' && r[i+1] == '|' {
		i += 2
	}
	return i
}

// ticketDelimited rejects a key that runs straight into more text, so
// "AF-1foo" is not mistaken for a tag.
func ticketDelimited(r []rune, end int) bool {
	return end >= len(r) || r[end] == ' '
}

func skipSpace(r []rune, i int) int {
	for i < len(r) && r[i] == ' ' {
		i++
	}
	return i
}

func fill(parts []titlePart, from, to int, p titlePart) {
	for i := from; i < to && i < len(parts); i++ {
		parts[i] = p
	}
}

func isLowerAlpha(r rune) bool { return r >= 'a' && r <= 'z' }

// titlePartStyle layers a part's colour onto the row's own style rather than
// replacing it, so a draft row stays faint and a selected row stays bold and
// keeps its background.
func titlePartStyle(st lipgloss.Style, p titlePart, typeWord string) lipgloss.Style {
	switch p {
	case partType:
		if commitTypes[typeWord] {
			return st.Foreground(typeStyle.GetForeground())
		}
	case partScope, partTicket:
		return st.Faint(true)
	}
	return st
}

// leadingType is the type word a parsed title opens with, "" for a bare-ticket
// or unparsed title.
func leadingType(s string, parts []titlePart) string {
	if len(parts) == 0 || parts[0] != partType {
		return ""
	}
	n := 0
	for n < len(parts) && parts[n] == partType {
		n++
	}
	return string([]rune(s)[:n])
}
