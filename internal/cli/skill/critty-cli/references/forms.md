# Attachment forms

Add interactive widgets to attached `.md` or `.markdown` files by annotating task-list items with `<!-- form: ... -->`.  Use one annotation per item with space-separated attributes; quote values containing spaces.  Unannotated task lists remain display-only, including checklists in publication drafts.  Items in quotations, code blocks, and escaped lists stay inert.  [`/critty-approval`](../../critty-approval/SKILL.md) defines when to use confirmations and consents and how to interpret permission.

## Widgets

| Widget | Annotation | Answer `value` | Constraints |
| --- | --- | --- | --- |
| Checkbox | `id=check` (or empty `<!-- form: -->`) | `true` or `false` | A source `[x]` sets the initial value. |
| Confirmation | `id=check required` | `true` once checked | Approval requires a human check; a source `[x]` is ignored. |
| Choice | `id=size value=small` on each option | the selected value, or `""` | Options share the ID with distinct values; `required` goes on every option. |
| Consent | `id=publish consent` | `true` or `false` | Grants permission only through `consents` on approval. |
| Text field | `id=reason text` | the entered string, or `""` | `required` demands text.  Cannot be `consent` or tagged. |
| Path field | `id=output type=file` (also `directory`, `path`) | the entered path, or `""` | Single-line input with path completion.  Can have `default="..."`. |
| Choice with a note | `value=other text` on an option | the option's value, with the note in `text` | The note is exported only while that option is selected. |
| Checkbox with input | `id=copy checkbox type=file` (or `text`) | `true` or `false`, with input in `text` | Input is exported only while checked.  Can be tagged or a consent. |
| Ranking | `id=lead rank value=a` on each item | values from first to last, or `null` | At least two items with distinct values.  Cannot be `consent`, `text`, or tagged. |
| Select-all | `id=all select-all=api` | none | Selects or clears every item tagged `api`.  Cannot be `required`, `consent`, `text`, or a choice. |

`required`, `consent`, choices, rankings, and select-alls need an explicit `id`.  Use explicit IDs whenever answers should survive document revisions.

## Path inputs and defaults

Use `type=file`, `type=directory`, or `type=path` instead of `text` for a single-line path.  A new file or directory name can be entered without creating it.  File inputs reject paths ending with `/` and existing directories, including symbolic links to directories.  Directory inputs reject existing files.  Relative completion starts at the review's working directory, and `~/` lists the user's home directory.  Answers retain the literal entered path, including spaces and Unicode, without an `@` prefix or shell escaping.  Environment variables and commands are not expanded.

Add `existing` to require a nonempty path to exist when saved.  Use `required` to require an answer.

`default="..."` supplies initial input for a standalone field or an input attached to a choice or checkbox.  Quote paths containing spaces using shell-style quotes inside the annotation.  A standalone default has `human: false` until the reviewer saves it, even unchanged.  Selecting a control with associated input accepts its current input, including a default, and opens the editor.  Closing that editor leaves a valid selection and input in place.  If a proposed path is invalid, selecting the control opens its editor without committing the new selection; a valid Save confirms it.  Closing that invalid draft keeps the previous selection.  A select-all containing an invalid path opens that input before applying the group; fix it and select the group again.  For associated inputs, `required` and `consent` apply to selecting the control; use a separate required field when an input value must be provided independently.

```markdown
- [ ] Existing source <!-- form: id=source type=file existing default="README.md" -->
- [ ] Output file <!-- form: id=output type=file default="reports/summary.md" required -->
- [ ] Project notes <!-- form: id=where value=notes type=directory default="docs/" -->
- [ ] Personal notes <!-- form: id=where value=personal type=directory default="~/Documents/" -->
- [ ] Save a copy <!-- form: id=copy checkbox type=file default="my copy.md" -->
```

Choice inputs use the same `type` or `text` attributes as standalone fields.  `checkbox` distinguishes a boolean control with associated input from a standalone field.  Unchecked controls and unselected choices retain their input drafts locally.  Results include only the selected control's input.  Changing a type, default, or `existing` restriction changes the definition and resets the previous answer.

## Tags and select-all

`tags=api,ui` adds an item to named sets within its document.  A select-all item selects every item with any listed tag, or clears them all when all are selected.  Nested select-all items follow their own tags.

A select-all item has no answer of its own.  Each target's answer is a human answer, including `consent`; `via` names the select-all item when that operation set it.  Critty rejects a tag matching no other item, cycles, and select-alls covering two choices of one field.

A reviewer can also toggle the checkboxes and choices whose controls start within a selected line range.  Each choice group contributes its topmost option in the range.  The operation clears the targets when they are all checked, including the topmost option of each group.  Standalone inputs, rankings, and tagged select-all controls are outside this operation.  Answers record human provenance without a `via` value.  Selected controls retain their associated inputs and the same path validation as tagged select-all.

## Text and images

Text answers use Markdown.  Pasted images are saved in the session and referenced by `attachments/` paths relative to the result's `image_base`.  These files are removed with the session.  When carrying answers into a durable document, such as an ADR or design document, copy each referenced image into that document's own asset directory and rewrite the links relative to the document.  Do this before cleaning up the session.

## Rankings

Use the same `id` and a distinct `value` on each ranked item.  Indent a ranked item under another item of that field to give it a parent.

```markdown
- [ ] Prepare <!-- form: id=steps rank value=prepare -->
  - [ ] Gather inputs <!-- form: id=steps rank value=inputs -->
  - [ ] Validate inputs <!-- form: id=steps rank value=validate -->
- [ ] Publish <!-- form: id=steps rank value=publish -->
```

Ranked branches move with their descendants.  Keep a ranking contiguous to let reviewers change a branch's depth and parent.

Add `fixed` to keep an item under its original parent and in order relative to other fixed siblings.  It can still receive children and travels with its branch when an ancestor moves.

Add `max-depth=N` to limit how many levels can exist below that item.  Use `0` for no children, `1` for children only, and `2` to allow grandchildren.  The limit applies to the source and every move, including the moving item's descendants.  Without it, the item has no depth limit of its own.

Add `min-level=N` and `max-level=N` to set the shallowest and deepest levels where an item can appear.  Root items are level `1`.  Use `min-level=2 max-level=2` to let items move between root parents while staying at level 2.  Moving an ancestor must keep every descendant within its level limits.

Until the reviewer moves an item or confirms the order, the answer stays `null`.  An answered ranking has a `value` array in depth-first order.  When it has children, the answer also has a `rank_parents` object that maps each child value to its parent value.  Root items are absent from the map.  The corresponding definitions use `rank_parent` for each item's original parent.

The attachment snapshot and Markdown source view retain the original order.  A comment on a ranked item keeps its source `start_line` and `anchor`.  Use the answer to determine its current position and parent.  Inside a reordered ranking, a comment covers one item, and a range reaching into it covers the whole ranking.

## Saved answers

`forms.json` stores definitions, draft answers, and human provenance separately from source documents and comments.  Version 1 covers the basic controls; version 2 adds tags, select-all, text, and ranking.  Version 3 adds path types, optional existence restrictions, defaults, associated checkbox inputs, and per-choice input drafts.  Version 4 adds ranked hierarchies in source definitions or answers, and `rank_fixed`, `rank_max_depth`, `rank_min_level`, and `rank_max_level` in definitions.  Older readers reject unsupported versions or annotations.  Results keep standalone string input in `value` and associated input in `text`; the saved `drafts` map is omitted from results.

Changed field definitions, including an item's visible wording, reset their answers.  Put clarifications in surrounding prose or comment replies to preserve the field.  Changed target snapshots or attachment contents, order, or membership reset every `required` or `consent` field, including required choices and text fields.  Ordinary explicit-ID answers survive unchanged definitions in the same attachment, whose identity comes from its absolute source path; renaming or moving that file loses its answers.  A previous select-all does not select newly added candidates; leave their source checkboxes unchecked.

### Before revising a form

1. Preserve the completed round's result, summary, complete output, and reviewed snapshots before editing inputs or resuming.  Use distinct paths such as `answers-round-1.json` and `answers-round-2.json`; keep both while decisions or execution depend on them.
2. Compare the proposed target, ordered attachments, and field definitions with that round.  For Git scopes, include the resolved base commit: a new commit between rounds can reset every `required` and `consent` field even when the diff is unchanged.  Keep the same attachment source path and semantic field IDs for unchanged questions, such as `example_project_42_approve`, rather than IDs derived from row positions or the current round.  Keep option values, visible wording, and source checkbox states unchanged too; the source checkmark is part of the definition fingerprint.  Stable IDs preserve eligible ordinary answers, not consent across a changed review revision.
3. Explain the expected reset before reopening: even an unrelated progress edit in an attachment resets all `required` and `consent` fields.  Keep routine progress in a separate record or comment instead.  If reviewed content must change, identify the permissions to confirm again.  Retain previous selections in a separate record or comment with their original round, rather than copying them into source checkmarks.  Leave current consent controls unchecked.
4. After submission, compare the actual new result with the preserved round using each answer's `human`, `author`, `review_revision`, and `round`.  Carried answers keep their original provenance; reset fields return to defaults with `human: false`.  Report retained, reset, and unanswered items from that result.  Overall approval can coexist with false optional consents; execute only operations covered by matching authorization.

An answer is human only when `human` is `true`; initial values carry no human provenance.

Pending-item warnings and confirmation for unviewed unanswered fields apply only to approval.  Comment and reject submissions return feedback and any answers immediately, even with required fields unanswered or threads unresolved.  These submissions grant no consent.
