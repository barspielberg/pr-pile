// Package config loads the rule list that drives the whole board.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Rule is one section of the board. Order is meaning: a PR is shown under the
// first rule that matches it, so moving a rule up widens it at the expense of
// everything below.
type Rule struct {
	Name  string `yaml:"name"`
	Query string `yaml:"query"`
	Limit int    `yaml:"limit"`
	Tree  bool   `yaml:"tree"`
}

// Action is a shell command bound to a key. The tool knows nothing about
// worktrees or editors; it renders the template and runs whatever it gets.
type Action struct {
	Key  string `yaml:"key"`
	Name string `yaml:"name"`
	Run  string `yaml:"run"`
	Mode string `yaml:"mode"` // background (default) | suspend
}

type Config struct {
	Repo     string        `yaml:"repo"`
	RepoPath string        `yaml:"repoPath"`
	Refresh  time.Duration `yaml:"refresh"`
	Rules    []Rule        `yaml:"rules"`
	Actions  []Action      `yaml:"actions"`
}

// Rule queries are fetched in parallel but revealed in order, so a slow rule
// near the top stalls everything under it. Cheap, high-value rules go first.
func Default() Config {
	return Config{
		Repo:    "acme/monorepo",
		Refresh: 3 * time.Minute,
		Rules: []Rule{
			{Name: "Mine", Query: "author:@me", Limit: 50, Tree: true},
			{Name: "Review requested", Query: "review-requested:@me", Limit: 50},
			{Name: "Involved", Query: "involves:@me -author:@me", Limit: 20},
			{Name: "All open", Query: "draft:false", Limit: 20},
		},
		Actions: []Action{
			{Key: "o", Name: "open in browser", Run: "", Mode: "background"},
		},
	}
}

func Path() string {
	if p := os.Getenv("PRS_MNG_CONFIG"); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "prs-mng", "config.yml")
}

// Load falls back to defaults when no config exists, so the tool runs before
// the user has written one.
func Load() (Config, error) {
	cfg := Default()
	path := Path()
	if path == "" {
		return cfg, nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("read %s: %w", path, err)
	}
	// Decode over the defaults so an absent key keeps its default rather than
	// zeroing; an explicit `rules:` list replaces the defaults wholesale.
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, cfg.Validate()
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.Repo) == "" {
		return fmt.Errorf("repo is required (owner/name)")
	}
	if !strings.Contains(c.Repo, "/") {
		return fmt.Errorf("repo %q must be owner/name", c.Repo)
	}
	if len(c.Rules) == 0 {
		return fmt.Errorf("at least one rule is required")
	}
	for i, r := range c.Rules {
		if strings.TrimSpace(r.Name) == "" {
			return fmt.Errorf("rule %d: name is required", i+1)
		}
		if strings.TrimSpace(r.Query) == "" {
			return fmt.Errorf("rule %q: query is required", r.Name)
		}
	}
	return nil
}

// SearchQuery scopes a rule to the configured repo and to open PRs. Rules only
// carry the part that distinguishes them.
func (c Config) SearchQuery(r Rule) string {
	return fmt.Sprintf("repo:%s is:pr is:open %s", c.Repo, r.Query)
}

func (r Rule) PageSize() int {
	if r.Limit <= 0 {
		return 20
	}
	return r.Limit
}
