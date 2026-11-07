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
- **Commit style** follows the existing history: imperative, one line, no scope
  prefix (`Add the review TUI guide`, `Rewrite the README`). A commit message
  that names a fixed value or a count is a claim about one tree at one moment —
  put facts the code enforces in a test or a table, not a message.
- **Docs live beside the change.** Behaviour that a user reads about is
  checked against the code; behaviour that only the design research discusses
  belongs in the [research archive](../design/research/), prefaced as history.
