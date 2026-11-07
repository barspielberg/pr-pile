---
title: Rules
description: The fields of a rule, and how rule ordering shapes the board.
---

A rule is one section of the board. This page lists its fields; the top-level
`rules:` list and the default rules are on the [config](../config/) page.

## Fields

| field | type | default | meaning |
| --- | --- | --- | --- |
| `name` | string | none — required | The section title. |
| `query` | string | none — required | GitHub search syntax for this section. |
| `limit` | int | `20` | How many PRs to fetch for this rule. |
| `tree` | bool | `false` | Group stacked PRs into a chain. |
| `author` | bool | `false` | Show the author's initials on each row. |

## Ordering is configuration

Rules are an ordered list, and a PR is shown under the **first** rule that
matches it. Moving a rule up widens it at the expense of everything below, so
the order is the configuration. The default rules put the cheapest,
highest-value queries first.

## Query

`query` is GitHub search syntax, written as only the part that distinguishes
the rule. pile scopes it to `repo:<repo> is:pr is:open` for you, so:

```yaml
- name: Mine
  query: author:@me
```

becomes the search `repo:owner/name is:pr is:open author:@me`. You never write
the repo or the open-PR filter.

The starter config shows the qualifiers that matter on a team board:
`review-requested:@me`, `-review:approved`, `involves:@me`,
`team-review-requested:ORG/TEAM`, and `author:a author:b` (which ORs).

## Limit

`limit` is the number of PRs fetched for a rule, not a cap applied after the
board is built. It truncates **before** first-match dedup, so a rule with a low
limit can hand a PR to a later rule that would not have matched it otherwise.

A `limit` of `0` or less fetches the default of `20`.

Keep a `tree: true` rule's limit above the number of PRs it matches: a chain is
worked out from the PRs in that section only, and a truncating limit can lose a
stack's base and regroup it into a shorter chain.

## Tree

`tree: true` groups stacked PRs into a chain. The chain is worked out from the
PRs in that section only.

## Author

`author: true` adds an initials column to the rule's rows. It is off by default
and set per rule: a rule like `author:@me` is all one person, so the column
would be dead weight there.
