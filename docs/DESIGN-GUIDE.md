# pile — design guide

`DESIGN.md` records *what* the board looks like and why each decision went the
way it did. This is the layer above: the principles those decisions fall out of,
so the next question has an answer before someone has to re-argue it.

Where the two disagree, `DESIGN.md` wins on specifics (it is checked against the
code) and this wins on direction.

Read the prior art notes in §8 before proposing anything that breaks a rule
here — most of these were learned the expensive way by lazygit, k9s, gh-dash and
this board.

---

## 0. The one sentence

**A board where everything is fine has almost no colour on it.**

Every rule below is a consequence of that. The board is not a dashboard to
admire; it is a thing you glance at to find out whether you can close the pane.
If a healthy board is loud, the unhealthy one has nowhere left to go.

The design target from `DESIGN.md` stands as the tiebreaker: a 20-row pane, the
user's own theme, understand state in 2 seconds.

---

## 1. Colour is a budget, not a palette

Colour is the scarcest resource on the board, because its only real power is
*relative*. A hue is loud only if its neighbours are quiet. Every element that
takes a colour taxes every other element's ability to signal.

Spend it in this order:

| tier | what it is | who gets it |
|---|---|---|
| **Loud** | something is wrong and you must act | failing CI, changes requested, conflicts, review required |
| **Quiet** | something is settled; confirmable, not noticeable | passing CI, approved |
| **Tinted** | classification — what *kind* of thing this is | conventional-commit type |
| **Silent** | structure and metadata | tree glyphs, age, author, scope, ticket, drafts, counts, footer |

**Rules:**

1. **Loud is reserved for the actionable.** If a colour can be bright on a row
   you would do nothing about, it is in the wrong tier.
2. **Tinted never outranks Loud.** A classification colour must never read as a
   status: `fix` on the same red as a failing check would make the row look
   broken. The type is ANSI 4 — a slot the status columns do not use — so it is
   kept apart by *which* colour it is rather than by being a desaturated
   version of one.
   The tier has exactly one occupant. Author identity was here and moved to
   Silent: it was a grouping hint on a fact the initials already carried, and
   classification is the tier most likely to overspend, because every field
   feels like it deserves its own hue (§3).
3. **Silent is the default.** New fields start `muted` and have to earn colour by
   showing that the board is harder to read without it.
4. **Don't let two channels encode the same fact.** If the glyph already says
   "failing", the colour is reinforcement, not a second signal — which is why
   removing the colour must leave the board usable (§2).

**On adding a new colour:** the question is never "is this hue nice". It is
"which existing element is this stealing attention from, and is that trade
right". If you cannot name the victim, you are over budget.

---

## 2. Monochrome first, three tiers of capability

Design in layers, and keep each tier independently correct:

- **Monochrome** — strip every colour. Is the board still usable? Glyph shape,
  column position and text must carry the whole meaning. `DESIGN.md` §3.2 keeps
  a literal monochrome read-out of the status cluster; that test is the standard.
- **16 ANSI** — is the hierarchy readable? Status colours live here on purpose
  (see §3).
- **256 / true colour** — is it pleasant? Enhancement only. Nothing load-bearing
  may live at this tier alone.

A user on a monochrome SSH session, with `NO_COLOR` set, or on a terminal theme
you have never seen, gets a working board. Colour is the last layer, never the
foundation.

**Corollary:** never encode a fact *only* in colour. The author column is the
clean example, and it went one step further than the rule requires. It carried
a hue that grouped while the initials stayed the identity, so a collision cost
nothing and a colourless terminal lost nothing — the rule was satisfied. The
hue was removed anyway, because a channel that can be deleted without losing a
fact is a channel worth pricing (§1): it was spending fifteen fixed values to
decorate what three characters already said.

"Not the sole carrier" is the floor, not the goal. The question after it is
whether the redundant channel earns its cost.

---

## 3. Two colour spaces, two different jobs

The board deliberately mixes ANSI 0–15 and the 256-cube. This is not
inconsistency; the spaces have opposite properties and each job needs one of
them.

**ANSI 0–15 — semantic status. The user's theme wins.**

When you say "red", the terminal decides what red is, so status colour matches
whatever palette the user already lives in. Use it for anything meaning
success / failure / warning.

- Use `1` not `9`, `2` not `10`, etc. Bright slots are frequently pale and
  low-contrast on light themes.
- Never use `0`, `7` or `15` as a foreground.
- `4` and `12` have no fixed relationship — several popular themes define `12`
  as `4`, so "brighten it" is a no-op there. If you need a *guaranteed* lift,
  leave the space (see below).

**256-cube — fixed identity. You win.**

Use it when you need a specific, stable, contrast-checked shade that must not
move under the user's theme — and **right now nothing on this board qualifies.**
There were three: the selected-row fill (237), the section header fill (234) and
the accent while selected (75). All three are gone, and how each died is the
useful part:

- **The accent (75)** was defending a constraint that did not exist. Its
  justification was a contrast figure computed against a nominal ANSI 4 rather
  than a real palette; measured properly, plain ANSI 4 was better on every fill.
  *A fixed value resting on an unverified number.*
- **The header fill (234)** was invisible in the theme it was meant to serve:
  1.04:1 against the background. *A fixed value solving a problem by being too
  timid to have any effect.*
- **The selection fill (237)** was defensible — mid-grey really is a bad
  backdrop — but the objection was about mid-grey, not about index 8, and the
  theme in use renders 8 as a tinted dark. *A fixed value whose premise was true
  in general and false here.*

**Backgrounds are still the honest case for reaching here**, and if anything
ever does qualify it will probably be a fill: a fill must contrast with the
terminal's own ground, which is unknown, and no ANSI index means "slightly
different from whatever is behind me". A *foreground* reaching for the cube is
nearly always the palette mistake below.

But the bar is a **measured** requirement in the **actual target theme**, not a
plausible-sounding one. Two of the three above would not have survived that
test.

The trade is explicit: these will not match the user's theme, so they must be
chosen to be legible against *both* light and dark backgrounds and against
`selBg`. That is a measurable obligation, not a taste call — see §6.

**The cube is a last resort, and the board has twice proved it.** The author
column and the conventional-commit types both started here and both came back.
Each was a *classification* — who wrote this, what kind of change is this —
and a classification does not need a fixed shade, because nothing breaks when
the user's theme moves it. Between them they spent twenty fixed values
overriding the user's palette to decorate two facts that the initials and the
type word already carried in text. `DESIGN.md` §3.3.1 and §3.3.2 record both
reversals.

The test before reaching for the cube: **would this still be correct if the
user's theme changed it?** If yes, it belongs in ANSI 0–15. Only a contrast
floor you can measure justifies leaving.

**Never use true colour for anything load-bearing.** Not every terminal has it.

---

## 4. Position is stronger than colour, and free

The board's real information channel is the grid. A column that never moves
teaches its meaning once and then costs nothing to read forever.

- **One flexing column, ever.** Title flexes; everything else is fixed. Slack
  has exactly one place to go, so the status cluster is at the same cells at
  every terminal width.
- **Always emit every slot, blank when empty.** Nothing shifts as state changes.
  A column that moves when data changes forces a re-scan of the whole row.
- **Reuse glyphs across columns when position disambiguates.** `✓` in the CI
  column and `✓` in the review column is the same meaning in two dimensions;
  this is a feature, and it shrinks the vocabulary the user must learn.
- **The left edge never moves.** Columns 0–20 are identical from 50 to 400
  columns. Responsive tiers drop fields from the right, in order of how little
  they change what you *do*: age and author first, then review state.

If you are reaching for colour to separate two things, first check whether a
column, an alignment, or a glyph shape would do it — those survive `NO_COLOR`,
theme changes and colourblindness.

---

## 5. Every line is a row

The viewport rests on one invariant: **row height never depends on data, and
nothing on the board is a non-row line.**

- One keypress scrolls by at most one line. Always.
- No header bands inside the list, no blank separators, no continuation lines,
  no data-dependent wrapping.
- Sections are a gutter, not a header line — a header line makes one keypress
  scroll two lines whenever it crosses the top edge.
- Chrome is one line (the footer), two while filtering.
- Empty and loading sections still occupy exactly one row, drawn with the same
  gutter.

This is the rule most likely to be broken by a reasonable-sounding feature
request ("just show the failing gate names under the row"). The answer is an
overlay, not a taller row. Anything that wants more space than a row gets a
mode, not an exception.

**Conditional chrome is the same bug.** A line that appears only sometimes
changes the list height as you scroll. If a line is worth drawing, draw it at
every height — including when it reads as trivially true.

---

## 6. Contrast is checked, not eyeballed

Any colour must clear a measured contrast ratio against every background it can
land on: the terminal default (light *and* dark) and `selBg`.

- Target **4.5:1** for text, **3:1** for glyphs and structural marks.
- **Name the theme, or the number is meaningless.** A ratio quoted for an ANSI
  slot is a ratio in one *specific* palette, because the slot is an indirection.
  This is not pedantry: the selected accent carried the comment "ANSI 4 measures
  1.21 against `selBg`" for months, justifying a fixed cube colour. The figure
  had been computed against a *nominal* ANSI 4 rather than any real theme's.
  Measured against the palette actually in use it was **5.40:1** — the
  constraint did not exist and the fixed value was protecting nothing. Read a
  palette out of the terminal's own theme file; do not reason from what "blue"
  ought to be.
- **A mid-grey fill is the worst possible backdrop** — only a small fraction of
  cube colours clear 3:1 against it, so a mid-grey selection makes the whole
  palette fight for legibility. This is why the fill must be measured, and it is
  a real constraint: it is the reason an earlier selected row moved *off* ANSI 8
  to a dark cube grey.
  **But it is a claim about mid-grey, not about index 8.** Many themes render 8
  as a tinted dark rather than a mid-grey — Catppuccin Mocha gives `#585b70` on
  a `#1e1e2e` ground — and there the objection simply does not apply. `selBg` is
  ANSI 8 today for that reason. The cost is that the fill's quality now varies by
  theme: `DESIGN.md` §3.4 has the cross-theme table, and it measures poorly in
  Gruvbox and Solarized. **Check 8 in the target theme before assuming either
  way.**
- **A highlight must set BOTH ground and figure.** A background-only highlight
  inherits whatever foreground the text already had, and there is no palette
  choice that makes that safe: the ANSI slots are *bright* on a dark theme by
  design, so a coloured fill plus an inherited light foreground is
  light-on-light. The search hit shipped this way and measured 1.03–1.38:1 in
  the author's own theme, under a fill that was 10.74:1 against the board — a
  loud block with invisible contents, which is worse than no highlight because
  it draws the eye to the one place it cannot read. `DESIGN.md` §3.3.3 has the
  measurements and the fix.
- **`Reverse` is the strongest highlight available, and it needs no palette.**
  It takes the theme's own foreground/background pairing, which is by
  definition the best contrast that theme guarantees, so it is correct on light
  and dark without measuring either — the only candidate that cleared 4.5:1
  across eight themes. It is the right tool for a *short run* of text. It is
  still the wrong tool for a whole row (§7), because a row carries status
  colour that reverse would swallow; a few runes inside a title do not.
- **Never force a foreground on the selected row.** It inherits the terminal
  default, which is guaranteed to contrast with the terminal's own background.
  Background detection is unreliable; first paint must be correct without it.

When a palette "keeps collapsing" to a handful of usable entries, the
constraint is almost always the background, not the hues. Fix the background
first.

**Palette sizing:** when colours are hash-bucketed, only the *length* decides
who collides — not which colours are in the list. Re-run the collision check
over the live data before changing the count, and accept that above ~N distinct
values collisions are pigeonhole, not a bug. That is fine when colour is a
grouping hint and something else carries identity (§2).

---

## 7. Selection, focus and feedback

- The selected row must be **unmistakable**: a fill to the terminal's right edge
  (a partial fill reads as a rendering artifact), plus a mark in column 0.
- Apply the fill **per-segment**, not wrapped around the finished line — each
  segment's style emits a reset that would terminate an outer background
  mid-row.
- Segments keep their own semantic foreground on top of the fill. Selection
  adds emphasis; it does not repaint meaning.
- **Never freeze the board.** All I/O is async with visible progress. A section
  that is loading shows a spinner and reserves plausible height so the skeleton
  starts near its final shape; it never shows an empty list or a fake "none".
- A failure claims nothing and says so in place, rather than stalling everything
  below it.

---

## 8. Progressive disclosure

Three tiers, and only three:

1. **The footer** — the few keys that matter right now.
2. **`?`** — the full reference.
3. **The docs** — everything else.

The board itself teaches nothing explicitly. It earns that by keeping the glyph
vocabulary small and shape-distinct enough to be learned by inference in a few
sessions.

Overlays (`?`, `d`) replace the whole view and carry no chrome — with one
exception: **if a surface scrolls, it must say so at every height**, including
when everything fits. A scrollable page with no affordance at exactly the
heights a full-screen terminal reports is indistinguishable from broken.

**Mode contracts matter more than key choice.** `d` closes on any key, because
`d` then `j` inspects a PR and carries on down the board in one motion. `?`
cannot, because `j` means scroll there — so its rule is the inverse: scroll keys
scroll, *everything else closes*, and the page says which. Pick the contract
first, then the keys.

Keys follow the terminal lingua franca: `j`/`k`, `/`, `?`, `g`/`G`, `esc`.
User-configured actions can never shadow a builtin — a taken key is dead config,
and silently dead config is worse than a refusal.

---

## 9. Density is the product

Terminal cells are not pixels: a wasted column is measurable real estate. The
board is 100% data at every shape, and that is the feature people came for.

But density is bought with *structure*, never with noise:

- More rows is better than more per row.
- A field earns its cells by changing what you do, not by being available.
- When something must go, drop what changes how urgent a thing *feels* before
  what changes what you *do*.
- Clipping is honest. A half-coloured prefix on a narrow terminal is better than
  drifting out of alignment with what is actually on screen.

---

## 10. Checklist for a visual change

Before it ships:

- [ ] Does it hold the one-line-per-row invariant at every width and height?
- [ ] Is it readable with all colour stripped?
- [ ] Does it survive `NO_COLOR`, a light theme, and a dark theme?
- [ ] Does any new fixed colour clear 4.5:1 (text) / 3:1 (glyph) on default
      background *and* `selBg`?
- [ ] If it takes colour: which element is it stealing attention from, and is
      that trade right?
- [ ] Does it move anything left of the title at any width?
- [ ] Does it add a second signal for a fact already signalled?
- [ ] Does it make a healthy board louder?
- [ ] Has it been seen in a real pane, not just reasoned about?

That last one is not a formality. Contrast, glyph rendering and font fallback
are not predictable from source.

---

## 11. Prior art, and what was taken from it

- **lazygit** — persistent multi-panel layout, fixed positions, spatial
  consistency; per-author colouring so the same person is the same colour on
  every row. Taken: the panel-position discipline. The author-colour idea was
  taken, shipped, and later removed — lazygit's board is a working surface you
  live in, where learning a person's colour pays off over a long session; this
  one is a glance-and-dismiss board, and the hue never earned its cost there
  (§1).
- **k9s** — drill-down navigation, dense uniform rows, status by glyph plus
  colour. Taken: rows stay uniform no matter what the data does.
- **gh-dash** — the direct ancestor, and the source of two anti-goals: it could
  not express `review-requested:@me OR involves:@me`, so the same PR showed up
  under two sections; and it shipped a `negative Repeat count` panic from an
  unclamped pad. Hence first-match-wins bucketing, and `max(0, w - width(s))`
  everywhere.
- **GitHub's own design system** — density treated as a feature, and semantic
  colour grouped by functional role rather than by appearance.
- **The `NO_COLOR` convention and the ratatui community's cross-terminal
  findings** — respect `NO_COLOR` unconditionally; prefer named ANSI for
  semantics; accept that perfect cross-terminal fidelity is impossible and
  design for the common case.

Sources: [The Terminal Renaissance](https://hyperbliss.tech/blog/2026.04.04_terminal-renaissance/) ·
[ratatui: choosing colors across terminal emulators](https://github.com/ratatui/ratatui/discussions/877) ·
[Terminal color fundamentals](https://terminfo.dev/fundamentals/color-fundamentals) ·
[lazygit](https://www.bytesizego.com/blog/lazygit-the-terminal-ui-that-makes-git-actually-usable) ·
[GitHub design system](https://oh-my-design.kr/design-systems/github)

---

## 12. Open questions

Honest about what this guide does not settle:

- **No accessibility standard exists for TUIs.** WCAG ratios are borrowed from
  the web and applied by analogy. The mitigation for what cannot be audited is
  §2 (colour is never the sole carrier), not the palette itself.
- **Light themes are measured thinly.** Everything is themed now, so light
  behaviour is inherited rather than chosen — which is the right default, but
  `selBg` = ANSI 8 on a *light* theme is the untested case: 8 is "bright black"
  and may land close to the ground. Not checked with a meter on a light theme.
- **Theme-dependent contrast is now a live risk rather than a fixed one.**
  `DESIGN.md` §3.4's cross-theme table shows the selection fill measuring well
  in Mocha and Tokyo Night and poorly in Gruvbox and Solarized. The board is
  developed against Mocha, so this is unmeasured in daily use for anyone else.
- **An audit that a later commit invalidates is worse than no audit.** The
  commit collapsing the type palette closed by naming the fixed values left;
  the very next commit added `headerBg` and nobody noticed until review. A count
  in a commit message is a claim about the whole tree at one moment, and nothing
  re-checks it. `DESIGN.md` §3.3's table is the place that gets maintained.

Three questions that were open here are now closed, and all three closed by
deletion rather than by an answer:

- *The author palette's colourblind safety* — there is no palette. The column is
  `muted`, and the initials were always the identity.
- *The commit-type tints on a light theme* — there are no tints. The type is
  ANSI 4, so the user's theme picks it and is correct on light by construction,
  the same way every status colour is.
- *Whether the fixed fills work on a light theme* — there are no fixed fills.
  The selection is ANSI 8 and the header has none at all.

That is the general shape of the fix, and worth stating plainly: **a fixed value
raises a light-theme question that a themed value never has.** All three were
answered by moving back into ANSI 0–15, not by measuring harder — though the
last one trades the question for a different one, since a themed fill varies in
*quality* by theme even while it is always *plausible* (§6).
