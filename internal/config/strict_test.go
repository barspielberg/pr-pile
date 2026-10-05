package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRejectsUnknownConfigFields(t *testing.T) {
	for _, body := range []string{
		"repos:\n  - name: o/r\nruless: []\n",
		"repos:\n  - name: o/r\nrules:\n  - name: mine\n    query: author:@me\n    limti: 20\n",
	} {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.yml")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		withConfigPath(t, path)
		if _, err := Load(); err == nil {
			t.Fatalf("Load accepted unknown field in:\n%s", body)
		}
	}
}

func TestLoadRejectsTrailingYAMLDocument(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	body := "repos:\n  - name: o/r\nrules:\n  - name: mine\n    query: author:@me\n---\nrepo: ignored/repo\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	withConfigPath(t, path)
	if _, err := Load(); err == nil {
		t.Fatal("Load accepted a trailing YAML document")
	}
}

func TestValidateRejectsMalformedRepo(t *testing.T) {
	for _, repo := range []string{"owner", "/repo", "owner/", "a/b/c", " owner/repo", "owner /repo", "owner/re po"} {
		cfg := Default()
		cfg.Repos = []Repo{{Name: repo}}
		if err := cfg.Validate(); err == nil {
			t.Errorf("repo %q validated", repo)
		}
	}
}

func TestValidateRuleLimitBounds(t *testing.T) {
	for _, tc := range []struct {
		limit int
		valid bool
	}{
		{limit: 0, valid: true},
		{limit: 1, valid: true},
		{limit: 100, valid: true},
		{limit: -1, valid: false},
		{limit: 101, valid: false},
	} {
		cfg := Default()
		cfg.Repos = []Repo{{Name: "owner/repo"}}
		cfg.Rules[0].Limit = tc.limit
		err := cfg.Validate()
		if (err == nil) != tc.valid {
			t.Errorf("limit %d: err=%v, valid=%v", tc.limit, err, tc.valid)
		}
	}
}

func TestValidateActionContracts(t *testing.T) {
	base := Default()
	base.Repos = []Repo{{Name: "owner/repo"}}
	for _, tc := range []struct {
		name    string
		actions []Action
		valid   bool
	}{
		{name: "default mode", actions: []Action{{Key: "w", Name: "worktree", Run: "true"}}, valid: true},
		{name: "background", actions: []Action{{Key: "w", Name: "worktree", Run: "true", Mode: "background"}}, valid: true},
		{name: "suspend", actions: []Action{{Key: "W", Name: "review", Run: "true", Mode: "suspend"}}, valid: true},
		{name: "unknown mode", actions: []Action{{Key: "w", Name: "worktree", Run: "true", Mode: "async"}}},
		{name: "blank key", actions: []Action{{Name: "worktree", Run: "true"}}},
		{name: "blank name", actions: []Action{{Key: "w", Run: "true"}}},
		{name: "blank run", actions: []Action{{Key: "w", Name: "worktree"}}},
		{name: "control key", actions: []Action{{Key: "\n", Name: "worktree", Run: "true"}}},
		{name: "reserved key", actions: []Action{{Key: "d", Name: "worktree", Run: "true"}}},
		{name: "duplicate key", actions: []Action{{Key: "w", Name: "one", Run: "true"}, {Key: "w", Name: "two", Run: "true"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			cfg.Actions = tc.actions
			err := cfg.Validate()
			if (err == nil) != tc.valid {
				t.Fatalf("Validate() error=%v, valid=%v", err, tc.valid)
			}
		})
	}
}

func TestPageSizeDefaultsOnlyZero(t *testing.T) {
	if got := (Rule{}).PageSize(); got != 20 {
		t.Fatalf("zero limit PageSize() = %d, want 20", got)
	}
	if got := (Rule{Limit: -1}).PageSize(); got == 20 {
		t.Fatal("negative limit silently defaulted")
	}
}

func TestValidateRejectsWhitespaceActionFields(t *testing.T) {
	cfg := Default()
	cfg.Repos = []Repo{{Name: "owner/repo"}}
	for _, action := range []Action{
		{Key: " ", Name: "name", Run: "true"},
		{Key: "w", Name: " ", Run: "true"},
		{Key: "w", Name: "name", Run: " \t"},
	} {
		cfg.Actions = []Action{action}
		if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "action") {
			t.Errorf("action %+v: error=%v", action, err)
		}
	}
}

// The selection keys are builtins, so a config that binds them is dead config.
// The guide's rule is that a taken key is refused loudly rather than silently
// losing to the builtin.
func TestSelectionKeysAreReserved(t *testing.T) {
	for _, key := range []string{"v", " ", "space"} {
		c := Config{
			Repos:   []Repo{{Name: "o/r"}},
			Rules:   []Rule{{Name: "Mine", Query: "author:@me"}},
			Actions: []Action{{Key: key, Name: "clash", Run: "true"}},
		}
		if err := c.Validate(); err == nil {
			t.Errorf("key %q was accepted, want it reserved", key)
		}
	}
}
