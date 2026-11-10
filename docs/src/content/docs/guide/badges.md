---
title: Badges
description: Showing a script's answer on every row — a ticket status, a deploy, a local branch.
---

A badge is a short piece of text a script prints for each PR, drawn on the row
after the title. pile cannot know your Jira status or whether you have a branch
checked out locally, but a one-line command can. The fields are on the
[Badges](../../reference/badges/) reference page; this page is how they behave.

```yaml
badges:
  - name: jira
    run: jira-status {{.Branch}}
    color: accent
  - name: local
    run: git rev-parse --verify --quiet {{.Branch}} >/dev/null && echo local
    side: right
    color: attention
```

`jira-status` stands for a script of your own. No badges are configured by
default.

## Where a badge goes

A `left` badge, the default, sits right after the title text. A `right` badge
sits just before the author initials, or at the end of the row on a rule
without an author column. The title starts in the same column either way, so
a badge arriving never moves the rest of the row.

When the title and its badges do not fit, the title is cut first, down to 24
cells, and then badges are dropped from the last one in config order. On a
terminal narrower than 60 columns badges are not drawn at all.

A badge is as wide as its text, cut with `…` past `max_width`.

## When scripts run

pile never waits for a badge. Scripts run in the background, at most
`badge_workers` at a time, and the board draws whatever answers it has. Rows
in higher rules get their badges first, then rows in board order.

An answer is kept for the badge's `ttl` and stays on screen while the script
runs again. A new commit or a new title does not make it run early, and nor
does the board's timed refresh. `r` does: it reruns every badge alongside the
board's refetch, and the old answers stay up until new ones land. A script
that fails keeps its last answer on the row and tries again within a minute.

A script that runs past `timeout` is killed with everything it started. So is
every script still running when you quit.

## What the script sees

`run` takes the singular [action fields](../actions/#template-fields), checked
the same way, so a branch or author that fails its check leaves the badge
blank on that row. There is no `{{.Title}}`; ask `gh` for it if you need it.

A script runs in the PR's checkout when its repo has a `path`, with no
terminal: anything that would prompt fails instead. `GIT_TERMINAL_PROMPT=0` and
`GIT_OPTIONAL_LOCKS=0` are set, so git neither asks for a password nor takes
the index lock your own commit needs.

The first line of output is the badge. Colours, escape sequences and
characters that change how text reads, such as right-to-left overrides, are
removed.

## Seeing what a badge said

`d` on a row shows each badge in full, how long ago it answered, and the last
error, which the row cannot show. `?` lists the configured badges, since there
is no column header to say what one is. A badge that fails on every row is
named in the footer, since an empty column would otherwise look like a script
with nothing to say. A search matches badge text like any
other text on the row.

## Badges on other people's PRs

A badge runs on every PR it applies to without a keypress, including PRs on a
rule like `All open` that anyone can open. pile checks the fields it passes,
but what the script does with the PR is up to you: a badge that checks out a
branch and runs it would run a stranger's code on every refresh. Keep such a
badge to your own rules with a rule's `badges` list:

```yaml
rules:
  - name: Mine
    query: author:@me
  - name: All open
    query: draft:false
    badges: []
```
