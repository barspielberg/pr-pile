package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withConfigPath(t *testing.T, path string) {
	t.Helper()
	t.Setenv("PILE_CONFIG", path)
}

// No repo is baked into the binary, so a bare Default is not usable on its own.
func TestDefaultCarriesNoRepo(t *testing.T) {
	if got := Default().Repo; got != "" {
		t.Errorf("Default() should not hardcode a repo, got %q", got)
	}
	if err := Default().Validate(); err == nil {
		t.Error("a config with no repo should not validate")
	}
}

func TestLoadReadsRepoAndRulesFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	os.WriteFile(path, []byte(`
repo: someorg/somerepo
rules:
  - name: Only mine
    query: author:@me
    limit: 7
`), 0o644)
	withConfigPath(t, path)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Repo != "someorg/somerepo" {
		t.Errorf("repo = %q", cfg.Repo)
	}
	// An explicit rules list replaces the defaults rather than merging.
	if len(cfg.Rules) != 1 || cfg.Rules[0].Name != "Only mine" {
		t.Errorf("rules = %+v", cfg.Rules)
	}
	if got := cfg.SearchQuery(cfg.Rules[0]); got != "repo:someorg/somerepo is:pr is:open author:@me" {
		t.Errorf("query = %q", got)
	}
}

func TestFirstRunWritesStarterConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "config.yml")
	withConfigPath(t, path)
	t.Setenv("PILE_REPO", "inferred/repo")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Repo != "inferred/repo" {
		t.Errorf("repo = %q", cfg.Repo)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("starter config was not written: %v", err)
	}
	if !strings.Contains(string(written), "repo: inferred/repo") {
		t.Errorf("starter config missing the repo:\n%s", written)
	}
	// The file it writes must be one it can read back.
	if _, err := Load(); err != nil {
		t.Errorf("re-reading the starter config failed: %v", err)
	}
}

func TestRepoEnvOverridesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	os.WriteFile(path, []byte("repo: from/file\nrules:\n  - name: m\n    query: author:@me\n"), 0o644)
	withConfigPath(t, path)
	t.Setenv("PILE_REPO", "from/env")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Repo != "from/env" {
		t.Errorf("env should win, got %q", cfg.Repo)
	}
}

func TestValidateRejectsBadConfigs(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  Config
	}{
		{"no repo", Config{Rules: []Rule{{Name: "a", Query: "b"}}}},
		{"repo without owner", Config{Repo: "justname", Rules: []Rule{{Name: "a", Query: "b"}}}},
		{"no rules", Config{Repo: "o/r"}},
		{"rule without name", Config{Repo: "o/r", Rules: []Rule{{Query: "b"}}}},
		{"rule without query", Config{Repo: "o/r", Rules: []Rule{{Name: "a"}}}},
	} {
		if err := tc.cfg.Validate(); err == nil {
			t.Errorf("%s: expected an error", tc.name)
		}
	}
}
