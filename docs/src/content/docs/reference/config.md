---
title: Config
description: Every top-level key in config.yml, its type, default, and how pile resolves the file.
---

The config file is where the whole board comes from: the repo, the rules, and
any actions. This page is the reference for the top-level keys; the fields of a
rule and an action have their own pages.

## Keys

| key | type | default | meaning |
| --- | --- | --- | --- |
| `repo` | string | none — required | The repo to show, as `owner/name`. |
| `repoPath` | string | empty | The local checkout path, exposed to action templates as `{{.RepoPath}}`. |
| `refresh` | duration | `3m` | How often the board refetches. `0` disables auto-refresh. |
| `rules` | list | the four default rules | Ordered sections. A PR is shown under the first rule that matches it. |
| `actions` | list | empty | Shell commands bound to keys, run for the selected PR. |

`repo` is required. On first run, pile infers it from the current GitHub
checkout and writes it to the starter config. Set `$PILE_REPO` to override
`repo:` for one run, even when the file already sets it.

## Location and precedence

The file is found by, in order:

1. `$PILE_CONFIG`, if set — the full path.
2. `$XDG_CONFIG_HOME/pile/config.yml`, if `$XDG_CONFIG_HOME` is set.
3. `~/.config/pile/config.yml`.

The file is decoded **over the defaults**, so a key left out keeps its default
rather than zeroing. An explicit `rules:` list replaces the default rules
wholesale — it does not merge with them. After the file loads, `$PILE_REPO`
overrides `repo:` for that run only; it does not edit the file.

pile does not use `os.UserConfigDir`, which on macOS returns
`~/Library/Application Support` rather than where a terminal tool's config
belongs.

## Environment variables

| variable | effect |
| --- | --- |
| `PILE_CONFIG` | Full path to the config file. |
| `PILE_REPO` | Overrides `repo:` for one run. |
| `XDG_CONFIG_HOME` | Replaces `~/.config` when locating the file. |

## The default rules

With no config file, pile writes the starter file and runs these four rules,
with `repo:` filled in from the checkout:

| name | query | limit | tree | author |
| --- | --- | --- | --- | --- |
| Mine | `author:@me` | 50 | true | — |
| Needs my review | `review-requested:@me -review:approved` | 50 | — | true |
| Involved | `involves:@me -author:@me` | 20 | — | true |
| All open | `draft:false` | 20 | — | true |

The meaning of each rule field is on the [rules](../rules/) page.

## Validation

A config that does not validate exits with an error that names the problem. A
valid config needs:

- `repo:` in the form `owner/name`.
- at least one rule, each with a `name:` and a `query:`.
