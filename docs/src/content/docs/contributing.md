---
title: Contributing
description: How to build and test pile, a map of the packages, and the conventions a change should follow.
---

pile is a single Go binary with a small codebase and a large test suite. This
page is what you need to make a change: how to build and test, where things
live, and the conventions the repo holds to.

## Build and test

There is no code generation and no special toolchain. Go 1.24+ is the only
requirement for the binary; the docs site is a separate Astro project and is
not part of the Go build.

```sh
make build      # go build -o pile ./cmd/pile
make test       # go test ./... -short
make check      # build + test + go vet ./...
```

`make check` is the gate; it is what `make install` runs before putting the
binary on your `PATH`. The docs build (`cd docs && pnpm build`) is
deliberately kept out of the Go path so a contributor never needs Node to work
on the tool.

To work on the docs, install Node.js 24+ and the pnpm version pinned in
`docs/package.json`, then run:

```sh
cd docs
pnpm install --frozen-lockfile
pnpm dev       # local preview at http://localhost:4321/pr-pile/
pnpm build     # production site in dist/, including the search index
```

Commit `pnpm-lock.yaml` when changing dependencies.

## Package map

| package | what it does |
| --- | --- |
| `cmd/pile` | the entrypoint: load config, build the client, run the reachability check, start the TUI |
| `internal/config` | the rule list, config loading, the starter file, `PILE_*` env vars |
| `internal/github` | the GraphQL client: `Search`, `Detail`, and the `CheckRepo` startup check |
| `internal/board` | turns per-rule results into ordered sections, first-match-wins bucketing, tree grouping |
| `internal/ui` | the Bubble Tea model, rendering, search, the detail overlay, the help page |
| `internal/browser` | `open` in the default browser |

The dependency graph is one direction: `cmd` → `internal/*`, and `internal/ui`
reads `internal/board`, `internal/config` and `internal/github` but not the
other way around.

## Tests

Tests outnumber code (about 6,600 lines of tests to 4,400 of code), most of it
in `internal/ui` — the rendering and scrolling are where the subtle bugs are,
so that is where the assertions are.

Two tests are load-bearing in a different way: they tie the docs to the code.

- `internal/config/docref_test.go` reads every `yaml:` tag on `Config`, `Rule`
  and `Action` by reflection and fails if a key is not mentioned on its
  reference page. Add or rename a config key and the test tells you the docs
  drifted.
- `internal/ui/keysref_test.go` reads the help page's `KEYS` block and fails if
  a binding is not on the keys reference page. Ship a key without documenting
  it and the test fails.

If you change behaviour, update the corresponding page and run `make check` —
those two tests are the backstop, not a substitute for reading what you wrote.

`internal/github/live_test.go` hits the real API and is not part of `-short`.
Run it deliberately with `PILE_REPO` set and `-run TestLive`; it skips itself
when there is no token or the repo is unreachable.

## Conventions

- **Order is meaning.** A PR is shown under the first rule that matches it, and
  rules are fetched in parallel but revealed in order. Most changes to
  `internal/board` touch one of those two properties — read
  [Rules and ordering](../guide/rules/) before changing either.
- **Colour is a budget.** New colour must pass the
  [principles](../design/) checklist, and the visual spec is the maintained
  record of what each token means. There is a test that fails if a fixed
  256-cube colour reappears.
- **Commit subjects and PR titles** use Conventional Commits, as described
  below. Keep the description imperative and explain the change.
- **Docs live beside the change.** Behaviour that a user reads about is
  checked against the code; behaviour that only the design research discusses
  belongs in the [research archive](../design/research/), prefaced as history.

## Commit subjects and PR titles

New commit subjects and all PR titles must follow:

```text
type(scope)!: description
```

The scope and breaking-change `!` are optional. Allowed lowercase types are
`feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`,
`chore`, and `revert`. Scopes start with a lowercase letter or digit and may
contain lowercase letters, digits, `.`, `_`, `/`, and `-`. Use exactly one space
after the colon and a nonempty description with no leading/trailing whitespace
or control characters. Commit bodies and trailers are unrestricted.

Examples:

```text
feat: add repository filters
fix(ui): keep selection after refresh
docs(contributing): explain local hooks
refactor(config)!: remove the legacy schema
```

`Add filters`, `Fix: handle refresh`, and `fix(ui):missing space` are rejected.
Use `!` for a breaking change, optionally explaining it with a `BREAKING CHANGE:`
trailer in the body.

### Install the local hook

With Go on `PATH`, run from your checkout:

```sh
git config --local core.hooksPath .githooks
```

The executable `commit-msg` hook runs the shared Go validator on the first line
of each new commit message, including amended messages. No Node packages or
additional tooling are needed. This setting replaces any existing hooks path;
if you already use hooks, integrate `.githooks/commit-msg` into your existing
`commit-msg` hook instead. Each checkout needs setup. Linked worktrees normally
share local Git configuration, so coordinate that setting across worktrees.
To remove this setup, run `git config --local --unset core.hooksPath` (or restore
your previous value).

To check a message file or run the policy's regression tests manually:

```sh
go run ./tools/commitlint --commit-msg /path/to/message
go test ./tools/commitlint
```

### Squash merges and CI

We squash and merge, using the PR title as the final commit subject. The
`Conventional PR title` check validates the current title on PR open, reopen,
new commits (`synchronize`), and edits (including title changes). It reads the
title as JSON data from the event file; title text is never evaluated as shell
code. The workflow uses unprivileged `pull_request`, read-only contents access,
and checkout without persisted credentials.

CI deliberately does not lint the individual commits in a PR or its base
history. Historical commits and existing commits in the packaging PR #9 are
grandfathered; do not rewrite them just to pass this policy. Use the hook for
new work and ensure the PR title describes the complete squash. Local hooks
can be bypassed, so they provide early feedback rather than a remote guarantee.
Maintainers must preserve the validated title as the squash commit subject
when merging.

A maintainer should require **Conventional PR title** in the branch protection
or ruleset for `main` after the workflow first runs. This change does not modify
repository settings. CI cannot prevent a maintainer from manually replacing
the squash subject with an invalid one in the merge dialog.
