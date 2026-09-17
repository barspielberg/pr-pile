# Constant cursor-to-bottom distance with variable-height rows

**ACHIEVABLE — but only if you allow the row at the *top* edge to be clipped.**
Snapping the viewport's top to a whole row is what makes the goal unsatisfiable; the bottom edge and the row grid cannot both be honoured at once, and the top is the only place to absorb the remainder.

---

## 1. The answer

The goal — "the cursor always has exactly N whole rows below it" — is a constraint on the **bottom** edge of the viewport. The viewport has a fixed height `H`, so fixing the bottom fixes the top:

```
start = end - H
```

where `end` is the line just past the N-th whole row after the cursor. `end` is determined entirely by row boundaries, and `H` is a constant. So `start` is fully determined — you do not get to choose it.

The current code chooses `start` first (the highest anchor that satisfies a line-based margin) and lets the bottom fall where it may. That is the defect. Rounding is not drifting because of a lines↔rows conversion bug; **the constraint is simply being applied to the wrong edge.**

The residual is real and cannot be eliminated: `end - H` generally does **not** land on a row boundary. Its distance from the nearest boundary below is an arbitrary value in `[0, rowHeight)`. That residual has to go somewhere, and there are exactly three choices:

| Where the residual goes | Result |
|---|---|
| Snap `start` **down** to a row boundary (scroll further) | The cursor rises; fewer than N rows below. |
| Snap `start` **up** to a row boundary (scroll less) | The cursor sinks; more than N rows below, or the cursor is pushed off the bottom. |
| **Clip the top row** — start mid-row | N is held exactly. |

The project currently refuses the third option, so it is forced to oscillate between the first two. That is precisely the observed 0–3 variation.

> **Answering question 4 directly: yes, partial-row rendering is the missing piece.** It is not a cosmetic nicety. It is the degree of freedom that makes the constraint satisfiable. Without it the goal is genuinely impossible; with it the goal is trivially achievable.

---

## 2. The proof

Two independent proofs, both from a simulation of this exact board structure (2-line section headers, 1- or 2-line PR rows, first header's leading blank stripped, `anchors` built exactly as `body()` builds them).

### 2a. Brute force: whole-row tops cannot satisfy the constraint

For a 26-line board of 20 rows in a 10-line viewport, enumerate **every** legal start line and ask which ones yield exactly 2 whole rows below the cursor:

```
cursor row08: starts giving exactly 2 rows below -> any-line [6]     | whole-unit [6]    ok
cursor row09: starts giving exactly 2 rows below -> any-line [7]     | whole-unit [7]    ok
cursor row10: starts giving exactly 2 rows below -> any-line [8 9]   | whole-unit [9]    ok
cursor row11: starts giving exactly 2 rows below -> any-line [10]    | whole-unit []     IMPOSSIBLE with whole-unit top
cursor row12: starts giving exactly 2 rows below -> any-line [11]    | whole-unit [11]   ok
...
cursor row15: starts giving exactly 2 rows below -> any-line [14]    | whole-unit []     IMPOSSIBLE with whole-unit top
```

At `row11` and `row15` the *only* start that satisfies the constraint is line 10 / line 14, and neither is a row boundary. No algorithm restricted to whole-row tops can succeed here — this is a property of the board, not of any particular implementation. Meanwhile the "any-line" column is **never empty**: a clipped top always has a solution.

Note `row10` has two valid starts (`[8 9]`), one of which happens to be a boundary. That accidental slack is why whole-row snapping *sometimes* works and makes the bug look intermittent rather than systematic.

### 2b. Side-by-side: the same board, the same cursor moves

Whole-row top (algorithm B) at `cursor=row10` is forced to start at line 7, because line 8 is not a boundary and line 9 would clip the cursor's own row:

```
cursor=row10  viewport starts at line 07   -> WHOLE ROWS BELOW CURSOR = 1
   +------------------+
   |PR row 05         |
   |   L failing 05   |
   |PR row 06         |
   |   L failing 06   |
   |PR row 07         |
   |PR row 08         |
   |PR row 09         |
   |   L failing 09   |
=> |PR row 10         |
   |PR row 11         |
   +------------------+
```

Clipped top (algorithm C), same cursor, starts at line 8 — the `L failing 05` detail line is cut in half at the top — and holds the margin:

```
cursor=row10  viewport starts at line 08   -> WHOLE ROWS BELOW CURSOR = 2
   +------------------+
   |   L failing 05   |   <- clipped: this row's first line is above the fold
   |PR row 06         |
   |   L failing 06   |
   |PR row 07         |
   |PR row 08         |
   |PR row 09         |
   |   L failing 09   |
=> |PR row 10         |
   |PR row 11         |
   |PR row 12         |
   +------------------+
```

Across every cursor position on that board:

```
A  current (line margin + anchor snap) [5 4 3 2 1 1 2 2 1 2 2 1 2 2 2 3 2 2 1 0]
B  bottom-anchored, WHOLE rows only    [5 4 3 2 2 2 2 2 2 2 1 1 2 2 2 1 2 2 1 0]
C  bottom-anchored, CLIPPED top ok     [5 4 3 2 2 2 2 2 2 2 2 2 2 2 2 2 2 2 1 0]
```

C is flat at 2 through the entire steady state. A oscillates 1–3, which matches the photographed 0–3 defect. B still oscillates 1–2 — **bottom-anchoring alone is not enough**; it must be combined with clipping.

### 2c. Fuzz: 5000 random boards

Random section counts, row counts, failing-CI distribution, and viewport heights 16–29:

```
A  current (line margin + anchor snap)  boards NOT constant: 1364 / 1370   (cursor clipped: 0)
B  bottom-anchored, WHOLE rows only     boards NOT constant: 1285 / 1370   (cursor clipped: 433)
C  bottom-anchored, CLIPPED top ok      boards NOT constant:    0 / 1370   (cursor clipped: 0)
```

C is exact on every single board. Note also that B clips the **cursor's own row** in 433 frames — the failure mode you get if you try to force whole-row tops hard enough. C never does.

A separate check confirmed all three algorithms are **monotone** (the viewport never scrolls backward while the cursor moves down), so clipping introduces no regression there.

### 2d. The mechanism, isolated

Instrumenting the *current* algorithm shows it is doing exactly what it was written to do — and that this is the bug:

```
cursor  start  linesBelowCursorEnd  wholeRowsBelow
   8      5           2                  1
   9      7           2                  2
  10      9           3                  2
  11      9           2                  1
  12     11           3                  2
  13     12           2                  2
  15     15           3                  3
```

`linesBelowCursorEnd` is pinned at 2–3 — `scrollOff` is working perfectly. But `wholeRowsBelow` swings between 1 and 3, because **the same number of lines buys a different number of rows** depending on how many 2-line rows happen to fall in that span. The margin is denominated in the wrong unit. `scrollOff` is a *line* constant being used to enforce a *row* invariant.


### 2e. The code's own diagnosis is wrong

`window()` in `internal/ui/model.go` carries this note:

> *"Known defect: the cursor's distance from the bottom still drifts, because a section header is two lines but counts as one unit here."*

That is not the cause. Test it by removing multi-line headers entirely — a single section, whose header is 1 line because `body()` strips the leading blank — and varying only the row heights:

```
--- single section, 1-line header, ALL rows 1 line (uniform) ---
  A  current      steady=[4 3 2 2 2 2 2 2 2 2 2 2 2 2] distinct=[4 3 2]
  B  whole rows   steady=[4 3 2 2 2 2 2 2 2 2 2 2 2 2] distinct=[4 3 2]
  C  clipped top  steady=[4 3 2 2 2 2 2 2 2 2 2 2 2 2] distinct=[4 3 2]

--- single section, 1-line header, SOME rows 2 lines ---
  A  current      steady=[1 1 2 2 1 2 2 1 2 2 2 3 2 2] distinct=[1 2 3]
  B  whole rows   steady=[2 2 2 2 2 2 1 1 2 2 2 1 2 2] distinct=[2 1]
  C  clipped top  steady=[2 2 2 2 2 2 2 2 2 2 2 2 2 2] distinct=[2]
```

With uniform rows **every** algorithm is flat — a 1-line header is harmless because it keeps the grid uniform. Introduce 2-line **rows**, with no multi-line header present anywhere, and A and B both break while C stays flat.

So the culprit is variable **row** height (failing CI), not the 2-line headers. The headers make it worse and more visible — the isolation experiment mentioned in the brief found that adding 2-line headers started the oscillation — but they are not necessary to reproduce it. Fixing the header accounting alone, which looks like the obvious ninth attempt, **would not fix this.**


### 2f. Why the previous recommendation would have failed too

`docs/view-restructure.md` §1 recommends: raise `scrollOff` from 2 to 3, and replace the greedy "first anchor that fits" loop with a search for the anchor landing the cursor *closest* to the target margin. It reports that this "holds the margin constant on every board shape I tested."

**It measures a different quantity.** That document's metric is `gapBelow` — the number of **lines** between the cursor's last line and the bottom edge. The goal as stated in this brief is in **rows**: "the number of rows visible below it ... should be constant at 2."

A constant line gap does not imply a constant row count, because the rows occupying those lines may be 1 or 2 lines tall. Implementing that proposal and measuring both metrics:

```
--- 5 sections, many failing (40 lines, 24 rows, h=19) ---
  D prior proposal (off=3, closest)  LINES below=[2 3 3 3 2 3 2 3 2 4 3] distinct=[2 3 4]
                                     ROWS  below=[2 2 2 2 1 0 2 2 1 1 1] distinct=[2 1 0]
  C clipped top (this doc)           LINES below=[4 2 3 3 2 5 5 2 2 3 5 4 2] distinct=[4 2 3 5]
                                     ROWS  below=[2 2 2 2 2 2 2 2 2 2 2 2 2] distinct=[2]
```

The prior proposal still produces **0, 1 and 2** rows below the cursor — the exact photographed defect. Note the inversion in the last two lines: algorithm C's *line* gap varies wildly (2–5) while its *row* count is rigid at 2. The two metrics trade off against each other; you cannot hold both, and the one the user can see is rows.

That document's §2.4 conclusion — "the section header is not the culprit; the failing-check continuation line is" — agrees with §2e above and is correct. Its §2.2 claim that a board with no variable-height rows "still drifts as soon as there is more than one section" is true *in lines* and false *in rows* (see §2e: uniform rows are flat at 2 under every algorithm). The disagreement between the two documents is entirely the choice of metric, not a factual dispute.


---

## 3. The algorithm

Work in **rows**, not lines. Keep a `[]unit` list where a unit is a section header or a PR row, each with a known line height. Never slice a flat `[]string` at a line offset computed from a row count — that is the conversion that has been causing the drift.

### Core

```
H        = viewport height in lines
want     = rows to keep below the cursor (2)
units    = []unit{kind, height}, in order
unitLo[] = first line of each unit (prefix sum of heights)
total    = sum of heights
```

```go
// end = line just past the want-th whole row after the cursor
func desiredEnd(cursorRow, want int) int {
    ci  := unitIdxOfRow(cursorRow)
    end := unitLo[ci] + units[ci].height
    n   := 0
    for i := ci + 1; i < len(units) && n < want; i++ {
        end = unitLo[i] + units[i].height
        if units[i].kind == row {
            n++
        }
    }
    if n < want {
        end = total   // ran out of list: let the bottom clamp handle it
    }
    return end
}

start := desiredEnd(cursorRow, want) - H
```

Then clamp, **in this order** (each later rule may override an earlier one):

```go
// 1. never scroll past the end of the list
if start + H > total { start = total - H }
// 2. never scroll above the start of the list
if start < 0 { start = 0 }
// 3. never clip the cursor's own row off the bottom
ci := unitIdxOfRow(cursorRow)
if hi := unitLo[ci] + units[ci].height; hi - H > start { start = hi - H }
// 4. never clip the cursor's own row off the top (wins over everything)
if unitLo[ci] < start { start = unitLo[ci] }
```

Render `lines[start : start+H]`. Because `start` is a plain line index, the top row is naturally clipped when `start` falls inside a unit — that is the whole point, and it requires no special-casing at render time if you keep rendering into a flat `[]string` and slicing it. **The flat line list is fine; deriving `start` from it was the problem.** Build the line list and the unit table in the same pass.

### Edge cases

| Case | Behaviour |
|---|---|
| **Top of list** | `start` clamps to 0. The cursor sits wherever it naturally falls and the margin is not held — correct, there is nothing to scroll. Rows below the cursor exceed `want`, which is what you want at the top. |
| **Bottom of list** | `desiredEnd` returns `total`, then rule 1 pins `start = total - H`. The last few rows show fewer than `want` below (the tail `[... 2 1 0]` in the proof). Unavoidable and correct. |
| **List shorter than viewport** | `total <= H`: return everything, `start = 0`. Guard this first, before any arithmetic. |
| **2-line row at the bottom edge** | `desiredEnd` counts *whole* rows, so a 2-line row only counts once both its lines fit. It is never half-shown at the bottom. |
| **2-line row at the top edge** | Deliberately clipped — its detail line shows without its header line. This is the mechanism, not a bug. See the cosmetic note below. |
| **Cursor row itself is 2 lines** | Rules 3 and 4 guarantee both lines are visible. `cursorHeight` needs no separate margin handling; the current code's "belt and braces" comment about `scrollOff` covering it can go. |
| **Resize** | Stateless: `start` is recomputed from `cursorRow` and the new `H` every frame. Nothing to migrate. This is a real advantage of the stateless form — keep it. |
| **Filter narrows the list** | Same: recomputed from scratch. `clampCursor` already handles the cursor falling off the end. |
| **Section header at the top edge** | A clipped header shows its rule line without its blank separator, which is visually fine. If you would rather not clip a header specifically, allow `start` to snap to the header's rule line — the blank separator is the one line that is safe to drop. |

### Cosmetic refinement (optional)

A bare orphaned detail line (`└ some-check`) at the top edge reads oddly. Two mitigations, neither of which breaks the invariant:

- Render the clipped top line dimmed, or
- Prefer the clipped start only when snapping would break the margin. Since `desiredEnd - H` is forced, the only real freedom is when multiple starts satisfy the constraint (like `[8 9]` at `row10` above) — pick the boundary one there. This keeps whole-row tops in the common case and clips only when it is the *only* way to hold the margin, which measurably reduces how often clipping is visible.

---

## 4. Stateless vs. stateful — and what the goal should be

This is worth deciding deliberately, because the two behave differently going **up**.

Driving the cursor down the full list and back up:

```
Stateful (vim-style, margin at BOTH edges):
  rowsBelow going down: [5 4 3 2 2 2 2 2 2 2 2 2 2 2 2 2 2 2 1 0]
  rowsBelow coming up : [0 1 2 3 4 5 6 6 5 5 5 4 4 4 4 4 4 4 4 5]

Stateless C (bottom-anchored only):
  rowsBelow going down: [5 4 3 2 2 2 2 2 2 2 2 2 2 2 2 2 2 2 1 0]
  rowsBelow coming up : [0 1 2 2 2 2 2 2 2 2 2 2 2 2 2 2 2 3 4 5]
```

Both hold the margin going down. They differ coming back up:

- **Stateless** holds the cursor 2-from-bottom in *both* directions. That means pressing `k` scrolls the board on every keypress — the cursor never travels up through the window. Literally satisfies the stated goal, but the board never holds still while you scan upward.
- **Stateful** (store `start` in the model, adjust only when the cursor violates a margin at either edge) lets the cursor travel up through the window until it hits a top margin, then scrolls. The board holds still, which is what every editor does and almost certainly what the user actually wants.

The stateful version still needs the clipped top to hold its *bottom* margin — clipping and statefulness are independent choices. Stateful costs you free resize/filter handling (you must re-clamp `start` on `WindowSizeMsg` and after filtering), which is the one thing the current stateless design genuinely does well.

---

## 5. What real tools do

### bubbles/list — uniform height is a hard architectural assumption

`ItemDelegate` (`list/list.go:46–61`) exposes `Height() int` with **no item and no index argument** — it is a property of the delegate, shared by every item. There is no per-item height hook at any layer.

The entire pagination model rests on one integer division, `updatePagination()` at `list/list.go:793`:

```go
m.Paginator.PerPage = max(1, availHeight/(m.delegate.Height()+m.delegate.Spacing()))
```

Items-per-page is therefore a constant, and index↔position is pure arithmetic — `Index()` is `Page*PerPage + cursor`, and `GetSliceBounds` is `Page*PerPage` to `Page*PerPage+PerPage`. That round-trips correctly only if every item is exactly the same height. `populatedView()` also pads short pages by assuming the delegate emitted exactly `Height()` rows — it never measures what was actually written, so a delegate that renders taller overflows into the title and status bars (issue #503).

Charm's own answer to variable content is to **clamp**: `DefaultDelegate.Height()` returns a fixed number and `Render` hard-truncates the description to fit.

The maintainers say so explicitly. In [bubbles#323](https://github.com/charmbracelet/bubbles/issues/323) (open, filed by a maintainer, conceding "this gets asked about quite a bit"), Charm founder `meowgorithm` writes:

> "In order to calculate the total number of pages all the items must still be of a fixed height. Otherwise we'd only be able to determine the total number of pages by rendering and measuring every single list item."

Related: [#117](https://github.com/charmbracelet/bubbles/issues/117) (multi-line items flicker — fixed by clamping, via [#155](https://github.com/charmbracelet/bubbles/pull/155)), [#503](https://github.com/charmbracelet/bubbles/issues/503) (a user diagnosing the exact line and proposing summing per-item heights; unanswered), [huh#184](https://github.com/charmbracelet/huh/issues/184) and [gum#443](https://github.com/charmbracelet/gum/issues/443) (the same assumption biting downstream Charm tools).

**There is no scrolloff anywhere in bubbles** (zero hits for `scrolloff`/`scroll_off` across `list.go`, `viewport.go`, `paginator.go`). The cursor is a *page-local* index and movement **jumps a whole page** rather than scrolling: `CursorDown()` increments `m.cursor`, and on overflow calls `Paginator.NextPage()` and resets `m.cursor = 0`. Within a page the rendered window does not move at all. So `bubbles/list` does not solve this problem — it does not even attempt continuous scrolling.

**This is the most important external finding: the thing being attempted is not supported by the framework's own list widget, by design.** Building it by hand, as this project does, is the correct call — there is nothing to reuse.

### bubbles/viewport — line-based, and clips partials correctly

Purely line-based: content is `[]string`, one element per screen row, and the scroll position `yOffset` is an integer **line** count. Every API is in line units (`ScrollDown(n)`, `PageDown()` → `ScrollDown(m.Height())`).

What it gives up: **it has no concept of an item at all.** It cannot keep the cursor a constant number of *items* from the bottom, because it does not know where item boundaries are — you must maintain the item→line-offset mapping yourself. `EnsureVisible` takes a *line*, has no margin parameter, and snaps the target to the very top when out of view.

What it gets right, and this is the relevant precedent: `visibleLines()` slices `m.lines[ridx:bottom]` **without caring about item boundaries**, so an item straddling an edge renders half-clipped rather than being dropped or overflowing. Charm's own line-based viewport already does exactly what this document recommends — it clips at the edges, and that is precisely why it handles arbitrary heights when `list` cannot.

### vim — `'smoothscroll'` and `w_skipcol` are exactly this mechanism

Vim is one of only two systems surveyed that genuinely solves variable-height rows with a constant margin, and it does it the way this document proposes.

Vim's viewport top is **three** variables, not one: `w_topline` (buffer line), `w_topfill` (diff filler), and crucially `w_skipcol` — *how many columns of the top line are scrolled off the top*. `:help 'smoothscroll'`:

> "When 'wrap' is set and the first line in the window wraps part of it may not be visible, **as if it is above the window**. `<<<` is displayed at the start of the first line."

"As if it is above the window" is the clipped top row, with a marker. Before `'smoothscroll'` landed (Vim 9.0.0640) vim had exactly the quantization jumpiness this project is fighting.

The load-bearing code is `scroll_cursor_bot()` in `vim/vim src/move.c` — note that it **anchors the bottom and walks backwards**, which is the algorithm in §3:

```c
if (used + loff.height > curwin->w_height)
{
    if (do_sms)
    {
        // 'smoothscroll' and 'wrap' are set.  The above line is
        // too long to show in its entirety, so we show just a part of it.
        if (used < curwin->w_height)
        {
            int plines_offset = used + loff.height - curwin->w_height;
            used = curwin->w_height;
            curwin->w_topline = loff.lnum;
            curwin->w_skipcol = skipcol_from_plines(curwin, plines_offset);
        }
    }
    break;
}
```

Read the `else` path: **without** `do_sms` the loop just `break`s, the overflowing line is dropped entirely, and the scroll position is quantized to line boundaries — which is algorithm B, the oscillating one. With it, `plines_offset` is the exact overflow and `w_skipcol` absorbs it so `used == w_height` exactly. `plines_offset` is the residual from §1, and `w_skipcol` is where vim puts it.

Vim is also stateful: `update_topline()` gates whether to scroll at all before deciding where.

### react-window `VariableSizeList` — anchors on pixels, clips both edges

The other system that genuinely solves it. It keeps a lazily-filled prefix sum of item offsets (`createCachedBounds.ts`: each entry `{size, scrollOffset}`, where measuring item *i* requires `0..i-1` — the O(n)-amortized-to-O(1) cost of variable heights). The visible range is then found by scanning offsets (`getStartStopIndices.ts`):

```ts
if (bounds.scrollOffset + bounds.size > containerScrollOffset) break;
```

Strictly greater: an item whose *end* is past the scroll offset is included **even if its start is above it**, and the container clips it. Symmetrically at the bottom. **react-window never quantizes the scroll position to an item boundary** — partial items at both edges, by construction. That is the same freedom `w_skipcol` buys vim.

### The negative finding: nobody else in TUI-land does this

Of the TUI tools surveyed, **not one has genuinely variable-height list rows with a constant scrolloff.** They all avoid the problem:

| Tool | Variable-height rows? | Offset | Anchored edge | Partial rows | Scrolloff |
|---|---|---|---|---|---|
| **aerc** | **No** — 1 line/message; thread arrows are an inline *column*, not extra rows | State (`scroll`) | Top | No | `msglist-scroll-offset`, default **0** |
| **lazygit** / gocui | **No** — 1 line/commit; the graph is drawn *inside* that line | State (`view.oy`) | Top | No | **None** — recenters instead |
| **gitui** | No | State (`scroll_top`) | Top | No | **None** |
| **gh-dash** | No — uniform `ListItemHeight` | State (two bound indices) | Top | No | **None** |
| **bubbles/list** | **No** — structurally forbidden | n/a — paginates | n/a | No | n/a |
| **bubbles/tree** | **Yes** — multi-line nodes | State (`viewport.YOffset`) | Top | No (line-quantized) | Yes, default 5 |
| **vim** `'smoothscroll'` | **Yes** | State (`w_topline`+`w_skipcol`) | **Bottom** in `scroll_cursor_bot` | **Yes** | Yes, genuinely constant |
| **react-window** | **Yes** | Derived (DOM px) | Top (px) | **Yes, both edges** | n/a |

Two corrections to assumptions worth flagging:

- **lazygit has no scrolloff at all.** `calculateNewOrigin` in `jesseduffield/gocui/view.go` returns `oldOrigin` while the selection is visible, and **recenters** (`selectedLine - viewHeight/2`) when it isn't. The `DESIGN.md` note that "lazygit ships 2" does not match its source. Its commits are also strictly one line each — the graph is drawn inside the line.
- **aerc's threading adds no rows either.** `threadView.Update()` injects the tree prefix as a column value; the table is built with `ml.height` rows, 1:1 with screen lines.

So the honest answer to "does any Bubble Tea app do genuinely variable-height list items?" is: **`bubbles/tree`, and it is the only one.**

### bubbles/tree — the closest existing model, and its one flaw

`tree/tree.go` maintains **two parallel prefix sums** per node — `yOffset` in item space and `lineOffset` in rendered-line space:

```go
child.yOffset    = t.yOffset    + above     + i + 1
child.lineOffset = t.lineOffset + linesAbove + i + 1
above      += child.Size()   - 1   // items
linesAbove += child.Height() - 1   // rendered lines
```

and then does scrolloff arithmetic **in line space**, clamping the degenerate case up front:

```go
scrolloff := min(m.scrollOff, height/2)
minTop    := max(lineOffset-scrolloff, 0)
minBottom := min(m.viewport.TotalLineCount()-1, lineOffset+scrolloff)
if m.viewport.YOffset() > minTop {
    m.viewport.SetYOffset(minTop)
} else if m.viewport.YOffset()+height < minBottom+1 {
    m.viewport.SetYOffset(minBottom - height + 1)
}
```

Two things to copy: the one-sided `if`s (the offset is *held* when the cursor is comfortably inside — the stateful pattern), and `min(scrollOff, height/2)` up front.

One thing **not** to copy: `lineOffset` is the selected node's **first** line, so a tall selected row can still overflow the bottom edge. If you adopt this shape, measure the top margin from the row's first line and the bottom margin from its **last** — which is what §3's rules 3 and 4 do.

Note also that `bubbles/tree` is the halfway house: line space is finer than item space, so its margin is stable *in lines*, but the viewport still quantizes to whole lines and it cannot subdivide a row. **For a terminal the line is the atom**, so a line-space prefix sum is the TUI-native equivalent of react-window's pixel anchoring — and clipping a row at the top is the remaining degree of freedom that vim's `w_skipcol` represents.

### The transferable principle

From the vim/react-window pair: **put the ragged edge where you have a mechanism to hide it.** Anchoring the top and filling downward gives an unpredictable bottom edge; anchoring the bottom when the cursor moves down puts the raggedness at the top, where a clipped first row absorbs it cleanly. Every simple tool anchors the top unconditionally — which is exactly why none of them can hold a constant row margin.

### A note on the implementation that landed during this research

While this research was being written, `window()` in `internal/ui/model.go` was rewritten to take `rowStarts []int` and do exactly this:

```go
// The bottom edge: just past the scrollOff-th whole row after the cursor.
end := len(lines)
if after := cursorRow + scrollOff + 1; after < len(rowStarts) {
    end = rowStarts[after]
}
start := end - height
```

with a comment that independently reaches the same conclusion — *"that remainder has to go somewhere — snapping it away is what made the gap under the cursor drift between 0 and 3 rows. So the top row is clipped instead."* That is the §1 argument, arrived at separately.

This document therefore serves as **confirmation rather than proposal** for that change: §2a proves no whole-row-top algorithm can work, and §2c shows this shape is exact on 1370/1370 random boards. Two review points on the code as written:

- `end = rowStarts[cursorRow + scrollOff + 1]` counts *rows* via `rowStarts`, which is correct — but it measures the bottom edge from the **start** of the row after the margin. If that row is 2 lines, only its first line is guaranteed on screen. That is the same off-by-one-row flaw §5 notes in `bubbles/tree`. Whether it matters depends on whether "2 rows below" means two rows *begun* or two rows *fully shown*; §3's `desiredEnd` uses the latter.
- The guard `if start > cursorStart { start = cursorStart }` handles the cursor's row being clipped off the **top**, but there is no corresponding guard for it being clipped off the **bottom** (§3 rule 3). With `end` derived from a row *after* the cursor, that case should not arise — but it is worth a test, since it is the failure mode that bit algorithm B in 433 fuzz frames.
- `snapToAnchor` and the `anchors` slice appear to be dead once this lands.

As of the end of this research the change is in the tree and `go test ./...` is green. `TestScrollAlwaysStartsOnAWholeRow` — which asserted the whole-row-top invariant §2a proves unsatisfiable — has been removed and replaced by `TestGapBelowCursorIsConstantWhileScrolling`, which asserts the row-based metric §2f argues is the correct one. That is the right trade, and it is the trade the previous eight attempts could not make while the old test stood.


---

## 6. Recommendation

**The goal is achievable. Do not restructure the layout.**

1. **Allow the top row to be clipped.** This is the one change that makes the constraint satisfiable, and it is what vim and react-window both do. The existing `TestScrollAlwaysStartsOnAWholeRow` test encodes the very assumption that makes the goal impossible — it must be deleted or inverted, not worked around. **This is the decision the previous eight attempts were missing**; none of them could succeed while that invariant held.

2. **Anchor the bottom, derive the top.** Compute `end` from the cursor row plus `want` whole rows, then `start = end - H`. Never pick `start` first.

3. **Count rows, not lines.** Build a `[]unit` table with per-unit heights in the same pass as the line list, and define the margin in rows. The current `scrollOff = 2` is a *line* constant enforcing a *row* invariant — that mismatch is the bug (§2d).

4. **Do not raise `scrollOff` to 3 or add a closest-anchor search.** §2f shows that proposal still yields 0–3 rows below the cursor. It optimizes line gap, which is not the visible goal.

5. **Do not fix the 2-line header accounting.** §2e shows the defect reproduces with no multi-line header present. That is the most tempting ninth attempt and it would fail.

6. **Decide stateless vs. stateful deliberately** (§4). Stateless is a ~30-line change and preserves the free resize/filter handling that the current design does well, but it scrolls on every `k`. Stateful matches every editor surveyed — all of aerc, gocui, gitui, bubbles/tree and vim hold the offset as state and nudge it one-sidedly — and keeps the board still when scanning upward, at the cost of re-clamping on resize and filter changes.

   **My recommendation is stateful**, because "constant distance from the bottom" is really a description of what downward scrolling should feel like, not an invariant worth holding while moving up. Note this means the literal stated goal — constant in *both* directions — is achievable but probably not what is wanted.

### Is "constant cursor-to-bottom distance" the right goal?

Mostly yes, with one correction. The right goal is: **"moving down, the cursor never comes closer than N whole rows to the bottom edge; the board holds still otherwise."** That is a one-sided constraint, it is what every surveyed tool implements, and it is achievable with clipping. The two-sided version (constant in both directions) is also achievable but makes `k` scroll the board on every keypress.

The layout restructure the user floated ("make each item take the same space") **would** work — with uniform rows every algorithm here is flat (§2e) — but it is the expensive way to buy a property available from a ~30-line scroll change, and it costs the failing-check names.

---

## 7. What I could not verify

- **Rendering a clipped row in this codebase.** The proof operates on the line list and `start` index, which is exactly what `View()` slices, so clipping should require no rendering change. But I did not build and run the TUI, and I did not check whether a clipped selected-row background band or a lipgloss-styled line survives being shown without its first line. The selected row is never clipped by construction, so the risk is confined to the *unselected* top row.
- **The photographed defect.** I reproduced a 0–3 row variation matching the description, but I never saw the photo, and I did not run the real binary against real GitHub data. My simulation reconstructs `body()`'s line/anchor layout from reading the source; I did not extract and link the actual functions into a test harness.
- **Real board shapes.** Section sizes, failing-CI frequency and terminal heights are synthetic. The fuzz covers heights 16–29 and 0–8 rows per section, which should cover the stated 20–28 viewport, but the real distribution may differ.
- **`placeholderRows` and the loading/`Pending` states.** My model covers `Ready` sections only. Placeholder blanks, the `Failed` error line, and the one-line collapsed empty section all add units my simulation does not model. They are all fixed-height and should slot into the unit table without changing the argument, but I did not verify that.
- **Whether the `<<<` marker convention is worth adopting.** Vim marks a clipped top line; whether that is desirable in a 20-row PR board is a taste question I did not resolve.
- **aerc's exact upstream.** The canonical repo is sr.ht `~rjarry/aerc`; I read the GitHub mirror `rjarry/aerc`. Line numbers may differ from upstream tip.
- **bubbles v1 vs v2.** The `bubbles/list` findings were verified identical on the `v0.21.0` tag and current `main`, but `tree` was read on `main` only, and this project pins `bubbles` indirectly via `bubbletea v1.3.10`.
