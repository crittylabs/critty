# Review history and archive design

This document describes the task model and version 1 archive layout implemented by `critty history`.  The retention lifecycle section defines the default policy and its current limits.  The [history skill](../internal/cli/skill/critty-history/SKILL.md) owns the agent workflow and command examples.  The implementation lives in [internal/history](../internal/history).

## Purpose and retention unit

Review records support retrospectives, later PR or git-notes summaries, and continuation of unfinished work.  One task can contain several review sessions and span conversations or agent tools.  Approval finishes a review; it does not finish the task or authorize deleting its records.

A task has an explicit ID, project name, task name, working directory, and ordered session IDs.  Each session belongs to at most one task.  There is no implicit current task shared by concurrent agents.  The project name defaults to the repository or directory name and can be overridden when starting the task.

Retention runs at explicit task boundaries.  Start-time maintenance considers old completed tasks in the same working directory.  Finishing applies to the named task.  Automatic policy is global, so a repository cannot enable deletion of the user's saved reviews.

| Operation | Result |
| --- | --- |
| `auto` | Retain recent originals quietly; offer age-based maintenance and rotate completed backups under capacity pressure.  This is the default. |
| `keep` | Retain originals regardless of age or capacity at this boundary. |
| `ask` | Return a decision request without removing data.  The host asks once for the task or boundary's candidates. |
| `export` | Create an archive and retain the originals. |
| `archive` | Create and verify an archive, then remove the original sessions. |
| `delete` | Remove the original sessions without an archive. |

Keeping originals does not automatically create dated archive folders.  A retained task can be reopened; an archive is a saved artifact, not an implemented session-import mechanism.

## Retention lifecycle

Retention must make accumulated records visible and give users a regular opportunity to organize them.  Merely retaining everything, or moving everything into an ever-growing archive directory, does not meet that goal.  Confirmation belongs at work boundaries, not at each review approval or each agent message.

### Default policy

Omitted or empty `history.start` and `history.end` mean `auto`.  Normal review approval and task completion retain resumable originals without asking whether to save them.  These originals are rolling backups.  An explicit archive is a durable artifact protected from automatic deletion.

Global policy uses `keep_days` (default 30) as the desired retention period and `max_bytes` (default 1 GiB) as the original-review storage budget.  These are separate controls: age makes completed records candidates for a maintenance conversation, while capacity permits automatic rotation even within the desired retention period.  `keep_days` is not a minimum preservation guarantee or an automatic expiry date.

| Boundary under `auto` | Behavior |
| --- | --- |
| Start a new task | Check capacity.  If below the limit, offer completed tasks idle beyond `keep_days` for one maintenance decision.  Recent records remain quiet. |
| Resume, hand off, or open another review | Reuse the task ID and pending decision.  Do not repeat boundary maintenance. |
| Approve an individual review | Keep its records with the task. |
| Finish the whole task | Mark it complete, retain originals quietly, and check capacity.  Wait until all authorized operations are verified or explicitly abandoned. |
| Interrupt, crash, or close the host | Preserve the open task.  Terminal closure does not establish completion. |
| Explicit maintenance | `history clean` applies the start policy; `history usage` estimates backup bytes without changing records. |

When a capacity check exceeds `max_bytes`, remove eligible completed tasks in this working directory in order of their most recent saved review update, oldest first, until measured usage is at most 80% of the limit.  Use the task ID to break timestamp ties.  The headroom avoids frequent repeated cleanup.  Do not ask for each rotation; report task identities and outcomes afterward.  A capacity pass replaces the age-based question at that boundary.

Ongoing tasks, active or locked sessions, ungrouped sessions, interrupted cleanup, and inconsistent membership are protected.  Recheck saved updates while holding session locks before removing a selected task; changed records leave the pass.  Never remove archives automatically or broaden deletion to another working directory to reach the budget.  If protected, inaccessible, or out-of-scope records prevent reaching the target, retain them and report the limit and partial results.  This budget is not a hard disk quota or a background scheduler.

Storage accounting sums regular-file logical sizes under default review directories and registered review directories belonging to this working directory.  It deduplicates overlapping paths and excludes symlink contents, archives, and small task receipts.  The JSON reports measured roots and incomplete scans.  External review directories belonging to other projects and unrecorded copies are outside the estimate.  Explicit archives consume additional disk space and require separately requested maintenance.

Users who want fixed behavior can configure `start` and `end` as `keep`, `archive`, or `delete`.  Start-time fixed actions apply only to completed tasks beyond `keep_days`; end-time actions apply to the named task.  Explicit `keep` suppresses rotation at that boundary.  `ask` remains available for callers requesting a decision, including `--action ask`; it is not the suggested everyday setting.  Invocation flags affect only that invocation.  A one-time answer never becomes a saved preference unless the user asks to change configuration.

### Age-based maintenance and useful archives

At the first eligible start boundary, explain that originals are resumable backups and explicit archives are durable Markdown records.  Present old task names, IDs, session counts, last-update times, available sizes, and known archive paths together.  Allow task exceptions and offer:

- **Save and clean up:** create and verify an archive, report its path, then remove originals.
- **Keep:** retain resumable originals.
- **Delete:** remove selected originals without a new archive; report any existing archive that remains.
- **Decide later:** retain records and record a 7-day deferral in the host's existing progress record.

Apply answers to the exact displayed IDs after checking their current state.  A dismissal supplies no deletion authorization.  Keep one pending decision and carry candidate IDs, last presentation, and reminder date across handoffs so the same question is not repeated at every review or boundary.  Persistent CLI snoozes and cross-host claims are not implemented; the host owns this record.  A deferral suppresses age-based questions, not the configured capacity rotation.  Choose an explicit export or reopen work that must survive backup rotation.

An agent may suggest saving a particularly useful discussion even before maintenance is due: for example, a design tradeoff resolved through human feedback, an unexpected insight, or substantial AI cowork/review discussion.  Give a concrete reason and offer once at a suitable work boundary.  Use `history export` to keep originals unless cleanup was also requested.  Routine approval, conversation length, or task completion alone does not merit an archive suggestion.  Respect a declined suggestion and inspect known archives before proposing another copy.

For saved archives, explicit maintenance can offer **Keep**, **Move**, or **Delete**, showing exact paths and whether an archive is the only remaining copy.  Moving verifies the destination before removing the source.  Archive age alone does not generate reminders or authorize deletion.  The CLI currently records only the latest archive location per task, not an inventory of every export; retain known locations in the host record and report inventory limits.  Local saving does not authorize PR, git-notes, or other publication.

### Concurrency, failures, and recovery

Serialize task maintenance and lock every member session before saving or removing records.  A reopened, changed, or busy task leaves the selected cleanup set.  Strict archival requires a verified save; export failures leave originals intact.  Interrupted removal keeps the verified archive and remaining-session journal, requiring an explicit retry with the same task and action.  Report partial removal and errors, including when a new task was successfully started despite failed maintenance.

Old open tasks and ungrouped sessions can be inspected during requested maintenance.  Offer continuation, grouping, explicit completion, saving, or deletion; age and process absence do not establish completion.  Treat corrupt membership as a repair case.  Protect uncertain staging and recovery data; successful exports remove their own staging files.  Task receipts remain small and contain no hidden second copy of removed review contents.

Verification covers quiet recent completion, age-only decision requests, oldest-first rotation with headroom, protection of busy and ongoing work, archive survival, directory scope, explicit fixed policy, and interrupted cleanup.  Complete archive inventory, persistent snoozes, archive move/delete commands, and background maintenance remain outside the current CLI.

### Related upstream behavior

[Crit's README](https://github.com/tomasz-tomczyk/crit/blob/main/README.md), checked on 2026-10-18, documents explicit `crit cleanup` and an age-based background sweep when reviews start (`stale_review_days`, default 14).  Critty uses explicit task membership and task-boundary decisions to preserve multi-review work and future interchange.  A stale review alone is insufficient evidence that its task or only saved copy may be discarded.

## Live records

The default state root is `$XDG_STATE_HOME/critty`, usually `~/.local/state/critty`.

```text
critty/
  history/
    <task-id>.json
    history.lock
  reviews/
    <session-id>/
      review.json
      history-task.json
      <other session files>
  sessions/
    <session-id>.json
  archives/
    <archive folder>/
```

Task and session IDs are 12 lowercase hexadecimal characters.  A task record stores `id`, `project`, `name`, `cwd`, `started_at`, `sessions`, and `pruned`.  Optional fields describe completion (`completed_at`), the saved archive (`archive`), and missing sessions (`missing_sessions`).  Interrupted removal records `pruning`, `action`, and `pending` session metadata so the same action can resume without losing the archive location.

`history-task.json` contains `{"task":"<task-id>"}` and supports direct membership lookup.  The task record owns the session list; the membership file must agree with it.  The ordinary session registry supplies each session's location and working directory.  These live paths are operational metadata, separate from the portable archive layout.

## Archive directory layout

The default archive parent is `<state root>/archives`; global policy or an export destination can select another parent.  Each export creates a new directory:

```text
2026-10-18 134000 - Critty - Review history - <task-id>-<unique suffix>/
  review.md
  attachments/
    <session-id>/
      <attachment-id>/
        document.md
        assets/
          <content-sha256>/
            preview.png
      source-<path-hash>/
        source.md
        assets/
          <content-sha256>/
            diagram.svg
  assets/
    <content-sha256>/
      comment-image.png
  sessions/
    <session-id>/
      review.json
      <original session files>
  manifest.json
```

The folder name combines local date/time, project name, task name, task ID, and a uniqueness suffix.  Human-readable components are sanitized for filenames.  IDs identify the records; consumers should not infer metadata by parsing the folder name.  The manifest retains timestamps with their UTC offsets.

Session and document directories separate repeated filenames.  Source snapshot folders use the first 12 hexadecimal characters of SHA-256 over the original source path.  Image directories use the full content SHA-256.  Image storage is local to its containing document, even when identical bytes also occur in another document.

### Main record and documents

`review.md` is the entry point.  It records the project/task identities, saved sessions, round and verdict, overall and file comments, anchors, authors, timestamps, resolution state, replies, and any saved overall draft.  It links to readable attachments, source snapshots, and original review JSON.  Attachment metadata includes the original source, revision, and whether the attachment was removed.

`attachments/` holds saved document content rather than a fresh read of the current source file.  Attached documents retain their attachment IDs.  Other reviewed files use `source-<path-hash>` folders.  Partial snapshots are identified in the main record.  An attachment already represented in the round snapshots is not emitted twice.

Each attachment or source snapshot has its own `assets/`.  Images referenced by comments or replies in `review.md` use the top-level `assets/`.  This keeps an attachment and its images together when browsing or reusing it.

### Links and image provenance

Markdown image destinations, image links, reference-style images, and HTML `img` sources are rewritten to saved local images.  Links to known saved documents are rewritten to their archive destinations.  Generated links are relative to the containing document, so moving the complete archive preserves those links.  Fenced examples remain text.  Unknown links remain unchanged; remote resources remain URLs and are not downloaded.

The exporter accepts local PNG, JPEG, GIF, WebP, and SVG images and checks their contents.  Managed comment images come from session attachments.  Other local image bytes are read at export time and may differ from the reviewed revision.  Asset provenance records this distinction; an archive must not imply that current image bytes were part of an earlier review.

### Original session data

`sessions/<session-id>/` copies regular session files byte-for-byte, including review JSON, attachments, forms, and saved plan versions.  Runtime `run.lock` and `review.json.lock` files are excluded.  Unsupported files such as symlinks fail the export.

Readable Markdown is a derived view.  Original JSON preserves fields the exporter does not understand and data not rendered in that view.  Preserving these files leaves room for future Crit interchange without promising bidirectional resume compatibility.

## Manifest version 1

`manifest.json` describes the archive's saved state:

| Field | Meaning |
| --- | --- |
| `version` | Integer archive version, currently `1`. |
| `saved_at` | Export timestamp in RFC 3339 format, with optional fractional seconds and a UTC offset. |
| `task` | Snapshot of the task record, including project/task identities and ordered session IDs. |
| `sessions` | Original session registry metadata for the sessions actually included. |
| `files` | Archive-relative path to lowercase hexadecimal SHA-256 of the saved bytes.  Paths use `/` separators. |
| `assets` | Optional map from archived image path to `source`, `origin`, and `captured_at`. |
| `warnings` | Optional list of incomplete-export warnings. |

Asset origins are `session attachment` and `local file at export`.  `source` identifies the source image path; `captured_at` records the export capture time.  Original absolute paths are provenance, not required lookup locations for browsing the archive.

The file map includes generated documents and original session files.  The manifest does not hash itself.  Verification accepts version 1, requires a nonempty file map, rejects paths escaping the archive, and checks every listed file's digest.  Digests detect changed or missing listed files; they do not authenticate the reviewer or independently prove authorization.

## Save, verify, and remove

1. Lock history changes and the task's session data.  Refuse active or busy reviews.
2. Write a staging directory under the destination parent, including readable documents, original files, and the manifest.
3. Verify the listed files, then rename staging to a unique final folder.  Reject archive destinations inside a source session directory.
4. For `export`, retain all original sessions.
5. For the `archive` action, durably record the verified archive path, verify it again before removal, and journal the remaining sessions while removing them.

Export failures leave originals intact.  Interrupted removal retains its progress and is resumed using the recorded action.  Start-time maintenance reports failed or skipped tasks without blocking new work.  The archive is not reported as a substitute for originals until saving and verification succeed.

## Incomplete exports and limits

Strict export and the `archive` action require all member sessions and referenced local images.  `export --allow-incomplete` can save remaining sessions and leave unavailable external image references unchanged.  Missing sessions and images appear as warnings in both `review.md` and the manifest.  It always keeps originals and cannot bypass corrupt session data or missing managed session images.  Automatic archival stays strict.

The archive represents state available at export.  It is not a complete edit transcript, every historical round, a signature, or independent evidence of the original host's approval result.  Retain separate approval evidence when required.  Local storage does not authorize publishing records to a PR, git notes, or another service.
