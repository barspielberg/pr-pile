package ui

import (
	"strings"
	"testing"
	"unicode"

	"github.com/barspielberg/pr-pile/internal/board"
	"github.com/barspielberg/pr-pile/internal/github"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Stripping the ESC rune alone left the rest of the sequence as printable
// text, so remote titles drew a literal "[31m" and paid column width for it.
func TestTerminalTextRemovesWholeEscapeSequences(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"sgr", "Fix \x1b[31mthe\x1b[0m parser", "Fix the parser"},
		{"osc title", "before\x1b]0;pwned\x07after", "beforeafter"},
		{"truncated csi", "tail\x1b[31", "tail"},
		{"plain text untouched", "Fix the parser", "Fix the parser"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := terminalText(tc.in); got != tc.want {
				t.Fatalf("terminalText(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestTerminalTextReplacesControlsAndPreservesUnicode(t *testing.T) {
	input := "שלום\x00\a\n\r\t\x1b[31mred\x1b]0;owned\a\u0085世界"
	got := terminalText(input)
	if strings.ContainsFunc(got, unicode.IsControl) {
		t.Fatalf("sanitized text still contains controls: %q", got)
	}
	for _, want := range []string{"שלום", "red", "世界", "�"} {
		if !strings.Contains(got, want) {
			t.Errorf("sanitized text lost %q: %q", want, got)
		}
	}
}

func TestRemoteTextCannotEmitTerminalControls(t *testing.T) {
	lipgloss.SetColorProfile(termenv.Ascii)
	bad := "visible\x1b[31m\a\r\n\u0085text"
	m := New(testCfg(), nil)
	m.width, m.height = 120, 30
	m.board.Apply(board.Result{Index: 0, PRs: []github.PR{{
		Number:       7,
		Title:        bad,
		Author:       bad,
		AuthorName:   bad,
		HeadRefName:  bad,
		BaseRefName:  bad,
		FailedGates:  []string{bad},
		PendingGates: []string{bad},
	}}})
	m.board.Apply(board.Result{Index: 1})
	m.cursor = m.firstRowSlot()
	m.detail[prKey(7)] = github.Detail{
		Number:        7,
		DefaultBranch: "main",
		Reviewers:     []github.Reviewer{{Name: bad, Login: bad}},
	}
	m.status = bad

	for name, output := range map[string]string{
		"board":  m.View(),
		"detail": m.detailOverlay(),
		"footer": m.footer(""),
	} {
		if strings.ContainsFunc(output, func(r rune) bool { return r != '\n' && unicode.IsControl(r) }) {
			t.Errorf("%s emitted a control rune: %q", name, output)
		}
	}
}
