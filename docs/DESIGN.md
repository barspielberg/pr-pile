# prs-mng — design

A terminal PR board. One keybind away in a Ghostty quick terminal, shows every
PR worth looking at with enough state to decide what to do, opens them, gets out
of the way. Go + Bubble Tea + lipgloss, single static binary, `gh` only for the
token.

Design target: a **20-row pane**, the user's own terminal theme (light or dark),
and "understand state in 2 seconds". That last one is the tiebreaker whenever
two goals conflict.

---

## 1. Core model

**Rules are the whole abstraction.** A rule is a name plus a GitHub search query;
the user lists rules in order, and a PR is shown under the **first** rule that
matches it. There are no built-in sections — "Mine" and "Needs my review" are
just the first two rules in the default config. Ordering *is* the configuration:
`involves:@me` would swallow your own PRs, except `author:@me` sits above it.
Reorder to change the meaning; no special-casing.

**Client-side bucketing, not one firehose.** GitHub does the narrowing it is good
at (one search per rule); merging and deduping happens in code, where boolean
logic is free. This exists because gh-dash could not express the bucket we
wanted: `review-requested:@me OR involves:@me` is not valid GitHub search syntax,
so it became two sections showing the same PRs twice.

**Parallel fetch, ordered reveal.** Every rule's query is fired concurrently, but
a section is only drawable once every section *above* it has resolved —
first-match-wins means an earlier rule can still claim a PR a later rule has
already fetched, so drawing it early would mean yanking it back out from under
the cursor. The board fills top-down; sections below the frontier show a spinner,
never an empty list or a fake "no open PRs". A failed rule claims nothing rather
than stalling everything below it. This still buys most of the felt-latency win,
because rule 1 is `author:@me` — the smallest, fastest query, and the one you
usually opened the tool for.

Code: `internal/board/board.go` (`Frontier`, `rebuild`), `internal/config/config.go` (`Rule`).

---

## 2. Decisions and why

| Decision | Why |
|---|---|
| GraphQL over HTTP, not the `gh` CLI | `gh search prs` cannot return `statusCheckRollup` / `reviewDecision` / `mergeable` — most of the board. `gh pr list` returns them but cannot express `review-requested:` / `team-review-requested:`. Neither subcommand alone works; shelling out also costs ~1.7s per call. |
| One search per rule, in parallel | A single combined query with aliased searches is *slower* — GitHub appears to run aliases serially, so one request pays the sum instead of the max. |
| No caching | Cold start is ~2–4s and the spinner is load-bearing. Revisit only if it grates. |
| Repo comes from config, not cwd | `pr-status.sh` inferred from cwd and silently showed the dotfiles repo. `PRS_MNG_REPO` overrides per-invocation. |
| Startup reachability check | GitHub's search index reports `issueCount 0` for a repo behind an org IP allow list, so being blocked was indistinguishable from having no PRs. A direct `repository(...)` query does error, so ask for one at startup. |
| ANSI 0–15 only, never hex or 256 | The 16 indices are an indirection layer, not a limitation: they resolve through the user's own theme. gh-dash shipped hex, got the bug report, and migrated (#770 / PR #771). Hex values of the same nominal colours do *not* get this (k9s #1234). |
| `muted` is `Faint(true)`, not index 8 | Index 8 is "bright black" — a *light* grey on many light themes, near-invisible on white. Faint is relative: SGR 2 dims whatever the theme's foreground already is, so it is correct on both by construction, and degrades to plain legible text where SGR 2 is ignored. Index 7 is avoided for the same class of reason. |
| Index 8 *is* used, as `selBg` | Its danger is as a foreground against index 0. As a background it sits between 0 and 7 in luminance in every mainstream scheme, so it contrasts with the terminal's own background at either end. |
| Selection = background fill + `▌` bar, not `reverse`, never dim | `reverse` swaps the row's foreground into the background, destroying status colour on exactly the failing rows that need it. Dim reads as "disabled" (lazygit #1845, #802). The bar lives in column 0 where nothing else ever draws, so selection survives a theme that eats the fill. |
| Status is glyph shape first, colour second | Red/green is both the commonest status pairing and the commonest colourblindness. Four distinct silhouettes (`✓ ✗ ◐ ○`) survive monochrome; the documented failure mode is several states rendering as one `•` in different hues. |
| No column header row | Our sub-slots are 1–2 cells — no word fits, so the only renderable header over `✗2 ○ !` is more symbols. gh-dash has a header row and its status headers *are* glyphs. Evidence: one uncommented legend request in five years across three trackers, versus repeated multi-user complaints about chrome rows. The legend lives in the `?` overlay instead, rendered from the same helpers the rows use so it cannot drift. |
| No global header row | Its three payloads were the app name (you just pressed a key), the repo (constant in a single-repo tool) and the spinner. Repo and spinner moved to the footer, which was already being paid for. One row back in a 20-row pane. (gh-dash #671: a 2-line logo drew "wasting space", "attention hogging".) |
| Plain Unicode BMP glyphs, not Nerd Font PUA | gh-dash #290: icons vanished on Nerd Font 3.0.2 — codepoints moved between font versions. Having "a Nerd Font" does not guarantee a specific glyph. No legibility gain at single-cell size. |
| Author is per-rule (`author: true`), not global | A rule like `author:@me` is all one person, so the column is dead weight there. This is what gh-dash #464 asks for and what neomutt has done for decades. Author is in the fuzzy haystack regardless of whether the column is shown. |
| Draft split out of the review slot | Draft is a lifecycle state, review is an outcome, and a draft PR has both. Showing draft in the review slot hid whether it was already approved. |
| `-review:approved` on "Needs my review" | Drops PRs somebody else already signed off, which is what makes it an action queue rather than a status list. Changes-requested PRs stay — those still want you. Note GitHub already drops a PR from `review-requested:@me` once *you* approve it. |
| Numeric filter queries are exact, not fuzzy | Fuzzy treated `3248` as a subsequence, so it also hit titles containing 3…2…4…8 scattered across them. Bare numbers now match PR numbers by prefix. |
| `sahilm/fuzzy`, not fzf's matcher | fzf ranks better but drags in tcell, go-colorful and a shellwords parser for one function. `bubbles/textinput` was also rejected: it pulls an OS clipboard shell-out for a paste binding this prompt does not need. |
| Scroll margin of 2 lines | lazygit ships 2, fzf migrated 0 → 3 in 2024, nnn/micro use 3; no tool surveyed migrated the other way. 2 is the smallest in the cluster and still guarantees the first line of the next PR is visible. Every implementation caps at half the viewport; none is proportional (helix rejected that on the record, #8403). |
| Sections keep their rows while refetching | Rebuilding from empty collapsed the board and re-expanded it section by section, shoving every row below each one that resolved. A row should move when a value changes, not because a request finished. |
| A resolved empty section collapses to one line | Reserving six blank rows for a section that *knows* it has nothing wasted most of a short pane. The reservation applies only while loading. |
| No CODEOWNERS path matching | GitHub already turns CODEOWNERS into team review requests, so `team-review-requested:` covers it for free. No per-PR file-list fetch. |
| Actions are shell templates | The tool knows nothing about worktrees, editors or multiplexers; it renders a template and runs what it gets. |
| Arc tab reuse built into `open` | Plain `open <url>` always spawns a new Arc tab, so opening the same PR twice buries the window in duplicates. Non-Arc browsers fall through to `open`. |

---

## 3. Visual spec

### 3.1 Row anatomy (FULL tier, 0-based cell offsets)

| # | field | offset | width | align | content |
|---|---|---|---|---|---|
| 1 | mark | 0 | 1 | — | `▌` when selected, else space |
| 2 | gutter | 1 | 1 | — | space |
| 3 | tree | 2 | 2 | left | `╭╴` `│ ` `╰╴` or two spaces |
| 4 | number | 4 | 6 | left | `#3248`, padded right |
| 5 | gutter | 10 | 1 | — | space |
| 6 | status | 11 | 7 | left | glyph cluster, §3.2 |
| 7 | gutter | 18 | 1 | — | space |
| 8 | title | 19 | **flex** | left | clipped with `…` on display width |
| 9 | gutter | — | 1 | — | space (only when author shown) |
| 10 | author | — | 3 | right | lowercase initials, only on rules with `author: true` |
| 11 | gutter | — | 1 | — | space |
| 12 | age | — | 3 | right | `now`, `2h`, `1d`, `3w`, `99+` |

**Exactly one column flexes: title.** Everything else is fixed, which is the
mechanical fix for "columns spread across the terminal" — slack has only one
place to go, so the status cluster is pinned at columns 11–17 at every width.

```
fixed = mark(1)+gut(1)+tree(2)+number(6)+gut(1)+status(7)+gut(1)+gut(1)+age(3) = 23
titleWidth = max(0, terminalWidth - 23)            [FULL]
titleWidth -= 4 when the rule sets author: true    [FULL only]
```

All padding clamps at zero (`max(0, w - width(s))`) — never an unclamped
`strings.Repeat`; gh-dash shipped a `negative Repeat count` panic from exactly
this. When `titleWidth <= 3` the title renders empty rather than a bare `…`.
Number holds `#99999` (the board this was built against is at ~#3200); if one ever exceeds it, let it eat
the following gutter — never push the status cluster right.

### 3.2 Status cluster (7 cells, sub-offsets relative to column 11)

| sub-offset | width | dimension |
|---|---|---|
| +0 | 1 | CI state glyph |
| +1 | 1 | CI failing count digit, or space |
| +2 | 1 | gutter |
| +3 | 1 | review state glyph |
| +4 | 1 | gutter |
| +5 | 1 | blocker (conflict or draft) |
| +6 | 1 | gutter / reserved |

Every sub-slot is always emitted, blank when absent, so nothing ever shifts as
state changes.

**CI** — `✓` U+2713 passing (`ok`) · `✗N` U+2717 failing, `N` is 1–9 and `+` at
≥10 (`error`) · `◐` U+25D0 running (`attention`) · `·` U+00B7 none (`muted`).
The failing count is the only numeral in the cluster, so red rows are scannable
by shape alone.

**Review** — `✓` approved (`ok`) · `✗` changes requested (`error`) · `○` U+25CB
review required (`attention`) · `·` none (`muted`). Reusing `✓`/`✗` is
deliberate: same meaning, different column, and position disambiguates. `○` is
the one hollow glyph — "not yet done" without a convention to learn.

**Blocker** — one cell, precedence **conflicts > draft**: `!` when
`Mergeable == "CONFLICTING"` (`error`), `~` when `IsDraft` (`muted`), else
space. A conflicted draft is primarily conflicted — the conflict is what will
bite. Both are ASCII so no font can break this slot. Draft *additionally* mutes
the whole title: a draft is by definition not actionable, so it recedes. The only
case where a boolean modifies the row.

Monochrome read-out, with all colour stripped:

```
✗2 ○ !    failing (2 checks), review required, conflicted
✓  ✓      passing, approved, clean
◐  ○      running, review required, clean
·  · ~    no CI, no review decision, draft
```

### 3.3 Colour tokens

Seven tokens, all ANSI 0–15 or terminal-default, referenced symbolically at every
render site (`internal/ui/render.go`).

| token | value | used for |
|---|---|---|
| `fg` | unset (terminal default) | PR titles |
| `error` | 1 | `✗` CI, `✗` review, `!` conflicts, section fetch errors |
| `attention` | 3 | `◐` running, `○` review required |
| `ok` | 2 | `✓` passing, `✓` approved |
| `header` | 6 + bold | section name and `━` rule |
| `accent` | 4 | PR number, selection bar `▌`, filter prompt |
| `muted` | unset fg + `Faint(true)` | tree glyphs, age, author, continuation lines, draft titles, `·`, counts, footer |
| `selBg` | 8, **background only** | selected-row fill |

`error` uses **1, not 9**: index 9 is "bright red", often a pale low-contrast
pink on light themes. Indices 7, 15 and 0 are never foregrounds.

**Loud** (`error`, `attention`) is failing CI, changes requested, conflicts,
review required. **Quiet** (`ok`) is passing and approved — settled states need
to be confirmable, not noticeable. **Silent** (`muted`) is everything else. A
board where everything is fine has almost no colour in it; that is the point.

### 3.4 Selection

- Column 0: `▌` U+258C in `accent`.
- Columns 1..end: `selBg` background, **filled to the terminal's right edge** — a
  partial fill reads as a highlight artifact, not a selection.
- Title bold; every other cell keeps its own semantic foreground on top of the fill.
- **No foreground is forced**: the row inherits the terminal default, which is
  guaranteed to contrast with the terminal's own background. Background detection
  is unreliable, so first paint must be correct without it.

Apply the background per-segment, not wrapped around the finished line: each
segment's own style emits a reset, which would terminate an outer background
part-way along the row.

### 3.5 Section header

```
━━ MINE ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 5
```

`━━ ` + NAME uppercased + ` ` + `━` fill to the edge + ` ` + count, full terminal
width. Uppercase is the only uppercased text on the board, which alone makes
headers identifiable without reading them. The count is right-aligned where the
eye already stops; it becomes the spinner frame while loading and `!` on failure.

A blank line precedes every header except the first — the only pure-whitespace
row in the design and the only section separator. While loading, a section with
no rows yet reserves a placeholder block (capped at 6 rows, and at
`(height-2)/len(rules)`) so the skeleton starts near its final height. Once
resolved and empty it collapses to a single `muted` `—`.

### 3.6 Continuation line (failing gate names)

```
  │                └ unit-tests / node-20, e2e / checkout-flows
   ^tree spine      ^aligned to the title column
```

Indented to the title column so it reads as a child of the title, prefixed `└ `
(a third glyph family, distinct from status and from selection), rendered
`muted` — the red `✗2` already says "this is broken"; this says *which*, which is
detail, not alarm. That is what stops it competing with titles. Multiple gates
join with `, ` on one line, clipped with `…`; never a second line, never wrapped.
Inside a stack, columns 2–3 carry `│ ` so the spine is unbroken. Always shown
when `FailedGates` is non-empty, at every pane height.

Gate names are deduped and stripped of shard suffixes
(`apps_ci / webapp_e2e / Run E2E Tests (1, 5)`), ported from `pr-status.jq`.

### 3.7 Stacked-PR tree glyphs

Columns 2–3, exactly 2 cells, `muted`, at every width — **never dropped**, since
losing them would silently change the meaning of adjacent rows.

| position | glyph |
|---|---|
| first (base of stack) | `╭╴` U+256D U+2574 |
| middle | `│ ` U+2502 + space |
| last (top of stack) | `╰╴` U+2570 U+2574 |
| not stacked | two spaces |

They cannot be confused with status or selection on three independent grounds:
glyph family (box-drawing vs dingbats), position (2–3 vs 11–17 vs 0), and
treatment (always faint; selection is a filled block, no tree glyph is filled).

A chain is one PR targeting another's head branch, emitted contiguously
root-first. An unset base ref must not match an unset head ref, or every PR looks
stacked on every other and none is emitted as a root. A base/head cycle is
guarded with a seen set. **Filtered rows lose their tree glyphs**: a chain is
almost never contiguous once filtered, and a dangling `╰╴` would draw a spine to
a row no longer above it.

### 3.8 Responsive tiers

| tier | width | columns | title width |
|---|---|---|---|
| FULL | ≥ 76 | mark, tree, number, status(7), title, [author], age | `w - 23` (`-4` with author) |
| MID | 60–75 | mark, tree, number, status(7), title | `w - 19` |
| NARROW | 40–59 | mark, tree, number, status(3), title | `w - 15` |
| below | < 40 | `terminal too narrow / (need 40 cols)` | — |

Drop order: **age and author first** (they change how urgent something feels but
never what you do), then the **review glyph** (the field most likely to be
re-derived by opening the PR anyway — CI and conflicts decide whether opening it
is even worth it). Mark, tree, number, CI, blocker and title are the irreducible
board.

Breakpoint arithmetic: FULL needs `w - 23 >= 53` → 76; MID needs `w - 19 >= 41`
→ 60; NARROW needs `w - 15 >= 25` → 40.

**Stability guarantee:** within a tier only the title's width changes; across
tiers columns 0–10 never move at all, so the left edge of the board is identical
from 40 to 400 columns.

### 3.9 Vertical budget

20 rows − 1 footer = 19 list rows. With three sections and one failing PR that is
3 headers + 2 blanks + 14 data rows, **74% data**. The footer is one `muted`
line, always: keybindings on the left, repo + spinner cell right-aligned. A
status message *replaces* the keybinding text rather than adding a row — the
keybindings are the least urgent thing on the board. While filtering, the prompt
is a second chrome row above the footer and the body loses one line. The body is
padded to the full viewport height so the footer stays pinned to the bottom edge
instead of floating under a short result set.

---

## 4. Config reference

`~/.config/prs-mng/config.yml`, honouring `$XDG_CONFIG_HOME`, overridable with
`$PRS_MNG_CONFIG`. XDG rather than `os.UserConfigDir`, which on macOS returns
`~/Library/Application Support` — not where a terminal tool's config belongs.
First run writes a commented starter config rather than failing, with the repo
inferred via `gh repo view`. Config decodes *over* the defaults, so an absent key
keeps its default; an explicit `rules:` list replaces them wholesale.

### Config

| field | type | meaning |
|---|---|---|
| `repo` | `owner/name` | required; the one value that cannot be guessed. `PRS_MNG_REPO` overrides it per invocation. |
| `repoPath` | path | local checkout, for `{{.RepoPath}}` in action templates |
| `refresh` | duration | auto-refresh interval, default `3m`; `<= 0` disables |
| `rules` | `[]Rule` | ordered; at least one required |
| `actions` | `[]Action` | key-bound shell commands |

### Rule

| field | type | meaning |
|---|---|---|
| `name` | string | required; the section header, uppercased on render |
| `query` | string | required; GitHub search syntax. Scoped automatically: `repo:<repo> is:pr is:open <query>` — a rule carries only what distinguishes it. |
| `limit` | int | page size, default 20 |
| `tree` | bool | group stacked PRs into a chain |
| `author` | bool | show the author's initials column |

Rules are fetched in parallel but revealed in order, so a slow rule near the top
stalls everything under it — cheap, high-value rules go first.

### Action

| field | type | meaning |
|---|---|---|
| `key` | string | bound key; matched *after* navigation keys so an action cannot shadow `j` |
| `name` | string | shown in the `?` overlay and as the status message |
| `run` | string | Go template, run via `sh -c` |
| `mode` | string | `background` (default) or `suspend` — suspend hands the terminal over for a TUI command and repaints on exit. Background actions are reaped so they do not become zombies. |

Template fields: `{{.Number}}` `{{.Repo}}` `{{.RepoPath}}` `{{.Branch}}`
`{{.Base}}` `{{.URL}}` `{{.Author}}` `{{.Title}}`.

### Default rules

```yaml
- {name: Mine,            query: "author:@me",                       limit: 50, tree: true}
- {name: Needs my review, query: "review-requested:@me -review:approved", limit: 50, author: true}
- {name: Involved,        query: "involves:@me -author:@me",         limit: 20, author: true}
- {name: All open,        query: "draft:false",                      limit: 20, author: true}
```

---

## 5. Keys

| key | action |
|---|---|
| `j` `k` / `↓` `↑` | move |
| `l` `h` / `→` `←` | next / previous section (empty sections skipped; `h` goes to the start of the current section first, then back) |
| `g` `G` / home end | top / bottom |
| `enter` `o` | open in browser (reuses an existing Arc tab) |
| `r` | reload |
| `/` | filter |
| `?` | help + glyph legend |
| `q` `esc` `ctrl+c` | quit |

**Filtering.** `/` opens a fuzzy prompt over number, author and title. Rows rank
by match score, so the board's newest-first order does not hold while filtering;
matched characters are underlined (weight, not hue — the title already spends
colour on draft and may sit on the selection fill); sections with no matches are
hidden header and all. A bare number matches PR numbers by prefix, not fuzzily.

Every printable key is query text while filtering, so navigation moves to chords:

| key | action |
|---|---|
| `ctrl+n` `ctrl+p` / `ctrl+j` `ctrl+k` / `↓` `↑` | move |
| `enter` | open and leave the filter |
| `backspace` / `ctrl+u` | edit / clear the query |
| `esc` | leave, restoring the full board |
| `ctrl+c` | quit |

From the help overlay, **only `ctrl+c` quits** — `esc` and `q` mean "back to the
board", so opening help can never cost you the session by reflex.

---

## 6. Measured facts

Against `acme/monorepo`, Sep 2026. Run-to-run variance is large (the same
query ranged 1.4s–4.1s); these are order-of-magnitude.

| measurement | value |
|---|---|
| cold start, 6 rules, parallel GraphQL | **~2–4s** (1.9 / 3.3 / 4.2s observed) |
| 1 combined GraphQL request, 6 aliased searches | **3.5–7.7s** — slower; aliases appear to run serially |
| `gh api rate_limit` (process startup + auth) | **1.7–2.4s** |
| the same call over raw curl, warm | **~0.45s** |
| `mine` rule alone | **1.4s** vs 3.1s for `allopen` |
| varying the row cap (5 / 10 / 20 / 50) | **no effect** — limit=5 took 5.7s, limit=20 took 2.9s in the same batch |

Latency is per-request overhead and search-backend variance, **not payload
size**, so capping rows is not a speed lever — set limits for display sanity.

Scroll-margin defaults across surveyed tools: lazygit 2, fzf 3 (was 0 until
v0.53), nnn 3, micro 3, LazyVim 4, vim `defaults.vim` 5, helix 5, yazi 5,
ranger 8, kickstart 10; gh-dash / k9s / htop / btop / `bubbles/viewport` have
none. Modal value 3. Every implementation caps at half the viewport height; none
is proportional.

---

## 7. Known problems and open questions

**Scroll behaviour is unresolved.** The cursor's distance from the bottom drifts
as you move. Two causes, both structural:

- The margin is measured in **lines**, but a section header is 2 lines (blank +
  header) that behaves as one unit, and a row is 1 or 2 lines (failing-gate
  continuation). A row-count-derived margin and a line-based viewport cannot
  agree.
- `window()` slices a flat line list at a line offset chosen from an **anchor**
  list (section headers and row first-lines), picking the earliest anchor that
  still honours the margin. That keeps the board still and guarantees the
  viewport never opens mid-row or on a blank separator, but it means the offset
  is rounded to whole units, so the cursor's distance from the bottom rounds with
  it.

Near the end of the list nothing satisfies the margin and `window()` falls back
to the last anchor showing the whole selected row, which can produce a short
final page. That beats starting mid-row, but it is a compromise, not a fix. The
current code (`internal/ui/model.go`) is stateless — offset is derived from the
cursor each frame, so resize needs no handling — and that property is worth
keeping through any rework. (`window()` also carries three duplicated copies of
its own doc comment, left over from successive rewrites.)

**`limit` is applied before dedup.** A rule's `limit` caps the GraphQL page size,
but first-match-wins removes already-claimed PRs *after* the fetch. A capped rule
under-fills: a `limit: 20` rule whose first 8 results were claimed above shows 12
rows, not 20.

**Review progress is deferred.** Who has approved / requested changes / is still
pending, unresolved thread counts, and "is this waiting on me or on them" — the
main gap versus the old `pr-status.sh` board. Deferred because it is the one
thing `gh pr list` does not give us, so it drags in an extra GraphQL query or a
per-row lazy fetch.

**Not built:** detail/preview pane; write actions (approve, comment, re-run CI,
mark ready) — the action model should not make them awkward to add; multi-repo
(config takes a repo, not a list — do not bake the assumption deeper); a
`glyphs = "ascii"` escape hatch (specified, cheap because widths are already
fixed); a config key for the scroll margin (hardcoded on purpose — there is one
user).

**Spec/implementation divergences**, stated rather than smoothed over:

- The research recommended board-scoped **disambiguation** of author initials and
  blanking the cells in your own section. The implementation is simpler: the
  first 3 lowercase characters of the login, shown only on rules with
  `author: true`. Collisions are possible and tolerated — the column answers "is
  this mine or someone else's", and the filter matches the full login for
  anything more precise.
- The research also proposed a **cursor-seeded author filter** key
  (lazygit-style, filter to the author of the row under the cursor). Not built.
- The spec's drop order was age → review glyph; adding the author column made it
  age+author → review glyph, and author only exists at FULL tier.
- The spec asserts the FULL breakpoint must be recomputed once the author column
  costs 4 cells (it would push the FULL floor to 80). The code kept **76** and
  subtracts the 4 cells from the title instead. The 53-cell title floor is
  therefore not met at 76–79 columns on author-enabled rules. Unresolved; the
  research flagged it and the spec never re-derived the table.
- The spec's §7.4 says the scroll offset must be computed in lines. The
  implementation deliberately anchors to whole units instead — see above.

**Untested against reality:** whether ~2–4s cold start is tolerable behind a
spinner in daily use; whether the board reads correctly on a light theme (the
`muted`/`selBg` reasoning is by construction, not observation).
