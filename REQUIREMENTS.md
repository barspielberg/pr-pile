# prs-mng — requirements (draft)

A terminal PR board. Opens instantly in a Ghostty quick terminal (`cmd+\``), shows every PR I care about with enough state to decide what to do, and gets out of the way.

Working name: `prs-mng`, invoked as `prs`.

## Why not what I already have

- `pr-status.sh` (bash + jq) renders my own PRs well — stacked-PR tree, CI rollup, failing gate names — but it is a read-only board for one bucket. No navigation, no actions, no review queue.
- `gh-dash` has the sections and keybindings, but its filters are GitHub search strings, and the bucket I actually want could not be expressed: `review-requested:@me OR involves:@me` is not valid GitHub search syntax, so it became two overlapping sections showing the same PRs twice.

The fix is client-side bucketing: let GitHub do the narrowing it is good at (one search per rule), then merge and dedupe in code, where boolean logic is free. Not one firehose fetch of every open PR.

The transport is **GraphQL, not `gh`** — see [Measured](#measured-monorepo-syb-2026). `gh search prs` cannot return `statusCheckRollup`, `reviewDecision`, or `mergeable`, which are most of the board.

## Shape

Go + Bubble Tea, same stack as gh-dash. Single static binary, no runtime deps beyond `gh auth token` for credentials. Talks to the GitHub GraphQL API over HTTP directly — not by shelling out to `gh` per rule.

## Rules

There are no built-in sections. Everything is a **rule**, the user lists them in order, and a PR shows up under the first rule that matches it. "Mine" and "Review requested" are not special — they are just the first two rules in the default config.

A rule is:

- a **name** (the section header)
- a **matcher** — what puts a PR in it
- optional **filters** on top (`draft: false`, `-author:@me`, …)

Default rule list, top to bottom:

1. **Mine** — `author:@me`. Stacked ones grouped as a tree, as today.
2. **Review requested** — review explicitly requested from me.
3. **My team's review** — review requested from a team I belong to, not from me personally. This also covers the CODEOWNERS case: GitHub already turns CODEOWNERS into team review requests, so there is no need to match paths myself.
4. **Teammates** — authored by someone on a configured teammate list (or GitHub team).
5. **Involved** — I commented, was mentioned, or already reviewed it.
6. **All open** — everything else, as a fallback.

Semantics:

- Each rule is named and toggleable; a row shows *why* it matched (e.g. `team-review`).
- A PR appears exactly once — first matching rule wins, in list order.

Because a PR appears only once, ordering is the whole configuration. `Involves:@me` would otherwise swallow my own PRs; it does not, because `author:@me` is above it. No special-casing needed — reorder to change the meaning.

## Row content

Port everything `pr-status.jq` already does:

- number, title (truncated on display cells, not bytes)
- CI rollup: passing / N running / N failing, with failing **gate names** on a continuation line (dedupe sharded jobs, drop rollup-of-rollup checks)
- review decision: draft / approved / changes requested / review required
- mergeable: conflicts marker as its own column
- stacked PRs grouped bottom-up with tree glyphs

Needs a Nerd Font, same as today.

## Actions

Everything except "open in browser" is a **configurable shell command** with template variables — `{{.Number}}`, `{{.Repo}}`, `{{.RepoPath}}`, `{{.Branch}}`, `{{.URL}}`, `{{.Author}}`. The tool does not know about worktrees, editors, or multiplexers; it just runs what I tell it to.

Built in:

- **open in browser** — the `open-url` logic moves *inside* the tool: look for an existing Arc tab with that URL and focus it, otherwise `open`. Other browsers fall through to `open` today; pluggable later.

Configured by me, as examples of what config must support:

- open a worktree the way I want (`wt switch -x nvim pr:{{.Number}}`, or the herdr script)
- open a terminal review session (`tuicr pr {{.Number}}`)
- anything else

Each command declares: key, display name, and whether it should suspend the TUI, run in background, or open a new window/pane.

Write actions (approve, comment, re-run CI, mark ready) — not in v1, but the action model should not make them awkward to add.

## Interaction

- Cold start is ~2-4s and there is no way around it — gh-dash spins and k9s splashes for the same reason: the GitHub search backend is the floor, not their implementation. So spend the effort on *felt* latency instead.
- **Reveal sections in order, as a prefix.** Fetch in parallel, but a section can only be drawn once every section above it has resolved. First-match-wins means a later section's contents are not knowable until the earlier ones have claimed their PRs — drawing section 2 early would risk showing a PR that section 1 is about to take, then yanking it out from under the cursor.
- So the board fills top-down: section 1, then 2, then 3. A section that arrives early just waits its turn. Sections below the frontier show a spinner, not an empty list.
- This still buys most of the felt-latency win, because section 1 is `mine` — the smallest, fastest query, and the one I usually opened the tool for. Measured, it came back in 1.4s against 3.1s for `allopen`.
- Never a blank frame or a fake "no open PRs" on cold start: a section that has not resolved yet says so.
- Auto-refresh on an interval, `r` to force.
- Navigate sections and rows with vim keys; `q` quits.
- Repaint on SIGWINCH without refetching.
- Optional detail/preview pane for the selected PR (body, files, reviewers) — decide later whether rows stay rich or go compact with detail in the pane.

## Config

One file (`~/.config/prs-mng/config.yml` or TOML), covering:

- repos to watch — explicit list, not cwd inference. `pr-status.sh` learned this the hard way: cwd inference silently showed the dotfiles repo.
- repo → local path mapping, for action templates
- rule list: name, matcher, filters, order
- teammate list / teams, owned path globs
- keybindings → commands
- refresh interval, theme/glyphs

## Decided

- **Single repo first** (`acme/monorepo`). Config takes a repo, not a list. Do not bake the assumption in deeper than that.
- **No caching.** Cold start shows a spinner/splash and fetches. Measured at ~2–4s (see below), so the spinner matters. Revisit caching if that grates.
- **No CODEOWNERS path matching.** "Review requested from my team" covers the ownership case well enough, and it is free — GitHub derives it from CODEOWNERS already. No per-PR file-list fetch.

## Measured (monorepo, Sep 2026)

Timed against `acme/monorepo` with the real rule set. Three findings, two of which change the design:

**1. `gh search prs` cannot return the columns we need.** It supports `author`, `title`, `isDraft`, `updatedAt`, `labels` — but *not* `statusCheckRollup`, `reviewDecision`, or `mergeable`. Those are the CI, review and conflict columns, i.e. most of the board. `gh pr list` returns them but cannot express the rule queries (no `review-requested:`, no `team-review-requested:`). So neither `gh` subcommand alone can do this. **Go straight to the GraphQL API** with `gh auth token` for credentials — one `search(type:ISSUE)` per rule returns every field in one shot, verified working.

**2. Shelling out to `gh` costs ~1.7s before any work happens.** A trivial `gh api rate_limit` takes 1.7–2.4s; the same call over raw curl is ~0.45s warm. That is process startup plus auth, paid per invocation. Another reason to talk HTTP directly from Go rather than shelling out per rule.

**3. Parallel beats combined, and the cap does not matter.** Six rules:

| approach | wall clock |
|---|---|
| 6 parallel GraphQL requests | 1.9s / 3.3s / 4.2s |
| 1 combined GraphQL request (6 aliased searches) | 3.5–7.7s |

The combined query is *slower* — GitHub appears to run aliased searches serially, so one request pays the sum. Parallel pays the max. Varying the cap (5 / 10 / 20 / 50 rows) moved nothing: limit=5 took 5.7s and limit=20 took 2.9s in the same batch. **Latency is per-request overhead and search backend variance, not payload size**, so capping rows is not a lever worth pulling — set limits for display sanity, not speed.

Caveat: run-to-run variance is large (the same query ranged 1.4s–4.1s), so these are order-of-magnitude, not precise. GitHub's search backend is just noisy.

**Conclusion:** parallel GraphQL, ~2–4s cold. Too slow to feel instant in a quick terminal, which makes the splash/spinner load-bearing rather than cosmetic. If it grates in practice, caching is the fix — not fewer rows.

## Open questions

- Is ~2–4s tolerable behind a spinner in daily use? Decide after living with it.
## TODO — later

**Review progress.** The main gap versus today's board — knowing *why* a PR is stuck:

- who has approved / requested changes / is still pending (avatars or initials)
- count of unresolved review threads
- for my PRs: is it waiting on me (comments to address) or on them (nobody reviewed yet)

Deferred because it is the one thing `gh pr list` does not give us, so it drags in a GraphQL query or a per-row lazy fetch. Decide after the basic board works.
