---
name: critty-history
description: Group Critty reviews by task, apply retention at task boundaries, and save portable Markdown archives for retrospectives or later publication. Use at the start and end of work using Critty, including tasks with several reviews, or when managing saved records.
---

# Review history

Organize review records at work boundaries so they remain useful without accumulating unnoticed.  One task may span several reviews, conversations, and agent tools.  Individual approval completes a review, not the task.  Keep the task ID and any pending retention decision in the caller's existing handoff or progress record.

## Start or continue a task

Use `critty history list` from the task's working directory to find an existing task for this work.  Retain its ID across conversations.  Use `critty history reopen <task-id>` to continue a completed task whose original sessions were kept.  Reopen only when continuing that work; a separate task gets its own ID.

For new work, run `critty history start '<task name>'` once.  The project name defaults to the repository or directory name; pass `--project '<project name>'` to override it.  Record `task.id` from its JSON output.  Start applies the configured policy in this directory; it leaves ongoing work alone.  Under `auto`, recent backups remain quiet, old completed tasks can request a decision, and capacity pressure rotates completed backups automatically.  Report automatic deletions and their task identities after the operation.  Process any `maintenance` results with `decision_required: true` together, once at this boundary.  Honor a pending age-maintenance deferral in the host record; it suppresses repeated questions, not capacity rotation.  Present old-task candidates together before the first review, without waiting for a maintenance choice to begin independent work.  Report `skip` and `error` results, their reasons, and any `maintenance_error`; old-task failures do not prevent starting the new task.  Repair failed tasks separately.  Resuming a task or opening another review does not repeat start-time maintenance.

Pass `--task <task-id>` when opening each review through `/critty`.  This associates the saved review before the TUI opens and defers approval cleanup to the task, including after resume.  To include an existing stopped review, use `critty history add <task-id> --session <session-id>`.  Membership is explicit so concurrent agents do not share an implicit current task.  Each review belongs to at most one task.

## Finish the work

Finish after all operations authorized by the task's reviews are verified or explicitly abandoned.  Keep a task open through waits, interruptions, and unfinished publication; a commit or a closed terminal alone does not finish a larger task.  Then run `critty history finish <task-id>` once.  Default `auto` retains originals quietly and checks capacity.  Report any automatic rotation and failures; ordinary completion needs no archive question.

## Present one retention decision

Commands return JSON.  When `decision_required` is true, use the host's question or approval UI once for the current task or batch of old candidates.  Show names and IDs, session counts, age, known archive paths, and sizes when available.  Distinguish the storage reclaimed from an additional saved copy; label unavailable size estimates rather than inventing them.  Allow per-task exceptions to a batch choice.

| Choice | CLI action and meaning |
| --- | --- |
| Save and clean up | `archive`: save and verify the standard archive, show its path, then remove originals.  Suggest this when continued TUI editing is unnecessary. |
| Keep for continuation | `keep`: complete the work and retain resumable originals; reopen before further reviews.  The capacity policy still applies at future boundaries; reopening marks continued work as ongoing. |
| Delete | `delete`: remove selected originals without saving.  State whether an existing archive remains. |
| Decide later | Retain records and record a pending decision for 7 days later in the caller's progress record.  Do not silently select archive or delete. |

Apply an answer with `critty history finish <task-id> --action <action>`.  For old candidates, use the exact IDs shown by the original result, never a fresh destructive age sweep.  Recheck the selected tasks and their saved updates before acting; changed, reopened, or busy tasks need reconsideration.  For automatic actions, respect their configured scope.

`ask` returns a decision request; it does not open another review or read interactive stdin.  If the user defers, dismisses the question, or cannot answer, complete finished work with `--action keep` and record that this is deferred retention, not an explicit keep choice.  Preserve the candidate IDs, boundary, last presentation, next reminder, and any known archive paths.  Reuse that record across agent handoffs.  Offer the deferred IDs at the first work boundary after the reminder is due, rechecking eligibility first.  A pending question is not another reason to interrupt each review.  If the host has no durable progress record, retain the records and report that a persistent reminder could not be saved.

Respect global policy, including `auto` capacity rotation.  Individual approval does not authorize an extra cleanup action.  Running or busy reviews are protected; stop them through `/critty` before retrying.  An export failure leaves original sessions intact.  Interrupted removal records its remaining sessions; rerun `finish` with the recorded action and task ID.  On failure, a nonzero exit accompanies JSON with `error` and the partial `result`; report the failure and retained archive path rather than declaring cleanup complete.

A review removed outside history does not prevent `keep`, `ask`, or `delete`; their JSON reports `missing_sessions`.  A strict archive requires all members.  To remove a missing or unwanted member explicitly without deleting its data, use `critty history detach <task-id> --session <session-id>`, then retry.  Short concurrent history writes wait briefly; after a longer busy error, retry the same operation.  If review association fails after creating a session, the error identifies that saved session; add it with `history add` before resuming instead of creating a replacement.

Unrelated corrupt task records produce warnings; capacity rotation stops when membership coverage is incomplete.  Explicit operations on valid tasks remain available.  When membership cannot be determined, reviews retain data even if approval cleanup is enabled.  Repair the reported task record before relying on automatic organization; a review directly associated with a corrupt task stays blocked to protect its records.

## Policy

Read the [configuration reference](../critty-cli/references/configuration.md) when resolving or editing global settings.  Run boundary commands without an action override unless the user selected one.  Omitted or empty `start` and `end` mean `auto`; fixed `keep`, `archive`, or `delete` actions override it.  `ask` remains available for a deliberate decision request.  A one-time answer does not authorize changing global preferences.  Report configuration errors and retain data rather than guessing an action.

`keep_days` defaults to 30 and expresses the desired backup retention period.  Age alone makes completed tasks candidates for a maintenance conversation at task start; it does not delete them.  Recent saved review updates reset age.  `max_bytes` defaults to 1 GiB and budgets known original-review storage, excluding explicit archives.  When exceeded under `auto`, Critty deletes eligible completed tasks in this working directory, oldest saved update first, until usage reaches 80% of the limit.  Capacity can override `keep_days`.  Ongoing, busy, ungrouped, interrupted, and out-of-scope records remain protected.  Report an unreachable target without expanding the deletion scope or asking about each automatic rotation.

`critty history usage` returns measured bytes, roots, and warnings without changing records.  This estimate covers default review storage and registered external review directories for this working directory; it is not a full disk inventory or quota.  `history clean` applies start policy explicitly; `--days 0` includes all completed tasks.  With `auto`, clean returns `outcomes` and `storage`; fixed actions return an outcome array.  Start uses `maintenance`; finish can also return `maintenance` and `storage`.  A failed operation returns partial results under `result` with `error`.  Use the actual measured scope and report incomplete scans.

## Suggest valuable records selectively

A substantial design discussion, a useful human correction, an unexpected insight, or an AI cowork/review exchange that resolves a difficult tradeoff may deserve a durable archive.  At an appropriate work boundary, explain the concrete value and offer an export once.  Routine approval, conversation length, and task completion alone are not reasons to ask.  Respect a declined suggestion.  Use `export` to preserve originals unless cleanup was also selected, and check the known archive before creating another copy.

## Save without removing originals

Use `critty history export <task-id>` for an explicit copy or handoff, optionally with `--output <parent-directory>`.  Export does not reclaim space.  Do not export on every approval or conversation; keep a known archive location with the task record and inspect it before proposing another copy.  After a successful save, retain its path in the host record.  Explicit archives are protected from automatic rotation and have no default age reminder.  Export creates a unique folder named with the local date/time, project name, task name, and ID.  Its default parent is `$XDG_STATE_HOME/critty/archives` (usually `~/.local/state/critty/archives`).  The JSON result gives the actual `archive` path.  If external images or review sessions have already disappeared, `export --allow-incomplete` explicitly saves the remaining material, leaves missing references unchanged, and records warnings in `review.md` and `manifest.json`.  It always keeps originals and cannot bypass corruption or missing managed session images.  Automatic archival remains strict.

```text
2026-10-18 134000 - Critty - Review history - <id>-<unique suffix>/
  review.md
  attachments/
    <session-id>/
      <attachment-id>/
        document.md
        assets/
      source-<path-hash>/
        source.md
        assets/
  assets/
  sessions/
  manifest.json
```

`review.md` contains the project and task identities, sessions, verdicts, comments, resolved threads, replies, and links to saved documents and source snapshots.  `attachments/` contains readable document snapshots, each with its own `assets/` for images referenced within that document.  Top-level `assets/` contains images referenced by the main review record.  Image references are relative to their document.  Remote resources remain URLs and are not fetched.  Original session files stay byte-for-byte under `sessions/`, including forms, plan versions, and review JSON.  `manifest.json` records session metadata, file hashes, and image provenance.  External local image bytes are captured at export time and may differ from the reviewed revision; managed session images come from saved attachments.  Exported data describes saved state; it is not a complete edit log, every past round, or independent evidence of the original host completion.  Keep required approval evidence under the caller's policy.

Use the Markdown archive as input for a retrospective or a requested PR/git-notes summary.  Saving locally does not authorize publication.  Preserve the original JSON for future Crit interchange; a compatible JSON shape alone does not establish bidirectional session-resume support.

## Archive maintenance and current limits

Saving records also consumes storage outside the rolling backup budget.  When the user requests archive maintenance, offer keep, move, or delete with exact paths and available sizes, especially when the archive is the only remaining copy.  Verify a destination copy before removing a moved source and update its locator.  Age or capacity alone does not authorize archive deletion.  Task `delete` removes originals, not saved archives.

The CLI does not implement persistent snoozes, a complete archive inventory, archive move/delete commands, or background scheduling.  The latest archive locator and host progress records are not a complete inventory.  Report these limits when relevant and use exact paths under the caller's file-operation permissions for requested archive maintenance.

During explicit maintenance, surface old open tasks, ungrouped sessions, and interrupted cleanup separately.  Ask whether to continue, group, save, or delete them; age and process absence do not prove that work is complete.  Preserve busy sessions and uncertain staging/recovery data.  Handle corrupt membership as a repair case.

For retention rationale, default policy, the task data model, and archive integrity boundaries, see the [review history design](https://github.com/crittylabs/critty/blob/main/docs/review-history.md).
