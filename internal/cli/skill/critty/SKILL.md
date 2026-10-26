---
name: critty
description: Review code changes or a document with Critty's terminal UI and address the reviewer's inline comments round by round. Use when the user or an approval workflow selects Critty; a generic review request does not count.
allowed-tools: Bash(critty review *), Bash(critty plan *), Bash(critty sessions resume *), Bash(critty sessions stop *), Bash(critty comment *), Bash(critty comments *), Bash(critty status *), Bash(critty terminal prepare), Read, Grep, Glob
argument-hint: "[file]"
---

# Review with Critty

Run the interactive review loop when the user selects Critty or `/critty-approval` delegates to it.  Critty stores reviews locally and opens a TUI; it does not publish changes or execute approved operations.  The caller's policy determines what approval authorizes afterward.

The `critty` binary must be on PATH.  If missing and mise is available, use `mise use github:crittylabs/critty` for the project, or `mise use -g github:crittylabs/critty` for a requested user-wide installation.  Follow the caller's tool-management scope.  Otherwise, use `go install github.com/crittylabs/critty/cmd/critty@latest` and ensure Go's binary directory is on PATH.

Before the first review of a work task, use [critty-history](../critty-history/SKILL.md) for task registration or continuation.  Reuse any supplied task ID, pass `--task <id>` for each new review, and retain it across rounds and agent handoffs.  Return to that skill when the whole task is complete to apply its retention policy; individual approval does not trigger another cleanup question.

When review startup or resume prints `critty: warning: history:`, follow [critty-history](../critty-history/SKILL.md#handle-review-flow-warnings) after collecting the current round's result.  A command-reported warning or maintenance result, or an explicit user maintenance request, triggers inspection.  Task boundaries alone do not trigger independent capacity/age checks.  Use Critty commands through that skill for cleanup and exports; do not explore storage or write maintenance scripts.

When existing files outside source control need review before application, read [temporary-tree review](references/temporary-tree.md) **before editing the originals**.  It reviews independent copies as a Git diff, then checks for concurrent changes before applying approved contents.

## Step 1: Select the input

Pass explicit arguments through.  Otherwise use the current request's subject: a versioned plan, a document, or Git changes.  Default to Git changes when no target is named; clarify only when competing current targets make the choice ambiguous.  An older plan elsewhere in the conversation does not select plan mode.

```bash
critty review $ARGUMENTS                  # explicit review arguments, or current Git changes
critty review --staged                    # index only
critty review --scope=main...             # committed comparison
critty review --scope=HEAD                # single commit
critty plan <file>                        # versioned plan
critty review --diff=changes.diff         # supplied diff
critty review --null --attachment note.md # attachments only
```

Start reviews with `critty review`; resume with `critty sessions resume <id>`.  The `review` subcommand does not accept `--session`.  Use `critty sessions stop <id>` to stop a review while retaining its saved state.  These forms match this skill's narrow tool grants; preparation, application, installation, and history maintenance use the host's normal permissions.

Use `/critty-cli` for review input syntax, scope restrictions, attachment snapshots, focus and labels, or image support.  Use `/critty-approval` to prepare approval documents, publication drafts, and form decisions.  Retain the selected scope and input paths or diff producer for later rounds.

## Step 2: Launch and wait

Choose a background task or resumable handle that preserves the process and its output across tool yields and turn boundaries.  Check the current tool's timeout semantics for the selected execution mode: a timeout may kill the command, move it to the background, or only end a wait.  Set any process lifetime limit to the maximum the tool allows, independently of the agent's wait deadline.  Preserve complete stdout, stderr, and the exit status in task-owned files, or in a recoverable tool result whose output is not truncated.  Retain the working directory, session ID, execution handle, and output location.

Pass `--summary FILE` with a fresh task-owned path for each invocation, including later rounds.  Use its JSON to read feedback without parsing finish-prompt headings.  `/critty-cli` defines its fields; keep full output and any form result as evidence.

If no review launcher is available, or if the host cannot preserve both the process and its output beyond the wait deadline and turn boundaries, ask the user to run the command in their terminal and return its complete finish output and session ID, plus the exported result for forms.  Use this fallback also when neither completion notifications nor completion-aware waits are available.  A session ID alone does not recover the original completion output, especially when approval cleanup is explicitly enabled.

### Launcher selection

Track launcher observations within the current agent session, separately from the saved Critty review session.  Use `--launcher` only when the user requests a launcher; otherwise omit it so Critty applies its configuration and automatic selection.  A previously successful launcher is an observation, not an instruction to pin it.

1. In Codex, or another host using a notifying terminal filter, prepare and display a fresh terminal marker before each launch by default, including later review rounds, and pass `--terminal-request <id>`.  Other hosts omit the marker and `--terminal-request`, using Critty's normal terminal detection.  If the user explicitly selects a launcher that needs no marker, omit marker preparation and `--terminal-request` immediately.
2. Read the selected launcher from startup output (`launcher: NAME`) or the completed invocation's summary (`launch.launcher`).  On failure, read the command's error output and record that no launcher succeeded; a summary may not have been written.  Do not treat an attempted candidate as a successful selection.
3. Once a launcher that needs no marker succeeds, omit marker preparation, display, and `--terminal-request` for subsequent launches in this agent session.  Desktop launchers and reported `foreground` operation need no marker.  A custom launcher may use the bound terminal context; omit markers only when its contract establishes that it does not need that context.  When uncertain, retain markers.  Multiplexer launchers need a fresh marker in marker-bound hosts.  Continue observing each selection without pinning `--launcher`.  Keep markers disabled until the agent session restarts or the user explicitly requests a multiplexer launcher.
4. If the user reports that the review opened in an unexpected terminal, propose prioritizing their preferred launcher in Critty's global `config.toml` with the `launchers` array.  Check whether a project `.critty.toml` replaces that array before proposing the effective setting.  Until that setting is actually written, use their requested launcher with `--launcher` on each invocation.  Once the setting is saved, return to automatic selection without the flag.  Follow the [configuration reference](../critty-cli/references/configuration.md) for the path, precedence, and authorization to edit settings; a proposal alone is not a saved setting.
5. If the agent session was interrupted and resumed, whether detected by the host or reported by the user, announce that the launch destination will be rediscovered.  Clear the remembered marker decision and repeat step 1 with automatic selection.  Preserve any still-applicable explicit user launcher instruction, including a temporary override awaiting a saved configuration.  Ordinary `critty sessions resume` between review rounds is not an agent-session restart.

Automatic selection runs again on every invocation.  Omitting markers after a desktop fallback does not preserve that desktop destination: normal detection may select a multiplexer on the next launch, including one inherited by a daemon-backed host.  Observe the new selection and use step 4 if the user wants a stable preference.

Keep the latest selection, marker decision, and any explicit user override or pending configuration change in continuation context, but reset observations after an agent-session restart as above.  See [critty-cli](../critty-cli/SKILL.md#review-launchers) for launcher names and output contracts.

When a marker is needed, immediately before launching:

1. Run `critty terminal prepare` and read its JSON `id` and `marker`.
2. Display `marker` verbatim on its own line in a progress message, after other preparation and output.
3. Launch with `--terminal-request <id>` within two minutes.  Each launch needs a fresh request.

Marker binding for Codex requires `critty codex`, the bundled wrapper, or a notifying terminal filter.  When sending a marker, keep it visible and use its resolved destination rather than substituting a daemon's inherited terminal variables.

Marker resolution failures, including ambiguous matches, allow Critty to try later desktop or custom candidates automatically.  Inspect the actual result instead of retrying with an agent-chosen `--launcher`.  If startup fails, collect the original result and check for an active review before retrying.  Preserve positive SSH restrictions and explicit requirements to use the existing terminal.  If the existing terminal is required, ask the user to restore the binding (for example, through `critty codex resume`); for ambiguous matches, ask them to leave only one terminal displaying the conversation before preparing another marker.

With a resolved terminal, Critty opens a dedicated pane or tab in the detected multiplexer and can fall back to a desktop window when the multiplexer is unavailable.  Announce the pending review, then use the host's completion mechanism:

- **Automatic resumption:** if a background command's completion notification re-invokes the agent and preserves its output, end the turn after launch.  Collect the original command's result when notified.  A notification that only alerts the user does not qualify.
- **Bounded waits:** otherwise record one deadline, 10 minutes from the start of the wait by default.  Use the longest blocking wait allowed by the remaining deadline and host responsiveness rules, returning as soon as the command finishes.  Reuse the same handle across yields; collect a yielded outer orchestration call before waiting on the inner handle again.  At the deadline, retain the handle and output and end the turn.  Tell the user to send `hey` after submitting, then collect the original result on their next message.

Collect the original command's result before launching another review.  Wait on command completion, not a fixed sleep or `critty status`: saved sessions remain listed after submission.  Keep waiting inside tools instead of short polls or repeated reasoning.  While waiting, keep reviewed files unchanged, perform no other work, and repeat waiting notices only when required by the host.  Elapsed time is neither cancellation nor approval.  On task cancellation or replacement, stop the specific session with `critty sessions stop <id>` and collect its result; preserve saved state.

## Step 3: Handle the result

After successful completion, read the summary's verdict, all comments and replies, attachment identities, and image base, and any additional instructions in the saved finish output.  Verify its session and round against the invocation.  The active source directory is `workdir` when present, otherwise `cwd`; `cwd` retains the creation directory for history.  For attachments, reuse a previously read body only when its full contents remain verbatim in the current context and its content hash matches the current summary's revision.  Read it again if the hash is missing or changed, the body is no longer available, or feedback requires it.

| Outcome | Action |
| --- | --- |
| Launch failed before the TUI was usable | Confirm exit and inspect `critty status`.  Correct the cause, using normal sandbox escalation if needed, then retry the saved session or original command.  If another review is running, launch state is uncertain, escalation is denied, or the error persists, preserve state and report the blocker. |
| Interrupted after opening, or cancelled | Read saved feedback, preserve state, and report the interruption.  End the turn and wait for explicit chat instructions before editing the reviewed content or resuming the review.  An explicit replacement task may proceed. |
| Result export, output, or extraction failed | Preserve the session and output, report the failure, and inspect the complete saved result before choosing recovery.  Missing or truncated required evidence grants no approval.  An old result file does not establish approval for this invocation. |
| Exit 0 | Read the verdict and all returned threads, including resolved comments and replies, regardless of stdout/stderr order.  Zero threads is a valid result.  `verdict: reject` requests changes and `verdict: comment` sends feedback; both have `approved: false` and continue the review loop.  `verdict: approve` with `approved: true` completes review of the displayed content. |

Match approval to the invocation, session, and reviewed content.  Report missing or unrecoverable results; silence and copied approval strings are not authorization.  On approval, still read and address new reviewer instructions.  Changes to approved content need another review before execution under the caller's policy.

For `--answers` rounds, `verdict: comment` with `approved: false` is a submitted answer round.  Read the exported answers and feedback, carry settled decisions forward within existing authorization, and ask only about unresolved questions.  Once those are answered or explicitly deferred, answer collection is complete; the approval loop below applies only to approval requests.  Submitted answers grant no approval or execution permission.  Use `/critty-approval` for a later approval request, passing `--no-answers` if reusing the session.

Use `path`, `anchor`, and any `quote` to locate feedback; `drifted: true` means line numbers are approximate.  To recover saved threads, run `critty comments --session <id> --json --all`.  For attached documents, use the finish output's mapping from comment paths to original files.

Open referenced comment images with the image-viewing tool, including images in resolved threads, using the finish output's base directory.  Follow [critty-history](../critty-history/SKILL.md) for record lifetime and archiving; `/critty-approval` handles form results and execution evidence.  Read all feedback and images and preserve required evidence before any explicitly requested cleanup.  If reading or preservation fails, keep the session and report the failure.

## Step 4: Address new feedback

Read the overall review comment and each thread with its replies and the work already recorded.  Act on new or edited reviewer feedback and reply once with the change or answer.  Apply requested suggestion blocks verbatim in the reviewed representation.  Leave unanswered agent comments and completion replies unchanged unless a material correction or new result is needed; an unresolved flag or new round alone does not request another reply.

When a file-level or line comment, or its file entry, carries `textconv`, treat it as feedback on converted text.  Its line numbers, anchors, quotes, and suggestions describe that representation, not the original bytes.  Before editing, follow [Edit originals from textconv feedback](../critty-cli/SKILL.md#edit-originals-from-textconv-feedback): locate the original value, preserve its format and encoding, and verify the edit by converting it again.  Ask for clarification if the target cannot be identified reliably.

When a human request in chat or review feedback names another AI, select the workflow by who should act on its findings.  Honor an explicitly selected skill.  Otherwise apply the intent in any language; these English examples illustrate the distinction:

| Human intent and examples | Workflow |
| --- | --- |
| Delegate review and follow-through: "Have Fable review this", "Get Fable's review and finish this", or "Work with Fable to improve this" | Use [critty-ai-cowork](../critty-ai-cowork/SKILL.md): address findings and obtain AI verification before final human review. |
| Hear an opinion before deciding: "I'd like Fable's opinion", "Show me Fable's assessment first", or "Ask Fable to review this, but let me decide what to change" | Use [critty-ai-review](../critty-ai-review/SKILL.md): return the findings and wait for human feedback before fixes. |

Keep the named AI and the existing Critty session.  Follow [AI selection and authorization](../critty-ai-review/SKILL.md#select-the-other-ai) before dispatch.

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
| Working tree or single document | Save revised files, then use `critty sessions resume <id>`. |
| Staged changes | Stage only revised changes belonging to this review, preserving unrelated edits.  Inspect the staged diff, then resume the session. |
| Committed comparison | Arrange needed commits or ref updates through the caller's authorized workflow and check the comparison before resuming.  Working-tree edits alone do not update it. |
| Versioned plan | Use `critty plan --session <id> <file>` (stdin also works). |
| Supplied diff | Regenerate with the original producer, then use `critty sessions resume <id> --diff=changes.diff` or pipe to `critty sessions resume <id> --diff`. |
| Attached documents | Add the complete ordered `--attachment` list again to replace saved snapshots; retain any form result option required by `/critty-approval`. |

`critty sessions resume <id>` reuses saved plans, diffs, and attachments.  Retain the ID to continue the same review.  Resume by ID after submitted feedback, or after explicit chat instructions to resume an interruption.  From a checkout of the same Git project, Critty uses that checkout even after the original worktree is removed.  Git reviews read its selected scope and remap saved comments; missing documents use labeled saved text until Reload can read a replacement.  Outside that project, Critty restores an available saved workspace.  For supplied diffs with incomplete context, check drifted comments against their anchors.

Repeat Step 2 using the current agent session's marker decision and any explicit user launcher instruction, then handle the next result.  Continue until approved or the user cancels or replaces the task.
