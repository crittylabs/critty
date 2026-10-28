# Review sessions

## Task history

Normal review start/resume reports history warnings on stderr while preserving stdout and verdicts.  `critty doctor` includes fresh history checks and suggested remedies, independent of the passive warning throttle.  On `critty: warning: history:`, follow [critty-history](../../critty-history/SKILL.md#handle-review-flow-warnings) for targeted inspection, policy-driven cleanup, or a batched user decision.

Use [critty-history](../../critty-history/SKILL.md) before the first review and after the whole work task, for task bookkeeping; Critty applies the configured policy.  Inspect capacity or old records only after a command warning/result or explicit maintenance request.  Cleanup and exports use Critty commands, without storage-tree traversal or custom maintenance scripts.  `critty history --help` lists commands; `--task <id>` on `critty`, `review`, and `plan` associates a review without prompting at each round.

## Targeting a session or plan

`critty sessions list` (or `critty sessions`) lists saved reviews for the current project.  Git worktrees sharing a common Git directory belong to one project; outside Git, the directory identifies the project.  New sessions save this identity; listings also retain identities detected for older entries without changing review data, start times, or registry modification times.  Git registration can identify a removed legacy worktree until pruning removes that information.  `--directory DIR` selects the project containing DIR, resolving relative paths and symbolic links.  `--all` includes every project and cannot be combined with `--directory`.  The default `--sort project` groups sessions by project, with running sessions first and then newest starts first.  `--sort time` sorts all selected sessions by start time, newest first, without prioritizing projects or running reviews.  Both orders apply to the table and JSON.  The table shows local start times and UTC offsets in `STARTED`, using `-` for missing or invalid times, which sort after valid times at the same priority.  Worktree paths are omitted from the table.  `--json` retains `cwd` and adds stable `project_id`, canonical `project_name`, optional `git_common_dir`, `workdir` (active workspace), `head_ref` (last resolved comparison head), `status`, `round`, `verdict` for finished rounds, and `error` when review data cannot be read.  Status values are `running`, `stopped`, `approved`, `changes-requested`, `feedback`, `submitted` (legacy finished rounds without a verdict), and `unavailable`.  A saved verdict does not authorize new actions.  The legacy `critty status` stays scoped to its current directory with the same JSON fields.

Each new review invocation creates independent storage.  Always use the session ID printed at startup or in the finish prompt, including for plans and bulk replies.  Commands without an ID refuse to guess when multiple saved reviews match.

```bash
critty status                              # saved sessions in this directory, with running status
critty comments --session <id> --json
critty comment --session <id> --reply-to <comment-id> --author '<agent-name>' '<what you did>'
critty comment --session <id> --json --file replies.json --author '<agent-name>'
```

`--scope` stays fixed for a session.  Identical scope arguments, document paths, or plan names can have independent session IDs.  `--plan <slug>` selects a plan only when its name is unambiguous; prefer the ID.

To resume a stopped session, run `critty sessions resume <id>` from a checkout of the same Git project.  Critty uses that checkout even if the original worktree has been deleted.  Git scopes read the active checkout.  Commit and range expressions are resolved there; recorded revisions provide a fallback when a named revision has disappeared.  Older reviews without recorded revisions still need those references.  Comments and ordinary answers carry forward; changed review inputs invalidate required confirmations and consents.  Saved plan versions and supplied diffs remain snapshots.  A missing document displays its saved full text with a notice until Reload can read a replacement.  When no current Git changes remain, saved full-text snapshots remain available outside the current changes.  Unsaved content and unavailable binary data cannot be reconstructed.

`cwd` remains the creation directory and history owner.  Use `workdir`, falling back to `cwd`, when assigning AI reviewers or resolving current source paths.  Repository attachments keep their identities and saved contents while their source paths move to the active workspace.  External source paths stay external.  Relative attachment, diff, plan-revision, result, summary, and explicit custom-launcher paths use the caller's directory.  Outside the project, Critty restores an available saved workspace.  An old session without project identity cannot move after its original directory and Git registration have both disappeared.

If a command is still waiting for the reviewer, retain its execution handle and collect that command's result instead.  The `critty` skill distinguishes retryable launch failures from interrupted reviews that require explicit chat instructions to resume.  To replace saved input, use `critty plan --session <id> <file>` or `critty sessions resume <id> --diff=<file>`.  The `critty` skill covers the interactive review loop.

## Stopping and deleting reviews

```bash
critty sessions stop <id>        # close the TUI, retaining saved state
critty sessions resume <id>      # resume by ID from any directory
critty sessions remove <id>      # delete one stopped review
critty clear --all               # delete every saved review for this directory
```

Each round closes its TUI and dedicated pane or tab.  On cancellation or a task switch, stop the specific review and collect the blocking command's result; neither cancellation nor a connection error is approval.  A stopped review keeps comments, replies, resolution status, and round context, but unsaved editor text is not retained.  Stop is also safe when that session is already stopped.

Use `clear` when deletion is requested, or when explicitly enabled approval cleanup directs it after its images have been read.  Preserve saved reviews for retrospectives, archiving, and later continuation by default; approval or task completion alone does not authorize clearing them.  `sessions stop` preserves data; `sessions remove` deletes the session and its attachments and refuses active reviews or task members.  For a task member, use `history finish <task-id> --action delete`, or explicitly detach the selected review before clearing it.  `status` reports `task`, `ungrouped`, or `history_error` so membership does not require storage inspection.  Use the history workflow for maintenance under its global policy or selected IDs; `clear --all` is not a substitute for that decision.  The legacy `stop --session <id>` and `clear --session <id>` remain supported.  Legacy `clear --code`, `clear <file>`, and `comment --clear` select saved reviews through their existing targeting rules; use an explicit session ID when several match.
