---
title: PR detail
description: What the d overlay shows about one PR, and what arrives on a second request.
---

`d` opens a one-PR page that answers "what do I do about this PR". Scroll with
`j`/`k`, arrows, page keys, `ctrl+d`/`ctrl+u`, or `g`/`G`; `/` searches within
the page and `n`/`N` step matches. `esc`, `q` or `d` closes it after clearing
any live search. `?` opens help and `ctrl+c` quits.

The overlay is two blocks. The checks block is the top and names what is
failing or running; the state block below it says what is true about the PR
itself.

## Checks

Failing checks are named with `✗`, still-running ones with `◐`, in the API's own
order. Passing checks are **counted, not named** — 47% of contexts on the board
this was built against are skipped and 45% pass, so listing everything would
bury the signal.

When nothing is wrong and nothing is running, the count is the whole answer:
`✓ all 12 checks passing`. A tally line under a list closes it the way
`gh pr checks` does, so the numbers reconcile against GitHub — `✓ 8 passing, 3
skipped`.

## State

The state block is labelled lines about the PR, not its CI:

| line | what it says |
| --- | --- |
| `! conflicted · 26 commits behind main` | a merge conflict, and how far behind its base |
| `· 26 commits behind main` | behind, without a conflict |
| `● 3 unresolved comments` | open review threads someone still has to answer |
| `author: Grace Park (grace-park)` | who wrote it, display name and login |
| `review: ✓ alice · ✗ bob` | who has formed an opinion, approved or changes requested |
| `size: +1,204 −318 · 14 files` | additions, deletions, files |
| `base: release-2026.06` | the target branch, shown only when it is not the default |
| `branch: grace/fix-sync` | the head branch |
| `opened: 3w ago · idle 4d` | how long it has been open, and how long it has sat |
| `labels: bug · needs-qa` | the PR's labels, shown only when it has some |

Lines with nothing to say are not drawn. Long pages scroll rather than dropping
failures or state lines. A PR behind its base shows whether the repository
requires an update before merging, rather than treating every behind count as
a blocker. A native GitHub stack is identified in the state block.

## What arrives late

The conflict/behind count, the unresolved-comment count, the reviewers, the
labels, and the default branch come from a second request, made once per `d` press. It takes
1.2–1.9s, so the overlay draws without those lines and they land a moment
later. The page never waits on the network, and a failed request costs those
lines and nothing else. The reviewers are only those who have actually
reviewed; requested reviewers are not listed, because they name no opinion yet.

The counts a row already carries — CI state, review decision, draft, conflict —
are available immediately and never part of the second request.
