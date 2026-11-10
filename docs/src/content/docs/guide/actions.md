---
title: Actions
description: Binding a shell command to a key, and what the footer reports while it runs.
---

An action is a shell command bound to a key. pile knows nothing about worktrees,
editors or multiplexers — it renders a template and runs whatever it gets. The
fields are on the [Actions](../../reference/actions/) reference page; this page is
how they behave, with two worked recipes: a [worktree](worktree/) and a
[review TUI](review-tui/).

```yaml
actions:
  - key: w
    name: worktree
    run: wt switch -x nvim pr:{{.Number}}
  - key: W
    name: review
    run: tuicr pr {{.Number}}
    mode: suspend
```

No actions are bound by default. The tool would have to guess at a command that
exists on your machine, and a default that shells out to a script only the
author has would fail with `command not found` on anyone else's.

## Template fields

`run` is a Go text template, filled in for the selected PR:

| field | value |
| --- | --- |
| `{{.Number}}` | the PR number |
| `{{.Repo}}` | the PR's own repo name from `repos` |
| `{{.RepoPath}}` | that repo's checkout `path` |
| `{{.Branch}}` | the PR's head branch |
| `{{.Base}}` | the PR's base branch |
| `{{.URL}}` | the PR's URL |
| `{{.Author}}` | the author's login |

The rendered command runs as `sh -c "<run>"` with whatever `PATH` the board
inherited — which is not necessarily the one an interactive shell has. Give a
script by full path, not by bare name.

## Background vs suspend

- `background` (the default) runs the command while the board stays responsive.
  The footer shows the board's own spinner and the action's `name` while it
  runs, then `name ✓` on success or `name failed: <last line of stderr>` on
  failure.
- `suspend` hands the terminal over to the command and repaints the board when
  it exits. Use it for commands that take over the terminal — a pager, a diff,
  a review TUI.

Success is reported and then clears itself after a few seconds. Failure
persists, because it is the one you have to act on. On failure the footer shows
the command's own last line of stderr rather than Go's `exit status 1`, which
names the mechanism rather than the problem.

## Key matching

Actions are matched **last**, after every navigation and built-in key, so a
configured action cannot shadow `j`, `d`, `/` or the rest. Binding an action to
`d` or `j` is a configuration error; pick a key the [Keys](../../reference/keys/)
table does not use. `v` and `m` are also reserved for selection and watches.

Branch, base, URL and author values are passed only when they contain no shell
syntax, so they are safe anywhere in the command; an odd one refuses the action.
The PR title is not passed at all. Actions run in the PR's configured checkout.
See the [reference](../../reference/actions/#github-fields-and-repository-context)
for quoting restrictions and `multi: true` selection behavior.

A second press of the same key while an action is still running is a no-op with
a `still running` message rather than a second process.
