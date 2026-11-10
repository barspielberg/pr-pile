---
title: Install
description: Requirements and install options for pile.
---

pile runs on macOS and Linux and needs `gh` for its token. On macOS, install
with Homebrew.

## Requirements

- **macOS or Linux** — macOS uses the built-in `open` and `pbcopy`. On Linux,
  install `xdg-open` (usually packaged as `xdg-utils`) to open PRs, and
  `wl-copy` (from `wl-clipboard`) on Wayland or `xclip` / `xsel` on X11 to copy.
  Both need a graphical session.
- **`gh`** — installed and logged in. pile reads your token from `gh auth token`
  and infers the repo with `gh repo view`, so it never asks you to provision a
  credential of its own.
- **Go 1.24.2+** — only if you build from source. Binary downloads and Homebrew
  use prebuilt release binaries and do not require Go.

## Install

### Homebrew

Recommended for macOS:

```sh
brew install barspielberg/tap/pile
```

The formula selects the macOS/Linux and amd64/arm64 release archive, verifies
its SHA-256 from the published manifest, and installs `gh` as a runtime
dependency. It preserves the release binary's version, commit and build date.
You still need to run `gh auth login` yourself.

### go install

Build from source on macOS or Linux:

```sh
go install github.com/barspielberg/pr-pile/cmd/pile@latest
```

For a source install of a specific published version, use
`go install github.com/barspielberg/pr-pile/cmd/pile@vVERSION` instead of `@latest`.

### Binary downloads

The [releases page](https://github.com/barspielberg/pr-pile/releases)
provides archives for macOS (`darwin`) and Linux, on Intel/AMD (`amd64`)
and ARM (`arm64`). Each archive includes `pile`, the README, and license files.

Download the matching archive and `checksums.txt` into an empty directory.
For example, for Linux amd64, replacing `VERSION` with the release version
without the `v` prefix:

```sh
sha256sum --ignore-missing -c checksums.txt
tar -xzf pile_VERSION_linux_amd64.tar.gz
install -Dm755 pile "$HOME/.local/bin/pile"
```

Ensure `~/.local/bin` is on your `PATH`. A binary download does not install
`gh` or Linux desktop tools; install the requirements above separately.

On macOS, use [Homebrew](#homebrew) or [`go install`](#go-install) instead:
the release binaries are not notarized, so Gatekeeper blocks a direct download.

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
pile --version
pile --help
cd path/to/a/github/repo
pile
```

On first run pile infers the repo from the checkout and writes a starter config
to `~/.config/pile/config.yml`. See [First run](../first-run/).

If `pile` exits with `gh auth token: … (is gh installed and logged in?)`, run
`gh auth login` first.

If an older `go install` binary is also present, check `command -v pile` and
`pile --version`, and put the install you want first in `PATH`.
