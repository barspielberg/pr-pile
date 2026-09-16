# prs-mng

A terminal PR board. One keybind away, shows every PR worth looking at, opens them.

Run it as `prs`. See [REQUIREMENTS.md](REQUIREMENTS.md) for the design and the latency measurements behind it.

## Status

Scaffolding. The board fetches, buckets, renders and opens PRs. Not yet: review progress, filtering, detail pane.

## Build

```sh
go build -o prs ./cmd/prs
```

Needs `gh` installed and logged in (used only for the token) and a Nerd Font for the glyphs.

## How it works

Every section is a **rule** — a GitHub search query plus a name. Rules are an ordered list, and a PR is shown under the **first** rule that matches it, so ordering is the configuration.

Rules are fetched in parallel but revealed in order: a section can only be drawn once every section above it has resolved, because an earlier rule may still claim a PR a later one has already fetched.

## Config

`~/.config/prs-mng/config.yml`, or `$PRS_MNG_CONFIG`. Defaults work with no file.

```yaml
repo: acme/monorepo
repoPath: ~/Repos/acme/monorepo
refresh: 3m

rules:
  - name: Mine
    query: author:@me
    tree: true          # group stacked PRs into a chain
    limit: 50
  - name: My team's review
    query: team-review-requested:acme/web-platform
  - name: Review requested
    query: review-requested:@me
  - name: All open
    query: draft:false
    limit: 20

actions:
  - key: w
    name: worktree
    run: wt switch -x nvim pr:{{.Number}}
  - key: d
    name: review
    run: tuicr pr {{.Number}}
    mode: suspend       # hand over the terminal; default is background
```

Action templates get `{{.Number}}`, `{{.Repo}}`, `{{.RepoPath}}`, `{{.Branch}}`, `{{.Base}}`, `{{.URL}}`, `{{.Author}}`, `{{.Title}}`.

## Keys

| key | action |
|---|---|
| `j` / `k` | move |
| `g` / `G` | top / bottom |
| `enter` / `o` | open in browser (reuses an existing Arc tab) |
| `r` | reload |
| `q` | quit |
