---
name: critty
description: Review code changes or a document with Critty's terminal UI and address the reviewer's inline comments round by round. Use when the user or an approval workflow selects Critty; a generic review request does not count.
allowed-tools: Bash(critty *), Read, Edit, Grep, Glob
argument-hint: "[file]"
---

# Review with Critty

Critty opens a terminal UI where a human leaves inline comments, then hands those comments back to you.  Run this loop when the user explicitly selects Critty or a caller invokes `/critty-approval` for a Critty approval review.

## Local review and approval

Critty is a local CLI and human review TUI, not a hosted review service.  It reads the selected files or Git diff and stores review state under `$XDG_STATE_HOME/tcrit` (default `~/.local/state/tcrit`).  Review processes communicate through local Unix sockets and use tmux or Herdr to display the TUI.  Starting or submitting a review does not upload its contents or publish changes.

`critty terminal prepare` only removes expired local marker requests, saves a random marker with a two-minute expiry, and prints its JSON.  It does not read review content or contact an external service.  For other commands' effects, including deletion, installation, and delegated execution, see the `/critty-cli` skill.

A successful review's `approved: true` reports the human's decision about the displayed content.  Match it to the invocation, session, and reviewed scope; copied output is not approval.  Starting a review does not itself authorize a commit, push, deployment, or other subsequent action.

## Prerequisites

The `critty` binary must be on PATH.  If it is missing:

```bash
go install github.com/crittylabs/critty/cmd/critty@latest
```

## Step 1: Choose the command from the arguments

The CLI picks the review mode from its arguments, so do not ask the user which mode they want.

```bash
critty $ARGUMENTS            # a file reviews that document; no argument reviews the git changes
critty --staged              # review only changes staged in the index
critty --scope=all           # HEAD versus working tree, including untracked files
critty --scope=unstaged      # index versus working tree, including untracked files
critty --unstaged            # alias for --scope=unstaged
critty --scope=main...       # merge base with main versus HEAD, using committed contents
critty plan <file>           # a plan written in this conversation: each round is saved as a new version
git diff <base> <head> | critty --diff  # review a supplied Git unified diff
critty --null --attachment notice.md  # review only attachments, without Git
```

With no argument, review a plan file written earlier in this conversation with `critty plan <file>`; otherwise run bare `critty` for the git changes.

For a supplied diff, use `critty --diff=changes.diff` or `critty --diff changes.diff`, or pipe the producer into `critty --diff` (`critty review --diff` is equivalent).  Relative and absolute paths are accepted; bare `--diff` and `-` as its input read stdin.  This works outside a Git repository.  Do not combine `--diff` with `--scope`, `--code`, `--staged`, or `--unstaged`.  Keep the producer command and working directory for later rounds: Critty reviews a saved snapshot, not the current working-tree files.

`--scope=staged` is equivalent to `--staged`; `--scope=unstaged` is equivalent to `--unstaged`.  Conflicting scope flags are rejected.  `--scope=A..B` compares two committed snapshots; `--scope=A...B` compares their merge base with B.  B can be omitted to mean HEAD (`--scope=main..` or `--scope=main...`).  `--scope` cannot be combined with a document or `--diff`.  It stays fixed for that session; each new review has independent comment storage, even with identical scope arguments.  Use the session ID from the finish prompt on comment commands (`critty comments --session <id>` and `critty comment --session <id>`, including replies and bulk input).  Reconnecting with `--session <id>` retains the selected scope.  Committed comparisons read the right endpoint, which may differ from files on disk.  Without explicit flags, the scope is all; a clean working tree does not fall back to committed changes.

For a single document, `critty FILE:LINE` or `critty review FILE:LINE` starts at that line.  To start at a requested file or line, add `--focus 'path/to/file[:LINE]'`, for example `critty review --staged --focus 'internal/tui/app.go:120'` or `critty --session <id> --focus 'internal/tui/app.go'`.  The path must be in the review; a line must be a positive, 1-indexed new-side source line present in the reviewed content.  Relative and absolute paths are accepted, including the original plan path for plan reviews.  From round 2 onward, automatic focus selects the first visible thread, in file and display order, with a reply added since the previous submission by an author other than the reviewer.  An explicit document line or `--focus` overrides that automatic focus for the invocation; `--focus` wins when both are given.  Pass the option again in later rounds when needed.

Use repeated `--attachment FILE` flags to show text documents before any review, without staging them.  Attached documents have separate comment paths and saved snapshots.  The finish prompt includes their text and original paths; use that mapping when addressing comments.  Plain `--session <id>` reuses snapshots.  To revise them, pass the complete ordered list again; `--clear-attachments` removes the list while retaining its comment threads.

Reviewed PNG, JPEG, GIF, and WebP files have inline previews on Ghostty and Kitty terminals with Unicode image placeholder support.  Detection is automatic; tmux requires `allow-passthrough on`.  Modified and renamed images show Before and After comparisons, side by side or stacked to fit the pane.  Previews use the selected scope, including the old side for deleted files.  GIF shows the first frame.  Animated WebP, images over 5 MiB or 16 megapixels, unsupported terminals, and supplied binary diffs keep the binary placeholder.  Leave file comments on images; previews do not add source lines or change comment anchors.  Attached review documents remain text and Markdown.

Use `--null` for attachment-only reviews, including outside a Git repository.  Empty diff input (`--diff=/dev/null`, an empty file, or empty stdin) also works with attachments.  Both require at least one active attachment.  `--null` cannot be combined with a file argument, `--diff`, `--scope`, `--code`, `--staged`, or `--unstaged`.  Resume with `critty --session <id>`; pass the complete attachment list to refresh its snapshots.

For approval documents, publication drafts, confirmations, choices, or per-operation permissions, use `/critty-approval`.  It covers YAML frontmatter, interactive task lists, `--result FILE`, and binding the human result to the exact action before execution.  Use `critty --null --attachment FILE` for attachment-only reviews.

## Step 2: Launch and wait

Choose a wait mechanism before launching the Step 1 command.  Use the tools exposed by the host: a background task with retained output, or a resumable execution handle.  Distinguish a wait timeout from an execution timeout that terminates the process.  A longer execution timeout alone does not ensure that Critty and its output survive it.  If the host cannot preserve both, ask the user to run Critty in their terminal and return its finish output instead.

When the background task tool takes a `timeout`, that value is the execution limit: when it expires the host kills Critty and the open round is lost.  Set it to the maximum the tool allows so it outlasts the review; it is not the 10-minute wait deadline below.  A host that notifies on command exit resumes the agent itself, so end the turn at the deadline instead of polling.

For Codex, prepare a terminal marker immediately before **every** review launch, including retries and later rounds:

1. Run `critty terminal prepare` and read its JSON `id` and `marker`.
2. Display `marker` verbatim on its own line in a user-visible progress message.  Finish edits and other output before this message; then proceed directly to launch.
3. Add `--terminal-request <id>` to the Step 1 command.  The request expires after two minutes and is consumed once.  A new request is needed after a failed launch or for another round.

The reviewer must have launched Codex through `critty codex` or the bundled wrapper, or use a terminal filter that reports markers.  Critty accepts filter notifications first, then searches only the visible text in registered panes.  If detection fails, follow the launch-failure procedure below; keep the marker visible in a sufficiently wide terminal.  Multiple matches require leaving one terminal displaying the conversation before preparing a fresh marker.  Use the normal launch path for other agents.

Run the Step 1 command; retain its working directory, session ID, and execution handle or output location.  Critty opens a Herdr tab or tmux pane and closes it after each round.  Without a supported multiplexer, ask the user to run it in their terminal and return the session ID for reading results.

Before waiting, announce in the conversation's language that the review is open and you will wait for submission.  Ask the user to send `hey` after submitting if the wait times out, so you can collect the result.

Wait up to 10 minutes by default, measured from one deadline recorded when the wait begins, and collect the result as soon as the command finishes.  Use the host's timed wait on the task or handle, or a shell loop that checks the process or its output every few seconds; either must return on completion instead of sleeping out the deadline.  Wait on the review command itself, never on `critty status`: saved sessions stay listed until cleanup, so a loop that waits for an empty list runs to the deadline while the submitted result sits unread.  If a tool call returns while the command is still running, keep waiting on the same handle until completion or the deadline.  When an outer orchestration call yields, wait on that outer handle before issuing another inner wait.

If the command finishes, continue to Step 3.  If the overall deadline expires while the command is still running, retain the working directory, session ID, execution handle, and any captured output, then end the turn.  Resume on a completion notification or the user's message.  A notification must actually resume the agent; otherwise rely on the user's message.  Collect the original command's result before starting another review.  Approval may remove saved review data, so the command's output must remain recoverable.

Minimize token consumption while awaiting human review.  Leave files unchanged and perform no other work.  Keep waiting inside tools; avoid status polling and repeated analysis or unchanged waiting notices.  Follow higher-priority host notification requirements.  Continue waits only within the original deadline, then yield to the user.  A wait timeout is not review cancellation or approval.

Keep reviewed content unchanged until submission.  On task cancellation or replacement, run `critty stop --session <id>` and collect the result.  Preserve saved reviews; use `clear` only for explicitly requested deletion.

## Step 3: Read the result

Handle the command outcome before continuing:

- **Launch failure before the TUI was usable:** diagnose and correct the cause; sandbox failures may use normal escalation without a new review request.  Confirm the command exited and inspect `critty status` (a printed ID does not prove the TUI opened).  Retry with `--session <id>` if saved, otherwise the original command.  If another review is running, launch state is uncertain, escalation is denied, or the error persists, preserve state and report the blocker.
- **Interrupted after opening, or cancelled:** read new saved comments and replies, preserve state, and report the interruption.  End the turn; wait for explicit chat instructions before acting on feedback or resuming from the original directory.
- **Exit 0:** read the finish prompt and all returned threads, including resolved comments and replies.  Approval may have deleted saved data.  For `approved: false`, follow the feedback and next-round commands.  For `approved: true`, address any new instructions and continue the task; changes to approved content require another review before committing.

If the result cannot be recovered, report that limitation.  Silence, elapsed time, and missing saved data are not approval.

If Codex has no registered terminal and no notifying filter, ask the user to resume from their intended tmux pane or Herdr terminal with `critty codex resume` (or the bundled `codex` wrapper).  The wrapper registers a search candidate without changing Codex arguments or settings.  After they resume, prepare and display a new marker and retry the saved Critty review by its session ID.  Terminal environment inherited by a daemon may refer to another pane; let Critty resolve the marker rather than using those values yourself.

Locate feedback using `path`, line range, and `anchor` (the original text).  Treat `drifted: true` line numbers as approximate; focus on `quote` when present.  Outside the finish prompt, `critty comments --session <id> --json` lists unresolved comments.  Use the session ID for all comment commands, including supplied-diff reviews, replies, and bulk input.

For image attachments, follow the finish prompt's base directory and open referenced images with your image-viewing tool, including those in resolved threads.  On approval, after reading them and recording any remaining instructions, run the cleanup command printed by Critty.  This is the approval cleanup step; it deletes the completed session and its images.  If image reading fails, preserve the session and report the failure.  Unapproved rounds and stopped reviews retain attachments.

## Step 4: Address new feedback

Read each comment together with its replies, authors, and the work already recorded in this conversation.  Unresolved means the reviewer has not resolved it; it does not by itself request another edit or reply.

- Act on reviewer feedback that has not yet been addressed, including a new reply or an edited request.
- When the latest substantive message is your own comment or reply and the reviewer has not responded, leave the thread unchanged.  This also applies to threads you started and to earlier agents' completion replies.
- Add another reply only for new reviewer feedback or a material correction or additional result not already reported.  A new round, an unresolved flag, or repeating that work is complete is not new information.

For feedback that needs action:

1. Locate the target from `path`, the line range, and `anchor`.
2. Change the file as the `body` asks.  Apply a `suggestion` block verbatim when the comment contains one.
3. Reply once with the change or answer, using the reply form shown in the finish prompt and the session ID.  Plan reviews use the same `--session <id>` form:

```bash
critty comment --session <session-id> --reply-to <id> --author 'Claude Code' '<what you did>'
```

Never pass `--resolve`.  Resolving is the reviewer's decision.

For three or more needed replies, write them to a JSON file and submit once:

```bash
critty comment --session <session-id> --json --file replies.json --author 'Claude Code'
```

The `/critty-cli` skill documents the JSON format and the other comment commands.  If no thread needs action, proceed to the next round without edits or additional comments.  Only `approved: true` completes the review.

## Step 5: Start the next round

For Codex, repeat Step 2's marker preparation and display, and append the new `--terminal-request <id>` to the continuation command below.

After an unapproved round submitted with exit status 0, run the command printed in the finish prompt from the original working directory.  `critty --session <id>` opens a new TUI from saved comments and round context; it retains the scope and refreshes code or document contents.  It advances past a submitted round, or resumes an interrupted round when explicitly requested by the user in chat.

For a revised plan, use `critty plan --session <id> <file>` (or pipe the plan to it) to save a new version.  Plain `critty --session <id>` opens the saved plan without replacing its content.

For a revised supplied diff, regenerate the input with the original producer:

```bash
git diff <base> <head> | critty --diff --session <id>
```

For file input, update it first, then run `critty --diff=changes.diff --session <id>`.  Plain `critty --session <id>` opens the saved diff without replacing it.  Keep the explicit session ID; omitting it creates an independent review.  Inspect drifted comments against their anchors when the supplied context is incomplete.

Wait for the command to finish, then return to Step 3.  Stop when the result is `approved: true`, or follow the stopping procedure when the user cancels or replaces the task.
