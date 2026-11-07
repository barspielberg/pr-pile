---
title: First run
description: What pile does the first time it starts, and the config it writes.
---

On first run pile has no config, so it infers the repo from your checkout and
writes a commented starter config you can edit. This page describes that from
`config.Load` and the `starterConfig` it writes.

## Config location

- `~/.config/pile/config.yml` by default, honouring `$XDG_CONFIG_HOME`.
- `$PILE_CONFIG` overrides the whole path.

pile deliberately avoids `os.UserConfigDir` (on macOS that is
`~/Library/Application Support`); the config belongs with your other terminal
tooling.

## Repo inference

Run `pile` inside a checkout of a GitHub repo. It asks `gh repo view` for the
`owner/name` and writes a starter `repos` list containing that repository.

Outside a checkout, write a config with a `repos` list yourself. Add more
repositories to that list to show them together. Use `PILE_CONFIG` to choose
a different board configuration; `PILE_REPO` is no longer an override.

## The starter config

First run writes a commented starter file. The four rules it writes, with the
comments elided:

```yaml
repos:
  - name: owner/name
refresh: 3m

rules:
  - name: Mine
    query: author:@me
    tree: true
    limit: 50

  - name: Needs my review
    query: review-requested:@me -review:approved
    author: true
    limit: 50

  - name: Involved
    query: involves:@me -author:@me
    author: true
    limit: 20

  - name: All open
    query: draft:false
    author: true
    limit: 20
```

These mirror the built-in defaults: your PRs, PRs asking for your review, PRs
you are involved in, and everything else open. Queries are GitHub search
syntax, scoped to `is:pr is:open` and the configured repo automatically — you
write only the part that distinguishes the rule.

## What loads

- The file is decoded **over the defaults**, so an absent key keeps its default
  rather than zeroing.
- An explicit `rules:` list replaces the defaults wholesale.
- Unknown fields are rejected. A valid config needs a `repos` list with at
  least one `name` in the form `owner/name`, and at least one rule
  with a `name:` and a `query:`. Anything else exits with an error that names
  the problem.
