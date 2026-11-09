---
title: Install
description: Requirements and install options for pile.
---

pile runs on macOS and Linux, needs `gh` for its token, and builds from source with
Go 1.24.2+. There are no tagged releases yet, so install is `go install` or a
checkout build. Binary downloads and Homebrew follow the first release.

## Requirements

- **macOS or Linux** — macOS uses the built-in `open` and `pbcopy`. On Linux,
  install `xdg-open` (usually packaged as `xdg-utils`) to open PRs, and
  `wl-copy` (from `wl-clipboard`) on Wayland or `xclip` / `xsel` on X11 to copy.
  Linux builds and automated tests run in CI; desktop integration still needs
  a graphical session.
- **`gh`** — installed and logged in. pile reads your token from `gh auth token`
  and infers the repo with `gh repo view`, so it never asks you to provision a
  credential of its own.
- **Go 1.24.2+** — only if you build from source. Binary downloads and Homebrew
  use prebuilt release binaries and do not require Go.

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

Available after the first stable release and tap setup. The formula selects
the macOS/Linux and amd64/arm64 release archive, verifies its SHA-256 from the
published manifest, and installs `gh` as a runtime dependency. It preserves
the release binary's version, commit and build date. You still need to run
`gh auth login` yourself.

Maintainers must first create the public `barspielberg/homebrew-tap` repository
with an initial commit. In `pr-pile`, add the Actions secret
`HOMEBREW_TAP_TOKEN` as a fine-grained token scoped to that tap only, with
Contents read/write permission. Set `HOMEBREW_TAP_ENABLED=true` only after the
tap, token and candidate tests are ready. Add secrets through GitHub settings;
never send them in chat. Drafts and RC releases do not update the stable tap.
See the [release guide](https://github.com/barspielberg/pr-pile/blob/main/RELEASING.md)
for authenticated draft/RC downloads and local Mac candidate-formula tests
before stable publication. Homebrew testing and direct-download Gatekeeper
testing are separate steps.

### Binary downloads

After the first release, the [releases page](https://github.com/barspielberg/pr-pile/releases)
will provide archives for macOS (`darwin`) and Linux, on Intel/AMD (`amd64`)
and ARM (`arm64`). Each archive includes `pile`, the README, and license files.

Download the matching archive and `checksums.txt` into an empty directory.
For example, for Linux amd64, replacing `VERSION` with the release version
without the `v` prefix:

```sh
sha256sum --ignore-missing -c checksums.txt
tar -xzf pile_VERSION_linux_amd64.tar.gz
install -Dm755 pile "$HOME/.local/bin/pile"
```

On macOS, check the matching archive with
`shasum -a 256 -c checksums.txt --ignore-missing`, extract it, then run
`mkdir -p "$HOME/.local/bin"` and `install -m755 pile "$HOME/.local/bin/pile"`.
Ensure `~/.local/bin` is on your `PATH`. A binary download does not install
`gh` or Linux desktop tools; install the requirements above separately.

For a source install of a specific published version, use
`go install github.com/barspielberg/pr-pile/cmd/pile@vVERSION` instead of `@latest`.

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
