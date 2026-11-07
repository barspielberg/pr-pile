---
title: Search
description: What / searches, and what is deliberately not searchable.
---

`/` searches the board without moving it. Nothing is hidden, nothing is
reordered, no glyph changes — matches are filled where they sit, the cursor
walks to the first one as you type, and `n` / `N` step through the rest,
wrapping at the ends.

## What a query matches

The query is a plain case-insensitive substring of **what the row draws**: the
PR number, the title as it appears at this width, and the author initials.
Nothing else, on purpose — matching text that is not on screen would mark a row
with nothing visibly marked on it. A section header matches on its name, as
clipped to fit.

Three things follow, and they are the design rather than caveats.

**Author search is the three characters you can see.** `imm` matches and
highlights the initials; `immanuel` matches nothing, because characters four
onward are not drawn.

**A title match past the clip point does not exist.** Widen the pane and it
appears; narrow it and it goes. In a narrow pane the author and age columns are
not drawn at all, so they are not searchable either.

**The age column is never searchable**, even when it is drawn: it is computed
from the clock, so a match on it would expire on its own. The header's row count
is omitted from the matchable text for the same reason.

## In the prompt

While the prompt is open, every printable key is query text, so navigation
moves to chords.

| key | action |
| --- | --- |
| `ctrl+n` / `ctrl+p` | next / previous match while typing |
| `ctrl+j` / `ctrl+k` | the same, other convention |
| `down` / `up` | the same |
| `enter` | keep the query and its highlights, close the prompt |
| `backspace` | edit the query |
| `esc` | cancel, restoring the cursor to where `/` was pressed |
| `ctrl+c` | quit |

## After the prompt

`enter` keeps the query and its highlights and closes the prompt — that is what
`n` / `N` walk afterwards. `esc` in the prompt cancels and puts the cursor back
where `/` was pressed; `esc` on the board clears the highlights. `n` / `N` wrap,
and the wrap is announced — `search hit BOTTOM, continuing at TOP` — because a
silent wrap is indistinguishable from being stuck.

The board cannot move to tell you a query found nothing, so the query text
itself turns red until a match exists, then reverts. A whitespace-only query
matches nothing.
