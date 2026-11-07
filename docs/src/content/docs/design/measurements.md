---
title: Measured facts
description: Latency and scroll-margin measurements, dated and named, that shaped the design.
---

Numbers that shaped the design, each named against what it was measured on and
when.

Against `acme/monorepo`, Sep 2026. Run-to-run variance is large (the same
query ranged 1.4s–4.1s); these are order-of-magnitude.

| measurement | value |
|---|---|
| cold start, 6 rules, parallel GraphQL | **~2–4s** (1.9 / 3.3 / 4.2s observed) |
| 1 combined GraphQL request, 6 aliased searches | **3.5–7.7s** — slower; aliases appear to run serially |
| `gh api rate_limit` (process startup + auth) | **1.7–2.4s** |
| the same call over raw curl, warm | **~0.45s** |
| `mine` rule alone | **1.4s** vs 3.1s for `allopen` |
| varying the row cap (5 / 10 / 20 / 50) | **no effect** — limit=5 took 5.7s, limit=20 took 2.9s in the same batch |

Latency is per-request overhead and search-backend variance, **not payload
size**, so capping rows is not a speed lever — set limits for display sanity.

Scroll-margin defaults across surveyed tools: lazygit 2, fzf 3 (was 0 until
v0.53), nnn 3, micro 3, LazyVim 4, vim `defaults.vim` 5, helix 5, yazi 5,
ranger 8, kickstart 10; gh-dash / k9s / htop / btop / `bubbles/viewport` have
none. Modal value 3. Every implementation caps at half the viewport height; none
is proportional.
