---
title: Review in a TUI
description: Binding a key that reviews the selected PR in a full-screen tool.
---

A review TUI takes over the terminal, so it wants the `suspend` mode: pile hands
the terminal over, and repaints the board when the tool exits.

```yaml
actions:
  - key: v
    name: review
    run: tuicr pr {{.Number}}
    mode: suspend
```

`tuicr` here is a stand-in for whatever full-screen reviewer you run. The point
is the mode, not the tool: `suspend` is for any command that needs the whole
terminal — a diff pager, an editor, a review session — where `background` would
leave the command drawing over a board that is still trying to render.

While suspended, the board is gone from the screen. When the tool exits, pile
repaints; a non-zero exit reports `review failed: <last line of stderr>` on the
status line, and a clean exit says nothing.

If your reviewer does not take over the terminal — one that writes a comment
file and exits — leave the default `background` mode and read the result in the
footer.
