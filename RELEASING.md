# Releasing pile

GoReleaser v2.20.0 produces `pile_VERSION_OS_ARCH.tar.gz` for macOS (`darwin`)
and Linux, each on `amd64` and `arm64`. `VERSION` has no `v` prefix; an RC keeps
its suffix, e.g. `pile_0.1.0-rc.1_darwin_arm64.tar.gz`. Archives contain the
binary, README and both licenses. `checksums.txt` lists their SHA-256 hashes.
The binary embeds version, commit and build date. Homebrew installs that same
binary, preserving its metadata; it needs `gh` at runtime and does not build Go.

## Local checks

Install GoReleaser v2.20.0, then run:

```sh
make check
python3 -B -m unittest discover -s scripts -p 'test_*.py' -v
PILE_REQUIRE_GORELEASER=1 go test ./scripts -count=1 -v
make release-snapshot
```

Snapshots write ignored `dist/` and publish nothing. Inside `dist/`, run
`sha256sum -c checksums.txt` (macOS: `shasum -a 256 -c checksums.txt`). Test the
host binary's `--version` and `--help` without gh/config, then open and copy a
PR in a desktop session. Cross-compiling is not a desktop integration test.

The required CI job is named **check**. It runs even if dependencies fail or
are skipped, and succeeds only when both OS test legs, packaging, and internal
docs validation succeed. Configure the repository ruleset to require **check**.
External-link availability never gates it.

## Docs links

With Node 24+, the pnpm version from `docs/package.json`, and Python 3:

```sh
cd docs
pnpm install --frozen-lockfile
pnpm build
cd ..
python3 scripts/install-lychee.py "$TMPDIR/pile-link-tools"
LYCHEE="$TMPDIR/pile-link-tools/lychee" PILE_REQUIRE_LYCHEE=1 \
  python3 -B -m unittest discover -s scripts -p 'test_doc_links.py' -v
python3 scripts/check-doc-links.py docs/dist --lychee "$TMPDIR/pile-link-tools/lychee"
```

Set `TMPDIR` to a writable scratch directory first. The installer pins Lychee
0.24.2 and platform-specific SHA-256 values, verifies before extracting, and
retries downloads three times. Its failure is an installation error, not a
broken-link result. Internal checking copies built HTML into scratch, maps the
deployed `/pr-pile` base (including absolute self-hosted URLs), requires directory
`index.html` files, supports Pages' extensionless `.html` URLs, and checks fragment
anchors offline. Exit 0 means success, 1 means link failures, and 2 means missing
build/tool prerequisites. Fixtures prove valid links pass and missing pages or
anchors fail. The checker does not consult the currently deployed site.

**External docs link report** runs weekly or via manual dispatch, builds HTML,
and checks external HTTP(S) URLs in up to three passes, retrying failed reports
after a short delay (including transient HTTP 5xx responses). Its Markdown
report is uploaded as the `external-links` artifact. It does not run on PRs;
external checker failures are reported without gating ordinary merges. To run
it locally, add `--external --report "$TMPDIR/external-links.md"` to the checker.

## Create a draft/RC and test it on a Mac before publication

`v0.1.0-rc.1` already has draft artifacts; see its [acceptance record](#v010-rc1-acceptance).
For subsequent candidates, after merging validated changes, a maintainer may
create and push a new RC tag. The release workflow runs tests, then creates a
**draft prerelease**.
It explicitly binds GoReleaser to the triggering tag, even when RC and stable
tags share a commit. Review release notes and inspect artifacts before deciding
to publish. Ordinary pushes and PRs create snapshots only.

Run the following **on a Mac**, from the repository root, to download release
assets and create a local candidate tap:

```sh
tag=v0.1.0-rc.1                 # also accepts an actual stable draft tag
candidate=$(mktemp -d "${TMPDIR:?}/pile-candidate.XXXXXX")
mkdir "$candidate/artifacts"
gh release download "$tag" --repo barspielberg/pr-pile \
  --pattern checksums.txt --pattern 'pile_*.tar.gz' --dir "$candidate/artifacts"
python3 scripts/homebrew-formula.py "$tag" "$candidate/artifacts/checksums.txt" \
  --candidate-dir "$candidate/artifacts" > "$candidate/pile-candidate.rb"
brew tap-new --no-git barspielberg/pile-candidate
cp "$candidate/pile-candidate.rb" "$(brew --repository barspielberg/pile-candidate)/Formula/pile-candidate.rb"
brew install barspielberg/pile-candidate/pile-candidate
brew test barspielberg/pile-candidate/pile-candidate
pile-candidate --version
```

`gh` must be authenticated as someone who can read the draft. Private draft
assets cannot reliably be fetched at public `/releases/download/TAG/` URLs;
`gh release download` uses authenticated asset APIs. Keep credentials out of
formulae, logs, and shared messages. All four archives must be downloaded: the
generator rejects malformed/missing/duplicate manifest entries, wrong versions,
and any archive hash mismatch before producing the candidate. Its `file://`
URLs reference already-verified local files, not authenticated URLs or secrets.
It retains the real RC version, uses class `PileCandidate`, installs
`pile-candidate`, and is rejected by the stable tap publication guard.

Keep the candidate directory until testing finishes. If the local candidate tap
already exists, omit `brew tap-new`, replace its formula and use
`brew reinstall barspielberg/pile-candidate/pile-candidate` followed by `brew test`.
Test opening/copying a PR with `pile-candidate` too. For cleanup, explicitly run
`brew uninstall barspielberg/pile-candidate/pile-candidate`, then
`brew untap barspielberg/pile-candidate`; remove only the candidate directory you
created when no longer needed.

**Direct archive/Gatekeeper testing is separate.** The binaries are not Developer
ID signed/notarized. A Homebrew/local-file test or gh download does not reproduce
browser quarantine. To check the end-user download experience, download the
matching archive via the authenticated GitHub draft UI in a browser, compare
its SHA-256 with the draft manifest, extract it and execute `pile --version` on
a clean Mac. Record any Gatekeeper prompt/failure; do not strip quarantine or
approve trust dialogs to silently turn that test into a pass. Check signing
status separately with `codesign --verify --verbose PATH_TO_PILE` and
`spctl --assess --type execute --verbose PATH_TO_PILE`. A valid checksum or
ad-hoc `codesign` verification does not establish Gatekeeper acceptance. Do not document quarantine removal or Gatekeeper bypasses as install
steps. Developer ID signing, notarization, and a passing first-run test using
public stable URLs are required before advertising macOS browser downloads.

## v0.1.0-rc.1 acceptance

Reported macOS acceptance on **Apple Silicon (arm64)** for the real draft
`v0.1.0-rc.1` at `7e9f8d1`, using GoReleaser 2.20.0 and Homebrew:

| Check | Actual result |
| --- | --- |
| All four draft archives against `checksums.txt` | PASS |
| Local candidate formula from those downloaded archives | PASS |
| `brew reinstall` and `brew test` of `pile-candidate` | PASS |
| `pile-candidate --version` | `pile 0.1.0-rc.1 (commit 7e9f8d1, built 2026-11-09T20:58:57Z)` |
| Board loads live PRs | PASS |
| `y` copies the selected URL | PASS; clipboard cleared first and checked with `pbpaste` |
| `enter` opens the selected PR | PASS; correct browser tab confirmed visually |
| `q` exits cleanly | PASS |
| Firefox download of the darwin arm64 archive | SHA-256 PASS; quarantine retained after both `tar` and Finder extraction |
| Signing checks on that extracted binary | Ad-hoc linker signature, no Team ID; `codesign --verify` valid on disk |
| Gatekeeper / first execution | `spctl` rejected; `pile --version` killed with exit 137 and “Apple could not verify” dialog |

The browser test preserved quarantine and did not approve trust dialogs. It
records a blocked first run. The working local candidate uses verified `file://`
archives and installs `pile-candidate`; it is distinct from the public
`barspielberg/tap/pile` formula and its install/upgrade path.

Earlier local-candidate testing at `7c77ab5` exposed a **test-only** macOS temp
path expectation (`/var` versus `/private/var`). The merge resolves the expected
path in the test, and all 11 `test_homebrew_formula.py` tests passed on macOS at
`7e9f8d1`. This was not a release binary behavior change. The pending glyph
rendering issue observed inside Herdr was isolated to Herdr, not a pile defect.

Current readiness and remaining validation:

- The public tap repository is initialized and the token is saved. Automation
  is **not enabled**; there is no public stable release.
- Public `barspielberg/tap/pile` install, `brew test`, version, and upgrade
  validation remain open. Candidate acceptance does not satisfy these checks.
- Linux release installation and desktop integrations remain untested. Exercise
  browser and clipboard behavior in native graphical sessions.
- Native runtime acceptance beyond macOS arm64 remains open: macOS amd64 and
  Linux amd64/arm64. Four matching archive hashes and cross-builds do not prove
  native execution or integrations.
- macOS browser downloads remain blocked pending Developer ID signing,
  notarization, and successful Gatekeeper validation. Recommend source installs
  now, then Homebrew once publicly available and validated.

## Stable publication and one-time Homebrew setup

Only after candidate acceptance, a maintainer may tag the validated commit with
the stable version and review/publish its draft. Released tags must not move.
Drafts and prereleases never publish to the tap.

Before enabling automation, configure the following prerequisites:

1. A **public `barspielberg/homebrew-tap` repository with an initial commit** on
   its default branch. A missing/empty repository cannot receive a formula.
2. In `pr-pile`, an Actions secret named **`HOMEBREW_TAP_TOKEN`**, containing a
   fine-grained PAT scoped to **only `homebrew-tap`, Contents: read and write**
   (plus GitHub's mandatory metadata access). It does not need write access to
   `pr-pile`. Add the secret through GitHub settings and keep its value private.
3. Set the `pr-pile` Actions variable **`HOMEBREW_TAP_ENABLED=true` only after**
   the tap, token, candidate tests, and workflow on the default branch are ready.
   Until then leave it unset/false.

After publishing the first stable release, manually run **Update Homebrew tap**
with its tag. Future stable `published` events trigger it automatically. The
workflow confirms public/stable state, downloads the published checksum manifest
with retries, renders four binary URLs/checksums, installs/tests the formula on
macOS, then checks the existing remote formula immediately before writing.
First creation, equal-version retries and forward versions are allowed; older
published backports/manual retries fail without changing the tap. Unreadable
state fails closed; the checked file SHA makes concurrent updates conflict.
The older source-formula format is recognized for safe upgrades.

After public stable publication and tap update, verify public installation on
macOS and Linux:

```sh
brew install barspielberg/tap/pile
brew test barspielberg/tap/pile
pile --version
```

Also validate `brew upgrade barspielberg/tap/pile` from a previously installed
public version when one is available; record the starting and resulting versions.
A fresh install or local candidate reinstall is not an upgrade test.

Update README/install/CLI status once the first stable release and tap are
actually public. Binary archives need gh and Linux desktop tools separately.
