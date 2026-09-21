# prs-mng

A terminal PR board. One keybind away, shows every PR worth looking at, opens them.

Run it as `prs`. See [docs/DESIGN.md](docs/DESIGN.md) for the design, the decisions behind it, and the measured numbers.

## Status

Scaffolding. The board fetches, buckets, renders, searches and opens PRs. Not yet: review progress.

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

No actions are bound by default. The tool knows nothing about worktrees,
editors or multiplexers — it renders a template and runs what it gets — so a
default that shelled out to a script only the author has would fail with
`command not found` on anyone else's machine. The examples below are ready to
paste.

### Example: open a PR as a worktree workspace (Worktrunk + herdr)

```yaml
repoPath: ~/Repos/owner/name    # {{.RepoPath}} is empty without this

actions:
  - key: w
    name: worktree
    run: $HOME/path/to/pr-workspace {{.RepoPath}} {{.Number}} {{.Branch}}
```

Give the script by path rather than by bare name: the action runs through
`sh -c` with whatever `PATH` the board inherited, which is not necessarily the
one an interactive shell has.

Two steps, and both are already idempotent, which is what makes this safe to
bind to a key that will get pressed twice:

```sh
p="$(wt switch "$3" --no-cd --format json | jq -r '.path // empty')"
[ -n "$p" ] || p="$(wt switch "pr:$2" --no-cd --format json | jq -r '.path // empty')"
exec herdr worktree open --cwd "$1" --path "$p" --no-focus
```

**Pass the branch and prefer it.** `wt switch pr:<n>` resolves the number
through the GitHub API on *every* call, even when the worktree already exists —
7.0s against 0.07s for `wt switch <branch>` on that same worktree. The board
already knows the head ref, so passing `{{.Branch}}` skips the round trip
entirely. Keep `pr:<n>` as the fallback: a branch resolves only if it is on
`origin`, so a PR from a fork still needs the number.

`herdr worktree open` returns the existing workspace id for a path it has
already opened — measured at three calls, one workspace — so no dedupe logic is
needed.

A background action reports itself on the status line: a spinner while it runs,
`worktree ✓` on success (cleared after a few seconds), and `worktree failed:`
followed by the command's own last line of stderr when it does not.

Use `herdr worktree open` (a **workspace**), not `herdr tab create` (a tab in
whatever workspace you happen to be in). `tab create` is also not idempotent: it
will open a second tab on the same directory every press.

`--no-focus` matters for a key binding. Opening the workspace focused yanks you
out of whatever you were doing on every press; without it the workspace is there
when you want it and you are not moved.

Leave it on the default `background` mode. `suspend` is for commands that take
over the terminal (a pager, a review TUI); this one only talks to the herdr
socket and exits, so suspending would blank the board for a few seconds to run
something that never wanted the terminal.

One sharp edge that is not this tool's doing: `wt remove` deregisters a worktree
but can leave the directory behind (a `node_modules` from a post-start hook is
enough), and the next `wt switch pr:<n>` then fails with *Directory already
exists* rather than reusing it. If the key stops working on a PR you have
removed before, clear the leftover directory.

Action keys are matched **last**, so they cannot shadow a built-in key: binding
an action to `d` or `j` means it never fires. Pick a key the table above does
not use.

Action templates get `{{.Number}}`, `{{.Repo}}`, `{{.RepoPath}}`, `{{.Branch}}`, `{{.Base}}`, `{{.URL}}`, `{{.Author}}`, `{{.Title}}`.

[docs/config-example.md](docs/config-example.md) works a five-section team board
through end to end: what each rule claims versus what it actually shows once the
rules above it have taken their share, why `-review:approved` and not
`review:required`, and why "PRs from a team" has to be written as a list of
authors.

## Keys

| key | action |
|---|---|
| `j` / `k` | move (also `↓` / `↑`) |
| `l` / `h` | next / previous section (also `→` / `←`) |
| `g` / `G` | top / bottom (also `home` / `end`) |
| `enter` / `o` | open in browser (reuses an existing Arc tab) |
| `d` | detail for the selected PR: failing and running checks named, passing counted, plus conflict, unresolved comments, author, reviewer, size and branches |
| `y` | copy the PR url to the clipboard (`pbcopy`) |
| `/` | search |
| `n` / `N` | next / previous match |
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

### Searching

`/` searches the board without moving it. Nothing is hidden, nothing is
reordered, no stack glyph changes — matches are **filled where they sit**,
the cursor walks to the first one as you type, and `n` / `N` step through the
rest, wrapping at the ends.

The query is a plain case-insensitive substring of **what the row draws**: the
PR number, the title as it appears at this width, and the three author initials.
Nothing else, on purpose — matching text that is not on screen would mark a row
with nothing visibly marked on it.

Three things follow from that, and they are the design rather than caveats:

- **Author search is the three characters you can see.** `imm` matches and
  highlights the initials; `immanuel` matches nothing, because characters four
  onward are not drawn.
- **A title match past the clip point does not exist.** Widen the pane and it
  appears; narrow it and it goes. In a narrow pane the author and age columns
  are not drawn at all, so they are not searchable either.
- **The age column is never searchable**, even though it is drawn: it is
  computed from the clock, so a match on it would expire on its own.

`enter` keeps the query and its highlights and closes the prompt — that is what
`n` / `N` run on afterwards. `esc` in the prompt cancels and puts the cursor
back where `/` was pressed; `esc` on the board clears the highlights.

While typing, every printable key is query text, so navigation moves to chords:

| key | action |
|---|---|
| `ctrl+n` / `ctrl+p` | next / previous match |
| `ctrl+j` / `ctrl+k` | same, other convention |
| `↓` / `↑` | same |
| `enter` | keep the query and the highlights, close the prompt |
| `backspace` | edit the query |
| `ctrl+u` | clear the query |
| `esc` | cancel, restoring the cursor |
| `ctrl+c` | quit |
