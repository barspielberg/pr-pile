package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func withConfigPath(t *testing.T, path string) {
	t.Helper()
	t.Setenv("PILE_CONFIG", path)
}

// No repo is baked into the binary, so a bare Default is not usable on its own.
func TestDefaultCarriesNoRepo(t *testing.T) {
	if got := Default().Repos; len(got) != 0 {
		t.Errorf("Default() should not hardcode a repo, got %v", got)
	}
	if err := Default().Validate(); err == nil {
		t.Error("a config with no repo should not validate")
	}
}

func TestLoadReadsRepoAndRulesFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	os.WriteFile(path, []byte(`
repos:
  - name: someorg/somerepo
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
	if len(cfg.Repos) != 1 || cfg.Repos[0].Name != "someorg/somerepo" {
		t.Errorf("repos = %+v", cfg.Repos)
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
	orig := detectRepo
	detectRepo = func() (string, error) { return "inferred/repo", nil }
	t.Cleanup(func() { detectRepo = orig })

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Repos) != 1 || cfg.Repos[0].Name != "inferred/repo" {
		t.Errorf("repos = %+v", cfg.Repos)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("starter config was not written: %v", err)
	}
	if !strings.Contains(string(written), "  - name: inferred/repo") {
		t.Errorf("starter config missing the repo:\n%s", written)
	}
	// The file it writes must be one it can read back.
	if _, err := Load(); err != nil {
		t.Errorf("re-reading the starter config failed: %v", err)
	}
}

// A config from before repos: fails with the replacement spelled out, rather
// than the strict decoder's bare unknown-field error.
func TestLoadExplainsTheOldRepoKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	os.WriteFile(path, []byte("repo: o/r\nrepoPath: ~/src/r\nrules:\n  - name: m\n    query: author:@me\n"), 0o644)
	withConfigPath(t, path)

	_, err := Load()
	if err == nil {
		t.Fatal("the old repo key loaded")
	}
	for _, want := range []string{"repos:", "- name: o/r", "path: ~/src/r"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q:\n%v", want, err)
		}
	}
}

func TestValidateRejectsBadConfigs(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  Config
	}{
		{"no repo", Config{Rules: []Rule{{Name: "a", Query: "b"}}}},
		{"repo without owner", Config{Repos: []Repo{{Name: "justname"}}, Rules: []Rule{{Name: "a", Query: "b"}}}},
		{"no rules", Config{Repos: []Repo{{Name: "o/r"}}}},
		{"rule without name", Config{Repos: []Repo{{Name: "o/r"}}, Rules: []Rule{{Query: "b"}}}},
		{"rule without query", Config{Repos: []Repo{{Name: "o/r"}}, Rules: []Rule{{Name: "a"}}}},
		{"repo: in a query", Config{Repos: []Repo{{Name: "o/r"}}, Rules: []Rule{{Name: "a", Query: "b repo:o/api"}}}},
		{"unknown rule repo", Config{Repos: []Repo{{Name: "o/r"}}, Rules: []Rule{{Name: "a", Query: "b", Repos: []string{"api"}}}}},
		{"repo twice", Config{Repos: []Repo{{Name: "o/r"}, {Name: "O/R"}}, Rules: []Rule{{Name: "a", Query: "b"}}}},
		{"labels collide", Config{Repos: []Repo{{Name: "o/r"}, {Name: "x/r"}}, Rules: []Rule{{Name: "a", Query: "b"}}}},
		{"label with a space", Config{Repos: []Repo{{Name: "o/r", Label: "my r"}}, Rules: []Rule{{Name: "a", Query: "b"}}}},
	} {
		if err := tc.cfg.Validate(); err == nil {
			t.Errorf("%s: expected an error", tc.name)
		}
	}
}

func TestWatchNotify(t *testing.T) {
	for _, tc := range []struct {
		watch Watch
		valid bool
	}{
		{Watch{}, true},
		{Watch{Notify: "osc"}, true},
		{Watch{Notify: "none"}, true},
		{Watch{Notify: "command", Command: "notify-send x"}, true},
		{Watch{Notify: "command"}, false},
		{Watch{Notify: "herdr"}, false},
	} {
		cfg := Default()
		cfg.Repos = []Repo{{Name: "o/r"}}
		cfg.Watch = tc.watch
		if err := cfg.Validate(); (err == nil) != tc.valid {
			t.Errorf("%+v: err=%v, want valid=%v", tc.watch, err, tc.valid)
		}
	}
}

func TestWatchKeyIsReserved(t *testing.T) {
	cfg := Default()
	cfg.Repos = []Repo{{Name: "o/r"}}
	cfg.Actions = []Action{{Key: "m", Name: "mine", Run: "true"}}
	if err := cfg.Validate(); err == nil {
		t.Error("binding m to an action should be refused")
	}
}

// A rule searches every declared repo unless it names some by label, and the
// repo: qualifiers come from repos: rather than from the query.
func TestSearchQueryScopesToTheRulesRepos(t *testing.T) {
	c := Config{
		Repos: []Repo{{Name: "o/web"}, {Name: "o/api"}, {Name: "x/lib", Label: "xlib"}},
		Rules: []Rule{
			{Name: "all", Query: "author:@me"},
			{Name: "some", Query: "draft:false", Repos: []string{"API", "xlib"}},
		},
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{
		"repo:o/web repo:o/api repo:x/lib is:pr is:open author:@me",
		"repo:o/api repo:x/lib is:pr is:open draft:false",
	} {
		if got := c.SearchQuery(c.Rules[i]); got != want {
			t.Errorf("rule %d: got %q, want %q", i, got, want)
		}
	}
}

// Two repos with the same name under different owners are fine once one has
// a label of its own.
func TestLabelSettlesACollision(t *testing.T) {
	c := Config{
		Repos: []Repo{{Name: "o/r"}, {Name: "x/r", Label: "xr"}},
		Rules: []Rule{{Name: "a", Query: "b"}},
	}
	if err := c.Validate(); err != nil {
		t.Error(err)
	}
}

func badgeCfg(rule Rule, badges ...Badge) Config {
	return Config{Repos: []Repo{{Name: "o/r"}}, Rules: []Rule{rule}, Badges: badges}
}

func TestValidateRejectsBadBadges(t *testing.T) {
	rule := Rule{Name: "a", Query: "b"}
	ok := Badge{Name: "jira", Run: "jira-status {{.Branch}}"}
	for _, tc := range []struct {
		name string
		cfg  Config
	}{
		{"no name", badgeCfg(rule, Badge{Run: "true"})},
		{"no run", badgeCfg(rule, Badge{Name: "x"})},
		{"twice", badgeCfg(rule, ok, ok)},
		{"does not parse", badgeCfg(rule, Badge{Name: "x", Run: "echo {{.Number"})},
		{"plural field", badgeCfg(rule, Badge{Name: "x", Run: "echo {{.Numbers}}"})},
		{"title", badgeCfg(rule, Badge{Name: "x", Run: "echo {{.Title}}"})},
		{"unknown color", badgeCfg(rule, Badge{Name: "x", Run: "true", Color: "blue"})},
		{"unknown side", badgeCfg(rule, Badge{Name: "x", Run: "true", Side: "middle"})},
		{"negative width", badgeCfg(rule, Badge{Name: "x", Run: "true", MaxWidth: -1})},
		{"negative ttl", badgeCfg(rule, Badge{Name: "x", Run: "true", TTL: -time.Second})},
		{"negative timeout", badgeCfg(rule, Badge{Name: "x", Run: "true", Timeout: -time.Second})},
		{"unknown badge in a rule", badgeCfg(Rule{Name: "a", Query: "b", Badges: &[]string{"nope"}}, ok)},
		{"negative workers", func() Config { c := badgeCfg(rule, ok); c.BadgeWorkers = -1; return c }()},
	} {
		if err := tc.cfg.Validate(); err == nil {
			t.Errorf("%s: expected an error", tc.name)
		}
	}
	if err := badgeCfg(rule, ok).Validate(); err != nil {
		t.Errorf("a plain badge was refused: %v", err)
	}
}

// Left out means every badge and an empty list means none, which only a
// pointer can tell apart once YAML has decoded it.
func TestRuleBadgesDistinguishesAbsentFromEmpty(t *testing.T) {
	var cfg struct {
		Rules []Rule `yaml:"rules"`
	}
	data := "rules:\n  - name: all\n  - name: none\n    badges: []\n  - name: one\n    badges: [jira]\n"
	if err := yaml.Unmarshal([]byte(data), &cfg); err != nil {
		t.Fatal(err)
	}
	for i, want := range []bool{true, false, true} {
		if got := cfg.Rules[i].ShowsBadge("jira"); got != want {
			t.Errorf("rule %q shows jira = %v, want %v", cfg.Rules[i].Name, got, want)
		}
	}
	if cfg.Rules[2].ShowsBadge("deploy") {
		t.Error("a rule listing jira also showed deploy")
	}
}

func TestBadgeDefaults(t *testing.T) {
	b := Badge{Name: "x", Run: "true"}.WithDefaults()
	if b.Color != "muted" || b.Side != "left" || b.MaxWidth != 20 || b.TTL != 5*time.Minute || b.Timeout != 10*time.Second {
		t.Errorf("defaults = %+v", b)
	}
	if got := (Config{}).Workers(); got != 4 {
		t.Errorf("workers = %d, want 4", got)
	}
}
