---
title: CLI
description: How pile is invoked, its environment variables, and its exit codes.
---

`pile` reads its board settings from the config file, the environment, and `gh`.
Help and version flags work without a config file, authentication, or a terminal.

## Invocation

```sh
pile
```

Run it inside a checkout of a GitHub repo. On first run it infers the repo with
`gh repo view`, writes the starter config, and draws the board.

## Options

| option | effect |
| --- | --- |
| `--help`, `-h` | Print usage, configuration environment variables, and the docs URL, then exit. |
| `--version` | Print the version and available build metadata, then exit. |

Release binaries include the version, commit, and build date. Versioned
`go install` builds use Go's embedded module version; checkout builds show
`dev` with VCS metadata when available. Unknown flags and positional arguments
exit with code `1`.

## Environment

| variable | effect |
| --- | --- |
| `PILE_CONFIG` | Full path to the config file. |
| `XDG_CONFIG_HOME` | Replaces `~/.config` when locating the config file. |

See [Config](../config/) for precedence.

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
| `0` | printed help/version, or ran and quit normally |
| `1` | invalid arguments or a startup error, printed as `pile: <reason>` |

A repo you cannot see (an org IP allow-list, for example) is caught at startup
rather than rendered as an empty board.

## Install

Homebrew is the recommended macOS release install:

```sh
brew install barspielberg/tap/pile
```

Stable v0.1.0 is public, with Homebrew installation and interactive checks
validated on Apple Silicon. Public upgrades remain untested. Direct macOS
browser downloads remain unsupported; the Gatekeeper block was observed on
v0.1.0-rc.1 and was not retested on stable.

Source installs on macOS or Linux require Go 1.24.2+:

```sh
go install github.com/barspielberg/pr-pile/cmd/pile@latest
```

See [Install](../../getting-started/install/) for Linux downloads, PATH checks,
and release validation limits.
