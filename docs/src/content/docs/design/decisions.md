---
title: Decisions and why
description: The design decisions that shaped pile, each with the evidence behind it, grouped by theme.
---

Every decision pile makes is a trade with a reason. This page is the record of
those reasons, grouped by what they concern — data, colour, layout,
interaction — so a decision can be revisited without re-arguing it from first
principles. The [principles](../) sit above this table; the
[visual spec](../visual-spec/) is what the decisions add up to.

## Data

How the board gets its information, and what it is allowed to assume about it.

| Decision | Why |
| --- | --- |
| GraphQL over HTTP, not the `gh` CLI | `gh search prs` cannot return `statusCheckRollup` / `reviewDecision` / `mergeable` — most of the board. `gh pr list` returns them but cannot express `review-requested:` / `team-review-requested:`. Neither subcommand alone works; shelling out also costs ~1.7s per call. |
| One search per rule, in parallel | A single combined query with aliased searches is *slower* — GitHub appears to run aliases serially, so one request pays the sum instead of the max. |
| No caching | Cold start is ~2–4s and the spinner is load-bearing. Revisit only if it grates. |
| Repo comes from config, not cwd | The previous tool inferred from cwd and silently showed the wrong repo. `PILE_REPO` overrides per-invocation. |
| Startup reachability check | GitHub's search index reports no PRs for a repo the caller cannot see — an org IP allow list blocking you is indistinguishable from having no PRs — but a direct `repository(...)` query returns a real error, so ask for one at startup. |
| Author is per-rule (`author: true`), not global | A rule like `author:@me` is all one person, so the column is dead weight there. This is what gh-dash #464 asks for and what neomutt has done for decades. The search matches the initials only when the column is drawn, since it matches what is drawn. |
| `-review:approved` on "Needs my review" | Drops PRs somebody else already signed off, which is what makes it an action queue rather than a status list. Changes-requested PRs stay — those still want you. Note GitHub already drops a PR from `review-requested:@me` once *you* approve it. |
| Draft split out of the review slot | Draft is a lifecycle state, review is an outcome, and a draft PR has both. Showing draft in the review slot hid whether it was already approved. |
| No CODEOWNERS path matching | GitHub already turns CODEOWNERS into team review requests, so `team-review-requested:` covers it for free. No per-PR file-list fetch. |

## Colour

How colour is spent, and why there is so little of it.

| Decision | Why |
| --- | --- |
| ANSI 0–15 only, never hex or 256 | The 16 indices are an indirection layer, not a limitation: they resolve through the user's own theme. gh-dash shipped hex, got the bug report, and migrated (#770 / PR #771). Hex values of the same nominal colours do *not* get this (k9s #1234). |
| `muted` is `Faint(true)`, not index 8 | Index 8 is "bright black" — a *light* grey on many light themes, near-invisible on white. Faint is relative: SGR 2 dims whatever the theme's foreground already is, so it is correct on both by construction, and degrades to plain legible text where SGR 2 is ignored. Index 7 is avoided for the same class of reason. |
| Index 8 *is* used, as `selBg` — and it is the shipped selection fill | Its danger is as a *foreground* against index 0. As a background it sits between 0 and 7 in luminance in every mainstream scheme, so it contrasts with the terminal's own background at either end. It was briefly a fixed cube grey (237) instead, on the reasoning that index 8 is mid-grey and mid-grey is the worst backdrop for foreground colour — true where 8 really is mid-grey, and wrong in the many themes where it is a tinted dark. The cost of going themed is that selection contrast now varies by theme; the [visual spec](../visual-spec/) has the cross-theme table and the ratios. |
| Selection = background fill + `▌` bar, not `reverse`, never dim | `reverse` swaps the row's foreground into the background, destroying status colour on exactly the failing rows that need it. Dim reads as "disabled" (lazygit #1845, #802). The bar lives in column 0 where nothing else ever draws, so selection survives a theme that eats the fill. The search hit *does* use reverse, and the visual spec explains why the objection does not reach it. |
| Status is glyph shape first, colour second | Red/green is both the commonest status pairing and the commonest colourblindness. Four distinct silhouettes (`✓ ✗ ◐ ○`) survive monochrome; the documented failure mode is several states rendering as one `•` in different hues. |
| Plain Unicode BMP glyphs, not Nerd Font PUA | gh-dash #290: icons vanished on Nerd Font 3.0.2 — codepoints moved between font versions. Having "a Nerd Font" does not guarantee a specific glyph. No legibility gain at single-cell size. |

## Layout

How the board is shaped, and why nothing takes more than a line.

| Decision | Why |
| --- | --- |
| Every row is exactly one line | Five commits tried to stabilise the scroll while rows had data-dependent heights, and each traded one symptom for another: a single keypress moved the board 0–3 lines depending on whether a neighbouring row carried a failing-check line and whether a section header was crossing the edge. Gate names moved to the `d` overlay, and section names to a header row the cursor can sit on, so every line in the list is a position the cursor can reach and one keypress scrolls at most one line — unconditionally, not by argument. |
| Section headers are inline, not pinned | A pinned line with a `N of M` count was built and reverted. Bound to the top visible row it could not name the wrong section — but on a pane tall enough to show the whole board nothing scrolls, so it froze on the first section and read `MINE · 1 of 5` while the cursor was three sections away. An inline header cannot go stale (it is always directly above its rows) and cannot be a constant (it scrolls away with them). |
| The cursor can sit on a section header | It is the difference between a delta of 1 and a delta of 2. A line the cursor cannot occupy has to be crossed *in addition to* the row it precedes, so a viewport following the cursor jumps. Making the header addressable — but not actionable, so `enter`/`d`/`y` do nothing there — makes the list's lines and the cursor's positions the same set, and the invariant arithmetic. This also removed the blank separator, the last unselectable line. |
| Scroll margin of 2 rows | lazygit ships 2, fzf migrated 0 → 3 in 2024, nnn/micro use 3; no tool surveyed migrated the other way. 2 is the smallest in the cluster and still guarantees the first line of the next PR is visible. Every implementation caps at half the viewport; none is proportional (helix rejected that on the record, #8403). |
| Sections keep their rows while refetching | Rebuilding from empty collapsed the board and re-expanded it section by section, shoving every row below each one that resolved. A row should move when a value changes, not because a request finished. |
| A resolved empty section collapses to one line | Reserving six blank rows for a section that *knows* it has nothing wasted most of a short pane. The reservation applies only while loading. |
| No column header row | Our sub-slots are 1–2 cells — no word fits, so the only renderable header over `✗2 ○ !` is more symbols. gh-dash has a header row and its status headers *are* glyphs. Evidence: one uncommented legend request in five years across three trackers, versus repeated multi-user complaints about chrome rows. The legend lives in the `?` overlay instead, rendered from the same helpers the rows use so it cannot drift. |
| No global header row | Its three payloads were the app name (you just pressed a key), the repo (constant in a single-repo tool) and the spinner. Repo and spinner moved to the footer, which was already being paid for. One row back in a 20-row pane. (gh-dash #671: a 2-line logo drew "wasting space", "attention hogging".) |

## Interaction

How a keypress is interpreted, and what it may trigger.

| Decision | Why |
| --- | --- |
| `/` searches rather than filters | Filtering answered "which PRs match?" and lost the rows around the answer, which are most of why you were looking. Worse, it made highlighting impossible for anything it hid. The board now holds still and matches are filled where they sit. Fuzzy matching, ranking and the numeric special case all go with it: one substring rule over the row's own drawn text. |
| The search matches only what is drawn | Every match has to be visibly highlighted, or the board marks a line and shows no reason. So the haystack is the rendered line: for a PR row, the padded number, the title as clipped to this width, the three author initials; for a section header, the rule name as clipped. The consequences are real and accepted — a full login matches nothing, a word past the clip point matches nothing, and the match set changes with the terminal width. |
| The search matches section titles too | A header is a line on the board the cursor already lands on, so a query that names a section had no reason to come back empty. It cost the match set its address space: matches used to be row indices, which cannot name a header at all, and are now slot indices — the one space that holds headers and rows in draw order. `n`/`N` and `ctrl+n`/`p` stop on a matched header like any other match. The row count on the header is deliberately *not* searchable, for the reason the age cell is not: it is derived state, so the match would appear and expire without the user typing. |
| The checks overlay names failures and pending, counts passes | 47% of check contexts on the live board are skipped and 45% pass, against 4% failing — listing passes buries 7 signal lines under 24 on the worst PR. Pending checks are named instead of counted because they are 1–3 per PR and say *what* you are waiting on. The overlay also stopped dead-ending: 47% of PRs are in PENDING rollup with nothing failing, where it used to say "no failing checks". |
| Actions are shell templates | The tool knows nothing about worktrees, editors or multiplexers; it renders a template and runs what it gets. |
| `y` copies via `pbcopy`, not a clipboard library | The same trade `browser.Open` already makes with `open`: a pipe to a binary that ships with the OS, against a dependency and its transitive tree. It is a `var` so tests never touch the real clipboard. |
| Arc tab reuse built into `open` | Plain `open <url>` always spawns a new Arc tab, so opening the same PR twice buries the window in duplicates. Non-Arc browsers fall through to `open`. |
