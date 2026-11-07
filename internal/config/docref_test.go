package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestReferencePagesDocumentEveryKey guards DOCS-PLAN.md §6.2: every `yaml:`
// tag on Config, Rule and Action must appear as a `code` span on its reference
// page. A renamed or added key then fails here instead of drifting out of the
// docs. The tags are read off the structs with reflection, so the test cannot
// fall out of step with config.go.
func TestReferencePagesDocumentEveryKey(t *testing.T) {
	root := filepath.Join("..", "..") // repo root, relative to internal/config
	cases := []struct {
		name string
		typ  reflect.Type
		page string
	}{
		{"Config", reflect.TypeOf(Config{}), "reference/config.md"},
		{"Rule", reflect.TypeOf(Rule{}), "reference/rules.md"},
		{"Action", reflect.TypeOf(Action{}), "reference/actions.md"},
	}
	for _, tc := range cases {
		body, err := os.ReadFile(filepath.Join(root, "docs", "src", "content", "docs", tc.page))
		if err != nil {
			t.Fatalf("%s: read %s: %v", tc.name, tc.page, err)
		}
		content := string(body)
		for i := 0; i < tc.typ.NumField(); i++ {
			key := tc.typ.Field(i).Tag.Get("yaml")
			key, _, _ = strings.Cut(key, ",") // drop options such as omitempty
			if key == "" || key == "-" {
				continue
			}
			if !strings.Contains(content, "`"+key+"`") {
				t.Errorf("%s: key %q is not documented on %s", tc.name, key, tc.page)
			}
		}
	}
}
