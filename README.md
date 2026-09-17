# prs-mng

A terminal PR board. One keybind away, shows every PR worth looking at, opens them.

Run it as `prs`. See [docs/DESIGN.md](docs/DESIGN.md) for the design, the decisions behind it, and the measured numbers.

## Status

Scaffolding. The board fetches, buckets, renders, filters and opens PRs. Not yet: review progress.

## Build

```sh
make build     # build ./prs
make run       # build and run it, without installing over the prs on your PATH
make test      # go test ./... -short
make check     # build, test, vet
```

Needs `gh` installed and logged in (used only for the token) and a Nerd Font for the glyphs.

## How it works

Every section is a **rule** — a GitHub search query plus a name. Rules are an ordered list, and a PR is shown under the **first** rule that matches it, so ordering is the configuration. The rule's name is shown in the left gutter on the section's first visible row, clipped to 8 cells, and on the top visible row even when that section began above the fold.

Every row is exactly one line, which is what keeps scrolling steady: one keypress moves the board by at most one line. Failing check names are behind `d` rather than in the list — see [docs/uniform-rows.md](docs/uniform-rows.md).

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
  - key: v
    name: review
    run: tuicr pr {{.Number}}
    mode: suspend       # hand over the terminal; default is background
```

Action keys are matched **last**, so they cannot shadow a built-in key: binding
an action to `d` or `j` means it never fires. Pick a key the table above does
not use.

Action templates get `{{.Number}}`, `{{.Repo}}`, `{{.RepoPath}}`, `{{.Branch}}`, `{{.Base}}`, `{{.URL}}`, `{{.Author}}`, `{{.Title}}`.

## Keys

| key | action |
|---|---|
| `j` / `k` | move (also `↓` / `↑`) |
| `l` / `h` | next / previous section (also `→` / `←`) |
| `gg` / `G` | top / bottom (also `g`, `home` / `end`) |
| `enter` / `o` | open in browser (reuses an existing Arc tab) |
| `d` | detail for the selected PR: failing and running checks named, passing counted, plus conflict, unresolved comments, author, reviewer, size and branches |
| `/` | filter |
| `r` | reload |
| `?` | help and the glyph legend |
| `q` / `esc` / `ctrl+c` | quit |

The `d` detail overlay closes on **any** key, and a movement key closes it *and*
moves, in one press.

The `?` help page **scrolls**, because the legend is longer than most panes:
`j`/`k`, the arrows, `ctrl+d`/`ctrl+u`, `pgdn`/`pgup`, and `g`/`G` for the ends.
Since `j` and `k` now scroll rather than close, `esc`, `q` and `?` close it —
and so does any other key that is not a scroll key, so you cannot get stuck. The
page's bottom row says both: `esc q ? close · j/k scroll` on the left, and your
position (`9-45 of 45 · end`) on the right. The row is there at every height —
when the whole legend fits it just reads `esc q ? close` and `all 45`.

The footer's right field names the section the cursor is in and where it sits
inside it — `NEEDS MY REVIEW · 3 of 10`. The gutter only has 8 cells, so this is
the only place the full rule name and the count are legible.

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
