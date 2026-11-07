---
title: Worktree recipe
description: Opening a PR as a worktree workspace, with Worktrunk and herdr.
---

This recipe binds a key that opens the selected PR as a worktree workspace. It
uses two tools the project does not ship — [Worktrunk](https://github.com/dkarter/worktrunk)
(`wt`) and `herdr`, a personal tool for opening workspaces — and they are the
honest example rather than a recommendation. Substitute whatever your own
worktree flow is; the shape of the recipe is what matters.

```yaml
repoPath: ~/Repos/owner/name    # {{.RepoPath}} is empty without this

actions:
  - key: w
    name: worktree
    run: $HOME/path/to/pr-workspace {{.RepoPath}} {{.Number}} {{.Branch}}
```

Give the script by path, not by bare name: the action runs through `sh -c` with
whatever `PATH` the board inherited.

## The script

Two steps, and both are already idempotent, which is what makes this safe to
bind to a key that will get pressed twice:

```sh
p="$(wt switch "$3" --no-cd --format json | jq -r '.path // empty')"
[ -n "$p" ] || p="$(wt switch "pr:$2" --no-cd --format json | jq -r '.path // empty')"
exec herdr worktree open --cwd "$1" --path "$p" --no-focus
```

**Pass the branch and prefer it.** `wt switch pr:<n>` resolves the number
through the GitHub API on *every* call, even when the worktree already exists —
7.0s against 0.07s for `wt switch <branch>` on that same worktree. The board
already knows the head ref, so `{{.Branch}}` skips the round trip. Keep `pr:<n>`
as the fallback: a branch resolves only if it is on `origin`, so a PR from a
fork still needs the number.

## The `herdr` half

`herdr worktree open` returns the existing workspace for a path it has already
opened, so no dedupe logic is needed. Use `worktree open` (a **workspace**), not
`herdr tab create` (a tab in whatever workspace you are in); `tab create` is not
idempotent and opens a second tab on every press.

`--no-focus` matters for a key binding: opening the workspace focused yanks you
out of whatever you were doing on every press.

Leave it on the default `background` mode. `suspend` is for commands that take
over the terminal; this one only talks to the herdr socket and exits, so
suspending would blank the board for a few seconds to run something that never
wanted the terminal.

## One sharp edge that is not pile's doing

`wt remove` deregisters a worktree but can leave the directory behind (a
`node_modules` from a post-start hook is enough), and the next `wt switch pr:<n>`
then fails with *Directory already exists* rather than reusing it. If the key
stops working on a PR you have removed before, clear the leftover directory.
