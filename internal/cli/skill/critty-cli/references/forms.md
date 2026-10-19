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
| Choice with a note | `value=other text` on an option | the option's value, with the note in `text` | The note is exported only while that option is selected. |
| Ranking | `id=lead rank value=a` on each item | values from first to last, or `null` | At least two items with distinct values.  Cannot be `consent`, `text`, or tagged. |
| Select-all | `id=all select-all=api` | none | Selects or clears every item tagged `api`.  Cannot be `required`, `consent`, `text`, or a choice. |

`required`, `consent`, choices, rankings, and select-alls need an explicit `id`.  Use explicit IDs whenever answers should survive document revisions.

## Tags and select-all

`tags=api,ui` adds an item to named sets within its document.  A select-all item selects every item with any listed tag, or clears them all when all are selected.  It shows a mixed state when only some are selected.  Nested select-all items follow their own tags.

A select-all item has no answer of its own.  Each target's answer is a human answer, including `consent`; `via` names the select-all item when that operation set it.  Critty rejects a tag matching no other item, cycles, and select-alls covering two choices of one field.

## Text and images

Text is Markdown, entered and shown like a comment body.  Pasted images are saved in the session and referenced by `attachments/` paths relative to the result's `image_base`.  These files are removed with the session.  When carrying answers into a durable document, such as an ADR or design document, copy each referenced image into that document's own asset directory and rewrite the links relative to the document.  Do this before cleaning up the session.

## Rankings

Until the reviewer moves an item or confirms the order, the answer stays `null`.  Critty displays a contiguous ranking in its answered order without changing the source.  A comment on a ranked item keeps that item's source `start_line` and `anchor`, so it names the item regardless of its displayed position; read the position from the answer.  Inside a reordered ranking, a comment covers one item, and a range reaching into it covers the whole ranking.

## Saved answers

`forms.json` stores definitions, draft answers, and human provenance separately from source documents and comments.  Its version is 1, or 2 when any item uses `tags`, `select-all`, `text`, or `rank`; older Critty releases reject these attributes.  Changed field definitions reset their answers.  Changed target snapshots or attachment contents, order, or membership reset all required confirmations and consents; ordinary explicit-ID answers survive unchanged definitions.

An answer is human only when `human` is `true`; initial values carry no human provenance.
