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
| Author is per-rule (`author: true`), not global | A rule like `author:@me` is all one person, so the column is dead weight there. This is what gh-dash #464 asks for and what neomutt has done for decades. The search matches the initials only when the column is drawn, since it matches what is drawn. |
| Draft split out of the review slot | Draft is a lifecycle state, review is an outcome, and a draft PR has both. Showing draft in the review slot hid whether it was already approved. |
| `-review:approved` on "Needs my review" | Drops PRs somebody else already signed off, which is what makes it an action queue rather than a status list. Changes-requested PRs stay — those still want you. Note GitHub already drops a PR from `review-requested:@me` once *you* approve it. |
| `/` searches rather than filters | Filtering answered "which PRs match?" and lost the rows around the answer, which are most of why you were looking. Worse, it made highlighting impossible for anything it hid. The board now holds still and matches are filled where they sit. Fuzzy matching, ranking and the numeric special case all go with it: one substring rule over the row's own drawn text. `bubbles/textinput` stays rejected — it pulls an OS clipboard shell-out for a paste binding this prompt does not need. |
| The search matches only what is drawn | Every match has to be visibly highlighted, or the board marks a row and shows no reason. So the haystack is the rendered row: the padded number, the title as clipped to this width, the three author initials. The consequences are real and accepted — a full login matches nothing, a word past the clip point matches nothing, and the match set changes with the terminal width. |
| The checks overlay names failures and pending, counts passes | 47% of check contexts on the live board are skipped and 45% pass, against 4% failing — listing passes buries 7 signal lines under 24 on the worst PR. Pending checks are named instead of counted because they are 1–3 per PR and say *what* you are waiting on. The overlay also stopped dead-ending: 47% of PRs are in PENDING rollup with nothing failing, where it used to say "no failing checks". `checks-page.md` has the measurements. |
| Every row is exactly one line | Five commits tried to stabilise the scroll while rows had data-dependent heights, and each traded one symptom for another: a single keypress moved the board 0–3 lines depending on whether a neighbouring row carried a failing-check line and whether a section header was crossing the edge. Gate names moved to the `d` overlay and section names to a left gutter, so every line on the board is a row and one keypress scrolls at most one line — unconditionally, not by argument. `uniform-rows.md` has the measurements; `view-restructure.md` is the earlier study this overturned. |
| No chrome line above the list | A pinned section line with a `N of M` count was built and reverted. Bound to the top visible row it could not name the wrong section — but on a pane tall enough to show the whole board nothing scrolls, so it froze on the first section and read `MINE · 1 of 5` while the cursor was three sections away. It also restated the gutter label directly beneath it, since both resolved from the same `meta[start]`. The full name and the per-section count are not on the board; `l`/`h` jumps between sections. `section-layout.md` §13 has the reproduction and what I got wrong. |
| The gutter name is window-relative, and `╷` ≠ `│` | `firstInSection` was `i == 0` over the whole section, so a scrolled board left every row above the boundary unnamed — the gutter went blank exactly when you most needed to know where you were. With no chrome line above the list this is again the only thing naming the top row's section. `╷` is retained separately from `│`: "a section starts here" and "its start is above the window" are different facts, and conflating them would claim a boundary that is not there. |
| Scroll margin of 2 rows | lazygit ships 2, fzf migrated 0 → 3 in 2024, nnn/micro use 3; no tool surveyed migrated the other way. 2 is the smallest in the cluster and still guarantees the first line of the next PR is visible. Every implementation caps at half the viewport; none is proportional (helix rejected that on the record, #8403). |
| Sections keep their rows while refetching | Rebuilding from empty collapsed the board and re-expanded it section by section, shoving every row below each one that resolved. A row should move when a value changes, not because a request finished. |
| A resolved empty section collapses to one line | Reserving six blank rows for a section that *knows* it has nothing wasted most of a short pane. The reservation applies only while loading. |
| No CODEOWNERS path matching | GitHub already turns CODEOWNERS into team review requests, so `team-review-requested:` covers it for free. No per-PR file-list fetch. |
| Actions are shell templates | The tool knows nothing about worktrees, editors or multiplexers; it renders a template and runs what it gets. |
| `y` copies via `pbcopy`, not a clipboard library | The same trade `browser.Open` already makes with `open`: a pipe to a binary that ships with the OS, against a dependency and its transitive tree. `bubbles/textinput` was turned down partly *because* it drags in a clipboard shell-out for a paste binding we do not want — the objection was to the dependency carrying it, not to the shell-out, so doing it directly costs nothing new. It is a `var` so tests never touch the real clipboard. |
| Arc tab reuse built into `open` | Plain `open <url>` always spawns a new Arc tab, so opening the same PR twice buries the window in duplicates. Non-Arc browsers fall through to `open`. |

---

## 3. Visual spec

### 3.1 Row anatomy (FULL tier, 0-based cell offsets)

| # | field | offset | width | align | content |
|---|---|---|---|---|---|
| 0 | section | 0 | 8 | left | rule name uppercased on the section's first **visible** row, else blank (§3.5) |
| 0b | rule | 8 | 2 | — | ` ╷` where a section starts, ` │` where it continues (§3.5) |
| 1 | mark | 10 | 1 | — | `▌` when selected, else space |
| 2 | gutter | 11 | 1 | — | space |
| 3 | tree | 12 | 2 | left | `╭╴` `│ ` `╰╴` or two spaces |
| 4 | number | 14 | 6 | left | `#3248`, padded right |
| 5 | gutter | 20 | 1 | — | space |
| 6 | status | 21 | 7 | left | glyph cluster, §3.2 |
| 7 | gutter | 28 | 1 | — | space |
| 8 | title | 29 | **flex** | left | clipped with `…` on display width |
| 9 | gutter | — | 1 | — | space (only when author shown) |
| 10 | author | — | 3 | right | lowercase initials, only on rules with `author: true` |
| 11 | gutter | — | 1 | — | space |
| 12 | age | — | 3 | right | `now`, `2h`, `1d`, `3w`, `99+` |

**Exactly one column flexes: title.** Everything else is fixed, which is the
mechanical fix for "columns spread across the terminal" — slack has only one
place to go, so the status cluster is pinned at columns 21–27 at every width.

**Every row is exactly one line, always.** Row height does not depend on the
data. This is the invariant the whole viewport rests on: it is what makes one
keypress scroll the board by at most one line. See `uniform-rows.md`.

```
section = name(8)+rule(2) = 10
body  = mark(1)+gut(1)+tree(2)+number(6)+gut(1)+status(7)+gut(1)+gut(1)+age(3) = 23
titleWidth = max(0, terminalWidth - 33)            [FULL]
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
| `accent` | 4 | PR number, selection bar `▌`, search prompt |
| `hit` | 5, **background only** | the runes a search query matched |
| `muted` | unset fg + `Faint(true)` | tree glyphs, age, author, continuation lines, draft titles, `·`, counts, footer |
| `selBg` | 8, **background only** | selected-row fill |

`error` uses **1, not 9**: index 9 is "bright red", often a pale low-contrast
pink on light themes. Indices 7, 15 and 0 are never foregrounds.

`hit` takes **5** because it is the one semantic slot a row does not already
spend: 1 failing, 2 passing, 3 pending, 4 the number and the commit type, 6 the
section gutter. A search hit can therefore never be misread as a status, which
is the same argument that keeps the type tint out of red. It sets no
foreground, so the matched text keeps the terminal's own default and the fill
is correct on a light theme by construction rather than by measurement — the
reason it is themed rather than a fixed cube shade.

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

### 3.5 Section gutter

```
NEEDS M… │    #3239  ◐  ○   feat(webapp): PROJ-2038 show an "On hold" status…   19h
         │    #3234  ◐  ○   feat(api-service): PROJ-2037 refuse order plan wri… 19h
MY TEAM… ╷    #3211  ✗1 ✓   feat(api-service,schemas): PROJ-1880 build the XLR… 23h
         │    #3134  ✓  ✓   feat(webapp): PROJ-1418 enable the Trade-ups ent…    1d
```

The rule name, uppercased and clipped to 8 cells, on the section's first
**visible** row; blank on the rest; then a one-cell rule. Uppercase is the only
uppercased text on the board, which alone makes a section boundary identifiable
without reading it.

**The rule breaks at a boundary.** A section's first row draws `╷` U+2577 where
a continuation row draws `│`, so the rule visibly *starts* rather than running
through. This is what makes a boundary readable when the name fills all 8 cells
and is clipped (`NEEDS M…`), which is exactly the case where the name itself
cannot signal "new section" by shape. A blanked cell was tried and is too weak
to read as intentional beside a column of `│`.

**The name is computed against the visible window, not the board.** The top
visible row always carries its section's name. Before this, `firstInSection` was
`i == 0` over the whole section, so every row above a scrolled-off boundary was
anonymous — on a scrolled board the gutter went blank and the section you were
looking at was unnamed, which was the actual defect.

The top row keeps `│` when its section began above the window. The two glyphs
carry different facts and the board does not conflate them:

| glyph | name | meaning |
|---|---|---|
| `╷` | yes | a section starts on this row |
| `│` | yes | you are inside this section; its start is above the window |
| `│` | no | continuation |

### 3.5.1 No chrome line above the list

The board's first line is a row. A pinned `header` line carrying the full
section name and a `N of M` position was built and removed; the reasoning is
kept here because the idea is an obvious one to have again.

It was bound to the **top visible row**, to stop it naming a section whose rows
were not beneath it. That binding is correct and was not the problem. The
problem is that on a pane tall enough to show the whole board — ~40 rows, which
is what this board is usually read at — nothing scrolls, so the top visible row
is always the first row of the first section. The line read `MINE · 1 of 5` and
stayed there while the cursor moved through every section on the board. It was a
constant occupying the most prominent line on the screen, and the count, shaped
like a cursor position, was never one.

It also duplicated the gutter. Both were resolved from the same
`meta[start]`, so whenever the top row's section started on screen the line
above the list restated the label directly beneath it.

Neither cursor-binding nor showing the line conditionally rescues it.
Cursor-binding is redundant when the board fits, because every boundary is then
visible and the cursor's section is the nearest `╷` above it. A conditional line
changes the list height as you scroll, which is the bug class
`uniform-rows.md` exists to kill. See `section-layout.md` §13.

So the full name and the per-section count are **not on the board**. The gutter
answers which section a row is in; `l`/`h` jumps between sections. Chrome is one
line — the footer — and two while the search prompt is open.

Sections are a gutter rather than a full-width header band because **a header
line is not a row**: whenever one crossed the top edge, a single keypress moved
the board two lines instead of one. Measured deltas for the three candidate
layouts are in `uniform-rows.md` §4. With the gutter, every line on the board is
a row and the invariant is unconditional.

The cost is 10 cells of title width (every breakpoint in §3.8 moved up by 10 to
compensate) and the per-section count, which the band used to carry.

A section with no rows to show occupies exactly one line, drawn with the same
gutter: the spinner while loading, `—` once resolved and empty, the error text
on failure. While loading, a section with no rows yet reserves a placeholder
block (capped at 6 rows, and at `(height-2)/len(rules)`) so the skeleton starts
near its final height.

### 3.6 The PR detail overlay (`d`)

Check names are **not** in the list. The row carries the count (`✗6`), which
answers "is this broken" and "how broken"; the overlay answers "why", which is
what you want after deciding a PR is worth investigating, not while scanning.

The overlay **names what you can act on and counts what you cannot**:

| bucket | treatment | glyph |
|---|---|---|
| failing | named, one per line, unclipped | `✗` `error` |
| in progress | named, one per line | `◐` `attention`, name `muted` |
| passing | counted | `✓` `ok`, text `muted` |
| skipped | counted, in the same tally | — |

```
  #3230 chore: bump @types/send from 0.17.4 to 1.2.1

  ✗ build-push-image customer-portal
  ✗ build-push-image billing-service
  ✗ build-push-image web-client
  ✗ build-push-image webapp
  ◐ webapp_e2e
  ◐ web_client_e2e
  ✓ 9 passing, 12 skipped

  any key closes
```

Glyphs are the same `ciCell` vocabulary the rows use (§3.2), so nothing new has
to be learned to read this page.

Below the checks sits a **state block**: what is true about the PR itself rather
than about its CI. It is what makes this the detail page rather than the checks
page. The key was `c` while it was the checks page and is now `d` for detail,
which is what the page became; the checks are still the top of it.

```
  #3186 fix(api-service): PROJ-1951 free unit numbers when a plan is abandoned

  ✓ all 18 checks passing

  ! conflicted · 26 commits behind master
  ● 9 unresolved comments
  author    Carol Diaz (cdiaz88)
  review    ✓ Alice Chen (alicechen)
  size      +596 −45 · 7 files
  base       fix-PROJ-1839-reject-legacy-option
  branch   PROJ-1951-unit-number-integrity
  opened    6d ago · idle 1d

  any key closes
```

**A line with nothing to say is not drawn.** An all-green PR opened this morning
renders three lines, not eleven: the `base` line appears only when the base is
not the repo default (which is read from the repo, never assumed to be
`master`), the age line only once the PR has been open a day, and the conflict,
comment and review lines only when they have something to report. A green PR
does not earn a full page just because a full page exists.

`●` (`attention`) is the one glyph added to §3.2's vocabulary, for unresolved
review comments. It is filled where `○` "review required" is hollow: something
is *here*, rather than something has not happened yet. That line is the highest
-value addition on the page — measured on the live board, 6 of 50 PRs have
unresolved comments and on **5 of those 6 the row's review glyph does not say
`✗`**; two are drawn `✓ approved` with conversations still open.

There is no conflicting-file list, and there will not be one. GitHub exposes no
per-file conflict data in either API, and the only computable proxy (files both
branches touched) over-reports by 3× in the median case and 8× at worst against
a real `git merge-tree`. `N commits behind` is the honest answer to the question
underneath "which files conflict" — *how much work is this to fix?*
`pr-detail.md` §4 has the ground-truth measurements.

**Two of these lines arrive late.** `behindBy`, unresolved threads, the reviewer
names and the default branch come from a second request made on `d` press,
measured at 1.2–1.9 s. The overlay is drawn immediately from data the board
already holds, so `d` is instant and `d`-then-any-key works even if the request
never returns. A **skeleton line stands where those lines will land** — `⠹
checking for conflicts and open conversations` — so the common case resolves in
place instead of inserting a row and pushing the rest of the block down. It is
one line and not one per field: both fields are conditional (unresolved threads
exist on 6 of 50 PRs), so a placeholder each would draw rows that vanish on most
PRs. `pr-detail.md` §7.1 records the reversal of the original "absent, not a
placeholder" rule and why the user's report is the evidence against it. Fetching them upfront
would mean ~100 requests on a cold start, roughly doubling the 2–4 s the tool
commits to in §2.

Most late lines are simply **absent** until they arrive — a line that appears
disturbs less than one that changes under the eye. The **reviewer line is the
exception** and holds its place with a `…`: it is the one field requested
by name, and a reviewer that materialises a second later reads as the page
having been wrong rather than merely incomplete. The placeholder is static, not
the board's spinner — the overlay is a still page, and the only moving thing on
it would pull the eye to the least important line.

Responses are keyed by PR number, so one that lands after the cursor has moved
on files itself under the PR it describes. Nothing is cancelled: holding `j`
with `d` at each row leaves requests answering questions nobody is asking, which
is harmless and cheaper than threading cancellation through the command model.
A PR already answered is never re-asked within a session.

The state block is clipped **before** the checks block and never interleaved
with it. Drop order, first dropped first: head branch, age, base (when it was
going to say the default anyway), size, author, review, unresolved comments,
then the conflict line last. Only once the state block is entirely gone does the
existing clip eat the check list from the bottom. A PR with 8 failing checks
must not drop a failure to make room for its branch name.

Passing checks are counted rather than listed because on the live board **47% of
check contexts are skipped and 45% pass, against 4% failing** — a full list puts
7 signal lines under 24 on the worst PR and overflows the 20-row design target.
Pending checks *are* named because they are few (1–3 per PR, never more) and
specific: knowing it is `webapp_e2e` rather than `lint` is the difference
between ten minutes and thirty seconds. `checks-page.md` has the measurements
and the industry comparison.

The tally reconciles: named lines plus counted ones account for every check
considered, the way `gh pr checks` closes with its own tally. Without the
skipped count the numbers would not add up against GitHub's UI.

When nothing is failing and nothing is running, the count **is** the answer and
the overlay is one line — `✓ all 20 checks passing`.

**Overflow clips from the bottom** with an honest `… N more lines not shown`,
rather than scrolling. Scrolling would need keys meaning "move within the
overlay", which contradicts the one property that makes this a look rather than
a mode: any key closes it, and a movement key **closes it and moves in the same
keypress**. The `any key closes` footer always survives the clip.

Names are deduped per bucket and stripped of shard suffixes
(`apps_ci / webapp_e2e / Run E2E Tests (1, 5)`), ported from
`pr-status.jq`. Umbrella gates (`CI Gate`, `E2E Status`) are dropped from every
bucket because they only restate their children — which means the tally counts
the contexts *we consider*, typically one or two below GitHub's raw total.

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
guarded with a seen set. Tree glyphs are now never disturbed: the search does not
reorder or hide rows, so a chain stays contiguous and no `╰╴` can dangle.

### 3.8 Responsive tiers

| tier | width | columns | title width |
|---|---|---|---|
| FULL | ≥ 86 | section, mark, tree, number, status(7), title, [author], age | `w - 33` (`-4` with author) |
| MID | 70–85 | section, mark, tree, number, status(7), title | `w - 29` |
| NARROW | 50–69 | section, mark, tree, number, status(3), title | `w - 25` |
| below | < 50 | `terminal too narrow / (need 50 cols)` | — |

Every breakpoint is 10 columns higher than it was before the section gutter,
which is exactly the gutter's width — the title keeps the same readable floor at
every tier.

Drop order: **age and author first** (they change how urgent something feels but
never what you do), then the **review glyph** (the field most likely to be
re-derived by opening the PR anyway — CI and conflicts decide whether opening it
is even worth it). Mark, tree, number, CI, blocker and title are the irreducible
board.

Breakpoint arithmetic: FULL needs `w - 33 >= 53` → 86; MID needs `w - 29 >= 41`
→ 70; NARROW needs `w - 25 >= 25` → 50.

**Stability guarantee:** within a tier only the title's width changes; across
tiers columns 0–20 never move at all, so the left edge of the board is identical
from 50 to 400 columns.

### 3.9 Vertical budget

Chrome is **one line**: the footer. 20 rows − 1 chrome = 19 list rows, and **all
19 are PRs** — there are no header rows inside the list, no blank separators and
no continuation lines, so the list is 100% data at every board shape. Measured on
the live board in a 27-row pane: 26 PRs visible, against 16 under the layout
before the uniform-row restructure.

A second chrome line was tried above the list (§3.5.1) and reverted, so the row
it cost is back. Verified in a real 120×40 pane: the whole 29-row board plus the
footer, with room to spare.

The footer is one `muted` line, always: keybindings on the left, and on the
right the **cursor's section with its position in it** — `NEEDS MY REVIEW · 3 of
10` — plus the spinner cell. The repo yielded that slot: it is a constant the
user chose, while the section changes under every keypress, and the 8-cell
gutter can only draw `NEEDS M…`. It is bound to the **cursor**, never to the top
visible row: at 40 rows the board does not scroll, so a top-row binding is
frozen at the first section forever, which is exactly how the reverted sticky
line failed. `section-layout.md` §14 has the walk that verifies it changes on
every keypress. When the two fields do not both fit the keys clip and the name
survives whole. A status message *replaces* the keybinding text rather than
adding a row — the keybindings are the least urgent thing on the board. While the
search prompt is open it is a second chrome row between the list and the footer
and the body loses one more line. The body is padded to the full viewport height so
the prompt and footer stay pinned to the bottom edge instead of floating under a
short result set.

The overlays (`?` and `d`) replace the whole view, so they carry no chrome, with
one exception: **`?` scrolls**, and spends its bottom row saying so.

The legend is 45 lines and does not fit the pane the board is usually run in. It
used to clip, losing its *top* — the key list, which is the reason anyone opens
it. A two-column fold was tried and reverted: it was more layout code and it
still clipped on a short pane, so it bought nothing. The page is now one column
that scrolls with `j`/`k`, the arrows, `ctrl+d`/`ctrl+u`, page keys, and `g`/`G`.

Scrolling costs the old **"any key closes"** contract, because `j` and `k` now
mean something else. The rule is the inverse of a mode: the scroll keys scroll
and **everything else closes**, so a key the reader guesses at still leaves the
page. The bottom row says which keys close it and where in the legend you are —
`esc q ? close · j/k scroll` on the left, `9-45 of 45 · end` on the right.

**That row is drawn at every height**, including when the whole legend fits (it
reads `esc q ? close` and `all 45`). Drawing it only on overflow cost a real
bug: for a 45-line legend in a 45- or 46-row pane the page showed no affordance
at all and `j` was a silent no-op that still swallowed the key, which is
indistinguishable from "scrolling is broken" — at exactly the heights a
full-screen terminal reports. One shape at every height is worth the row, and
the closing keys need saying everywhere regardless, since scrolling took the
`any key closes` contract away at all heights, not just short ones.

**The detail overlay does not scroll and should not.** `fitChecks` clips it from
the bottom of a list already ranked by what you pressed `d` for, and its any-key
contract is load-bearing: `d` then `j` inspects a PR and carries on down the
board in one motion. Adding a scroll mode there would spend that motion to solve
a problem the ranking already handles.

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
| `limit` | int | page size, default 20. Applied as `first:N` on the search, so it truncates before `tree` groups anything |
| `tree` | bool | group stacked PRs into a chain. Chains are computed from the PRs *in that section*, so first-match-wins splits a stack into per-section sub-chains (§4.1) |
| `author` | bool | show the author's initials column |

Rules are fetched in parallel but revealed in order, so a slow rule near the top
stalls everything under it — cheap, high-value rules go first.

### 4.1 `tree` across section boundaries

`stacks()` derives a chain from the head/base refs of the PRs a section actually
claimed, not from the repo's true stack graph. Since first-match-wins routinely
splits one stack across two sections, this is the common case, not an edge case.
It degrades in a defined way:

| case | renders |
|---|---|
| whole chain in one section | full `╭╴ │ ╰╴` chain |
| chain split across sections | each section draws a correctly-closed sub-chain of what it holds |
| only the top of a stack present | no glyph — an ordinary row |
| a gap in the middle | the sub-chain below the gap groups; the PR above it falls out as a plain row |

The glyph sequence is always well-formed and a child is never orphaned from a
parent that is on screen. The honest reading of `╭╴` is "stacked on something in
this section", not "stacked" — the gap case is the one where a root is stacked
on something not shown and nothing says so.

Because `limit` is `first:N` on the search and results come back newest-first,
a limit that cuts a chain cuts it at the **root** (the oldest PR), leaving the
children to regroup into a valid but shorter chain. Tree rules therefore want a
limit above the number of PRs they match.

### Action

| field | type | meaning |
|---|---|---|
| `key` | string | bound key; matched *after* navigation keys so an action cannot shadow `j` |
| `name` | string | shown in the `?` overlay and as the status message |
| `run` | string | Go template, run via `sh -c` |
| `mode` | string | `background` (default) or `suspend` — suspend hands the terminal over for a TUI command and repaints on exit. Background actions report their lifecycle on the status line (§4.2) and are reaped rather than left as zombies. |

Template fields: `{{.Number}}` `{{.Repo}}` `{{.RepoPath}}` `{{.Branch}}`
`{{.Base}}` `{{.URL}}` `{{.Author}}` `{{.Title}}`.

**No action is bound by default.** Actions are shell templates and the tool
knows nothing about worktrees, editors or multiplexers, so a default that
shelled out to a script only the author has would fail with `command not found`
on every other machine — a bound key that is guaranteed to break is worse than
an unbound one. The worktree-workspace action is documented in the README as a
ready-to-paste example instead, which is the same call the starter config
already makes by shipping its `actions:` block commented out.

**A workspace, not a tab, and the primitive choice is what buys idempotency.**
`herdr tab create` puts the worktree in a tab of whatever workspace you happened
to be in, and it is not idempotent — the second press opened a second tab on the
same directory (`w33:tC` and `w33:tD`, same label). `herdr worktree open` is the
workspace-level primitive (it is what `hwt` calls) and it *is* idempotent:
measured at three consecutive calls returning the same workspace id, `w3B`. So
the action needs no dedupe logic of its own; picking the right primitive
replaced the wrapper that was working around the wrong one.

Verified end to end by pressing `w` three times on a real PR from the board:
worktree count held at 5, workspace count at 7, one workspace `w3C`
(`is_linked_worktree: true`), no error on the status line.

**`--no-focus`, because a keypress should not move you.** `hwt` passes `--focus`,
which is right when a human runs it from a shell and wrong for a key on a board
— it yanks the user out of what they were doing on every press. `herdr worktree
open` takes `--no-focus`, so the workspace is created in the background and is
there when wanted.

One failure mode worth knowing: `wt remove` deregisters a worktree but can leave
the directory on disk (a `node_modules` from a post-start hook is enough), and
the next `wt switch pr:<n>` then fails with *Directory already exists* instead of
reusing it. This bit during testing and looks exactly like the action silently
doing nothing, since a background action reports only its name.

**`background`, not `suspend`, for this one.** `suspend` hands the terminal over
via `tea.ExecProcess` and repaints on exit, which is right for a pager or a
review TUI. This command opens a herdr tab over a socket and exits — timed at
~4s with no terminal output — so suspending would blank the board to run
something that never wanted the terminal.

### 4.2 What a background action says while it runs

The first version started the process, put the action's name on the status line
and threw the result away — `go cmd.Wait()` with no receiver. So the footer said
`worktree` the instant the key was pressed whether the command took 7 seconds or
failed outright, and a failure was indistinguishable from a success. That is the
bug this section replaces.

A background action now reports three states on the status line:

| state | footer |
|---|---|
| running | `⠹ worktree`, the board's own spinner |
| succeeded | `worktree ✓`, cleared after 4s |
| failed | `worktree failed: <last line of stderr>`, until something replaces it |

**The running glyph animates, and that costs a tick.** `spinMsg` returns early
on a board that has finished fetching, so reusing it naively gives a frozen
glyph — which reads as wedged, the opposite of what the indicator is for. The
detail overlay hit this first and solved it by keeping the tick alive for the
duration (§7 of `pr-detail.md`); an action does the same, restarting the tick on
start when no other tick is live and letting it lapse when the run ends. A
static marker was the alternative and was rejected on the same evidence: the
complaint being answered is "is this thing doing anything", and a character that
never changes does not answer it. Measured live across a 6s action: seven
distinct frames, with `j`/`k` responsive throughout.

**Failure quotes the command, not Go.** `exit status 1` names the mechanism and
not the problem, while the script already wrote something better — `herdr not
running (no socket at ...)`. Stderr is captured into a 4KB tail buffer (bounded,
because a chatty command should not grow the board's memory) and the last
non-blank line becomes the message. A command that does not exist gets `sh`'s
own `command not found`, which is likewise more useful than the exit status.

It is **one line, clipped not wrapped**. Stderr is arbitrarily long and a second
footer row would break the one-line-per-row invariant that `uniform-rows.md`
establishes.

**Success expires, failure does not.** The status line otherwise persists until
something replaces it, which would have the footer claiming an action is current
long after it finished. Four seconds is long enough to read on looking back from
whatever the action opened. A failure is the one message the user has to act on,
so it stays. Only the run that set a success may expire it — a newer action, a
refresh or a copy owns the line by then.

**A second press while one is running is refused**, with `worktree still
running` rather than silence: starting a second process would orphan the first
one's result. Runs are numbered, so a result that lands after the user moved on
is dropped instead of overwriting the newer status, and a refresh drops the
running *indicator* without pretending the process died.

One failure mode worth knowing: `wt remove` deregisters a worktree but can leave
the directory on disk (a `node_modules` from a post-start hook is enough), and
the next `wt switch pr:<n>` then fails with *Directory already exists* instead of
reusing it. This used to look exactly like the action silently doing nothing;
the failure line now quotes what `wt` said.

### 4.3 Pass the branch, not just the number

`{{.Branch}}` exists because the board already knows the PR's head ref, and a
script that resolves the PR *number* pays for what the board could have told it.
Measured against a worktree that already existed:

| form | time |
|---|---|
| `wt switch pr:<n>` | 7.0s — resolves the number through the GitHub API on every call |
| `wt switch <branch>` | 0.07s — no network at all |

A 100x difference on a key pressed many times a day, and the API round trip buys
nothing when the worktree is already there. The branch form is not only the
existing-worktree fast path: it also *creates* one from `origin/<branch>` when
there is none, measured at 2.8s and still without an API call.

The number is still worth passing as a fallback. A branch resolves only if it is
on `origin`, so a PR from a fork — whose head lives on the contributor's remote
— needs `pr:<n>`, which is exactly the case where asking GitHub is the point.
Try the branch, fall back to the number: first use and forks keep working, and
the common case stops paying for them. Note when writing such a script that
`wt switch` on an unknown branch prints its error and still exits 0 when its
output is piped, so test the resolved path rather than the exit code.

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
| `g` `G` / home end | top / bottom. Documented as `g` rather than `gg`: bare `g` is the whole move, so advertising a chord that is one key repeated read as confusing. Typing `gg` still works — top is its own fixed point, so the second press lands in the same place, and there is no pending-key mode to wedge |
| `enter` `o` | open in browser (reuses an existing Arc tab) |
| `y` | copy the selected PR's url to the clipboard. `y` because the board already speaks vim (`j`/`k`, `g`/`G`, `l`/`h`), so yank is the key those fingers already reach for. A clipboard write is invisible, so it is acknowledged on the status line |
| `r` | reload |
| `/` | search |
| `n` `N` | next / previous match, wrapping |
| `d` | detail for the selected PR: failing and running checks named, passing counted, plus the state block (any key closes; a movement key closes *and* moves) |
| `?` | help and the glyph legend. It scrolls (`j`/`k`, arrows, `ctrl+d`/`ctrl+u`, page keys, `g`/`G`); `esc`, `q` and `?` close it, as does any key that is not a scroll key |
| `q` `esc` `ctrl+c` | quit |

**Searching.** `/` is vim's `/`. The board does not move — no row hidden, no
section hidden, nothing reordered, no stack glyph changed — and matches are
filled where they sit -- a background block, the way vim's `Search` group works.
A fill rather than an underline because the board spends foreground colour
everywhere, and an underline under a faint scope or a muted draft title was
missable, which is the one thing a search highlight may not be. The cursor previews the match as you
type (`incsearch`), `enter` keeps the query and the highlights (`hlsearch`),
`n`/`N` step through the matches with a wrap message, and `esc` on the board
clears them (`:noh`).

The query is a case-insensitive substring of the row's **rendered** text: the
padded number, the title as clipped to this width, and the author initials when
that column is drawn. The age cell is the one deliberate exclusion — it is
computed from the clock, so a match on it would expire with no input from the
user, and a bare digit would collide with the PR number.

`?` stays the help key, so there is no backwards-open and therefore no direction
flag: `n` is always forward, `N` always backward.

Every printable key is query text while typing, so navigation moves to chords:

| key | action |
|---|---|
| `ctrl+n` `ctrl+p` / `ctrl+j` `ctrl+k` / `↓` `↑` | next / previous match |
| `enter` | keep the query and the highlights, close the prompt |
| `backspace` / `ctrl+u` | edit / clear the query |
| `esc` | cancel, restoring the cursor to where `/` was pressed |
| `ctrl+c` | quit |

From either overlay, **only `ctrl+c` quits** — `esc` and `q` mean "back to the
board", so opening one can never cost you the session by reflex.

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

**Scroll behaviour — solved, and worth not re-litigating.** A constant row gap
under the cursor is a constraint on the **bottom** edge of the viewport: `end`
is fixed by the rows that must follow the cursor, and a fixed height then
determines the top as `start = end - height`. That value generally does not land
on a row boundary, and the leftover fraction of a row has to be absorbed
somewhere.

Eight attempts anchored the **top** to a row boundary and snapped the remainder
away; every one of them made the gap oscillate (measured: 0–3 rows where it
should be 2). Snapping down scrolls too far and the cursor rises; snapping up
and the cursor sinks or falls off. The constraint is genuinely unsatisfiable
that way — brute force found board states where the only viewport start giving
"2 rows below" is a non-boundary line.

The top row is therefore **clipped**. That is the degree of freedom that makes
the constraint satisfiable at all. A blank section separator is still skipped
(it is chrome, not content), but a clipped continuation line at the top is
expected and correct.

Two things this depends on, both easy to reintroduce as bugs: the bottom edge
must be measured past the **last line** of the margin row, not the start of the
row after it, or a two-line row there is only half-guaranteed; and the cursor's
own row needs an explicit guard against being clipped off the bottom.

Note the metric: the cursor's **screen line** still varies, because rows above
it differ in height. What is constant is the number of whole PR rows below it.

Evidence: `docs/scroll-feasibility.md` (a 5000-board fuzz put clipped-top at
1370/1370 exact against 6/1370 for the anchored version) and
`docs/view-restructure.md` (the drift tracks anchor spacing, not row heights —
so restructuring the layout would not have fixed it). `bubbles/list` cannot help
here: `ItemDelegate.Height()` takes no item, so variable row heights are
structurally impossible in it.

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
  this mine or someone else's". Those three characters are also the whole of
  what an author search can match, since they are the whole of what is drawn.
- The research also proposed a **cursor-seeded author filter** key
  (lazygit-style, filter to the author of the row under the cursor). Not built.
- The spec's drop order was age → review glyph; adding the author column made it
  age+author → review glyph, and author only exists at FULL tier.
- The spec asserts the FULL breakpoint must be recomputed once the author column
  costs 4 cells (it would push the FULL floor to 80). Fixed: `widthTierFor`
  starts FULL 4 columns later for a rule that shows an author, so the 53-cell
  title floor holds in both cases.
- The spec's §7.4 says the scroll offset must be computed in lines. The
  implementation anchors the viewport's bottom edge and clips the top row
  instead — see above. The spec is wrong, not the code.

**Untested against reality:** whether ~2–4s cold start is tolerable behind a
spinner in daily use; whether the board reads correctly on a light theme (the
`muted`/`selBg` reasoning is by construction, not observation).
