---
title: Badges
description: The fields of a badge, its template variables, and how it runs.
---

A badge is a shell command whose output is drawn on a row, after the title. It
takes the same template fields as an [action](../actions/), runs in the
background, and its answer is kept for a while, so a slow script makes a badge
late, never the board slow.

## Fields

| field | type | default | meaning |
| --- | --- | --- | --- |
| `name` | string | none — required | Names the badge in a rule's `badges` list and on the help page. |
| `run` | string | none — required | The shell command, with template fields filled in. |
| `color` | string | `muted` | `fg`, `muted`, `ok`, `attention`, `error` or `accent`. |
| `side` | string | `left` | `left`, right after the title, or `right`, just before the author. |
| `max_width` | int | `20` | Longer output is cut with `…`. |
| `ttl` | duration | `5m` | How long an answer is kept before the script runs again. |
| `timeout` | duration | `10s` | How long the script may run before it is killed. |

`run` executes through `sh -c`. Names must be unique. A template that does not
parse, or that uses a plural field such as `{{.Numbers}}`, is a configuration
error.

## Template fields

`run` takes the singular [action fields](../actions/#template-fields):
`{{.Number}}`, `{{.Repo}}`, `{{.RepoPath}}`, `{{.Branch}}`, `{{.Base}}`,
`{{.URL}}` and `{{.Author}}`. They are checked the same way, so a PR whose
branch or author fails its check shows no badge on its row.

There is no `{{.Title}}`. A script that needs the title can ask for it:

```sh
gh pr view {{.Number}} --repo {{.Repo}} --json title --jq .title
```

## Output

The first line of stdout is the badge. Empty output draws nothing.

## Which rules show a badge

A rule without a `badges` key shows every badge. A rule with `badges: [jira]`
shows only those, and `badges: []` shows none:

```yaml
badges:
  - name: jira
    run: jira-status {{.Branch}}
    color: accent

rules:
  - name: Mine
    query: author:@me
  - name: All open
    query: draft:false
    badges: []
```

## Running

At most `badge_workers` scripts run at once (default 4). Rows in higher rules
get their badges first, then rows in board order. An answer stays on screen
until the script answers again after its `ttl`.

A badge runs on every PR it applies to without a keypress, including PRs opened
by other people. What the script does with the PR is up to you: a badge that
checks out the branch and runs it would run someone else's code. Use a rule's
`badges` list to keep such a badge to your own PRs.
