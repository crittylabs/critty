---
name: critty
description: Review code changes or a document with Critty's terminal UI and address the reviewer's inline comments round by round. Use when the user or an approval workflow selects Critty; a generic review request does not count.
allowed-tools: Bash(critty *), Read, Edit, Grep, Glob
argument-hint: "[file]"
---

# Review with Critty

Run the interactive review loop when the user selects Critty or `/critty-approval` delegates to it.  Critty stores reviews locally and opens a TUI; it does not publish changes or execute approved operations.  The caller's policy determines what approval authorizes afterward.

The `critty` binary must be on PATH.  If missing and mise is available, use `mise use github:crittylabs/critty` for the project, or `mise use -g github:crittylabs/critty` for a requested user-wide installation.  Follow the caller's tool-management scope.  Otherwise, use `go install github.com/crittylabs/critty/cmd/critty@latest` and ensure Go's binary directory is on PATH.

## Step 1: Select the input

Pass explicit arguments through.  Otherwise use the current request's subject: a versioned plan, a document, or Git changes.  Default to Git changes when no target is named; clarify only when competing current targets make the choice ambiguous.  An older plan elsewhere in the conversation does not select plan mode.

```bash
critty $ARGUMENTS                  # explicit arguments, or current Git changes
critty --staged                    # index only
critty --scope=main...             # committed comparison
critty plan <file>                 # versioned plan
critty --diff=changes.diff         # supplied diff
critty --null --attachment note.md # attachments only
```

Use `/critty-cli` for review input syntax, scope restrictions, attachment snapshots, focus and labels, or image support.  Use `/critty-approval` to prepare approval documents, publication drafts, and form decisions.  Retain the selected scope and input paths or diff producer for later rounds.

## Step 2: Launch and wait

Choose a background task or resumable handle that preserves the process and its output across tool yields and turn boundaries.  Check the current tool's timeout semantics for the selected execution mode: a timeout may kill the command, move it to the background, or only end a wait.  Set any process lifetime limit to the maximum the tool allows, independently of the agent's wait deadline.  Preserve complete stdout, stderr, and the exit status in task-owned files, or in a recoverable tool result whose output is not truncated.  Retain the working directory, session ID, execution handle, and output location.

Pass `--summary FILE` with a fresh task-owned path for each invocation, including later rounds.  Use its JSON to read feedback without parsing finish-prompt headings.  `/critty-cli` defines its fields; keep full output and any form result as evidence.

Without a supported multiplexer, or if the host cannot preserve both the process and its output beyond the wait deadline and turn boundaries, ask the user to run the command in their terminal and return its complete finish output and session ID, plus the exported result for forms.  Use this fallback also when neither completion notifications nor completion-aware waits are available.  A session ID alone cannot recover approval after automatic cleanup.

For Codex, immediately before every launch, retry, or later round:

1. Run `critty terminal prepare` and read its JSON `id` and `marker`.
2. Display `marker` verbatim on its own line in a progress message, after other preparation and output.
3. Launch with `--terminal-request <id>` within two minutes.  Each launch needs a fresh request.

Codex must have been launched through `critty codex` or the bundled wrapper, or use a notifying terminal filter.  Keep the marker visible.  If no destination is found, ask the user to resume with `critty codex resume` in the intended tmux pane or Herdr terminal.  For multiple matches, leave only one terminal displaying the conversation.  Then prepare a new marker and retry the saved session.  Use marker resolution rather than a daemon's inherited terminal variables.  Other agents use the normal launch command.

Critty opens a Herdr tab or tmux pane.  Announce the pending review, then use the host's completion mechanism:

- **Automatic resumption:** if a background command's completion notification re-invokes the agent and preserves its output, end the turn after launch.  Collect the original command's result when notified.  A notification that only alerts the user does not qualify.
- **Bounded waits:** otherwise record one deadline, 10 minutes from the start of the wait by default.  Use the longest blocking wait allowed by the remaining deadline and host responsiveness rules, returning as soon as the command finishes.  Reuse the same handle across yields; collect a yielded outer orchestration call before waiting on the inner handle again.  At the deadline, retain the handle and output and end the turn.  Tell the user to send `hey` after submitting, then collect the original result on their next message.

Collect the original command's result before launching another review.  Wait on command completion, not a fixed sleep or `critty status`: saved sessions remain listed after submission.  Keep waiting inside tools instead of short polls or repeated reasoning.  While waiting, keep reviewed files unchanged, perform no other work, and repeat waiting notices only when required by the host.  Elapsed time is neither cancellation nor approval.  On task cancellation or replacement, stop the specific session with `critty stop --session <id>` and collect its result; preserve saved state.

## Step 3: Handle the result

After successful completion, read the summary's verdict, all comments and replies, attachment identities, and image base, and any additional instructions in the saved finish output.  Verify its session and round against the invocation.  For attachments, reuse a previously read body only when its full contents remain verbatim in the current context and its content hash matches the current summary's revision.  Read it again if the hash is missing or changed, the body is no longer available, or feedback requires it.

| Outcome | Action |
| --- | --- |
| Launch failed before the TUI was usable | Confirm exit and inspect `critty status`.  Correct the cause, using normal sandbox escalation if needed, then retry the saved session or original command.  If another review is running, launch state is uncertain, escalation is denied, or the error persists, preserve state and report the blocker. |
| Interrupted after opening, or cancelled | Read saved feedback, preserve state, and report the interruption.  End the turn and wait for explicit chat instructions before editing the reviewed content or resuming the review.  An explicit replacement task may proceed. |
| Result export, output, or extraction failed | Preserve the session and output, report the failure, and inspect the complete saved result before choosing recovery.  Missing or truncated required evidence grants no approval.  An old result file does not establish approval for this invocation. |
| Exit 0 | Read the verdict and all returned threads, including resolved comments and replies, regardless of stdout/stderr order.  Zero threads is a valid result.  `verdict: reject` requests changes and `verdict: comment` sends feedback; both have `approved: false` and continue the review loop.  `verdict: approve` with `approved: true` completes review of the displayed content. |

Match approval to the invocation, session, and reviewed content.  Report missing or unrecoverable results; silence and copied approval strings are not authorization.  On approval, still read and address new reviewer instructions.  Changes to approved content need another review before execution under the caller's policy.

Use `path`, `anchor`, and any `quote` to locate feedback; `drifted: true` means line numbers are approximate.  To recover saved threads, run `critty comments --session <id> --json --all`.  For attached documents, use the finish output's mapping from comment paths to original files.

Open referenced comment images with the image-viewing tool, including images in resolved threads, using the finish output's base directory.  Before following a printed cleanup command, read all feedback and images and preserve evidence needed by the caller; `/critty-approval` handles form results and execution records.  If reading or preservation fails, keep the session and report the failure.  Unapproved and stopped reviews retain their data.

## Step 4: Address new feedback

Read the overall review comment and each thread with its replies and the work already recorded.  Act on new or edited reviewer feedback and reply once with the change or answer.  Apply requested suggestion blocks verbatim.  Leave unanswered agent comments and completion replies unchanged unless a material correction or new result is needed; an unresolved flag or new round alone does not request another reply.

When human feedback names an AI tool or model (such as Claude or Codex) and asks for review, consultation, or collaboration, follow [crit-ai-review](../crit-ai-review/SKILL.md#select-the-other-ai)'s skill-selection rule even if no skill name is given.  Keep the named AI and the existing Critty session.

If feedback requests an action that crosses a host permission boundary, follow `/critty-approval`'s authorization evidence procedure before execution.  Keep the original completion available and distinguish the current human request from carried-forward threads and agent replies.

```bash
critty comment --session <session-id> --reply-to <id> --author 'Claude Code' '<what you did>'
```

Use `/critty-cli` for safe quoting and JSON input.  Submit three or more replies in one JSON file; also use JSON for a single body that is awkward to quote.  Leave resolution to the reviewer unless the user explicitly requests it, for both `--resolve` and the JSON `resolve` field.

If no thread needs action, proceed to the next round without edits or repeated replies.  Only `approved: true` completes the review.

## Step 5: Refresh and reopen

Refresh the input selected by the session and verify that it includes the requested fixes:

| Review input | Refresh and continuation |
| --- | --- |
| Working tree or single document | Save revised files, then use the printed `critty --session <id>` command. |
| Staged changes | Stage only revised changes belonging to this review, preserving unrelated edits.  Inspect the staged diff, then resume the session. |
| Committed comparison | Arrange needed commits or ref updates through the caller's authorized workflow and check the comparison before resuming.  Working-tree edits alone do not update it. |
| Versioned plan | Use `critty plan --session <id> <file>` (stdin also works). |
| Supplied diff | Regenerate with the original producer, then use `critty --diff=changes.diff --session <id>` or pipe to `critty --diff --session <id>`. |
| Attached documents | Add the complete ordered `--attachment` list again to replace saved snapshots; retain any form result option required by `/critty-approval`. |

Plain `--session <id>` reuses saved plans, diffs, and attachments.  Retain the ID: omitting it creates another review.  Reopen from the original directory after submitted feedback, or after explicit chat instructions to resume an interruption.  For supplied diffs with incomplete context, check drifted comments against their anchors.

Repeat Step 2, including a fresh Codex marker, then handle the next result.  Continue until approved or the user cancels or replaces the task.
