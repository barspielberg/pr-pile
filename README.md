# pr-pile

A terminal PR board. One keybind away, shows every PR worth looking at, opens them.

Run it as `pile`. See [docs/DESIGN.md](docs/DESIGN.md) for the design, the decisions behind it, and the measured numbers.

## Status

Working. The board fetches, buckets and renders PRs with their CI, review and
merge state; searches and highlights in place; opens PRs in the browser; copies
urls; runs configured shell actions; and has a per-PR detail overlay covering
checks, conflicts, unresolved conversations, reviewers and size.

## Build

```sh
make build     # build ./pile
make run       # build and run it, without installing over the pile on your PATH
make test      # go test ./... -short
make check     # build, test, vet
make install   # check, then put this checkout on PATH as pile
```

Currently requires macOS: browser integration uses `open`, and
clipboard integration uses `pbcopy`. It also needs `gh` installed and logged in
(used only for the token) and a Nerd Font for the glyphs.

`enter` opens a new tab every time, so opening the same PR twice leaves two
tabs. To avoid that, install a tab dedupe extension in your browser (for
example "Don't open duplicate tabs" in Firefox), which focuses the tab you
already have instead.

## How it works

Every section is a **rule** — a GitHub search query plus a name. Rules are an ordered list, and a PR is shown under the **first** rule that matches it, so ordering is the configuration. Each section opens with a full-width header row carrying the rule's name and its row count; the header is a position the cursor can sit on, which is what keeps one keypress to one line.

Every row is exactly one line, which is what keeps scrolling steady: one keypress moves the board by at most one line. Failing check names are behind `d` rather than in the list — see [docs/uniform-rows.md](docs/uniform-rows.md).

Rules are fetched in parallel but revealed in order: a section can only be drawn once every section above it has resolved, because an earlier rule may still claim a PR a later one has already fetched.

## Config

`~/.config/pile/config.yml` (honors `$XDG_CONFIG_HOME`), or `$PILE_CONFIG`.

Configuration is strict: unknown fields are errors, every repo name must be
exactly `owner/name`, rule limits are `1` through `100` (`0` means the default
`20`), and action mode is either `background` or `suspend`.

On first run inside a GitHub checkout, the repo is inferred with `gh` and a
commented starter config is written. Outside a checkout, write the config
yourself with a `repos:` list.

`repos` lists every repo the board reads. Each has a `name`, an optional `path`
where it is checked out (for `{{.RepoPath}}`), and an optional `label`, which
defaults to the part after the slash. A rule covers every repo unless it lists
some by label under its own `repos`. Write repos there rather than as `repo:` in
a query: GitHub ORs `repo:` qualifiers, so one in a query would widen the rule
instead of narrowing it, and the config refuses it.

With more than one repo, every row starts with its repo's label, and a PR is
named `label#N` in the detail header, the status line and notifications. With
one repo the column is left out. Every repo is checked at startup, so a typo
fails loudly rather than drawing an empty section.

A rule over several repos is still one search, so its `limit` is shared between
them: a busy repo can crowd a quiet one out of the section.

```yaml
repos:
  - name: owner/name
    path: ~/Repos/owner/name
  - name: owner/api
    label: api          # the default; shown in the repo column

refresh: 3m

rules:
  - name: Mine
    query: author:@me
    tree: true          # group stacked PRs into a chain
    limit: 50
  - name: My team's review
    query: team-review-requested:acme/web-platform
  - name: API review requested
    query: review-requested:@me
    repos: [api]        # only this repo; every repo when left out
  - name: All open
    query: draft:false
    limit: 20

actions:
  - key: w
    name: worktree
    run: wt switch -x nvim pr:{{.Number}}
  - key: W
    name: review
    run: tuicr pr {{.Number}}
    mode: suspend       # hand over the terminal; default is background

  - key: b
    name: browse all
    run: open {{.URLs}}
    multi: true         # runs once for the whole selection
```

Select PRs with `space` (one) or `v` (a range), then `y` copies every selected
url, one per line. `Y` opens a copy menu for the number, title, url, branch,
author or a markdown link instead, also one per line.

`m` watches the selected PRs (or the one under the cursor) and `m` again stops.
Watched PRs are checked on every refresh: the timed one (`refresh`) and `r`.
When CI passes or first fails, a review lands, conflicts appear or clear, an
otherwise ready PR falls behind a base it must be up to date with, or the
PR merges or closes, the status line says so and a desktop notification goes
out. A merged or closed PR leaves the board straight away. A watched row shows
`⚐` in the column left of its number, and `⚑` until you open it with `enter` or
`d`. Watches end when pile quits, so `q` asks first while any are running, in the same box as the 3+ confirm, listing the watched PRs.

Notifications default to OSC 9, which Ghostty, iTerm2, kitty and WezTerm show
with nothing to install. A terminal multiplexer may not pass OSC 9 through; use
`notify: command` with its own notifier instead. The command runs through
`sh -c` with the change in `$PILE_TITLE`, `$PILE_MESSAGE` (the PR title),
`$PILE_URL` and `$PILE_NUMBER`. Quote them, and keep them out of the command
text itself, since titles are written by other people.

```yaml
watch:
  notify: command          # osc (default) | command | none
  # herdr
  command: herdr notification show "$PILE_TITLE" --body "$PILE_MESSAGE"
  # macOS, outside a multiplexer
  # command: terminal-notifier -title "$PILE_TITLE" -message "$PILE_MESSAGE" -open "$PILE_URL"
```

An action only sees a selection if it says `multi: true`. Without it the action
keeps the singular fields (`{{.Number}}`, `{{.URL}}`, ...) and refuses when
several PRs are selected, rather than quietly running on whichever one came
first. A `multi` action gets the plural fields instead — `{{.Numbers}}`,
`{{.URLs}}`, `{{.Branches}}`, `{{.Bases}}`, `{{.Authors}}`, `{{.Titles}}` —
space-joined and individually quoted, and runs once. Mixing the two forms in
one template is an error, reported on the status line the first time you press
the action's key. With nothing selected, a `multi` action gets the row under
the cursor as a list of one.

Acting on three or more PRs at once asks first — both `enter`/`o` and any
`multi` action, in a box over the middle of the board. `y` or `enter` confirms,
any other key cancels and keeps the selection.

No actions are bound by default. The tool knows nothing about worktrees,
editors or multiplexers — it renders a template and runs what it gets — so a
default that shelled out to a script only the author has would fail with
`command not found` on anyone else's machine. The examples below are ready to
paste.

### Example: open a PR as a worktree workspace (Worktrunk + herdr)

```yaml
repos:
  - name: owner/name
    path: ~/Repos/owner/name    # the action refuses a repo without one

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

Action keys cannot shadow a built-in key, and duplicate action keys are rejected
when the configuration loads. Pick a key the table above does not use. `m` is
taken by watching, so a config that bound an action to `m` before watching
existed now fails to load; move that action to another key.

Action templates get `{{.Number}}`, `{{.Repo}}`, `{{.RepoPath}}`, `{{.Branch}}`, `{{.Base}}`, `{{.URL}}`, `{{.Author}}`, `{{.Title}}`.
GitHub-sourced string fields (`Branch`, `Base`, `URL`, `Author`, and `Title`)
are already POSIX-shell-quoted; use those placeholders directly, without adding
quotes around them. Each remote placeholder must be a standalone shell word;
quoted, embedded, command-substitution, and heredoc contexts are rejected when
the action is invoked. Explicit shell evaluators such as `sh -c` and `eval` are
also rejected when the template uses a remote field. `Repo` and `RepoPath` are
the PR's own repo's `name` and `path` from `repos`, so they are trusted config
text and remain shell text, as does the command itself, so expansions, pipes and
redirects written in `run` still work. An action that uses `{{.RepoPath}}`
refuses a PR whose repo has no `path`.

An action runs in its PR's checkout when that repo has a `path`, so
`tuicr pr {{.Number}}` reviews the right #12 whichever directory pile was
started from. Without a `path` it runs where pile was started. On a board with
several repos that would be a guess, so an action using a number, branch or base
refuses there unless it names the repo with `{{.Repo}}` or `{{.RepoPath}}`.
Numbers and branches repeat across repos, so a `multi` action using any of
those, or `{{.Repo}}`/`{{.RepoPath}}`, refuses a selection that spans repos;
one using only `{{.URLs}}`, `{{.Authors}}` or `{{.Titles}}` still runs.

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
| `ctrl+d` / `ctrl+u` | down / up half a page (`pgdn` / `pgup` move a full page) |
| `enter` / `o` | open in the default browser |
| `d` | detail for the selected PR: failing and running checks named, passing counted, plus conflict or a required update from base, unresolved comments, author, reviewer, size and branches |
| `y` | copy the PR url to the clipboard (`pbcopy`) |
| `m` | watch / stop watching |
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
inside it — `NEEDS MY REVIEW · 3 of 10`. A section header scrolls away with its
section, so this is the only place the cursor's section is named once you are
past the top of it.

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
| `esc` | cancel, restoring the cursor |
| `ctrl+c` | quit |

`ctrl+u` deliberately does not edit a live query; outside the prompt it keeps
its board/help meaning of moving half a page upward.
