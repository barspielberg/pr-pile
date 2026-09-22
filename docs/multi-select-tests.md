# Multi-select — test cases

Companion to [multi-select.md](multi-select.md). One section per build step, so
each step ships with its own tests green.

These are cases, not code. Names are the Go test names to use; they follow the
existing convention of naming the behaviour, not the function.

Helpers already available in `internal/ui`: `testCfg()`, `actionBoard`,
`press`, `pressAction`, `drain`, `flatten`, `stripANSI`, `m.firstRowSlot()`,
`m.rowSlot(n)`, `m.headerSlot(n)`.

**One new helper.** A board with several PRs across both sections, so a range can
cross a boundary:

```go
// selectBoard builds a two-section board: 3 PRs under Mine, 2 under Review
// requested, both resolved. The slot space is therefore
// header, row, row, row, header, row, row.
func selectBoard(t *testing.T) Model
```

---

## Step 1 — state, `space`, the mark, `esc`

### Toggling

- `TestSpaceSelectsThePRUnderTheCursor` — press `space` on a row, that PR number
  is in the set and nothing else is.
- `TestSpaceTwiceDeselects` — press it twice, set is empty.
- `TestSpaceOnAHeaderSelectsNothing` — cursor on `headerSlot(0)`, press `space`,
  set stays empty and no error status appears. It is a no-op, not a failure.
- `TestSpaceOnANoteSelectsNothing` — same for a section that resolved empty.
  Build it with `board.Result{Index: 1}` and no PRs.
- `TestSpaceDoesNotMoveTheCursor` — cursor index is unchanged after `space`. This
  is the k9s decision; without a test someone will "improve" it.
- `TestSelectionSpansSections` — `space` a row in each section, both numbers are
  in the set.

### Stored by number, not position

This is the constraint most likely to be broken by a later refactor, so it gets
tested directly rather than through the UI.

- `TestSelectionSurvivesRowsMovingOnRefresh` — select PR 42, then re-`Apply` the
  section with 42 in a different position and a new PR above it. 42 is still
  selected; the new PR is not.
- `TestSelectionDropsPRsThatLeftTheBoard` — select PR 42, re-`Apply` without 42.
  Reading the selection yields nothing, and it does not panic.

### The mark

- `TestSelectedRowDrawsItsMark` — `stripANSI` the view, the selected row has `•`
  in column 1.
- `TestUnselectedBoardLooksExactlyAsBefore` — render a board with nothing
  selected, compare to the same board before the feature existed. Byte-identical.
  This is the design guide's "does it make a healthy board louder" check.
- `TestCursorAndSelectionAreSeparateMarks` — a row that is both selected and under
  the cursor shows `▌` **and** `•`. Then move the cursor away: the `•` stays, the
  `▌` goes.
- `TestSelectionMarkSurvivesNoColor` — with colour stripped the `•` is still
  there. It is a glyph, not a hue.

### `esc`

The order matters and is easy to get wrong, so each rung is its own test.

- `TestEscClearsSelectionBeforeSearch` — with both a selection and a query live,
  `esc` clears **only** the selection; the query and its highlights remain.
- `TestEscThenClearsTheSearch` — a second `esc` clears the query.
- `TestEscThenQuits` — a third `esc` returns `tea.Quit`.
- `TestEscQuitsWhenNothingIsSelected` — no selection, no query: `esc` still quits
  on the first press. The new rung must not cost a keypress to users who never
  select anything.

---

## Step 2 — `v` ranges

### Extent

- `TestRangeSelectsFromAnchorToCursor` — `v` on the first row, `j` twice, three
  PRs selected.
- `TestRangeExtendsBackwards` — `v`, then `k`. The rows *above* the anchor are
  selected. A range that only works downward is the obvious bug here.
- `TestRangeShrinksWhenTheCursorComesBack` — `v`, `j`, `j`, then `k`. Two
  selected, not three. The range tracks the cursor, it does not accumulate.
- `TestRangeSkipsHeadersAndNotes` — a range from the first section into the
  second. Only PR rows are selected; the header between them contributes nothing
  and does not error.
- `TestRangeOverAHeaderOnlySelectsNothing` — `v` on a header, no movement. Empty
  selection, no crash.

### Leaving the mode

- `TestVAgainEndsRangeAndKeepsSelection` — `v`, `j`, `v`. Still selected, but `j`
  no longer extends it.
- `TestEscDuringRangeClearsIt` — `v`, `j`, `esc`. Nothing selected and ranging is
  off.
- `TestRangeAddsToAnExistingSelection` — `space` one row, then `v` a run
  elsewhere. Both the lone row and the run are selected. This is the documented
  route to scattered selection, so it needs to actually work.

### Footer

- `TestFooterShowsRangeKeysWhileRanging` — footer left side contains `esc clear`
  while ranging.
- `TestFooterShowsTheSelectionCount` — with 3 selected, the footer says `3`.
- `TestFooterReturnsToNormalWhenSelectionCleared`.

---

## Step 3 — `y` over a selection

**This is the feature the user asked for.** It works at the end of this step.

`copyToClipboard` is already a `var`, so a test swaps it and reads what would
have been copied.

- `TestCopyWithNothingSelectedCopiesTheCursorRow` — unchanged behaviour, still
  `copied #42 url`.
- `TestCopyCopiesEverySelectedURL` — 3 selected, all 3 urls in the payload.
- `TestCopyJoinsWithNewlines` — payload is exactly `url1\nurl2\nurl3`. Decided:
  one per line. No trailing newline.
- `TestCopyUsesBoardOrderNotSelectionOrder` — select bottom-up (`space` on row 3,
  then row 1). Payload is still row 1 then row 3. A Go map has no order, so
  without this test the output is random per run.
- `TestCopyReportsTheCount` — status says `copied 3 urls`. And `copied 1 url` for
  one — singular, not `1 urls`.
- `TestCopySkipsSelectedPRsWithNoURL` — a selected PR with an empty `URL` is left
  out and the count reflects what was actually copied.
- `TestCopyClearsTheSelection` — after a successful copy, the set is empty. This
  is the stale-selection rule.
- `TestCopyFailureKeepsTheSelection` — `copyToClipboard` returns an error, status
  reports it, and the selection is **kept** so the user can retry. Success clears,
  failure does not — same shape as the existing `clearSeq` rule for statuses.

---

## Step 4 — `enter`/`o` with the confirm

Decided: it asks at **3 or more**.

- `TestOpenOneSelectedDoesNotAsk` — opens immediately.
- `TestOpenTwoSelectedDoesNotAsk` — two is a normal thing to want, no prompt.
- `TestOpenThreeSelectedAsks` — the confirm appears and **nothing opens yet**.
  Assert on the browser call, not just the prompt: a confirm that opens anyway is
  the failure that matters.
- `TestOpenConfirmAcceptOpensAll` — confirm, all three open.
- `TestOpenConfirmRejectOpensNothing` — `esc` or `n` at the prompt, nothing
  opened, and the selection survives so the user can adjust it.
- `TestOpenWithNothingSelectedNeverAsks` — the plain single-PR path is untouched
  at any threshold.
- `TestOpenClearsTheSelectionAfterOpening`.

---

## Step 5 — `multi: true` actions

The largest step and the one with a security boundary.

### Refusing

- `TestSingleActionRefusesWithSeveralSelected` — action without `multi`, 2 PRs
  selected, pressing the key sets a status like `worktree: one PR at a time` and
  **starts no process**. Assert `running` is empty — the point is that it does not
  quietly run on the first PR.
- `TestSingleActionStillWorksWithOneSelected` — one selected behaves exactly like
  no selection.
- `TestSingleActionUnaffectedByAnEmptySelection` — every existing action test
  still passes untouched. Literally: do not modify `action_test.go`.

### Plural fields

- `TestMultiActionRunsOnceForTheWholeSelection` — 3 selected, one process, not
  three. Count invocations.
- `TestMultiActionExpandsPluralFields` — `{{.URLs}}` renders all three,
  space-joined.
- `TestMultiActionPluralFieldsAreQuotedPerElement` — run `printf '%s\n' {{.URLs}}`
  through a real `sh` and assert each url comes back on its own line, intact. The
  existing `TestActionTemplateStringsAreShellQuoted` is the model.
- `TestMultiActionWithNothingSelectedUsesTheCursorRow` — a list of one, so the
  template needs no special case.
- `TestSingularFieldInMultiTemplateIsAnError` — `{{.Number}}` in a `multi: true`
  action is rejected, and rejected at **config load** if possible rather than at
  keypress. A config error the user sees at startup beats one they see when they
  press the key.
- `TestPluralFieldInSingleTemplateIsAnError` — the reverse, same reasoning.

### Security — the part that must not regress

Mirror the existing suite exactly, extended to the plural fields.

- `TestEveryRemotePluralTemplateFieldCannotInjectShell` — the direct analogue of
  `TestEveryRemoteStringTemplateFieldCannotInjectShell`, over `URLs`, `Branches`,
  `Bases`, `Authors`, `Titles`. Set every field of every selected PR to
  `literal; touch $marker`, run it, assert the marker file does not exist.
- `TestInjectionFromASecondSelectedPR` — the first PR is benign and the **second**
  carries the payload. A validator that only checks one element would pass every
  other test in this file.
- `TestPluralFieldsRejectQuotedPlaceholders` — `"{{.URLs}}"` and `'{{.URLs}}'` are
  refused, same as the singular fields are today.
- `TestPluralFieldsRejectIndirectEvaluation` — `eval {{.URLs}}` and
  `sh -c {{.URLs}}` are refused. Reuse the singular cases in
  `action_security_test.go`.
- `TestPluralFieldsRejectNonTopLevelContext` — `$( {{.URLs}} )`, backticks, and
  inside a heredoc.
- `TestRemoteFieldListCoversEveryPluralField` — walk the plural template fields by
  reflection and assert each string-valued one appears in `remoteActionFields`.
  This is the guard against someone adding `{{.Labels}}` later and forgetting to
  validate it. The design guide's §12 point — an audit nothing re-checks goes
  stale — applies directly.
- `TestNumbersFieldNeedsNoQuoting` — `{{.Numbers}}` is integer-derived, so it is
  allowed unquoted like `{{.Number}}`.

### Concurrency

- `TestSecondMultiActionRefusedWhileOneRuns` — the existing `running != ""` guard
  still holds for a multi action.

---

## Cross-cutting

- `TestSelectionIsIgnoredInsideOverlays` — with PRs selected, open `?` and `d`.
  `space` and `v` do their overlay thing (or close it), and do not change the
  selection.
- `TestSelectionClearedOnRefresh` — press `r`, selection is empty. From "not
  doing" in the plan; a test pins it so it does not get added by accident.
- `TestSearchAndSelectionCoexist` — a query and a selection at once: matches stay
  highlighted, marks stay drawn, and neither clears the other.
- `TestSelectAllRowsThenCopy` — end-to-end on a full board: `v`, `G`, `y`, and
  every url on the board comes back newline-joined in board order.
