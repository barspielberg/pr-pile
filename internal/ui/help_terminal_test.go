package ui

import (
	"strings"
	"testing"
	"unicode"

	"github.com/barspielberg/pr-pile/internal/config"
)

func TestHelpSanitizesDynamicConfigText(t *testing.T) {
	bad := "visible\x1b]52;c;owned\a\ntext"
	t.Setenv("PILE_CONFIG", "/tmp/"+bad)
	cfg := testCfg()
	cfg.Actions = []config.Action{{Key: "x", Name: bad, Run: "true"}}
	m := New(cfg, nil)

	for _, line := range m.helpPage() {
		if strings.ContainsFunc(line.text, unicode.IsControl) {
			t.Fatalf("help line contains terminal control text: %q", line.text)
		}
	}
}
