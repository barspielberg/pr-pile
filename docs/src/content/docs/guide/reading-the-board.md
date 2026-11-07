---
title: Reading the board
description: What every glyph and column means, and how the board changes as the terminal narrows.
---

The board is one line per PR, with a section header above each group of rows.
This page explains what each part of a line says. Every colour on the board is
an ANSI 0–15 slot, so it resolves through your terminal theme rather than a
fixed palette, and every state is also carried by the glyph's shape — colour is
reinforcement, not the only signal.

## One row, field by field

```text
  ▌ #143 add --since flag to the sync command   ✓ ✓ !   imm  2h
  ╰╴ #144 tidy the --since parser               ◐ · ·   bna  4d
```

Left to right:

| part | what it is |
| --- | --- |
| `▌` | the cursor. A selected row shows this mark in column 0 and a fill across the line. |
| `╭╴ │ ╰╴` | a stack. The PR targets the PR above it; `╭╴` is the top, `╰╴` the bottom of the chain. Only present on `tree: true` rules. |
| `#143` | the PR number, padded to a fixed width and drawn in blue. |
| `✓ ✓ !` | the status cluster: CI, then review, then a blocker. |
| title | the PR title. A conventional-commit prefix (`fix(scope): …`) is coloured by part; a draft's whole title recedes. |
| `imm` | the author's initials, lowercase, on rules with `author: true`. |
| `2h` | how long ago the PR was last updated. |

## The status cluster

Three glyphs, always in the same order, always at the same screen offset.

### CI

| glyph | meaning |
| --- | --- |
| `✓` | checks passing |
| `✗2` | `2` checks failing — the number is the count of failing gates |
| `◐` | still running |
| `·` | no checks |

The failing count saturates at `✗+` for ten or more. Failing and running checks
are named in the [detail](../detail/) overlay; the row only counts them.

### Review

| glyph | meaning |
| --- | --- |
| `✓` | approved |
| `✗` | changes requested |
| `○` | review required |
| `·` | no decision |

### Blocker

| glyph | meaning |
| --- | --- |
| `!` | merge conflict |
| `~` | draft |
| (blank) | neither |

One cell holds both: a conflicted draft shows `!`, because the conflict is the
thing that will bite.

## The section header

Each section opens with a full-width header: the rule's name on the left in bold
teal, the row count right-aligned on the right. The header has no fill — it
separates itself from the rows by indentation (name at column 2, rows at column
4), weight (bold, and nothing else on the board is), and hue.

A header is a position the cursor can sit on, which is what keeps scrolling to
one line per keypress. While a rule is still loading, its count is empty; an
empty resolved section collapses to a single `—` line under its header; a failed
one shows the error in that line's place.

## The footer

The bottom row names where you are. On the left, the key reminder (or a status
message when one is showing). On the right, the section and your position inside
it — `NEEDS MY REVIEW · 3 of 10`. A header scrolls away with its section, so
this right-hand field is the only place the current section is named once you
are past its top.

## Responsive tiers

Exactly one column flexes — the title — so the status cluster stays at the same
screen offset at every width. What drops as the terminal narrows:

| width | row contents |
| --- | --- |
| `76`+ columns (`80`+ with an author column) | CI, review, blocker, title, author initials, age |
| `60`–`75` (`60`–`79` with author) | CI, review, blocker, title — age drops |
| below `60` | CI, blocker, title — review glyph and age drop |

The author column appears only on rules with `author: true`, and only at full
width. Below `40` columns the board refuses to draw and asks for a wider
terminal.

What drops is also what stops being searchable: the [search](../search/) page
explains the connection, because a match is always on something you can see.
