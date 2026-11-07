---
title: Troubleshooting
description: What goes wrong at startup and while the board runs, and how to read pile's error messages.
---

This page is the symptoms you are likely to hit, in the order you will hit
them: startup, then opening, then the board itself.

## `gh` is missing or logged out

pile reads your token with `gh auth token` and exits if it cannot:

```
pile: gh auth token: exit status 1 (is gh installed and logged in?)
```

```
pile: gh auth token returned nothing; run `gh auth login`
```

Fix with `gh auth login`, then run `pile` again. pile never stores a credential
of its own, so there is nothing to configure on pile's side — the token comes
from `gh` every run. See [CLI](../../reference/cli/) for the full list of what
`gh` is used for.

## The repo exists but pile says you cannot see it

On startup pile asks GitHub directly for the configured repo, before drawing
anything. If GitHub will not give it to you — a private repo you have no access
to, or an organisation whose **IP allow-list** does not include your address —
pile exits instead of rendering a board:

```
pile: cannot read acme/monorepo: repo acme/monorepo not found, or you cannot see it
```

That message is the whole story: either the name is wrong, or your account is
not allowed to see the repo from this network. If it is an org IP allow-list,
the fix is to add your address in the org's settings (or use a network that is
already listed) — pile cannot work around it, because GitHub will not return
the data.

pile checks this first for a reason: GitHub's search index returns *zero* PRs
for a repo the caller cannot see, which is indistinguishable from a repo that
simply has no open PRs. Without the startup check you would get a board that
looks empty when it is actually blocked. The check turns that silent failure
into the error above.

## Opening a PR creates another browser tab

`enter` / `o` uses `open` on macOS or `xdg-open` on Linux to launch the default
browser. On Linux, install `xdg-utils` and run inside a graphical session.
Opening the same PR twice
creates two tabs; pile does not script Arc or reuse tabs. Use a browser tab
deduplication extension if you want repeated opens to focus an existing tab.

## `y` does not copy anything

`y` copies the selected PR's URL; `Y` opens the copy menu. Copy failures appear
in the status line. macOS uses `pbcopy`. On Linux, install `wl-clipboard` for
Wayland (`wl-copy`) or `xclip` / `xsel` for X11. pile prefers `wl-copy` when
`WAYLAND_DISPLAY` is set, then tries `xclip` and `xsel` when `DISPLAY` is set.
A headless or SSH session without access to a desktop clipboard cannot copy
through these tools.

## A section looks empty while the board loads

Sections fill in top-down, and a section below the one still loading shows a
spinner — never an empty list. That is correct behaviour, not a bug: because a
PR is shown under the *first* rule that matches it, a later section cannot be
drawn until every section above it has resolved, or a row could be shown and
then claimed by an earlier rule a moment later.

A section that finishes with no rows collapses to a single `—` line. A section
whose request failed shows its error in place rather than stalling the rest of
the board. If a spinner sits for a long time, the request is slow or failed —
`r` refreshes the board, and the [rules guide](../../guide/rules/) explains why a
slow rule near the top stalls everything under it.
