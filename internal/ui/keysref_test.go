package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestKeysReferencePageDocumentsEveryBinding guards DOCS-PLAN.md §6.2: every
// binding in the help page's KEYS table must appear as a `code` span on the
// keys reference page. A key shipped in the UI but left off the docs fails
// here. The bindings are read from helpBlocks, the same table the user sees, so
// the test cannot fall out of step with help.go.
func TestKeysReferencePageDocumentsEveryBinding(t *testing.T) {
	var bindings []string
	for _, blk := range (Model{}).helpBlocks() {
		if blk.title != "KEYS" {
			continue
		}
		for _, r := range blk.rows {
			bindings = append(bindings, keyBindings(r.key)...)
		}
	}
	if len(bindings) == 0 {
		t.Fatal("no KEYS bindings found")
	}

	body, err := os.ReadFile(filepath.Join("..", "..", "docs", "src", "content", "docs", "reference", "keys.md"))
	if err != nil {
		t.Fatalf("read keys.md: %v", err)
	}
	content := string(body)
	for _, b := range bindings {
		if !strings.Contains(content, "`"+b+"`") {
			t.Errorf("binding %q is not documented on the keys page", b)
		}
	}
}

// keyBindings splits a help-row key cell such as "j / k" or "ctrl+d/u" into its
// individual bindings. The search key "/" is its own binding, not a separator.
func keyBindings(key string) []string {
	if key == "/" {
		return []string{"/"}
	}
	// "ctrl+d/u" is compact for ctrl+d and ctrl+u: the shared "ctrl+" prefix is
	// written once. Re-expand it before the generic split.
	if key == "ctrl+d/u" {
		return []string{"ctrl+d", "ctrl+u"}
	}
	var out []string
	for _, part := range strings.Split(key, "/") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
