// Package config loads the rule list that drives the whole board.
package config

import (
	"fmt"
	"os"
	"os/exec"
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
	// Author shows the PR author's initials. Off by default and set per rule:
	// a rule like author:@me is all one person, so the column would be dead
	// weight there.
	Author bool `yaml:"author"`
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
//
// No repo is set here: it is the one value that cannot be guessed, so it comes
// from the config file or the PRS_MNG_REPO env var.
func Default() Config {
	return Config{
		Refresh: 3 * time.Minute,
		Rules: []Rule{
			{Name: "Mine", Query: "author:@me", Limit: 50, Tree: true},
			{Name: "Needs my review", Query: "review-requested:@me -review:approved", Limit: 50, Author: true},
			{Name: "Involved", Query: "involves:@me -author:@me", Limit: 20, Author: true},
			{Name: "All open", Query: "draft:false", Limit: 20, Author: true},
		},
	}
}

// starterConfig is written on first run so there is something to edit rather
// than a bare error. Rules mirror Default() so the file is the single source of
// truth from then on.
const starterConfig = `# prs-mng configuration.
# Rules are an ordered list: a PR is shown under the FIRST rule that matches it,
# so ordering is the configuration. Queries are GitHub search syntax, scoped to
# the repo and to open PRs automatically.

repo: %s
refresh: 3m

rules:
  - name: Mine
    query: author:@me
    tree: true          # group stacked PRs into a chain
    limit: 50

  # -review:approved drops PRs somebody else has already approved. Note that
  # GitHub removes a PR from review-requested:@me once *you* approve it.
  - name: Needs my review
    query: review-requested:@me -review:approved
    author: true        # show the author's initials; pointless where it is you
    limit: 50

  # Review requested from a team you belong to rather than from you personally.
  # This also covers CODEOWNERS, which GitHub turns into team review requests.
  # - name: My team's
  #   query: team-review-requested:ORG/TEAM

  - name: Involved
    query: involves:@me -author:@me
    author: true
    limit: 20

  - name: All open
    query: draft:false
    author: true
    limit: 20

# Actions run a shell command for the selected PR. Available template fields:
# {{.Number}} {{.Repo}} {{.RepoPath}} {{.Branch}} {{.Base}} {{.URL}}
# {{.Author}} {{.Title}}
# actions:
#   - key: w
#     name: worktree
#     run: wt switch -x nvim pr:{{.Number}}
#   - key: v
#     name: review
#     run: tuicr pr {{.Number}}
#     mode: suspend     # hand over the terminal; default is background
`

// Path follows the XDG convention rather than os.UserConfigDir, which on macOS
// returns ~/Library/Application Support -- not where a terminal tool's config
// belongs, and not where the rest of this user's tooling lives.
func Path() string {
	if p := os.Getenv("PRS_MNG_CONFIG"); p != "" {
		return p
	}
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "prs-mng", "config.yml")
}

// Load reads the config file, writing a starter one if none exists. The repo
// can be overridden per-invocation, which is what makes the tool usable from a
// repo other than the configured one without editing the file.
func Load() (Config, error) {
	cfg := Default()
	path := Path()
	if path == "" {
		return cfg, fmt.Errorf("cannot locate a config directory")
	}

	data, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
		repo, rerr := detectRepo()
		if rerr != nil {
			return cfg, fmt.Errorf("no config at %s and no repo to infer one from: %w", path, rerr)
		}
		if werr := writeStarter(path, repo); werr != nil {
			return cfg, werr
		}
		cfg.Repo = repo
		return cfg, cfg.Validate()
	case err != nil:
		return cfg, fmt.Errorf("read %s: %w", path, err)
	}

	// Decode over the defaults so an absent key keeps its default rather than
	// zeroing; an explicit `rules:` list replaces the defaults wholesale.
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", path, err)
	}
	if env := os.Getenv("PRS_MNG_REPO"); env != "" {
		cfg.Repo = env
	}
	return cfg, cfg.Validate()
}

func writeStarter(path, repo string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	body := fmt.Sprintf(starterConfig, repo)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// detectRepo asks gh for the current directory's repo, so first run in a
// checkout needs no arguments.
func detectRepo() (string, error) {
	if env := os.Getenv("PRS_MNG_REPO"); env != "" {
		return env, nil
	}
	out, err := exec.Command("gh", "repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner").Output()
	if err != nil {
		return "", fmt.Errorf("run prs from a GitHub checkout, or set PRS_MNG_REPO=owner/name")
	}
	repo := strings.TrimSpace(string(out))
	if repo == "" {
		return "", fmt.Errorf("gh returned no repo name")
	}
	return repo, nil
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
