---
name: critty-history
description: Group Critty reviews by task, handle history warnings through Critty commands, and export requested or valuable discussions. Use for task bookkeeping, command-reported history warnings or maintenance results, and explicit history or archive requests.
---

# Review history

One work task can span several reviews and conversations.  Retain its task ID across rounds and handoffs.  Individual approval completes a review, not the task.

## Maintenance trigger and command boundary

Inspect capacity or old records only after a Critty warning, a returned maintenance decision/error, or an explicit user maintenance request.  Ordinary review startup/resume performs passive checks.  Routine task bookkeeping lets Critty apply its policy; it is not a reason for a separate storage scan, `history usage`, `doctor`, or old-record discussion.

Use Critty commands and their JSON results for inspection, saving, cleanup, and recovery.  Keep task/session IDs and returned archive paths as locators.  Do not traverse storage trees, inspect internal task/session/manifest files for maintenance, calculate usage yourself, or write scripts to reconstruct, archive, move, repair, or delete Critty records.  If the CLI cannot perform an operation or provide required information, retain the records and report that limitation.  Reading exported Markdown for a requested retrospective is separate from maintaining stored records.

## Start or continue a task

Reuse a supplied or recorded task ID.  Only if continuing work without its ID, use `critty history list` to identify that task; unrelated entries are not maintenance candidates merely because they appear.  Retained completed reviews can be opened for inspection with `critty sessions resume <session-id>` without reopening the task.  Use `critty history reopen <task-id>` when continuing the work or adding reviews.

For new work, run `critty history start '<task name>'` once and record `task.id`.  The project name defaults to the repository or directory name; `--project '<name>'` overrides it.  Critty applies the configured start policy itself.  Report returned automatic deletions, `skip`/`error` outcomes, and `maintenance_error`.  Also report `limited: true` and its reason; task completion can succeed even when protected records prevent reaching the capacity target.  If the result actually contains `decision_required: true`, follow the decision procedure below.  Critty honors saved deferrals across hosts.  Otherwise continue the work quietly.  Do not gather additional old candidates or repeat maintenance when resuming a conversation or opening another review.

Pass `--task <task-id>` to each new review through `/critty`.  Use `critty history add <task-id> --session <session-id>` to include an existing stopped review.  Membership is explicit so concurrent agents do not share an implicit current task; a review belongs to at most one task.

## Finish the work

Keep the task open through interruptions, waits, commits, and unfinished publication.  After every authorized operation is verified or explicitly abandoned, run `critty history finish <task-id>` once.  Critty applies the end policy itself; default `auto` keeps originals quietly and checks capacity.  Report returned rotation, `limited`/`reason`, or `maintenance_error`.  A successful completion remains complete when capacity maintenance is limited; do not retry completion as if it failed.  Completion alone does not trigger another inspection, archive offer, or cleanup question.

## Handle review-flow warnings

When review startup/resume prints `critty: warning: history:`, preserve the output and collect the current round's result first.  Then handle its category through the relevant commands:

| Warning | Response |
| --- | --- |
| `capacity` | Use `critty history usage` for the reported scope, then apply `critty history clean` under the configured policy when permitted.  Default `auto` rotates eligible completed backups without a confirmation; report actual deletions and any unreachable target.  Fixed `keep` remains keep.  Ask about specific protected records only if a further decision is needed. |
| `age` | Use `critty history list` and `critty status` to identify the reported records.  Honor fixed policy and saved deferrals.  Present one decision for completed tasks that need it.  Ask whether to continue, group, save, or explicitly complete/remove open or ungrouped work; age does not establish completion. |
| `recovery` | Use `critty history list` to identify the exact task and recorded action.  Retry with `critty history finish <task-id> --action <recorded-action>`.  Let the CLI verify the archive and remaining originals; report failures. |
| `incomplete` or `check unavailable` | Use `critty doctor --json` and the relevant history/status command to obtain diagnostics.  Use `history detach <task-id> --session <id>` for a confirmed missing member reported by the CLI; otherwise retain uncertain records and report the blocker.  Do not repair metadata by hand. |

`critty doctor` provides a fresh diagnosis with suggested commands when handling a warning or explicit diagnosis request.  It bypasses passive warning suppression.  Batch related warnings into one discussion for the work task; a warning does not authorize stopping another agent's review, deleting archives, changing preferences, or publishing records.  Keep the current task open throughout maintenance.  Critty throttles its own checks and repeated warning categories; agents do not schedule or poll for reminders.

## Present one reported decision

When a command returns `decision_required: true`, show its task names/IDs, session counts, and any reported ages, sizes, or archive paths together.  Use only information returned by commands; label unavailable estimates instead of calculating them or exploring the storage tree.  Allow per-task exceptions.

| Choice | Command and meaning |
| --- | --- |
| Save and clean up | `critty history finish <task-id> --action archive`: save, verify, report the archive path, and remove originals. |
| Keep | `critty history finish <task-id> --action keep`, then `critty history defer <task-id> --days 0`: retain originals and suppress age questions until saved content changes.  Future capacity rotation still applies. |
| Delete | `critty history finish <task-id> --action delete`: remove selected originals without a new archive; existing archives remain. |
| Decide later | `critty history defer <task-id>`: persist a seven-day age deferral; `--days <n>` honors a selected duration.  Capacity rotation still applies.  Revisit only after a command notification or user request. |

For old candidates, act only on returned IDs and refresh their state with `history list` if needed before acting.  The CLI owns membership, busy checks, verification, and removal.  A dismissal supplies no destructive authorization.  Elapsed time or a work boundary alone does not trigger a deferred question or recheck.  Keep the task IDs across handoffs; the CLI persists age deferrals.  A one-time answer does not change global settings.

## Save valuable discussions or requested records

A concrete insight, useful human correction, or substantial AI cowork/review discussion resolving a difficult tradeoff may merit one archive suggestion at a suitable work boundary.  Explain the value and respect a declined suggestion.  Routine approval, length, and completion alone are not reasons to ask.  This is a suggestion about known content, not a reason to inspect old records or capacity.

Use `critty history export <task-id>` for a selected save or handoff, optionally with `--output <parent-directory>`.  It keeps originals and returns the actual `archive` path.  Use recorded archive paths, or `critty history archives list` when an archive lookup is needed, before suggesting another copy.  Critty generates the dated project/task folder, readable `review.md`, attachments, assets, original session files, and manifest; agents do not assemble or verify this layout themselves.

For reported missing sessions or external images, `export --allow-incomplete` explicitly saves the remaining material and reports warnings while keeping originals.  It cannot bypass corruption or missing managed images.  The `archive` action stays strict.  Use `critty history detach <task-id> --session <session-id>` only for a selected missing/unwanted member, then retry the original command.  Exported state is not every historical round or independent evidence of the original host's approval; retain required completion evidence separately.  Local saving does not authorize PR, git-notes, or other publication, and session import is not implemented.

## Policy and limits

Read the [configuration reference](../critty-cli/references/configuration.md) when resolving or editing settings for an actual decision.  Run task boundaries without action overrides unless the user selected one.  Omitted or empty `start`/`end` mean `auto`; fixed global actions take precedence.  Invalid configuration is an error to report, not a reason to guess a destructive action.

Critty applies `keep_days` and a per-working-directory allocated-byte `max_bytes` budget.  The finish-time auto pass protects the task being finished.  Age alone does not delete records; capacity rotation can remove newer completed backups.  Ongoing, busy, ungrouped, inconsistent, and out-of-scope records are protected, as are explicit archives.  An unreachable budget does not authorize broader cleanup.

On failure, nonzero exit accompanies JSON with `error` and partial `result`.  Report partial removal and any archive path.  Interrupted pruning requires the recorded task/action.  If association failed after creating a session, use the returned session ID with `history add` before resuming.  Selected ungrouped reviews can be removed with `critty sessions remove <id>` after an explicit deletion decision and required evidence preservation; never substitute `clear --all` for selected maintenance.

For explicitly requested archive maintenance, use `critty history archives list` to get names, paths, and allocated sizes, `archives verify <path>` to verify a selected copy, and `archives delete <path>` only after an explicit deletion decision.  Listing includes default/configured destinations and indexed custom exports; `archives list --directory <parent>` discovers copied archives at a supplied parent.  Deletion verifies identity and checksums, rejects unrelated additions, and keeps live reviews.  Archives never rotate automatically.  Archive moving and background scheduling are not implemented; report unsupported requests without filesystem workarounds.  For the maintained format and retention contract, see the [review history design](https://github.com/crittylabs/critty/blob/main/docs/review-history.md).
