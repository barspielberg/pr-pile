---
title: Visual spec
description: Row geometry, status slots, color tokens, selection, stack glyphs and responsive tiers.
---

## 3.1 Row anatomy (FULL tier, 0-based cell offsets)

There are two kinds of line in the list, and both are **slots** — positions the
cursor can occupy (§3.5).

**PR row:**

| # | field | offset | width | align | content |
|---|---|---|---|---|---|
| 1 | mark | 0 | 1 | — | `▌` when selected, else space |
| 2 | sign | 1 | 1 | — | `•` when picked, else the watch flag (below), else space |
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

**Section header** — one per section, full width (§3.5):

| # | field | offset | width | align | content |
|---|---|---|---|---|---|
| 1 | mark | 0 | 1 | — | `▌` when selected, else space |
| 2 | name | 1 | flex | left | the rule name, **not** uppercased, **not** clipped to a fixed width |
| 3 | count | — | — | right | the section's row count, 2 cells from the right edge; blank while pending |

**Exactly one column flexes: title.** Everything else is fixed, which is the
mechanical fix for "columns spread across the terminal" — slack has only one
place to go, so the status cluster is pinned at columns 11–17 at every width.

**The sign column** (column 1) carries the user's own marks, like vim's: `•`
for a pick, else `⚐` U+2690 watched (`muted`) or `⚑` U+2691 something changed
you have not looked at (`attention`, cleared by `enter`/`o` or `d`). A pick
wins the cell: it is short-lived and acted on next, and the flag is back once
the selection clears. The flag is kept out of the status cluster and away from
the number because both were tried. After the number, one space short of CI, it
read as a seventh status glyph, its column shifted with the number's length,
and it pushed the cluster right on every row. A Nerd Font eye before the number
overhung its cell and left an uneven gap. Here the column is fixed, it scans as
one vertical strip, and an unwatched row is exactly what it was before
watching existed. While a watch's poll is newer than the board's last fetch,
the row's CI, review and blocker cells draw from the poll, so a row does not
say `◐` for minutes after the footer said CI passed.

**Every line is exactly one line, and every line is a slot.** Row height does
not depend on the data, and there is no line in the list the cursor cannot
occupy. Together those give the invariant the whole viewport rests on: one
keypress scrolls the board at most one line. See `uniform-rows.md` §4.1.

```
body  = mark(1)+gut(1)+tree(2)+number(6)+gut(1)+status(7)+gut(1)+gut(1)+age(3) = 23
titleWidth = max(0, terminalWidth - 23)            [FULL]
titleWidth -= 4 when the rule sets author: true    [FULL only]
```

All padding clamps at zero (`max(0, w - width(s))`) — never an unclamped
`strings.Repeat`; gh-dash shipped a `negative Repeat count` panic from exactly
this. When `titleWidth <= 3` the title renders empty rather than a bare `…`.
Number holds `#99999` (the board this was built against is at ~#3200); if one ever exceeds it, let it eat
the following gutter — never push the status cluster right.

## 3.2 Status cluster (7 cells, sub-offsets relative to column 11)

| sub-offset | width | dimension |
|---|---|---|
| +0 | 1 | CI state glyph |
| +1 | 1 | CI failing count digit, or space |
| +2 | 1 | gutter |
| +3 | 1 | review state glyph |
| +4 | 1 | gutter |
| +5 | 1 | blocker (conflict, draft or behind) |
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

**Blocker** — one cell, precedence **conflicts > draft > behind**: `!` when
`Mergeable == "CONFLICTING"` (`error`), `~` when `IsDraft` (`muted`), `↓`
U+2193 when `MergeState == "BEHIND"`, else space. A conflicted draft is
primarily conflicted — the conflict is what will bite — and a draft cannot
merge whether or not it is behind. `BEHIND` only happens where the repo
requires an up-to-date branch, so `↓` means "update before you can merge", not
"some commits landed on base". It is `attention` when being behind is the last
thing left (review not pending or refused, CI not failing) and `muted`
otherwise, so a busy board does not light up with arrows on PRs that will be
updated again anyway. `!` and `~` are ASCII; `↓` is the one exception, no
riskier than the `◐` and `○` already in the cluster. Draft *additionally* mutes
the whole title: a draft is by definition not actionable, so it recedes. The only
case where a boolean modifies the row.

Monochrome read-out, with all colour stripped:

```
✗2 ○ !    failing (2 checks), review required, conflicted
✓  ✓      passing, approved, clean
◐  ○      running, review required, clean
·  · ~    no CI, no review decision, draft
✓  ✓ ↓    passing, approved, must update from base to merge
```

## 3.3 Colour tokens

Nine tokens, **all** ANSI 0–15 or terminal-default, referenced symbolically at
every render site (`internal/ui/render.go`, `internal/ui/title.go`).

| token | value | used for |
|---|---|---|
| `fg` | unset (terminal default) | PR titles |
| `error` | 1 | `✗` CI, `✗` review, `!` conflicts, section fetch errors |
| `attention` | 3 | `◐` running, `○` review required |
| `ok` | 2 | `✓` passing, `✓` approved |
| `header` | 6 + bold | the section header row (§3.5) |
| `accent` | 4 | PR number, selection bar `▌`, search prompt — on selected **and** unselected rows alike |
| `type` | 4 | the conventional-commit type word (§3.3.1) |
| `hit` | `Reverse` — no colour at all | the runes a search query matched (§3.3.3) |
| `muted` | unset fg + `Faint(true)` | tree glyphs, age, **author**, scope, ticket, draft titles, `·`, counts, footer |
| `selBg` | 8, **background only** | the selected row and the selected header (§3.4) |

`error` uses **1, not 9**: index 9 is "bright red", often a pale low-contrast
pink on light themes. Indices 7, 15 and 0 are never foregrounds.

## 3.3.3 The search hit sets both ground and figure

A hit is drawn with **reverse video** (`SGR 7`): the fill takes the theme's
foreground and the text takes its background. No colour is chosen.

**The rule this encodes: a highlight must set both ground and figure.**
Inheriting the foreground is the bug, not a tuning problem. `hit` was
`Background(5)` alone, which left each run with whatever foreground it already
had — and a dark theme's ANSI 5 is a *light* pink by design, so every
foreground landed light-on-light. Measured in Catppuccin Mocha:

| a hit landing on | contrast |
|---|---|
| commit type / PR number (`accent` 4) | 1.38:1 |
| title (`fg`) | 1.06:1 |
| `muted` author | 1.03:1 |

…against a fill that was itself 10.74:1 on the board. A loud block with
invisible contents is worse than no highlight, because it draws the eye to the
one place it cannot read.

**Why reverse rather than a chosen pair of slots.** The contrast becomes
whatever the theme already guarantees for ordinary text — by definition its
best pairing — so it is correct on light and dark without measuring either.
Across eight themes it is the only candidate clearing 4.5:1 everywhere:

| highlight | worst case across 8 themes |
|---|---|
| `Background(5)`, no foreground | 1.06:1 |
| bg 5 + fg 0 | 2.37:1 |
| bg 3 + fg 0 | 2.39:1 |
| **reverse** | **4.75:1** |

Every fixed-slot pair collapses to about 2.4:1 on a light theme, where ANSI 5's
luminance flips relative to the ground. Reverse cannot flip, because it has no
fixed luminance to flip.

It also cannot clash. There is no hue to collide with the commit type's ANSI 4
(§3.3.1), so hue and hit remain the independent channels §3.3 requires: hue
says what kind of change, the fill says where the query landed.

**Why §2's objection to reverse does not reach a hit run.** §2 rejects reverse
for the *selected row*, because swapping a whole row's foreground into its
background destroys the status colour on exactly the failing rows that need it.
That is sound, and it does not apply here — for a structural reason rather than
a judgement call. `hitRuns` is applied to **three cells only**: the number, the
title and the author. The status glyphs (`✓ ✗ ◐ ○ !`) never pass through it and
are not searchable, so **a hit cannot recolour a status**. Those three cells are
also already surrendering their own styling deliberately — a hit takes the style
whole rather than composing — so there is no hue left for reverse to destroy.

**On the selected row** the hit does not inherit `selBg` either: the fill stays
the theme foreground, reading at 4.62:1 against `selBg` in Mocha. The highlight
is still visible on the cursor line, which is the case most likely to break.

Under `NO_COLOR` the fill disappears, as any background-channel highlight must.
A match stays locatable by the cursor, the prompt's `N of M` count and `n`/`N`,
so the feature degrades rather than breaking.

**There are no fixed 256-cube values left.** Every colour on the board is ANSI
0–15 or the terminal default, so the whole board resolves through the user's own
theme. Three cube values existed and all three are gone:

| was | used for | why it went |
|---|---|---|
| 237 | `selBg`, the selected-row fill | now ANSI 8. The cube grey was chosen because index 8 is *nominally* mid-grey and mid-grey is the worst backdrop for foreground colour. That holds where 8 really is mid-grey and fails where it is a tinted dark — Catppuccin Mocha renders it `#585b70` on a `#1e1e2e` ground. Neutral grey on a blue-tinted board also read as grafted on, which is what prompted the change. §3.4 has the ratios and the theme caveat |
| 234 | `headerBg`, the section header fill | **deleted, not replaced.** Against Mocha's background 234 measures **1.04:1** and 235 measures 1.08:1 — the band was invisible in the theme it was meant to serve. The label carries the header on its own: indentation, bold weight and ANSI 6's hue, at 11.01:1 (§3.5) |
| 75 | the accent while selected | now plain ANSI 4 on every row. The comment justifying 75 claimed "ANSI 4 measures 1.21 against `selBg`" — a figure computed against a *nominal* ANSI 4, not any real theme's. Against Mocha's `#89b4fa` it is **5.40:1** on the old 237 and **3.17:1** on ANSI 8, beating 75 (4.91 / 2.88) on both. It was protecting against nothing |

> **Two lessons worth keeping.** First, a contrast figure for an ANSI slot is
> meaningless without naming the theme it was measured in; the 1.21 above was
> wrong for five months because nobody asked which blue. Second, an audit that a
> later commit invalidates is worse than no audit, because it gets trusted — the
> commit collapsing the type palette closed by naming the fixed values left, and
> the next commit added `headerBg` without anyone noticing. This table is the
> maintained record; commit messages are not.

A fixed value is not banned in principle. [Design principles](../) §3 allows one where
a *measured* contrast requirement has no themed answer. There is currently no
such case, and `TestNoFixedCubeColoursAnywhereOnTheBoard` fails if one
reappears without this table changing first.

## 3.3.1 One colour for every commit type

Most titles on this board are conventional commits, so the type word is
coloured to separate the prefix from the subject a reader is actually scanning
for. **Every recognised type shares one colour** (`accent`, ANSI 4); the scope
and any Jira key are `muted`; the subject keeps the terminal default.

The eleven types used to carry five cube tints between them — `fix` terracotta,
`feat` sage, and a shared grey-blue for the types that are not release-visible.
That put the board's largest block of fixed colour on the field that says the
least. The type word is written out on the row: distinguishing `feat` from
`fix` by hue was decoration on a word that already reads, and it overrode five
slots of the user's theme to do it.

The hierarchy rule that chose the cube still stands — a type is not a status,
and `fix` on the same red as a failing check would make the row read as broken
— but it argued for keeping types *out of* the status colours, not for giving
each type its own. One themed colour clears the same bar.

Sharing ANSI 4 with the PR number is deliberate. Both are the row's structural
furniture — the number identifies it, the type classifies it — and they sit at
columns 14 and 29 with the status cluster between them, so they never abut and
differ in shape as well as position. Verified on the live board before it was
settled; ANSI 6 was the fallback if the two had read as confusable.

The **set** of recognised types is unchanged, and the colon is still required:
a title with no conventional prefix renders byte-for-byte as it did before
prefix colouring existed.

## 3.3.2 The author column is muted

The author renders `muted`, the same as age — the two columns right of the
title are one quiet tier, not two competing ones.

It used to hash the login into a fifteen-entry cube palette so the same person
was the same colour on every row. The initials were always the identity; the
hue was a grouping hint layered on a fact the three characters already carried,
which is the case §2 of [Design principles](../) names as the clean example of never
encoding anything in colour alone. Two commits went into tuning the palette
size — eight entries, then fifteen — and thirty-odd logins collide by
pigeonhole against any palette that fits a terminal, so no count ended it.

Colour is a budget. Who wrote a PR does not belong in the same register as
whether it is broken.

**Loud** (`error`, `attention`) is failing CI, changes requested, conflicts,
review required. **Quiet** (`ok`) is passing and approved — settled states need
to be confirmable, not noticeable. **Silent** (`muted`) is everything else. A
board where everything is fine has almost no colour in it; that is the point.

## 3.4 Selection

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

A **selected section header** takes the same fill, so the cursor is never
invisible; it is the only fill on the board, since the header has no band of its
own (§3.5).

### `selBg` is ANSI 8, and that is a deliberate trade

The fill resolves through the user's theme rather than being a fixed grey. In
Catppuccin Mocha — a `#1e1e2e` background with ANSI 8 at `#585b70` — every
foreground that can appear on a selected row:

| foreground | on ANSI 8 |
|---|---|
| title (default fg) | 4.62:1 |
| commit type + PR number (`accent` 4) | 3.17:1 |
| passing / approved (`ok` 2) | 4.49:1 |
| running / review required (`attention` 3) | 5.25:1 |
| **failing / conflict (`error` 1)** | **2.88:1** |
| **`muted` author and age** | **~2.54:1** |

The two below 3:1 are an accepted cost. Both clear 2:1, and status is glyph
shape first with colour as reinforcement (§3.2), so neither is the sole carrier
of anything. The `muted` figure is an **estimate**: SGR 2 has no specified blend
ratio and is modelled as ~55% foreground over the fill.

**The trade, stated plainly: this swaps a fixed-but-uniform result for a
themed-but-variable one.** Every other colour on the board already takes that
bet; `selBg` was deliberately exempt from it, and the exemption is what is being
given up. `accent` on the fill, across mainstream dark themes:

| theme | `accent` on `selBg` | `selBg` vs background |
|---|---|---|
| Tokyo Night | 3.55:1 | 1.91:1 |
| **Catppuccin Mocha** | **3.17:1** | **2.46:1** |
| Nord | 2.74:1 | 1.69:1 |
| One Dark | 2.56:1 | 2.32:1 |
| Dracula | 1.95:1 | 3.03:1 |
| Solarized Dark | 1.46:1 | 2.79:1 |
| Gruvbox Dark | 1.36:1 | 4.02:1 |

It measures well in Mocha and Tokyo Night and **poorly in Gruvbox (1.36:1) and
Solarized (1.46:1)**, where the accent on a selected row approaches the fill.
This board is developed and run against Mocha, so it is the right call here. **If
this tool is ever distributed more widely, that is the first row of the table to
look at** — and the fix would be a themed fallback or a config knob, not a
return to a fixed cube grey, which had its own failure (§3.3).

## 3.5 Section headers, and the cursor's address space

```
▌ Mine                                                                       5
   #3248  ✓  ○   fix(checkout): reach the settings panel before it settles  2h
   #3251  ◐  ○   feat(api-service): backfill the roster by materialized path  2h
  Involved                                                                   6
   #3199  ✓  ✗   fix(payment-gateway): stop double-sending the reminder     1d
```

Each section is announced by a **full-width header row**: the rule name at the
left, the section's row count right-aligned two cells from the edge, on a
background one step off the board ground. The name is neither uppercased nor
clipped to a fixed width, so `Needs my review` renders whole.

There is **no blank separator** between sections. The header's own background
does the separating, and §3.9 explains why the blank had to go.

**The cursor can sit on a header.** This is the load-bearing decision, not a
detail. The list's lines and the cursor's positions are the same set — called
*slots* in `internal/ui/model.go` — so moving the cursor one position moves the
board at most one line, arithmetically rather than by argument:

| slot kind | selectable | actionable | when |
|---|---|---|---|
| PR row | yes | yes | always |
| section header | yes | **no** | one per section, always |
| section note | yes | **no** | only when a section has no rows to show |

Headers and notes are **addressable but not actionable**. `enter`, `o`, `d`, `y`
and every user-configured action resolve through `selected()`, which returns
nothing on either, so they are silent no-ops there. Nothing needs a special
case: the absence of a PR is the guard.

`slots()` is the single source of truth for this sequence, and everything that
needs to know where the cursor is — the footer, `l`/`h`, `selected()` — indexes
into it rather than re-deriving the layout. That is not tidiness: the note slot
was added to `body()` first and `slots()` did not know about it, which shifted
every slot index after an empty section by one and had the footer naming the
wrong section. Two walks that must agree will eventually not.

`l`/`h` jump to a section's **header**, not to its first row — the header is
the section's first slot, and landing on the line that names where you arrived
is the point of the jump. Empty and still-loading sections are jump targets
too, which the gutter could not offer: they have a header whether or not they
have contents, so `l` tells you a section resolved empty instead of skipping
silently past it.

While **filtering**, `ctrl+n`/`ctrl+p` step between PR rows and never stop on a
header, and entering or editing a query re-seats the cursor on the first match.
A filtered board is a list of matches and a header is not one of them, so
stopping there would read as the motion having missed.

**Why this shape and not the alternatives.** A header that the cursor *cannot*
occupy costs exactly one line of scroll delta, as does a blank separator; two
independent implementations measured `3` for blank-plus-unselectable-header and
`2` for unselectable-header-alone, against `1` here. The general rule —
**every unselectable line in the list costs one to the worst-case delta** — and
the three passes it took to find it are recorded in `uniform-rows.md` §4.1.
The gutter this replaced is §4 of that document.

A section with no rows to show occupies its header plus exactly one **note**
line — the spinner while loading, `—` once resolved and empty, the error text on
failure — and that line is a slot like any other. It has to be: a note the
cursor could not reach was an unselectable line in the middle of the list, and
by the rule above it cost a line of delta. That was a real defect, shipped and
caught in review; `uniform-rows.md` §4.1 records it.

**There is no placeholder block.** A pending section used to reserve roughly its
final height in blank lines so that sections below it were not pushed down as it
resolved. Every one of those lines was unselectable *and* the count scaled with
the pane, so on a tall board one keypress moved the viewport several lines —
worse than the defect the uniform-row restructure existed to remove, and on
every cold start rather than in some edge case.

Losing it costs some layout stability while loading, and that is a real cost: a
row can now move once as the section above it resolves. The scroll invariant
outranks it. A row that moves when a value changes is permitted (§3.9); a board
that scrolls five lines per keypress is the bug five earlier attempts chased.

## 3.5.1 The header scrolls; it is not pinned

The distinction this section exists to preserve: §3.5's header sits **inside**
the list and scrolls with the section it names. A header **pinned** above the
list was built and removed, and the reasoning is kept because the idea is an
obvious one to have again — and because it is a different idea from the one
that shipped.

The pinned line carried the full section name and a `N of M` position, bound to
the **top visible row** so it could not name a section whose rows were not
beneath it. That binding is correct and was not the problem. The problem is
that on a pane tall enough to show the whole board — ~40 rows, which is what
this board is usually read at — nothing scrolls, so the top visible row is
always the first row of the first section. The line read `MINE · 1 of 5` and
stayed there while the cursor moved through every section on the board. It was
a constant occupying the most prominent line on the screen, and the count,
shaped like a cursor position, was never one.

Neither cursor-binding nor showing the line conditionally rescues it.
Cursor-binding is redundant when the board fits, because every boundary is then
visible. A conditional line changes the list height as you scroll, which is the
bug class `uniform-rows.md` exists to kill. See `section-layout.md` §13.

**An inline header avoids all of it by scrolling.** It cannot go stale, because
it is always directly above the rows it describes; it cannot be a constant,
because it leaves the viewport with its section; and it costs the list no height
it does not give back, because the cursor can stop on it (§3.5). The two designs
fail and succeed for unrelated reasons, so measurements about one say nothing
about the other.

Chrome is still one line — the footer — and two while filtering. The section
name and its count are now on the board, in the header, which is where the
pinned line was trying to put them.

## 3.6 The PR detail overlay (`d`)

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
  #3230 chore: bump yaml-parser from 0.17.4 to 1.2.1

  ✗ build-push-image partner-console
  ✗ build-push-image payment-gateway
  ✗ build-push-image web-viewer
  ✗ build-push-image portal
  ◐ portal_e2e
  ◐ web_viewer_e2e
  ✓ 9 passing, 12 skipped

  esc q d close · j/k scroll · / search
```

Glyphs are the same `ciCell` vocabulary the rows use (§3.2), so nothing new has
to be learned to read this page.

Below the checks sits a **state block**: what is true about the PR itself rather
than about its CI. It is what makes this the detail page rather than the checks
page. The key was `c` while it was the checks page and is now `d` for detail,
which is what the page became; the checks are still the top of it.

```
  #3186 fix(api-service): PROJ-1951 release the lock when a sync is cancelled

  ✓ all 18 checks passing

  ! conflicted · 26 commits behind master
  ● 9 unresolved comments
  author    Carol Diaz (cdiaz88)
  review    ✓ Alice Chen (alicechen)
  size      +596 −45 · 7 files
  base       fix-PROJ-1839-reject-stale-session
  branch   PROJ-1951-retry-budget-accounts
  opened    6d ago · idle 1d

  esc q d close · j/k scroll · / search
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
already holds, so `d` is instant and opening and scrolling works even if the request
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

**A PR in a GitHub native stack ends the page with a stack block**, last so
that its late half only grows the bottom of the page. Its header is from the
search, so it draws at once: `stack  #4215 · 8 PRs · on master`. The entries
come with the same second request, behind a `⠹ listing the stack` loader,
bottom first (position 1 to the top), merged and closed PRs included:

```
  stack     #2904 · 5 PRs · on master
    1  #2899  merged  [1/5] feat(checkout-ms): accept a one-time SAML code
    4  #2902  open    [4/5] test(checkout-e2e): stub the SAML code exchange
  ▌ 5  #2906  open    [5/5] refactor(checkout-e2e): rewrite the upstream stub
```

`▌` marks the PR the page is about. States are words, not glyphs: `✓` and `✗`
already mean CI and review everywhere else. An open PR that is not on the board
says `(not on board)`; merged and closed ones never are, so they do not. The
size counts merged PRs, as GitHub's own stack page does.

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
specific: knowing it is `portal_e2e` rather than `lint` is the difference
between ten minutes and thirty seconds. `checks-page.md` has the measurements
and the industry comparison.

The tally reconciles: named lines plus counted ones account for every check
considered, the way `gh pr checks` closes with its own tally. Without the
skipped count the numbers would not add up against GitHub's UI.

When nothing is failing and nothing is running, the count **is** the answer and
the overlay is one line — `✓ all 20 checks passing`.

**Overflow scrolls.** Detail uses the same scrolling and search keys as help.
Its bottom hint row remains visible, including when every line fits, so the
reader always knows how to move and leave the page.

Names are deduped per bucket and stripped of shard suffixes
(`main_ci / portal_e2e / Run E2E Tests (1, 5)`), ported from
`pr-status.jq`. Umbrella gates (`CI Gate`, `E2E Status`) are dropped from every
bucket because they only restate their children — which means the tally counts
the contexts *we consider*, typically one or two below GitHub's raw total.

## 3.7 Stacked-PR tree glyphs

Columns 2–3, exactly 2 cells, `muted`, at every width — **never dropped**, since
losing them would silently change the meaning of adjacent rows.

| position | glyph |
|---|---|
| first (base of stack) | `╭╴` U+256D U+2574 |
| middle | `│ ` U+2502 + space |
| last (top of stack) | `╰╴` U+2570 U+2574 |
| an end where a native stack goes on outside this section | `╎╴` U+254E U+2574 |
| a middle row with a PR of the stack missing just below it | `╎ ` U+254E + space |
| not stacked | two spaces |

The dashed `╎` is the one glyph a native stack adds (§4.1): the stroke is
broken where the chain leads to a PR this section does not show. It is `muted`
like the rest. There is no separate glyph family for a native stack against a
branch chain; the `d` page says which it is.

They cannot be confused with status or selection on three independent grounds:
glyph family (box-drawing vs dingbats), position (2–3 vs 11–17 vs 0), and
treatment (always faint; selection is a filled block, no tree glyph is filled).

A chain is a GitHub native stack, ordered by each PR's position in it; a PR in
no native stack falls back to one PR targeting another's head branch. Either
way a chain is emitted contiguously, root-first, where its root would sort. An unset base ref must not match an unset head ref, or every PR looks
stacked on every other and none is emitted as a root. A base/head cycle is
guarded with a seen set. Tree glyphs are now never disturbed: the search does not
reorder or hide rows, so a chain stays contiguous and no `╰╴` can dangle.

## 3.8 Responsive tiers

| tier | width | columns | title width |
|---|---|---|---|
| FULL | ≥ 76 | mark, tree, number, status(7), title, [author], age | `w - 23` (`-4` with author) |
| MID | 60–75 | mark, tree, number, status(7), title | `w - 19` |
| NARROW | 40–59 | mark, tree, number, status(3), title | `w - 15` |
| below | < 40 | `terminal too narrow / (need 40 cols)` | — |

Every breakpoint is 10 columns **lower** than it was under the section gutter,
which is exactly the gutter's width. The title keeps the same readable floor at
every tier and gains those 10 columns back, because the section name now lives
on its own header row instead of on every PR row (§3.5).

Drop order: **age and author first** (they change how urgent something feels but
never what you do), then the **review glyph** (the field most likely to be
re-derived by opening the PR anyway — CI and conflicts decide whether opening it
is even worth it). Mark, tree, number, CI, blocker and title are the irreducible
board.

Breakpoint arithmetic: FULL needs `w - 23 >= 53` → 76; MID needs `w - 19 >= 41`
→ 60; NARROW needs `w - 15 >= 25` → 40.

**Stability guarantee:** within a tier only the title's width changes; across
tiers columns 0–10 never move at all, so the left edge of the board is identical
from 40 to 400 columns. The section header is full-width at every tier and has
no columns to keep stable.

## 3.9 Vertical budget

Chrome is **one line**: the footer. 20 rows − 1 chrome = 19 list lines: one
header per section, one note for each section with nothing to show, and PRs for
the rest. There are no blank separators, no continuation lines and no
placeholder block, so **every line in the list is a slot the cursor can reach**
— which is what keeps the scroll rhythm even (§3.5).

The headers are the one deliberate spend of vertical space on the board, and
they are cheap in the way the blank separator was not: a header carries the
section's name and its count, and the cursor can rest on it, so it costs a line
and gives back both a label and a scroll delta of 1. A blank separator carried
nothing and cost a line *and* a line of delta — which is why there is none.
`uniform-rows.md` §4.1 has the measurement.

A second chrome line was tried above the list (§3.5.1) and reverted, so the row
it cost is back. Verified in a real 120×40 pane: the whole 29-row board plus the
footer, with room to spare.

The footer is one `muted` line, always: keybindings on the left, and on the
right the **cursor's section with its position in it** — `NEEDS MY REVIEW · 3 of
10` — plus the spinner cell. On a section header it drops the position and reads
`NEEDS MY REVIEW · 10`, the section's size: the cursor is *at* the section
rather than inside it, and a `1 of 10` there would be a position it does not
have. The repo yielded that slot, being a constant the user chose while the
section changes under every keypress. It is bound to the **cursor**, never to
the top visible row: at 40 rows the board does not scroll, so a top-row binding is
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

Scrolling costs the old **"esc q d close · j/k scroll · / search"** contract, because `j` and `k` now
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
`esc q d close · j/k scroll · / search` contract away at all heights, not just short ones.

The detail page now scrolls and searches like help; see [PR detail](../../guide/detail/) for its current key contract.

---
