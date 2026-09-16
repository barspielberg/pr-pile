# prs-mng — Visual Design Specification

Implementation-ready visual spec for the `prs-mng` board. Derived from
`docs/design-research.md` (requirements R1–R19) and the current implementation in
`internal/ui/render.go`.

This document is normative. Every width, glyph, and color index below is a
decision, not a suggestion. Where a judgment call was made, it is stated inline
in one sentence.

---

## 0. The governing idea

Three changes carry almost all of the improvement:

1. **Collapse four wide text columns into one 7-cell glyph cluster.** The eye
   travels 19 cells from row start to title instead of ~38. (R5)
2. **Selection becomes a full-row background plus a solid left bar**, not a
   floating triangle. (R1)
3. **Only actionable state gets saturated color.** Everything settled, clean, or
   informational is unstyled or faint. Red is scarce enough to be preattentive. (R4, R10)

The design is legible in monochrome first, 16-color second. Color never carries
information alone. (R3, R16)

---

## 1. Row anatomy

### 1.1 Column map (FULL tier, the reference layout)

One PR = one line. Columns, left to right, at fixed 0-based cell offsets:

| # | field    | offset | width | alignment | fixed/flex | content |
|---|----------|--------|-------|-----------|------------|---------|
| 1 | mark     | 0      | 1     | —         | fixed      | `▌` when selected, else space |
| 2 | gutter   | 1      | 1     | —         | fixed      | space |
| 3 | tree     | 2      | 2     | left      | fixed      | `╭╴` `│ ` `╰╴` or two spaces |
| 4 | number   | 4      | 6     | left      | fixed      | `#3248` padded right |
| 5 | gutter   | 10     | 1     | —         | fixed      | space |
| 6 | status   | 11     | 7     | left      | fixed      | glyph cluster, §3 |
| 7 | gutter   | 18     | 1     | —         | fixed      | space |
| 8 | title    | 19     | flex  | left      | **FLEX**   | clipped with `…` on display width |
| 9 | gutter   | —      | 1     | —         | fixed      | space |
| 10| age      | —      | 3     | **right** | fixed      | `2h`, `1d`, `3w` |

**Exactly one column flexes: title.** Everything else is fixed. This is the
mechanical fix for "columns are too widely spaced": slack has only one place to
go, so the status cluster is pinned to columns 11–17 at every terminal width.
(R5, R13)

### 1.2 Budget arithmetic

```
fixed = mark(1) + gut(1) + tree(2) + number(6) + gut(1)
      + status(7) + gut(1) + gut(1) + age(3)
      = 23

titleWidth = terminalWidth - 23        [FULL tier]
```

Worked examples:

| terminal | titleWidth |
|---|---|
| 120 | 97 |
| 100 | 77 |
| 80  | 57 |
| 72  | 49 (MID tier: see §6, age dropped → `terminalWidth - 19`) |

**Clamp rule (R7, anti-pattern 4.7):** `titleWidth = max(0, computed)`. When
`titleWidth <= 3`, render the title as the empty string rather than a bare `…`.
All padding uses `max(0, w - lipgloss.Width(s))` — never `strings.Repeat` with an
unclamped count.

### 1.3 Number column

6 cells holds `#99999`. acme/monorepo is at ~#3200, so 6 is right for years.
If a number exceeds 6 cells, let it overflow into the following gutter by
one cell and drop the gutter — never push the status cluster right. (Judgment: a
7-digit PR number is a decade away; keeping status pinned is worth more than a
perfect gutter in a case that will not occur.)

### 1.4 Age column

`updatedAt` rendered as the largest single unit: `<1h` → `now`, `<24h` → `NNh`,
`<7d` → `Nd`, else `Nw`. Right-aligned in 3 cells so the digits line up. This is
new data on the row (it was not rendered before) and it earns its 4 cells because
"stale vs fresh" is a triage input the user currently has to infer from nothing.

### 1.5 The continuation line (failing checks)

Kept, demoted. (Resolved: research open question 4.)

```
  │                └ unit-tests / node-20, e2e / checkout-flows
   ^tree spine      ^aligned to the TITLE column (offset 19)
```

Rules:
- Indented to the **title column** (offset 19, or 14 at MIN tier), so it reads as
  a child of the title rather than as a new row. The current implementation
  indents it to an arbitrary position where it competes with titles.
- Prefixed `└ ` — a different glyph family from status (`✓ ✗ ◐ ○`) and from
  selection (`▌`). (R19)
- Rendered in **`muted`**, not `error`. The red `✗2` in the status cell already
  says "this is broken"; the continuation line says *which*, which is detail, not
  alarm. This is the single change that stops it competing with PR titles. (R9)
- Clipped to one line at `terminalWidth - titleCol`. Never wraps.
- If the PR is inside a stack, columns 2–3 carry `│ ` to keep the spine
  unbroken; otherwise two spaces.
- **Always shown** when `FailedGates` is non-empty, at every pane height. In a
  20-row pane it costs one row and answers "why is it red" without an action —
  that is a better trade than one more clean PR row.
- Multiple failing gates are joined with `, ` on one line. If they do not fit,
  clip with `…`; do not add a second continuation line.

### 1.6 Section header

```
━━ MINE ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 5
```

- Full-terminal-width rule built from `━` (U+2501, HEAVY HORIZONTAL).
- `━━ ` + **NAME (uppercased)** + ` ` + fill + ` ` + count + ` `.
- Fill width = `max(0, terminalWidth - len("━━ NAME ") - len(" N "))`.
- Name uppercased; this is the only uppercased text on the board, which alone
  makes headers identifiable without reading them. (R8)
- The count is required by R8 and sits right-aligned at the terminal edge, where
  the eye already stops.
- A blank line precedes every section header except the first. That blank line
  is the only pure-whitespace row in the design and it is what separates
  sections; there are no other separators or padding rows. (R18 — explicit
  density decision: maximum density, one blank line per section boundary.)

Non-Ready states reuse the same header, with the body replaced:

```
━━ TEAMMATES ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ ⠹
                   loading…
```
```
━━ ALL OPEN ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ ! 
                   search timed out after 30s
```
Empty Ready section: the count reads ` 0 ` and the body is a single `muted` line
`                   —` at the title column.

---

## 2. Selection treatment

**Decision (resolves research open question 1): selection is a chosen background
fill plus a solid left bar. Not `reverse`.**

Rationale: `reverse` swaps the row's own foreground into the background, which
destroys status color on exactly the rows that most need it — a failing red row
would become a red block with the CI glyph inverted out of it. A background fill
keeps the status glyph's own color intact on top of it.

### 2.1 Rendering

The selected row (and its continuation line, if any) is rendered as:

| element | treatment |
|---|---|
| column 0 (mark) | `▌` (U+258C LEFT HALF BLOCK) in `accent` |
| columns 1..end | `selBg` background applied to the **full terminal width**, padded with spaces to the right edge |
| title text | **bold**, `fg` foreground |
| all other cells | keep their own semantic foreground (§5), drawn on `selBg` |

The background must extend to the terminal's right edge, not to the end of the
text. A partial fill reads as a highlight artifact rather than a selection.

### 2.2 `selBg` choice and the light/dark problem

`selBg` = **ANSI index 8** (bright black / "dark grey" in dark themes,
"light grey" in light themes) as the background.

This is the one token where 8 is genuinely safe as a *background*: in every
mainstream scheme, index 8 sits between index 0 and index 7 in luminance, so it
contrasts with the terminal's own background whichever end that background sits
at. Index 8's danger (§5) is as a *foreground* against index 0, not as a
background behind default foreground text.

Additionally, `fg` on the selected row is forced to **ANSI 15** on dark
backgrounds and **ANSI 0** on light. Since background detection is unreliable
(R15, research §4.8), the default before detection resolves is: **do not set a
foreground on the selected row at all** — inherit the terminal's default
foreground, which is guaranteed to contrast with the terminal's own background
and is close enough to contrasting with index 8 in both directions. Detection, if
it arrives, only upgrades this to the explicit 15/0. First paint is correct
without it.

### 2.3 Selected + failing (the collision, addressed explicitly)

A row that is both selected and failing renders as:

```
▌ ╰╴#3254  ✗2 ○    feat(api-service): PROJ-2039 expose cutoff in admin API     1h
^bar  ^^^^^^^^^^^^^^ all of this on selBg; the ✗2 stays error-red on top
```

Three channels stay independent and none is lost:
- **Selection** = left bar `▌` + background fill (a *row-level* property).
- **Failure** = the `✗` glyph + `error` foreground (a *cell-level* property).
- **Position** = the bar is at column 0 where no other element ever appears.

Because the left bar lives in column 0 and column 0 is otherwise always blank,
selection is detectable by scanning a single column — the cheapest possible
search, and it survives even if the background fill is lost to an exotic theme.
That redundancy is the answer to "unmistakable from two feet away". (R1)

**Never** use faint/dim for selection. (Research §4.9: lazygit #1845 — dimming
reads as "disabled".)

---

## 3. Status encoding

**Decision (resolves research open question 8): four dimensions, one 7-cell
fixed cluster, each dimension owning its own sub-slot.** Booleans do not get
their own columns (too expensive) and do not modify the whole row (would collide
with selection). They get a shared 1-cell "blocker" slot, because draft and
conflicted are mutually rankable and rarely co-occur in a way that matters.

### 3.1 Sub-slot map inside the status cell (offsets relative to column 11)

| sub-offset | width | dimension | contents |
|---|---|---|---|
| +0 | 1 | CI | state glyph |
| +1 | 1 | CI | failing count digit, or space |
| +2 | 1 | — | gutter |
| +3 | 1 | review | state glyph |
| +4 | 1 | — | gutter |
| +5 | 1 | blocker | conflict or draft glyph, or space |
| +6 | 1 | — | gutter / reserved |

Total 7. Every sub-slot is always emitted, blank when absent, so nothing ever
shifts. (R13)

### 3.2 CI state

| state | glyph | codepoint | count slot | color token | reads without color as |
|---|---|---|---|---|---|
| passing | `✓` | U+2713 | space | `ok` | check mark |
| N failing | `✗` | U+2717 | `N` (1–9, `+` if ≥10) | `error` | cross + a number |
| running | `◐` | U+25D0 | space | `warn` | half-filled circle |
| none | `·` | U+00B7 | space | `muted` | a small dot |

All four silhouettes are distinct (check / cross / half-disc / dot) — this is the
specific failure mode research §4.3 names (several states as one `•` in different
hues), and it is avoided. The failing count is the only numeral in the cluster,
which makes red rows scannable by shape alone.

### 3.3 Review state

| state | glyph | codepoint | color token | reads without color as |
|---|---|---|---|---|
| approved | `✓` | U+2713 | `ok` | check mark |
| changes requested | `✗` | U+2717 | `error` | cross |
| review required | `○` | U+25CB | `attention` | hollow circle |
| unknown / none | `·` | U+00B7 | `muted` | small dot |

Review reuses `✓`/`✗` deliberately: they mean the same thing (good / bad) in a
different column, and column position disambiguates. Adding a fifth and sixth
glyph shape would cost the user more than it buys. (R17)

`○` for "review required" is the one glyph that is *hollow* — an unfilled circle
reads as "not yet done" without any convention to learn, and it is the most
common actionable state on the board.

### 3.4 Blocker slot (draft, conflicts)

One cell, one glyph, precedence **conflicts > draft**:

| condition | glyph | codepoint | color token | reads without color as |
|---|---|---|---|---|
| `Mergeable == "CONFLICTING"` | `!` | U+0021 | `error` | exclamation mark |
| `IsDraft` (and not conflicting) | `~` | U+007E | `muted` | tilde |
| neither | space | — | — | blank |

Judgment: a conflicted draft is still primarily conflicted — the conflict is the
thing that will bite. Both are ASCII, so this slot cannot be broken by any font.

Draft additionally renders the **whole row's title in `muted`** rather than
default foreground. That is a second channel for draft (weight, not color), and
it directly serves R10: a draft is by definition not actionable, so it should
recede. This is the only case where a boolean modifies the row.

### 3.5 Monochrome read-out

With all color stripped, the cluster still reads:

```
✗2 ○ !    failing (2 checks), review required, conflicted
✓  ✓      passing, approved, clean
◐  ○      running, review required, clean
·  · ~    no CI, no review decision, draft
```

Every distinction survives. (R3)

### 3.6 Glyph fallback (R11)

All eight glyphs above are **plain Unicode (BMP), not Nerd Font PUA
codepoints** — resolving research open question 7 in favor of plain Unicode.
Nerd Font glyphs carry the #290 version-skew risk and inconsistent advance
widths for no legibility gain at this size.

If a glyph is missing, the terminal substitutes a box or blank; because every
glyph occupies exactly one fixed cell and the cell is padded to width regardless
of what is drawn in it, alignment cannot break. Nothing else on the row depends
on the glyph rendering.

A config escape hatch `glyphs = "ascii"` maps the set to
`+` (passing), `x` (failing), `*` (running), `.` (none), `+` (approved),
`x` (changes), `o` (review required), `!`, `~`. Same widths, same positions.
Building this is cheap because the widths are already fixed.

---

## 4. Visual hierarchy

Three levels, differing on **four channels at once** so that losing any one
channel does not collapse the hierarchy. (R9)

| | section header | PR row | secondary detail |
|---|---|---|---|
| **case** | UPPERCASE | mixed case | mixed case |
| **weight** | bold | regular (title bold only when selected) | faint |
| **color** | `header` (ANSI 6) for name and rule; `muted` for count | `fg` title, semantic status | `muted` throughout |
| **indent** | column 0 | column 0 (data starts col 2) | column 19 (title column) |
| **shape** | full-width `━` rule | no rule | `└ ` leader |

Actionability ordering *within* a level (R10) is expressed by color saturation,
not by position — since section order and PR order are the user's configuration,
not the designer's:

- **Loud** (`error`, `attention`): failing CI, changes requested, conflicts,
  review required.
- **Quiet** (`ok`): passing, approved. These are settled states; they need to be
  *confirmable*, not *noticeable*. Green is the least saturated thing on the row.
- **Silent** (`muted`): drafts, no-CI, age, continuation lines.

A board where everything is fine has almost no color in it. That is the point:
the user can answer "do I need to do anything" from the presence or absence of
red and amber, without reading a single row. (R4, interpreted as visual weight
within user-configured order.)

---

## 5. Color tokens

Seven tokens. All are ANSI indices 0–15 so they resolve through the user's own
terminal theme. (R2, R14, R17)

| token | ANSI | meaning | used for | light+dark safe? |
|---|---|---|---|---|
| `fg` | *unset* (terminal default) | primary text | PR titles | **yes — by construction** |
| `error` | 1 | broken, blocking, needs a fix | `✗` (CI), `✗` (review), `!` conflicts | **yes** |
| `attention` | 3 | waiting on someone, in flight | `◐` running, `○` review required | **yes** |
| `ok` | 2 | settled, no action | `✓` passing, `✓` approved | **yes** |
| `header` | 6 | structural chrome | section name + `━` rule | **yes** |
| `accent` | 4 | identity | PR number, selection bar `▌` | **yes** |
| `muted` | *unset fg + `Faint(true)`* | secondary, recessive | continuation lines, age, draft titles, `·`, counts, footer | **yes — see below** |

Plus one background-only value:

| token | ANSI | meaning | used for |
|---|---|---|---|
| `selBg` | 8 (as background) | the selected row | full-width row fill |

### 5.1 Resolving research open question 5 — muted text

**Decision: `muted` is `Faint(true)` on the terminal's default foreground. It is
NOT ANSI index 8, and NOT a 256-palette grey.**

Reasoning, and this is the one that bites hardest in practice:

- **Index 8 as a foreground is unsafe.** On a light theme, index 8 is often a
  *light* grey (it is "bright black", and many light schemes brighten it toward
  the background), which can approach invisibility on a white background. The
  current implementation uses `Color("8")` for tree glyphs and drafts and is
  therefore already carrying this bug on light themes.
- **A 256-palette grey is worse.** It is a fixed RGB-ish value that stops
  following the theme entirely — precisely the k9s #1234 failure. It will be
  correct on the author's theme and wrong elsewhere.
- **`Faint` is relative, not absolute.** SGR 2 tells the terminal to reduce the
  intensity of whatever the current foreground is. On a dark theme that darkens;
  on a light theme that lightens. It is the only "dimmer" that is correct on both
  by construction, and it degrades to plain default foreground (still legible) on
  terminals that ignore SGR 2.

`muted` therefore never disappears and never needs a light/dark pair.

### 5.2 Notes on the other tokens

- **Index 7 is not used anywhere.** It is the token research flags as most
  theme-unstable (it is the *default* foreground on many dark schemes and nearly
  the background on many light ones). Using terminal-default foreground instead
  of explicit 7 gets the same result and is always correct.
- **Index 15 and index 0 are not used as foregrounds.** The current
  `selStyle` uses 15, which is white-on-white on light themes. Replaced by
  "unset foreground + bold" on the selected row (§2.2).
- `error` uses **1, not 9**. Index 1 is the theme's "red"; index 9 is "bright
  red", which on light themes is often a pale, low-contrast pink. 1 is safer at
  both ends. Optional 256/truecolor enhancement may substitute 9 on a detected
  dark background — this is polish only. (R16)
- `accent` at index 4 (blue) for the PR number keeps the identity column visually
  distinct from every status color without adding a hue that means "status". The
  current implementation uses 6 for both section headers and numbers, which
  makes the two read as the same level of information.

### 5.3 Capability tiers (R16)

| profile | rendering |
|---|---|
| **monochrome** | glyphs + bold/faint + `▌` + `━` rules + indentation. Fully usable; §3.5 is the proof. |
| **16 color (baseline)** | the table above, exactly. This is the correctness target. |
| **256 / truecolor** | optional only: `selBg` may use a specific dark grey (e.g. 236) when a dark background is *confirmed*; `error` may use 9 on confirmed dark. No new information may be encoded at these tiers. |

---

## 6. Responsive behavior

Four named tiers with hard breakpoints and a **mode change** at each, not a
gradual squeeze. (R7, P8 — resolves research open question 3.)

| tier | width | columns present | title width |
|---|---|---|---|
| **FULL** | ≥ 76 | mark, tree, number, status(7), title, age | `w - 23` |
| **MID** | 60–75 | mark, tree, number, status(7), title | `w - 19` |
| **NARROW** | 48–59 | mark, tree, number, status(3), title | `w - 15` |
| **MIN** | 40–47 | mark, tree, number, status(3), title | `w - 15` |
| **BELOW MIN** | < 40 | single message, no board | — |

### 6.1 Drop order and why

1. **Age goes first** (FULL→MID). It is the only field that is purely contextual
   — it changes how urgent something feels but never changes what the user does.
2. **Review glyph goes second** (MID→NARROW). The status cell compresses from 7
   to 3 cells: `[CI glyph][CI count][blocker]`. Review state is the field most
   likely to be re-derived by opening the PR anyway; CI and conflicts are the
   ones that decide whether opening the PR is even worth it.
3. **Nothing else is ever dropped.** Mark, tree, number, CI, blocker, and title
   are the irreducible board. Below 40 columns there is no honest layout, so the
   board is replaced by:

```
  terminal too narrow
  (need 40 cols)
```

Tree glyphs are **never** dropped, at any width. Losing them would silently
change the meaning of adjacent rows (R19), which is worse than any truncation.

### 6.2 Breakpoint arithmetic

Thresholds are set so the title never falls below a readable floor:

- FULL requires `w - 23 >= 53`  → `w >= 76`
- MID requires `w - 19 >= 41`  → `w >= 60`
- NARROW/MIN require `w - 15 >= 25` → `w >= 40`

Minimum viable width: **40 columns.**

### 6.3 Stability guarantee

Within a tier, only the title's width changes; every other column keeps its exact
offset. Across tiers, columns 0–10 (mark, tree, number) never move at all, so the
left edge of the board is completely stable from 40 to 400 columns. (R13)

---

## 7. Vertical economy

The 20-row pane is the design target, not a degraded case. (R6)

### 7.1 Chrome budget — every non-data row justified

| row | cost | verdict |
|---|---|---|
| global header (`PRs  acme/monorepo  ⠹`) | 1 | **CUT.** Resolves research open question 6: no global header. |
| blank line above each section | 1 each | **KEEP.** It is the only section separator; removing it makes headers collide with the row above. |
| section header | 1 each | **KEEP.** Required by R8, carries the count. |
| blank line above footer | 1 | **CUT.** The footer is visually distinct by being faint. |
| footer | 1 | **KEEP, merged.** |
| status message line | 1 | **CUT as a separate line.** Merged into the footer. |

**Global header cut — reasoning (open question 6):** it carried three things:
the app name (the user just pressed a key to open it — they know), the repo
(single-repo tool; constant, so it is chrome the user stops seeing), and the
loading spinner. The repo and the spinner move to the right end of the footer,
where the row is already being paid for. The app name is deleted outright. That
is one row back — 5% of a 20-row pane — for zero information loss. (Research
§4.4: gh-dash #671.)

### 7.2 Footer

One line, always, `muted` throughout:

```
  j/k move · enter open · r reload · q quit                acme/monorepo ⠹
```

- Left: keybindings.
- Right, right-aligned to the terminal edge: repo, then the spinner cell (one
  cell, blank when idle — so nothing shifts when loading starts or stops).
- When a status message is present, it **replaces** the keybinding text for 3
  seconds, then reverts. Same row, no extra row. The keybindings are the least
  urgent thing on the board and are the correct thing to displace.
- Below 60 columns the keybinding text shortens to `j/k · ⏎ · r · q`; below 48
  the repo name is dropped, leaving keys + spinner.

### 7.3 Row budget in a 20-row pane

20 rows − 1 footer = **19 list rows.** With three sections and one failing PR,
that is 3 headers + 2 blanks + 14 data rows — **74% data.** R6 requires a
majority; this clears it comfortably.

Compare with the current design: 20 − 1 header − 1 blank − 1 footer = 17 list
rows, 71% data with the same content. The saving is modest in the small case and
grows with section count, but the larger win is that the row that was cut was
pure chrome.

### 7.4 Scrolling

Unchanged from the current implementation: scroll minimally to keep the cursor
line visible; do not center. A row may be 1 or 2 lines tall (continuation line),
so the scroll offset must continue to be computed in **lines**, not rows. When a
selected row has a continuation line, both lines must be brought into view
together — never scroll to a position that shows the continuation line without
its parent row.

---

## 8. Stacked-PR tree glyphs

Columns 2–3, exactly 2 cells, `muted`. (R19)

| position in chain | glyph | codepoint |
|---|---|---|
| first (base of stack) | `╭╴` | U+256D, U+2574 |
| middle | `│ ` | U+2502, space |
| last (top of stack) | `╰╴` | U+2570, U+2574 |
| not stacked | `  ` | two spaces |
| continuation line, inside a stack | `│ ` | U+2502, space |
| continuation line, not in a stack | `  ` | two spaces |

These are the glyphs the current implementation already produces; the spec keeps
them and changes only their color and their relationship to neighbors.

### 8.1 Why they cannot be confused with status or selection

Three separations, any one of which would be sufficient:

1. **Glyph family.** Tree glyphs are box-drawing (U+2500 block); status glyphs
   are dingbats and geometric shapes (U+2713, U+2717, U+25D0, U+25CB) plus two
   ASCII marks. No codepoint is shared.
2. **Position.** Tree is columns 2–3, always. Status is columns 11–17, always.
   Selection is column 0, always. Three disjoint, never-moving regions.
3. **Color.** Tree is `muted` (faint) at all times, including on the selected
   row. Status glyphs are never faint. Selection is `accent`. The selection bar
   `▌` is a *filled block*; no tree glyph is filled.

The `│` spine on a continuation line inherits the same `muted` treatment as the
tree, so a two-line stacked failing row reads as one nested unit rather than as
two rows at different depths.

---

## 9. Mockups

All mockups below are **character-exact**: box borders mark the true terminal
edge, and every column offset is literal. Colors cannot be shown in ASCII, so a
token key follows each.

### 9.1 Full board at 120 columns

Contains: a stacked chain (`#3248` → `#3251` → `#3254`), a selected row
(`#3254`, also conflicted), a failing PR with its continuation line (`#3248`), a
draft (`#3260`), a conflicted PR (`#3254`), and a changes-requested PR (`#3199`).

```
+------------------------------------------------------------------------------------------------------------------------+
|━━ MINE ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 5 |
|  ╭╴#3248  ✗2 ○    feat(api-service): PROJ-2037 refuse order plan writes after cutoff                                  2h|
|  │                └ unit-tests / node-20, e2e / checkout-flows                                                         |
|  │ #3251  ◐  ○    feat(api-service): PROJ-2038 backfill order_cutoff_at column                                        2h|
|▌ ╰╴#3254  ✓  ✓ !  feat(api-service): PROJ-2039 expose cutoff in admin API                                             1h|
|    #3199  ✓  ✗    fix(billing-service): PROJ-1994 stop double-counting idle vehicles                                    1d|
|    #3260  ·  · ~  chore(telematics-ms): PROJ-2101 bump @acme/rabbit to 4.2.0                                     4h|
|                                                                                                                        |
|━━ REVIEW REQUESTED ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 2 |
|    #3241  ✓  ○    feat(admin-console): PROJ-1880 driving sessions tab                                                3h|
|    #3233  ✗1 ○    fix(auth-service): PROJ-2004 guard null context closure                                              6h|
|                   └ lint / eslint                                                                                      |
|                                                                                                                        |
|━━ INVOLVED ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 1 |
|    #3188  ✓  ✓    docs(monorepo): PROJ-1770 document rabbit retry semantics                                           2d|
|  j/k move · enter open · r reload · q quit                                            acme/monorepo              ⠹|
+------------------------------------------------------------------------------------------------------------------------+
```

**Color key:**

| element | token |
|---|---|
| `━━ MINE ━━…` rule and name | `header` (6), bold |
| trailing ` 5 ` count | `muted` (faint) |
| `╭╴ │ ╰╴` tree | `muted` (faint) |
| `#3248` numbers | `accent` (4) |
| `✗2` `✗` `!` | `error` (1) |
| `◐` `○` | `attention` (3) |
| `✓` | `ok` (2) |
| `·` `~` | `muted` (faint) |
| PR titles | `fg` (terminal default) |
| `#3260` title (draft) | `muted` (faint) |
| `└ unit-tests / node-20, …` | `muted` (faint) |
| `2h` `1d` age | `muted` (faint) |
| row `#3254` cols 1→120 | `selBg` background (8); title bold, `fg` |
| `▌` on row `#3254` | `accent` (4) |
| footer | `muted` (faint) |

Note what the color key shows: on a board with 8 PRs, exactly **four cells** are
red and **three** are amber. The user's eye lands on `#3248` (failing), `#3199`
(changes requested), `#3254` (conflicts), and `#3233` (failing) without reading
anything. That is the "what do I need to do right now" answer.

### 9.2 Same board at 80 columns (FULL tier, title clipped)

```
+--------------------------------------------------------------------------------+
|━━ MINE ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 5 |
|  ╭╴#3248  ✗2 ○    feat(api-service): PROJ-2037 refuse order plan writes aft…  2h|
|  │                └ unit-tests / node-20, e2e / checkout-flows                 |
|  │ #3251  ◐  ○    feat(api-service): PROJ-2038 backfill order_cutoff_at col…  2h|
|▌ ╰╴#3254  ✓  ✓ !  feat(api-service): PROJ-2039 expose cutoff in admin API     1h|
|    #3199  ✓  ✗    fix(billing-service): PROJ-1994 stop double-counting idle v…  1d|
|    #3260  ·  · ~  chore(telematics-ms): PROJ-2101 bump @acme/rabbit to…  4h|
|                                                                                |
|━━ REVIEW REQUESTED ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 2 |
|    #3241  ✓  ○    feat(admin-console): PROJ-1880 driving sessions tab        3h|
|    #3233  ✗1 ○    fix(auth-service): PROJ-2004 guard null context closure      6h|
|                   └ lint / eslint                                              |
|                                                                                |
|━━ INVOLVED ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 1 |
|    #3188  ✓  ✓    docs(monorepo): PROJ-1770 document rabbit retry semantics   2d|
|  j/k move · enter open · r reload · q quit             acme/monorepo     ⠹|
+--------------------------------------------------------------------------------+
```

Same color key as §9.1. **Every column left of the title sits at the identical
offset as at 120 columns.** Only the title's clip point moved.

### 9.3 MID tier at 72 columns (age dropped)

```
+------------------------------------------------------------------------+
|━━ MINE ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 5 |
|  ╭╴#3248  ✗2 ○    feat(api-service): PROJ-2037 refuse order plan writes…|
|  │                └ unit-tests / node-20, e2e / checkout-flows         |
|  │ #3251  ◐  ○    feat(api-service): PROJ-2038 backfill order_cutoff_at…|
|▌ ╰╴#3254  ✓  ✓ !  feat(api-service): PROJ-2039 expose cutoff in admin A…|
|    #3199  ✓  ✗    fix(billing-service): PROJ-1994 stop double-counting id…|
|    #3260  ·  · ~  chore(telematics-ms): PROJ-2101 bump @acme/rabbi…|
|                                                                        |
|━━ REVIEW REQUESTED ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 2 |
|    #3241  ✓  ○    feat(admin-console): PROJ-1880 driving sessions tab  |
|    #3233  ✗1 ○    fix(auth-service): PROJ-2004 guard null context closure|
|                   └ lint / eslint                                      |
|                                                                        |
|━━ INVOLVED ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 1 |
|    #3188  ✓  ✓    docs(monorepo): PROJ-1770 document rabbit retry seman…|
|  j/k · ⏎ · r · q                             acme/monorepo       ⠹|
+------------------------------------------------------------------------+
```

### 9.4 Minimum supported width — 44 columns (MIN tier)

Status compressed to 3 cells: `[CI glyph][CI count][blocker]`. Review glyph
dropped. Title column moves to offset 14.

```
+--------------------------------------------+
|━━ MINE ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 5 |
|  ╭╴#3248  ✗2  feat(api-service): PROJ-2037 …|
|  │           └ unit-tests / node-20, e2e /…|
|  │ #3251  ◐   feat(api-service): PROJ-2038 …|
|▌ ╰╴#3254  ✓ ! feat(api-service): PROJ-2039 …|
|    #3199  ✓   fix(billing-service): PROJ-1994…|
|    #3260  ·  ~ chore(telematics-ms): AF-13…|
|                                            |
|━━ REVIEW REQUESTED ━━━━━━━━━━━━━━━━━━━━━ 2 |
|    #3241  ✓   feat(admin-console): AF-128…|
|    #3233  ✗1  fix(auth-service): PROJ-2004 g…|
|              └ lint / eslint               |
|                                            |
|━━ INVOLVED ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 1 |
|    #3188  ✓   docs(monorepo): PROJ-1770 doc…|
|  j/k · ⏎ · r · q                          ⠹|
+--------------------------------------------+
```

At 40 columns (the absolute minimum) the same layout holds with a 25-cell title.
Below 40:

```
+--------------------------------------+
|  terminal too narrow                 |
|  (need 40 cols)                      |
+--------------------------------------+
```

### 9.5 The 20-row pane — the default case

100 columns × 20 rows. 19 list rows + 1 footer. The board above fits entirely
with 4 rows to spare, which is the normal state for a 6-rule config where most
sections are small.

```
+----------------------------------------------------------------------------------------------------+
|━━ MINE ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 5 |
|  ╭╴#3248  ✗2 ○    feat(api-service): PROJ-2037 refuse order plan writes after cutoff              2h|
|  │                └ unit-tests / node-20, e2e / checkout-flows                                     |
|  │ #3251  ◐  ○    feat(api-service): PROJ-2038 backfill order_cutoff_at column                    2h|
|▌ ╰╴#3254  ✓  ✓ !  feat(api-service): PROJ-2039 expose cutoff in admin API                         1h|
|    #3199  ✓  ✗    fix(billing-service): PROJ-1994 stop double-counting idle vehicles                1d|
|    #3260  ·  · ~  chore(telematics-ms): PROJ-2101 bump @acme/rabbit to 4.2.0                 4h|
|                                                                                                    |
|━━ REVIEW REQUESTED ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 2 |
|    #3241  ✓  ○    feat(admin-console): PROJ-1880 driving sessions tab                            3h|
|    #3233  ✗1 ○    fix(auth-service): PROJ-2004 guard null context closure                          6h|
|                   └ lint / eslint                                                                  |
|                                                                                                    |
|━━ INVOLVED ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 1 |
|    #3188  ✓  ✓    docs(monorepo): PROJ-1770 document rabbit retry semantics                       2d|
|                                                                                                    |
|━━ TEAMMATES ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ ⠹ |
|                   loading…                                                                         |
|  j/k move · enter open · r reload · q quit                            acme/monorepo          ⠹|
+----------------------------------------------------------------------------------------------------+
```

Counted: 19 body lines = 4 section headers + 3 blanks + 11 PR/detail lines +
1 loading line. **14 of 19 rows (74%) carry PR data.** (R6 satisfied.)

### 9.6 Monochrome rendering (R16 acceptance check)

The 120-column board with all color stripped, bold and faint only. Faint is shown
here as lowercase-looking dimness which ASCII cannot convey — the point is that
the *glyphs and positions* alone carry the state:

```
━━ MINE ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 5
  ╭╴#3248  ✗2 ○    feat(api-service): PROJ-2037 refuse order plan writes aft…  2h
  │                └ unit-tests / node-20, e2e / checkout-flows
  │ #3251  ◐  ○    feat(api-service): PROJ-2038 backfill order_cutoff_at col…  2h
▌ ╰╴#3254  ✓  ✓ !  feat(api-service): PROJ-2039 expose cutoff in admin API     1h
    #3199  ✓  ✗    fix(billing-service): PROJ-1994 stop double-counting idle v…  1d
    #3260  ·  · ~  chore(telematics-ms): PROJ-2101 bump @acme/rabbit to…  4h
```

Readable: `#3248` has 2 failing checks and needs review; `#3254` is selected,
green, approved, and conflicted; `#3260` has no CI and is a draft. Nothing was
lost. (R3, R16)

---

## 10. Open questions from research §6 — all resolved

| # | question | decision | one-line reasoning |
|---|---|---|---|
| 1 | selection background vs `reverse` | **Background fill (ANSI 8) + `▌` bar in column 0** | `reverse` would invert the status colors on exactly the failing rows that most need them; a background fill leaves status foregrounds intact, and the column-0 bar is a second, independent channel that survives any theme. |
| 2 | priority ordering vs section stability | *(pre-resolved by the brief)* | Order is the user's config; actionability is expressed through visual weight within sections, not by moving rows. |
| 3 | narrow-width drop order | **age first, then review glyph; never tree, number, CI, blocker, or title** | Age changes urgency but never the decision; review state is the one signal the user re-derives on opening the PR anyway, whereas CI and conflicts decide whether to open it at all. |
| 4 | failing-check continuation line | *(pre-resolved: keep)* — demoted to `muted` and indented to the title column | The red `✗2` already raises the alarm, so the gate names are detail, not alarm; muting them is what stops them competing with titles. |
| 5 | ANSI 8/7 vs 256 greys for muted | **`Faint(true)` on default foreground; no index 8, no index 7, no 256 grey** | Faint is the only dimmer that is relative to the theme's own foreground, so it is correct on light and dark by construction and degrades to plain legible text where SGR 2 is ignored. |
| 6 | does the board need a global header | **No — cut it entirely** | Its three payloads were the app name (self-evident), the repo (constant in a single-repo tool), and the spinner; the latter two move into the footer, buying back a row for free. |
| 7 | Nerd Font vs plain Unicode glyphs | **Plain Unicode BMP + ASCII; an `ascii` config mode as fallback** | Nerd Font PUA codepoints carry the documented #290 version-skew risk and inconsistent advance widths for no legibility gain at single-cell size. |
| 8 | four state dimensions sharing a row | **One 7-cell cluster: CI (2 cells), review (1), blocker (1), gutters (3); draft additionally mutes the title** | Each dimension gets its own never-moving sub-slot so nothing shifts; draft and conflict share a slot because they are rankable (conflict wins) and the pair rarely needs simultaneous display. |

---

## 11. Implementation checklist

Render-layer changes only; no data-layer work required.

- [ ] Replace the 5 text status columns (`ci`/`review`/`draft`/`merge`, 11+13+8+11
      cells) with the single 7-cell glyph cluster (§3).
- [ ] Add the 3-cell right-aligned age column from `UpdatedAt` (§1.4).
- [ ] Selection: `▌` in column 0 + full-terminal-width background fill on ANSI 8
      + bold title; delete `selStyle`'s `Foreground(15)` (§2).
- [ ] Section headers: uppercase name, full-width `━` rule, right-aligned count (§1.6).
- [ ] Continuation line: reindent to the title column, restyle from `error` to
      `muted`, keep the `│` spine inside stacks (§1.5).
- [ ] Replace every `Color("8")` foreground with `Faint(true)` (§5.1).
- [ ] Number column: `accent` (4), not `header` (6) (§5.2).
- [ ] Delete the global header; move repo + spinner into the footer; make the
      status message replace the footer's left half rather than add a line (§7).
- [ ] Introduce the seven-token palette as a single struct, referenced
      symbolically at all render sites (R14).
- [ ] Implement the four width tiers with the exact breakpoints in §6.2; clamp
      all padding at zero.
- [ ] Acceptance: render the board at 40, 44, 59, 60, 75, 76, 80, 120 columns and
      confirm columns 0–10 never move; render with `TERM` forced monochrome and
      confirm §9.6; render on a light theme and confirm the selected row and all
      `muted` text remain legible.
