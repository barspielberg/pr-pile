# pr-pile — documentation plan

**Status:** research + proposal. Nothing here is built yet.
**Date:** 2026-11-06. Version numbers and measurements are from this date.
**Scope:** a new README and a documentation site on GitHub Pages.

---

## 0. Summary

`pr-pile` is a working tool with unusual depth in its notes and almost no
on-ramp for anyone who did not write it. The docs tree is 47.7k words, most of
it design research written for one reader; the README is 1,655 words that open
with build commands and spend ~57 lines on one personal worktree recipe. There is
no screenshot, no documented install path (though `go install …@latest` works
today; only `brew` is gated on a release), no license, and no CI.

The plan is to split the material by audience instead of rewriting it wholesale:

| layer | what it is | where it goes |
|---|---|---|
| landing | what it is, why, one install command, demo | `README.md` (~450 words) |
| user docs | install, concepts, guides, reference, troubleshooting | docs site |
| design rationale | principles, decisions, visual spec, measurements | docs site, `/design/` |
| research archive | the five long studies (uniform rows, section layout, view restructure, PR detail, checks page) | docs site, `/design/research/`, prefaced as historical |

**Recommended stack:** Astro Starlight, source in `docs/`, deployed by GitHub
Actions to `https://barspielberg.github.io/pr-pile/`.

**Work:** ~30 pages at launch, of which ~10 are edited migrations and ~12 are
new writing; two to three focused days in six shippable phases.

**Prerequisites outside the docs themselves** (see §8, decisions 3–4 for two of
them): a LICENSE, a CI workflow, and a release story (`goreleaser` + Homebrew
tap) — the install page cannot honestly promise a one-liner until one exists.

---

## 1. Where the project is today

### 1.1 Repo facts

| fact | value |
|---|---|
| repo | `barspielberg/pr-pile`, public, 1 star, no description, no homepage |
| code | Go 1.24.2, Bubble Tea 1.3.10, lipgloss 1.1.0 — 4,446 lines non-test, 6,482 lines of tests |
| packages | `cmd/pile`, `internal/{board,browser,config,github,ui}` |
| releases | none — no tags, no GitHub releases, no binaries |
| CI | none (`.github/` does not exist) |
| license | **none** |
| Pages | not enabled |
| last commit | 2026-09-21 |
| docs | 8 files in `docs/`, 47,682 words |
| build | `go build`, `go test ./... -short`, `go vet ./...` all pass locally (verified) |
| platform | **macOS only in practice**: `open`, `pbcopy`, `osascript` (Arc tab reuse) — not stated anywhere |

### 1.2 The docs tree, by audience

| file | words | what it actually is | audience today |
|---|---:|---|---|
| `README.md` | 1,655 | feature list + build + config + one long worktree recipe + search semantics | newcomer, badly served |
| `docs/DESIGN.md` | 12,402 | the design spec: model, decision table, visual spec, config reference, keys, measurements, known problems | author / contributors |
| `docs/DESIGN-GUIDE.md` | 3,614 | principles above the spec (colour budget, monochrome first, density) | contributors |
| `docs/config-example.md` | 2,013 | a five-rule team board worked end to end | user, if they find it |
| `docs/pr-detail.md` | 8,120 | research memo: what belongs in the `d` overlay, with API cost measurements | author |
| `docs/section-layout.md` | 10,656 | research memo: section headers, pinned vs inline, with mockups | author |
| `docs/view-restructure.md` | 5,579 | research memo: whether to restructure the view (superseded) | author |
| `docs/uniform-rows.md` | 3,442 | research memo: the uniform-row restructure, measurements | author |
| `docs/checks-page.md` | 1,856 | research memo: what belongs on the checks page | author |

Three audiences are interleaved with no signal: **user**, **contributor**, and
**author reasoning out loud**. A reader who wants to install the tool has to
walk past a decision table to find `make build`.

### 1.3 README problems, concretely

1. **No image.** A terminal board is a visual product; the README never shows
   the board. Every comparable tool leads with a screenshot or a recording.
2. **The lede is buried.** Line 3 is a one-liner, then Status, then Build;
   `make install` is line 21, and the first thing a visitor learns is the author's
   assessment of their own feature completeness.
3. **A false requirement.** Line 24 says a Nerd Font is needed for the glyphs.
   The code uses only BMP characters (`✓ ✗ ◐ ○ ╭╴ │ ╰╴ ▌ …`); there is not one
   Private Use Area codepoint in non-test code, and `DESIGN.md` §2 explicitly
   rejects Nerd Font glyphs. The README contradicts both the code and the spec.
4. **One recipe eats the page.** §"Example: open a PR as a worktree workspace"
   is ~57 lines — about a quarter of the file — about Worktrunk and `herdr`, tools the
   project does not ship. It is excellent content in the wrong place.
5. **Undocumented config.** `author: true` exists in `config.go` and in the
   starter config, but never appears in the README's rule example. `limit`
   truncating *before* first-match dedup is documented in `DESIGN.md` §4 and §7 and the starter config.
6. **Platform silence.** `open`/`pbcopy`/`osascript` mean macOS today. The
   README reads as cross-platform.
7. **The search essay belongs on a docs page.** §Searching is 38 lines of
   precise semantics (matches only what is drawn, author initials only, age
   never, width-dependent). It is the best writing in the file and the least
   skimmable.
8. **No license, no CI badge, no docs link, no table of contents.** 216 lines
   with no way to jump.

### 1.4 What is good and must survive

- The **decision table** in `DESIGN.md` §2 — each row names the alternative and
  the evidence. This is rare and is the project's best argument.
- **Measured facts** (§6): fetch latency by rule count, why row caps do not
  change speed, scroll-margin values across nine tools.
- `config-example.md`'s worked reasoning: what each rule *claims* vs what it
  *shows* after the rules above take their share.
- The writing voice: specific, unhedged, evidence-first. The site should not
  sand it into marketing prose.

### 1.5 Gaps a newcomer hits

| missing | why it matters |
|---|---|
| install that does not require the repo | `make install` needs Go and a checkout |
| quickstart | repo inference, starter config, `PILE_REPO`, first refresh |
| concepts page | "rules are the whole abstraction" is the mental model, taught nowhere |
| reference with types/defaults | config lives in prose examples |
| keys by context | the README table mixes board/search/detail/help |
| troubleshooting | `gh auth token`, org IP allow-lists, unreadable repo at startup, Arc |
| limitations | the honest "not built" list is buried in `DESIGN.md` §7 |
| contributing | package map, test conventions, why tests outnumber code 1.5:1 |

---

## 2. What comparable tools do

Surveyed the sites and repos of eight terminal tools on 2026-11-06.

| tool | stars | site | generator | IA shape | notable |
|---|---:|---|---|---|---|
| gh-dash | 12.7k | gh-dash.dev | Astro **Starlight** (0.35 / Astro 5) | Getting Started (install, usage, updating) · Keybindings (by context) · Configuration (17 pages) · Companion app · Insiders · Contributing | config split per object: `pr-section`, `issue-section`, `repo-paths`, `theme`, `layout/*`; config *schema* page separate from examples |
| yazi | 43.3k | yazi-rs.github.io | Docusaurus 3.10 | Quick start · Installation · Configuration (keymap split into 3 pages) · Plugins (8 pages) · Tips · FAQ | **versioned docs**: `/docs/next/` for main, `/docs/` for stable |
| jj | 32.4k | docs.jj-vcs.dev | MkDocs Material + `mike` | Concepts → guides → CLI reference → GitHub | version switcher from day one; `edit_uri` set |
| helix | 46.9k | docs.helix-editor.com | mdBook | one book: install → usage → configuration → languages | "book" feel, no landing page or nav chrome |
| delta | 32.7k | dandavison.github.io/delta | mdBook | one book | README does the selling; book is the manual |
| k9s | 34.9k | k9scli.io | static HTML on GitHub Pages | headline + features + screenshots + topic pages | topic URLs (`/topics/config`, `/topics/plugins`); site source is not in the code repo |
| zellij | 36k | zellij.dev | Hugo | marketing landing + docs section | landing is separate from docs |
| lazygit | 83.7k | none | — | `docs/*.md` in repo (`Config.md` 50 KB) | proves a site is optional; the 50 KB config page is the pain point |

### Lessons worth copying

1. **README sells, site explains.** k9s, delta and zellij all keep the README
   short and put the manual elsewhere; lazygit's 50 KB `Config.md` is what a
   README-shaped reference eventually becomes.
2. **Sidebar by task, not by file.** gh-dash's sidebar is a user journey
   (`Getting started → Keybindings → Configuration`), not a mirror of `docs/`.
3. **Configuration gets a section, one page per object.** Nothing is worse than
   a single config page that has to be searched. gh-dash and yazi both split:
   top-level file, then per-section, then layout, then theme.
4. **Keys are split by context.** gh-dash has seven keybinding contexts (plus
   completions): global, navigation, preview, selected item, selected PR,
   selected issue, notifications. pile has four contexts: board, search prompt,
   detail overlay, help page.
5. **Every project of this class shows a recording, not a paragraph.** gh-dash
   ships `.tape` files and mp4/webm pairs in the docs repo — the demo is
   generated, not hand-recorded and forgotten.
6. **Versioning is deferred until there are versions.** yazi has it because it
   has releases; gh-dash does not. pile has zero tags — a version dropdown now
   would be a menu with one item.

---

## 3. Generator research

### 3.1 Requirements

| # | requirement | weight |
|---|---|---|
| R1 | plain markdown authoring, no framework knowledge required for routine edits | must |
| R2 | static output deployable to GitHub Pages under a sub-path (`/pr-pile/`) | must |
| R3 | local full-text search with no external service | must |
| R4 | first-class dark **and** light theme | must |
| R5 | code blocks with shell/YAML/GraphQL highlighting and copy button | must |
| R6 | sidebar nested 2–3 deep, explicit or autogenerated | must |
| R7 | components for callouts, tabs, key-cap/legend tables (optional, not required for every page) | should |
| R8 | sitemap, 404, "edit this page", accessible defaults | should |
| R9 | versioning later, without a rewrite | should |
| R10 | small maintenance surface: no server, no database, no build service | must |

### 3.2 Measured comparison

Both candidates were installed and built locally on 2026-11-06, Node 26.7.0 /
npm 12.0.2, minimal sites: 8 pages (Starlight) and 4 pages (VitePress), same
content shape (hero, install, rules, config).

| | **Astro Starlight** | **VitePress** | mdBook | Docusaurus | MkDocs Material | Hugo | Zensical |
|---|---|---|---|---|---|---|---|
| version measured | Starlight 0.42.10 / Astro 7.2.10 | 1.6.4 | — | 3.10 (yazi) | maintenance mode | — | 0.1.0 |
| language / deps | Node, 269 packages | Node, 125 packages | Rust binary | Node, heavy | Python | Go binary | Python + Rust |
| `node_modules` | **199 MB** | **102 MB** | 0 | ~400 MB+ | venv | 0 | venv |
| cold install | **18.5 s** | **13.6 s** | n/a | minutes | seconds | seconds | seconds |
| build (small site) | **1.5 s** | **1.6 s** | <1 s | ~10–30 s | ~1 s | <1 s | ~1 s |
| output | 1.2 MB / 9 HTML (696 KB Pagefind) | 1.1 MB / 4 pages | small | larger | medium | small | medium |
| sub-path base | `site` + `base`, verified `/pr-pile/…` | `base: '/pr-pile/'` | `--base` | `baseUrl` | `site_url` | `baseURL` | similar |
| search | Pagefind, built in | MiniSearch, built in | built in | plugin (local/Algolia) | built in | external | built in |
| versioning | plugin (`starlight-versions`) | none built in | none | **built in** | `mike` | manual | compat |
| theming | CSS custom properties + component overrides | Vue theme, CSS vars | limited | React swizzle | theme config | templates | Material-like |
| custom pages | MDX + Astro components | Vue SFC | none | React | HTML templates | templates | none |
| authoring cost | low (`.md` or `.mdx`) | low (`.md`) | lowest | medium | low | medium | low |

**Setup gotcha found while measuring:** Starlight no longer scaffolds content
implicitly. Without `src/content.config.ts` exporting
`docs: defineCollection({ loader: docsLoader(), schema: docsSchema() })`, the
first build fails at `/404` with *"The slug … specified in the sidebar config
does not exist"* — while still emitting assets, so the error looks unrelated.
Worth knowing before someone concludes the tool is broken.

### 3.3 Recommendation: Astro Starlight

It is the only candidate that satisfies all ten requirements with **no
configuration for search, sitemap, 404, a11y and dark/light**, and it has the
closest peer precedent: gh-dash runs the same stack for the same audience
(GitHub TUI, same page taxonomy, `.tape` demo assets in the docs tree).

Mapping to the requirements: R1 plain `.md` works (MDX only where a component
earns it) · R2 verified with `base: '/pr-pile'` · R3 Pagefind · R4 theme toggle
built in · R5 Expressive Code · R6 explicit or `autogenerate` sidebars · R7
`Aside`, `Tabs`, `Steps`, `Card`, `Badge` ship in the box · R8 sitemap + 404 +
`editLink` · R9 `starlight-versions` when tags exist · R10 static, no service.

Two components are directly useful rather than decorative: `<Tabs>` for
macOS/Linux install commands and for rule examples vs rendered result, and
`<Steps>` for the quickstart. Everything else can stay plain markdown, which
also keeps a future generator swap cheap.

### 3.4 Why not the others

- **VitePress** — genuinely close: 40% fewer packages, half the disk, same build
  time, excellent local search. Rejected because customising the theme means
  writing Vue (Starlight's overrides are plain Astro/HTML), and because the
  nearest peer already proves the Starlight path end to end. It is the fallback
  if Node dependency weight becomes the deciding factor.
- **Docusaurus** — the only one with built-in versioning, which pile does not
  need yet; React + a large dependency tree for a 28-page site is a poor trade.
- **MkDocs Material** — the best reference UX of the group, but the project is
  in maintenance mode: critical fixes and security updates only through
  2027-05-05, with the team focused on Zensical. Choosing it now means planning
  a migration.
- **Zensical** — the successor, but it shipped **v0.1.0 on 2026-11-05**, one day
  before this research. Pre-1.0 config and theme APIs are the wrong foundation
  for a docs site meant to last.
- **Hugo** — fastest builds and a Go toolchain (nice for a Go repo), but theme
  wiring is Go-template work and there is no first-class docs component set;
  the site would need search bolted on.
- **mdBook** — right for helix and delta because their docs *are* a book. No
  landing page, no sidebar control beyond SUMMARY.md, no theming worth the name;
  a product page would have to be built beside it.

### 3.5 The cost, stated plainly

199 MB of `node_modules` and 269 packages for a repo that is otherwise a single
Go binary: a second toolchain for contributors, a lockfile to keep, and Astro's
major-version cadence (this measurement is Astro 7; gh-dash's docs are still on
Astro 5). Mitigations: commit the lockfile, pin majors, build docs only in CI
and for doc changes, and never commit `node_modules` or `dist`.

---

## 4. Hosting on GitHub Pages

### 4.1 URL and base path

| option | URL | config | note |
|---|---|---|---|
| **project site (recommended)** | `https://barspielberg.github.io/pr-pile/` | `site: 'https://barspielberg.github.io'`, `base: '/pr-pile'` | free, no DNS, works today on a public repo; verified locally that assets and links emit `/pr-pile/…` |
| custom domain (later) | e.g. `pile.barspielberg.dev` | `site`, `base: '/'`, `public/CNAME` | needs a DNS record — ask before touching it |

Pages on a **public** repo is free on GitHub Free; this repo is public.

### 4.2 One-time settings

Settings → Pages → Build and deployment → **Source: GitHub Actions**. (Or
`gh api -X POST repos/barspielberg/pr-pile/pages -f build_type=workflow`.)
Without this the workflow runs and the deploy step fails with a permissions
error that reads like a bug in the site.

### 4.3 Workflow shape

`.github/workflows/docs.yml`, triggered on `push` to `main` for `docs/**` and on
`workflow_dispatch`; `permissions: contents: read, pages: write, id-token:
write`; concurrency group `pages` with `cancel-in-progress: true`. Astro's
official action does install + build + upload:

```yaml
- uses: actions/checkout@v7
- uses: withastro/action@v6
  with:
    path: docs          # the Astro project lives here
- uses: actions/deploy-pages@v5
```

(`configure-pages@v5` / `upload-pages-artifact@v4` are what `withastro/action`
wraps; pin whatever is current at implementation — these majors move.) Node 24
is the action default. If `lastUpdated` is wanted, `fetch-depth: 0` is required
or every page claims the same date.

### 4.4 What Pages does not do

- **No PR previews.** Every deploy is the live site. If previews matter later,
  that is Cloudflare Pages/Netlify, not Pages.
- **No HTTP redirects** — only `meta refresh` from Astro's `redirects`, and only
  for pages that exist in the build. Old GitHub blob URLs (`docs/DESIGN.md`)
  will 404 after a move; git history keeps the content.
- **No custom headers** (CSP, cache-control). Fine for static docs.

Limits (from GitHub's docs): 1 GB published site, 100 GB/month soft bandwidth,
10-minute deploy timeout, 10 builds/hour — the build limit applies only to
branch-based builds, not Actions.

### 4.5 Search and analytics

Pagefind, built at deploy time, no service and no key. Index for a site this
size will be dominated by the WASM runtime (~700 KB for 9 pages in the spike);
that is a one-time cost on first search, acceptable, and can be trimmed by
excluding the research archive from the index if it gets noisy.

No analytics at launch. If wanted later, a cookie-free counter (GoatCounter or
Plausible) is a one-line script; anything heavier is not worth the weight.

---

## 5. Information architecture

### 5.1 Editorial principles

1. **Diátaxis split.** Tutorials (getting started), how-to (recipes),
   reference (config, keys), explanation (design). A page that is two of these
   is two pages.
2. **The repo's own standard: checked against the code.** Where `DESIGN-GUIDE`
   says "DESIGN.md wins on specifics because it is checked against the code",
   the site inherits that rule as a build-time test (§6.2).
3. **One screen of prose per idea.** Reference pages are tables first, prose
   second; guides are ≤1,200 words.
4. **Show the board, do not describe it.** ASCII blocks in the repo's own
   glyphs for row anatomy and legends (they are copy-pasteable and theme-proof),
   plus two real screenshots for atmosphere (dark and light).
5. **Keys always in `code` spans**, glyphs always in `code` spans, config always
   a runnable block with a comment on the non-obvious line.
6. **Measurements are dated.** Any number says what it was measured against and
   when, or it does not ship (`DESIGN.md` already does this).
7. **No marketing.** "One keybind away" is fine; "supercharge your workflow" is
   not.

### 5.2 Site map

`S` = source of truth, `M` = migrated/edited, `N` = new writing.

| path | page | job | source | est. words |
|---|---|---|---|---:|
| `/` | landing | what it is, demo, install CTA, 3 cards | N | 250 |
| `/getting-started/install/` | Install | requirements, macOS note, brew/go/checkout, verify | N (README Build) | 600 |
| `/getting-started/first-run/` | First run | repo inference, starter config, env overrides, what loads | N (config.go, starterConfig) | 500 |
| `/getting-started/board-tour/` | The board in 60 seconds | annotated board, header/row/status/footer, one keypress = one line | N (DESIGN §3.1, §3.9) | 600 |
| `/guide/rules/` | Rules and ordering | first-match-wins, ordering as configuration, limits, refresh, parallel fetch/ordered reveal | N (README, DESIGN §1) | 1,100 |
| `/guide/reading-the-board/` | Reading the board | glyph legend, colour tiers, author column, responsive tiers, footer | N (DESIGN §3.2–3.4, §3.8; help.go) | 1,300 |
| `/guide/search/` | Search | / semantics, what is searchable and why, prompt keys | M (README §Searching) | 900 |
| `/guide/detail/` | PR detail | failing/pending named, passing counted, conflicts, comments, size, branches | M (pr-detail.md §1, §3, §4.4) | 800 |
| `/guide/actions/` | Actions | templates, background vs suspend, status line, key precedence | M (README §Actions) | 900 |
| `/guide/actions/worktree/` | Worktree recipe | Worktrunk + herdr, branch-first, idempotence, sharp edges | M (README §Example) | 900 |
| `/guide/actions/review-tui/` | Review in a TUI | `tuicr` example, suspend mode | N | 300 |
| `/guide/team-board/` | A five-section team board | worked rule set, claims vs shows, `-review:approved`, authors list | M (config-example.md) | 1,500 |
| `/reference/config/` | Config file | top-level keys, types, defaults, env vars, precedence, location | N (config.go) | 900 |
| `/reference/rules/` | Rule fields | name/query/limit/tree/author, query syntax, limit-before-dedup | N | 900 |
| `/reference/actions/` | Action fields | key/name/run/mode, template variables, key matching order | N | 700 |
| `/reference/keys/` | Keys | four tables: board, search prompt, detail, help | N (model.go) | 900 |
| `/reference/cli/` | CLI and env | no flags today, `PILE_*`, exit codes, `gh` token | N | 350 |
| `/design/` | Design principles | colour budget, monochrome first, density | M (DESIGN-GUIDE) | 1,800 |
| `/design/decisions/` | Decisions and why | curated §2 table, grouped: data, colour, layout, interaction | M (DESIGN §2) | 2,400 |
| `/design/visual-spec/` | Visual spec | row anatomy, status cluster, colour tokens, tiers | M (DESIGN §3) | 2,000 |
| `/design/measurements/` | Measured facts | latency, limits, scroll margins across tools | M (DESIGN §6) | 600 |
| `/design/research/` | Research archive (index + 5 pages) | the long studies, prefaced as historical | M (5 memos) | 30,000 |
| `/help/troubleshooting/` | Troubleshooting | `gh auth`, IP allow-lists, unreadable repo at startup, Arc, clipboard, wide glyphs | N | 900 |
| `/help/limitations/` | Limitations | not built, known problems, divergences | M (DESIGN §7) | 700 |
| `/contributing/` | Contributing | build/test, package map, test conventions, commit style | N | 900 |

~30 pages; ~15k words of user-facing prose plus the archive. Roughly 40% of the
user-facing text exists already and needs editing, not writing.

### 5.3 README blueprint

Target: **≤150 lines, ~450 words**, down from 216 lines / 1,655 words.

```
# pile  ·  badge: docs · CI · release · license
A terminal PR board for GitHub. Rules are the board; one keypress is one line.
[ demo GIF, dark theme, 5s, ≤3MB, alt text ]
Homepage: <docs URL> · Design notes: <docs URL>/design/

## Install
macOS · `gh` logged in · Go 1.24+ to build
brew install barspielberg/tap/pile        # after the first release
go install github.com/barspielberg/pr-pile/cmd/pile@latest
# or: git clone … && make install

## Quickstart
1. `pile` in a checkout writes a starter config to ~/.config/pile/config.yml
2. set `repo:`, keep the four default rules
3. j/k move · enter open · d detail · / search · ? help · q quit
[ 10-line YAML ]

## What it does
3 bullets: first-match rules, one-line rows, parallel fetch with ordered reveal.

## Docs
Getting started · Configuration · Keys · Design notes · Limitations

## Status
personal tool, 0.x, no releases yet; macOS-first; not built: write actions,
multi-repo, preview pane. See limitations.

## License
MIT (pending — decision 3)
```

Moved out of the README: the search essay (→ `/guide/search/`), the worktree
recipe (→ `/guide/actions/worktree/`), the help-page scrolling details
(→ `/reference/keys/`), the rule-set walkthrough (→ `/guide/team-board/`).

### 5.4 Where the existing 47.7k words go

| existing | destination | treatment |
|---|---|---|
| `README.md` | landing pages + README | split: prose becomes guides, essentials stay |
| `docs/DESIGN.md` §1–§2 | `/guide/rules/`, `/design/decisions/` | edited, grouped by theme not by row order |
| `docs/DESIGN.md` §3 | `/design/visual-spec/` | published as-is (grown-up spec) |
| `docs/DESIGN.md` §4–§5 | `/reference/config/`, `/reference/keys/` | re-shaped into tables with types and defaults |
| `docs/DESIGN.md` §6 | `/design/measurements/` | dated, with "measured against acme/monorepo, Sep 2026" |
| `docs/DESIGN.md` §7 | `/help/limitations/` | edited; drop the superseded scroll section, keep the honest list |
| `docs/DESIGN-GUIDE.md` | `/design/` | light edit; it is already a principles document |
| `docs/config-example.md` | `/guide/team-board/` | edit for a public reader |
| `docs/pr-detail.md` | `/guide/detail/` (user part) + archive | extract §1/§3/§4.4; keep the rest as a study |
| `docs/{uniform-rows,section-layout,view-restructure,checks-page}.md` | `/design/research/*` | published as historical studies, each with a status banner (`superseded by …`) and a one-paragraph preface |

### 5.5 Sanitising before publication

- `acme/monorepo`, `web-platform` and `web-team` are already pseudonyms — keep
  them, and say in the archive preface that the board was measured against a
  private monorepo.
- `herdr` and `pr-status.sh` are personal tools; the recipe is still the honest
  example, but the page must say so.
- No tokens, no employer names, no absolute home paths beyond `~/Repos/…`.
- The memos contain "OVERRULED BY THE USER" and self-criticism. That candour is
  the value; edit only for the reader who was not there, not to sound
  authoritative.

### 5.6 Navigation, search, look

- Sidebar: explicit top-level groups (Getting started · Guide · Reference ·
  Design · Help · Contributing), `autogenerate` only inside Reference and the
  research archive.
- Landing: hero + demo + three cards (Rules, One-line rows, Detail overlay) +
  install block. Starlight's `splash` template.
- Look: the board's own palette rather than Starlight purple — accent from ANSI
  blue 4, muted text via opacity, monospace for everything that is a key,
  glyph, path or line of config. Light and dark must both be checked; the
  project's whole design argument is that light themes are not an afterthought.
- One logo mark: the tree glyph pair `╭╴` or the selection bar `▌` as a
  favicon-scale mark. No gradients, no glow, no illustration.
- OG image: a 1200×630 board crop with the wordmark and the tagline; regenerate
  from the same script as screenshots so it cannot drift.

---

## 6. Keeping the docs true

### 6.1 Structure after the move

```
docs/                        # Astro project (site source of truth)
  astro.config.mjs
  package.json               # lockfile committed
  src/content/docs/          # all pages, .md by default
  public/                    # demo.mp4, board-dark.png, board-light.png, og.png, favicon.svg
  assets/demo.tape           # VHS script that regenerates the GIF/mp4
DOCS-PLAN.md                 # this file
```

`docs/` becomes the site because it keeps one tree and one build, exactly as
gh-dash does. The alternative — site in `site/`, prose left in `docs/` — was
rejected: it duplicates or symlinks content, and the design notes would be
published from a second location.

### 6.2 Drift guards (the project's own standard, applied to docs)

| guard | mechanism | catches |
|---|---|---|
| config reference | a Go test parses `yaml:` tags in `internal/config/config.go` and asserts every key appears on its page — `Config` → `/reference/config/`, `Rule` → `/reference/rules/`, `Action` → `/reference/actions/` | an undocumented or renamed key |
| keys reference | a table in `internal/ui` (or a test that reads `model.go`'s key cases) asserts every binding appears on `/reference/keys/` | a key shipped but undocumented |
| glyph legend | the legend page is generated from the same helper `help.go` uses, or a test asserts the glyph set matches | a glyph changed in code, stale docs |
| links | `lychee` or `astro-broken-links-checker` in CI, internal links only | moved pages, dead anchors |
| screenshots | `docs/assets/demo.tape` + a `make docs-assets` target | screenshots older than the UI |
| build | docs build runs on any PR touching `docs/` | a broken deploy discovered after merge |

Two of these (config, keys) are worth writing on day one; the rest are cheap
additions in phase 6.

### 6.3 Versioning stance

No versioned docs at 0.x. One site, describing `main`, with a line on the
landing page: "no tagged releases yet; these docs track `main`." When the first
release exists, add `starlight-versions` and freeze that version — before then a
version selector is a lie.

### 6.4 Docs style rules (short, enforceable)

- One `H1` per page; sentence-case headings; no "Introduction" pages.
- Every page opens with one sentence saying what it is for.
- Config keys in `code`, with type and default in a table — never only inside
  an example.
- Numbers carry a date and a subject (`cold start, 6 rules, Sep 2026`).
- Banned: "simply", "just", "obviously", "powerful", "seamless".
- A page that needs a warning about a caveat says it in an `Aside`, not in a
  footnote.

---

## 7. Delivery plan

Each phase ends somewhere shippable; none requires the next.

| phase | work | output | acceptance | estimate |
|---|---|---|---|---|
| **0. Decisions** | answer §8 (framework, license, archive, install story, domain) | decisions recorded at the top of this file | no open "must" questions | 30 min |
| **1. Skeleton live** | Astro project in `docs/`, base path, theme tokens, sidebar, Pages enabled, workflow; landing + install + first run | URL serves three real pages | `https://barspielberg.github.io/pr-pile/` loads, install page works on a clean machine | 3–4 h |
| **2. Reference** | config, rules, actions, keys, CLI; config/key drift tests | five reference pages, every key documented | tests pass; no config field undocumented | 4–5 h |
| **3. Guides** | rules, reading the board, search, detail, actions (3 pages), team board | seven guide pages | a reader who has never run it can build a team board from the guide alone | 5–6 h |
| **4. README + assets** | VHS tape, GIF/mp4, dark+light screenshots, OG image, README rewrite | new README ≤150 lines, demo ≤3 MB | README renders correctly on GitHub, dark and light; no dead links | 2–3 h |
| **5. Design + help** | principles, decisions, visual spec, measurements, archive preface, troubleshooting, limitations, contributing | `/design/*`, `/help/*`, `/contributing/` | archive pages carry status banners; no unlabelled internal reference | 4–5 h |
| **6. Polish** | link-check CI, Pagefind tuning, a11y/perf pass, optional custom domain | green CI, Lighthouse ≥95 on perf/a11y/best-practices | one broken-link guard in CI; mobile nav usable | 2–3 h |

Total ≈ 21–26 hours, i.e. two to three focused days. Phase 1 is the go/no-go:
once the URL is live, everything else is writing.

---

## 8. Decisions needed from Bar

1. **Generator** — Starlight (recommended) or VitePress (lighter) or a Go-native
   option (Hugo/mdBook)? This decides the rest.
2. **Publish the research archive?** Recommended yes, curated and prefaced. The
   studies are the project's most distinctive writing; hiding them wastes the
   best content it has.
3. **License** — MIT for code (recommended; nothing today means "all rights
   reserved" by default) and CC BY 4.0 for docs, or MIT for both?
4. **Install story** — add `goreleaser` + a Homebrew tap before launch so the
   install page can promise `brew install`, or ship the docs saying "build from
   a checkout" for now (recommended: goreleaser, it is a day's work and it is
   what the docs need to be honest).
5. **Domain** — project URL `barspielberg.github.io/pr-pile/` now (recommended,
   zero DNS) or a custom domain at launch (needs a DNS record and your sign-off)?
6. **Platform claim** — document "macOS, tested" (recommended, honest) or port
   `open`/`pbcopy` to Linux first and document both?
7. **Screenshots** — public repo or a sanitised demo board for the captures? The
   board itself is the product; the safest is a public repo with real PR titles.

---

## 9. Risks and non-goals

**Risks**

- **Astro churn.** Astro 7 is current; gh-dash's docs are on 5. Pin versions,
  commit the lockfile, budget an hour per major upgrade.
- **Two toolchains.** Contributors now need Node for docs. Keep the docs build
  out of the Go path; `make check` must not require npm.
- **Content drift.** The docs restate behaviour that lives in code. Without the
  config/key tests, they will be wrong within a month.
- **Scope creep into a redesign.** The site should describe the tool as it is;
  the research archive is history, not a roadmap.
- **Publishing candour.** The memos are written for one reader; a light edit is
  needed so a stranger is not lost, without sanding off the specificity.
- **No PR previews** on Pages — typos go live. Mitigate with a local `npm run
  dev` habit and the link checker.

**Non-goals**

No blog, no i18n, no versioned docs before 1.0, no Algolia, no analytics, no
CMS, no hand-built theme, no documentation of unbuilt features (write actions,
multi-repo, preview pane, ascii glyph mode), no marketing landing page separate
from the docs.

---

## Appendix A — measurements taken for this plan

Local, 2026-11-06, Linux, Node 26.7.0 / npm 12.0.2, warm network:

| | Starlight | VitePress |
|---|---|---|
| install | 269 packages, 18.5 s, 199 MB | 125 packages, 13.6 s, 102 MB |
| build | 1.52 s, 9 HTML files | 1.58 s, 4 pages, 35 files |
| dist | 1.2 MB (Pagefind 696 KB) | 1.1 MB |
| base path | `site` + `base: '/pr-pile'` → `/pr-pile/…` in every asset and link | `base: '/pr-pile/'` works |
| failure mode to know | missing `src/content.config.ts` → cryptic sidebar slug error | none encountered |

Repo build check: `go build ./cmd/pile`, `go test ./... -short`, `go vet ./...`
all pass with Go 1.24.2 (`make` was absent in the research shell; the Makefile
targets themselves are simple and unchanged).

## Appendix B — sources

- GitHub API (`/repos`, `/git/trees`, `/contents`, `/releases`) — repo facts for
  pr-pile, gh-dash, lazygit, k9s, helix, fzf, jj, yazi, zellij, delta.
- gh-dash: `docs/astro.config.mjs`, `docs/src/content/docs/**`, sitemap.
- yazi: sitemap (`/docs/next/` versioned), Docusaurus meta.
- jj: `mkdocs.yml` (Material + mike + `edit_uri`), `docs.jj-vcs.dev`.
- helix and delta: `book/book.toml` (mdBook, `cname`).
- Zensical docs and the Material for MkDocs maintenance announcement;
  `squidfunk/mkdocs-material` issue #8523 (fixes through 2027-05-05).
- Starlight docs: site search (Pagefind), components; `starlight-versions`
  plugin.
- Astro docs: GitHub Pages deployment (`withastro/action`, `base`).
- GitHub docs: Pages limits; custom workflows.
- Diátaxis (tutorials / how-to / reference / explanation).

## Appendix C — config and workflow sketches

```js
// docs/astro.config.mjs
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

export default defineConfig({
  site: 'https://barspielberg.github.io',
  base: '/pr-pile',
  integrations: [
    starlight({
      title: 'pile',
      description: 'A terminal PR board for GitHub.',
      social: [{ icon: 'github', label: 'GitHub', href: 'https://github.com/barspielberg/pr-pile' }],
      editLink: { baseUrl: 'https://github.com/barspielberg/pr-pile/edit/main/docs/' },
      sidebar: [
        { label: 'Getting started', items: ['getting-started/install', 'getting-started/first-run', 'getting-started/board-tour'] },
        { label: 'Guide', items: ['guide/rules', 'guide/reading-the-board', 'guide/search', 'guide/detail', 'guide/actions', 'guide/team-board'] },
        { label: 'Reference', autogenerate: { directory: 'reference' } },
        { label: 'Design', autogenerate: { directory: 'design' } },
        { label: 'Help', autogenerate: { directory: 'help' } },
        { slug: 'contributing' },
      ],
    }),
  ],
});
```

```yaml
# .github/workflows/docs.yml
name: Docs
on:
  push: { branches: [main], paths: ['docs/**'] }
  workflow_dispatch:
permissions: { contents: read, pages: write, id-token: write }
concurrency: { group: pages, cancel-in-progress: true }
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
        with: { fetch-depth: 0 }   # lastUpdated
      - uses: withastro/action@v6
        with: { path: docs }
  deploy:
    needs: build
    runs-on: ubuntu-latest
    environment: { name: github-pages, url: '${{ steps.deployment.outputs.page_url }}' }
    steps:
      - id: deployment
        uses: actions/deploy-pages@v5
```

## Appendix D — writing backlog (new pages, and the facts each must get right)

| page | must state | verify against |
|---|---|---|
| install | macOS today; `gh` for token only; Go 1.24+; no releases yet | `browser.go`, `github.New`, `go.mod` |
| first run | config path, XDG, `PILE_CONFIG`, `PILE_REPO`, repo inference, starter file | `config.Load`, `starterConfig` |
| board tour | 20-row target, slot model, header-as-position, footer right field | `DESIGN.md` §3.1/§3.5/§3.9, `render.go` |
| rules | first-match-wins, ordering, limit-before-dedup, `tree` scope, refresh, frontier | `board.go`, `config.go` |
| reading the board | four glyph silhouettes, colour tiers, author column per rule, tiers/drops | `DESIGN.md` §3.2–§3.4/§3.8, `help.go` |
| search | haystack = drawn text; initials only; age never; width-dependent; prompt keys | `search.go`, `model.go` |
| detail | failing/pending named, passes counted, conflict flag, unresolved count, size, branches | `detail.go`, `github.go` |
| actions | `sh -c`, template vars, background vs suspend, status line, keys matched last | `model.go`, `config.go` |
| config | every key, type, default, precedence, env | `config.go` |
| keys | four contexts, chords while typing, `d` closes on any key | `model.go`, `help.go` |
| troubleshooting | `gh auth token`, org IP allow-list → startup error (`repo not found, or you cannot see it`), Arc, pbcopy, empty section while loading | `main.go` (`CheckRepo`), `github.go`, `browser.go` |
| limitations | deferred review progress, no write actions, single repo, hardcoded margin, spec divergences | `DESIGN.md` §7 |
