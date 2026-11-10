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
| `multi` | bool | `false` | Run once for the whole selection using plural fields. |

`run` executes through `sh -c`. Empty commands, duplicate keys and keys
reserved by the board are configuration errors.

## Template fields

`run` is a Go text template. The fields available are:

| field | value |
| --- | --- |
| `{{.Number}}` | the PR number |
| `{{.Repo}}` | the PR's own repo name from `repos` |
| `{{.RepoPath}}` | that repo's checkout `path` |
| `{{.Branch}}` | the PR's head branch |
| `{{.Base}}` | the PR's base branch |
| `{{.URL}}` | the PR's URL |
| `{{.Author}}` | the author's login |

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

  - key: W
    name: review
    run: tuicr pr {{.Number}}
    mode: suspend
```

## GitHub fields and repository context

`Branch`, `Base`, `URL` and `Author` come from GitHub, so whoever opened the PR
chooses them. pile passes each one only if it contains no shell syntax, and the
action refuses with the offending value otherwise:

| field | accepted |
| --- | --- |
| `Branch`, `Base` | a letter or digit, then letters, digits and `. _ / + @ , -` |
| `URL` | `https://github.com/<owner>/<repo>/pull/<number>` |
| `Author` | a GitHub login: letters, digits and `-` |

A value that passes is safe anywhere in the command: quoted, embedded as
`pr-{{.Branch}}`, or handed to `ssh` or `sh -c`. It is not quoted for you, and
the full template syntax (`if`, `with`, `range`, `printf`) is available.
`Repo`, `RepoPath` and the command itself are trusted configuration text.

The PR title is not a field: it is free text, so no check could make it safe
in a command. A script that needs it can look it up:

```sh
gh pr view {{.Number}} --repo {{.Repo}} --json title --jq .title
```

Actions run in the PR's checkout when its repo has a `path`. An action using
`RepoPath` refuses a repo without one. On a multi-repo board, an action using
a PR number, branch or base without a checkout must explicitly name the repo
with `Repo` or `RepoPath`.

## Multiple selected PRs

An action without `multi: true` refuses a selection containing several PRs.
A multi action runs once using space-joined plural fields, one word per PR and
each checked like its singular form: `{{.Numbers}}`, `{{.URLs}}`,
`{{.Branches}}`, `{{.Bases}}`, `{{.Authors}}`. Mixing singular and plural fields
is an error. With no selection, the row under the cursor is a list of one.

Numbers, branches, bases and repository paths are ambiguous across repositories,
so a multi action using those fields refuses a selection spanning repos.
Actions using only URLs or authors can span repos. Acting on three or more PRs
asks for confirmation first.
