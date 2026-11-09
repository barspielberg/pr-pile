---
title: Install
description: Requirements and install options for pile.
---

pile runs on macOS and Linux, needs `gh` for its token, and builds from source with
Go 1.24.2+. Install from source today using `go install` or a checkout build.
`v0.1.0-rc.1` has draft artifacts, but there is no public stable release yet.
The public Homebrew install and upgrade paths have not been validated.

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

For a source install of a specific published version, use
`go install github.com/barspielberg/pr-pile/cmd/pile@vVERSION` instead of `@latest`.

### Homebrew

**Not publicly available yet.** Once the first stable release and public tap
are available and validated, Homebrew will be the recommended macOS release
install:

```sh
brew install barspielberg/tap/pile
```

The formula selects the macOS/Linux and amd64/arm64 release archive, verifies
its SHA-256 from the published manifest, and installs `gh` as a runtime
dependency. It preserves the release binary's version, commit and build date.
You still need to run `gh auth login` yourself.

A local `pile-candidate` formula using verified draft RC archives passed
reinstall, `brew test`, version, board, clipboard, browser, and quit checks on
Apple Silicon. That test does not validate `barspielberg/tap/pile` or public
upgrades. See the
[release guide and acceptance record](https://github.com/barspielberg/pr-pile/blob/main/RELEASING.md#v010-rc1-acceptance)
for maintainer checks and remaining release work.

### Binary downloads

After the first public stable release, the [releases page](https://github.com/barspielberg/pr-pile/releases)
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

Ensure `~/.local/bin` is on your `PATH`. A binary download does not install
`gh` or Linux desktop tools; install the requirements above separately.

#### macOS browser-download limitation

The current macOS RC binary is not Developer ID signed and notarized. In the
Apple Silicon test, Firefox downloads retained quarantine after both Terminal
`tar` extraction and Finder extraction. macOS blocked execution with an
“Apple could not verify” dialog; `pile --version` was killed with exit 137.

The archive's SHA-256 matched the release manifest, and `codesign --verify`
accepted the linker's ad-hoc signature, which has no Team ID. Those checks do
not establish Gatekeeper acceptance: `spctl --assess --type execute` rejected
the binary. A successful local Homebrew candidate test does not reproduce the
browser-download experience.

Use the [source install](#go-install) today. Once publicly available and
validated, use [Homebrew](#homebrew) for macOS release binaries. Direct macOS
browser downloads are not a supported install path until signing, notarization,
and first-run Gatekeeper validation are complete.

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
