---
title: Limitations
description: What pile does not do yet, what it diverges from the research on, and what is untested.
---

The honest list of what pile does not do yet, where the code diverges from the
design research, and what has not been tested against reality.

**`limit` is applied before dedup.** A rule's `limit` caps the GraphQL page size,
but first-match-wins removes already-claimed PRs *after* the fetch. A capped rule
under-fills: a `limit: 20` rule whose first 8 results were claimed above shows 12
rows, not 20.

**Review follow-up is incomplete.** `d` shows reviewers who approved or
requested changes, plus unresolved review-thread counts. Requested reviewers
and “is this waiting on me or on them” are still missing.

**Not built:** a separate diff/description preview; built-in write actions
(approve, comment, re-run CI, mark ready); a `glyphs = "ascii"` escape hatch
(specified, cheap because widths are already fixed); a config key for the
scroll margin (hardcoded on purpose — there is one user).

**Spec/implementation divergences**, stated rather than smoothed over:

- The research recommended board-scoped **disambiguation** of author initials and
  blanking the cells in your own section. The implementation is simpler: the
  first 3 lowercase characters of the login, shown only on rules with
  `author: true`. Collisions are possible and tolerated — the column answers "is
  this mine or someone else's". Those three characters are also the whole of
  what an author search can match, since they are the whole of what is drawn.
- The research also proposed a **cursor-seeded author filter** key
  (lazygit-style, filter to the author of the row under the cursor). Not built.
- The spec's drop order was age → review glyph; adding the author column made it
  age+author → review glyph, and author only exists at FULL tier.
- The spec asserts the FULL breakpoint must be recomputed once the author column
  costs 4 cells (it would push the FULL floor to 80). Fixed: `widthTierFor`
  starts FULL 4 columns later for a rule that shows an author, so the 53-cell
  title floor holds in both cases.
- The research said the scroll offset must be computed in lines. The
  implementation anchors the viewport's bottom edge and clips the top row
  instead. The research is wrong, not the code.

**Untested against reality:** whether ~2–4s cold start is tolerable behind a
spinner in daily use; whether the board reads correctly on a light theme (the
`muted`/`selBg` reasoning is by construction, not observation).

**Release coverage:** the Linux v0.1.0 amd64 release has been confirmed
natively for the board, browser opening, clipboard copying and quitting. On
macOS Apple Silicon, the public Homebrew v0.1.0 install, `brew test`, version
output, board, clipboard, browser opening and quitting have been confirmed.
Linux Homebrew installation, public version-to-version upgrades, and other
architectures remain untested. CI builds and tests on Linux and macOS.
