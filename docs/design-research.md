# prs-mng — TUI Design Research

Research for the redesign of the `prs-mng` terminal PR board. This document is
**requirements and evidence**, not visual design. The designer decides the how.

Researched: 2026-09-16. All claims below are sourced; where I could not verify
something I say so explicitly rather than guessing.

---

## 0. Constraints (binding on the redesign)

These are fixed inputs, not findings. The designer must respect all of them.

1. **Terminal only.** Go + Bubble Tea + lipgloss. No pixel graphics, no mouse
   assumptions, no web-style layout primitives.
2. **Must survive the user's existing terminal theme.** Users run wildly
   different color schemes, light and dark. Hardcoding colors that only work on
   one background is a documented failure mode (see §4.1, §4.2). This is the
   single highest-risk constraint in the project.
3. **Nerd Font is available.** Glyphs may be used — but see §4.5 for the version
   -skew failure mode found in the wild.
4. **Short panes are the common case.** Ghostty quick-terminal, often 20-30
   rows. Sometimes full screen. Both must work; the short case is the *default*,
   not the degraded case.
5. **Variable width.** Must degrade gracefully when narrow.
6. **Glance tool.** "Understand state in 2 seconds" beats "maximum information".
   This is the tiebreaker whenever two requirements conflict.

---

## 1. Summary — the most important findings

1. **Hardcoded hex colors are the #1 theming failure mode, and the closest
   comparable tool already fixed it by migrating to ANSI indices** — gh-dash
   accepted exactly this bug report and merged PR #771, and its current defaults
   are `lipgloss.ANSIColor(...)` pairs, not hex (§2.1).
2. **The 16 ANSI colors are an indirection layer, not a limitation** — they
   resolve through the user's own terminal theme, so an app that uses them is
   automatically themed by every scheme the user already has (§4.1).
3. **"Usable at 16 colors, beautiful at true color"** is the governing design
   rule from the most rigorous TUI design writeup found; corollary: *if removing
   all color makes the app unusable, the design is broken* (§2.6).
4. **Selection should be a full-row background, not a glyph** — lazygit's default
   is a background-colored selected line, and its own issue tracker shows that
   weight-only/dimmed selection reads as "greyed out / disabled" to users (§2.2).
5. **Color alone does not scale as a triage mechanism — ordering does.** The k9s
   tracker documents operators unable to find failing pods among healthy ones
   despite correct color coding; the ask was to sort problems to the top (§2.3).
6. **Status must be encoded in a non-color channel too** (shape/glyph/text), both
   for red-green colorblindness (~1 in 12 men) and for theme robustness (§4.3).
7. **Vertical space is contested and users notice waste** — gh-dash's 2-line logo
   drew "wasting space", "biggest configuration annoyance", "attention hogging"
   (§4.4). In a 20-row pane every non-data row is expensive.
8. **Responsive layout wants an explicit breakpoint, not gradual squeeze** —
   gh-dash flips its preview pane from right to bottom when the main content
   would drop below 80 columns (§2.1).

---

## 2. Findings by tool

### 2.1 gh-dash — the closest analogue

GitHub PR/issue dashboard TUI, Go + Bubble Tea. Same stack, same domain.

**Column model.** Default PR columns and widths
([layout/pr docs](https://www.gh-dash.dev/configuration/layout/pr/)):

| column | width | note |
|---|---|---|
| updatedAt | 7 | |
| state | 3 | narrow status cell |
| repo | 15 | |
| **title** | 15 | **`grow: true`** — absorbs all leftover width |
| author | 15 | |
| labels | 22 | `hidden: true` by default |
| numComments | 3 | narrow status cell |
| reviewStatus | 3 | narrow status cell |
| ci | 3 | narrow status cell |
| lines | 16 | |

Columns support `width`, `grow`, `hidden`, `align`
([options docs](https://www.gh-dash.dev/configuration/layout/options/)). Docs
recommend "only setting [`grow`] for one column in a given section layout".
Overflowing text is truncated to two characters shorter than the column width.

**What this does well:** status lives in **3-cell columns packed together**, and
exactly one column (title) flexes. That keeps the status cluster tight and
constant-width regardless of terminal size — the eye learns one fixed position.
This is directly applicable to our "columns are too widely spaced" complaint.

**Theming — the important part.** gh-dash originally enforced hex-only colors
and shipped separate light/dark hex defaults. Issue
[#770](https://github.com/dlvhdr/gh-dash/issues/770) ("Support ANSI color
indices in theme configuration") argued:

> "I use a terminal color scheme (Solarized) and I regularly switch between dark
> and light variants depending on ambient light. Since my 16 base ANSI colors are
> remapped by the terminal theme, apps that reference ANSI indices automatically
> look right in both modes — no config change needed."
>
> "Right now, the `HexColor` type enforces hex-only values … I end up having to
> hardcode hex values that only match one of my two themes."

The maintainer agreed ("Sounds good! Happy to look at a pr"), and
[PR #771](https://github.com/dlvhdr/gh-dash/pull/771) **merged 2026-02-13**.
Current source (`internal/tui/theme/theme.go`, fetched via the GitHub API)
confirms the defaults are now ANSI indices wrapped in adaptive light/dark pairs:

```go
SuccessText: compat.AdaptiveColor{Light: lipgloss.ANSIColor(10), Dark: lipgloss.ANSIColor(10)},
WarningText: compat.AdaptiveColor{Light: lipgloss.ANSIColor(11), Dark: lipgloss.ANSIColor(11)},
ErrorText:   compat.AdaptiveColor{Light: lipgloss.ANSIColor(1),  Dark: lipgloss.ANSIColor(9)},
SelectedBackground: compat.AdaptiveColor{Light: lipgloss.ANSIColor(7), Dark: lipgloss.ANSIColor(236)},
```

Note the pattern: **semantic status colors use 0-15** (theme-following), while
some structural greys use 256-palette values where a specific shade is needed.
Success/warning are identical across light and dark *because* ANSI 10/11 already
follow the terminal theme. This is a validated reference implementation of our
exact constraint, in our exact stack.

**Responsive behavior.** Preview pane `position: auto` "automatically chooses
`right` when the terminal is wide enough, and switches to `bottom` when the main
content would have fewer than 80 columns"
([defaults](https://www.gh-dash.dev/configuration/defaults/)). An explicit
breakpoint with a layout *mode change*, not a gradual squeeze.

**What it does badly / open complaints:**
- [#671](https://github.com/dlvhdr/gh-dash/issues/671) "Make logo smaller or at
  least configurable" — "The logo uses two lines in the shell, so effectively
  wasting space". Comments: "This is my biggest configuration annoyance right
  now", "that logo is attention hogging. can be very distracting at times."
- [#937](https://github.com/dlvhdr/gh-dash/issues/937) users want the PR/issue
  **number** as a dedicated column — identity matters for scanning.
- [#920](https://github.com/dlvhdr/gh-dash/issues/920) wants unresolved review
  threads and mergeable state surfaced as columns — i.e. *actionability* signals
  are missing from the default board.
- [#530](https://github.com/dlvhdr/gh-dash/issues/530) "[BUG] Some PR completely
  break layout" — layout collapse triggered by content, only with preview open.
- [#461](https://github.com/dlvhdr/gh-dash/issues/461) ANSI colors from preview
  content **bleed into the list on the next line** — untrusted content escaping
  its style scope.
- [#913](https://github.com/dlvhdr/gh-dash/issues/913) /
  [#876](https://github.com/dlvhdr/gh-dash/issues/876) crashes when
  `BackgroundColorMsg` had not arrived yet — background *detection* is async in
  Bubble Tea and is a real source of nil-state bugs. Directly relevant if we
  adopt adaptive color.
- [#796 (release note)] fixed a `strings: negative Repeat count` panic "when the
  sidebar is narrow" — narrow-width arithmetic is a known crash class.

### 2.2 lazygit — selection and theme integration

[`docs/Config.md`](https://raw.githubusercontent.com/jesseduffield/lazygit/master/docs/Config.md)
theme defaults, verbatim:

| key | default |
|---|---|
| `activeBorderColor` | `[green, bold]` |
| `inactiveBorderColor` | `[default]` |
| `selectedLineBgColor` | `[blue]` |
| `inactiveViewSelectedLineBgColor` | `[bold]` |
| `unstagedChangesColor` | `[red]` |
| `defaultFgColor` | `[default]` |

**Key observations:**
- Defaults are **named ANSI colors** (`green`, `blue`, `red`) plus a `default`
  keyword meaning "the terminal's own". Hex is *permitted* but not the default.
  Same conclusion as gh-dash arrived at independently.
- Selection default is a **background fill** (`selectedLineBgColor: [blue]`), not
  a marker glyph.
- **Focus is encoded separately from selection**: the focused panel gets
  `activeBorderColor: [green, bold]`, while the selected row in an *unfocused*
  panel degrades to `[bold]` only. Two independent visual channels for two
  independent states — worth stealing conceptually even though we have no panels.
- `reverse` attribute is documented as "useful for high-contrast" — a
  theme-independent way to guarantee selection contrast, since it swaps whatever
  fg/bg the terminal already has.

**Complaints found.** [Issue #1845](https://github.com/jesseduffield/lazygit/issues/1845)
"Unintuitive color hint on selected/focus item": the reporter said selected items
appeared **"greyed out" rather than highlighted**, which they found unintuitive
and uncommon in UI design. I could not retrieve the full comment thread, so I
cannot report the maintainer's resolution — treat the *complaint* as the
evidence, not the outcome. [Issue #802](https://github.com/jesseduffield/lazygit/issues/802)
"Some highlighted lines are still very hard to see" is a second data point that
low-contrast selection is a recurring real-world failure.

Community discussion notes lazygit users commonly bind the selection background
to their colorscheme's `Visual` highlight group, i.e. users *want* selection to
match the highlight color they already recognize elsewhere.

### 2.3 k9s — dense tables and the limits of color

- **`default` as a first-class color.** k9s ships a
  [`transparent.yaml`](https://github.com/derailed/k9s/blob/master/skins/transparent.yaml)
  skin where **all 20+ color slots are `"default"`**, preserving the terminal's
  own background throughout (body, prompt, info, dialog, frame, table, logs).
  Per the [skins docs](https://k9scli.io/topics/skins/), `default` explicitly
  means "transparent background color".
- **The same hardcoding bug.**
  [Issue #294](https://github.com/derailed/k9s/issues/294): "Prior to 0.8.0, the
  skin colors were respecting my terminal emulator's color scheme (for the first
  16 colors, i.e. system colors). Now they are not" — a regression across
  multiple terminal emulators.
  [Issue #1234](https://github.com/derailed/k9s/issues/1234) asks for ANSI
  indices, arguing the 16 base colors are "an indirection to the real colors
  rendered, based on a system wide terminal specific config", enabling
  "consistently looking terminal apps"; the reporter noted that using *hex codes
  for the same 16 colors does not work* — "they remain rendered into the same
  colors while changing the system colors." That distinction matters: `#00ff00`
  and ANSI 10 are not interchangeable.
- **Color coding alone fails at scale.**
  [Issue #3589](https://github.com/derailed/k9s/issues/3589): with 1000+ pods,
  sorting by status scatters `CrashLoopBackOff` / `ImagePullBackOff` / `Error`
  alphabetically among healthy `Running` pods. "In production environments,
  operators need to immediately spot failing pods, but the current sorting forces
  them to scroll through hundreds of healthy pods to find the problematic ones."
  The requested fix is **priority ordering** — errors first, healthy last. This
  is the single most transferable finding for "what do I need to do right now".

### 2.4 btop / bottom — polish techniques

btop's visual reputation comes from Unicode box-drawing plus **Braille patterns
(U+2800–U+28FF)** for sub-cell graph resolution, truecolor gradient meters that
sweep a palette ramp across a bar's length, and multiple built-in themes
([btop](https://github.com/aristocratos/btop)). Reviews describe it as looking
"more like a movie prop than a typical terminal utility".

**Assessment for us: mostly not applicable.** Braille density and gradient meters
serve *continuous quantitative* data (CPU %, network throughput). Our data is
categorical (passing/failing/running) and textual. Adopting these would be
decoration. The one transferable idea is btop's *theme-as-first-class-artifact*
approach: a named, swappable palette rather than colors scattered through render
code.

### 2.5 delta — background detection

delta auto-detects light vs dark terminal background by querying the terminal via
OSC sequences (`terminal-colorsaurus`), and lets users force `dark = true` /
`light = true`
([docs](https://dandavison.github.io/delta/choosing-colors-styles.html)).

**Critical caveat, and it applies to us.** Delta's docs state that querying the
terminal requires "exclusive" access — it reads/writes the terminal and toggles
raw mode — which **causes race conditions with pagers such as `less`**, and that
manual override "is necessary when running delta in some contexts such as
lazygit or zellij." Combined with the gh-dash `BackgroundColorMsg` crashes
(§2.1), the lesson is: **background auto-detection is useful but unreliable, may
arrive late or never, and must have a manual override and a safe default.**

I attempted to retrieve delta's full `STYLES` guidance on 16 vs 256 vs truecolor
but the hosted page excerpt did not contain it; I am not reporting details I
could not read.

### 2.6 General TUI design guidance

["The Terminal Renaissance"](https://hyperbliss.tech/blog/2026.04.04_terminal-renaissance/)
is the most substantive general source found. Key positions:

- **"Usable at 16 colors, beautiful at true color."** Capability tiers: 16 ANSI
  = foundation (theme-controlled, automatic coherence); 256 = emphasis, use
  sparingly; truecolor = optional enhancement layer.
- **"If you stripped all color from your app and it became unusable, your design
  is broken."** Color reinforces a hierarchy established by layout, weight and
  symbols — it never carries the sole information load.
- **Semantic tokens over scattered hex**: `text.primary` / `text.muted`,
  `status.success` / `.warning` / `.error`, resolving to palette entries. Enables
  theme switching without touching layout code.
- **"Every cell on screen earns its place."** Information density is a feature.
- **Status symbols carry meaning**: `●` online, `○` offline, `◐` transitioning.
- **Anti-pattern — layout instability**: "every time you shuffle the layout, you
  reset their spatial model to zero." Panels should stay put so users build
  location-based muscle memory.
- **Test by degradation**: verify monochrome first (structural clarity), then 16
  colors (hierarchy), then truecolor (polish).
- Layout archetype matching us: **"Header + Scrollable List"** — fixed header,
  scrollable content.

**Color accessibility.** Red/green is the most common semantic pairing and the
most common colorblindness affects exactly that pair, ~1 in 12 men. Guidance
found consistently: use multiple channels — distinct *shapes* (check, X, warning
triangle each have distinct silhouettes), text labels, and position — with color
as reinforcement rather than sole carrier. A concrete failure described: mapping
several states onto the same `•` glyph differing only in color collapses them all
into "a filled dot" for colorblind users.

---

## 3. Patterns worth stealing

**P1 — One flexible column, everything else fixed-width.**
*What:* Status fields get small fixed cells (gh-dash uses width 3); exactly one
column (title) has `grow: true` and absorbs slack.
*Who:* gh-dash.
*Why it works:* Status indicators stay in a constant screen position across all
terminal widths, so the eye learns one location. It also mechanically prevents
the "columns spread across the terminal" problem — slack has only one place to go.
*Fits us:* **Yes, strongly.** This is the direct fix for our stated complaint.

**P2 — Pack the status cluster adjacently, put the flexible field at the end.**
*What:* gh-dash's `numComments`/`reviewStatus`/`ci` sit next to each other as
three 3-wide cells.
*Why it works:* Related signals are read as one glance-unit instead of three
separate saccades. Long travel from number to title is eliminated when the
variable-width field is terminal, not medial.
*Fits us:* **Yes.** Our number → CI → review → title ordering already has the
right shape; the gap is spacing discipline, not field order.

**P3 — Full-row background for selection.**
*What:* `selectedLineBgColor: [blue]` fills the row.
*Who:* lazygit (default), gh-dash (`background.selected`).
*Why it works:* A filled row is preattentive — detected without search. A small
glyph requires foveal search. Directly addresses "the cursor is a nearly
invisible small triangle".
*Fits us:* **Yes.** Highest-value single change.

**P4 — `reverse` attribute as a contrast guarantee.**
*What:* lazygit documents `reverse` as "useful for high-contrast".
*Why it works:* It inverts whatever fg/bg the terminal actually has, so contrast
is guaranteed by construction on *any* theme without knowing the theme.
*Fits us:* **Yes, as a fallback/safety option** — a designer's escape hatch when a
chosen background can't be guaranteed to contrast everywhere.

**P5 — ANSI 0-15 for semantics, `default` for surfaces.**
*What:* Status colors reference ANSI indices; backgrounds use the terminal's own.
*Who:* lazygit defaults, k9s `transparent.yaml`, gh-dash post-#771.
*Why it works:* The terminal theme is a user-maintained indirection layer; using
it makes us correctly themed on every scheme the user already owns, including
ones that don't exist yet. Hex for the same nominal color does *not* get this
(k9s #1234).
*Fits us:* **Yes — this is the central theming decision.**

**P6 — Separate "focused/active" from "selected".**
*What:* lazygit uses border color for panel focus and background for row
selection; unfocused panels degrade selection to bold.
*Why it works:* Two orthogonal states get two orthogonal channels, so neither is
ambiguous.
*Fits us:* **Partially.** We have no multi-panel focus model. But the underlying
principle — one visual channel per independent state — applies to our
CI/review/draft/conflict overload.

**P7 — Priority ordering so problems surface themselves.**
*What:* Sort/group by actionability, worst first.
*Who:* Requested in k9s #3589 (not yet shipped there).
*Why it works:* Position is the strongest preattentive cue and the only one
immune to theme, colorblindness, and font issues. "What do I need to do right
now" becomes "look at the top" — a zero-search operation.
*Fits us:* **Yes, strongly.** This is likely the most impactful non-color change
available, and it is the thing our current design most lacks.

**P8 — Explicit responsive breakpoints with mode changes.**
*What:* gh-dash's `position: auto` flips preview right→bottom below 80 effective
columns.
*Why it works:* A discrete mode switch keeps each mode well-designed, versus
every element degrading simultaneously into mush.
*Fits us:* **Yes.** Define named width tiers and what each drops.

**P9 — Semantic color tokens, not literals at call sites.**
*What:* `status.error` / `text.muted` resolving to a palette.
*Who:* Terminal Renaissance; matches gh-dash's `Theme` struct.
*Why it works:* Enables the degradation testing in P10 and makes theming a
one-file change.
*Fits us:* **Yes** — a structural requirement, cheap now and expensive later.

**P10 — Degradation testing as a design gate.**
*What:* Check monochrome → 16 colors → truecolor, in that order.
*Why it works:* Proves color is reinforcing rather than carrying information.
*Fits us:* **Yes**, as an acceptance check.

**P11 — Distinct glyph silhouettes per status, not one dot in N colors.**
*What:* `✓` / `✗` / `◐` / `○` — shapes distinguishable without color.
*Why it works:* Survives colorblindness, monochrome, and unlucky themes.
*Fits us:* **Yes.** Note the Nerd Font caveat in §4.5.

---

## 4. Anti-patterns (with evidence)

### 4.1 Hardcoding hex colors
Breaks on every theme but the author's, and breaks *hardest* for users who switch
light/dark by time of day. Evidence: gh-dash #770 ("I end up having to hardcode
hex values that only match one of my two themes"), k9s #1234, k9s #294 regression.
Note specifically that hex values of the 16 ANSI colors are **not** a substitute —
they stop following the terminal (k9s #1234).

### 4.2 Assuming a dark background
gh-dash originally shipped separate light/dark hex sets and still needed adaptive
pairs afterward. Any design validated only on the author's dark theme is unproven.

### 4.3 Encoding status in color alone
Red/green is both the most common status pairing and the most common
colorblindness. Multiple sources converge: pair color with shape and/or text.
Failure mode named explicitly in the accessibility sources — several states all
rendered as `•` differ only by hue and collapse into one symbol.

### 4.4 Spending vertical rows on chrome
gh-dash #671: a 2-line logo drew "wasting space", "biggest configuration
annoyance right now", "attention hogging. can be very distracting at times." In a
20-row quick-terminal pane, a 2-row banner is ~10% of the viewport.

### 4.5 Assuming Nerd Font glyph availability
gh-dash #290: icons vanished on **Nerd Font 3.0.2** — glyphs were removed/moved
between font versions; fixed only by running `nerdfix` and re-picking codepoints
(PR #291). Also #301, #220, #21 (Windows/cmder). Having "a Nerd Font" does not
guarantee a *specific glyph* renders. Prefer widely-supported codepoints, and
don't let a missing glyph destroy column alignment.

### 4.6 Letting content styling escape its cell
gh-dash #461: ANSI colors from preview content bled into the list on the next
line. PR titles are untrusted input and can contain ANSI or wide/emoji characters.

### 4.7 Unguarded width arithmetic
gh-dash shipped a fix for a `strings: negative Repeat count` panic when the
sidebar was narrow; #530 documents content-triggered layout collapse. Padding
math must clamp at zero.

### 4.8 Trusting background auto-detection
delta documents OSC-query race conditions with pagers and the need for manual
override under lazygit/zellij; gh-dash crashed twice (#913, #876) when
`BackgroundColorMsg` hadn't arrived. Detection must be optional, late-safe, and
overridable.

### 4.9 Low-contrast / weight-only selection
lazygit #1845 (selection reads as "greyed out", called unintuitive) and #802
("Some highlighted lines are still very hard to see"). Bold-only or dim-only
selection is not reliably visible.

### 4.10 Layout instability
"Every time you shuffle the layout, you reset their spatial model to zero."
Fields must not move between refreshes or change position based on content.

---

## 5. Requirements for the redesign

Outcome-focused and prioritized. **P0 = must ship**, P1 = should, P2 = nice.

### P0 — Core glance requirements

**R1. The selected row must be unmistakable at a glance from two feet away**,
without the user hunting for a marker, on both light and dark terminal themes.
It must remain identifiable when the row also carries error/warning coloring.
*(Evidence: lazygit #1845, #802; current "invisible triangle" complaint.)*

**R2. The board must read correctly under any terminal color scheme the user
already has, including light backgrounds, without per-user configuration.**
Semantic colors must resolve through the terminal's own palette rather than
fixed RGB values. *(gh-dash #770/#771, k9s #294/#1234.)*

**R3. Every status distinction must survive removal of all color.** With color
stripped, a user must still be able to tell passing from failing from running,
approved from changes-requested, draft from ready, conflicted from clean.
*(Terminal Renaissance; colorblindness sources.)*

**R4. "What do I need to act on right now" must be answerable from position
alone, without reading any row in detail.** The layout must bring actionable
items to where the eye lands first rather than relying on the user to scan for
color. *(k9s #3589 — the core unsolved problem in comparable tools.)*

**R5. The status signals for one PR must be readable as a single visual unit.**
The horizontal distance the eye travels from a PR's identity to its state to its
title must be short and — critically — **constant across terminal widths**.
*(gh-dash fixed-width status columns + single growing column.)*

**R6. Must be fully usable in a 20-row pane.** Non-data chrome must be
justified row-by-row against the cost of one fewer visible PR. In the shortest
supported pane, the majority of rows must be PR data. *(gh-dash #671.)*

**R7. Must degrade gracefully across terminal widths with no truncation of
identity or corrupted alignment.** Below defined width thresholds the design must
drop or collapse information in a deliberate, specified order — never overflow,
wrap unexpectedly, or misalign. Width math must never panic or produce negative
padding. *(gh-dash narrow-sidebar panic, #530.)*

**R8. Section headers must be distinguishable from PR rows pre-attentively** —
the user must never have to read a line to know whether it is a header or a PR.
Each header must carry its count.

### P1 — Hierarchy and robustness

**R9. There must be at least three clearly distinct levels of visual weight:**
section headers, PR rows, and secondary detail (failing check names on
continuation lines). Secondary detail must never compete with actionable rows.

**R10. Rows needing the user's action must outrank rows that are merely
informational**, within a section as well as across sections. Weight must
correlate with actionability, not with field count.

**R11. Nothing may depend on a specific Nerd Font glyph rendering.** A missing or
substituted glyph must degrade to a still-readable row with intact alignment,
never to a blank cell or a shifted column. Prefer widely-supported codepoints.
*(gh-dash #290/#301/#220.)*

**R12. PR titles must be treated as untrusted input.** Embedded ANSI sequences,
emoji, CJK/wide characters and control characters must not bleed styling, break
alignment, or corrupt adjacent rows. Width must be computed by display width, not
byte or rune count. *(gh-dash #461; the "vis_len counts chars not display width"
class of bug.)*

**R13. Field positions must be stable across refreshes and content changes.** A
PR gaining a conflict marker must not shift other fields' positions.
*(Terminal Renaissance layout-stability anti-pattern.)*

**R14. Colors must be defined once as semantic tokens** (e.g. "error",
"attention", "muted") and referenced symbolically at render sites, so the entire
palette can be swapped or re-tiered in one place. *(Terminal Renaissance;
gh-dash `Theme` struct.)*

**R15. If terminal background detection is used, it must be optional, safe when
the answer arrives late or never, and manually overridable.** The first paint
must be correct-enough before detection resolves. *(delta OSC caveats; gh-dash
#913/#876.)*

**R16. The design must be validated at 16-color, 256-color and truecolor
capability, and in monochrome.** The 16-color rendering is the correctness
baseline; richer profiles may only add polish. *(Terminal Renaissance.)*

### P2 — Polish

**R17. Total distinct colors on screen should be small enough that each carries
a learnable meaning.** Every additional hue costs the user a lookup. Avoid
per-repo / per-author color variety that competes with status color.

**R18. Density should be tunable or at least deliberately chosen** — the
tradeoff between separators/padding and rows-visible should be an explicit
decision, given the short-pane default. *(gh-dash exposes `compact` and
`showSeparators`; note #711 records compact mode breaking, so if we add a mode we
must test both.)*

**R19. The stacked-PR tree glyph must remain legible as structure** and must not
be confused with status iconography or with the selection indicator.

---

## 6. Open questions for the designer

1. **Selection background vs `reverse`.** A chosen background color (lazygit's
   `[blue]`) is predictable but can collide with a user's theme or with row
   status coloring; `reverse` guarantees contrast anywhere but surrenders control
   of the color and may fight status colors on the same row. Which, and does the
   answer change for a row that is *both* selected and failing?

2. **Priority ordering vs section stability.** R4 wants problems surfaced to the
   top; R13 wants positional stability. Sorting by actionability means rows move
   as CI completes. Does actionability ordering apply *within* existing sections,
   or should there be an "needs you now" section above everything? I did not find
   a tool that has solved this — k9s #3589 is still open.

3. **How many status columns survive at narrow widths, and in what order do they
   go?** Drop order should follow decision value, which I could not determine
   from research alone — it depends on this user's actual triage habits.

4. **Is the failing-check continuation line worth its rows in a 20-row pane?**
   It is the most detailed information on the board and the most vertically
   expensive. Always, only when selected, or only when the section is short?

5. **Semantic ANSI colors vs 256-palette greys.** gh-dash's shipped answer is a
   hybrid: 0-15 for status semantics, 256 indices for structural greys. The
   hybrid risks a grey that vanishes on some themes. Is muted text expressible
   safely in ANSI 8/7 alone, or is a 256 grey worth the theme risk?

6. **Does the board need a persistent header at all?** R6 says every chrome row
   is expensive. Is section-header-only viable, with no global header?

7. **Nerd Font glyphs vs plain Unicode for status.** Nerd Font glyphs are richer
   but carry the #290 version-skew risk and inconsistent advance widths; plain
   Unicode (`✓ ✗ ● ○ ◐`) is far more portable. Which set, and is a no-glyph
   fallback mode worth building?

8. **Draft and conflict markers are booleans competing for space with two
   multi-state fields (CI, review).** Should booleans get their own cells, fold
   into a single "blockers" cell, or modify the row's overall treatment? No
   researched tool handled four independent state dimensions per row well.

---

## 7. Sources

- gh-dash docs: [layout/pr](https://www.gh-dash.dev/configuration/layout/pr/),
  [layout/options](https://www.gh-dash.dev/configuration/layout/options/),
  [theme](https://www.gh-dash.dev/configuration/theme/),
  [defaults](https://www.gh-dash.dev/configuration/defaults/)
- gh-dash issues/PRs: [#770](https://github.com/dlvhdr/gh-dash/issues/770),
  [#771](https://github.com/dlvhdr/gh-dash/pull/771),
  [#671](https://github.com/dlvhdr/gh-dash/issues/671),
  [#530](https://github.com/dlvhdr/gh-dash/issues/530),
  [#461](https://github.com/dlvhdr/gh-dash/issues/461),
  [#290](https://github.com/dlvhdr/gh-dash/issues/290),
  [#913](https://github.com/dlvhdr/gh-dash/issues/913),
  [#876](https://github.com/dlvhdr/gh-dash/issues/876),
  [#937](https://github.com/dlvhdr/gh-dash/issues/937),
  [#920](https://github.com/dlvhdr/gh-dash/issues/920),
  [#711](https://github.com/dlvhdr/gh-dash/issues/711)
- gh-dash source: `internal/tui/theme/theme.go` (fetched via GitHub API, 2026-09-16)
- lazygit: [Config.md](https://raw.githubusercontent.com/jesseduffield/lazygit/master/docs/Config.md),
  [#1845](https://github.com/jesseduffield/lazygit/issues/1845),
  [#802](https://github.com/jesseduffield/lazygit/issues/802)
- k9s: [skins docs](https://k9scli.io/topics/skins/),
  [transparent.yaml](https://github.com/derailed/k9s/blob/master/skins/transparent.yaml),
  [#294](https://github.com/derailed/k9s/issues/294),
  [#1234](https://github.com/derailed/k9s/issues/1234),
  [#3589](https://github.com/derailed/k9s/issues/3589)
- [btop](https://github.com/aristocratos/btop)
- delta: [choosing colors/styles](https://dandavison.github.io/delta/choosing-colors-styles.html)
- [lipgloss](https://pkg.go.dev/github.com/charmbracelet/lipgloss/v2) (AdaptiveColor,
  CompleteColor, ANSIColor, profile downsampling)
- ["The Terminal Renaissance"](https://hyperbliss.tech/blog/2026.04.04_terminal-renaissance/)
- Color accessibility: [Bloomberg UX](https://www.bloomberg.com/ux/2021/10/14/designing-the-terminal-for-color-accessibility/)
  (title//premise only — page returned 403, not read),
  [accessibility.chat](https://www.accessibility.chat/articles/when-color-coding-fails-why-status-indicators-need-more-than-pretty-colors)

### Verification gaps

- **lazygit #1845 resolution unknown** — only the opening report was retrievable;
  the maintainer decision is not reported here.
- **Bloomberg terminal accessibility article returned HTTP 403** and was not
  read. Colorblindness claims in §4.3 rest on the other accessibility sources.
- **delta's full `STYLES` guidance** on 16/256/truecolor was not in the fetched
  page; only the auto-detection and override behavior is reported.
- **No substantive Reddit/HN discussion of gh-dash was found** despite searching.
  The user-complaint evidence here is from issue trackers, which skews toward
  bugs over abandonment reasons. I did not find direct evidence of *why people
  abandon* PR dashboard TUIs.
