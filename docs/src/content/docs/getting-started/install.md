---
title: Install
description: Requirements and install options for pile.
---

pile runs on macOS and Linux, needs `gh` for its token, and builds from source with
Go 1.24+. There are no tagged releases yet, so install is `go install` or a
checkout build; Homebrew follows the first release.

## Requirements

- **macOS or Linux** — macOS uses the built-in `open` and `pbcopy`. On Linux,
  install `xdg-open` (usually packaged as `xdg-utils`) to open PRs, and
  `wl-copy` (from `wl-clipboard`) on Wayland or `xclip` / `xsel` on X11 to copy.
  Linux builds and automated tests run in CI; desktop integration still needs
  a graphical session.
- **`gh`** — installed and logged in. pile reads your token from `gh auth token`
  and infers the repo with `gh repo view`, so it never asks you to provision a
  credential of its own.
- **Go 1.24+** — only if you build from source.

## Install

### go install

Works today, no release needed:

```sh
go install github.com/barspielberg/pr-pile/cmd/pile@latest
```

### Homebrew

```sh
brew install barspielberg/tap/pile
```

Coming after the first release. This line is gated on a `goreleaser` + Homebrew
tap setup, which does not exist yet.

### From a checkout

```sh
git clone https://github.com/barspielberg/pr-pile
cd pr-pile
make install
```

`make install` runs the build, tests and vet first, then puts this checkout's
`pile` on your `PATH`.

## Verify

```sh
cd path/to/a/github/repo
pile
```

On first run pile infers the repo from the checkout and writes a starter config
to `~/.config/pile/config.yml`. See [First run](../first-run/).

If `pile` exits with `gh auth token: … (is gh installed and logged in?)`, run
`gh auth login` first.
