---
title: Actions
description: The fields of an action, its template variables, and how it runs.
---

An action is a shell command bound to a key. pile knows nothing about worktrees
or editors; it renders the template and runs whatever it gets.

## Fields

| field | type | default | meaning |
| --- | --- | --- | --- |
| `key` | string | none | The key that runs the action. |
| `name` | string | none | The label shown in the footer and the help page. |
| `run` | string | none | The shell command, with template fields filled in. |
| `mode` | string | `background` | `background` or `suspend`. |

`run` is executed as `sh -c "<run>"` for the selected PR. An action with an
empty `run` is ignored.

## Template fields

`run` is a Go text template. The fields available are:

| field | value |
| --- | --- |
| `{{.Number}}` | the PR number |
| `{{.Repo}}` | `owner/name` from `repo:` |
| `{{.RepoPath}}` | the local checkout path from `repoPath:` |
| `{{.Branch}}` | the PR's head branch |
| `{{.Base}}` | the PR's base branch |
| `{{.URL}}` | the PR's URL |
| `{{.Author}}` | the author's login |
| `{{.Title}}` | the PR title |

## Mode

- `background` (the default) runs the command while the board stays responsive.
  The footer shows a spinner and the action's `name` while it runs, then
  `name ✓` on success or `name failed: <last line of stderr>` on failure.
- `suspend` hands the terminal over to the command and repaints the board when
  it exits. Use it for TUI commands such as a diff pager or a review session.

## Key matching

Actions are matched **last**, after every navigation and built-in key, so a
configured action cannot shadow `j`, `d`, `/` or the rest.

A second press of the same key while an action is still running is a no-op with
a `still running` message, rather than a second process.

```yaml
actions:
  - key: w
    name: worktree
    run: wt switch -x nvim pr:{{.Number}}

  - key: v
    name: review
    run: tuicr pr {{.Number}}
    mode: suspend
```
