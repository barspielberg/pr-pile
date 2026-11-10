package shelltmpl

import (
	"reflect"
	"testing"
)

// The guard against someone adding a GitHub string to the template data as a
// plain field, which would reach the command unchecked. An audit nothing
// re-checks goes stale (design/index.md §12).
func TestTemplateDataExposesNoUncheckedString(t *testing.T) {
	// Repo and RepoPath come from the user's own config, not from GitHub, and
	// Numbers is integer-derived. GitHub strings are methods that check them.
	trusted := map[string]bool{"Repo": true, "RepoPath": true, "Numbers": true}
	for _, data := range []any{Data{}, MultiData{}} {
		ty := reflect.TypeOf(data)
		for i := 0; i < ty.NumField(); i++ {
			f := ty.Field(i)
			if f.IsExported() && f.Type.Kind() == reflect.String && !trusted[f.Name] {
				t.Errorf("%s.%s reaches templates unchecked; make it a method that validates it",
					ty.Name(), f.Name)
			}
			// fmt prints a nested pointer as an address but any other nested
			// value in full, so {{.}} would dump it unchecked.
			if !f.IsExported() && f.Type.Kind() != reflect.Pointer {
				t.Errorf("%s.%s would print in {{.}}; hold it behind a pointer", ty.Name(), f.Name)
			}
		}
	}
}

// Config calls Check at load time, so a template that cannot parse has to
// fail there rather than only when it runs.
func TestCheckRejectsATemplateThatDoesNotParse(t *testing.T) {
	if err := Check("echo {{.Number", false); err == nil {
		t.Fatal("an unterminated template was accepted")
	}
}
