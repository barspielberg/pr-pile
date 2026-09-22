# Multi-select — research and plan

Proposal. Nothing here is built yet.

Goal: copy several PR urls at once. That needs two things — a way to pick more
than one row, and a way to act on what you picked.

---

## 1. How other tools do it

### lazygit

Press `v`, move, and everything between where you started and where you are now
is selected. Four things they decided, all worth copying:

- **Only adjacent items.** They considered scattered selection and said no. It
  makes both the UI and the code harder, and for some commands it has no clear
  meaning.
- **`esc` clears the selection before it does anything else.**
- **Moving to another view drops the selection.** You can't act on something you
  picked a while ago and forgot about.
- **A command that can't take several items says so.** It doesn't quietly run on
  just the first one.

Source: [#3196](https://github.com/jesseduffield/lazygit/issues/3196)

### lazygit, custom commands

This one is still unsolved there, and it's the same problem we have.

Their custom commands got `{{.SelectedCommit.Sha}}` back when you could only
select one thing. Now that ranges exist, they need to add a plural version — and
they're stuck on the question we also have to answer: **what should an old
single-item command do when the user has several things selected?**

The lesson is about order. They shipped the singular field first, so now they
can't change what it means without breaking everyone's config. We're in the same
spot with `{{.Number}}`.

Source: [#4184](https://github.com/jesseduffield/lazygit/issues/4184)

### k9s

`space` marks the row you're on. `ctrl+space` marks everything between your last
mark and where you are now. There's no mode to enter or leave, and marks don't
have to be next to each other.

Two useful findings:

- Range-select works on some resource types but not others
  ([#2882](https://github.com/derailed/k9s/issues/2882)). They built it per-view
  instead of in one place, and the views drifted apart.
- Someone asked for `space` to also move down a line, to save keystrokes. Closed,
  won't do ([#1204](https://github.com/derailed/k9s/issues/1204)). It helps when
  you're going down and gets in the way when you're going up.

### yazi

Has both: `space` toggles one file, `v` starts a range. Also `ctrl+a` for all and
`ctrl+r` to invert. Your selection survives changing directory, which is the
whole point — you collect files from a few different folders, then act on them
together.

That's the best argument for having `space` and not just `v`: the things you want
usually aren't next to each other.

### Side by side

| | pick one | pick a range | can they be scattered? |
|---|---|---|---|
| lazygit | — | `v` | no, on purpose |
| k9s | `space` | `ctrl+space` | yes |
| yazi | `space` | `v` | yes |

Two of the three use `space` to toggle and `v` for a range. `v` never means
anything else anywhere. Neither key is taken on our board.

---

## 2. Three things that make our board different

**The cursor can sit on headers and notes, not just PR rows.** See
`internal/ui/model_slots.go`. So a range can cover a header. Those aren't PRs, so
the rule is: you drag a range over slots, but only the PR rows inside it get
selected.

**The board reloads on a timer, and sections arrive one at a time.** Rows move.
That's why `clampCursor` exists. So we can't remember "rows 4 through 7" — after
a reload those are different PRs. **Remember PR numbers instead.**

**This is a board you glance at, not one you live in.** The design guide is
strict about this: a healthy board is nearly colourless and every new thing
starts muted. A selection marker has to compete with the CI and review glyphs,
so it needs to be quiet.

---

## 3. The plan

### 3.1 What we store

```go
// in Model
selected map[int]bool  // PR numbers, not row positions
anchor   int           // where a range started, -1 if not ranging
ranging  bool
```

PR numbers because rows move when the board reloads. If a selected PR disappears
from the board, we drop it when we read the selection, so nothing needs checking
on every frame.

### 3.2 Keys

| key | what it does |
|---|---|
| `space` | select or deselect the PR you're on; does nothing on a header or note |
| `v` | start a range here; `v` again or `esc` stops it |
| `esc` | clear the selection |

`space` does **not** move you down a line. k9s rejected that, and their reason is
good: it helps going down and hurts going up.

A range only covers adjacent rows, like lazygit. You can still pick scattered
PRs — use `v` for a run, `esc`, then `space` for the odd ones.

`esc` now does three things, in this order: **clear the selection, then clear the
search, then quit.** Selection first, same as lazygit.

Leaving out for now:

- `shift+↓`/`shift+↑` — plenty of terminals can't tell these apart from plain
  arrows. `v` then `j` is the same two keys and always works.
- `ctrl+a` for all, `ctrl+r` to invert — no one has asked. Easy to add later,
  annoying to take away.
- `V` for a whole section — see 3.4.

### 3.3 Acting on what you picked

`y` and any configured action use the selection if there is one, and fall back to
the row under the cursor if there isn't. If you never press `space`, nothing
changes for you.

- **`y`** copies one url per line, in board order, and the status line says
  `copied 4 urls`.
- **`enter`/`o`** opens all of them. At 3 or more it asks first — opening a pile
  of tabs by mistyping a key isn't something `esc` can undo.
- **`d`** stays single-PR. There's nothing sensible to show for six.
- **`r`, `/`, `n`, `?`** unchanged.

**The selection clears once an action finishes.** lazygit keeps theirs; we
shouldn't. There, it's a working set you keep using through a long session. Here,
you run one action and you're usually done — and a selection still sitting there
after a reload is how you act on the wrong thing.

### 3.4 Can you select a header?

**No.** You asked whether it's just confusing. It's worse than confusing — it's
ambiguous, and you can't fix it by drawing it better.

A selected header could mean "this section as one thing" or "all the PRs in it".
Those stop being the same the moment the board reloads and a new PR shows up in
that section — is it selected or not? Whatever we pick, the screen can't show
which one we meant.

`v` on a header already handles it: start a range there, press `l` to jump to the
next header, and the whole section is covered. The marks land on the rows, so you
can see exactly what you've got.

If we want a real "select this section" key later, make it `V` and have it
immediately mark the rows. The header is a shortcut for marking rows, never a
thing you select on its own.

### 3.5 Custom actions — the hard part

This is where lazygit is still stuck, and their ordering lesson applies to us. We
already shipped `{{.Number}}`, `{{.Branch}}` and `{{.URL}}`. We can't change what
those mean.

**So: an action says whether it can take several PRs, and by default it can't.**

```yaml
actions:
  - key: w
    name: worktree
    run: wt switch -x nvim pr:{{.Number}}
    # no multi -> one PR only

  - key: b
    name: browse all
    run: open {{.URLs}}
    multi: true
```

Without `multi`, the existing fields mean exactly what they mean today, and the
key **refuses** when several PRs are selected: `worktree: one PR at a time`.
Refusing is better than running on whichever PR happened to be first — that's the
kind of bug you don't notice for weeks.

With `multi: true`, the action runs **once** and gets plural fields —
`{{.Numbers}}`, `{{.URLs}}`, `{{.Branches}}` — joined by spaces, each one quoted
the way `shellQuote` already quotes the singular fields. Using a singular field in
a `multi` template is an error. If you want to do something per PR, write a shell
loop.

If you run a `multi: true` action with nothing selected, it gets the row under the
cursor as a list of one. That way templates don't need a special case.

**Security.** `validateRemoteActionFields` currently insists that anything coming
from GitHub — `Branch`, `Base`, `URL`, `Author`, `Title` — appears as a bare
placeholder at the top level of the command. The plural versions carry the exact
same untrusted data, so they need the same check. Add `URLs`, `Branches`, `Bases`,
`Authors` and `Titles` to `remoteActionFields`. `Numbers` is fine unchecked, like
`Number`, because it's integers.

This is the one part of the feature that touches security, so it's the part to get
right. `internal/ui/action_security_test.go` is where it gets proven.

The existing `running != ""` guard still holds, since a multi action is still one
process.

### 3.6 What it looks like

A selected row needs a mark that isn't the cursor's mark. The cursor is `▌` in
`accentStyle` at column 0, with the row filled in `selBg` (`render_row.go`).

**Use column 1 — it's a blank spacer today — and put `•` there in `accentStyle`.**
That keeps cursor and selection as two separate signals, so a row can be either,
both, or neither, and you can tell which.

Against the design guide's checklist: still one line per row, since we're reusing
a column that already exists and nothing shifts. Readable with colour off, because
it's a glyph. No new fixed colour. Nothing left of the title moves. And a board
with nothing selected looks exactly like it does now.

While a range is active, the footer should say so. It already swaps its left side
for searching, so this is a third variant:
`space mark · v range · y copy 3 · esc clear`.

### 3.7 Not doing

- Keeping the selection through `r`. Cheap to do with PR numbers, but a board that
  reloads into a selection you made two minutes ago is the stale-selection problem
  again.
- Selecting inside the `?` or `d` overlays.
- Remembering a selection between runs.

---

## 4. Order to build it

1. Add `selected` and `anchor` to the model, `space` to toggle, the mark in column
   1, and the new `esc` rung. Nothing acts on the selection yet.
2. `v` for ranges, the footer variant, and tests for ranges that cover headers and
   notes.
3. `y` over a selection, with the count in the status line. **This is the actual
   goal, and it works at the end of this step.**
4. `enter`/`o` with the confirm at 3 or more.
5. `multi: true`, the plural fields, `remoteActionFields` extended, security tests.
   Biggest piece, and last, because once people have configs we can't change it.

Steps 1–3 stand on their own. If we never build the rest, copying several urls
still works.

---

## 5. Settled

These were open; they're decided now.

- **`enter`/`o` asks at 3 or more.** Two tabs is a normal thing to want. Three is
  where a mistyped key starts to cost you.
- **`y` joins urls with newlines.** One url per line pastes into a PR
  description, a Slack message or a ticket, which is what this feature is for. If
  you want them on one line, the shell can join them.
- **`multi: true` stays on the action, and stays required.** Inferring it from a
  plural field in the template would mean less config, but then typing
  `{{.URLs}}` when you meant `{{.URL}}` silently changes what the action does.
  Being explicit is worth the extra line.
