# prs-mng — making sections read clearly without spending a line

Design study, written in response to:

> *"The sections are not structured clearly enough"* — with three ideas floated,
> all optional: a short horizontal line at a section boundary, gaps in the
> vertical rule where one section ends, or sticky headers. Plus: *"section names
> are cropped to 8 cells; when a section is at the top of the viewport, could
> its name expand over the list — use space that is otherwise blank?"*

Researched 2026-09-17 against the shipped code at `48b085d`, and against the
live board captured from `acme/monorepo` in real panes at 120×30, 120×14
and 80×20.

This document is **design only**. No Go was changed.

It sits under `uniform-rows.md`, which is the binding constraint, and reuses the
industry survey in `view-restructure.md` §4 and §6 — whose *conclusion* was
overturned, but whose evidence still holds.

---

## 1. Recommendation, in one paragraph

Three changes, all of which cost **zero lines and zero cells of title width**.
(1) The vertical rule **breaks** at a section boundary: the first row of a
section draws `╷` U+2577 at the rule cell instead of `│`, so the rule visibly
starts rather than continues. (2) The section name is rendered against the
**visible window**, not the whole board, so the top visible row *always* carries
its section's name — today it goes blank the moment the true first row scrolls
off, which is the actual defect and the one the user is feeling. (3) The
**footer's right-hand field carries the full, unclipped section name** of the
cursor's section, replacing the repo string there.

The 8-cell gutter, `sectionWidth`, the title width, and every responsive
breakpoint are **unchanged**. Nothing in `window()` changes. No constant in
`render.go` changes except the glyph chosen in `sectionGutter`.

---

## 2. What is actually wrong today, measured

I ran the real binary in real tmux panes rather than reasoning from the source.

At 120×30 the whole board (29 rows) fits, every section name is on screen, and
honestly it reads fine. The complaint only bites in a short pane. At **120×14**,
after 18 `j`, the board renders:

```
         │    #3228  ◐  ○ ! feat(api-service): PROJ-2036 return the source fleet spec status on order plan read ali 19h
         │    #3189  ✗3 ○   feat(api-service): PROJ-1879 mint the external order id on the unit                 car 22h
         │    #3185  ✗1 ○   refactor(api-service,schemas): PROJ-1675 put order submission state on the unit     car 22h
         │    #3237  ·  ○ ! feat(webapp): PROJ-1373 sticky row-level delete on the Order Plan units table       osc 22h
         │    #3186  ✓  ○ ! fix(api-service): PROJ-1951 free unit numbers when a plan is abandoned              car  1d
         │    #2565  ◐  ○   fix(webapp): PROJ-1094 respect explicit catalog sort over search relevance          osc  2d
         │    #2902  ◐  ○   [4/5] test(webapp-e2e): PROJ-1700 stub the ZPX7 code exchange in the shared upstr…  fra  1w
MY TEAM… │    #3211  ✗1 ✓   feat(api-service,schemas): PROJ-1880 build the ZPX7 order-submit payload            car 22h
```

**The top seven rows carry no section label at all.** Not a cropped one — none.
`NEEDS MY REVIEW`'s first row is above the window, and `body()` decides
`firstInSection` from `i == 0` over the *whole* board (`model.go:428` and
`:452`) while `window()` slices the rendered lines afterwards (`model.go:514`).
The label is computed before the thing that could have preserved it.

So the user's two complaints are really one defect and one cosmetic gap:

| complaint | severity | cause |
|---|---|---|
| "not structured clearly enough" | **real, and worst in short panes** | the name is absent, not merely cropped, for every row above a visible boundary |
| `NEEDS M…` cropping | mild | 8 cells; `NEEDS M…` is still recognisable, and the ambiguity is only against other names sharing 7 characters |

Fixing the first is free. That matters, because it means the expensive ideas do
not have to carry the weight.

---

## 3. The crux: does an expansion that depends on scroll position break the invariant?

The brief asks for this explicitly, and it deserves a direct answer rather than
a reflex. `view-restructure.md` §4.1 rejected sticky headers partly because
*"the rendered chrome becomes a function of the cursor position, so the line map
changes as you navigate"*, and `uniform-rows.md` §2 makes the same coupling the
root of the whole bug class.

**That objection does not apply to a label whose width varies within an existing
row, and the distinction is exact.** Here is the accounting.

`window()` does `lines[start : start+height]`. The three quantities that decide
whether one keypress scrolls one line are:

- `len(lines)` — the total line count of the board
- `rowStarts` — the row-index → line-index map
- `start` — computed from `rowStarts[cursorRow]`, `height` and `scrollOff`

A label that changes the **cell content** of `lines[start]`:

- does not change `len(lines)` — no line is added or removed
- does not change `rowStarts` — it is still the identity map, one row per line
- therefore does not change `start` by even one line, at any cursor position

Scroll-jump cost: **0 lines, at every cursor position, on every board shape.**
The invariant `uniform-rows.md` §2 defends is about *line count* being a
function of data or position. This is not that.

Contrast a sticky header, which is genuinely rejected: it occupies a line, so
`height` for the list drops by one, and worse, the line it occupies is not a
row — the exact thing §4 of `uniform-rows.md` measured at scroll deltas of
0, 1, **2**, **3**. A sticky header costs one permanent row out of 19 (5.3% of
the board) and reintroduces a non-row line. An expanding in-row label costs
neither.

So the user's instinct in point 2 is sound on the invariant, and the brief's
suspicion — that it *might* be safe precisely because it costs cells and not
lines — is correct. **It is safe.**

**But it fails for a different reason, and this is why I am not recommending
in-row expansion.** There is no blank space in the row to expand into. Working
outward from offset 0 (§3.1 of `DESIGN.md`):

| offsets | field | can the label take it? |
|---|---|---|
| 0–7 | section name | already the label |
| 8–9 | ` │` rule | no — it is the separator being reinforced |
| 10 | mark `▌` | no — selection |
| 11 | gutter | 1 cell, useless alone |
| 12–13 | tree | no — `DESIGN.md` §3.7: "never dropped, since losing them would silently change the meaning of adjacent rows" |
| 14–19 | number | no — the scanning key |
| 21–27 | status | no — the whole point of the board |
| 29+ | title | **the payload** |

Every cell is spoken for. "Space that is otherwise blank" is an illusion: the
right-hand whitespace on a short-title row is not structurally blank, it is a
*short title*, and the next row's title fills it. A label sized to that gap
would flicker as you scroll past rows with different title lengths.

I built the two in-row variants and looked at them in a real terminal:

```
=== A: name overflows past the rule, columns shift right ===
NEEDS MY REVIEW  #3129  ✓  ○   refactor(frontend): share makeAutoObservable…  ali 18h
         │    #3239  ◐  ○   feat(webapp): PROJ-2038 show an "On hold" statu…  ali 18h
```

Variant A moves the number, status and title columns right on one row. That
breaks `DESIGN.md` §3.8's stability guarantee — *"columns 0–20 never move at
all"* — and it breaks it **as a function of scroll position**, which is the one
version of column instability that is worse than the static kind. Rejected.

```
=== E: name drawn over the title's leading cells on the top row only ===
NEEDS M… ┃ ▌  #3129  ✓  ○   NEEDS MY REVIEW ·· share makeAutoObservableAnd…  ali 18h
```

Variant E keeps every column fixed and is invariant-safe. It is rejected on
value, not on mechanism: it destroys the title of a row the user is actively
scanning, in order to spell out a name the gutter already abbreviates to
`NEEDS M…`. `scrollOff` is 2, so the top visible row is never the cursor row in
steady state — it is a row you are reading, not one you are leaving. Trading a
PR title for seven more characters of a label you have already recognised is a
bad trade at 120 cols and a worse one at 80.

**Conclusion on point 2:** the expansion is safe on the invariant and the user's
reasoning about it is right. It fails on the simpler ground that the row has no
spare cells. The full name is still worth having, so §4.3 puts it in the one
region that genuinely has slack — the footer, which is already chrome and
already has a right-hand field.

---

## 4. The recommended layout

### 4.1 The boundary indicator — the rule breaks

The first row of each section draws **`╷` U+2577 BOX DRAWINGS LIGHT DOWN** at
offset 9, where continuation rows draw `│` U+2502. The rule appears to *start*
at each section, leaving a one-cell gap above it.

This is the user's second floated idea ("gaps in the vertical rule") and the
first ("a short horizontal line") merged into the cheapest possible form — a
single glyph substitution in `sectionGutter`, at an offset that is already
drawn.

Rendered in a real terminal:

```
MINE     ╷ row
         │ row
         │ row
NEEDS M… ╷ row
         │ row
```

The break is visible even when the name is fully clipped, which is the property
that matters: `NEEDS M…` fills all 8 cells and cannot itself signal "new
section" by shape.

I tested a short horizontal tick inside the name field (`MINE ───╮`) and
rejected it: it works for `MINE` and disappears entirely for `NEEDS M…`, which
already occupies all 8 cells. A boundary marker that vanishes on exactly the
names that need it most is worse than none. I also tested a pure gap — blanking
the rule cell with a space — which is too weak to read as intentional next to a
column of `│`.

`╷` stays in `muted`, like the rest of the rule.

**Cost: zero.** One glyph swapped for another at the same offset.

### 4.2 The name is computed against the window, not the board

This is the substantive fix, and it is the one that answers "not structured
clearly enough".

Today `firstInSection` is `i == 0` over the section's rows. Change it to: a row
carries its section's name if it is the section's first row **or** it is the
first row of the section that is visible in the current window. In practice
that means the top visible row always carries a name, and everything below it
behaves exactly as today.

Applied to the 120×14 capture from §2, the same board becomes:

```
NEEDS M… │    #3228  ◐  ○ ! feat(api-service): PROJ-2036 return the source fleet spec status on order plan read ali 19h
         │    #3189  ✗3 ○   feat(api-service): PROJ-1879 mint the external order id on the unit                 car 22h
         │    #3185  ✗1 ○   refactor(api-service,schemas): PROJ-1675 put order submission state on the unit     car 22h
         │    #3237  ·  ○ ! feat(webapp): PROJ-1373 sticky row-level delete on the Order Plan units table       osc 22h
         │    #3186  ✓  ○ ! fix(api-service): PROJ-1951 free unit numbers when a plan is abandoned              car  1d
         │    #2565  ◐  ○   fix(webapp): PROJ-1094 respect explicit catalog sort over search relevance          osc  2d
         │    #2902  ◐  ○   [4/5] test(webapp-e2e): PROJ-1700 stub the ZPX7 code exchange in the shared upstr…  fra  1w
MY TEAM… ╷    #3211  ✗1 ✓   feat(api-service,schemas): PROJ-1880 build the ZPX7 order-submit payload            car 22h
```

Note the top row draws `│`, **not** `╷`. The distinction carries real
information: `╷` means "a section starts here", `│` with a name means "you are
inside this section and its start is above the window". Those are different
facts and the board should not conflate them.

This is a sticky header's benefit without a sticky header's line. It is also
exactly the coupling §3 analysed: the top row's *content* depends on `start`.
Per §3 that costs zero lines, and unlike variants A and E it costs zero cells
too, because it writes into gutter cells that are currently blank.

**Cost: zero.** It fills 8 cells that are blank today.

**Note for the implementer, and this is the one real structural consequence:**
`body()` currently renders before `window()` slices, so `firstInSection` cannot
know the window. Either `window()` must return `start` so the top row can be
re-rendered, or the label decision must move after the slice. This is the only
change in the whole document that touches control flow rather than a string.
See §8.

### 4.3 The full name goes in the footer — **OVERRULED BY THE USER, see §11**

> **Status: rejected.** The user's words: *"I agree the repo is less important
> than the section but I don't like the location, bottom, grey, corner… it
> doesn't stand out. I think sticky at the top is better and leave the repo name
> as is for now."* The repo keeps the footer slot. §11 records the revision and
> what replaces this. The analysis below is kept because its *measurements* are
> still valid and §11 reuses them — but its recommendation is dead.

The footer is already one `muted` line, always present, with a right-hand field
carrying repo + spinner (`model.go:578`). The cursor's section name takes that
field, **unclipped**, and the repo yields it.

```
  j/k move · l/h section · enter open · c checks · / filter · ? help · q quit                            NEEDS MY REVIEW
```

The repo yields because it is constant for the whole session — it is in the
window title, in the config, and the user chose it — while the section name
varies as you move and is the thing being asked for. The spinner keeps its cell.

Measured fit, with the existing keybinding string (77 cells):

| width | free cells with `NEEDS MY REVIEW` | result |
|---|---|---|
| 120 | 26 | fits |
| 100 | 6 | fits |
| 86 | −8 | keys clip, name survives |
| 80 | −14 | keys clip, name survives |

Below ~100 columns the footer is *already* over-full today (the repo string
alone leaves gap −11 at 86 and −17 at 80), and `footer()` already handles it by
clipping the left field. So this does not introduce a new degradation, it
reuses one that exists. The name is shorter than the repo string in four of the
five configured rules, so at most widths this footer is *less* crowded than
today's.

This is `view-restructure.md` §4.2, which that study called *"the good version
of option 1 [sticky headers] … cheap, safe, and worth doing independently of
everything else in this document"* — and then never got implemented. It is
still right, and it is now the thing that answers the user's point 2 without
touching a row.

**Cost: the repo string's position in the footer.** Zero lines, zero title
cells.

---

## 5. Mockups, real board content

Content is the live `acme/monorepo` board captured from the running binary,
so these are comparable with `uniform-rows.md` §4 and `DESIGN.md` §3.5.

### 5.1 120 columns, FULL tier, board at the top

```
MINE     ╷▌ ╭╴#3103  ✓  ✓   fix(api-service,schemas): pair interior colours with seat types                         now
         │  ╰╴#3109  ✓  ✓   fix(api-service): forbid only the move that re-opens a cycle                            22h
         │    #3248  ✓  ○ ~ fix(webapp): reach the dependency popup on a Default promotion                           1h
         │    #3031  ✓  ○ ! chore(admin-console): enforce no-floating-promises                                       2w
         │    #2909  ✗2 ○ ! feat(webapp,admin-console): federation spike — DO NOT MERGE                              4w
NEEDS M… ╷    #3129  ✓  ○   refactor(frontend): share makeObservableAndExpose, drop web-client tracking sh…     ali 18h
         │    #3239  ◐  ○   feat(webapp): PROJ-2038 show an "On hold" status chip when the fleet spec is not …  ali 18h
         │    #3234  ◐  ○   feat(api-service): PROJ-2037 refuse order plan writes while the fleet spec is not … ali 18h
         │    #3228  ◐  ○ ! feat(api-service): PROJ-2036 return the source fleet spec status on order plan read ali 18h
         │    #3189  ✗3 ○   feat(api-service): PROJ-1879 mint the external order id on the unit                 car 22h
         │    #3185  ✗1 ○   refactor(api-service,schemas): PROJ-1675 put order submission state on the unit     car 22h
         │    #3237  ·  ○ ! feat(webapp): PROJ-1373 sticky row-level delete on the Order Plan units table       osc 22h
         │    #3186  ✓  ○ ! fix(api-service): PROJ-1951 free unit numbers when a plan is abandoned              car  1d
         │    #2565  ◐  ○   fix(webapp): PROJ-1094 respect explicit catalog sort over search relevance          osc  2d
         │    #2902  ◐  ○   [4/5] test(webapp-e2e): PROJ-1700 stub the ZPX7 code exchange in the shared upstr…  fra  1w
MY TEAM… ╷    #3211  ✗1 ✓   feat(api-service,schemas): PROJ-1880 build the ZPX7 order-submit payload            car 22h
         │    #3134  ✓  ✓   feat(webapp): PROJ-1418 enable the Trade-ups entry point behind a flag              jud  1d
INVOLVED ╷    #2741  ✗2 ○   feat(api-service): PROJ-1905 audit a fleet spec's as-created option states          gra  1d
         │    #345   ·  ✓ ! Change docker file location                                                         nad  2d
         │    #2994  ◐  ✓ ! feat(shared-common): PROJ-1678 extract the shared pricing math into libs/c…         osc  1w
         │    #2906  ✗2 ○   [5/5] refactor(webapp-e2e): PROJ-1700 rewrite the upstream stub in TypeScript, wi…  fra  1w
ALL OPEN ╷    #3246  ✗1 ✗   feat(context-map): add context tree pagination                                      eri now
         │    #3240  ◐  ✓   Fix location widget                                                                 vic  1h
         │    #3227  ◐  ○   fix(accelerate): land ABC vehicle deep-link on the correct tab                      rup  1h
         │    #3229  ✗6 ○   chore: bump the npm-production group across 1 directory with 15 updates             dep  3h
         │    #3249  ✗2 ○   ci: bump the github-actions group with 2 updates                                    dep  3h
         │    #3233  ✓  ○   chore(admin-console): remove DEPRECATED propTypes (React 19 readiness, 1/2)         ali 12h
         │    #3247  ◐  ○   feat(admin-console): PROJ-1600 PROJ-1858 design-system vehicle form, with the VIN … rup 16h
         │    #3219  ✗1 ✓   feat(webapp,api-service): PROJ-1872 order plan activity view with the created eve…  hei 17h
  j/k move · l/h section · enter open · c checks · / filter · ? help · q quit                                       MINE
```

Every title, offset and glyph above is the real render; only the `╷` at five
boundaries and the footer's right field differ from what ships today.

### 5.2 120 columns, scrolled — the case that is broken today

Cursor on `#345`, `NEEDS MY REVIEW` and `MY TEAM'S` both started above the
window. Compare against the §2 capture, where the top seven rows were unlabelled.

```
NEEDS M… │    #3228  ◐  ○ ! feat(api-service): PROJ-2036 return the source fleet spec status on order plan read ali 19h
         │    #3189  ✗3 ○   feat(api-service): PROJ-1879 mint the external order id on the unit                 car 22h
         │    #3185  ✗1 ○   refactor(api-service,schemas): PROJ-1675 put order submission state on the unit     car 22h
         │    #3237  ·  ○ ! feat(webapp): PROJ-1373 sticky row-level delete on the Order Plan units table       osc 22h
         │    #3186  ✓  ○ ! fix(api-service): PROJ-1951 free unit numbers when a plan is abandoned              car  1d
         │    #2565  ◐  ○   fix(webapp): PROJ-1094 respect explicit catalog sort over search relevance          osc  2d
         │    #2902  ◐  ○   [4/5] test(webapp-e2e): PROJ-1700 stub the ZPX7 code exchange in the shared upstr…  fra  1w
MY TEAM… ╷    #3211  ✗1 ✓   feat(api-service,schemas): PROJ-1880 build the ZPX7 order-submit payload            car 22h
         │    #3134  ✓  ✓   feat(webapp): PROJ-1418 enable the Trade-ups entry point behind a flag              jud  1d
INVOLVED ╷    #2741  ✗2 ○   feat(api-service): PROJ-1905 audit a fleet spec's as-created option states          gra  1d
         │▌   #345   ·  ✓ ! Change docker file location                                                         nad  2d
         │    #2994  ◐  ✓ ! feat(shared-common): PROJ-1678 extract the shared pricing math into libs/c…         osc  1w
         │    #2906  ✗2 ○   [5/5] refactor(webapp-e2e): PROJ-1700 rewrite the upstream stub in TypeScript, wi…  fra  1w
  j/k move · l/h section · enter open · c checks · / filter · ? help · q quit                                   INVOLVED
```

Top row: `NEEDS M…` with `│`, not `╷` — inside the section, start is above.
Footer: `INVOLVED`, the cursor's section, unclipped.

### 5.3 80 columns, MID tier

Author and age are already dropped at this tier; title width is `w − 29 = 51`.
Nothing in this design changes either number.

```
MINE     ╷▌ ╭╴#3103  ✓  ✓   fix(api-service,schemas): pair interior colours wi…
         │  ╰╴#3109  ✓  ✓   fix(api-service): forbid only the move that re-ope…
         │    #3248  ✓  ○ ~ fix(webapp): reach the dependency popup on a Def  …
         │    #3031  ✓  ○ ! chore(admin-console): enforce no-floating-promises.
         │    #2909  ✗2 ○ ! feat(webapp,admin-console): federation spike —    …
NEEDS M… ╷    #3129  ✓  ○   refactor(frontend): share makeAutoObservableAndExp…
         │    #3239  ◐  ○   feat(webapp): PROJ-2038 show an "On hold" status  …
         │    #3234  ◐  ○   feat(api-service): PROJ-2037 refuse order plan wri…
         │    #3228  ◐  ○ ! feat(api-service): PROJ-2036 return the source fle…
         │    #3189  ✗3 ○   feat(api-service): PROJ-1879 mint the external ord…
         │    #3185  ✗1 ○   refactor(api-service,schemas): PROJ-1675 put order…
         │    #3237  ·  ○ ! feat(webapp): PROJ-1373 sticky row-level delete o …
         │    #3186  ✓  ○ ! fix(api-service): PROJ-1951 free unit numbers when…
         │    #2565  ◐  ○   fix(webapp): PROJ-1094 respect explicit catalog so…
         │    #2902  ◐  ○   [4/5] test(webapp-e2e): PROJ-1700 stub the ZPX7 c …
MY TEAM… ╷    #3211  ✗1 ✓   feat(api-service,schemas): PROJ-1880 build the XLR…
         │    #3134  ✓  ✓   feat(webapp): PROJ-1418 enable the Trade-ups entr …
INVOLVED ╷    #2741  ✗2 ○   feat(api-service): PROJ-1905 audit a fleet spec's …
         │    #345   ·  ✓ ! Change docker file location
  j/k move · l/h section · enter open · c checks · / filter · ? help · q quit
```

At 80 the footer's left field already consumes 77 of 80 cells, so the right
field is dropped by the existing clip path — same as today, where the repo is
also dropped. The full name is unavailable at this width. That is a genuine
limitation of §4.3 and I have not hidden it; §7 records it.

---

## 6. Behaviour in the cases the brief asks about

### 6.1 A section taller than the viewport, no boundary on screen

This is the §5.2 situation taken to its limit: `NEEDS MY REVIEW` has 10 rows,
and in a 9-row pane none of its boundaries are visible.

Every row shows `│`. The top row shows `NEEDS M…`; every other row shows blank.
The footer shows `NEEDS MY REVIEW` in full. So the answer to "where am I" is
available in two independent places, and the answer to "is there a boundary
near me" is correctly *no* — no `╷` is drawn, because none exists in view.

That is the behaviour I want: the absence of a break is information, and the
design does not manufacture one. A sticky header would have to render something
header-shaped here, which is why sticky headers have to answer "does the sticky
row mean a boundary or not" and this design does not.

### 6.2 Scrolling one line across a boundary

`j` moves `start` by one. The row leaving the top was the top row; the row
arriving becomes the top row and acquires its section's name. The row below —
formerly the top, formerly labelled — loses its label, unless it is a genuine
section first row, in which case it keeps it and keeps `╷`.

So at most two rows change their gutter content per keypress, and zero rows
change position relative to each other. `len(lines)` is constant, `rowStarts` is
constant, `start` moves by one. One keypress, one line. Unconditional.

### 6.3 Section notes (loading, empty, failed)

`renderSectionNote` (`render.go`, bottom) draws these with the same gutter and
`first = true`. They get `╷` like any section first row. No other change. The
placeholder-block blanks emitted by `body()` for a pending section render as
empty lines with no rule, which is already true today and this design does not
alter it.

### 6.4 Filtering

`sections()` drops a section whose rows all fail the query, so a filtered board
has fewer sections and every visible one still has a genuine first row.
Window-relative labelling composes with this without a special case: it operates
on whatever `body()` produced. The footer name follows the cursor, which
`clampCursor` keeps in range.

---

## 7. What this costs

- **The repo string loses its footer slot.** It is still in the config, the
  window title, and `?`. This is the only thing given up, and it is given up in
  exchange for the thing the user asked for.
- **Below ~86 columns the full name is not shown**, because the footer's left
  field already fills the line. At those widths the user has exactly today's
  information plus the rule break plus the window-relative name — still a net
  gain, just not the full-name part.
- **A row renders differently depending on whether it is the top visible row.**
  This is real and I want it stated plainly rather than buried: the same PR
  shows `NEEDS M…` at one scroll position and blank at another. Per §3 it costs
  no line and no column position, but it does mean a row is not a pure function
  of its own data. I judge this acceptable because the varying cells are
  currently *blank* — nothing is covered, revealed or moved — and because the
  variation is what carries the information. It is worth a regression test.
- **One glyph more to learn** (`╷` vs `│`). Small; it reads as "the rule starts
  here" without being told.

What is **not** spent: no line, no title cell, no breakpoint, no change to
`sectionWidth`, no change to `window()`, no change to `rowStarts`.

---

## 8. Constants and code that change

| thing | today | after |
|---|---|---|
| `sectionWidth` (`render.go`) | 8 | **8, unchanged** |
| rule width (offsets 8–9) | 2 (` │`) | **2, unchanged** — glyph at 9 varies |
| `fixedFull` / `fixedMid` / `fixedNarrow` | `23/19/15 + sectionWidth + 2` | **unchanged** |
| `minWidth` / `narrowUntil` / `midUntil` / `fullFrom` | 50 / 58 / 70 / 86 | **unchanged** |
| `titleWidth()` | `w − fixed` | **unchanged** |
| `scrollOff` | 2 | **unchanged** |
| `window()` | clamp over lines | **unchanged in behaviour**; may need to expose `start` (§4.2) |
| `sectionGutter()` | name or blank | takes the boundary flag, emits `╷` or `│` |
| `renderRow()` signature | `..., section string, firstInSection bool` | needs a third state, not a bool: `first` / `continuing-with-name` / `continuing` |
| `footer()` | right field = repo + spinner | right field = section name + spinner |

**No responsive breakpoint moves.** This is worth stating explicitly given
`uniform-rows.md` §5 had to move every breakpoint by 10 for the gutter — nothing
here widens anything.

The one non-trivial piece is §4.2's ordering problem: the label depends on
`start`, and `start` is computed from lines that already contain labels. The
cheapest shape is probably for `window()` to return `(slice, start)` and for the
top line to be re-rendered — the row data is still addressable via `rowStarts`.
An alternative is a two-pass `body()`. I did not implement either, so I do not
know which is less invasive in practice; see §10.

---

## 9. Options rejected, and why

**Sticky header pinned at the top.** The brief's third floated idea. Costs one
permanent line out of 19 (5.3% of the board), and that line is not a row, which
is the precise thing `uniform-rows.md` §4 measured at scroll deltas of 0, 1, 2
and 3. §4.2 of this document delivers the same benefit — "which section am I
in" when the boundary has scrolled away — for zero lines. Rejected on the
constraint, and made redundant by the recommendation.

**A short horizontal line at the boundary, as a line of its own.** Same
objection, same cost, worse value: it marks the boundary but does not name the
section.

**A short horizontal tick inside the name field** (`MINE ───╮`). Invariant-safe
and costs nothing, but it needs spare cells inside the 8, and `NEEDS M…` /
`ALL OPEN` / `INVOLVED` have none. A marker that vanishes on 3 of 5 configured
rules — and specifically on the long names most in need of disambiguation — is
worse than a marker that always appears. Rejected on coverage.

**Blanking the rule cell at the boundary** (a pure gap, no glyph). The literal
reading of "gaps in the vertical rule". Free and correct, but next to a column
of `│` a single missing glyph reads as a rendering artifact rather than a
deliberate mark. `╷` says the same thing and looks intentional. Rejected on
legibility, and it is the closest runner-up.

**In-row name expansion, columns shifting right (variant A, §3).** Breaks
`DESIGN.md` §3.8's "columns 0–20 never move" guarantee, as a function of scroll
position. Rejected.

**In-row name expansion over the title (variant E, §3).** Invariant-safe and
column-safe. Rejected because the cells it takes are a PR title the user is
scanning, bought with a name already 90% legible as `NEEDS M…`.

**Widening `sectionWidth` to 15** to fit `NEEDS MY REVIEW` uncropped. Costs 7
cells of title on every row at every width, and moves every breakpoint up by 7 —
`uniform-rows.md` §5 already spent 10 cells on this gutter and called it out as
a real cost. Paying 7 more so the longest of five names stops being cropped is
not proportionate.

**Per-section counts back in the gutter.** Out of scope, and there is nowhere to
put them without the above. Still one `l`/`h` away, as `uniform-rows.md` §6
notes.

**Colour-coding the rule per section.** Considered and dropped without much
work: `DESIGN.md` §3.3 spends seven tokens carefully and reserves loud colour
for failing states. Giving the rule a hue would be a fourth colour channel
competing with the three that mean "something is wrong".

---

## 10. What I could not verify

- **Whether the `╷` break actually reads as a boundary in daily use.** I
  rendered it in a real terminal and it reads correctly to me at both widths,
  but that is one person looking at a static frame, not the user scanning.
  It is a one-glyph change, so it is cheap to try and cheap to revert.
- **Whether losing the repo from the footer is felt.** I argued it is constant
  and therefore low-value, but I do not know if the user reads it — for instance
  when running two boards side by side against different repos, which would make
  it load-bearing. **This is the single decision I would most like the user to
  confirm before it is built.**
- **Which implementation shape §4.2 should take.** I did not write the code, so
  "`window()` returns `start`" vs "two-pass `body()`" is a guess about
  invasiveness, not a measurement.
- **`╷` U+2577 glyph coverage.** It rendered correctly in this terminal and font.
  It is a less common box-drawing codepoint than `│`, and I did not test other
  fonts. If it is missing anywhere, `├` U+251C is the fallback — it reads
  slightly worse (it implies a branch rather than a start) but is far more widely
  covered. Worth one check on whatever font the user actually runs.
- **Behaviour at very short panes (height < 6).** `window()` centres instead of
  clamping when `2*scrollOff+1 > height`. I reasoned this is unaffected — the
  label logic reads `start` whatever produced it — but I did not exercise it.
- **Board shapes other than this repo's.** All measurements are from
  `acme/monorepo` with five configured rules whose uppercased names are 4,
  15, 9, 8 and 8 cells. A config with several rules sharing a 7-character prefix
  would make the 8-cell crop genuinely ambiguous, and then the footer's full
  name stops being a nicety and becomes necessary — which strengthens §4.3 at
  exactly the widths where §7 says it is unavailable. I did not test that config.

---

## 11. Revision: the footer is overruled, and a sticky line is worth its row

The user accepted §4.1 (the `╷` break) and §4.2 (the window-relative name), and
rejected §4.3:

> *"I agree the repo is less important than the section but I don't like the
> location, bottom, grey, corner… it doesn't stand out. I think sticky at the
> top is better and leave the repo name as is for now."*

That objection is **about prominence**, which is not what §4.3 was answering.
§4.3 solved *availability* — the full name existed nowhere on the board — and it
solved it in the cheapest slot rather than the most visible one. Cheapest was
the wrong objective. Muted text in the bottom-right corner is where this board
puts the things you are allowed to ignore (`DESIGN.md` §3.3: "Silent (`muted`)
is everything else"), so the footer actively signalled *"this is not important"*
about the one field the user wanted to notice. The user is right and §4.3 was
wrong on its own design language's terms.

**Recommendation: (a), a pinned header line, carrying the section name and a
per-section position count.** Not (b).

### 11.1 Why (b) is not merely weak — it is a no-op

(b) was "render the top visible row's gutter name bright/bold instead of muted".
**The gutter name is already bright and bold.** `render.go:304` wraps
`sectionGutter` in `headerStyle`, which is `Foreground("6").Bold(true)` — cyan,
bold. I confirmed it against the running binary rather than the source; the
captured cells for `MINE` on the live board begin `ESC[1m ESC[36m`.

So there is no muted-to-bright change available: the section name is *already*
the most emphasised text on a board whose entire palette is built to be quiet
(`DESIGN.md` §3.3 — "A board where everything is fine has almost no colour in
it"). (b) as stated changes nothing.

The only way to make (b) real is to escalate *beyond* `headerStyle` — inverse
video or a background fill on those 8 cells. I rendered that:

```
NEEDS M… │    #3228  ◐  ○ ! feat(api-service): PROJ-2036 return the source flee…
         │    #3189  ✗3 ○   feat(api-service): PROJ-1879 mint the external orde…
MY TEAM… ╷    #3211  ✗1 ✓   feat(api-service,schemas): PROJ-1880 build the XLR…
```

Two problems, and they are why I am not offering it as a third option. First,
`selBg` is already a background fill and it means *selection*; a second filled
region 10 cells to the left of the selection bar competes with the one piece of
state the user changes on every keypress. Second — and this is decisive — it is
still 8 cells reading `NEEDS M…`. The user's complaint was that a small grey
thing in a corner does not stand out. A small **bright** thing in a corner is
better, but it is still small, still cropped, and still cannot carry a count.
Making the least-prominent answer slightly louder does not reach "prominent".

**So: (b) does not answer the complaint.** Saying so plainly, as asked.

### 11.2 Re-deriving the sticky line's cost, without reusing §9

§9 rejected sticky headers on two grounds. One has dissolved and one has to be
re-measured.

*"§4.2 makes it redundant"* — this was always the weaker half, and it was
premised on §4.3 carrying the full name. With the footer gone, §4.2 gives a
cropped `NEEDS M…` in a gutter cell and nothing else. The full name now exists
nowhere. That ground is gone.

*"It costs one permanent line out of 19"* — still true, and now the whole
question. Measured concretely on the real board rather than as a percentage:

The live `acme/monorepo` board is **29 rows** across five sections. So:

| pane height | list lines | rows lost to sticky | what actually happens |
|---|---|---|---|
| 30 | 29 | 29 → 28 | **the board stops fitting.** `#3219` falls off the bottom and the list begins scrolling |
| 24 | 23 | 23 → 22 | already scrolling; one fewer row of context |
| 20 | 19 | 19 → 18 | already scrolling |
| 14 | 13 | 13 → 12 | already scrolling, 7.7% of the board |
| 40+ | 39+ | 39 → 38 | board fits either way, no observable cost |

I verified the h=30 threshold by resizing the live pane: at 30 the last row is
`#3219` and the board is complete; at 29 it is `#3247` and `#3219` is gone. That
is the one sharp, real cost and it is worth naming precisely rather than
averaging it away — **there exists exactly one pane height (30, for this board
today) where a sticky line converts "I can see everything" into "I have to
scroll".** It also moves with the board: one merged PR and the threshold is 29.

Against that, in **every** pane where the board already scrolls — which is every
height below 30, including the 14 and 20 cases where the user's complaint is
sharpest — the row is not converting a complete view into an incomplete one. It
is trading the 13th row of an already-truncated list for a permanent answer to
"where am I". At h=14 that is 12 rows instead of 13.

`uniform-rows.md` §4 bought 10 rows (16 → 26) by removing gate lines and header
bands. Spending **one** of those ten on the single piece of chrome the user has
explicitly asked for, and which is the only one that was ever load-bearing for
orientation, is proportionate. The restructure was not an argument that chrome
is forbidden; it was an argument that chrome **inside the scrolling list** is
forbidden, because it makes row height position-dependent. A line above the
list is outside `window()` entirely.

### 11.3 The invariant is untouched, and this is mechanically different from the old rejection

This matters, because `view-restructure.md` §4.1 rejected sticky headers partly
on the invariant and §9 of this document repeated it. That was imprecise, and I
should correct it.

A sticky line above the list is **fixed chrome**, exactly like the footer and the
filter prompt. `View()` already models this: `chrome := 1`, `chrome = 2` while
filtering, and `avail := m.height - chrome`. A sticky header is `chrome = 2`
(or 3 while filtering) and nothing else changes.

Concretely, inside `window()`:

- `len(lines)` — unchanged, still one line per row
- `rowStarts` — unchanged, still the identity map
- `height` — smaller by one, as it already is while filtering
- lines scrolled per keypress — **still exactly one**

The thing `uniform-rows.md` §4 measured at deltas of 0/1/**2**/**3** was a
header line *interleaved between rows inside the scrolled region*, which made
the row→line map non-identity and made `start` jump. A line that never enters
`lines` cannot do that. The filter prompt has been doing this since day one and
has never contributed a scroll delta.

So the honest statement is: **a sticky line costs a row, and costs nothing else.**
§9's invariant-flavoured objection to it was wrong, and I am correcting it here
rather than leaving it to be cited later.

### 11.4 What the sticky line carries

Binding it to the **top visible row's** section, not the cursor's.

The cursor binding is the tempting one — it changes rarely, only on a deliberate
boundary crossing. It is also the one that lies: with the cursor in `MY TEAM'S`
and the top five visible rows still in `NEEDS MY REVIEW`, a cursor-bound header
names a section that is not the section of the rows underneath it. That is
precisely `view-restructure.md` §4.1's *"the line under it shifts meaning"*, and
it is a real defect rather than a theoretical one. The top-row binding is never
wrong about the row immediately beneath it, and it is the same binding §4.2
already uses, so the two agree by construction.

It carries the name in full, plus the position of the visible window within the
section:

```
NEEDS MY REVIEW  ·  6 of 10
         │    #3228  ◐  ○ ! feat(api-service): PROJ-2036 return the source fleet spec status on order plan read ali 19h
         │    #3189  ✗3 ○   feat(api-service): PROJ-1879 mint the external order id on the unit                 car 22h
         │    #3185  ✗1 ○   refactor(api-service,schemas): PROJ-1675 put order submission state on the unit     car 22h
         │▌   #3237  ·  ○ ! feat(webapp): PROJ-1373 sticky row-level delete on the Order Plan units table       osc 22h
         │    #3186  ✓  ○ ! fix(api-service): PROJ-1951 free unit numbers when a plan is abandoned              car  1d
         │    #2565  ◐  ○   fix(webapp): PROJ-1094 respect explicit catalog sort over search relevance          osc  2d
MY TEAM… ╷    #3211  ✗1 ✓   feat(api-service,schemas): PROJ-1880 build the ZPX7 order-submit payload            car 22h
  j/k move · l/h section · enter open · c checks · / filter · ? help · q quit                            acme/monorepo
```

**The count is what makes the row pay for itself, and it is the reason I moved
from "sticky is affordable" to "sticky is right".** `uniform-rows.md` §5 listed
the per-section count as a real loss of the restructure and §6 listed "whether
losing the per-section count is felt" as unverified. A sticky line is the only
place it can go — it does not fit in 8 gutter cells, and §9 correctly rejected
widening the gutter to make room. So this line is not merely a louder copy of
what the gutter says; it answers a question nothing on the board currently
answers: *how much of this section am I not looking at.*

Name in `header` (cyan bold, the existing token, unchanged). `·` and the count
in `muted`, matching the footer's separator idiom. No `━` band and no full-width
rule: at 120 cols a full-width line is a lot of ink for one label, and the name
plus count already reads as a header because it is the only line outside the
list.

Repo stays in the footer, untouched.

### 11.5 80 columns

```
NEEDS MY REVIEW  ·  6 of 10
         │    #3186  ✓  ○ ! fix(api-service): PROJ-1951 free unit numbers when…
         │    #2565  ◐  ○   fix(webapp): PROJ-1094 respect explicit catalog so…
         │    #2902  ◐  ○   [4/5] test(webapp-e2e): PROJ-1700 stub the ZPX7 c …
MY TEAM… ╷    #3211  ✗1 ✓   feat(api-service,schemas): PROJ-1880 build the XLR…
         │    #3134  ✓  ✓   feat(webapp): PROJ-1418 enable the Trade-ups entr …
INVOLVED ╷    #2741  ✗2 ○   feat(api-service): PROJ-1905 audit a fleet spec's …
  j/k move · l/h section · enter open · c checks · / filter · ? help · q quit
```

The sticky line is the one element here that works *better* at 80 than the
footer did: §7 had to admit the footer's full name was unavailable below ~86
columns because the keybindings already filled the line. The sticky line has a
line to itself, so the full name survives to `minWidth`. The longest configured
name is 15 cells plus ` · 6 of 10`, about 26 — comfortable at 50.

### 11.6 Does §4.2 still earn its place?

Asked directly, and it is the closest call in this revision. **Yes, but its
justification changes and it is now the weakest of the three.**

Before: §4.2 was *the* fix — the top rows were unlabelled and it labelled them.
Now the sticky line names the top row's section in full, one line above, so the
gutter's `NEEDS M…` is strictly redundant *as information*.

It still earns its place on two grounds, both weaker than the original:

1. **The gutter column stays uniformly populated.** Without it there is a
   stretch of bare `│` under a header that names a section, and the eye has to
   bind a line *outside* the list to a column *inside* it. I rendered both and
   the version with the gutter name reads better — the header and the gutter
   reinforce each other rather than the header floating free.
2. **It is free.** Zero lines, zero cells, and the cells it writes into are
   blank today.

If the implementer finds §4.2's ordering problem (§8 — `window()` must expose
`start`) expensive enough to matter, **it is now the one piece that can be
dropped** without losing information, since the sticky line needs the same
`start` and would already be paying that cost. I would build it, but I would not
argue hard for it.

The `╷` / `│` distinction from §4.1 and §4.2 **is unaffected and still carries
its meaning**: `╷` means a section starts on this row, `│` means continuation.
That is about boundaries within the visible list, which the sticky line says
nothing about. Both remain worth having.

### 11.7 The honest case against (a)

- **It costs a row, permanently, and at exactly one pane height (30, today) it
  converts a board that fits into a board that scrolls.** That is the real
  price and it is not recoverable. Every other height it is one row of context.
- **It reverses a decision made five commits ago** with measurements behind it.
  The reversal is defensible because the measurement was about chrome *inside*
  the scrolled region and this is outside it (§11.3) — but if I am wrong about
  that distinction being clean, this is where it breaks, and it breaks on the
  exact bug class the restructure existed to kill.
- **The header's content changes as you scroll**, which is the property
  `view-restructure.md` §4.1 disliked. I have bound it so it can never
  contradict the row beneath it (§11.4), which I believe defuses the objection,
  but "the top line's text changes while you scroll" remains true and some
  people find it restless. This is the single thing most worth looking at in a
  real session before keeping it.
- **I could not find a TUI in the earlier survey that does this** — `view-restructure.md`
  §4.1 says so explicitly. That is a weak negative (the survey did not claim to be
  exhaustive) but it is not nothing; we would be doing something the ecosystem
  does not.
- **The count needs a definition decision.** `6 of 10` as drawn means "the top
  visible row is the 6th of this section's 10 rows". "Rows visible of total"
  and "cursor position within section" are both defensible and read the same at
  a glance. See §12.

### 11.8 Do I still think the footer was right?

**No.** Asked plainly, so answered plainly: the user changed my mind, and not
just on taste.

§4.3 optimised for cost when the binding constraint was attention. It also
contained a cost I under-weighted in §7 — below ~86 columns the full name simply
was not shown, which means the answer to "what is this section" evaporated at
precisely the narrow widths where the 8-cell crop is most ambiguous. I flagged
that in §10 and then recommended it anyway. The sticky line does not have that
failure mode (§11.5).

The one thing §4.3 had that (a) does not is that it cost nothing. That is a real
advantage and it is why I proposed it. It is not worth a field the user will not
look at.

---

## 12. Revised summary of what changes

| change | status | cost |
|---|---|---|
| §4.1 `╷` at section boundaries | accepted, unchanged | zero |
| §4.2 window-relative gutter name | accepted, unchanged; justification weakened (§11.6) | zero |
| §4.3 section name in the footer | **overruled by the user** | — |
| §11 sticky header line above the list | **new recommendation** | one row of the list |
| repo in the footer | **unchanged**, keeps its slot | — |

Constants: `sectionWidth` stays 8, `titleWidth()` unchanged, no responsive
breakpoint moves, `scrollOff` stays 2, `window()`'s behaviour is unchanged. The
only new arithmetic is `chrome` in `View()` going from 1 to 2 (2 to 3 while
filtering) — a path that already exists for the filter prompt.

### For the implementer to decide

1. **What the count means.** `6 of 10` is drawn as "top visible row is the 6th
   of 10". "Rows visible of total" or "cursor index within section" are equally
   defensible; I have no evidence favouring one and they look identical at a
   glance. Pick one and put it in `DESIGN.md`.
2. **Whether to build §4.2 at all**, given §11.6 — it is now redundant as
   information and survives on consistency. Both need `start` from `window()`,
   so the marginal cost is small, but if the two-pass render is ugly this is
   the droppable one.
3. **Whether the sticky line hides when the whole board fits.** It would recover
   the h=30 threshold in §11.2 exactly. I am against it — chrome that appears
   and disappears is its own kind of jump, and it would mean the row count
   changes by one as PRs merge — but it is a coherent position and it is the
   user's call, not mine.
4. **Whether the header shows anything while filtering.** The prompt is already
   a second chrome row; three chrome lines in a 14-row pane is a lot. Showing
   the section name during a filter, when the board is deliberately narrowed and
   sections are being dropped, may be worth less than the row.

## 13. What I could not verify in this revision

- **Whether a top line whose text changes while scrolling feels restless.** This
  is the main risk of (a) and I cannot settle it from static frames. It needs a
  real session.
- **The h=30 threshold is today's board.** 29 rows is a snapshot; it moves as
  PRs open and merge, so the "board stops fitting" cost is real but not fixed
  at any particular height.
- **Which pane heights the user actually runs.** The entire cost argument in
  §11.2 turns on this and I am guessing. If they live at h=50 the row is free;
  if they live at h=14 it is 7.7%. I did not ask, and it would change how hard I
  argued.
- **The count's value.** I claim it is what makes the row pay for itself, but
  `uniform-rows.md` §6 already listed "whether losing the per-section count is
  felt" as unverified, and I have not verified it either — I have only argued
  that if it is felt, this is the only place it can go.
- **(b) with a background fill.** I rendered it and rejected it on reasoning
  about `selBg` competition (§11.1); I did not put it side by side with a real
  selected row to confirm the clash is as bad as I expect.

---

## 13. Revision two: the sticky line is reverted

Shipped at `39ad152`, run by the user, and rejected:

> *"is the designer ok with that? looks bad imho. duplicated headers, main top
> header show the 1 of 5 is very not clear, not related to the cursor at all…"*

**The line is removed.** §4.1 (`╷` at boundaries) and §4.2 (the window-relative
gutter name) stay — neither was implicated, and §4.2 is again the thing that
answers the original complaint. `chrome` in `View()` returns to 1, and 2 while
filtering.

### 13.1 What was actually wrong

I reproduced it in a real 120×40 tmux pane against the live board before
touching anything. All three of the user's complaints are correct, and they are
one root cause plus two consequences.

**The root cause: at 40 rows the board does not scroll, so `start` is pinned at
0 and the header is a constant.** I walked the cursor down 34 rows, from `MINE`
through to deep inside `ALL OPEN`. The header read `MINE · 1 of 5` on every
single frame. Not "rarely updates" — it never updated once. A field that cannot
change carries no information, and it sat in the most prominent position on the
board. *"Not related to the cursor at all"* is not a perception; it is literally
what the code does at this height.

**Consequence 1 — duplicated headers.** `stickyHeader` and `nameTopSection` both
read `meta[start]`. They are the same fact rendered twice, one line apart, by
construction — not an edge case:

```
  MINE  ·  1 of 5            <- stickyHeader(meta, start)
MINE     ╷▌   #3248  ✓  ○ ~ fix(webapp): reach the dependency popup  …
```

Whenever the top visible row's section starts on screen — which at 40 rows is
*always*, and at 14 rows is whenever you are near the top — the line above the
list says exactly what the gutter beneath it says. §11.6 argued the two
"reinforce each other". Adjacent and identical is not reinforcement.

**Consequence 2 — `N of M` reads as a cursor position.** `1 of 5` is shaped like
one. §11.7 flagged the definition as an open question and §12 told the
implementer to "pick one and put it in `DESIGN.md`". That was the wrong framing:
the problem is not which definition to pick, it is that *any* top-row-indexed
count is written in the visual language of a cursor position while meaning
something else. On a non-scrolling board it is pinned at `1 of N` forever, so the
one thing §11.4 claimed "makes the row pay for itself" is dead in the dominant
case.

### 13.2 Why I missed it

Being specific, because "I measured the wrong pane heights" is the honest
version and it is not the whole of it.

**I measured 14, 20, 24, 30 and 40+, and then reasoned about the wrong one.**
§11.2's table has a `40+` row. It says *"board fits either way, no observable
cost"* — I treated "fits" as meaning the row is **free**, and stopped. Fitting is
exactly the condition under which `start` is always 0, which makes the header a
constant and the count a constant. The cheapest case on the cost axis is the
*most broken* case on the value axis, and I never turned the table around to ask
what the feature is worth at each height. I only ever asked what it costs.

**§11.7 asked the right question and I answered it from a static frame.** It
says the content changing as you scroll is *"the single thing most worth looking
at in a real session"*. I never ran that session. Had I, the first thing I would
have seen is that at the user's height it does not change at all — the opposite
failure to the one I was braced for, and invisible to every mockup in §11.4 and
§11.5, because a mockup is one frame and the defect is that all frames are
identical.

**I asked which heights the user runs, in §13 of the old numbering, and shipped
before the answer arrived.** The doc says *"The entire cost argument in §11.2
turns on this and I am guessing."* Naming a load-bearing unknown is not the same
as waiting for it. That is the process error, and it is the one worth keeping.

**The §11.4 binding argument was sound and irrelevant.** Top-row binding does
guarantee the header never contradicts the row beneath it, and §11.7 correctly
identified the invariant as the risky part. Both were about *correctness*. The
line died on *usefulness*, which nothing in §11 measured.

### 13.3 Re-deriving cursor-binding, as asked

§11.4 rejected cursor-binding because it can name a section whose rows are not
beneath it. That objection assumed a scrolling board. Re-derived against the
non-scrolling 40-row case:

Cursor-binding **works** there — it is the only binding that tracks anything at
all when `start` is frozen at 0. But it does not save the line, for two reasons
measured on the real board:

1. **It becomes redundant against something already on screen.** When the board
   fits, every section boundary is visible, so the cursor's section is always
   the nearest `╷` above it — usually within a few rows. In the verified 40-row
   frame the cursor sits on `#345` with `INVOLVED ╷` one row above. A chrome line
   restating that is spending a row to repeat an adjacent fact.
2. **It would still be wrong in the short pane.** Captured at 120×14, scrolled:
   the shipped top-row binding says `NEEDS MY REVIEW · 5 of 10` while the cursor
   is in `INVOLVED`, two sections later. Cursor-binding fixes that frame and
   breaks the mirror-image one. There is no wrong-free binding, because one line
   cannot simultaneously mean "where the viewport is" and "where you are".

So the choice was never top-row vs cursor. It was: is there a question the board
does not already answer? At 40 rows, no.

**"Show it only when the top row's section start is not visible"** was also
considered and rejected. It fixes the duplication and would blank the line
entirely at 40 rows, which is the correct amount of ink — but chrome that
appears and disappears changes the list height by one as you scroll, and that is
the bug class `uniform-rows.md` exists to kill. A conditional header is worse
than no header and worse than an unconditional one.

### 13.4 What the board says now

Nothing was lost that the user had before `39ad152`, and one row came back:

- **Which section is this row in** — the gutter, on the section's first visible
  row, plus §4.2's window-relative name on the top row. Verified at 120×14
  scrolled: the top row reads `NEEDS M…` with `│`, so the name survives when the
  boundary has scrolled off. That was the original defect and it is still fixed.
- **Where do sections begin** — `╷` versus `│`, unchanged.
- **The full, unclipped name and the per-section count** — not on the board.
  They were not on the board before `39ad152` either. `l`/`h` jumps by section,
  which is the affordance that existed and still does. If the full name turns out
  to be genuinely needed, the footer (§4.3) is still available and is now the
  only proposal left standing that costs no row — but the user rejected it on
  prominence and I am not relitigating that without new evidence.

The cost recorded in §11.2 is recovered exactly: at 40 rows the board is 29 rows
of PRs plus a footer, with room to spare, and the h=30 "board stops fitting"
threshold is gone.

### 13.5 What I still cannot verify

- **Whether the full section name is missed.** It is now absent at every width.
  §10 flagged that a config whose rule names share a 7-character prefix would
  make the 8-cell crop ambiguous; this config does not have that problem
  (`MINE`, `NEEDS M…`, `MY TEAM…`, `INVOLVED`, `ALL OPEN` are all distinct at 8
  cells), but another config could.
- **Whether the per-section count is missed.** `uniform-rows.md` §6 listed this
  as unverified before the sticky line and it is unverified after it. Two
  attempts to place it have now been rejected; I would want evidence that it is
  wanted before proposing a third.

---

## 14. Revision three: the footer, bound to the cursor

The user changed their mind and asked for §4.3 back, with the count:

> *"sticky header looks good and I now think the designer was right, we should
> display the full header of the current section in the bottom with 1 of 5"*

**Shipped.** The footer's right field carries `SECTION · N of M` and the repo
yields the slot, which is §4.3 exactly. What is new is the binding, and it is
the whole reason this can work where §11 could not.

### 14.1 The binding is the cursor, not the top visible row

§13.1 measured the defect that killed the sticky line: at 40 rows the board does
not scroll, `start` is pinned at 0, and a top-row-bound field reads `MINE · 1 of
5` on every frame of a 34-row walk. The user's *"not related to the cursor at
all"* was a literal description of the code.

So this field is bound to the cursor's own section and the cursor's index within
it. Verified against the live board in a 40×120 pane, walking `j` 34 times from
`MINE` into `ALL OPEN`:

| keypress | footer | selected row |
|---|---|---|
| 4 | `MINE · 5 of 5` | `#2909` |
| 5 | `NEEDS MY REVIEW · 1 of 10` | `#3189` |
| 14 | `NEEDS MY REVIEW · 10 of 10` | `#2902` |
| 15 | `MY TEAM'S · 1 of 2` | `#3211` |
| 17 | `INVOLVED · 1 of 4` | `#2741` |
| 21 | `ALL OPEN · 1 of 14` | `#3253` |

The value changed on **every one of the 34 keypresses** and matched the selected
row on every one. That is the property §13.1 found absent, and it is what a test
now asserts directly rather than by argument.

§13.3 re-derived cursor-binding and found it correct but not worth a *row*. That
reasoning is intact and is why this is in the footer: the footer already exists,
so the field costs no line. The two objections §13.3 raised against cursor
binding were both about spending a row on it, and neither survives when the row
is not spent.

At 14 rows, scrolled, the mirror case §13.3 worried about was captured live: the
top row is `NEEDS M…`, the cursor is on `#345` in `INVOLVED`, and the footer
reads `INVOLVED · 2 of 4`. The shipped sticky line said `NEEDS MY REVIEW · 5 of
10` on this exact shape.

### 14.2 Why the footer is acceptable now and was not in §11

§11 rejected the footer on prominence — muted text in the bottom-right corner is
where this board puts what you may ignore. That objection was correct and is
unchanged; what changed is the alternative. §11 was choosing between a footer
and a pinned row. §13 removed the pinned row for reasons that have nothing to do
with prominence, so the choice now is between the footer and **nothing at all**,
which is what §13.4 left on the board. Muted and available beats absent.

It also answers something the board never could: the gutter has 8 cells, so
`MY TEAM'S` is drawn `MY TEAM…` and `NEEDS MY REVIEW` is drawn `NEEDS M…`. The
footer is the only place either name is legible in full, and the only place the
count exists at all — §13.5 listed both as unverified losses.

### 14.3 Degradation

The footer's left field already clipped when the two fields did not both fit;
what changed is that the right field no longer disappears when it does. Measured
live at 80 columns:

```
  j/k move · l/h section · enter open · d detail · / filter · ?… MINE · 1 of 5
  j/k move · l/h section · enter open · d detail · / filte… MY TEAM'S · 2 of 2
```

The keys clip and the name survives whole, and the keys give up exactly as many
cells as the name needs. Below the point where the name would leave the keys
under 8 cells, the right field goes too rather than being clipped into a
half-truth like `NEEDS MY REVI`.

### 14.4 What this does not change

`chrome` in `View()` stays 1, and 2 while filtering. No line was added, so
`TestNoChromeLineAboveTheList` and the one-keypress-one-line invariant are
untouched — this is the property that made the footer the only proposal left
standing in §13.4.

### 14.5 What I could not verify

- **Whether the count is what the user wanted it to mean.** `N of M` is now the
  cursor's position in its section, which is the reading §13.1 said the shape
  implies. The user asked for "1 of 5" without defining it; this is the only
  definition that changes on every keypress, so it is the one that carries
  information, but it is an inference from the shape rather than from the ask.
- **Whether the footer is prominent enough now.** §11's objection was never
  answered, only outvoted by the alternative being nothing. If it turns out to
  be too quiet, the remaining lever is colour — the field could leave `muted`
  without costing a row — and that was not tried.
