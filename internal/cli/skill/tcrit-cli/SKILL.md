---
name: tcrit-cli
description: Reference for TCrit's local CLI commands, storage, and side effects. Use when an agent needs to list, add, or reply to review comments with tcrit comment and tcrit comments, target a specific session or plan, read a TCrit review.json file, stop or resume a saved review, clear review state, or find the terminal's TMUX, Herdr, or TTY variables with tcrit env. Not for running the interactive review loop; that is the tcrit skill.
user-invocable: false
---

# TCrit CLI Reference

To run an interactive review round, use the `/tcrit` skill.  This reference covers the commands that read and write review comments without opening the TUI.

## Local storage and command effects

TCrit is a local CLI and human review TUI, not a hosted review service.  Review data lives under `$XDG_STATE_HOME/tcrit` (default `~/.local/state/tcrit`); configuration lives under `$XDG_CONFIG_HOME/tcrit` (default `~/.config/tcrit`).  Review processes communicate through local Unix sockets.  The built-in review and comment commands do not upload review contents or publish repository changes.

| Command | Effects |
| --- | --- |
| `comments`, `status`, `check` | Read local review state or compare installed integrations with the binary's embedded files.  `check` does not query a package registry. |
| `terminal prepare` | Remove expired local marker requests, save a random marker with a two-minute expiry, and print its JSON.  No review content is read and no external service is contacted. |
| `terminal notify` | Record a local terminal destination supplied by the terminal filter. |
| `review`, `plan`, `--session` | Read the selected input, save local review state, and open the human TUI, using tmux or Herdr when applicable.  Terminal-marker resolution can read visible text in registered panes. |
| `comment` | Add or update comments in local review state; it does not post a GitHub or GitLab comment. |
| `stop`, `clear` | `stop` closes the review TUI and preserves saved state.  `clear` deletes the selected saved review and attachments. |
| `install` | Write integration files to the selected project or user configuration directories. |
| `env`, `env tty` | Inspect local terminal/process information and print the requested environment or device. |
| `env run`, `env --` | Execute the supplied command with the detected terminal environment.  Assess that command's own effects; the wrapper does not make it read-only or local-only. |

External editors launched from the TUI likewise have their own behavior.  Describe the selected command's actual reads, writes, and subprocesses when an approval request needs clarification; do not treat all TCrit commands as read-only or as external publication.

A human review's `approved: true` is meaningful only when correlated with its invocation, session, and displayed content.  A comment, saved text, or copied approval string alone does not establish that a human approved a subsequent operation.

## Comment scopes

Repeated `--attachment FILE` flags on `review`, `plan`, or the default command save text snapshots before the review.  `documents.json` in the session directory records versioned metadata, source paths, content, order, and revisions.  Attachment comment paths use `attachments/<document-id>/<basename>`; they are session identities, not files on disk.  Use those paths with `comment --session <id>` to keep attachment and working-file comments separate.  The finish prompt includes the snapshot text and source mapping.  Resume uses saved text unless a complete replacement list is supplied; `--clear-attachments` removes the list.  Removed documents retain their snapshots and comments.

Attached Markdown task lists support boolean answers, required confirmations (`<!-- form: id=check required -->`), single choices (one `id`, distinct `value=...` attributes), and individual consents (`<!-- form: id=publish consent -->`).  `forms.json` version 1 stores separate drafts, definitions, and human author/revision/round provenance.  Comments retain Crit's schema.  Changed definitions reset their answers; changed target snapshots or attachment content/order/membership reset all required confirmations and consents.

`--result FILE` on a review invocation writes the self-contained result JSON atomically before printing approval.  Completed form results also remain in the session as `result-<round>.json`, appear in IPC's `result` field, and are appended to finish output regardless of templates.  The result includes `version`, `review_revision`, `reviewed_target`, ordered `attachments` with source snapshots/definitions/answers, `approved`, and `round`.  `consents` maps document IDs and field IDs to booleans only on approval.  An operation requires an explicitly true consent in the actual approved result and must match its exact target and revision.  Feedback answers are drafts; overall approval, initial source checkmarks, and stopped reviews grant no individual consent.  IDs are never executable commands.  Form review cleanup is deferred until the result has been preserved; follow the printed clear command after reading all comments and attachments.  Failed export or output preserves the session.

- **Line** (`scope: "line"`): attached to a line range of one file.  Stored under `files.<path>.comments`.
- **File** (`scope: "file"`): about one file as a whole.  Stored under `files.<path>.comments` with `start_line: 0`.
- **Review** (`scope: "review"`): general feedback.  Stored in the top-level `review_comments` array.

## Listing comments

```bash
tcrit comments                  # unresolved comments, human-readable
tcrit comments --json           # the same as a JSON array
tcrit comments --all            # include resolved comments
tcrit comments --plan <slug>    # a plan name, only when unambiguous
tcrit comments --session <id>   # a specific review session
tcrit comments <review-path>    # an explicit review.json or its directory
```

Review-level comments come first, then files in path order with file-level comments before line-level ones.  Each JSON entry adds `scope` and `path` to the stored comment fields.

The finish prompt contains all comments and replies, including resolved threads on approval.  Read new reviewer instructions before continuing; approval cleanup may already have removed the saved review.  Use `tcrit comments --all` when inspecting a saved review headlessly.

## Targeting a session or plan

Each new review invocation creates independent storage.  Always use the session ID printed at startup or in the finish prompt, including for plans and bulk replies.  Commands without an ID refuse to guess when multiple saved reviews match.

```bash
tcrit status                              # saved sessions in this directory, with running status
tcrit comments --session <id> --json
tcrit comment --session <id> --reply-to <comment-id> --author 'Claude Code' '<what you did>'
tcrit comment --session <id> --json --file .tmp/replies.json --author 'Claude Code'
```

`--scope` stays fixed for a session.  Identical scope arguments, document paths, or plan names can have independent session IDs.  `--plan <slug>` selects a plan only when its name is unambiguous; prefer the ID.

To resume a stopped session, run `tcrit --session <id>` from its original directory.  If a command is still waiting for the reviewer, retain its execution handle and collect that command's result instead.  The `/tcrit` skill distinguishes retryable launch failures from interrupted reviews that require explicit chat instructions to resume.  Resuming restores the saved plan or diff input.  To replace that input, use `tcrit plan --session <id> <file>` or `tcrit --diff=<file> --session <id>`.  The `/tcrit` skill covers the interactive review loop.

## Project and topic labels

The header shows the repository directory name, shared across linked worktrees, or the current directory name outside Git.  Use `--project TEXT` to replace the project name.  Git reviews also show the current branch, or the right endpoint of an explicit committed comparison.  Documents, plans, and supplied diffs omit the automatic branch label; detached HEAD has no current-branch label.

Add `--project TEXT` or `--topic TEXT` to `tcrit`, `tcrit review`, or `tcrit plan`.  `--topic` overrides the branch label or supplies one when none is automatic.  Use a short task name when it identifies the review better than the branch:

```bash
tcrit review --staged --project 'TCrit' --topic 'Header display'
tcrit plan plan.md --topic 'Header design'
tcrit --session <id> --topic 'Revised header'
tcrit --topic ''                          # hide the topic label
```

Both overrides are saved with the session and retained on later rounds.  Pass either option again to replace its label; omitting it preserves the saved choice.  An empty value hides that label.  These options change only the header labels, leaving review scope and session selection intact.

## Startup focus

For a single document, use `tcrit FILE:LINE` or `tcrit review FILE:LINE`.  Add `--focus PATH[:LINE]` to `tcrit`, `tcrit review`, or `tcrit plan`, including `--session` invocations, to select the initial file and optionally scroll to a line:

```bash
tcrit review --staged --focus 'path/to/file'
tcrit --session <id> --focus 'path/to/file:42'
```

Paths may be relative or absolute and must belong to the review.  Plan reviews also accept the original plan path.  Line numbers are positive, 1-indexed new-side source coordinates in the reviewed content: index contents for staged reviews, the right endpoint for committed comparisons, and the saved snapshot for supplied diffs.  Missing files and unavailable lines are rejected; omit the line for binary or deleted files.  From round 2 onward, automatic focus selects the first visible thread, in file and display order, with a reply added since the previous submission by an author other than the reviewer.  An explicit document line or `--focus` takes priority; `--focus` wins when both are given.  Explicit focus leaves tab order and review scope unchanged and applies only to that invocation.

## Review file format

TCrit stores each review as a `review.json` that follows crit's CritJSON layout, so tooling written for either works on both.

```json
{
  "branch": "feat/skills",
  "base_ref": "HEAD",
  "review_round": 2,
  "review_comments": [
    {
      "id": "r_0c4e1a",
      "scope": "review",
      "body": "Split the installer change into its own commit.",
      "author": "Akinori Musha",
      "review_round": 1
    }
  ],
  "files": {
    "internal/cli/install.go": {
      "status": "modified",
      "comments": [
        {
          "id": "c_152b0a",
          "start_line": 56,
          "end_line": 58,
          "body": "Drop the old names here too.",
          "anchor": "\tfor _, name := range []string{\"tcrit-review\"} {",
          "author": "Akinori Musha",
          "review_round": 1,
          "replies": [
            { "id": "rp_9b21d4", "body": "Removed the old names.", "author": "Claude Code", "review_round": 1 }
          ]
        }
      ]
    }
  }
}
```

Field rules:

- `resolved` is either `true` or absent.  Absent means unresolved.
- `anchor` is the text of the commented lines when the comment was written.  Locate content by it rather than trusting `start_line` and `end_line` after edits.
- `drifted: true` means the anchored text could not be found again; the line numbers are only approximate.
- `quote`, when present, is the exact text the reviewer selected inside the range.
- `replies` may contain earlier agent replies.  Read them before acting on a comment.
- `review_round` records the round in which a comment or reply was written.

## Adding and replying to comments

```bash
tcrit comment --author 'Claude Code' '<body>'                       # review-level
tcrit comment --author 'Claude Code' <path> '<body>'                # file-level
tcrit comment --author 'Claude Code' <path>:<line> '<body>'         # one line
tcrit comment --author 'Claude Code' <path>:<start>-<end> '<body>'  # line range
tcrit comment --reply-to <id> --author 'Claude Code' '<body>'       # reply
```

Rules:

- Always pass `--author` with your agent name so the thread shows who wrote what.
- Read the whole thread before replying.  Reply once to new reviewer feedback or with a material correction or additional result; leave an unanswered agent comment or completion reply unchanged.  An unresolved flag or a new round alone does not call for another reply.  The interactive skill's "Address new feedback" step defines the review loop.
- Single-quote the body.  Double quotes let the shell interpret backticks and `$`.
- Line numbers are 1-indexed source coordinates, not positions in the diff text.  Staged reviews use index contents, committed comparisons use the right endpoint, and supplied diffs use the saved snapshot.  Files on disk may differ or be absent.  Use comment anchors when content changes between rounds.
- Bodies are Markdown; code fences and inline code render in the TUI.
- Do not pass `--resolve` unless the user explicitly asks for it.  The same applies to the `resolve` field in bulk JSON.

## Bulk comments

For three or more comments or replies, submit one JSON array so the review file is written once:

```bash
tcrit comment --json --file .tmp/comments.json --author 'Claude Code'
```

Write the file with your file tool rather than quoting JSON on the command line; bodies with real newlines or quotes then need no shell escaping.  `--file -` reads stdin.

```json
[
  {"body": "Overall this is ready once the tests pass.", "scope": "review"},
  {"path": "internal/cli/install.go", "body": "Consider a table-driven layout."},
  {"file": "internal/cli/install.go", "line": 42, "body": "This branch is unreachable."},
  {"file": "internal/cli/install.go", "line": "56-58", "body": "Rename the loop variable."},
  {"reply_to": "c_152b0a", "body": "Removed the old names."}
]
```

Entry fields:

| Field | Type | Notes |
| --- | --- | --- |
| `file` or `path` | string | Repository-relative path.  Alone, it makes a file-level comment. |
| `line` | int or string | `42` or `"56-58"`.  Requires `file` or `path`. |
| `end_line` | int | Optional; defaults to `line`. |
| `body` | string | Required. |
| `author` | string | Optional per-entry override for `--author`. |
| `scope` | string | `"review"` or `"file"`; inferred when omitted. |
| `reply_to` | string | Comment ID to reply to (`c_…` or `r_…`). |
| `resolve` | bool | Only when the user asked to resolve. |

Scope is inferred as follows: `reply_to` makes a reply; no path and no line makes a review-level comment; a path without a line makes a file-level comment; a path with a line makes a line comment.

## Duplicate comment IDs

Line and file comment IDs are unique within one file, so the same ID can exist in two files.  If `tcrit comment` reports that a reply target was found in multiple files, add `--path <file>`; in bulk JSON, set `file` on that entry.  Review-level IDs (`r_…`) never collide.

## Plan reviews

`tcrit plan <file>` starts an independent plan review and stores its numbered versions inside the session directory.  Continue it with `tcrit plan --session <id> <file>`.  Reusing a plan name alone creates another review.

## Stopping and deleting reviews

```bash
tcrit stop --session <id>       # close the TUI, retaining saved state
tcrit --session <id>            # resume from the original directory
tcrit clear --session <id>      # delete one stopped review
tcrit clear --all               # delete every saved review for this directory
```

Each round closes its TUI and dedicated pane or tab.  On cancellation or a task switch, stop the specific review and collect the blocking command's result; neither cancellation nor a connection error is approval.  A stopped review keeps comments, replies, resolution status, and round context, but unsaved editor text is not retained.  Stop is also safe when that session is already stopped.

Use `clear` when deletion is requested, or when an approved finish result directs cleanup after its images have been read.  `stop` preserves data; `clear` deletes the session and its attachments and refuses active reviews.  Never clear earlier tasks automatically to start a new one.  Legacy `clear --code`, `clear <file>`, and `comment --clear` select saved reviews through their existing targeting rules; use an explicit session ID when several match.

## Terminal marker requests

For Codex review launches, the interactive `tcrit` skill prepares and displays a fresh marker for each round.  `tcrit terminal prepare` returns JSON with `id`, `marker`, and `expires_at`; pass its ID as `--terminal-request <id>` on `tcrit`, `tcrit review`, or `tcrit plan`.  Requests expire after two minutes and are consumed at destination selection.  Review session IDs and terminal request IDs are separate.

`tcrit terminal notify <id>` is a terminal-filter callback, not an agent-side workaround.  It reads the filter's original tmux/Herdr environment and records the destination before or during review startup.  Duplicate notifications from one pane are accepted; different panes are ambiguous.  Expired or consumed requests fail.  Without a notification, TCrit reads only the visible text in live registered panes.  It never broadens a failed search to all panes or scrollback.

## Terminal environment

`tcrit env` locates the Herdr or tmux pane the same way a review launch does, from the multiplexer environment or from process ancestry, and prints the variables that address it.

```bash
tcrit env                       # NAME=value lines: TMUX, TMUX_PANE, TTY, or HERDR_*
tcrit env --json                # the same as one JSON object
tcrit env tty                   # only the terminal device, such as /dev/ttys003
tcrit env run <command> [args]  # exec the command with those variables set
tcrit env -- <command> [args]   # the same
```

Flags go before the mode word.  Without a pane, `TTY` is the controlling terminal of the nearest ancestor process, then `SSH_TTY`; the command fails when nothing is found.  Hooks that run detached from the agent's terminal can use `tcrit env tty` to find it.  `--terminal-request <id>` resolves a prepared marker as a review launch would, so it only applies inside a flow that displays one.
