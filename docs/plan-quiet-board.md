# Plan — muted author, single-hue type

Two changes, decided from mockups in a design review. Each is independently
shippable and should be its own commit.

A third change — replacing the section gutter with header rows — was attempted
and withdrawn; §1 records why, so it is not proposed again.

Design rationale lives in `DESIGN-GUIDE.md`; the concrete spec being amended is
`DESIGN.md` §3. **Both docs must be updated as part of this work** — a change
that leaves §3 describing the old board is not done.

---

## 1. ~~Replace the section gutter with an inline header row~~ — REJECTED

**Do not implement this.** It was proposed in a design review, built, measured,
and withdrawn. Recording it here so it is not proposed a third time.

The design: drop the 8-cell gutter, draw a full-width header row before each
section, drop every width breakpoint by 10.

**Why it fails.** A header is a line that is not a row, so whenever one crosses
the top edge a single `j` scrolls the board more than one line. Measured during
this attempt at a section boundary: **3 lines** with a blank separator, **2**
without.

That exactly reproduces `uniform-rows.md` §4, which measured the same two
layouts at `height=19` and recorded deltas of `0,1,2,3` and `0,1,2` against
`0,1` for the gutter. The gutter **is** the fix that came out of that
measurement. §4 is titled "the brief was right and I was wrong" for this reason.

The plan as first written asserted "one keypress still moves at most one line"
would hold for an inline header. That claim was wrong and was made without
reading `uniform-rows.md`. `DESIGN.md` §3.5.1 covers the *sticky* variant; §4 of
`uniform-rows.md` covers the *inline* variant. Both fail, for different reasons.

Anchoring on the header and rate-limiting the viewport were both tried during
this attempt; they either fail or push the cursor off-screen on short panes.

**What stays unsolved, and is not worth the invariant:** the section name clips
to `NEEDS M…`, and there is no per-section count on the board.
`uniform-rows.md` §6 already lists both as known, accepted costs.

---

## 2. Mute the author column

`internal/ui/render.go` — delete `authorPalette` and `authorStyle`; render the
author with `mutedStyle`, like the age column.

Rationale: the initials were always the identity (`DESIGN-GUIDE.md` §2); hue was
a grouping hint that cost 15 hardcoded values overriding the user's theme. The
existing comment on `authorWidth` already says this column should be "readable
without being loud".

Keep `initials()` and `authorWidth` exactly as they are.

---

## 3. One hue for every conventional-commit type

`internal/ui/title.go` — replace the `typeStyles` map with a single style
applied to any recognised type.

- Use a **themed ANSI slot**, not a 256-cube value. ANSI 4 is the candidate;
  `accentStyle` already uses it for the PR number, so check in a pane whether
  sharing reads as confusing or as consistent. If it collides, ANSI 6 is the
  fallback — but do not reach for the cube.
- Scope and ticket stay `Faint`, unchanged.
- Subject stays terminal-default, unchanged.
- The set of **recognised** types does not change — every key currently in
  `typeStyles` still parses as a type; they just share one colour now. An
  unrecognised word still falls through to prose.

`titlePartStyle` keeps layering onto the row's own style, so drafts stay faint
and selected rows stay bold.

After this, the only fixed non-ANSI values left in the UI are `selBg` (237) and
the selected-row accent (75), both deliberate and documented.

---

## Tests

`internal/ui/title_test.go` asserts per-type colours — update it to assert one
shared style, and keep the existing cases proving that a non-conventional word
with a colon stays prose and an unparsed title emits no SGR.

No layout tests change: neither remaining commit touches widths, row structure
or the cursor.

`make check` must pass.

---

## Docs

- `DESIGN.md` §3.3 (colour tokens) — the author palette and the per-type map both
  leave the table; one themed type colour replaces them.
- `DESIGN-GUIDE.md` §12 — drop the author-palette colourblind question and the
  light-theme question for these two systems, since neither is a fixed value any
  more. `selBg` 237 and accent 75 remain open.
- `uniform-rows.md` §4 — add a short note that the inline header was proposed
  again and rejected again, with the measured deltas from this attempt.

---

## Verification

Build, install, and look at it in a real pane before calling any of this done —
contrast, glyph rendering and font fallback are not predictable from source.
Check specifically:

- the type hue against the PR number colour if both end up ANSI 4,
- the muted author against the muted age column — they should read as one
  quiet tier, not two,
- a light terminal theme, at least once,
- `NO_COLOR=1`.

---

## Out of scope

Anything touching layout, row structure, widths or the cursor. Collapsible
sections. Any change to `selBg` or the selection accent. The section-name
truncation and the missing per-section count stay as they are.
