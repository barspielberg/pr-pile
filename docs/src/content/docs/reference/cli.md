---
title: CLI
description: How pile is invoked, its environment variables, and its exit codes.
---

`pile` takes no flags. Everything it needs comes from the config file, the
environment, and `gh`.

## Invocation

```sh
pile
```

Run it inside a checkout of a GitHub repo. On first run it infers the repo with
`gh repo view`, writes the starter config, and draws the board.

## Environment

| variable | effect |
| --- | --- |
| `PILE_CONFIG` | Full path to the config file. |
| `PILE_REPO` | Overrides `repo:` for one run. |
| `XDG_CONFIG_HOME` | Replaces `~/.config` when locating the config file. |

See [Config](config/) for precedence.

## `gh`

pile never stores a credential of its own. It reads your token with
`gh auth token` on startup, and requires `gh` to be installed and logged in:

```sh
gh auth login
```

If `gh` is missing or logged out, pile exits with `gh auth token: … (is gh
installed and logged in?)`.

## Exit codes

| code | meaning |
| --- | --- |
| `0` | ran and quit normally |
| `1` | startup failed — a config, token, or repo error, printed as `pile: <reason>` |

A repo you cannot see (an org IP allow-list, for example) is caught at startup
rather than rendered as an empty board.

## Install

`go install` works today, before any release:

```sh
go install github.com/barspielberg/pr-pile/cmd/pile@latest
```

Homebrew is planned for the first release:

```sh
brew install barspielberg/tap/pile
```

See [Install](../getting-started/install/) for the full options.
