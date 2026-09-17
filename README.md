# prs-mng

A terminal PR board. One keybind away, shows every PR worth looking at, opens them.

Run it as `prs`. See [docs/DESIGN.md](docs/DESIGN.md) for the design, the decisions behind it, and the measured numbers.

## Status

Scaffolding. The board fetches, buckets, renders, filters and opens PRs. Not yet: review progress.

## Build

```sh
go build -o prs ./cmd/prs
```

Needs `gh` installed and logged in (used only for the token) and a Nerd Font for the glyphs.

## How it works

Every section is a **rule** — a GitHub search query plus a name. Rules are an ordered list, and a PR is shown under the **first** rule that matches it, so ordering is the configuration. The rule's name is shown in the left gutter on the section's first row.

Every row is exactly one line, which is what keeps scrolling steady: one keypress moves the board by at most one line. Failing check names are behind `c` rather than in the list — see [docs/uniform-rows.md](docs/uniform-rows.md).

Rules are fetched in parallel but revealed in order: a section can only be drawn once every section above it has resolved, because an earlier rule may still claim a PR a later one has already fetched.

## Config

`~/.config/prs-mng/config.yml` (honors `$XDG_CONFIG_HOME`), or `$PRS_MNG_CONFIG`.

On first run inside a GitHub checkout, the repo is inferred with `gh` and a
commented starter config is written. `PRS_MNG_REPO=owner/name` overrides the
configured repo for one invocation.

```yaml
repo: owner/name
repoPath: ~/Repos/owner/name
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
| `c` | failing check names for the selected PR |
| `/` | filter |
| `r` | reload |
| `q` | quit |

### Filtering

`/` opens a fuzzy filter over the board. The query matches the PR title and the
PR number together, so `3248` finds `#3248` and `apisvc` finds
`feat(api-service): …`. Matches are ranked best-first and the matched characters
are underlined; sections with no matches are hidden entirely.

While filtering, every printable key is query text, so navigation moves to chords:

| key | action |
|---|---|
| `ctrl+n` / `ctrl+p` | move down / up |
| `ctrl+j` / `ctrl+k` | same, other convention |
| `↓` / `↑` | same |
| `enter` | open the selected PR and leave the filter |
| `backspace` | edit the query |
| `ctrl+u` | clear the query |
| `esc` | leave the filter, restoring the full board |
| `ctrl+c` | quit |
