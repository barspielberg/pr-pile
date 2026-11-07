---
title: Research archive
description: The original design studies that shaped the board, preserved as history with their conclusions and reversals intact.
sidebar:
  label: Overview
---

:::note[Archived research]
These five studies are the working notes that produced the board's design. They
are published as **history, not as a roadmap**. The conclusions that survived
live in the [visual spec](../visual-spec/) and the
[decisions](../decisions/) table; where a study's conclusion was later
overturned, the study says so in place rather than being quietly edited.
:::

Each study was written on 2026-09-17 for a single reader — the author of the
tool — against a specific commit and a live board captured from a private
monorepo (referred to here as `acme/monorepo`, a pseudonym). They are candid in
a way a spec is not: several record the author being wrong, measuring it, and
reversing. That candour is the value, so the text is edited only to point at
where the surviving content now lives, never to sound authoritative.

Read them in the order they were written, or jump to the one that interests
you:

| study | what it settled | status |
| --- | --- | --- |
| [View restructure](view-restructure/) | whether to restructure the list into uniform rows | superseded — its §1 recommendation was implemented and the jump survived |
| [Uniform rows](uniform-rows/) | the uniform-row restructure and the one-keypress-one-line invariant | shipped; §4.1 records the later passes that refined it |
| [Section layout](section-layout/) | how section headers work, and why they are inline not pinned | design only, no code changed; the inline header shipped |
| [Checks page](checks-page/) | what the detail overlay names versus counts | shipped; the overlay names failures and pending, counts passes |
| [PR detail](pr-detail/) | the state block under the checks, and what arrives on a second request | shipped; extends the checks page |

The dates and commit hashes in each study are part of the record and have not
been updated — they name the code the study was written against, not the code
as it is now.
