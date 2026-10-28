# Review output and results

## Forms and exported results

Task-list items annotated with `<!-- form: ... -->` in attached `.md` or `.markdown` files are interactive forms: checkboxes, required confirmations, choices, consents, text answers, rankings, and select-all controls.  Unannotated items remain display-only.  Read [attachment forms](forms.md) before writing a form or interpreting its answers; it lists each widget's annotation, answer value, and constraints.  `critty-approval` defines when to use confirmations and consents and how to interpret permission.

`--answers` labels Comment as **Confirm answers** and selects it by default, for rounds that collect form answers without approving the design; the verdict remains `comment`.  The setting persists across the session's rounds until `--no-answers`.

`--result FILE` requires an interactive attachment form.  It writes the result atomically before printing approval; omit it for ordinary document approval.  The parent directory must exist, the file must be outside the session directory, and it must not overwrite reviewed input.  The same self-contained result is saved as `result-<round>.json`, returned in IPC's `result`, and appended to finish output regardless of templates.

| Result field | Meaning |
| --- | --- |
| `version`, `session`, `round` | Schema version, session ID, and completed round. |
| `image_base` | Session directory that resolves `attachments/` image paths in answers and comments. |
| `review_revision`, `reviewed_target` | Revision identifier and the exact reviewed input. |
| `attachments` | Ordered documents with source paths, snapshots, field `definitions`, and `answers`. |
| `approved` | Human review outcome, including required-confirmation checks. |
| `consents` | On approval, document ID → field ID → boolean for individual permissions. |

Answers submitted with `verdict: comment`, including `--answers` rounds, can record preferences or requirements without granting approval or execution permission.  Overall approval and initial checkmarks supply no individual consent.  Form review cleanup is deferred.  Preserve the result before running the printed cleanup command, and follow `critty-approval` to match decisions to operations.

## Review command output

On submission, stdout contains the finish prompt, threads, and attached document snapshots; stderr contains `verdict: approve|reject|comment`, `approved: true|false`, and diagnostics.  Capture both streams and the exit status.  Only `approve` sets `approved: true`; `reject` requests changes and `comment` sends feedback.  Exit 0 can mean any of these submissions, so inspect the verdict and all threads.  A nonzero exit does not grant approval.  Export or output failures retain session data; a pre-existing result file may be from an earlier round.  Correlate any recovered result with its invocation before using it.

`--summary FILE` on `critty`, `review`, `resume`, or `plan` saves a compact JSON index before printing approval.  It works with or without forms and custom finish prompts.  Version 1 contains `session`, `round`, `cwd` (creation directory), optional `workdir` (active workspace when different), `verdict`, `approved`, optional `review_comment`, all `comments` with resolved threads and replies, ordered `attachments` with `path`, `source`, `revision` (content hash), and optional `removed`, `image_base` for comment images, and optional `next_command`.  Empty comment and attachment lists are arrays.  Attachment bodies remain in complete stdout rather than the summary.

`launch` describes this invocation's startup: `launcher` is the concrete launcher name (or `foreground` for direct current-terminal operation), `failures` is an ordered array of unavailable candidates with `launcher` and `reason`, and optional `terminal_error` contains a failed marker lookup.  Repeated candidates appear once; no failed checks produces `failures: []`.  Each invocation records its own selection, including resumed reviews.  Older summaries omit `launch`.  The summary is written at review completion, so startup failures still report through the command's error output.

Use a fresh path for each invocation and verify exit 0, session, round, and the reviewed inputs before acting.  The parent directory must exist; the file must be outside the session directory and must not overwrite reviewed input or the `--result` file.  Summary export failure returns nonzero without printing approval or cleaning up the review.  Keep full finish output for additional instructions and evidence; a summary does not contain form answers or individual consents.  Use `--result` and `critty-approval` for those decisions.

Saved reviews are retained after approval by default.  The Crit-compatible `cleanup_on_approve` setting defaults to `false` in Critty; explicitly setting it to `true` enables approval cleanup.  Task-associated reviews defer cleanup to their task even when this setting is enabled.  Ungrouped ordinary reviews with cleanup enabled are deleted after successful finish output, while form results and comment images defer cleanup as described in their finish output.  Preserve the finish output even when retaining the session; saved state does not replace the original completion evidence.  Use `critty` for launch, wait, and recovery decisions.
