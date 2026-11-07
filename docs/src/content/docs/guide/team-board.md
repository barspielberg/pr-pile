---
title: A five-section team board
description: A worked rule set, and why it is ordered the way it is.
---

A complete board for someone who writes code, reviews for a small team, and
wants to keep an eye on two wider teams without drowning in them. Copy the
config, edit `repo`, `repoPath` and the logins, and run it.

The repo, team names and logins below are stand-ins, not a real team. The
counts are real measurements from a private monorepo, taken Sep 2026, kept
because the *shape* is the point — no section is empty, and no section swallows
the one below it. They will drift; treat them as illustrative proportions, not
numbers to reproduce.

## The config

```yaml
repo: acme/monorepo
repoPath: ~/Repos/acme
refresh: 3m

rules:
  # Your own PRs. No author column: it is all you.
  - name: Mine
    query: author:@me
    tree: true
    limit: 50

  # Your team, only what is actually asking for review. draft:false because a
  # draft is not requesting anything yet, and -review:approved to drop what is
  # already signed off. PRs with changes requested stay -- those still want work.
  - name: My team needs review
    query: >-
      author:alicechen author:barspielberg author:bobnak
      author:frank-oye author:grace-park
      author:heidimeyer
      -review:approved draft:false
    tree: true
    author: true
    limit: 50

  # The same people, no review qualifier. The rule above already took the ones
  # needing review, so this is the remainder: drafts and approved work.
  - name: My team
    query: >-
      author:alicechen author:barspielberg author:bobnak
      author:frank-oye author:grace-park
      author:heidimeyer
    tree: true
    author: true
    limit: 50

  # Both platform teams, by enumerating their members: GitHub has no qualifier
  # for "authored by a team". Only the members not claimed above ever show here.
  - name: All platform
    query: >-
      author:alicechen author:barspielberg author:bobnak
      author:frank-oye author:grace-park
      author:heidimeyer
      author:ivanpetrov author:cdiaz88 author:judy-f-2
      author:mallory-q author:niajrahman author:oscar-lin
    tree: true
    author: true
    limit: 50

  # Everything else open and not a draft.
  - name: Open
    query: draft:false
    author: true
    limit: 30
```

## What each rule claims, and what it actually shows

`raw` is what the query returns on its own; `shown` is what survives after every
rule above it has taken its share. The gap between the two columns *is*
first-match-wins.

| # | rule | raw | shown |
| --- | --- | ---: | ---: |
| 1 | Mine | 5 | **5** |
| 2 | My team needs review | 9 | **8** |
| 3 | My team | 33 | **20** |
| 4 | All platform | 45 | **12** |
| 5 | Open | 30 | **17** |

Rule 2 returns 9 and shows 8, because one of them is yours and rule 1 took it.
Rule 3 returns 33 and shows 20: the 8 rule 2 claimed, plus that same PR of
yours, are gone. Rule 4 returns 45 and shows 12 — **every web-platform member is
already covered by rules 1–3, so what is left is exactly the second team's
work**:

| author | rows |
| --- | ---: |
| `oscar-lin` | 5 |
| `cdiaz88` | 4 |
| `mallory-q` | 2 |
| `judy-f-2` | 1 |

That is worth knowing before editing rule 4: dropping the second team's members
from it does not narrow the section, it **empties** it. Check `shown` after any
reorder — a rule showing 0 looks broken even when it is working correctly.

Rule 5's `raw` is 30 because that is its `limit` — 66 open non-draft PRs match
it, and the board fetches the newest 30. It is the catch-all, so it is the one
rule where truncating is fine; it carries no `tree`, so there is no chain to cut.

## Why the order is what it is

Every rule here is a superset of the one above it, which is what makes the board
readable: each section is "the rest of" the previous one.

- **Mine** first because it is the smallest and fastest query, and the one you
  usually opened the tool for. Rules are revealed in order, so a slow rule at
  the top stalls everything under it.
- **My team needs review** above **My team** is the whole trick in rule 3. Rule
  3 carries no review qualifier at all — it does not need one. Anything of
  theirs that needed review was claimed one line up, so what lands in rule 3 is
  the remainder by construction. Swap those two rules and rule 2 goes
  permanently empty.
- **All platform** below both, so web-platform stays in its own section rather
  than being diluted into the wider group. Rule 4's list is a superset of rules
  2–3, and first-match-wins is the whole reason that is fine: everyone shared
  between them is claimed above, so rule 4 shows only the second team.
- **Open** last as the catch-all.

## Why `-review:approved` and not `review:required`

Three states mean "nobody has signed this off": `REVIEW_REQUIRED`,
`CHANGES_REQUESTED`, and no decision at all. On the team's 22 unapproved PRs,
`review:required` returns 19 — it drops all three of these:

| PR | review state | dropped by `review:required` |
| --- | --- | --- |
| #2859 | `CHANGES_REQUESTED` | yes |
| #3006 | none | yes |
| #3224 | none | yes |

A PR with changes requested still wants work, and a PR nobody has been asked
about is the most "needs review" state there is. `-review:approved` keeps all
three by subtracting only the settled case.

`draft:false` is the other half. Of the 22 unapproved PRs, 14 are drafts —
without the filter the section is 64% work that is not asking for anything, and
the 8 rows that *are* actionable are buried. The drafts are not lost; they fall
through to rule 3. Dropping `draft:false` moves the split from 8/20 to 22/6; it
is filtered here because a draft is by definition not requesting review.

## Why authors, not `team-review-requested`

`team-review-requested:acme/web-platform` means "this team was asked to review
this PR". It does **not** mean "this PR came from that team". Measured on the
same board the two sets barely agree:

| | count |
| --- | ---: |
| authored by the team, review never asked of the team | 19 |
| review asked of the team, author is outside it | 1 |
| both | 26 |

The 19 are the point: using `team-review-requested` for rule 4 would drop all of
them.

**GitHub search cannot express "authored by any member of this team", and there
is no dynamic form of it.** `author:acme/web-platform`, `team:acme/web-platform`
and `author:@acme/web-platform` all return **0** rather than an error, against 12
for `team-review-requested:` on the same team. Enumerating members is the only
shape that works. Two things to know before you rely on it:

- Repeating the *same* qualifier ORs: `author:alicechen author:heidimeyer`
  returns 19, which is 15 + 4, the two counts added. That is what makes the
  enumeration work.
- Mixing *different* qualifiers ANDs: `author:alicechen team-review-requested:…`
  returns 3, not 27. So you cannot write one rule meaning "from the team **or**
  asked of the team" — it would have to be two rules, with the usual
  first-match-wins split between them.

### The list goes stale, and nothing warns you

Because it is enumerated rather than resolved, the board keeps showing the old
team until someone edits this file. A new joiner's PRs fall through to `Open`; a
leaver keeps a section they no longer belong in. There is no error — it quietly
shows the wrong set, which is the failure mode hardest to notice. Treat the
refresh as a standing chore whenever either team changes:

```sh
# rules 2 and 3 -- the web-platform team
gh api orgs/acme/teams/web-platform/members -q '.[].login' \
  | sed 's/^/author:/' | tr '\n' ' '

# rule 4 -- both teams
for t in web-platform web-platform-eu; do
  gh api "orgs/acme/teams/$t/members" -q '.[].login'
done | sort -u | sed 's/^/author:/' | tr '\n' ' '
```

Eleven people across the two teams, so the lists are short — but they are
manual, and they are three separate copies that have to move together. Rule 4
must stay a superset of rules 2–3: it is the catch-all for the wider group, and
narrowing it empties the section rather than shrinking it.

## Stacks in team sections

`tree: true` groups a chain of stacked PRs and draws `╭╴ │ ╰╴` down the side. It
is on for rules 1–4 here. A chain is worked out from the PRs **in that section**,
which matters because first-match-wins routinely splits one:

```text
MY TEAM NEEDS REVIEW          MY TEAM
  ╭╴ #3228                      ╭╴ #3251
  │  #3234                      ╰╴ #3254
  ╰╴ #3239
```

That reads correctly — `#3251` and `#3254` genuinely are stacked on each other.
In every case the glyph sequence is well-formed and no child is orphaned from
its parent; the layout degrades to a shorter chain, never a broken one.

Two cases are worth knowing. Only the top of a stack being in the section means
it gets no glyph at all and renders as an ordinary row — quiet rather than
wrong. A gap in the middle, where the linking PR was claimed elsewhere, groups
the sub-chain below and drops the PR above it out as a plain row with no sign it
is stacked on something off-screen. The glyph means "stacked on something in
this section", not "stacked".

The `limit: 50` on the tree rules is deliberate: a limit that cuts a chain cuts
it at the root, leaving the children to regroup into a valid but shorter chain
with no sign the base is missing. Keep a tree rule's limit above its raw count
(the largest here is 41). `Open` is the exception — it truncates at 30, but with
no `tree` there is no chain for the cut to damage.
