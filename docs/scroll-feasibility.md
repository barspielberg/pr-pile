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
