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

---

## 8. Round two — author display and column headers

Researched 2026-09-16, after the spec in `docs/design-spec.md` was written. This
section answers two questions the spec left open: how to show and filter by PR
author without making the board busy (Q1), and whether the glyph columns need a
header or legend (Q2). Sections 0–7 are unchanged and still binding.

Method note: as in round one, issue trackers and **read source** were far more
productive than web search. Where a claim below comes from code, the file is
named so it can be re-checked. Where I could not verify something, §8.6 says so.

### 8.1 Findings by tool

#### gh-dash — the author column is not what the docs imply

§2.1 recorded a 15-cell `author` column from the layout docs. Reading the source
shows the docs are describing the **non-default** layout.

`internal/tui/components/prrow/prrow.go`, function `ToTableRow`, branches on
`Theme.Ui.Table.Compact`:

- **`compact: true`** (the legacy mode) emits a flat row that includes
  `renderAuthor()` — the 15-cell column the docs document.
- **`compact: false`** — the **default since v4.5.0**, called *sparse layout* —
  emits `renderExtendedTitle()` instead, and there is **no author column at
  all**. Author is folded into a dim secondary line *above* the title:

```
owner/repo #3248 by @someone · feat/branch-name      <- SecondaryText, 1 line
feat(api-service): PROJ-2037 refuse order plan …      <- PrimaryText, bold
```

Verified in `renderExtendedTitle`: the top line is rendered with
`Foreground(Theme.SecondaryText)` and the title below with
`Foreground(Theme.PrimaryText).Bold(true)`. The v4.5.0 release notes
(<https://github.com/dlvhdr/gh-dash/releases/tag/v4.5.0>) confirm: *"Support for
sparse layout — **This is the new default!**"*, with `compact: true` given as
the opt-out.

So the closest analogue's answer to "where does author go" is: **not in a
column.** It costs a second row per PR instead of 15 horizontal cells, and it is
demoted to secondary color. For us that trade is unavailable — a second line per
PR in a 20-row pane halves the board (R6) — but the *demotion* half of the idea
transfers.

**Complaint evidence on the author column.**
[#464 "Customize title column"](https://github.com/dlvhdr/gh-dash/issues/464) is
the one directly on point:

> "I have a section that shows only content from one repo (with a long name) and
> I want to save space by not showing the repo name in that section. Similarly in
> the **'My PRs' section I don't want to show the author**."

That is verbatim Q1 option 3 (per-section column config), requested by a user, and
it is **still open**. The same reporter then discovered the sparse layout hides
columns and added: *"I think it's quite confusing that this makes some of the
columns always hidden, it should be at least clearly mentioned in the
documentation."* — i.e. the silent-hiding approach has its own discoverability
cost, which is directly relevant to Q1 option 5.

I searched the gh-dash tracker for complaints that the author column is too wide
or too noisy (`author column`, `hide author`, `width`, `avatar`) and found
**none** other than #464. Read that as weak evidence: nobody is angry about 15
cells, but nobody is defending them either, and the maintainer moved the default
away from that layout anyway.

**Author role icons — an adjacent shipped feature.**
[#515](https://github.com/dlvhdr/gh-dash/issues/515) asked for a first-time
contributor / author-role indicator and **shipped**. `internal/data/utils.go`
has `GetAuthorRoleIcon(role, theme)` mapping `AuthorAssociation` to a
**one-cell colored Nerd Font glyph** appended after the name
(`internal/data/prapi.go`, `GetAuthor`). Two details matter for us:

1. It is **opt-in** — gated on `cfg.ShowAuthorIcons`, default off
   (`internal/tui/theme/theme.go`).
2. Its colors (`internal/tui/theme/theme.go`) are `ANSIColor(77)`,
   `ANSIColor(75)`, `ANSIColor(178)` — **256-palette, not 0–15**, and four of the
   six roles share index 178. That is the k9s #1234 failure mode (§4.1) shipped
   in the very tool that fixed it elsewhere, and it is a concrete warning about
   what happens when you try to put a *categorical identity* dimension into a
   color system that already spent 0–15 on status.

**Header row — gh-dash does have one, and it does not explain anything.**
`internal/tui/components/table/table.go`: `View()` = `renderHeader()` +
`renderBody()`, unconditionally. `internal/tui/common/styles.go` sets
`TableHeaderHeight = 2`. So gh-dash pays **two rows, every frame, forever** — 10%
of a 20-row pane.

What is *in* those cells is the important part. From
`internal/tui/components/prssection/prssection.go`, the `Title` of each column:

| column | width | header cell |
|---|---|---|
| state | 3 | `` (Nerd Font glyph) |
| Title | grow | `"Title"` |
| Author | 15 | `"Author"` |
| Base | n | `"Base"` |
| labels | 22 | `󰌖` |
| numComments | 4 | `` |
| reviewStatus | 4 | `󰯢` |
| ci | n | `` |
| updatedAt | n | `󱦻` |

**Every narrow glyph column's header is itself another glyph.** Only the wide
text columns get words. `renderHeaderColumns` even carries the comment
*"Center short headers (icons), right-align longer headers (text)"*. So gh-dash's
header row does not label the status columns — it adds a *second* glyph the user
must also learn, at a cost of two permanent rows. That is the strongest single
piece of evidence against a header row for our cluster.

Related: [#520](https://github.com/dlvhdr/gh-dash/issues/520) — a footer glyph at
U+F5D1 rendered as a **boxed-question-mark replacement character** because the
codepoint was removed from Nerd Fonts. A header made of Nerd Font glyphs inherits
the §4.5 version-skew risk on top of everything else.

**The legend request.** [#44 "Add labels explaining UI
legends"](https://github.com/dlvhdr/gh-dash/issues/44), opened **2021-12-26**:
*"the UI shows various kinds of legends for the PRs. There should be a key to
explain what each symbol means."* It has **zero comments**, no maintainer
response, and is **still open ~5 years later**. One request, no upvoting
discussion, in a project with 900+ issues.

#### lazygit — initials by default, hashed color, and filter-by-row

The most directly useful tool for Q1. Verified in
`pkg/gui/presentation/authors/authors.go`:

```go
// AuthorWithLength returns a representation of the author that fits into a
// given maximum length:
// - if the length is less than 2, it returns an empty string
// - if the length is 2, it returns the initials
// - otherwise, it returns the author name truncated to the maximum length
```

`getInitials` takes the first letter of the first two space-separated tokens
(`"Paul Oberstein"` → `PO`), falls back to the first 2 chars for a single-token
name, and returns the first grapheme cluster whole if it is wide (CJK/emoji-safe).

**The defaults are the finding.** `pkg/config/user_config.go`:

```go
CommitAuthorShortLength: 2,    // non-expanded commits view  -> INITIALS
CommitAuthorLongLength:  17,   // expanded commits view      -> full name
```

and the field docs read *"Length of author name in (non-expanded) commits view.
**2 means show initials only.**"* `pkg/gui/presentation/commits.go` picks between
them on `fullDescription`.

So lazygit's shipped answer is exactly the two-tier shape Q1 is circling:
**2 cells of initials in the dense list, 17 cells of name in the detail view.**
This is the single strongest precedent found for either question.

**Per-author color — verified, and it is truecolor, not ANSI.** `AuthorStyle`
MD5-hashes the name and derives an HSL color:

```go
c := colorful.Hsl(randFloat(hash[0:4])*360.0,
                  0.6+0.4*randFloat(hash[4:8]),
                  0.4+randFloat(hash[8:12])*0.2)
return style.New().SetFg(style.NewRGBColor(...))
```

Note the constraints baked into that expression: hue is unconstrained (full 360°),
but **saturation is clamped to 0.6–1.0 and lightness to 0.4–0.6** — a deliberately
narrow mid-luminance band so the color is legible on light and dark alike. That
is only expressible in truecolor. **There is no 16-color version of this
technique**, and lazygit does not attempt one.

lazygit also ships two escape hatches (`docs/Config.md` §"Custom Author Color"):
per-author overrides, and a **`'*'` wildcard** described as *"in case you are
lazy to customize the color for every author or you just want a single color for
all/other authors"*. `AuthorStyle` checks the wildcard cache **before** hashing.
A maintainer shipping a "make them all one color" switch is a signal that the
per-author rainbow is not universally wanted.

I searched the lazygit tracker for complaints about author colors
(`author color`, `random color`, `author initials`) and found **none**. So: no
evidence users hate it, but also no evidence they need it.

**Filter by author — and how it is discovered.** [#2351 "Filter commits by
author"](https://github.com/jesseduffield/lazygit/issues/2351) closed as a dup of
#914; shipped. The integration test
`pkg/integration/tests/filter_by_author/select_author.go` shows the actual
interaction, and it is not what I expected:

```go
t.Views().Commits().Focus().SelectedLineIdx(0).Press(keys.Universal.FilteringMenu)
t.ExpectPopup().Menu().Title(Equals("Filtering")).
    Select(Contains("Filter by 'Paul Oberstein <paul.oberstein@email.com>'")).Confirm()
...
t.Views().Information().Content(Contains("Filtering by 'Paul Oberstein <…>'"))
```

Three properties worth stealing:

1. **The filter is seeded from the row under the cursor.** The user never types a
   username and never needs to know one. "Show me more like this person" is a
   two-keystroke operation on a row they are already looking at.
2. **The menu entry spells out the full name and email** — so the identity is
   fully legible at the moment of choosing, even though the list only shows 2
   initials.
3. **An active filter is persistently displayed** ("Filtering by '…'") in the
   information view, so filtered state is never invisible.

There is also a sibling test `type_author.go`, i.e. typing a name is supported
too — but selecting from the cursor is the primary, tested path.

#### tig — initials are a first-class display mode, and author is toggleable

`doc/tigrc.5.adoc`, the `author, committer` column type:

> - `display` (mixed) [full|abbreviated|email|email-user|<bool>]: How to display
>   author/committer names. **If set to "abbreviated" author/committer initials
>   will be shown.**
> - `width` (int): Fixed width for the column. **When set to a value between 1 and
>   10, the author/committer name will be abbreviated to the author/committer's
>   initials.** When set to zero, the width is automatically sized to fit the
>   content.
> - `maxwidth` (int): Maximum width of the column. … Can be specified either as
>   the number of columns, e.g. '15', **or as a percentage of the view width,
>   e.g. '20%'**.

Two independent mechanisms in one tool: an explicit `abbreviated` mode, *and* an
automatic collapse to initials whenever the column is ≤10 cells. A second
implementation of lazygit's rule, arrived at separately.

`tigrc` also binds:

```
bind generic	A	:toggle author		# Toggle author display
```

alongside `D` for date and `F` for file name. **Author is one of the fields tig
considers worth a dedicated toggle key** — direct support for Q1 option 4. Note
tig's default `main-view` is `author:full`, so it shows author by default and lets
you hide it, which is the opposite polarity from what I will recommend.

tig has **no column header row** in any view.

#### gitui — a responsive author width, and one flat color

`src/components/commitlist.rs`:

```rust
let author_width = (width.saturating_sub(19) / 3).clamp(3, 20);
let author = string_width_align(&e.author, author_width);
```

The author column is a **third of the leftover width, clamped to [3, 20]** — it
never gets a fixed budget, and its floor is 3 cells, not 15.

`src/ui/style.rs` sets `commit_author: Color::Green` — a **single flat color for
every author**. gitui deliberately does not do lazygit's per-author hash, and its
whole default theme is named ratatui colors (`Color::Green`, `Color::Blue`, …)
i.e. ANSI 0–15. That is the 16-color-compatible answer to "how do you style an
author field": you don't vary it per person.

gitui has no column header row and no legend; searching its tracker for `legend`
returns nothing.

#### neomutt / mutt — the 30-year-old answer, and it is option 3

`docs/config.c`, the shipped default:

```
index_format = "%4C %Z %{%b %d} %-15.15L (%<l?%4l&%4c>) %s"
```

Reading it left to right: 4-cell index number, **`%Z` = a three-character
unlabeled status flag cluster**, date, **`%-15.15L` = the person field,
left-aligned, fixed 15 cells, truncated at 15**, size, subject (flex, last).

That is structurally our row: fixed identity, fixed unlabeled glyph cluster,
fixed-width person, flexing text last. It has been approximately this shape since
the early 1990s.

The person field is where the real finding is. From the same file:

| fmt | expansion | documentation |
|---|---|---|
| `%F` | `{sender}` | **"Author name, or recipient name if the message is from you"** |
| `%n` | `{name}` | "Author's real name (or address if missing)" |
| `%I` | `{initials}` | **"Initials of author"** |
| `%L` | `{from-list}` | mailing-list name if the message went to a subscribed list |

`%F` is **Q1 option 3, shipped as a format primitive**: the person column shows
"the other party", automatically swapping to recipient in folders where the
author is always you. Mail clients solved "the author column is dead weight in my
Sent folder" by making the field mean *the interesting party* rather than
*the author*. That is precisely the argument for dropping author in the Mine
section.

`%I` is **Q1 option 1 as a first-class format field** — a third independent
implementation of initials (lazygit, tig, neomutt).

neomutt's index has **no column header row**. `%Z`'s three flag characters are
permanently unlabeled.

#### aerc — percentage-width person column, no header, ASCII status glyphs

`config/aerc.conf`:

```
index-columns=flags:4,name<20%,subject,date>=
column-name={{index (.From | names) 0}}
column-flags={{.Flags | join ""}}
column-separator="  "
```

- `flags:4` — a **4-cell unlabeled glyph cluster**, first column, exactly our
  status cluster's role.
- `name<20%` — the person column is **20% of terminal width**, not a fixed cell
  count. At 100 columns that is 20 cells; at 60 it is 12. A third responsive
  approach, alongside tig's `maxwidth` percentages and gitui's `/3` clamp.
- `column-name={{index (.From | names) 0}}` — renders the **first token of the
  display name** (first name), not the email, not the full name. Another form of
  shortening-by-convention.
- aerc has **no column header row**; I grepped the whole default config and every
  `header` hit is about *message* headers (From/To/Cc), not a table header.
- aerc's PGP status indicators are plain ASCII in brackets — `icon-encrypted=[e]`,
  `icon-signed=[s]`, `icon-unknown=[s?]`, `icon-invalid=[s!]` — unlabeled, and
  self-describing by initial letter rather than by legend.

#### weechat — the only ANSI-16 per-identity color scheme found

`src/core/core-config.c`, `weechat.color.chat_nick_colors` default:

```
cyan,magenta,green,brown,lightblue,lightcyan,lightmagenta,lightgreen,
31,35,38,40,49,63,70,80,92,99,112,126,130,138,142,148,160,162,167,169,
174,176,178,184,186,210,212,215,248
```

The first **8 entries are named ANSI colors**, then it extends into the 256
palette. Assignment is by hash (`weechat.look.nick_color_hash`, default `djb2`,
with a `nick_color_hash_salt` to reshuffle). Three details matter:

1. The ANSI-16 head of that list **deliberately omits red and the blacks/whites**
   — `red`, `lightred`, `black`, `white`, `default`, `yellow`, `blue` are all
   absent from the named portion. Red is reserved (weechat uses it for errors and
   highlights), exactly as our §5 reserves 1 for `error`.
2. **`weechat.color.chat_nick_self` is a separate option, default `white`** — the
   user's own nick is deliberately *not* in the hashed rotation. Self is a
   different category from other people.
3. `weechat.look.nick_color_force` lets a user pin specific nicks, same escape
   hatch as lazygit's `authorColors`.

So the one tool that does per-identity color at 16 colors gets there by having
only ~8 usable slots left after reserving status colors — and its context is a
chat log where nick color is the *primary* categorical signal and there is no
competing CI/review/conflict encoding. That reservation pressure is much worse in
our row.

#### k9s — headers are always on, but every *other* chrome row is removable

Q2-relevant. `internal/config/flags.go` ships **four** separate chrome-suppression
flags:

```go
Headless   *bool   // the info panel
Logoless   *bool   // the ASCII logo
Crumbsless *bool   // the breadcrumb row
Splashless *bool   // the splash screen
```

plus runtime toggles `Ctrl+E` (header) and `Ctrl+G` (crumbs)
(`internal/view/app.go`). The history behind them:

- [#270 "Full Screen mode / Hide headers"](https://github.com/derailed/k9s/issues/270):
  *"When I split my terminal horizontally, I would like to hide the headers …
  because I have not so much space."* Maintainer shipped `--headless` in 0.8.1.
  The reporter followed up asking for a runtime toggle and noting *"the
  3-empty-lines above table are still visible."*
- [#1978](https://github.com/derailed/k9s/issues/1978) asked again for a toggle
  shortcut; answer: `Ctrl+E`.
- [#3648 "Free up space in the header"](https://github.com/derailed/k9s/issues/3648):
  *"By default the header takes up **7 lines**. I think this can be improved."*
  A second user pushed back — *"I think the current header size works fine as-is…
  If you want maximum screen space, you can run K9s in headless mode"* — which is
  itself the finding: the disagreement was resolved by the row being *optional*,
  not by picking a winner.

This is §4.4 (chrome rows draw complaints) confirmed a third time, in a second
tool, with a maintainer response of "make it removable".

**But the column header row is exempt.** `internal/ui/table.go` builds header
cells unconditionally (`AddHeaderCell` in the render path, no suppression check),
and none of the four `-less` flags touches it. k9s' header earns that exemption
under conditions we do not share: its columns are *word* headers (READY, STATUS,
RESTARTS, AGE), the column **set changes per resource type** (pods vs. nodes vs.
deployments have different columns, so there is nothing stable to memorize), and
the headers are **interactive** — `Shift+←/→` selects a column to sort by
([#3955](https://github.com/derailed/k9s/issues/3955),
[#3768](https://github.com/derailed/k9s/issues/3768) are both about making the
*selected* header visible). A header that is a sort control is not chrome; it is
a widget. Ours would be neither of those things.

Also worth noting from #3955, which argues our §4.3 line independently:

> "Color alone is a fragile affordance. Reverse color is a terminal-native,
> colorblind-safe, high-contrast signal that works on every terminal and every
> skin."

**k9s' `?` help overlay does not contain a legend.** `internal/view/help.go`
builds exactly four sections — `RESOURCE`, `GENERAL`, `NAVIGATION`, `HOTKEYS` —
all of them keybindings, from `MenuHints`. There is no glyph or status key in it.
So "put the legend in the `?` overlay" is **not** a pattern k9s actually ships,
and I should not claim it is.

#### Cross-tracker: how often do users actually ask what the glyphs mean?

I searched gh-dash, lazygit, and gitui for `legend`, `what does`, `symbol mean`,
`icon meaning`. Total substantive hits across all three projects, all time:
**one** — gh-dash #44, 2021, zero comments, still open.

For scale, the same trackers produced multiple, repeatedly-upvoted complaints
about *chrome taking vertical space* (gh-dash #671, k9s #270 / #1978 / #3648).
The asymmetry is the evidence: **users complain about permanent rows far more
than they complain about unlabeled glyphs.**

#### General UX literature (and its limits)

[NN/g, "Icon Usability"](https://www.nngroup.com/articles/icon-usability/) is the
canonical anti-unlabeled-icon source and says, verbatim: *"A text label must be
present alongside an icon to clarify its meaning in that particular context"* and
*"Icon labels should be visible at all times, without any interaction from the
user."* It also explicitly rejects hover-to-reveal: *"Don't rely on hover to
reveal text labels: not only does it increase the interaction cost, but it also
fails to translate well on touch devices."*

**I am going to argue against applying this here, and I want the reason on the
record rather than buried.** The article's subject is *icons as affordances* —
clickable controls whose icon must communicate an available action to someone who
has never seen the app, in a context where the cost of a wrong guess is an
unintended action. Our glyphs are *state readouts*: they are not clickable, a
misread costs one glance and no action, and the audience is one person opening
the same board many times a day. When I asked the article directly, it **does not
address** whether repeated exposure inside a single consistent app teaches an
icon — its learnability argument is entirely about *cross-application*
inconsistency (*"users cannot rely on it having the same functionality every time
it is encountered"*), which by construction does not apply to a single-app,
single-user, self-consistent glyph set.

The applicable literature is the learnability/efficiency tradeoff.
[NN/g, "How to Measure Learnability"](https://www.nngroup.com/articles/measure-learnability/)
separates three things — first-use ease, curve steepness, and **plateau
efficiency** (*"How high is the productivity that users can reach with this
interface, once they have fully learned how to use it?"*) — and warns
*"a learnable system is not always efficient."* On timing it suggests planning
**5–10 trials** to reach the plateau, with a worked example saturating at
**trial 4**, and flags 30 trials as too slow. Its design guidance is to shape the
curve *"mostly to those users who have the highest business value"*.

For a tool opened by one person several times a day via a quick-terminal keybind,
the plateau is reached within the first day and every subsequent use is at the
plateau. Permanent chrome that only serves trials 1–4 is paid approximately
forever for approximately no return. NN/g's own framing
([Flexibility and Efficiency of Use](https://www.nngroup.com/articles/flexibility-efficiency-heuristic/))
recommends serving both by adding *accelerators* for experts rather than
stripping guidance from novices — but that assumes the two populations coexist.
Here they are the same person, four uses apart.

### 8.2 Q1 — author: options analysed

Scored against the binding constraints: **16-color only**, **not busy**,
**one flex column / constant status offset**, **40-column floor**,
**survives losing color**, **small learnable palette (R17)**.

#### Option 1 — initials / short abbreviation (2–3 cells)

*Evidence for.* Three independent implementations, all defaulting to it in the
dense view: lazygit `CommitAuthorShortLength: 2` with `getInitials`; tig
`display=abbreviated` **and** an automatic collapse at `width ≤ 10`; neomutt
`%I {initials}`. aerc reaches for a related shortening (first name only). No
tool found shows a full author name in its densest list view by default.

*Evidence against.* Collisions. lazygit's `getInitials` on a GitHub *login* (not
a "First Last" display name) hits its single-token branch and returns the first 2
characters — so `barspielberg` → `ba`, and any teammate whose login starts `ba`
collides. None of the three tools appears to do collision detection; I found no
issue complaining about it either, which suggests it is tolerable in practice at
team scale but I cannot prove that. Mitigation available to us: GitHub gives us
both `login` and (via the API) a display name, and a repo-scoped board sees a
small closed set of authors, so initials can be **disambiguated against the
authors actually present** — 2 cells normally, 3 when two authors would collide.
That is a variable-width field, which fights R13, so the safer form is a fixed
3-cell slot with a deterministic 3-char abbreviation.

*Constraint fit.* 16-color: fine, it needs no color at all. Not-busy: **best of
any option that displays something** — 2–3 cells of lowercase text is visually
quieter than any glyph or color. Monochrome: survives completely. Width: 3 cells
is affordable at FULL and MID, not at NARROW/MIN. Palette: costs zero new colors
if rendered `muted`.

#### Option 2 — per-author deterministic color (hash → one of N)

*Evidence for.* lazygit does it and nobody complains. weechat does it and it is a
beloved 20-year-old feature.

*Evidence against, and it is decisive.* **The two tools that do this both need
more than 16 colors to do it.**

- lazygit's `trueColorStyle` emits an **RGB** color from an HSL triple, with
  saturation clamped 0.6–1.0 and lightness 0.4–0.6. That clamping is what keeps
  it legible on both light and dark backgrounds. You cannot clamp the luminance
  of ANSI index 5 — you get whatever the user's theme assigned it. There is no
  16-color port of this algorithm.
- weechat's palette starts with 8 named ANSI colors and then **immediately
  escapes into the 256 palette** for the other 28, precisely because 8 is not
  enough rotation.
- gitui, whose theme is entirely ANSI-16, **chose a single flat color** for the
  author field rather than varying it.
- gh-dash's one attempt at a per-identity color axis (the author-role icons) used
  256-palette indices 75/77/178 — the §4.1 anti-pattern — and gave four of six
  roles the same index anyway.

Now count our budget. §5 already spends 1 (`error`), 3 (`attention`), 2 (`ok`),
6 (`header`), 4 (`accent`), 8 (`selBg`), plus default-fg and Faint. Of 0–15 the
genuinely free, theme-stable, non-conflicting hues are roughly 5 (magenta) and
13 (bright magenta) — and both bright variants are the ones §5.2 already flags as
unreliable on light themes. A two-color author rotation is worse than useless:
it would imply a categorical distinction that does not exist, and half the team
would share a color with the other half.

Worse, it is a **direct R17 violation**. R17 says in so many words: *"Avoid
per-repo / per-author color variety that competes with status color."* §4 of the
spec builds its entire "loud / quiet / silent" scheme on color saturation meaning
*actionability*. Injecting a hue that means *identity* into the same channel
destroys the property the spec calls the point: *"the user can answer 'do I need
to do anything' from the presence or absence of red and amber, without reading a
single row."*

*Verdict:* **rejected on the 16-color constraint alone**, and independently
rejected by R17. This is the clearest no in either question.

#### Option 3 — show author only where it is not you

*Evidence for.* Two independent implementations, one of them ~30 years old:

- **neomutt `%F`** — *"Author name, **or recipient name if the message is from
  you**"* — the field's meaning is context-dependent by design, and this is in
  the **default** `index_format`.
- **gh-dash #464**, open, user-requested: *"in the 'My PRs' section I don't want
  to show the author."*

*Evidence against.* Per-section column configuration is not shipped by gh-dash
(#464 is open, not closed). And the reporter's follow-up on that same issue is a
warning: when gh-dash silently hid columns via the sparse layout, the user found
it *"quite confusing"*. Hiding a column in some sections and not others makes the
title column start at **different offsets in different sections**, which is an
R13 / §6.3 violation — the spec's stated stability guarantee is *"columns 0–10
never move at all… the left edge of the board is completely stable."*

*Constraint fit.* 16-color: free. Not-busy: excellent in the Mine section
(nothing at all), unchanged elsewhere. Stability: **this is the problem** — a
column that exists in some sections and not others breaks the fixed-offset model
the whole spec is built on. Resolvable only by keeping the cells reserved and
rendering them blank in Mine, which recovers stability but throws away the space
saving, leaving a 3-cell hole in the section the user looks at most.

#### Option 4 — author on demand (keypress / selected row only / detail pane)

*Evidence for.* **tig binds `A` to `:toggle author`**, alongside `D` for date and
`F` for filename — author is in the small set of fields tig thinks deserves a
toggle key. lazygit's two-tier `CommitAuthorShortLength: 2` /
`CommitAuthorLongLength: 17` is the same idea on a different axis: initials in the
list, full name in the expanded view. gh-dash's sparse layout is a third variant
— author is present but demoted to a secondary line.

*Evidence against.* A toggle is a mode, and modes are invisible state; tig's
polarity is show-by-default-and-hide, which suggests the toggle is a
decluttering affordance rather than a discovery one. "Only on the selected row"
is attractive but changes a row's content as the cursor passes over it, which is
the §4.10 layout-instability anti-pattern in miniature — and with a fixed column
it would mean text appearing and disappearing in the same cells as you scroll.

*Constraint fit.* Zero cost when off, which is the appeal. But the README shows
we have no detail pane yet (*"Not yet: review progress, detail pane"*), so the
lazygit two-tier form has nowhere to put the long name today.

#### Option 5 — filter-only (never displayed, matched by the fuzzy filter)

*Evidence for.* Cheapest possible option: zero cells, zero colors, zero rows. The
data is already fetched — `internal/github/github.go:178` sets `Author` from
`n.Author.Login`, and `Author` is already exposed to action templates.

*Evidence against, and it is specific to our implementation.* Two concrete
problems, both visible in `internal/ui/filter.go`:

1. **The highlighter would lie.** `haystack()` is
   `fmt.Sprintf("#%d %s", r.PR.Number, r.PR.Title)` and `titleOffset()` /
   `matchedTitleIndexes()` translate match positions back into title indexes to
   underline them. Append the author to the haystack and a query that matched
   *only* the author produces a row with **no underlined characters anywhere** —
   the user sees a row survive the filter with nothing visibly matching. That is
   a worse failure than not supporting it: it reads as a bug.
2. **Discoverability.** This is the concern the brief raises, and the research
   does bear it out — but the sharper finding is that **the tools that ship
   filter-by-author do not rely on the user knowing the name.** lazygit seeds the
   filter from the row under the cursor and prints the full name in the menu; it
   also displays a persistent *"Filtering by '…'"* indicator. Invisible-field
   matching with no visible anchor and no state indicator is not a pattern I found
   shipped anywhere.

I searched for "searchable but invisible field" as a named UX pattern and did not
find one; the closest adjacent literature is on hidden interactions needing
visible alternatives. I am not going to dress that up as evidence — see §8.6.

### 8.3 Q1 — recommendation

**First choice: a fixed 3-cell author slot showing a lowercase abbreviation,
rendered `muted`, present in every section, plus filter-by-author seeded from the
selected row.**

Concretely:

1. **Display.** Insert a 3-cell fixed column. Content is a deterministic
   lowercase abbreviation of the author: initials from the display name when
   there are two tokens (`Bar Spielberg` → `bs`), otherwise the first 2–3
   characters of the login, disambiguated against the set of authors currently on
   the board so two visible people never share an abbreviation. Left-aligned,
   padded to 3, **always emitted** so nothing ever shifts (R13).
2. **Color: none.** Render it in `muted` — `Faint(true)` on default foreground,
   the existing token from §5.1. **It introduces zero new colors.** This is what
   keeps it from being busy, and it is the direct answer to the user's "not too
   busy on the eyes": a faint 3-character lowercase blob has almost no visual
   weight, and it is the one field on the row that is *supposed* to be ignorable
   until you go looking for it.
3. **Self is not special-cased by content, only by section.** In the Mine
   section the cells are **rendered blank** rather than removed, so the title
   column stays at its fixed offset in every section (§6.3 preserved) while the
   Mine section stays as quiet as it is today. This is neomutt `%F`'s intent
   without neomutt's variable layout. (Judgment call: 3 blank cells in the
   busiest section is a real cost; it buys the stability guarantee the spec
   calls non-negotiable. If the user would rather have the 3 cells back, the
   fallback is to always render the abbreviation, including your own — it is
   faint enough to disappear.)
4. **Filter.** Add author to the fuzzy haystack **and** fix the highlighter so an
   author-only match underlines the author cell, not nothing. Additionally — and
   this is the part that carries discoverability — add a key that filters to the
   author of the **row under the cursor**, lazygit-style, and show the active
   author filter in the footer (the footer already has a status-message slot per
   §7.2, so this costs no rows). The user never has to know a login to use it.
5. **Responsive.** Drop the author column at NARROW (<60 columns), before the
   review glyph. It is the only field on the row that never changes what the user
   does — the same argument §6.1 uses to drop age first. New drop order:
   **age → author → review glyph.**

**Why this and not the others.**

- It is the option with **three independent shipped precedents** (lazygit, tig,
  neomutt) all converging on the same 2–3 cell shape for the same reason.
- It is the only display option that costs **zero colors**, which is what makes
  it compatible with both the 16-color constraint and R17's "small learnable
  palette". Every other display option either spends hue (option 2) or spends
  rows (gh-dash's sparse line).
- It is the only one that keeps the board legible in monochrome without
  qualification (R3): initials *are* text.
- Filter-only alone (option 5) is cheaper but leaves the user unable to see who
  wrote what, which is half of what they asked for ("see **and** filter by").
  Pairing display with cursor-seeded filtering gets both, and the visible
  abbreviation is exactly the anchor that makes the filter discoverable — the
  same relationship lazygit has between its `SK` initials and its
  filter-by-this-author menu.

**Costs I am accepting, stated plainly.** 3 cells + 1 gutter = **4 cells off the
title** at FULL tier (title becomes `w - 27`, so 93 at 120 columns, 53 at 80).
The FULL breakpoint in §6.2 was set to keep title ≥ 53, so **80 columns is now
exactly the FULL floor** — the breakpoints in §6.2 need recomputing, not just the
budget in §1.2. Abbreviation collisions are possible but board-scoped
disambiguation makes them rare; a collision degrades to "two people share a
label", which is annoying, not wrong.

**If the user rejects it.** In descending order of what I would try next:

1. **Filter-only, plus cursor-seeded filter and a footer indicator** (option 5
   done properly, no column). Zero cells, zero colors, zero rows — the absolute
   minimum-noise answer, and it still satisfies "filter by author". It loses
   "see the author at a glance", and the highlighter fix is mandatory, not
   optional. This is the right answer if "not busy" turns out to dominate
   everything else.
2. **Author on demand** (option 4): the 3-cell column exists but is off by
   default, toggled by a key, tig-style. Costs one keybinding and one piece of
   invisible mode state. Best if the user wants author *occasionally*.
3. **Author on the selected row only**, rendered in the footer next to the repo
   name rather than in the row — this avoids the mid-row content change that
   would make the selected-row variant an R13 violation, and it costs nothing.
   Worth mentioning because it is nearly free.

I would not fall back to per-author color under any circumstances; at 16 colors
it is not implementable, and at 256 it is a documented anti-pattern (§4.1).

### 8.4 Q2 — headers: options analysed

#### Option A — a persistent header row over the status columns

*Evidence for.* gh-dash and k9s both render one. That is the whole case.

*Evidence against.*

- **gh-dash's header does not label the glyph columns.** Its header cells for
  `state`, `labels`, `numComments`, `reviewStatus`, `ci` and `updatedAt` are
  themselves Nerd Font glyphs (``, `󰌖`, ``, `󰯢`, ``, `󱦻`). A header that
  replaces one unlearned glyph with a *different* unlearned glyph has taught
  nothing and cost two rows.
- **k9s' header earns its row under conditions we do not meet**: word headers,
  a column set that **changes per resource type** (so there is nothing durable to
  memorize), and headers that are **interactive sort controls**
  (`Shift+←/→`, per #3768 / #3955). Ours would be static, constant, and
  non-interactive — chrome, not a widget.
- **Our columns cannot take word headers anyway.** The CI sub-slot is 2 cells,
  review is 1, blocker is 1 (§3.1). "CI" fits in 2; "REVIEW" and "CONFLICT" do
  not fit in 1. Any honest header would need to widen the cluster or abbreviate
  to single letters — i.e. replace glyphs with different glyphs again.
- **Cost.** gh-dash pays `TableHeaderHeight = 2`. Even a 1-row version costs **5%
  of a 20-row pane, on every frame, forever**, against §7.1 which already cut the
  global header on exactly this reasoning and R6 which requires every non-data
  row be justified individually.
- **It would need repeating or floating.** We have multiple sections. A single
  header above all sections is far from the rows it labels (and scrolls away); a
  per-section header multiplies the cost by the section count — 3–6 rows in a
  6-rule config, which is a third of the pane.
- **Demand is absent.** One legend request across three comparable trackers in
  five years (gh-dash #44, zero comments, still open), versus repeated,
  multi-user complaints about chrome rows (gh-dash #671; k9s #270, #1978, #3648).

#### Option B — a `?` help overlay containing a glyph legend

*Evidence for.* Both lazygit (`optionMenu: '?'`) and k9s (`?` → `helpCmd`) ship a
`?` overlay, so the affordance is a genuine convention and users will try it.

*Evidence against, and this corrects an assumption in the brief.* **Neither
overlay actually contains a glyph legend.** k9s' `internal/view/help.go` builds
exactly `RESOURCE`, `GENERAL`, `NAVIGATION`, `HOTKEYS` — all keybindings, sourced
from `MenuHints`. I found no shipped TUI whose help screen explains its status
glyphs. So this would be novel rather than proven; the *container* is
conventional, the *content* is not.

*Cost.* Zero permanent rows. One keybinding, which is already conventional.

#### Option C — a footer legend

*Evidence against.* §7.2 already spends the single footer row on keybindings,
repo and spinner, with the status message **displacing** the keybindings rather
than adding a row. A legend of four CI states, four review states and two
blockers does not fit alongside that at 80 columns, let alone 44. It would also
be permanent chrome by another name — the same cost as option A, just relocated.

#### Option D — a one-time hint

*Evidence for.* Nothing found; I did not locate a TUI shipping first-run
onboarding for a glyph set.

*Evidence against.* Requires persistent state (a "seen it" file), which this tool
does not otherwise have. NN/g's learnability data says the plateau arrives around
trial 4–5 — one exposure is on the wrong side of that, so a single hint is likely
to be both forgotten and unrepeatable.

#### Option E — labels only at wide terminal widths

*Evidence against.* Makes the layout width-dependent in a way §6.3 explicitly
forbids (*"columns 0–10 never move at all"*), and inverts the need: the user who
needs the legend is a new user, not a wide-terminal user. It also means the board
looks different on the laptop than on the desktop, which is §4.10.

#### Option F — no header, glyphs stand alone

*Evidence for.* **This is what almost every comparable tool actually does for
narrow status columns**, including ones far older and more scrutinized than ours:

| tool | narrow status field | header? |
|---|---|---|
| neomutt | `%Z` — 3 unlabeled flag chars | none |
| aerc | `flags:4` — 4 unlabeled glyph cells | none |
| tig | status/graph columns | none |
| lazygit | commit graph, status chars | none |
| gitui | status chars | none |
| gh-dash | 3–4 cell status columns | header exists, but its cells are glyphs, not labels |

`mutt`'s `%Z` has been three unlabeled characters since the early 1990s and I
could find no sustained campaign to label it.

*Supporting argument.* §3 already does most of the work a legend would: four
distinct glyph **silhouettes** per dimension (check / cross / half-disc / dot),
column position disambiguating the reused `✓`/`✗`, and §3.5's monochrome
read-out as an explicit acceptance test. A glyph set designed so that *shape*
carries the meaning is a glyph set that needs a legend less. And two of the three
blocker/draft glyphs are already self-describing ASCII (`!`, `~`) in the aerc
`[e]`/`[s?]` tradition.

*Cost.* Trials 1–4 are slower. For a single-user tool opened several times a day,
that is one afternoon, once, ever.

### 8.5 Q2 — recommendation

**First choice: no header row, no footer legend. Put the legend in the `?` help
overlay, next to the keybindings.**

Reasoning:

1. **A header would not actually label anything.** This is the finding that
   settles it, and it is empirical rather than theoretical: gh-dash *has* a header
   row and its status-column headers are glyphs. Our sub-slots are 1–2 cells; no
   word fits. The only header we could actually render over `✗2 ○ !` is more
   symbols, which teaches nothing and costs 1–2 permanent rows.
2. **The cost is permanent and the benefit is not.** NN/g's learnability data
   puts the plateau around trial 4–5 and advises shaping the curve for the
   highest-value users; here the novice and the expert are the same person, four
   openings apart. A header row is paid on every one of the thousands of frames
   after that. §7.1 already applied exactly this reasoning to cut the global
   header, and R6 requires every non-data row be justified individually — a
   header row does not clear that bar where the global header did not.
3. **The demand evidence points the other way.** One uncommented legend request
   in five years across three trackers, versus repeated multi-user complaints
   about chrome rows in two tools, with maintainers responding by making chrome
   *removable* (k9s' four `-less` flags, `Ctrl+E`, `Ctrl+G`).
4. **The overlay is the conventional place to look and costs nothing.** Both
   lazygit and k9s bind `?`; a user who wants to know what `◐` means will press
   it. Honest caveat: **neither tool actually puts a glyph legend there** — I
   verified k9s' is keybindings-only — so we would be extending the convention,
   not copying it. The extension is low-risk because the container already exists
   and the cost is zero permanent rows.
5. **§3's glyph design already substitutes for a legend** in the way that matters:
   distinct silhouettes, fixed positions, and a documented monochrome read-out.
   The failure mode a legend guards against — several states collapsing into one
   symbol distinguished only by hue (§4.3) — is already designed out.

**Concretely, what to build:** a `?` overlay with two panes — keybindings (which
the footer currently abbreviates anyway) and a **status key** rendering each
glyph next to its meaning in the same styles the board uses:

```
 CI          ✓ passing   ✗N N failing   ◐ running   · none
 review      ✓ approved  ✗ changes req  ○ review required   · none
 blockers    ! conflicts   ~ draft
 author      3-char abbreviation; blank in your own section
```

Rendering the key **with the real glyphs and real colors** is the point — it is a
legend and a color-and-font smoke test at once, which also gives the user a way
to check whether their font is substituting anything (§4.5, gh-dash #520).

**If the user rejects it** — i.e. wants something visible on the board:

1. **A one-line legend shown only when the board is empty or loading.** Those rows
   are already being spent on "loading…" / "—" (§1.6), so the legend is free,
   appears exactly when the user has nothing else to read, and is naturally
   repeated on every cold start until they stop needing it. This is the option I
   would actually push for if "in the `?` screen" feels too hidden; it is close to
   free and it front-loads the trials NN/g says matter.
2. **A footer legend toggled by a key** (`Ctrl+E`-style, k9s' shipped answer to
   this exact complaint), replacing the keybinding half of the footer the way the
   status message already does. Zero permanent rows, visible on the board, no new
   row when off.
3. **A single header row above all sections, not per section**, at 1 row not 2,
   with single-letter labels (`C R B`) — only if the user specifically wants
   always-visible labels. I would argue against it: 5% of a 20-row pane forever,
   it scrolls away from the rows it labels, and `C`/`R`/`B` are three more symbols
   to learn.

### 8.6 What I could not verify

- **Whether gh-dash users find the 15-cell author column too wide or too noisy.**
  I searched the tracker for `author column`, `hide author`, `width`, `avatar`
  and found only #464. Absence of complaints is weak evidence; also note the
  column is not in the default layout any more, so recent users may simply not be
  seeing it.
- **Whether initials collide in practice at team scale.** No tool found does
  collision detection, and I found no issue reporting a collision — but I also
  found no data showing collisions are rare. The disambiguation scheme I
  recommend is my proposal, not something I saw shipped.
- **lazygit's `ShortAuthor` behaviour on GitHub-style logins.** I read
  `getInitials` and reasoned about the single-token branch (`barspielberg` →
  `ba`); I did **not** run lazygit to confirm, and lazygit's input is a git
  `AuthorName` (usually "First Last"), not a login, so this is inference from
  source, not observation.
- **Whether any TUI ships a glyph legend in its help overlay.** I verified k9s'
  does **not** (`internal/view/help.go` — keybindings only) and found no counter-
  example, but I did not audit every TUI's help screen. Treat "nobody does this"
  as "I found nobody doing this".
- **btop and lazydocker** were in the brief for "repeated categorical string in
  dense rows". I did not get useful evidence: btop's data is continuous and
  numeric (§2.4 already concluded it is not applicable), and the lazydocker
  source path I tried returned 404. Neither appears in the findings above and I
  have not substituted guesses for them.
- **slack-term** was in the brief and I found nothing usable; the project appears
  dormant and I did not locate a maintained source of truth for its layout.
  weechat covers the same "show who, densely, in chat" ground with far better
  evidence, so I spent the budget there instead.
- **NN/g's icon research does not address within-app repeated exposure.** I asked
  the article directly and it does not cover it. My §8.4 argument that a
  self-consistent single-app glyph set is learnable is therefore reasoning from
  the learnability/efficiency literature plus the observed behaviour of mutt /
  aerc / tig, **not** a cited finding. It is the weakest link in the Q2 case and
  should be argued with on those terms.
- **The exact breakpoint arithmetic** in §6.2 after adding 4 cells. I have flagged
  that 80 columns becomes the FULL floor, but I have not re-derived the full
  tier table — that is a spec change, not research, and belongs to whoever
  updates `design-spec.md`.

### 8.7 Additional sources (round two)

- gh-dash source (fetched via GitHub API, 2026-09-16):
  `internal/tui/components/prrow/prrow.go`,
  `internal/tui/components/prssection/prssection.go`,
  `internal/tui/components/table/table.go`,
  `internal/tui/common/styles.go`,
  `internal/tui/theme/theme.go`,
  `internal/tui/constants/constants.go`,
  `internal/data/utils.go`, `internal/data/prapi.go`
- gh-dash: [v4.5.0 sparse layout release](https://github.com/dlvhdr/gh-dash/releases/tag/v4.5.0),
  [#44](https://github.com/dlvhdr/gh-dash/issues/44),
  [#464](https://github.com/dlvhdr/gh-dash/issues/464),
  [#515](https://github.com/dlvhdr/gh-dash/issues/515),
  [#520](https://github.com/dlvhdr/gh-dash/issues/520)
- lazygit source: `pkg/gui/presentation/authors/authors.go`,
  `pkg/gui/presentation/commits.go`, `pkg/config/user_config.go`,
  `pkg/integration/tests/filter_by_author/select_author.go`;
  [Config.md §Custom Author Color](https://github.com/jesseduffield/lazygit/blob/master/docs/Config.md);
  [#2351](https://github.com/jesseduffield/lazygit/issues/2351)
- tig: [`tigrc`](https://github.com/jonas/tig/blob/master/tigrc),
  [`doc/tigrc.5.adoc`](https://github.com/jonas/tig/blob/master/doc/tigrc.5.adoc)
- gitui source: `src/components/commitlist.rs`, `src/ui/style.rs`
- neomutt: [`docs/config.c`](https://github.com/neomutt/neomutt/blob/main/docs/config.c)
  (`index_format` default and the `%F` / `%I` / `%L` / `%Z` expansions)
- aerc: [`config/aerc.conf`](https://github.com/rjarry/aerc/blob/master/config/aerc.conf),
  [`doc/aerc-templates.7.scd`](https://github.com/rjarry/aerc/blob/master/doc/aerc-templates.7.scd)
- weechat: `src/core/core-config.c` (`chat_nick_colors`, `chat_nick_self`,
  `nick_color_hash`, `nick_color_force`)
- k9s source: `internal/config/flags.go`, `internal/view/app.go`,
  `internal/view/help.go`, `internal/ui/table.go`;
  [#270](https://github.com/derailed/k9s/issues/270),
  [#1978](https://github.com/derailed/k9s/issues/1978),
  [#3648](https://github.com/derailed/k9s/issues/3648),
  [#3768](https://github.com/derailed/k9s/issues/3768),
  [#3955](https://github.com/derailed/k9s/issues/3955)
- NN/g: [Icon Usability](https://www.nngroup.com/articles/icon-usability/),
  [How to Measure Learnability](https://www.nngroup.com/articles/measure-learnability/),
  [Flexibility and Efficiency of Use](https://www.nngroup.com/articles/flexibility-efficiency-heuristic/)
- prs-mng source read for feasibility: `internal/ui/filter.go` (`haystack`,
  `titleOffset`, `matchedTitleIndexes`), `internal/github/github.go:178`
