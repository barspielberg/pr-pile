package badge

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

// Clean turns a script's stdout into the text a row can draw. The output is
// as untrusted as the PR it describes, so escape sequences, control
// characters, and the bidi and zero-width characters that can make a badge
// read as other text all go. A badge is one line: anything after the first
// newline is dropped.
func Clean(out []byte) string {
	line, _, _ := strings.Cut(string(out), "\n")
	line = strings.ToValidUTF8(line, "�")
	line = ansi.Strip(line)
	line = strings.Map(func(r rune) rune {
		switch {
		case r == '\t':
			return ' '
		case unicode.IsControl(r), invisible(r):
			return -1
		}
		return r
	}, line)
	return strings.TrimSpace(line)
}

// invisible covers the bidi controls, which reorder what follows them, and the
// zero-width characters, which some terminals draw with a width and some do
// not, so a row's columns would disagree.
func invisible(r rune) bool {
	switch {
	case r >= 0x200B && r <= 0x200F, // zero-width space, joiners, LRM, RLM
		r >= 0x202A && r <= 0x202E, // embeddings and overrides
		r >= 0x2060 && r <= 0x2064, // word joiner and invisible operators
		r >= 0x2066 && r <= 0x2069, // isolates
		r == 0x061C, r == 0xFEFF:
		return true
	}
	return false
}
