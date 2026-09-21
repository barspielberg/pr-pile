# prs-mng — what belongs on the PR detail overlay

Research and specification, written in response to:

> *"checks is good but we now can expand it, we can display conflicting files,
> author full name, reviewer and so on."*

Researched 2026-09-17 against the shipped code at `39ad152` and against 50 live
open PRs from `acme/monorepo`, plus a local clone of that repo used to
ground-truth the conflict measurements in §4.

This document extends `checks-page.md` rather than replacing it. Its principle —
**name what you can act on, count what you cannot** — survives intact and is in
fact what decides most of the questions below.

---

## 1. Recommendation

**Expand it, rename it, and add four fields — not the ones that were asked
for.** The overlay becomes the PR detail page rather than the checks page, and
the single highest-value line on it is one the brief did not mention:
**unresolved review comments**, which exist on 6 of 50 PRs and are invisible on
the board for 5 of those 6 — including two PRs the board draws as `✓ approved`.
Of the three items named in the brief, one is impossible, one is nearly
worthless, and one is cheap and fine: **a per-file conflict list does not exist
in the GitHub API** (§4 — I measured the closest available proxy against `git
merge-tree` on all 10 conflicting PRs and it over-reports by 3× in the median
case and 8× at worst, so it is not shippable), **naming reviewers adds nothing
the `reviewCell` glyph does not already say on 49 of 50 PRs** (§5.3), and
**author full name is one free field but is null for 36% of this board** so it
must degrade to the login rather than replace it. What earns a line instead is
the stuff that answers *"what do I do about this PR"* when CI is green:
unresolved comments, how far behind the base branch it is, its size, and — for
stacked PRs, which this board has — what it is actually targeting. Everything
stays in tier (a) or (b) except two fields worth one on-demand request measured
at **1.2–1.9 s**, fetched on `c` press, which I recommend accepting because it
is paid once for the one PR you asked about rather than 100 times for the board.
The overlay stays a glance: no scrolling, no sections you navigate, any key
still closes it.

---

## 2. The cost model, measured

All numbers are live `gh api graphql` against `acme/monorepo`, the user's
own token, 2026-09-17. Latency figures are the median of 3–5 runs.

### 2.1 Baseline: what the board costs today

The shipped `searchQuery`, `first: 50`, the largest rule (`is:pr is:open`):

| metric | value |
|---|---|
| response size | **159,594 bytes** |
| latency | 4.9 – 6.2 s (median 5.96) |
| rate-limit cost | **1 point** |

The 5 rules in the user's config run in parallel. Measured together:

| rule | PRs returned | bytes |
|---|---|---|
| `author:@me` | 4 | 23,142 |
| `review-requested:@me -review:approved` | 5 | 21,592 |
| `team-review-requested:…` | 12 | 47,014 |
| `involves:@me -author:@me` | 10 | 39,162 |
| `draft:false` | 50 | 187,432 |
| **total** | | **~318 KB, 8 s wall** |

### 2.2 Tier (b): adding fields to the existing search query

Each group added to the baseline query in isolation, same rule, same limit:

| added | bytes | Δ vs baseline | cost | latency |
|---|---|---|---|---|
| *(baseline)* | 159,594 | — | 1 | 5.96 s |
| `author { name }` | 160,441 | **+847 (+0.5%)** | 1 | 7.19 s |
| `additions deletions changedFiles createdAt` + 3 more scalars | 167,819 | +8,225 (+5.2%) | 1 | 9.32 s |
| `reviewRequests` + `latestOpinionatedReviews` | 166,159 | +6,565 (+4.1%) | **2** | 6.99 s |
| `labels(first: 10)` | 162,191 | +2,597 (+1.6%) | **2** | 4.62 s |

**The recommended tier-(b) set** — `author { name }`, `additions`, `deletions`,
`changedFiles`, `createdAt` — measured over 3 runs:

| metric | value | vs baseline |
|---|---|---|
| response size | **171,163 bytes** | **+11,569 (+7.3%)** |
| latency | 6.05 – 8.49 s | within baseline's own 4.9–9.3 s spread |
| rate-limit cost | **2 points** | +1 |

### 2.3 Does tier (b) risk secondary rate limits across 5 rules at 50 PRs?

**No, and it is not close.** Measured directly by running all 5 configured rules
in parallel with the recommended field set:

- **Total cost: 10 points** (2 per rule) against a limit of **5000/hour**.
- After the entire measurement session — dozens of queries including several
  50-PR fetches — `rateLimit` reported `used: 253, remaining: 4747`.
- **Zero** secondary-rate-limit responses, zero 403s, zero `RATE_LIMITED` errors
  across the whole session.
- Wall time for the parallel fan-out: **8 s**, unchanged from baseline.

At the configured 3-minute refresh, 10 points per refresh is **200 points/hour**
— 4% of the budget. The board could refresh every 10 seconds and still fit.

One caveat found the hard way and worth recording: **there is a real ceiling,
and it is latency, not rate limiting.** A first attempt that added *everything*
at once (name + 7 scalars + reviewers + labels + `reviewRequests` +
`latestOpinionatedReviews` + `mergeStateStatus` + `isInMergeQueue` on a 50-PR
search) returned **HTTP 504 "We couldn't respond to your request in time"**.
GitHub timed the query out. This is the actual budget constraint on tier (b) and
it is the strongest argument in this document against adding fields
speculatively: the search endpoint has a server-side time limit, and a kitchen-
sink selection set crosses it. The recommended set was re-measured three times
after that and never timed out — but it is close enough to the edge that
**anything added later must be re-measured, not assumed.**

### 2.4 Tier (c): a second request per PR

Everything tier-(c) I recommend, bundled into **one** query (base-branch
comparison + review threads):

| metric | value |
|---|---|
| response size | **912 bytes** |
| latency | **1.18 – 1.95 s** (median 1.36) |
| rate-limit cost | **1 point** |

For reference, fetching the PR's changed-file list is similar — 969 bytes /
0.72 s for a 9-file PR, 8,036 bytes / 1.27 s for a 379-file PR.

**When to fetch: on `c` press, not upfront.** The arithmetic is one-sided:

| strategy | requests | points | cost paid |
|---|---|---|---|
| upfront, whole board | ~100 (deduped across rules) | ~100 | on every cold start, for PRs you never open |
| on `c` press | 1 | 1 | only for the PR you asked about |

`DESIGN.md` §2 records that this tool **refuses caching** and that cold start is
~2–4 s with a load-bearing spinner. Upfront fetching would add ~100 requests to
that cold start and roughly double it — it would break the one number the design
document commits to. On-demand adds **1.4 s to a keypress you made
deliberately**, on a page you are about to read for several seconds anyway.

That 1.4 s is not free and the spec must handle it honestly: see §7 for the
progressive-render rule — **the overlay draws immediately from tier (a)/(b) data
and the two on-demand lines appear when they arrive.** The overlay is never
blocked on the network, so `c` remains instant and `c`-then-any-key still works
even if the fetch never returns.

---

## 3. The field list, prioritised

Ordered by what earns its line. Tier is (a) already in the `PR` struct, (b) one
field on the existing search query, (c) a second request.

| # | field | tier | cost | why it earns a line |
|---|---|---|---|---|
| 1 | **failing / pending checks** | (a) | free | unchanged; this is what `c` is for and it stays at the top |
| 2 | **unresolved review comments** | (c) | shared 1.4 s request | **the single best addition.** 6 of 50 PRs have them; on **5 of those 6 the board's review glyph does not say `✗`** — two are drawn `✓ approved`. Nothing else on the board or overlay surfaces this. §5.4 |
| 3 | **commits behind base** | (c) | same request, no extra round trip | the honest, *correct* replacement for "conflicting files". `CONFLICTING` says you have a problem; `26 behind` says how big it is. §4.4 |
| 4 | **author** (name, falling back to login) | (b) | +847 bytes (+0.5%) | the row shows 3-letter initials — `osc`, `cdi`, `sau` are not people. But `name` is **null for 36% of this board**, so it must render `name (login)` or bare `login`. §5.2 |
| 5 | **size** — `+N/−M`, `K files` | (b) | +8,225 bytes with its group | decides *how* you review before you open: median 9 files, but the board spans 1 to 379 files and 3 to 4,489 added lines. One line, three numbers, no wrapping. §5.5 |
| 6 | **base branch, when it is not the default** | (a) | **free — already fetched, unused in UI** | #3109 targets `fix-PROJ-1839-reject-legacy-option`, not `master`. On a board with a stacked-PR tree this is load-bearing and currently invisible. Shown **only** when `BaseRefName != "master"`. §5.6 |
| 7 | **head branch** | (a) | free | the string you need to `git checkout`. Cheap, but it is the first thing dropped (§6) |
| 8 | **age** — created, and idle-since | (b) | `createdAt`, in the scalar group | the row's `2h` is *updated*, which is not the same question. 14 of 50 PRs are >7 days old; 4 are idle >7 days. Shown as one line, only when it says something (§5.7) |

Everything else was rejected. §5 says what and why.

### 3.1 What this does to the query

```graphql
# tier (b) additions to searchQuery — measured +7.3% response, +1 cost point
author { login ... on User { name } }
additions deletions changedFiles createdAt
```

```graphql
# tier (c), one request, fired on `c` press only — 912 bytes, 1.36 s, 1 point
query($n: Int!, $head: String!) {
  repository(owner: $o, name: $r) {
    pullRequest(number: $n) {
      baseRef { compare(headRef: $head) { behindBy aheadBy status } }
      reviewThreads(first: 100) { nodes { isResolved isOutdated } }
    }
  }
}
```

Note `compare(headRef:)` takes a **per-PR literal**, so it cannot be batched
across a search result with one variable — verified, it errors. This is the
mechanical reason "commits behind" cannot be tier (b) and must ride the
on-demand request. It is also why fetching it upfront would mean 100 separate
queries rather than one bigger one.

---

## 4. Conflicting files: what GitHub actually exposes

The brief asked me to find out rather than assume. I did, and the answer is no.

### 4.1 The API has no per-file conflict list

Verified by introspection and by direct request against a known-conflicting PR:

- **GraphQL `PullRequestChangedFile`** has exactly five fields:
  `path`, `additions`, `deletions`, `changeType`, `viewerViewedState`.
  `changeType` is a `PatchStatus` — `ADDED`/`MODIFIED`/`REMOVED`/… — which
  describes *what this PR did to the file*, not whether it conflicts.
- **Schema-wide search** for conflict types returns only
  `BranchProtectionRuleConflict` and the merge-queue family. Nothing per-file.
- **REST `pulls/{n}/files`** returns
  `filename, status, additions, deletions, changes, patch, mal, …` — `status` is
  again added/modified/removed. No conflict field.
- **REST `pulls/{n}`** exposes `mergeable` and `mergeable_state` and nothing
  more granular.

`mergeable` (already fetched, already the board's `!` glyph) and
`mergeStateStatus` (`DIRTY`) are the *entire* conflict vocabulary GitHub offers.
**GitHub's own web UI computes the conflicted-file list server-side for its
conflict editor and does not expose it through either public API.**

### 4.2 The closest proxy, and why it is not shippable

The only API-computable approximation is: *files this PR touched* ∩ *files the
base branch touched since the merge base*. Two `compare` calls plus an
intersection.

I ground-truthed it against `git merge-tree --write-tree --name-only` — git's
own real merge — on **all 10 conflicting PRs on the live board**:

| PR | PR files | proxy says | **git says** | over-report |
|---|---|---|---|---|
| #3237 | 9 | 6 | **2** | 3.0× |
| #3228 | 4 | 3 | **1** | 3.0× |
| #3186 | 7 | 3 | **2** | 1.5× |
| #3143 | 79 | 36 | **31** | 1.2× |
| #3137 | 16 | 3 | **2** | 1.5× |
| #3083 | 26 | 13 | **3** | 4.3× |
| #3031 | 129 | 16 | **2** | **8.0×** |
| #3027 | 26 | 2 | **2** | 1.0× |
| #2994 | 37 | 8 | **2** | 4.0× |
| #2909 | 21 | 16 | **12** | 1.3× |

**It over-reports on 9 of 10 PRs.** Median 3.0×, worst case 8×: on #3031 it
would list 16 files when 2 actually conflict. Two files both branches touched
usually merge cleanly — that is what three-way merge is *for* — so "both
touched" is simply a different fact from "conflicts".

This fails the standard the rest of this codebase holds itself to.
`checks-page.md` §3 went out of its way to make the tally *reconcile* against
GitHub's own numbers, on the grounds that "a reader who notices is left
wondering what was hidden". A list that names 16 files when 2 conflict is worse
than that: it does not merely hide, it **fabricates**. You would open the PR,
find 14 of the named files merge fine, and stop trusting the page.

It also costs 2 extra requests (~1.7 s each, and the base-side `compare` needs
`--paginate` on a busy master) to produce a number that is wrong.

**Rejected.** Not on cost — on correctness.

### 4.3 The local-git option, and why not

`git merge-tree` gives the exact answer and is fast — measured **82 ms, 92 ms,
271 ms** on three conflicting PRs in the user's own clone at
`~/Repos/acme/monorepo`. Tempting, and rejected for three reasons:

1. It needs the branch fetched. A bare `git fetch origin master` measured
   **2.3 s** — slower than the API call it replaces, and that is one ref.
2. It needs `repoPath`, which **is not set in the user's config** (it exists in
   `config.go:40` and is currently only interpolated into action templates).
   The feature would silently do nothing for the default configuration.
3. `DESIGN.md` §2 is explicit that the tool "knows nothing about worktrees,
   editors or multiplexers; it renders a template and runs what it gets."
   Shelling into a clone to compute merge state is exactly the coupling the
   tool has avoided, and the package doc opens by explaining that shelling out
   was rejected on principle *and* on a 1.7 s cost.

Worth recording as the only correct option, so the next person does not
re-derive it. Not worth building.

### 4.4 What to show instead

**`N commits behind <base>`**, from `baseRef.compare(headRef:){ behindBy }`.

It is exact, it is one field on a request already being made for review threads,
and it answers the question the user is actually asking underneath "which files
conflict" — *how much work is this to fix?* #3186 is 26 commits behind; #3237 is
9. A PR 3 behind is a rebase; a PR 26 behind is an afternoon.

Paired with the `!` the board already draws and the `CONFLICTING` state already
fetched, the overlay says: **conflicted, 26 commits behind master** — which is
true, actionable, and costs nothing that is not already being spent.

---

## 5. What was rejected, and why

### 5.1 A conflicting-file list — §4. Does not exist; the proxy fabricates.

### 5.2 Author full name as a *replacement* — reshaped, not rejected

`author { login name }` is one field, **+847 bytes on a 50-PR response (+0.5%)**
— genuinely free. And the row's 3-letter initials are bad at the job: `osc`,
`cdi`, `sau`, `eri` are not names.

But **`name` is null for 18 of 50 PRs (36%)**, including every `dependabot` PR
and several human accounts (`victoramara`, `peggy-osei`, `frank-oye`,
`oscar-lin`) that simply have no display name set. A line that renders
`author:` blank on a third of the board is worse than no line.

**Accepted as `name (login)`, degrading to bare `login`.** The login is the
identifier you actually use to `@`-mention or search, so it earns its place even
when the name is present. Same problem applies to reviewer names — the `opin`
data returns `Alice Chen` for one reviewer and bare `WalterN12` for another — so
any place a person is rendered uses the same fallback helper.

### 5.3 Naming reviewers — rejected

This is the item where the measurement most clearly contradicts the request.
Across 50 live PRs:

| situation | PRs | what naming people would add |
|---|---|---|
| no reviewer data at all (both lists empty) | **26 (52%)** | nothing — an empty line |
| exactly 1 opinionated reviewer | 14 (28%) | the glyph already says `✓` or `✗`; the name adds *who* |
| **2+ opinionated reviewers** (glyph hides disagreement) | **0** | — |
| requested reviewer is a **team** | 17 | `web platform`, `web-platform-eu` — a team, not a person |
| requested reviewer is a **named person** | **1 (2%)** | the one genuinely useful case |

The brief asked what naming people adds that `reviewDecision` does not. Measured:
**on 49 of 50 PRs, nothing.** `reviewDecision` is a faithful summary here because
this repo's PRs have exactly one opinionated reviewer or none — I verified
`latestOpinionatedReviews` is not under-reporting by cross-checking against the
full `reviews(states: [APPROVED, CHANGES_REQUESTED])` connection on three PRs;
`totalCount` was 1 in every case. The glyph is not lossy on this board.

And the requested-reviewer list is mostly **teams**, which is what
`team-review-requested:` already turns into a whole board *section*. Rendering
`requested: web platform` under a PR that is sitting in the "My team's" section
restates the section header.

It also costs the most of any tier-(b) candidate: +6,565 bytes and **+1
rate-limit point** (connections cost more than scalars), for 1 PR in 50.

**Rejected.** If the user wants it anyway, §9 flags it as a decision — it is
cheap enough that "I want to see who approved it" is a legitimate reason to
overrule the measurement. But it should be a choice made knowingly, not a field
added because it was on the list.

### 5.4 What replaced it: unresolved review comments — accepted, top priority

While measuring reviewers I found the field that actually earns the line.
`reviewThreads` filtered to `isResolved == false && isOutdated == false`:

| PR | board's review glyph | unresolved threads |
|---|---|---|
| #3246 | `✗` changes requested | 1 |
| #3227 | `○` review required | 1 |
| #3186 | `○` review required | **9** |
| #3137 | **`✓` approved** | 2 |
| #3134 | **`✓` approved** | 1 |
| #3099 | `○` review required | 1 |

6 PRs have unresolved comments. **On 5 of those 6 the glyph does not say `✗`** —
and on two of them the board draws `✓ approved`, which reads as "done, merge it"
while two conversations are still open.

This is precisely the overlay's stated principle: it is a thing you can act on,
it is not visible anywhere else, and it is small (one line, a count). It is
strictly more useful than the reviewer names it displaces, and it comes from the
same on-demand request as `behindBy` — **no additional round trip**.

### 5.5 Size (`+N/−M`, `K files`) — accepted

+8,225 bytes as part of the scalar group. The board spans **1 to 379 changed
files** and **3 to 4,489 added lines**; median 9 files. 10 of 50 PRs are under 50
lines changed and 10 are over 1,000. That is the difference between "review this
now" and "block out an hour", and the board cannot show it. One line, three
numbers.

### 5.6 Branch names — accepted for base, conditionally; head last

Both are **already in the `PR` struct and unused in the UI** — tier (a), free.

The base branch is the interesting one. #3109 targets
`fix-PROJ-1839-reject-legacy-option`, not `master`. This board has a stacked-PR
tree (`DESIGN.md` §3.7) built from exactly this field, so "what is this stacked
on" is a question the tool already models and never states in words. Shown
**only when the base is not the repo default**, so it costs zero lines on the
~90% of PRs that target `master`.

The head branch is the string you need to check out. Real but weaker — it is
also derivable from the PR page you can open with `enter`. Kept, and first to be
dropped (§6).

### 5.7 Age — accepted in reshaped form

The row's `2h` is `updatedAt`. "How long has this been open" and "how long has
this been untouched" are different questions and the board answers neither
precisely. Measured: **14 of 50 PRs are >7 days old, 4 are idle >7 days**, max
age 15 days, max idle 14 days.

`createdAt` is one scalar in the group already being added. Rendered as one line,
and **only when it is worth saying** — a PR opened 2 hours ago suppresses the
line entirely, since the row's age column already covers it.

### 5.8 Labels — rejected

+2,597 bytes and **+1 rate-limit point** (a connection). Measured on the live
board:

- **18 of 50 PRs have no labels at all.**
- Of the labels that exist, `Webapp 🏎️` appears on **29 of 32 labelled PRs** —
  it is a near-constant, not a discriminator.
- The rest: `ready-for-e2e` (8), `dependencies` (4), `javascript` (3),
  `claude-review` (1), `github_actions` (1).

One label that is on 91% of labelled PRs and a long tail of ones that are not.
`dependencies`/`javascript` restate what the title already screams
(`chore: bump @types/send…`). Rejected: it costs a rate-limit point to render a
constant.

Worth noting this is board-specific. A repo using labels as a real workflow
signal would flip this, and the measurement is the thing to re-run — not the
conclusion to inherit.

### 5.9 The PR body — rejected, firmly

The single strongest thing the overlay must not become. The body is unbounded
multi-line Markdown: it needs wrapping, it needs truncation policy, it would
dominate the page, and on this repo it is frequently a PR template with unchecked
boxes. Rendering it is the first step toward a page you scroll, which is the one
property `checks-page.md` §4 and `uniform-rows.md` §3.2 both spent real effort
refusing. `enter` opens the browser; that is where prose belongs.

### 5.10 Total comment count — rejected in favour of §5.4

`totalCommentsCount` is free-ish and looks similar to unresolved threads. It is
not: measured max **34** comments on a PR, and only 1 of 50 PRs has zero. A
number that is nonzero on 98% of the board discriminates nothing, and it counts
resolved and outdated conversations — i.e. mostly things already dealt with.
**Unresolved** threads are nonzero on 12% of the board and every one of them is
live. Same neighbourhood, one is signal.

### 5.11 Merge-queue state — rejected

`isInMergeQueue` / `mergeQueueEntry`. **Zero PRs on this board are in a merge
queue**; the repo does not appear to use one. Adding a field that is constant
`false` on every row, and which contributed to the 504 in §2.3, is pure cost.

### 5.12 `mergeStateStatus` — rejected as a display field

Tempting because it is granular (`DIRTY`, `BLOCKED`, `BEHIND`, `UNSTABLE`,
`CLEAN`). But **35 of 50 PRs are `BLOCKED`**, which on this repo means "waiting
for a required review" — a fact `reviewDecision` already states. And its
vocabulary is GitHub-internal jargon the overlay would have to translate. The
two states it adds over what we have (`BEHIND`, and `DIRTY` disambiguated) are
better expressed by `behindBy` as an actual number. Rejected.

### 5.13 Linked issues — rejected

`closingIssuesReferences`. This team tracks work in **Jira**, not GitHub Issues —
the ticket is in the title (`PROJ-1373`, `PROJ-2038`) on most PRs, so it is already
on the board. A GitHub-Issues field would be empty on essentially every row.

### 5.14 Listing passing checks — still rejected

`checks-page.md` §1 settled this and nothing measured here disturbs it. 45% of
contexts pass and 47% are skipped; naming them buries the signal. The expansion
is a reason to re-affirm the principle, not to relax it.

---

## 6. Layout, degradation, and the short pane

### 6.1 Structure

The page is **two blocks separated by one blank line**, in strict priority order:

```
  <header: #number + title>

  <CHECKS BLOCK — unchanged from checks-page.md>
  ✗ failing gates, named
  ◐ pending gates, named
  ✓ tally

  <STATE BLOCK — new>
  ! conflicted / behind
  ● unresolved comments
  · author
  · size
  · base branch (only when non-default)
  · head branch
  · age

  any key closes
```

The checks block stays on top and stays exactly as specified in
`checks-page.md` §4 — it is what `c` was built for and what the user just called
"good". The state block is strictly below it. **No scrolling, no sections you
move between, no second key.** Any key still closes; a movement key still closes
and moves.

Glyphs are the existing vocabulary from `DESIGN.md` §3.2 — `✗` `◐` `✓` `!` `·`
in `error` / `attention` / `ok` / `muted`. **One glyph is added**: `●` (U+25CF,
`attention`) for unresolved comments. It is justified against the existing set —
it is filled where `○` "review required" is hollow, which reads as "there is
something here" against "this has not happened yet", and it is in the same
dingbat family as the rest. If even that is too much, `·` in `attention` works
and adds nothing new; this is a judgement call flagged in §9.

Labels in the state block are aligned to a fixed width, the same way
`helpOverlay()`'s `pad(label, 10)` already does it, so the values form a column.

### 6.2 Degradation order

`fitChecks()` currently clips the whole body from the bottom with `… N more
lines not shown`. That mechanism is kept and its reasoning is untouched. What
changes is that the state block must be clipped **before** the checks block,
never interleaved with it — otherwise a PR with 8 failing checks would drop the
failures to make room for its branch name, which inverts the page's whole point.

**Drop order, first dropped first:**

1. **head branch** — derivable by opening the PR
2. **age** — the row's age column is an approximation of it
3. **base branch** — but **never when it is non-default**; a stacked PR's base
   is load-bearing, so this entry means "drop the line if it was going to say
   `master` anyway", and in that case it was already suppressed
4. **size** — helps you plan, does not change what you do
5. **author** — the row's initials are a lossy version of it, but they exist
6. **unresolved comments** — the last of the new lines to go
7. **behind / conflict line** — the last state line standing
8. *then, and only then*, the existing `fitChecks()` clip eats the checks block
   from the bottom, exactly as it does today

The blank line between blocks is dropped when the state block is empty.

**At 14 rows.** Budget: 1 header + 1 blank + 1 blank + 1 footer = 4 chrome, plus
1 elision line when clipping — leaving **9–10 body lines**. On the worst live PR
(#3230: 4 failing, 2 pending, 1 tally = 7 check lines) that leaves 2–3 state
lines, which is items 1–2 of the priority list: the conflict/behind line and
unresolved comments. Exactly the right two. On an all-green PR the checks block
is 1 line and the whole state block fits with room to spare.

`fitChecks()`'s `chrome` constant goes from 6 to 6 — unchanged, since the state
block lives inside the same body budget rather than adding fixed chrome.

---

## 7. Rendering before the data arrives — **REVISED, see §7.1**

The on-demand request is **1.2–1.9 s**. The overlay must not wait for it.

- `c` opens the overlay **immediately**, rendered from tier (a)/(b) data only.
- The two on-demand lines (`behind`, `unresolved`) are **absent** until the
  response lands, then appear.
- They are absent, not a spinner placeholder: a line that appears is less
  disruptive than a line that changes content in place, and the overlay is
  frequently dismissed in under a second anyway.
- If the request fails or the overlay has already closed, nothing is shown and
  nothing is logged at the user. The page degrades to exactly today's behaviour
  plus the tier-(b) lines.
- **In-flight requests are dropped on close.** A user scrolling through 10 PRs
  with `c` at each must not queue 10 requests; the implementer should cancel or
  ignore all but the newest (see §10).

This is the one place the page is not instantaneous, and it is the price of
`behindBy` and unresolved threads. Both were judged worth it — they are the top
two items on the field list — but the *page* stays instant, which is the
property that matters.

### 7.1 Revision: there is a loading line, and the "absent" rule was wrong

The user, after running it:

> *"I do not see a loader in the details page, usually in the web we do a
> placeholder loading state for stuff like that."*

There **was** a loader — `reviewerLine` renders `…` — and it is invisible in
practice for two reasons the spec did not account for. `m.detail` is a cache
keyed by PR number, so the loading state only ever appears on the *first* open
of a given PR; and the reviewer line is the one field that is silent on an
unreviewed PR, so on a large share of the board there is nothing to hold a
placeholder at all.

**The rule above — "absent, not a spinner placeholder" — is reversed.** The
argument was that *"a line that appears is less disruptive than a line that
changes content in place"*. That is the wrong comparison. A line that appears
does not merely appear: it **inserts**, and `behind` and `unresolved` insert at
the *top* of the state block, so every line below them — author, review, size,
branch, opened — moves down a row. A placeholder that resolves in place changes
one line and moves nothing. The spec compared "appear" against "change" and
should have compared "push five lines down" against "change one line".

The user reporting it is the evidence. They were not asking for a spinner as
decoration; they were describing a page that visibly rearranged itself and said
nothing about why.

**What shipped: one skeleton line, in the on-demand lines' own position.**

```
  ! conflicted
  ⠹ checking for conflicts and open conversations      <- the skeleton
  author    Daniel (danielmar121)
  review    …
```

resolving to

```
  ! conflicted · 17034 commits behind master
  author    Daniel (danielmar121)
  review    ✓ Bar Spielberg (barspielberg)
```

**One line and not one per field**, which is the judgement call. Per-field
placeholders are the obvious reading of "web skeleton" and they are wrong here,
because both fields are conditional: unresolved threads exist on **6 of 50 PRs**
(§5.4), and `behindBy > 0` is common but not universal. Reserving a row for each
would draw two rows on most PRs that then vanish — trading a reflow on arrival
for a worse one on resolution, on more PRs. One line is what the group usually
resolves to, so the common case moves nothing at all.

Verified live against `#3220`, a PR not previously opened, in a 40×120 pane: the
loader animated for ~5 s and then `· 20 commits behind master` landed **on the
loader's own row**, with every other line byte-identical. A test asserts that
directly.

It is not free in every case. When the group resolves to zero lines, or to two,
or when `behind` merges into an existing `conflicted` line as it does on `#345`,
the block changes height by one and the lines below shift a row. That is the
same magnitude as the behaviour it replaces, and it now happens on the minority
of PRs rather than all of them.

The spinner also had to be taught to keep ticking: it stopped once the *board*
finished fetching, so a loader opened on a settled board would have been a
frozen glyph, which reads as stuck rather than as working.

---

## 8. Mockups, from live data

Captured 2026-09-17 from a real board; PR numbers, titles and people are
replaced with stand-ins throughout, the shapes and counts are as measured.

### 8.1 Failing — #3230, at 120 columns

Board row: `✗4 ○` — `dependabot`, no display name, 5 failing contexts (one is the
dropped `CI Gate` umbrella), 2 pending, 9 passing, 12 skipped.

```
  #3230 chore: bump @types/send from 0.17.4 to 1.2.1

  ✗ build-push-image customer-portal
  ✗ build-push-image billing-service
  ✗ build-push-image web-client
  ✗ build-push-image webapp
  ◐ webapp_e2e
  ◐ web_client_e2e
  ✓ 9 passing, 12 skipped

  author      dependabot
  size        +194 −143 · 1 file
  branch      dependabot/npm_and_yarn/types/send-1.2.1
  opened      1d ago

  any key closes
```

The checks block is byte-for-byte what ships today. Note `author` renders the
bare login — `dependabot` has no `name` — and there is no base-branch line
because it targets `master`. No conflict line and no comments line, because it
is mergeable with zero unresolved threads: **lines that have nothing to say are
not drawn.**

### 8.2 Conflicted, green CI, hidden comments — #3186, at 120 columns

The case that justifies the whole expansion. The board draws `✓ ○ !` — CI all
green, review required, conflicted. Everything below the first two lines is
currently invisible.

```
  #3186 fix(api-service): PROJ-1951 free unit numbers when a plan is abandoned

  ✓ all 18 checks passing

  ! conflicted · 26 commits behind master
  ● 9 unresolved comments
  author      Carol Diaz (cdiaz88)
  size        +596 −45 · 7 files
  branch     PROJ-1951-unit-number-integrity
  opened      6d ago · idle 1d

  any key closes
```

CI is green and the PR is still completely stuck: 26 commits behind, conflicted,
with 9 live conversations. **The old overlay showed one line here** — `✓ all 18
checks passing` — which is true and useless. This is the dead-end problem
`checks-page.md` §2.2 identified, one level up.

### 8.3 All green — #3253, at 120 columns

The "what does this show when nothing is wrong" case.

```
  #3253 fix(admin-console): Enable Ops Report Templates on root level, Attach fleetId filter   …

  ◐ run platform e2e
  ✓ 8 passing, 13 skipped

  author      Erin Walsh (erin-w-74)
  size        +3 −3 · 2 files
  branch     PROJ-2089

  any key closes
```

Approved, tiny, one e2e running. **No conflict line, no comments line, no age
line** — it was opened 9 minutes ago, so the row's `now` already says it. The
page is 10 lines. This is the answer to "what does it show when nothing is
wrong": *almost nothing, and quickly.* A green PR does not earn a full page just
because a full page exists.

### 8.4 Stacked, fully green — #3109, at 120 columns

```
  #3109 fix(api-service): forbid only the move that re-opens a cycle

  ✓ all 16 checks passing

  author      Bar Spielberg (barspielberg)
  size        +28 −4 · 2 files
  base         fix-PROJ-1839-reject-legacy-option
  branch        fix-PROJ-1850-seat-mapping-error
  opened      10d ago · idle 1d

  any key closes
```

The `base` line appears **only** because the base is not `master`. This PR is
stacked, the board draws it with `╰╴`, and this is the only place the tool says
what it is stacked *on*. 10 days old with everything green — visible here,
invisible on a row whose age column reads `1d`.

### 8.5 At 80 columns — #3186 and #3253

Only the title clips and the long branch name clips. The label column and every
value stay put; nothing reflows, nothing wraps.

```
  #3186 fix(api-service): PROJ-1951 free unit numbers when a plan is aba…

  ✓ all 18 checks passing

  ! conflicted · 26 commits behind master
  ● 9 unresolved comments
  author      Carol Diaz (cdiaz88)
  size        +596 −45 · 7 files
  branch     PROJ-1951-unit-number-integrity
  opened      6d ago · idle 1d

  any key closes
```

```
  #3253 fix(admin-console): Enable Ops Report Templates on root level,  …

  ◐ run platform e2e
  ✓ 8 passing, 13 skipped

  author      Erin Walsh (erin-w-74)
  size        +3 −3 · 2 files
  branch     PROJ-2089

  any key closes
```

### 8.6 At 80 columns, 14 rows — #3230 clipped

The degradation case. 14 rows, 4 chrome lines, 1 elision line → 9 body lines.

```
  #3230 chore: bump @types/send from 0.17.4 to 1.2.1

  ✗ build-push-image customer-portal
  ✗ build-push-image billing-service
  ✗ build-push-image web-client
  ✗ build-push-image webapp
  ◐ webapp_e2e
  ◐ web_client_e2e
  ✓ 9 passing, 12 skipped

  author      dependabot
  … 3 more lines not shown

  any key closes
```

The checks block survives whole, the tally still reconciles, `author` is the one
state line that fits, and the elision is honest about the three dropped. Drop
order from §6.2 removed head branch, age, and size — in that order.

---

## 9. Decisions for the user

1. **Reviewer names are rejected on the measurement (§5.3) — overrule me if you
   want them anyway.** On 49 of 50 PRs the glyph says everything the names
   would, requested reviewers are overwhelmingly *teams* (which the board's
   sections already express), and it costs +6,565 bytes and a rate-limit point.
   But "I want to see who approved it" is a preference, not an error, and the
   cost is affordable. Say the word and it becomes one more line under `author`.
2. **The name of the page, and whether `c` is still right.** It is no longer the
   checks page. I recommend **keeping `c` and calling it the detail overlay** —
   `c` is in muscle memory and in the `?` legend, and the checks block is still
   the top of the page, so the key still means what it did. A second key was
   considered and rejected: two keys for one PR means choosing which page you
   want before you know what is wrong with it, which is exactly the dead-end
   `checks-page.md` §2.2 removed. The `?` legend entry should change from
   `checks for this PR` to `detail for this PR`.
3. **`●` as a new glyph** for unresolved comments (§6.1). It is the only addition
   to the visual vocabulary in this spec. `·` in `attention` is the
   zero-new-glyph alternative. My preference is `●`; it is a weak preference.
4. **The 1.4 s on-demand fetch.** §7 keeps the page instant and lets two lines
   arrive late. If you would rather the overlay never showed a line that appears
   after the fact, drop `behindBy` and unresolved comments — the page still works
   and is still better than today, but it loses its two best fields.

---

## 10. What the implementer will need to decide

- **Detecting "the default branch".** §5.6 shows the base line only when the base
  is not the repo default. `master` is hardcoded nowhere sensible; the honest fix
  is `repository { defaultBranchRef { name } }` on the search query (one field,
  unmeasured — measure it before adding, per §2.3) or a config value. Comparing
  against the literal `"master"` works for this repo and is wrong in general.
- **Cancelling in-flight detail requests.** Holding `j` with the overlay
  reopening must not queue N requests. Cancel-on-close, or tag responses with the
  PR number and drop stale ones. Either works; the Bubble Tea `tea.Cmd` idiom
  makes the tag-and-drop version simpler.
- **`reviewThreads(first: 100)` is unpaged**, the same pre-existing ceiling
  `checks-page.md` §6 flagged for `contexts(first: 100)`. Max observed is 9
  unresolved (and the connection returns all threads, not just unresolved, so the
  real ceiling is total threads). Not worth paging; worth knowing.
- **Where the search interacts.** Settled since: search matches only what the
  row draws, which is the three-character initials cell. `author.name` is
  deliberately not matched — a hidden term no column shows cannot be
  highlighted, and an unhighlightable match reads as a bug. `DESIGN.md` records
  the rule.
- **Whether `aheadBy` is worth rendering.** The `compare` call returns it for
  free. I left it out: "3 commits ahead" is not a question anyone asks. Available
  if it proves otherwise.
- **The exact idle-line threshold.** §5.7 says "only when worth saying". I did
  not pick a number. Something like: show `opened Nd ago` when age > 24 h, append
  `· idle Nd` when idle > 48 h. Tunable, and the measurements in §5.7 are the
  data to tune against.

---

## 11. What I could not verify

- **Whether the user wants any of this more than they want the page to stay
  small.** Every field here was justified against measured data, but the brief's
  own framing — *"checks is good"* — describes a page that already works. The
  risk this spec carries is that a good page becomes a busy one. §6.2's drop
  order and §8.3's 10-line green page are the mitigations; whether they are enough
  is a preference I cannot measure.
- **Board shapes other than this repo's.** Every ratio here is from
  `acme/monorepo`: dependabot-heavy, Jira-tracked, single-reviewer,
  team-review-requested, one dominant label. The reviewer rejection (§5.3) and
  the label rejection (§5.8) are the two most board-dependent conclusions in this
  document and would plausibly flip on a repo with genuine multi-reviewer PRs or
  a real label workflow. The measurements are the thing to re-run.
- **The 504 boundary.** §2.3 records that a kitchen-sink query times out and that
  the recommended set does not, three times. I did not bisect to find exactly
  where the line is, so I cannot say how much headroom the recommended set has.
  This is the main reason §10 says to measure any later addition rather than
  assume it is free.
- **`git merge-tree` as ground truth.** §4.2 treats it as the correct answer. It
  is git's own merge, so it is correct for a merge commit — GitHub's merge
  strategy (squash, and any `.gitattributes` merge drivers) could in principle
  differ. The 3×–8× over-report is far too large for that to change the
  conclusion, but the exact conflict counts are "what git says", not "what the
  GitHub merge button would say".
- **Latency under a cold cache or a slow network.** All timings are from one
  machine on one connection in one session, and GitHub's search endpoint is
  visibly noisy — the *same* baseline query ranged 4.9–9.3 s across this session.
  The +7.3% payload claim is solid; the latency claims are indicative.
- **Whether unresolved threads stay a 12% signal.** 6 of 50 PRs is a small
  absolute number, measured once. The *asymmetry* it exposes (5 of 6 invisible on
  the board) is the finding, and that is structural rather than statistical — but
  the 12% rate itself is one sample.

---

## 12. Implementation record

Shipped on `feat/pr-detail`. §3, §6, §7 and §8 are implemented as specified,
with one accepted change to §5.3 and the §10 decisions resolved below.

### 12.1 Reviewer names: accepted, and they cost nothing

The user overruled §5.3's rejection — *"I do like it, can we just pull it only
when we enter this view?"* — and that reframing removes the entire objection.
§5.3 costed reviewer names **on the search query**: +6,565 bytes and +1
rate-limit point on every board fetch, for a field useful on 1 PR in 50.

Moved into the §7 on-demand request instead, they were re-measured live:

| | bytes | cost | latency |
|---|---|---|---|
| tier-(c) request as specified | 912 | 1 | 1.2–1.9 s |
| **+ `latestOpinionatedReviews`** | ~1,400 | **1** | **1.7 s** |

**No extra rate-limit point and no extra round trip.** The connection is cheap
against a single PR and expensive against 50; the spec measured the wrong one of
those because it had assumed tier (b). The board fetch is untouched.

The line also turns out to earn more than §5.3 credited. `reviewDecision` says
*an* opinion exists; the name says *whose*, which is who you go talk to. On
#3246 the board draws `✗` and the overlay now says `✗ Alice Chen (alicechen)`.
Requested reviewers are still **not** carried — 17 of 50 are teams, and naming
one under a PR sitting in the "My team's" section restates the section header.

Measured while re-verifying: the recommended tier-(b) set costs **1 point, not
the 2 §2.2 reported**, at 188,254 bytes over three runs with no 504.

### 12.2 The four decisions §10 left open

**Detecting the default branch — from the repo, via the request already being
made.** `repository { defaultBranchRef { name } }` rides the on-demand query at
no measurable cost, so the literal `"master"` §10 warned about is nowhere in the
code. The consequence is that the `base` line waits for the response rather than
guessing: before it lands the overlay cannot know whether a base is worth
naming, and silence is better than a line that might be a constant. Putting it
on the search query was rejected — §2.3's 504 says every field there must be
paid for, and this one would be re-fetched 50 times to answer once per repo.

**In-flight requests — tagged, not cancelled.** Responses carry their PR number
and file themselves into a map keyed by it, so a late response can never
overwrite the overlay you are looking at; it is simply kept for the PR it
describes. Holding `j` with `c` leaves requests in flight that nobody is waiting
for, which costs a rate-limit point each and nothing else. A PR already answered
or already asked about is not re-requested, which is what actually bounds the
storm. Verified by sending 15 `c`+`j` pairs as fast as tmux will deliver them:
board stable, cursor intact, no stuck overlay. This is the `tea.Cmd`-idiomatic
version §10 predicted would be simpler, and it is.

**The filter now matches `author.name` — yes.** `haystack` carries login and
display name together, so `Carol` finds `cdiaz88`. The row shows three
initials of either, so both are things a person would plausibly type.
`titleOffset` is computed from the same string, so highlight positions still
translate correctly; the existing filter tests cover that and pass unchanged.

**`aheadBy` — not rendered.** Measured across the six PRs in §8: it is 1–4 on
every one of them, including PRs that are 0 behind and perfectly clean. A field
that is nonzero on essentially every row discriminates nothing, which is the
same argument §5.8 used to reject labels and §5.10 used to reject the total
comment count. §10 was right to leave it out.

**The idle threshold** is §10's suggestion unchanged: `opened Nd ago` above 24 h,
`· idle Nd` appended above 48 h.

### 12.3 What the real terminal caught that the tests did not

The overlay ended with a trailing newline — 15 emitted lines in a 14-row pane —
so the terminal scrolled and **ate the `#3230` header line**. Every assertion
passed throughout, because they measured the trimmed line count. It was visible
in the first real 120×14 frame.

This is the §8.6 case the drop order was designed for, and the defect was in the
one line the drop order never touches. The tests now count lines as emitted
rather than trimmed, at six heights and three widths. Recorded because it is the
second time on this project that a static check agreed with a broken session.

### 12.4 Verified in a real session

Live board, `acme/monorepo`, real API, tmux panes at 120×40, 120×30,
120×14 and 80×24:

- **#3186** (§8.2, conflicted + green CI + hidden comments) — `! conflicted ·
  26 commits behind master`, `● 9 unresolved comments`, matching the API
  exactly. The case that justifies the expansion, and it reads as intended.
- **#3246** (§8.1, failing) — 4 gate lines, tally, and the reviewer loader
  resolving in place to `✗ Alice Chen (alicechen)`.
- **#3253** (§8.3, all green) — 11 lines. No conflict, comment, age or base
  line. `✓ WalterN12` renders bare, as a null display name should.
- **#3211** — a stacked PR, so the `base` line appears: the §5.6 case.
- **#3230 at 120×14** (§8.6) — all 6 check lines and the tally survive whole,
  the state block clips to `· 6 commits behind master` with `… 4 more lines not
  shown`, footer intact, exactly 14 lines.
- **#3186 at 80 columns** (§8.5) — only the title clips; the label column and
  every value stay put, max line width 78.
- **The in-flight frame**, which is what the user asked about: every tier-(a)/(b)
  line is already readable at ~120 ms with `review    …` holding its place, and
  the name lands ~1.7 s later without moving anything around it.
- `j` from an open overlay closes it and moves in one keypress, at every size.
