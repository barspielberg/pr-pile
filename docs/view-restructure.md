# prs-mng — should the view be restructured?

Research and recommendation, written in response to:

> *"maybe we should restructure the view, maybe we need to change it to each item
> take the same space and header and errors would be somewhere else"*

Researched: 2026-09-17. Builds on `docs/design-research.md` (three rounds) and
`docs/design-spec.md`. Claims about `prs-mng`'s own behaviour are from a
transcription of the shipped `window()` into a simulator and exhaustive search
over its state space; claims about other tools are cited. Where I could not
verify something I say so in §6.

---

> Note: this study cites `design-research.md` and `design-spec.md`, which have
> since been consolidated into `DESIGN.md`. The citations are kept as written
> for provenance; the surviving conclusions live in `DESIGN.md`.

> **Superseded (2026-09-17).** §1's recommendation — "fix the scroll code, not
> the layout" — was implemented and the jump survived. The board was then
> restructured to uniform rows, which is what this study argued against. See
> `uniform-rows.md` for the measurements that overturned it. The diagnosis in §2
> and the industry survey in §6 remain accurate and are still worth reading;
> it is the §1 conclusion that did not hold.


## 1. Recommendation

**Fix the scroll code, not the layout.** The drift is caused by
*anchor-snapping* in `window()` — the viewport is forced to start on a row
boundary, and because anchors are unevenly spaced the cursor's distance from the
bottom edge changes as it moves. It is **not** caused by rows having different
heights: I ran the current algorithm against a hypothetical uniform grid and
against variable-height boards, and the drift tracks the anchor spacing, not the
row heights.

Concretely: raise `scrollOff` from 2 to 3 and replace the greedy
"first anchor that fits" loop with a search for the anchor that lands the cursor
*closest to* the target margin. That is a change to one ~25-line function. It
holds the margin constant on every board shape I tested and keeps the no-orphan
guarantee that `TestScrollAlwaysStartsOnAWholeRow` exists to protect.

If you want the layout change anyway, the one worth doing is **Q1 option 3**
(fold the failing check names into the title line) — but do it for vertical
economy, not to fix scrolling, and understand it costs you the full gate list.
**Do not** add a detail pane (§3.1) and **do not** switch sections to tabs (§4.3).

---

## 2. The diagnosis, and why it matters before anything else

This section is the load-bearing part of the document. Everything in §3 and §4
is downstream of it.

### 2.1 What the code actually does

`window()` (`internal/ui/model.go`) does not scroll to a line. It scrolls to an
**anchor** — a section header or a row's first line — chosen by a greedy loop:

```go
for _, a := range anchors {
    if a > cursorLine { break }
    if cursorEnd+off-a < height { return page(lines, a, height) }
}
```

It returns the *first* (highest) anchor that leaves at least `off` lines below
the cursor. The margin it actually achieves is therefore
`height - (cursorEnd - a)`, which depends entirely on **where the nearest anchor
happens to be**. Anchors are unevenly spaced: a 2-line failing row pushes the
next anchor 2 lines along, and a section boundary (blank + header) pushes it 2-3.
So the achieved margin is whatever the anchor grid allows, not what `off` asked
for.

### 2.2 Measured drift

Transcribing `window()` verbatim into a simulator and walking the cursor down a
board of 3 sections / 18 rows / 2 failing, at `height=19`:

```
row  line  h   start  gapBelow
10   17    1   1      2
11   18    1   2      2
12   19    1   4      3     <-- drift
13   20    1   4      2
14   21    1   5      2
```

`gapBelow` is the number of lines between the cursor's last line and the bottom
edge. It should be constant. It goes 2, 2, **3**, 2, 2.

Across board shapes (`height=19`, steady state, excluding the list ends):

| board | CURRENT gapBelow | line-based clamp |
|---|---|---|
| 3 sections, 2 failing | 2 and 3 (drifts) | **2, constant** |
| 5 sections, many failing | 2 and 3 (drifts) | **2, constant** |
| 1 section, no failing rows | 2, constant | 2, constant |
| 1 section, **all** rows failing | 3, constant | 2, constant |

Read the last two rows carefully, because they are the whole argument:

- A board with **no variable-height rows at all** still drifts under the current
  code as soon as there is more than one section (row 1 of the table).
- A board where **every single row is 2 lines** — maximum irregularity — does
  *not* drift. It just holds a constant margin of 3 instead of 2.

Variable row height is not what breaks it. **Uneven anchor spacing is.**

### 2.3 The confirming experiment

Run the **current, unmodified algorithm** against a fully uniform grid (1-line
rows, 1-line headers, no blank separators) — i.e. exactly what the user proposed
building:

```
uniform grid (30 rows, 3 sections, h=19): gapBelow={2: 12}  distinct=1
uniform grid (45 rows, 5 sections, h=19): gapBelow={2: 27}  distinct=1
uniform grid (60 rows, 2 sections, h=19): gapBelow={2: 41}  distinct=1
```

Constant everywhere. So the user's instinct is *mechanically correct*: a uniform
grid does fix it, because when every line is an anchor, anchor-snapping becomes a
no-op. The question is whether that is the cheapest way to get there — and it
is not, because the same property is available by changing the anchor search.

### 2.4 Which of the two irregularities is actually to blame

Isolating them (`height=19`):

| variant | gapBelow |
|---|---|
| 1-line header, rows still 1-or-2 lines | 2 and 3 — **still drifts** |
| uniform 1-line rows, header still 2 lines (blank + rule) | **2, constant** |

This inverts the brief's hypothesis. Dropping the blank line above section
headers — the cheap fix, Q2 option 4 — **does not solve the problem**. Keeping
the 2-line header but making rows uniform **does**. The section header is not
the culprit; the failing-check continuation line is.

### 2.5 Why eight attempts failed: two requirements in direct conflict

The git history shows the oscillation:

```
0989c62  Keep context below the cursor when scrolling     <- line-based margin
8b0758f  Scroll by whole rows instead of by lines         <- anchors (fixes orphans)
b385563  Hold the board still until the cursor reaches... <- tries to fix the drift
```

There are two requirements, each with a test:

1. **Constant margin** — `TestCursorKeepsContextBelowIt`. Wants a line-based offset.
2. **No orphaned detail line at the viewport top** — `TestScrollAlwaysStartsOnAWholeRow`.
   Wants an anchor-snapped offset.

A pure line clamp satisfies (1) perfectly and violates (2). I measured the
violation rather than assuming it:

```
board 0: ORPHAN: cursor row at line 19 -> start=3 is a continuation line
board 2: 7 orphan positions out of 15 rows
```

And the obvious hybrid — take the line clamp, then nudge the start up by one when
it lands on a continuation line — **fails outright**:

```
HYBRID: line clamp + orphan nudge
  C 1sec,all fail   gapBelow={-1: 1, 1: 6}  orphans=0
```

`gapBelow = -1` means the cursor was pushed off the bottom of the screen.
Nudging up removes a line of context below, which is the one direction that
cannot be given away. This is almost certainly one of the eight failed attempts.

### 2.6 The constraints are reconcilable — at a margin of 3, not 2

I searched exhaustively: for each cursor row, does **any** viewport start exist
that simultaneously shows the whole selected row, leaves exactly K lines below
it, and does not begin on a continuation line?

```
board A (3 sec, 2 failing):   K=2 -> 0 impossible rows   K=3 -> 0 impossible rows
board C (1 sec, ALL failing):  K=2 -> 3 impossible rows   K=3 -> 0 impossible rows
board D (mixed):               K=2 -> 0 impossible rows   K=3 -> 0 impossible rows
uniform grid:                  K=2 -> 0 impossible rows   K=3 -> 0 impossible rows
```

At **K=3 a legal start always exists**, even on the maximally irregular board.
At K=2 it does not — which is why the current code, with `scrollOff = 2`,
*cannot* satisfy both constraints and is forced to drift.

This is the actionable finding. The fix is:

- `scrollOff = 3` (also the modal value across fzf, nnn and micro — see
  `design-research.md` §9.1, and it is what guarantees a full 2-line row of
  lookahead rather than half of one).
- Replace the greedy first-anchor-that-fits loop with: among anchors that keep
  the whole selected row visible, pick the one whose resulting `gapBelow` is
  closest to `scrollOff` without going under. Still stateless, still derived from
  `(cursorLine, cursorHeight, height, anchors)` each frame.

No layout change. No new pane. No lost information.

---
## 3. Q1 — should every row be exactly one line?

The failing-check continuation line is the only thing that makes a PR row taller
than one line. Five options were considered.

For reference, the board as it stands today at 120 columns — note the two
continuation lines, which are the only rows taller than one line:

```
+------------------------------------------------------------------------------------------------------------------------+
|== MINE ============================================================================================================== 5|
|  /-#3248  x2 o    feat(api-service): PROJ-2037 refuse order plan writes after cutoff                                 2h|
|  |        \ lint-typecheck-test api-service, Tag-Deploy                                                                |
|  | #3251  (  o    feat(api-service): PROJ-2038 backfill order_cutoff_at column                                       2h|
|| \-#3254  v  v !  feat(api-service): PROJ-2039 expose cutoff in admin API                                            1h|
|    #3199  v  x    fix(billing-service): PROJ-1994 stop double-counting idle vehicles                                 1d|
|    #3233  x1 o    fix(auth-service): PROJ-2004 guard null context closure                                            6h|
|           \ lint / eslint                                                                                              |
+------------------------------------------------------------------------------------------------------------------------+
```

(ASCII stand-ins throughout: `x2`=`✗2`, `o`=`○`, `v`=`✓`, `|` in column 0 =`▌`,
`/- | \-` = the `╭╴ │ ╰╴` tree glyphs, `\` = `└`, `==` = the `━` rule.)

Framing note, because it changes the weighting: per §2, moving this line does
**not** on its own make scrolling tractable at `scrollOff = 2` — it makes it
tractable at any margin, but so does fixing the anchor search. So the
continuation line should be judged on its own merits (vertical economy, clarity),
not as a scroll fix.

### 3.1 Option 1 — a detail pane (bottom or right). **Rejected.**

A fixed region showing the selected PR's detail: failing checks, reviewers,
branch, body.

**The evidence from the closest analogue is a warning, not an endorsement.**
gh-dash ships a preview sidebar, and the numbers are worse than they look:

- `preview.open: true` by **default**, `width: 0.45` — 45% of terminal width
  (`internal/config/parser.go:349-353`). Docs confirm: "Display the preview pane
  to the right at 45% width, or below at 40% height when the terminal is narrow."
- Its chrome is **4 lines** before any PR is drawn: 3-line tab bar + 1-line
  footer (`internal/tui/common/styles.go:12-24`, `HeaderHeight = 2`,
  `TabsHeight = 3`).
- Its default row height is **3 lines per PR**, not 1
  (`internal/tui/components/table/table.go:55-61`: `ShowSeparator: true` and
  `Compact: false` by default).

So a default gh-dash at 40 rows shows about 12 PRs. `prs-mng` at 20 rows shows
14. We are already 2x denser than the tool we would be borrowing from, and the
pane is the main reason.

The deeper problem is that gh-dash's preview is **not optional in practice**. A
failing row renders one glyph with no count and no names
(`internal/tui/components/prrow/prrow.go:112-133` returns a bare `FailureIcon`),
and the check detail exists *only* in the sidebar
(`internal/tui/components/prview/checks.go`). The evidence that this is a trap:
issue **#798** (2026-03-07) is a panic — `strings: negative Repeat count` — that
fires when you set `preview.open: false`. The non-preview path was broken on
release because effectively nobody runs it.

That is exactly the architecture we would be adopting: move the "why is CI red"
answer out of the row, and the pane becomes load-bearing. For a tool the user
opens with cmd+\` to *glance* and dismiss, a permanently-occupied 45% of width or
40% of height is the wrong trade. It also directly contradicts
`design-spec.md` §7.1, which cut the global header to buy back a single row.

Worth noting for honesty: I found **no gh-dash issue complaining the preview is
on by default or wastes space**. The space complaints are aimed at the logo
(#671). Users ask for *more* preview (#107, #594 want a bottom position for
narrow terminals). So the pane is not disliked — it is simply expensive, and
expensive in the dimension `prs-mng` has spent three rounds of research
protecting.

### 3.2 Option 2 — expand the selected row on demand. **Rejected — this is the trap.**

The brief flags this as the crux, and it is.

The appeal is obvious: only one row is ever tall, and it is the row you are
looking at. But it makes scrolling **strictly harder**, not easier, for a reason
that is easy to miss:

Under the current design, a row's height is a property of the *data* — it is 2
lines iff `FailedGates` is non-empty, and it does not change as you navigate. The
line layout of the whole board is therefore stable while the cursor moves. Under
expand-on-demand, a row's height becomes a function of **the cursor position
itself**. Moving the cursor changes the line map, which changes the scroll
offset, which is computed from the line map. The computation becomes
self-referential: you cannot evaluate "where should the viewport start" without
first knowing where the cursor is, and the cursor's own line number now depends
on whether rows above it are expanded.

fzf is the only tool I found that handles variable-height items in a scroll
margin, and its implementation is a **two-phase loop** whose source comment says
it exists "to avoid infinite loop of alternating between moving up and down"
(`src/terminal.go`, cited in `design-research.md` §9.4). That loop is needed
precisely because item heights vary. Expand-on-demand would force us into that
class of algorithm while the current design lets us stay with a stateless clamp.

So: it does not relocate the problem, it **amplifies** it. Rejected.

### 3.3 Option 3 — fold it into the row, truncated. **The one worth considering.**

Put the first failing check (or two) at the end of the title line:

```
+------------------------------------------------------------------------------------------------------------------------+
|== MINE ============================================================================================================== 5|
|  /-#3248  x2 o    feat(api-service): PROJ-2037 refuse order plan writes after cutoff       [lint-typecheck-test +1]  2h|
|  | #3251  (  o    feat(api-service): PROJ-2038 backfill order_cutoff_at column                                       2h|
|| \-#3254  v  v !  feat(api-service): PROJ-2039 expose cutoff in admin API                                            1h|
|    #3199  v  x    fix(billing-service): PROJ-1994 stop double-counting idle vehicles                                 1d|
|    #3233  x1 o    fix(auth-service): PROJ-2004 guard null context closure                           [lint / eslint]  6h|
+------------------------------------------------------------------------------------------------------------------------+
```

(ASCII stand-ins: `x2`=`✗2`, `o`=`○`, `v`=`✓`, `|`=`▌`, `/- | \-` = tree glyphs,
`==` = the `━` rule.)

**What is actually lost.** The user's stated need is to see *why* CI is red
without opening the PR. In practice a failing PR usually has **one or two**
failing gates, and the first name is the one that identifies the failure class
(`lint-typecheck-test` vs `e2e / checkout-flows` vs `Tag-Deploy`). The `+1`
suffix preserves the fact that more exist. What is lost is the *complete* list
when 3+ gates fail — which is the case where the list was least readable anyway,
since `design-spec.md` §1.5 already clips it to one line with `…`.

**What is gained.** Every row becomes exactly 1 line. At `height=19` on a board
with 4 failing PRs that is 4 rows back — over 20% more PRs visible. And the
anchor grid becomes uniform, so `window()` is correct with or without the §2.6
fix.

**The cost.** It eats title width. At 120 columns the title has 97 cells; a
24-cell gate suffix leaves 73, still comfortable. At 80 columns the title has 57
and the suffix would leave 33 — too tight. So this option needs its own
responsive rule: **show the gate suffix only at FULL tier and only above ~100
columns**, otherwise fall back to the bare `✗2` glyph. That is a new breakpoint,
which `design-spec.md` §6 currently does not have.

This is a real, defensible option. It is not a scroll fix — it is a density win
that happens to make the grid uniform.

### 3.4 Option 4 — a key to reveal, in an overlay. **Viable as an addition, not a replacement.**

The `?` help overlay already exists (`internal/ui/help.go`), so the mechanism is
built. Pressing e.g. `c` on a failing PR would show its full check list.

Against it as a *replacement*: it costs a keystroke and a mode, and it fails the
"glance" test in `design-research.md` §0.6 — the user wants to see why CI is red
while scanning, not after deciding to interrogate a specific row. A board where
the answer is one keypress away is meaningfully worse than one where it is
already on screen, for a tool whose whole premise is cmd+\`, glance, dismiss.

For it as an *addition*: it pairs perfectly with option 3. Truncate to one gate
in the row; press a key for the full list on the rare 3+-gate PR. That recovers
everything option 3 gives up, at zero permanent cost.

### 3.5 Option 5 — drop it entirely. **Rejected, and the user is right.**

The brief asks me to argue this honestly rather than dismiss it. Having done so,
it still loses.

The case for dropping: `✗2` already answers "is this broken", and
`design-spec.md` §4 says green/settled states should be *confirmable*, not
noticeable — one could argue the same about failure detail.

The case against, which is stronger: `✗2` answers "is it broken" but not "is it
**my** problem". `lint-typecheck-test` means "I broke something, 2 minutes to
fix". `e2e / checkout-flows` means "possibly flaky, re-run it". `Tag-Deploy`
means "infrastructure, not me". These lead to *different actions*, and the whole
board exists to answer "what do I need to do right now"
(`design-spec.md` §9.1). Collapsing them to `✗2` deletes the triage signal and
forces a browser round-trip — the exact cost the tool is meant to avoid. The user
has said this explicitly and the design spec already resolved it once
(§1.5, "Kept, demoted", and research open question 4).

Dropping it would also be the *only* change here that loses information
outright. Options 3 and 4 relocate or compress it; this one deletes it.

---
## 4. Q2 — should section headers leave the scrolling list?

**Short answer: no.** And per §2.4 this question is largely moot for the scroll
problem — a 1-line header does not fix the drift, and the 2-line header does not
cause it.

### 4.1 Option 1 — sticky header pinned at the top

The current section's name stays at the top and changes as you cross boundaries.

**For.** Answers "which section am I in" when the header has scrolled away, which
on a long board is a genuine gap. Familiar from mobile and web lists.

**Against, and it is decisive for this tool.** A sticky header is a region whose
*content* changes as you scroll but whose *position* does not — which means the
line under it shifts meaning as you move. More practically: it costs a permanent
row (the thing `design-spec.md` §7.1 just spent a section buying back), and it
reintroduces exactly the coupling §3.2 rejects — the rendered chrome becomes a
function of the cursor position, so the line map changes as you navigate.

I did not find a TUI in this survey that implements sticky section headers inside
a scrolling list. See §6 — this is a negative I could not exhaustively prove.

### 4.2 Option 2 — section name in the footer

The footer already exists and already has a right-hand field (repo + spinner).
Showing the current section there costs zero rows.

**This is the good version of option 1.** It answers the same question — "where
am I" — with no new chrome, no position-dependent layout, and no change to the
list. It composes with keeping headers inline: the inline header is the
*boundary* marker, the footer is the *current state* readout.

Cheap, safe, and worth doing independently of everything else in this document.
It does not fix scrolling and does not claim to.

### 4.3 Option 3 — tabs, one section at a time. **Rejected.**

The brief notes `l`/`h` already jump between sections, so this looks like a small
step. It is not.

gh-dash does exactly this, and I confirmed the mechanics:
sections are a **carousel** of tabs with exactly one active
(`internal/tui/components/tabs/tabs.go`; `CurrSectionId()` is literally
`m.carousel.Cursor()`), switched with `right`/`l` and `left`/`h`
(`internal/tui/keys/keys.go`) — the same keys `prs-mng` already uses. The tab bar
costs **3 lines** (`internal/tui/common/styles.go`: `TabsHeight =
TabsBorderHeight + TabsContentHeight = 3`).

Three reasons it is wrong here:

1. **It costs 3 rows to save 2.** The current design spends 2 lines per section
   boundary (blank + rule). With 3-5 sections that is 4-8 lines. A tab bar is a
   flat 3 — but it is 3 lines of *permanent* chrome in a 20-row pane, and it
   only breaks even at 3+ sections while making the other sections invisible.
2. **It destroys the one-glance property.** `design-spec.md` §9.1's closing
   argument is that the eye lands on the four red cells across the *whole board*
   without reading anything. With tabs, "do I need to do anything right now"
   requires visiting every tab. That is the single most important property of
   this tool and tabs trade it away.
3. **`l`/`h` today is a jump within one continuous list** — the user keeps
   peripheral vision of neighbouring sections. As a tab switch it becomes a
   context swap. Same keys, different mental model.

### 4.4 Option 4 — keep inline, drop the blank separator (1-line header)

The cheap option. Per §2.4 it **does not solve the scroll problem** — a board
with 1-line headers and variable rows still drifts.

It is also a real regression in legibility: `design-spec.md` §1.6 states the
blank line "is the only pure-whitespace row in the design and it is what
separates sections". Without it the header rule sits directly against the last
row of the previous section, and the `━` band abuts a PR title.

It buys 1 row per section boundary (2-4 rows on a typical board). If vertical
space becomes critical, this is available — but it should be spent knowingly, and
not in the belief that it fixes scrolling.

### 4.5 Option 5 — keep as is. **Recommended.**

The 2-line header is not implicated in the drift (§2.4), it is the only section
separator, and it carries the count that `design-research.md` R8 requires.
Combine with option 2 (section name in the footer) if you want the "where am I"
answer on long boards.

---

## 5. Q3 — the case against restructuring

Argued as the strongest available case, because on the evidence it wins.

### 5.1 The complaint was about scrolling, and the layout is not the cause

This is the whole argument and §2 is its proof. The user asked for a restructure
as a *hypothesis about the cause* — "maybe each item should take the same space"
— not as a complaint about the layout. The measurements say the hypothesis is
half right in a way that matters: a uniform grid would indeed fix it, but so does
a ~25-line change to `window()`, and the layout is not what broke.

Redesigning a layout to fix a bug in the code that scrolls it is the expensive
way to get the same result, and it spends a design that took three rounds of
research and that the user liked.

### 5.2 The design was derived, not chosen, and each piece has a reason

`design-spec.md` is explicit that every width and glyph is a decision. The two
irregularities under scrutiny are both *resolved open questions*, not oversights:

- The continuation line is research open question 4, resolved "keep, demoted"
  (§1.5, §10). Its reasoning — the red `✗2` raises the alarm, the names are
  detail — is still sound.
- The blank line above headers is an explicit density decision (§1.6, R18:
  "maximum density, one blank line per section boundary").

Reopening both to fix a scroll bug means relitigating settled decisions for a
reason that turns out not to apply.

### 5.3 A uniform grid costs real information

Every option that makes rows uniform either deletes the gate names (§3.5),
truncates them (§3.3), hides them behind a keypress (§3.4), or spends permanent
screen space on a pane (§3.1). There is no version that is free. The current
design pays **one line, only on rows that are actually broken** — which is a
minority of rows on a healthy board, and zero lines on a board where nothing is
wrong. That is close to an optimal cost curve: you pay only when there is
something to say.

### 5.4 Uniformity is not obviously the right aesthetic here

The strongest *design* argument for a uniform grid is scannability: a regular
rhythm is easier for the eye to track. But the irregularity here is not noise —
it is *signal*. A taller row means "this one is broken". That is a fourth
redundant channel for failure, on top of glyph, color and count, and it is
visible in peripheral vision before any glyph is resolved. Flattening the board
removes it.

### 5.5 Where the case against is weak

Two honest concessions:

- **Option 3 (§3.3) is genuinely attractive on density grounds.** It recovers
  ~20% more visible rows on a board with several failing PRs and makes the anchor
  grid uniform as a side effect. If the user's real complaint tomorrow is "not
  enough PRs fit", this is the answer, and it should be judged on that.
- **`scrollOff = 2` is not satisfiable** (§2.6). So "just fix the scroll code" is
  not literally a no-op — it requires accepting a margin of 3, which costs one
  more row of reachable screen. That is a real cost, though a small and
  well-precedented one (fzf, nnn and micro all ship 3).

---
## 6. The industry evidence, weighed honestly

This survey cuts **against** my recommendation in one important respect, and the
document would be dishonest without saying so plainly.

### 6.1 Uniform row height is close to universal, and it is structural

Every mature TUI list framework examined hard-codes uniform row height into its
viewport math:

| framework | evidence |
|---|---|
| **bubbles `list`** | `ItemDelegate.Height() int` takes **no item argument** — varying height per item is structurally impossible. `list.go:793`: `PerPage = max(1, availHeight/(delegate.Height()+delegate.Spacing()))` — flat division by a constant. |
| **bubbles `table`** | `renderRow` applies `Inline(true)` (strips newlines) + `ansi.Truncate` per cell. One line, enforced. |
| **tview `List`** | `showSecondaryText` is a flag on the *List*, not the item; scroll math is `if showSecondaryText { index /= 2 }`. |
| **tview `Table`** (k9s) | "Each row will then occupy one row on screen." k9s's own `Row{Fields []string}` cannot carry a second line. |
| **lazygit** | `getDisplayStrings` returns columns, not lines; index mapping is `viewIndex = modelIndex + (# preceding headers)` with **no height term**. |

And the two systems that *did* add variable height both immediately produced
scroll bugs of exactly our kind:

- **fzf multi-line** (0.53.0, opt-in, requires `--read0`): issue **#4069** —
  "page-up/down doesn't take into account the number of lines of items in
  multi-line mode", fixed in 0.56.1; plus "Fixed extra scroll offset in
  multi-line mode" in 0.56.0. It also needed new compensating options
  (`--gap`, `--marker-multi-line`) to restore visual separation.
- **lazygit's `NonModelItem` section headers**: the in-tree source comment says
  stale index conversion *"used to cause both wrong results and
  index-out-of-range panics."*

**This is the strongest argument for restructuring, and it is a good one.** We
are doing something that the ecosystem's own primitives refuse to support, and
we have hit precisely the bug class it refuses to support it *because of*.

### 6.2 Why it still does not flip the recommendation

Three reasons, in order of weight:

1. **We are not using those primitives.** `prs-mng` renders to a `[]string` and
   windows it by hand. The frameworks' constraint is an artifact of their
   pagination and hit-testing designs (`PerPage = height/itemHeight`,
   `index /= 2`); our `window()` is a stateless clamp over a line list, which has
   no such division. §2.6 demonstrates by exhaustive search that our version of
   the problem **is solvable** at `scrollOff = 3`. fzf's is harder than ours
   because it also supports wrapping and mouse hit-testing.
2. **fzf's bugs are evidence for care, not for surrender.** fzf shipped
   multi-line, hit scroll bugs, and **fixed them in two point releases** — it did
   not revert. It is still shipping.
3. **The detail-goes-elsewhere consensus is weaker than it appears.** aerc's
   preview is **off by default** (`message-list-split=` is empty); neomutt's
   `pager_index_lines` defaults to **0**; fzf's `--preview` does not exist unless
   you pass a command. So the actual industry default is not "detail in a pane" —
   it is **"detail in a separate screen you opt into"**, which is §3.4 (a key to
   reveal), not §3.1 (a permanent pane). That materially strengthens the case
   against the detail pane and slightly strengthens the case for the overlay.

### 6.3 What this changes

It raises §3.3 (fold gate names into the row) from "defensible" to "the right
second step". If `prs-mng` ever adopts `bubbles/list` or `bubbles/table`, or
grows mouse support, uniform rows stop being a preference and become a
requirement. Doing it now would be reasonable. Doing it *to fix scrolling* would
still be misattributing the bug.

---

## 7. Cost of the change

What breaks under each route.

### 7.1 Fixing `window()` only (recommended)

| area | impact |
|---|---|
| `design-spec.md` column arithmetic | **none** |
| responsive tiers (§6) | **none** |
| filter | **none** — `window()` already takes the filtering-time `avail` |
| `l`/`h` section jumping | **none** — operates on row indices, not lines |
| existing tests | `TestCursorKeepsContextBelowIt` asserts `below >= scrollOff`; still passes at 3. `TestScrollAlwaysStartsOnAWholeRow` still passes (anchors retained). Add a test asserting the margin is *constant*, not merely sufficient — its absence is why the drift survived eight attempts. |
| new cost | one more row of reachable screen (margin 2 → 3) |

### 7.2 Adding the gate names to the row (§3.3)

| area | impact |
|---|---|
| column arithmetic | **breaks.** `fixedFull = 23` assumes title is the only flex column. A gate suffix is a second variable-width field; the §1.2 budget and the "exactly one column flexes" invariant (§1.1) both need restating. |
| responsive tiers | **breaks.** Needs a new breakpoint above FULL (~100 cols) to decide whether the suffix appears. `design-spec.md` §6 has four tiers and no fifth. |
| `renderRow` | title clip width becomes a function of gate-name length — `TestLongTitleDoesNotOverflowWidth` and `TestNoRowOverflowsAtAnyWidth` need extending to the new field. |
| filter | title match highlighting (`renderTitle`) must not treat the suffix as title text. |
| tests | `TestViewRendersStatesAndGates` asserts gate names appear; would need rewriting for the new position. `TestScrollAlwaysStartsOnAWholeRow`'s orphan check becomes vacuous. |
| spec | §1.5 (continuation line), §4 (hierarchy table's "secondary detail" column), §9.1-9.6 mockups all need redrawing. |

### 7.3 A detail pane (§3.1) — for completeness

Largest blast radius: a second focusable region, a width/height split, a
position-auto breakpoint, pane-local scrolling, and per gh-dash #798 a
well-tested "pane closed" path. Every mockup in `design-spec.md` §9 invalidated.
Not recommended.

---

## 8. What I could not verify

- **Sticky section headers in a TUI.** I did not find an implementation in k9s,
  lazygit, aerc, neomutt or fzf. This is a negative over a bounded survey, not a
  proof that none exists — I did not search exhaustively, and absence of evidence
  here is weak.
- **The "eight fix attempts".** I reconstructed the trajectory from four commits
  (`998a6d1`, `0989c62`, `8b0758f`, `b385563`). The other attempts are not in git
  history, so my claim in §2.5 that the failed hybrid "is almost certainly one of
  them" is inference, not fact.
- **Real board shapes.** All simulations use synthetic boards (3-5 sections,
  15-25 rows, failing-rate 0-100%) at `height=19`. I did not measure the user's
  actual config or a live board. The drift is structural, so I expect it to hold,
  but the specific `gapBelow` values are shape-dependent.
- **Whether the user perceives margin 3 as better than margin 2.** §2.6 shows 3
  is required for the constraints to be jointly satisfiable. Whether one extra
  row of lookahead feels right is a judgment I cannot make from here.
- **gh-dash preview sentiment.** I found no issue complaining it wastes space —
  but I cannot prove users *like* it either. The strongest claim the evidence
  supports is that they want it *relocated* on narrow terminals (#107, #594), not
  removed.
- **neomutt/aerc defaults across distro packaging.** Verified from upstream
  source (`pager/config.c:66-68`, aerc docs). Distributions may ship different
  defaults.
- **Whether `scrollOff = 3` is satisfiable on *every* possible board**, as
  opposed to the four shapes I searched exhaustively. The search was exhaustive
  per board, not over the space of all boards. A pathological board (e.g. a
  2-line row at every anchor near a section boundary in a very short pane) could
  still fail; the implementation should keep the existing fallback branch.
