# prs-mng — the uniform-row restructure

Research and implementation record, written in response to:

> *"we did a lot of work trying to fix the jumping cursor when moving up and
> down. I think it's time for a big restructure. I think each row should be in
> the same line size, errors should be only displayed on pressing a key and gone
> when moving again. Headers should be maybe just a pointer in the side?"*

Researched 2026-09-17, against the shipped code at `4b5e6f6` and against a live
board captured from `acme/monorepo`.

This document supersedes the recommendation in `view-restructure.md` §1. That
study was right about the mechanism it measured and wrong about what to do,
for two reasons it could not have known at the time. Both are recorded in §2.

---

## 1. Recommendation

**Restructure. Make every row exactly one line.** The user's instinct is
correct, and the earlier "fix the scroll code, not the layout" recommendation
has now been tried — twice — and has not held.

Specifically:

1. **Every row is one line.** The failing-gate names leave the list.
2. **Gate names move to a key-toggled overlay** (`c`), which closes on the next
   movement. This is the user's "errors only on pressing a key, gone when
   moving again", and it is also what the industry actually does (§3.3).
3. **Section headers become a left gutter** — the brief's "pointer in the side".
   I initially argued against this and the measurements overturned me; §4 records
   both the argument and the numbers that beat it.

The result is that every line on the board is a row. That makes `window()` a
plain clamp and gives an unconditional invariant: **one keypress scrolls the
board by at most one line**, which is the property five earlier attempts were
reaching for. On the live board it also takes visible PRs from 16 to 26.

---

## 2. Why the previous recommendation is overturned

`view-restructure.md` concluded: fix `window()`, raise the margin to 3, do not
touch the layout. Two things have happened since.

### 2.1 The recommended fix was implemented, and the jump survived

The study proposed replacing greedy anchor-snapping with a smarter anchor
search. What shipped went further — `c9bd6c7` and `4b5e6f6` abandoned anchors
entirely for a line clamp anchored to the cursor row's first line, clipping the
top row. That is a *stronger* fix than the one recommended.

It did fix the thing it aimed at. Measuring the shipped `window()` by walking
the cursor down synthetic boards at `height=19`, the cursor's own screen line is
now genuinely constant in steady state (`screenPos` 16 on every board tested).

But the jump the user reports is still there, because it was never the cursor's
screen line. It is **how far the board scrolls per keypress**:

| board | lines scrolled per single `j` |
|---|---|
| 3 sections, some failing | **0, 1, 2 or 3** |
| 4 sections, some failing | **0, 1, 2 or 3** |
| 1 section, all rows failing | 1 or 2 |
| 1 section, **no** failing rows | always 1 |

One identical keypress moves the world by an amount varying 4x, decided by
invisible data — whether a neighbouring row happens to carry a gate line, and
whether a section boundary is passing through the margin. The cursor is nailed
in place while the content heaves underneath it. That reads as "jumping" to the
eye just as much as a moving cursor does, and arguably worse, because the thing
that moves is the thing you are trying to read.

Note the last row of that table. The only configuration that never jumps is the
one with uniform rows. That is the whole finding.

### 2.2 The study's cost model does not match this repo's real board

`view-restructure.md` §5.3 argued the continuation line is close to free:
*"you pay only when there is something to say — a minority of rows on a healthy
board, and zero lines on a board where nothing is wrong."*

Captured from the live board in a 27-row pane:

```
16 PR rows visible
 8 continuation lines
```

**Half the rows are two lines tall.** The reason is dependabot. Its PRs fail
many gates at once, and the resulting continuation lines are the least
informative ones on the board:

```
#3229  ✗6 ○   chore: bump the npm-production group across 1 directory with 15 updates
       └ bump-affected, build-push-image admin-console, build-push-image customer-port…
#3231  ✗4 ○   chore: bump @types/serve-static from 1.15.7 to 2.2.0
       └ build-push-image customer-portal, build-push-image billing-service           …
```

The triage argument that justified keeping the names (§3.5 of the old study:
`lint-typecheck-test` = my problem, `Tag-Deploy` = infrastructure) is sound for
a hand-written PR with one failing gate. It collapses for a 4-to-6 gate
dependabot PR, where the line is clipped mid-name and tells you nothing that
`✗6` did not already say.

So the cost is 50% of rows, not a minority, and a large share of that cost buys
no signal. That inverts the §5.3 conclusion.

### 2.3 Four commits have now attacked this

```
0989c62  Keep context below the cursor when scrolling
8b0758f  Scroll by whole rows instead of by lines
b385563  Hold the board still until the cursor reaches the bottom margin
c9bd6c7  Anchor the viewport to its bottom edge and clip the top row
4b5e6f6  Hold the cursor's screen line, not its row margin
```

Five, in fact. Each is a locally reasonable fix that traded one manifestation of
variable row height for another. The pattern itself is the evidence: the
constraint being fought is that **a list whose items have data-dependent heights
cannot have a stable scroll rhythm**. The old study proved a legal viewport
position always *exists* at margin 3; it did not prove the *sequence* of
positions is smooth, which is what the eye judges.

`view-restructure.md` §6.1 already collected the industry evidence for this and
labelled it "the strongest argument for restructuring, and it is a good one" —
bubbles, tview, lazygit and k9s all make uniform height structural, and fzf's
opt-in multi-line mode shipped scroll bugs of exactly this class. §6.2 set it
aside on the grounds that our clamp is solvable. It is solvable; it is just not
smooth. The ecosystem's constraint was load-bearing after all.

---

## 3. What the restructure changes

### 3.1 Rows

The continuation line is deleted. `renderRow` returns exactly one line, always.
Row height stops being a function of the data.

The failure count stays in the status cluster (`✗6`), so "is this broken" and
"how broken" are unchanged at a glance. Only the gate *names* move.

### 3.2 Gate names — the `c` overlay

> **The key is now `d`.** It was `c` for checks; the page grew a state block and
> became the detail page, and the user renamed the key to match. Everything
> below is unchanged apart from which key opens it. See `DESIGN.md` §3.6.

Pressing `c` on a row with failing gates opens an overlay listing them in full,
one per line, unclipped. This is strictly *more* information than the old
continuation line, which clipped to a single line with `…` and in the
dependabot case lost most of the list.

It closes on `c`, `esc`, `q`, or — per the brief — **any movement key**. That
last part matters: it makes the overlay feel like a transient inspection rather
than a mode you have to remember to leave.

The `?` help overlay already establishes the mechanism (`internal/ui/help.go`),
so this is an instance of an existing pattern, not a new concept.

### 3.3 Why an overlay and not a pane

`view-restructure.md` §3.1 rejected a permanent detail pane, and that analysis
holds — gh-dash spends 45% of width on one, its no-preview path was broken on
release (#798) because nobody runs it, and for a cmd+\` glance-and-dismiss tool
a permanently occupied region is the wrong trade.

The old study's §6.2 also found that the real industry default is not a pane:
aerc's preview is off by default, neomutt's `pager_index_lines` is 0, fzf has no
`--preview` unless you pass one. The consensus is **detail on a separate screen
you opt into** — which is exactly this overlay. The brief and the evidence agree
here.

---

## 4. Section headers: the brief was right and I was wrong

My first draft of this document argued for keeping the full-width header band
and rejected the "pointer in the side" idea. Measuring it changed the answer,
and the reversal is worth recording.

The argument I made was that section headers are fixed furniture rather than a
data-dependent wildcard, so they are not the same class of problem as the gate
line. That is true, and it is also not sufficient. A header still takes a line
that is not a row, and whenever it crosses the top edge one keypress moves the
board two lines instead of one.

Measured on synthetic boards at `height=19`, counting lines scrolled per single
`j` once rows were already uniform:

| section chrome | scroll deltas |
|---|---|
| blank + band (as shipped) | 0, 1, **2**, **3** |
| band only, no blank | 0, 1, **2** |
| no header line at all | 0, 1 |

Only the third row achieves the invariant. Dropping the blank line is a genuine
improvement and would have been the cheap fix, but it does not finish the job.

So the section name moved into a left gutter on each row, carried on the
section's first row and blank on the rest:

```
MINE     │ #3248  ✗2 ○  feat(api-service): refuse…   2h
         │ #3251  ◐  ○  feat(api-service): backfill… 2h
REVIEW   │ #3199  ✓  ✗  fix(billing-service): stop…  1d
ALL OPEN │ #3246  ✗1 ✗  feat(context-map): add…      now
```

Every line on the board is now a row. That is what makes `window()` a plain
clamp and the one-keypress-one-line invariant unconditional.

**What it costs.** The gutter is 10 cells (8 + a rule), taken from the title
column, so every width breakpoint moved up by 10 to keep the title above its
readable floor. The per-section count is gone from the band; the section name is
clipped to 8 cells (`NEEDS M…`). Sections no longer have a visual separator
beyond the gutter label changing.

**What it buys**, measured on the live board in a 27-row pane: **26 PRs visible,
up from 16.** Roughly two thirds of that is the gate lines and one third the
header bands.

---

## 5. What this costs, honestly

- **The gate names are no longer visible while scanning.** On a single-gate
  hand-written PR, the old row told you `lint-typecheck-test` without a
  keystroke; now it costs `c`. This is a real regression for that case, and it
  is the price of the restructure. It is offset by the fact that the overlay
  shows the *complete* list, where the old line clipped it.
- **One new key and one new transient mode.** Small, and it reuses the help
  overlay's mechanism.
- **The fourth redundant failure channel is gone.** `view-restructure.md` §5.4
  argued a taller row signals "broken" in peripheral vision. True, and now lost.
  Glyph, colour and count remain, which is three channels for a binary fact.
- **10 cells of title width**, spent on the gutter. Every width breakpoint moved
  up by the same 10 so the title keeps its readable floor, which means the FULL
  tier now starts at 86 columns rather than 76.
- **The per-section count and the full rule name.** The band showed
  `━━ NEEDS MY REVIEW ━━ 12`; the gutter shows `NEEDS M…` and no count.

What is gained: one keypress scrolls at most one line, unconditionally, and the
live board shows 26 PRs where it showed 16.

---

## 6. What I could not verify

- **Whether the user prefers the overlay to the inline names in daily use.**
  The density and scroll arguments are measured; this preference is not, and it
  is the one that decides whether the trade was worth it.
- **Board shapes other than this repo's.** The 50%-continuation-line measurement
  is from `acme/monorepo`, which is dependabot-heavy. A repo without that
  traffic would see a smaller density win — though the scroll fix is structural
  and does not depend on the ratio.
- **The five-commit history.** I read the messages and the current code, not
  each intermediate diff, so my characterisation of what each attempt traded
  away is inference from the message and the surviving code.
- **Whether 8 cells is the right gutter width.** It fits this config's rule
  names to a recognisable stem and nothing longer. A config with several rules
  sharing a prefix would clip them to the same string; I did not test that.
- **Whether losing the per-section count is felt.** `view-restructure.md` cites
  research requirement R8 as wanting it. It is still one `l`/`h` jump away, but
  it is no longer on screen.
