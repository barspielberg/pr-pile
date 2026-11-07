---
title: Config
description: Config keys, repositories, notifications, defaults, and validation.
---

The config file declares the repositories the board reads, its ordered rules,
actions and watch notifications. See [Rules](../rules/) and [Actions](../actions/)
for the fields of those lists.

## Keys

| key | type | default | meaning |
| --- | --- | --- | --- |
| `repos` | list | inferred on first run | Repositories the board reads; at least one is required. |
| `refresh` | duration | `3m` | Refetch interval; `0` disables automatic refresh. |
| `rules` | list | four starter rules | Ordered sections with first-match-wins bucketing. |
| `actions` | list | empty | Shell commands bound to keys. |
| `watch` | object | terminal notifications | How watched PR changes are announced. |

## Repositories

```yaml
repos:
  - name: owner/web
    path: ~/Repos/owner/web
  - name: owner/api
    label: api
refresh: 3m
```

Each entry has a required `name` in exactly `owner/name` form, an optional
checkout `path`, and an optional `label`. The label defaults to the part after
the slash. Names and labels must be unique, ignoring case, and labels cannot
contain whitespace. Every configured repo is checked at startup.

With multiple repos each row shows its repo label; with one repo that column
is omitted. Rules cover all repos unless their own `repos` list selects labels.
Actions use the selected PR's repository and checkout path.

The old top-level `repo` and `repoPath` fields are rejected with a migration
message. Move them into `repos` as `name` and `path`. `PILE_REPO` no longer
overrides the board: edit the list, or use a separate file through `PILE_CONFIG`.

## Location and precedence

The file is found by, in order:

1. `$PILE_CONFIG`, if set — the full path.
2. `$XDG_CONFIG_HOME/pile/config.yml`, if `$XDG_CONFIG_HOME` is set.
3. `~/.config/pile/config.yml`.

On first run in a GitHub checkout, pile asks `gh repo view` for its name and
writes a starter file with one `repos` entry. Outside a checkout, write the
file yourself. The file is decoded over the defaults; an explicit `rules`
list replaces the default list wholesale.

## Environment variables

| variable | effect |
| --- | --- |
| `PILE_CONFIG` | Full path to the config file. |
| `XDG_CONFIG_HOME` | Replaces `~/.config` when locating the file. |

## Watch notifications

`m` watches the selected PRs, or the one under the cursor, and presses again
stop watching. Watches are checked on every timed refresh and `r`, and end
when pile quits. Changes appear on the status line and through `watch`:

| field | default | meaning |
| --- | --- | --- |
| `notify` | `osc` | `osc` sends OSC 9 to the terminal; `command` runs a notifier; `none` disables external notifications. |
| `command` | empty | Required when `notify` is `command`; executed through `sh -c`. |

```yaml
watch:
  notify: command
  command: herdr notification show "$PILE_TITLE" --body "$PILE_MESSAGE"
```

The command receives `$PILE_TITLE`, `$PILE_MESSAGE` (the PR title), `$PILE_URL`
and `$PILE_NUMBER`. Quote these variables; keep remote text out of the command
itself. Multiplexers may not pass OSC 9 through, so use a command notifier there.

## The default rules

| name | query | limit | tree | author |
| --- | --- | --- | --- | --- |
| Mine | `author:@me` | 50 | true | — |
| Needs my review | `review-requested:@me -review:approved` | 50 | — | true |
| Involved | `involves:@me -author:@me` | 20 | — | true |
| All open | `draft:false` | 20 | — | true |

## Validation

Unknown fields and multiple YAML documents are errors. Repositories require
valid, unique names and labels. Rules require a name and query; limits must be
`0` (the default 20) or `1`–`100`, and `repo:` qualifiers belong in a rule's
`repos` list instead. Action keys cannot shadow built-ins or duplicate another
action; names and commands are required, and mode is `background` or `suspend`.
