---
title: Rules and ordering
description: How rules become sections, and why their order is the configuration.
---

Rules are the whole abstraction. A rule is a GitHub search query plus a name;
the board is that list of rules drawn top to bottom, one section each. This
page is the mental model behind them. The fields are on the
[Rules](../../reference/rules/) page and the top-level list on
[Config](../../reference/config/); the [team board](../team-board/) guide works a
five-rule board through end to end.

## A PR belongs to the first rule that matches

A rule's `query` is GitHub search syntax, scoped for you: pile prepends
`repo:<repo> is:pr is:open`, so you write only the part that distinguishes the
rule. `author:@me` becomes `repo:owner/name is:pr is:open author:@me`.

Each rule is fetched as its own search, then the results are walked top down. A
PR is shown under the **first** rule that matched it, and every later rule skips
it. Two things follow.

**Ordering is the configuration.** Moving a rule up widens it at the expense of
everything below it; moving it down shrinks it. There is no other way to say
which section a PR belongs in, and none is needed — the order already says it.

**A section's contents are only knowable once every rule above it has
resolved.** An earlier rule may still claim a PR a later one has already
fetched, so the board cannot finalise a section until the one above it is done.

## Rules are fetched in parallel, revealed in order

Every rule's search runs concurrently. Only the reveal is ordered: the board
draws as far down as the first rule that has not finished, and stops there.
Everything below it is still loading even if its own request has already come
back.

The practical effect is that a slow rule near the top stalls everything under
it. Put the cheap, high-value queries first — your own PRs and the review queue
— and the wide, slow catch-alls last. The default rules are ordered this way.

## `limit` truncates before dedup

`limit` is `first: N` on the search itself: GitHub truncates the result set
before the tool sees it. A `limit` of `0` or less fetches the default of `20`.

Because the truncation happens before first-match dedup, a rule with a low
limit can fail to claim a PR it would otherwise have matched, and that PR falls
through to a later rule. That is not an error; it is the cost of a small page.
The catch-all rule at the bottom is the one place truncation is harmless,
because nothing below it is waiting.

## `tree` is scoped to the section

`tree: true` groups stacked PRs into a chain, drawn with `╭╴ │ ╰╴` down the
side. The chain is worked out from the PRs **in that section only**. Because
first-match-wins routinely splits one stack across two sections, each section
draws the correctly-closed sub-chain of what it actually holds — the glyph
means "stacked on something in this section", not "stacked".

Keep a tree rule's `limit` above the number of PRs it matches. Results come back
newest-first and the newest PR in a stack is its top, so a limit that cuts a
chain cuts it at the root: the children regroup into a shorter, valid chain
with no sign that the base is missing.

## A failed rule claims nothing

If one rule's search errors, that section is marked failed and shows the error
in place of rows. A failed rule cannot claim anything, so the rules below it
show the PRs it would have taken. That is deliberate: one broken query should
degrade one section, not stall the whole board.

## Refresh keeps the board on screen

`refresh` (default `3m`; `0` disables) refetches every rule on a timer, and `r`
does it on demand. While the refetch runs, the current rows stay on screen,
marked stale, so the board does not collapse and re-expand under the cursor.
Sections then resolve progressively again, top down, exactly as at startup.

## Order as "the rest of"

A readable board arranges its rules so that each is a superset of the one above:
*your PRs*, then *your team needing review*, then *your team*, then *everyone
else*. Each section then reads as "the rest of" the previous one — whatever the
more specific rule above it did not take. The [team board](../team-board/) guide
shows why this matters: swap two rules and a section goes permanently empty
without a single error, because the more specific rule stopped taking its
share.
