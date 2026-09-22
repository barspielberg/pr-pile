// Package config loads the rule list that drives the whole board.
package config

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"

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
// from the config file or the PILE_REPO env var.
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
const starterConfig = `# pile configuration.
# Rules are an ordered list: a PR is shown under the FIRST rule that matches it,
# so ordering is the configuration. Queries are GitHub search syntax, scoped to
# the repo and to open PRs automatically.

repo: %s
refresh: 3m

rules:
  # A chain is worked out from the PRs in this section only, and limit truncates
  # before that happens -- keep a tree rule's limit above the number of PRs it
  # matches, or a stack can lose its base and regroup into a shorter chain.
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
  # Note this is not the same as "PRs written by that team" -- GitHub has no
  # qualifier for that; enumerate the members as author:a author:b instead,
  # which ORs. docs/config-example.md works a team-based board through.
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
# GitHub string fields are already shell-quoted; use their placeholders without
# adding quotes around them. Repo, repoPath, and run remain trusted shell text.
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
	if p := os.Getenv("PILE_CONFIG"); p != "" {
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
	return filepath.Join(dir, "pile", "config.yml")
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
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return cfg, fmt.Errorf("parse %s: %w", path, err)
		}
		return cfg, fmt.Errorf("parse %s: multiple YAML documents are not supported", path)
	}
	if env := os.Getenv("PILE_REPO"); env != "" {
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
	if env := os.Getenv("PILE_REPO"); env != "" {
		return env, nil
	}
	out, err := exec.Command("gh", "repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner").Output()
	if err != nil {
		return "", fmt.Errorf("run pile from a GitHub checkout, or set PILE_REPO=owner/name")
	}
	repo := strings.TrimSpace(string(out))
	if repo == "" {
		return "", fmt.Errorf("gh returned no repo name")
	}
	return repo, nil
}

func (c Config) Validate() error {
	parts := strings.Split(c.Repo, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" ||
		strings.TrimSpace(c.Repo) != c.Repo || strings.IndexFunc(c.Repo, unicode.IsSpace) >= 0 {
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
		if r.Limit < 0 || r.Limit > 100 {
			return fmt.Errorf("rule %q: limit must be 0 or between 1 and 100", r.Name)
		}
	}
	seen := make(map[string]bool, len(c.Actions))
	for i, a := range c.Actions {
		if strings.TrimSpace(a.Key) == "" || strings.IndexFunc(a.Key, unicode.IsControl) >= 0 {
			return fmt.Errorf("action %d: key is required and cannot contain control characters", i+1)
		}
		if reservedActionKeys[a.Key] {
			return fmt.Errorf("action %q: key %q is reserved", a.Name, a.Key)
		}
		if seen[a.Key] {
			return fmt.Errorf("action %q: key %q is already bound", a.Name, a.Key)
		}
		seen[a.Key] = true
		if strings.TrimSpace(a.Name) == "" {
			return fmt.Errorf("action %d: name is required", i+1)
		}
		if strings.TrimSpace(a.Run) == "" {
			return fmt.Errorf("action %q: run is required", a.Name)
		}
		if a.Mode != "" && a.Mode != "background" && a.Mode != "suspend" {
			return fmt.Errorf("action %q: mode must be background or suspend", a.Name)
		}
	}
	return nil
}

var reservedActionKeys = map[string]bool{
	"ctrl+c": true, "q": true, "esc": true, "/": true, "n": true, "N": true,
	"enter": true, "o": true, "r": true, "d": true, "y": true, "?": true,
	"j": true, "down": true, "k": true, "up": true, "l": true, "right": true,
	"h": true, "left": true, "g": true, "home": true, "G": true, "end": true,
	"ctrl+d": true, "ctrl+u": true, "pgdown": true, "pgup": true,
}

// SearchQuery scopes a rule to the configured repo and to open PRs. Rules only
// carry the part that distinguishes them.
func (c Config) SearchQuery(r Rule) string {
	return fmt.Sprintf("repo:%s is:pr is:open %s", c.Repo, r.Query)
}

func (r Rule) PageSize() int {
	if r.Limit == 0 {
		return 20
	}
	return r.Limit
}
