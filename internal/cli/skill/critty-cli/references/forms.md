# Attachment forms

Task-list items in attached `.md` or `.markdown` files are interactive.  Annotate an item with one `<!-- form: ... -->` comment of space-separated attributes; quote values containing spaces.  Items in quotations, code blocks, and escaped lists stay inert.  [`/critty-approval`](../../critty-approval/SKILL.md) defines when to use confirmations and consents and how to interpret permission.

## Widgets

| Widget | Annotation | Answer `value` | Constraints |
| --- | --- | --- | --- |
| Checkbox | optional `id=check` | `true` or `false` | A source `[x]` sets the initial value. |
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

Use `type=file`, `type=directory`, or `type=path` instead of `text` for a single-line path.  Long paths can wrap on screen.  Directory inputs suggest directories only; file and path inputs suggest files and directories so users can navigate into folders.  A new file or directory name can be entered without creating it.  File inputs reject paths ending with `/` and existing directories, including symbolic links to directories.  Directory inputs reject existing files.  Relative completion starts at the review's working directory, and `~/` lists the user's home directory.  Answers retain the literal entered path, including spaces and Unicode, without an `@` prefix or shell escaping.  Environment variables and commands are not expanded.

Missing paths show a notice without blocking Save: cyan on dark backgrounds when only the target is missing, yellow when its parent directory is also missing.  Light backgrounds use darker shades.  Add `existing` to require a nonempty path to exist; the same notices become red and Save is rejected.  Empty optional inputs can still be cleared; use `required` to require an answer.  The path is checked again when saving.

`default="..."` supplies initial input for a standalone field or an input attached to a choice or checkbox.  Quote paths containing spaces using shell-style quotes inside the annotation.  A standalone default has `human: false` until the reviewer saves it, even unchanged.  Selecting a control with associated input accepts its current input, including a default, and opens the editor.  Closing that editor leaves a valid selection and input in place.  If a proposed path is invalid, selecting the control opens its editor without committing the new selection; a valid Save confirms it.  Closing that invalid draft keeps the previous selection.  A select-all containing an invalid path opens that input before applying the group; fix it and select the group again.  For associated inputs, `required` and `consent` apply to selecting the control; use a separate required field when an input value must be provided independently.

```markdown
- [ ] Existing source <!-- form: id=source type=file existing default="README.md" -->
- [ ] Output file <!-- form: id=output type=file default="reports/summary.md" required -->
- [ ] Project notes <!-- form: id=where value=notes type=directory default="docs/" -->
- [ ] Personal notes <!-- form: id=where value=personal type=directory default="~/Documents/" -->
- [ ] Save a copy <!-- form: id=copy checkbox type=file default="my copy.md" -->
```

Choice inputs use the same `type` or `text` attributes as standalone fields.  `checkbox` distinguishes a boolean control with associated input from a standalone field.  Each choice keeps its own input draft when switching options.  Checking a control opens its input; unchecking does not.  Unchecked controls retain their input locally, but that input has no effect until the control is checked again.  Results include only the selected control's input, never the other drafts.  Changing a type, default, or `existing` restriction changes the definition and resets the previous answer.

Path input uses the existing completion menu without requiring `@`.  Select a candidate with the keyboard or mouse.  Paste and kill-ring editing remain available.  A final clipboard line ending is removed; multiline paths are rejected without changing the input.  Built-in Help lists the completion and save controls.

## Tags and select-all

`tags=api,ui` adds an item to named sets within its document.  A select-all item selects every item with any listed tag, or clears them all when all are selected.  It shows a mixed state when only some are selected.  Nested select-all items follow their own tags.

A select-all item has no answer of its own.  Each target's answer is a human answer, including `consent`; `via` names the select-all item when that operation set it.  Critty rejects a tag matching no other item, cycles, and select-alls covering two choices of one field.

## Text and images

Text is Markdown, entered and shown like a comment body.  Pasted images are saved in the session and referenced by `attachments/` paths relative to the result's `image_base`.  These files are removed with the session.  When carrying answers into a durable document, such as an ADR or design document, copy each referenced image into that document's own asset directory and rewrite the links relative to the document.  Do this before cleaning up the session.

## Rankings

Until the reviewer moves an item or confirms the order, the answer stays `null`.  Critty displays a contiguous ranking in its answered order without changing the source.  A comment on a ranked item keeps that item's source `start_line` and `anchor`, so it names the item regardless of its displayed position; read the position from the answer.  Inside a reordered ranking, a comment covers one item, and a range reaching into it covers the whole ranking.

## Saved answers

`forms.json` stores definitions, draft answers, and human provenance separately from source documents and comments.  Version 1 covers the basic controls; version 2 adds tags, select-all, text, and ranking.  Version 3 adds path types, optional existence restrictions, defaults, associated checkbox inputs, and per-choice input drafts.  Older readers reject unsupported versions or annotations.  Results keep standalone string input in `value` and associated input in `text`; the saved `drafts` map is omitted from results.  Changed field definitions reset their answers.  Changed target snapshots or attachment contents, order, or membership reset all required confirmations and consents; ordinary explicit-ID answers survive unchanged definitions.

An answer is human only when `human` is `true`; initial values carry no human provenance.
