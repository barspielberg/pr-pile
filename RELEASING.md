# Releasing pile

pile ships one binary for each of macOS and Linux on amd64 and arm64.
GoReleaser v2.20.0 builds archives and SHA-256 checksums. Each archive contains
the binary, README, and both license files. Go and gh are not bundled; gh is a
runtime requirement, and Linux desktop integrations use the tools listed in
the installation guide.

## Validate locally

Install GoReleaser v2.20.0, then run:

```sh
make check
python3 -B -m unittest discover -s scripts -p 'test_*.py' -v
PILE_REQUIRE_GORELEASER=1 go test ./scripts -count=1 -v
make release-snapshot
```

The snapshot command writes to ignored `dist/` and publishes nothing. Extract
the archive for your host and check `pile --version` and `pile --help` without
gh or config. Inside `dist/`, verify checksums with `sha256sum -c checksums.txt`
(macOS: `shasum -a 256 -c checksums.txt`).

CI runs tests and vet on macOS and Linux, builds all four packages, and
smoke-tests the Linux amd64 archive. PR CI also checks Homebrew publication
payloads (creation, retries, upgrades, downgrade rejection, and invalid input)
and runs a GoReleaser integration test with simulated RC/stable tags at HEAD.
The simulation creates no Git tags. Before claiming desktop integration for
a platform, also check opening a PR and copying a URL in a graphical session.
Cross-compilation alone does not verify those integrations. The macOS archives
are not Developer ID signed or notarized; verify the download experience on
a clean Mac before publishing. The Homebrew formula builds from source.

## Create a release

1. Merge the packaging changes and confirm CI passes on the release commit.
2. For a rehearsal, tag a prerelease such as `v0.1.0-rc.1` and push that tag.
   The release workflow checks macOS and Linux, then creates a draft prerelease.
3. Download and inspect the draft artifacts, verify checksums and version output,
   and review the generated release notes. Publish the prerelease when ready.
4. Tag the validated commit with a stable version, for example `v0.1.0`, and
   push that tag. Review its draft and publish it. Released tags must not move.
5. Verify the public archive downloads, versioned `go install`, and Homebrew
   installation. Update the README and install/CLI docs when the first release
   and tap are available; until then they correctly describe source installs.

Only tag pushes create release drafts. Ordinary pushes and pull requests run
snapshot builds. GitHub Releases remain drafts until a maintainer publishes
them. The release job alone receives repository contents write permission.
GoReleaser is explicitly bound to the triggering tag, so promoting an RC's
commit to a stable release selects the stable tag even when both point at HEAD.

## Set up Homebrew once

1. Create the public repository `barspielberg/homebrew-tap` with an initial
   commit on its default branch.
2. In `pr-pile`, create the Actions secret `HOMEBREW_TAP_TOKEN`: a fine-grained
   token with contents read/write access to **only** `homebrew-tap`.
3. Set the Actions repository variable `HOMEBREW_TAP_ENABLED` to `true`.
4. After publishing the first stable release, run **Update Homebrew tap** with
   its tag. Future published stable releases trigger the workflow automatically;
   drafts and prereleases do not update the tap.
5. Test on macOS and Linux:

   ```sh
   brew install --build-from-source barspielberg/tap/pile
   brew test barspielberg/tap/pile
   pile --version
   ```

The workflow hashes the published tag's source archive and renders
`scripts/homebrew-formula.py` into `Formula/pile.rb` in the tap, building and
testing it on macOS before publishing the update. The formula
declares Go as a build dependency, gh as a runtime dependency, and tests help
and version output without authentication. A manual dispatch retries a failed
tap update after the release is public. Immediately before the API write, the
workflow compares the requested version with the existing formula's source
URL. First creation, same-version retries, and forward updates are allowed.
Older versions (including published backports and manual old-tag retries)
fail the job without updating the tap. Unreadable existing formula state also
fails closed. The write includes the checked file's SHA, so a concurrent change
causes an API conflict rather than bypassing the comparison. To inspect a formula locally:

```sh
python3 scripts/homebrew-formula.py v0.1.0 SOURCE_ARCHIVE_SHA256 > /tmp/pile.rb
```

Use the actual source archive checksum, not the checksum of a binary archive.
The placeholder above is rejected until replaced with a 64-character SHA-256.
