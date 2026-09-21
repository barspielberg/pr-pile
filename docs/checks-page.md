# pile — what belongs on the checks page

Research and implementation record, written in response to:

> *"now that we have a dedicated page for that, maybe we want to show also the
> success and in-progress ones? Let the agent research and make its mind."*

Researched 2026-09-17 against the shipped code at `48b085d` and against 30 live
PRs captured from `acme/monorepo` (731 check contexts).

---

## 1. Decision

**Partial yes, and the interesting half of the answer is the half that was not
asked about.**

1. **In-progress checks: yes, named.** This is the change that earns its keep.
2. **Passing checks: no, not named.** They collapse into a count.
3. **Skipped checks: no, not even named as a bucket.** Counted only.

The overlay now reads:

```
  #3230 chore: bump @types/send from 0.17.4 to 1.2.1

  ✗ build-push-image customer-portal
  ✗ build-push-image billing-service
  ✗ build-push-image web-client
  ✗ build-push-image webapp
  ◐ webapp_e2e
  ◐ web_client_e2e
  ✓ 9 passing, 12 skipped

  any key closes
```

The principle: **the overlay names what you can act on and counts what you
cannot.** A failing check is a thing to go fix. A running check is a thing to
go wait for — and knowing *which* one tells you whether that is 30 seconds or
20 minutes. A passing check is not a thing at all; its only job is to be
accounted for, which a number does as well as a list.

---

## 2. The measurements that decided it

30 open PRs, every `statusCheckRollup` context, taken live.

### 2.1 The board is mostly noise by volume

| state | contexts | share |
|---|---|---|
| SKIPPED | 346 | **47%** |
| SUCCESS | 328 | 45% |
| FAILURE | 28 | 3.8% |
| PENDING | 27 | 3.7% |
| CANCELLED | 2 | 0.3% |

**92% of all check contexts are things that are fine or never ran.** The median
PR carries 26 contexts; the largest carries 41.

This is the whole argument against listing passes. On the worst PR measured
(#3229: 31 contexts, 7 failing) a full list would put 7 signal lines under 24
lines of noise, and the overlay would no longer fit a 20-row pane — the design
target. The brief asked whether 40 passing checks bury the 2 that matter. On
this repo the answer is measured: **yes, by roughly 3 to 1.**

Worse, skipped is the *largest* single bucket, and it is the least meaningful
one. `apps_ci / Deploy` being skipped means a path filter did not match. It
is a fact about the workflow YAML, not about the pull request.

### 2.2 Nearly half the board has nothing failing at all

| rollup state | PRs | what `c` used to show |
|---|---|---|
| PENDING | **14 of 30 (47%)** | *"no failing checks"* |
| FAILURE | 9 | the gate names |
| SUCCESS | 6 | *"no failing checks"* |
| none | 1 | *"no failing checks"* |

**This is the finding that changed the design.** The overlay existed to answer
"why is CI red", and on 21 of 30 PRs it answered with a dead end — a keystroke
that opened a page to tell you nothing. The brief framed the question as
*additive* ("show more?"); the data says the feature was *incomplete*.

And the pending names are cheap and specific. Every PENDING context on the whole
board resolved to one of three jobs:

```
 13  webapp_e2e
 11  run platform e2e
  3  web_client_e2e
```

Never more than 3 per PR, usually 1. So pending names cost almost nothing in
lines and answer a real question — *e2e is running, that is ten minutes, go do
something else* versus *lint is running, wait for it*.

The asymmetry is the design, and it is stark:

| bucket | lines on a typical PR | lines on an all-green PR |
|---|---|---|
| pending | 1–3 | 0 |
| passing | — | **22 distinct names** |

Pending is small and specific. Passing is large and uniform. They do not deserve
the same treatment just because both are "not a failure".

### 2.3 The data was already on the wire

The brief flagged this as the load-bearing question: if we do not fetch passing
and pending names, is adding them worth the query cost?

**There is no query cost.** `searchQuery` already requests
`contexts(first: 100)` with `name` and `conclusion`, and `toPR()` threw away
every non-failure client-side. The bytes were already being paid for and
discarded.

One field was genuinely missing: `CheckRun.status`. A check that is still
running reports `conclusion: null`, so conclusion alone cannot distinguish
"running" from "no result". Adding `status` to the selection set is one scalar
on a node we already fetch — no extra round trip, no extra rate-limit cost, and
no measurable payload change against a 28-node context list.

So the honest cost is: **one scalar field, zero extra requests.**

---

## 3. What comparable tools do

| tool | on a checks view | default |
|---|---|---|
| `gh pr checks` | every check: pass, skip, pending. Sorted failures-first. `--required` narrows. Closes with a tally: `X cancelled, Y failing, Z successful, A skipped, and B pending` | show all |
| **gh-dash** sidebar | lists individual names **including passing**, grouped by status with counted headers (`Awaiting Approval (2)`) | show all |
| **gh-dash** overview | the same data as counts only — `3 failing`, `2 in progress`, `5 successful` + a proportional bar | counts |
| **lazygit** (≥ v0.64.0) | rollup state only — a single ✓/✗ per branch, **never any names**, links out to GitHub | counts |

**gh-dash's sidebar is direct evidence against this decision and is worth
stating plainly**: a close cousin of this tool lists passing checks by name and
has no open issue complaining about it.

Two things stop that from overturning the measurement. First, gh-dash's sidebar
is a **persistent pane occupying 45% of the width** with its own scrollback — it
is a place you dwell, so a long list costs nothing you were not already
spending. Our overlay is a modal that any keypress dismisses, sized to a 20-row
pane; it is far closer to gh-dash's *overview* than to its sidebar, and the
overview is counts. Second, gh-dash is repo-agnostic. A repo with 6 checks loses
nothing by listing them. This repo has 26.

The spread across these four is the real signal: from lazygit's counts-only to
`gh`'s everything, **there is no industry default to inherit**. The choice has
to come from the board in front of us, which is why §2 is measured rather than
argued.

One thing worth stealing, and stolen: `gh pr checks` closes with a tally that
**reconciles** — the named lines plus the counted ones account for every check.
That is why the summary reads `9 passing, 12 skipped` rather than a bare
`9 passing`. Without the skipped count the numbers do not add up against
GitHub's own UI, and a reader who notices is left wondering what was hidden.

---

## 4. Ordering, grouping, and the pane

**Order is failing → pending → tally.** Fixed buckets, not interleaved. The list
is read top-down and the top is what you pressed `c` for. Within a bucket the
API's own order is kept, which groups a workflow's jobs together — an
alphabetical sort would interleave `apps_ci` jobs with unrelated ones and
break the visual run of `build-push-image *` that makes a dependabot failure
legible at a glance.

**Overflow clips, it does not scroll.** The overlay can now exceed the pane: 7
failing + 3 pending is fine, but nothing bounds a pathological PR. Three options
were considered:

- **Scroll** — rejected. The overlay's defining property is that *any key
  dismisses it*. Scrolling needs keys that mean "move within the overlay", which
  is exactly the mode the design spent `docs/uniform-rows.md` §3.2 avoiding. It
  would make `j` ambiguous: move the list, or close and move the board?
- **Collapse the failures too** — rejected. That deletes the only thing the page
  is for.
- **Clip from the bottom, with an honest notice** — chosen. The list is already
  ranked by what you came to read, so the last line is the least valuable. The
  elision line says `… N more lines not shown`, so the page never silently lies
  about being complete, and the `any key closes` footer always survives.

A PR failing more checks than fit a pane is one you open in a browser anyway.

---

## 5. What this costs

- **A passing check's name is no longer reachable from this tool.** If you want
  to confirm that `lint-typecheck-test webapp` specifically ran and passed,
  `c` will not tell you — only that 9 things passed. This is the real regression
  and the deliberate one. `gh pr checks <n>` and the browser both answer it.
- **Skipped checks are invisible beyond a count.** If a job you expected to run
  was skipped by a path filter, the overlay shows it in the skipped tally and
  nowhere else. This is a plausible debugging need and it is not served.
- **The umbrella-gate exclusion now skews a number, not just a list.**
  `gateName()` drops `CI Gate` and `E2E Status` because they only restate their
  children. That was invisible when it only affected which names were listed;
  now it means `PassedCount` can read one or two below what GitHub's own UI
  shows. The tally reconciles against the *contexts we consider*, not against
  GitHub's raw total. Verified on #3230: we report 4+2+9+12 = 27 against
  GitHub's 28, the difference being exactly one dropped `CI Gate`.
- **One more line on a green PR than strictly needed.** `all 20 checks passing`
  could arguably be pushed onto the row itself. It is not, because the row has
  no space and `✓` already says it.

---

## 6. What I could not verify

- **Whether the user actually wants pending names.** The measurement says the
  old overlay dead-ended on 47% of the board; it does not say the user ever
  pressed `c` there. This is the assumption the whole change rests on.
- **Board shapes other than this repo's.** Every ratio in §2 is from
  `acme/monorepo`, which is dependabot-heavy and has an unusually large
  skipped bucket (47%) because of a monorepo path-filter matrix. A 6-check repo
  would find the collapse pointless — though not harmful, since a 6-check green
  PR renders as one line either way.
- **The 100-context ceiling.** `contexts(first: 100)` is unchanged and unpaged.
  No PR measured came close (max 41), but a PR exceeding it would silently
  undercount the tally. Not fixed here; it was pre-existing and is not made
  worse by this change, since the failure list had the same ceiling.
- **Whether `NEUTRAL` should count as passing.** It is counted as passing on the
  grounds that it does not block a merge. No PR on this board used it, so this
  is reasoned, not observed.
- **k9s and other TUIs.** I looked for an established "N passed" collapse
  convention outside the GitHub tooling above and found none documented. Absence
  of evidence.
