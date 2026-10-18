---
name: critty-cli
description: Reference for Critty commands, review data, and local effects. Use for scripted comments, session targeting, review inputs, attachments, form results, or terminal setup. Use the critty skill for the interactive review loop.
user-invocable: false
---

# Critty CLI Reference

Use `/critty` to run the interactive review loop and `/critty-approval` to prepare approval requests and act on their results.  This reference owns command syntax, input and output formats, and local effects.  Consult the sections needed for the current operation.

## Local storage and command effects

Critty is a local CLI and human review TUI, not a hosted review service.  Review data lives under `$XDG_STATE_HOME/critty` (default `~/.local/state/critty`).  Review processes communicate through local Unix sockets.  The built-in review and comment commands do not upload review contents or publish repository changes.

For editing settings or locating custom prompts, read [configuration](references/configuration.md).  It covers TOML paths, precedence, and prompt overrides.

| Command | Effects |
| --- | --- |
| `comments`, `status` | Read local review state. |
| `doctor` (`check` alias) | Compare installed integrations with embedded files and inspect local Python/MCP prerequisites and image forwarding settings, plus desktop launcher availability, Codex wrapper selection, and tfil detection for the current platform.  `--terminal-request ID` consumes a prepared request to test terminal binding without opening a review.  Desktop checks do not open windows or test automation permission.  Runs bounded Python imports and an image capability probe on its foreground controlling TTY.  Temporarily uses raw terminal input, then restores it; sends a transparent virtual placement, deletes it, and restores the cursor.  Does not start a reviewer, contact a model or package registry, or change stored settings. |
| `terminal prepare` | Remove expired local marker requests, save a random marker with a two-minute expiry, and print its JSON.  No review content is read and no external service is contacted. |
| `terminal notify` | Record a local terminal destination supplied by the terminal filter. |
| `review`, `plan`, `--session` | Read the selected input, save local review state, and open the human TUI, using tmux or Herdr when applicable.  Terminal-marker resolution can read visible text in registered panes. |
| `comment` | Add or update comments in local review state; it does not post a GitHub or GitLab comment. |
| `stop`, `clear` | `stop` closes the review TUI and preserves saved state.  `clear` deletes the selected saved review and attachments. |
| `install` | Write integration files to the selected project or user configuration directories. |
| `env`, `env tty` | Inspect local terminal/process information and print the requested environment or device. |
| `env run`, `env --` | Execute the supplied command with the detected terminal environment.  Assess that command's own effects; the wrapper does not make it read-only or local-only. |

External editors launched from the TUI likewise have their own behavior.  Describe the selected command's actual reads, writes, and subprocesses when an approval request needs clarification; do not treat all Critty commands as read-only or as external publication.

`install --user <target>` selects user scope from any directory; running from the home directory also selects user scope.  Otherwise, installation is relative to the current project root.

| Target | Project | User |
| --- | --- | --- |
| `codex` | `.codex/skills` | `~/.codex/skills` |
| `opencode` | `.opencode` | OpenCode's XDG config directory |
| `antigravity` (`agy`) | `.agents/skills` | `~/.gemini/antigravity-cli/skills` |
| `agents` | `.agents/skills` | `~/.agents/skills` |

`agents` provides shared, host-neutral skill references and comment attribution.  Antigravity skills use the ordinary `critty` names.  Prefer a shared installation or selected host variants, because hosts may discover duplicate names across directories.  Existing installations in old locations are left untouched.  `install all` includes Antigravity; legacy Gemini CLI skills remain available through `install gemini`.

## Review inputs

| Input | Command and source |
| --- | --- |
| Current Git changes | `critty` or `critty --scope=all`: HEAD versus working tree, including untracked files. |
| Staged changes | `critty --staged` or `critty --scope=staged`: HEAD versus index. |
| Unstaged changes | `critty --unstaged` or `critty --scope=unstaged`: index versus working tree, including untracked files. |
| Committed comparison | `critty --scope=A..B` compares snapshots; `A...B` compares the merge base with B.  An omitted endpoint means HEAD. |
| Document | `critty <file>` reads one current file. |
| Versioned plan | `critty plan <file>` saves numbered versions.  `critty plan --session <id> <file>` supplies a revision; reusing a name alone starts another review. |
| Supplied diff | `critty --diff=changes.diff` or `critty --diff changes.diff`; bare `--diff` or `--diff=-` reads stdin.  Works outside Git. |

`critty review` is the explicit form of the default command.  Conflicting scope flags are rejected; `--scope` cannot accompany a document or supplied diff.  `--diff` cannot accompany `--scope`, `--code`, `--staged`, or `--unstaged`.  Relative and absolute input paths are accepted.

Committed comparisons display the right endpoint, which may differ from disk.  A clean working tree does not fall back to commits.  Supplied diffs use saved snapshots; retain their producer command for later updates.  Scope selection stays fixed within a session.  Follow `/critty` to refresh the selected input between rounds.

## Attached documents

Repeated `--attachment FILE` flags on `review`, `plan`, or the default command save text snapshots before the review.  `documents.json` in the session directory records versioned metadata, source paths, content, order, and revisions.  Attachment comment paths use `attachments/<document-id>/<basename>`; they are session identities, not files on disk.  Use those paths with `comment --session <id>` to keep attachment and working-file comments separate.  The finish prompt includes the snapshot text and source mapping.  Resume uses saved text unless a complete replacement list is supplied; `--clear-attachments` removes the list.  Removed documents retain their snapshots and comments.

`critty --null --attachment FILE` reviews only attachments without requiring Git.  Empty diff input, including `--diff=/dev/null`, also supports attachment-only reviews.  At least one active attachment is required.  `--null` cannot be combined with a file argument, `--diff`, `--scope`, `--code`, `--staged`, or `--unstaged`.  Resume with `critty --session <id>`, optionally replacing the complete attachment list.  Removing all attachments from an empty review is rejected.

## Forms and exported results

Task lists in attached `.md` or `.markdown` files are interactive.  `<!-- form: id=check required -->` makes a confirmation, distinct `value` attributes on the same ID make a choice, and `<!-- form: id=publish consent -->` gates one operation.  `/critty-approval` defines when to use them and how to interpret permission.

`forms.json` version 1 stores definitions, draft answers, and human provenance separately from source documents and comments.  Changed field definitions reset their answers.  Changed target snapshots or attachment contents, order, or membership reset all required confirmations and consents; ordinary explicit-ID answers survive unchanged definitions.

`--result FILE` requires an interactive attachment form.  It writes the result atomically before printing approval; omit it for ordinary document approval.  The parent directory must exist, the file must be outside the session directory, and it must not overwrite reviewed input.  The same self-contained result is saved as `result-<round>.json`, returned in IPC's `result`, and appended to finish output regardless of templates.

| Result field | Meaning |
| --- | --- |
| `version`, `session`, `round` | Schema version, session ID, and completed round. |
| `review_revision`, `reviewed_target` | Revision identifier and the exact reviewed input. |
| `attachments` | Ordered documents with source paths, snapshots, field `definitions`, and `answers`. |
| `approved` | Human review outcome, including required-confirmation checks. |
| `consents` | On approval, document ID → field ID → boolean for individual permissions. |

Feedback answers are drafts; overall approval and initial checkmarks supply no individual consent.  Form review cleanup is deferred.  Preserve the result before running the printed cleanup command, and follow `/critty-approval` to match decisions to operations.

## Task history

Normal review start/resume reports history warnings on stderr while preserving stdout and verdicts.  `critty doctor` includes fresh history checks and suggested remedies, independent of the passive warning throttle.  On `critty: warning: history:`, follow [critty-history](../critty-history/SKILL.md#handle-review-flow-warnings) for targeted inspection, policy-driven cleanup, or a batched user decision.

Use [critty-history](../critty-history/SKILL.md) before the first review and after the whole work task, for task bookkeeping; Critty applies the configured policy.  Inspect capacity or old records only after a command warning/result or explicit maintenance request.  Cleanup and exports use Critty commands, without storage-tree traversal or custom maintenance scripts.  `critty history --help` lists commands; `--task <id>` on `critty`, `review`, and `plan` associates a review without prompting at each round.

## Review command output

On submission, stdout contains the finish prompt, threads, and attached document snapshots; stderr contains `verdict: approve|reject|comment`, `approved: true|false`, and diagnostics.  Capture both streams and the exit status.  Only `approve` sets `approved: true`; `reject` requests changes and `comment` sends feedback.  Exit 0 can mean any of these submissions, so inspect the verdict and all threads.  A nonzero exit does not grant approval.  Export or output failures retain session data; a pre-existing result file may be from an earlier round.  Correlate any recovered result with its invocation before using it.

`--summary FILE` on `critty`, `review`, or `plan` saves a compact JSON index before printing approval.  It works with or without forms and custom finish prompts.  Version 1 contains `session`, `round`, `cwd`, `verdict`, `approved`, optional `review_comment`, all `comments` with resolved threads and replies, ordered `attachments` with `path`, `source`, `revision` (content hash), and optional `removed`, `image_base` for comment images, and optional `next_command`.  Empty comment and attachment lists are arrays.  Attachment bodies remain in complete stdout rather than the summary.

Use a fresh path for each invocation and verify exit 0, session, round, and the reviewed inputs before acting.  The parent directory must exist; the file must be outside the session directory and must not overwrite reviewed input or the `--result` file.  Summary export failure returns nonzero without printing approval or cleaning up the review.  Keep full finish output for additional instructions and evidence; a summary does not contain form answers or individual consents.  Use `--result` and `/critty-approval` for those decisions.

Saved reviews are retained after approval by default.  The Crit-compatible `cleanup_on_approve` setting defaults to `false` in Critty; explicitly setting it to `true` enables approval cleanup.  Task-associated reviews defer cleanup to their task even when this setting is enabled.  Ungrouped ordinary reviews with cleanup enabled are deleted after successful finish output, while form results and comment images defer cleanup as described in their finish output.  Preserve the finish output even when retaining the session; saved state does not replace the original completion evidence.  Use `/critty` for launch, wait, and recovery decisions.

## Environment diagnostics

Run `critty doctor` before troubleshooting integration or image setup.  Missing files in a partially installed integration and stale or unreadable files are errors.  Absent integrations and optional reviewers are informational.  Desktop launcher availability is checked independently of launch priorities and SSH restrictions: all three macOS launchers, `xdg-terminal-exec` on Linux, and additionally Windows Terminal under WSL.  Missing or incompatible desktop launchers, Python/MCP problems, and image setup problems are warnings; they do not block unrelated review modes.  Errors return exit status 1; warnings alone return 0.  The text report groups checks by area under a version header, marks each line with its status (`ok`, `error`, `warn`, or `info`), indents advice beneath the check, and ends with a summary count; color-capable terminals color the statuses, and `NO_COLOR`, redirected output, and JSON retain plain text.

Doctor distinguishes PATH selection from current-session behavior.  It identifies the bundled Codex wrapper from Go build metadata, tfil-generated wrappers from their exact `#!/bin/sh` and `# tfil-wrapper` header, and tfil in process ancestry; an installed executable alone does not establish an active wrapper.  Shims and custom wrappers remain unverified, not broken.  Without a marker request, terminal binding is reported as untested.  For an end-to-end check, run `critty terminal prepare`, display its marker verbatim in agent output, then run `critty doctor --terminal-request <id>` within two minutes.  This resolves and consumes the request without opening a review.  It uses a terminal filter notification or the visible screen of a live wrapper registration.  For daemon-backed agents, a resolved marker also supplies the client process ancestry for tfil detection.  Add `--terminal-notify-only` to require a terminal-filter callback instead of reading registered pane screens.  This verifies the notification path even inside tmux or Herdr.  With `tfil --critty-notify`, a missing callback reports a warning rather than succeeding through a registered pane.  Without this option, a successful binding test can also use registered multiplexer panes.

Use `--python /absolute/path/to/venv/bin/python` to inspect the interpreter selected for restricted AI review; the default is `python3` on PATH.  Codex requires Python 3.11+ and the bundled MCP SDK pin and server API; Claude requires Python 3.10+.  MCP advice includes a quoted install command using an absolute path to a matching installed requirements file, or the bundled package pin when no current copy exists.  CLI availability does not certify authentication, supported flags, or the effective restricted tool surface.  Validate those at reviewer launch.

`--json` returns an object with `version` and `checks`.  Each check has `check`, `status` (`ok`, `info`, `warn`, or `error`), `message`, and optional `advice`.  A report is still written when errors cause a nonzero exit.

Image diagnostics read tmux pane settings, including inherited options, when its environment identifies the pane; otherwise they label a global setting or report unknown context.  Herdr diagnostics parse local config with the current `terminal.kitty_graphics` key taking precedence over legacy `experimental.kitty_graphics`.  Configuration checks do not verify a running server or remote client.  On macOS and Linux, `images/terminal` probes the current foreground controlling TTY using the same virtual-placement and cursor-position test as the TUI, including tmux extended-key replies.  A missing, inaccessible, busy, or background TTY reports `info` with a `skipped:` message and adds no error.  Negative or incomplete replies are warnings; an unanswered probe retries up to three times.  Probe traffic uses the TTY directly, so redirected text and JSON contain only the diagnostic report.  Run from the actual review terminal; the TUI Images control can retry support checks later.

## Listing comments

```bash
critty comments                  # unresolved comments, human-readable
critty comments --json           # the same as a JSON array
critty comments --all            # include resolved comments
critty comments --plan <slug>    # a plan name, only when unambiguous
critty comments --session <id>   # a specific review session
critty comments <review-path>    # an explicit review.json or its directory
```

Review-level comments come first, then files in path order with file-level comments before line-level ones.  Each JSON entry adds `scope` and `path` to the stored comment fields.

The finish prompt contains all comments and replies, including resolved threads on approval.  The current overall comment and verdict are appended even with a custom prompt.  Its body uses Crit-compatible `review_comments` with `scope: "review"`; the explicit verdict is a Critty extension stored in `critty_state.verdict`.  IPC and form results also expose `verdict` and optional `review_comment`.  Unsubmitted drafts stay in `critty_state.review_comment_draft` and are not submitted comments.  Read new reviewer instructions before continuing; explicitly enabled approval cleanup may already have removed the saved review.  Use `critty comments --all` when inspecting a saved review headlessly.

## Targeting a session or plan

Each new review invocation creates independent storage.  Always use the session ID printed at startup or in the finish prompt, including for plans and bulk replies.  Commands without an ID refuse to guess when multiple saved reviews match.

```bash
critty status                              # saved sessions in this directory, with running status
critty comments --session <id> --json
critty comment --session <id> --reply-to <comment-id> --author 'Claude Code' '<what you did>'
critty comment --session <id> --json --file replies.json --author 'Claude Code'
```

`--scope` stays fixed for a session.  Identical scope arguments, document paths, or plan names can have independent session IDs.  `--plan <slug>` selects a plan only when its name is unambiguous; prefer the ID.

To resume a stopped session, run `critty --session <id>` from its original directory.  If a command is still waiting for the reviewer, retain its execution handle and collect that command's result instead.  The `/critty` skill distinguishes retryable launch failures from interrupted reviews that require explicit chat instructions to resume.  Resuming restores the saved plan or diff input.  To replace that input, use `critty plan --session <id> <file>` or `critty --diff=<file> --session <id>`.  The `/critty` skill covers the interactive review loop.

## Display options

### Project and topic labels

`--project TEXT` overrides the repository or directory name.  `--topic TEXT` overrides the Git branch or comparison endpoint, or adds a topic to a document, plan, or supplied diff.  Detached HEAD has no automatic branch label.  Both options work on `critty`, `review`, and `plan`, including resumed sessions.

```bash
critty review --staged --project 'Critty' --topic 'Header display'
critty --session <id> --topic 'Revised header'
critty --topic ''                          # hide the topic label
```

Overrides persist in the session until replaced; an empty value hides the label.  They do not change scope or session selection.

### Startup focus

`critty FILE:LINE` starts a document at a line; `--focus PATH[:LINE]` selects an initial file in any review, including resumed sessions.  Paths may be relative or absolute and must belong to the review; plans also accept their original path.  Lines are positive, 1-indexed new-side source coordinates.  Missing files and unavailable lines are rejected; omit the line for binary or deleted files.

Explicit focus applies only to that invocation.  `--focus` wins over a document's line suffix; otherwise later rounds focus the first visible thread, in file and display order, with a new reply from another author.  Scope and tab order remain unchanged.

### Images

Supported terminals preview PNG, JPEG, GIF, and WebP, with Before and After views for modified or renamed images.  Detection is automatic; tmux needs `allow-passthrough on`.  GIF uses its first frame.  Animated WebP, images over 5 MiB or 16 megapixels, and unsupported terminals retain placeholders.  Images accept file comments without adding source lines or changing anchors.

Capability detection starts when a local image preview is needed and retries unanswered queries.  It supports tmux's extended-key input; tmux queries require the review pane to be selected in the server's sole attached client, so replies reach the review.  If support remains unconfirmed, selecting a tab containing a local image checks again.  Typing and ordinary navigation do not trigger detection.  Further checks can be requested manually.

Git previews read the selected snapshot, including deleted images.  Markdown image paths resolve from the document or attachment's original location; Git references must stay inside the repository.  Supplied diffs remain text-only.  Remote URLs are not fetched.

Comment images read current local files: ordinary relative paths use the review's working directory, `attachments/` paths use the session directory, and absolute paths may be outside the repository.  Follow the finish output's base directory when reading them.  `/critty` covers reading images before approval cleanup.

## Review file format

Critty stores comments in Crit-compatible `review.json`:

- **Line** (`scope: "line"`): a range in one file, stored in `files.<path>.comments`.
- **File** (`scope: "file"`): a whole file, in the same array with `start_line: 0`.
- **Review** (`scope: "review"`): the review as a whole, in `review_comments`.

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
          "anchor": "\tfor _, name := range []string{\"critty-review\"} {",
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
critty comment --session <id> --author 'Claude Code' '<body>'                     # review-level
critty comment --session <id> --author 'Claude Code' <path> '<body>'              # file-level
critty comment --session <id> --author 'Claude Code' <path>:<line> '<body>'       # one line
critty comment --session <id> --author 'Claude Code' <path>:<start>-<end> '<body>' # line range
critty comment --session <id> --reply-to <comment-id> --author 'Claude Code' '<body>'
```

Rules:

- Always pass `--author` with your agent name so the thread shows who wrote what.
- Read the whole thread before replying; `/critty` defines how to identify new feedback and avoid repeated replies.
- Use a single-quoted argument for a short body without apostrophes.  For bodies containing apostrophes, multiple lines, or complex quoting, use a JSON file even for one comment.  Double quotes let the shell interpret backticks and `$`.
- Line numbers are 1-indexed source coordinates, not positions in the diff text.  Staged reviews use index contents, committed comparisons use the right endpoint, and supplied diffs use the saved snapshot.  Files on disk may differ or be absent.  Use comment anchors when content changes between rounds.
- Bodies are Markdown; code fences and inline code render in the TUI.
- Do not pass `--resolve` unless the user explicitly asks for it.  The same applies to the `resolve` field in bulk JSON.

## Bulk comments

Use a JSON array for three or more comments or replies, or for any body that is awkward to quote safely:

```bash
critty comment --session <id> --json --file comments.json --author 'Claude Code'
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

Line and file comment IDs are unique within one file, so the same ID can exist in two files.  If `critty comment` reports that a reply target was found in multiple files, add `--path <file>`; in bulk JSON, set `file` on that entry.  Review-level IDs (`r_…`) never collide.

## Stopping and deleting reviews

```bash
critty stop --session <id>       # close the TUI, retaining saved state
critty --session <id>            # resume from the original directory
critty clear --session <id>      # delete one stopped review
critty clear --all               # delete every saved review for this directory
```

Each round closes its TUI and dedicated pane or tab.  On cancellation or a task switch, stop the specific review and collect the blocking command's result; neither cancellation nor a connection error is approval.  A stopped review keeps comments, replies, resolution status, and round context, but unsaved editor text is not retained.  Stop is also safe when that session is already stopped.

Use `clear` when deletion is requested, or when explicitly enabled approval cleanup directs it after its images have been read.  Preserve saved reviews for retrospectives, archiving, and later continuation by default; approval or task completion alone does not authorize clearing them.  `stop` preserves data; `clear` deletes the session and its attachments and refuses active reviews.  Use the history workflow for maintenance under its global policy or selected IDs; `clear --all` is not a substitute for that decision.  Legacy `clear --code`, `clear <file>`, and `comment --clear` select saved reviews through their existing targeting rules; use an explicit session ID when several match.

## Terminal marker requests

For Codex review launches, the interactive `critty` skill prepares and displays a fresh marker for each round.  `critty terminal prepare` returns JSON with `id`, `marker`, and `expires_at`; pass its ID as `--terminal-request <id>` on `critty`, `critty review`, or `critty plan`.  Requests expire after two minutes and are consumed at destination selection.  Review session IDs and terminal request IDs are separate.

`critty terminal notify <id>` is a terminal-filter callback, not an agent-side workaround.  It reads the filter's original terminal environment and records the destination and client hints, including without a multiplexer before or during review startup.  Duplicate notifications from one pane are accepted; different panes are ambiguous.  Expired or consumed requests fail.  Without a notification, Critty reads only the visible text in live registered panes.  It never broadens a failed search to all panes or scrollback.

## Review launchers

`--launcher` selects `mux`, `tmux`, `tmux-floating`, `tmux-split`, `herdr`, `desktop-terminal`, `xdg-terminal`, `ghostty-window`, `iterm-window`, `terminal-app-window`, `windows-terminal-wsl`, or `custom:EXECUTABLE`.  It applies to code, document, plan, and resumed reviews; pass it again when resuming.  `mux` selects the nearest detected terminal multiplexer (currently tmux or Herdr), retaining current-terminal operation when neither exists.  Repeat `--launcher` to try launchers in the given order.  The defaults are `mux`, then `desktop-terminal`.  Missing defaults are appended in that order; explicit positions are preserved.  Availability failures stay silent when a later launcher succeeds.  If all launchers are unavailable, the error reports their failures, including failed multiplexer detection.  Opening errors stop the review rather than starting another launcher.

Set `launchers` to an ordered TOML array in the global or project configuration, for example `launchers = ["mux", "custom:my-terminal"]`.  Configured entries follow all CLI `--launcher` entries, before missing defaults are added.  With no CLI entries, the configured list is tried first.  A project array replaces the global array; `[]` clears the global list.  Explicit default placement across the combined list is preserved.

`tmux` tries `tmux-floating` first, then `tmux-split`.  Floating panes require the tmux 3.8+ `new-pane` controls; Critty checks the server's command capabilities.  They are nonmodal and centred against the whole window at 90% width and 85% height.  `tmux-split` explicitly selects the traditional split.  An unavailable `tmux-floating` also falls back through `mux`, allowing a split on older servers.  It selects the review within the source window when that window still selects the source pane and the session has one attached client.  This also applies while the client views another window or the terminal is inactive; the displayed window and OS focus stay unchanged.  On finish, it restores the source only while the review remains selected in the sole attached client, that client is focused, and focus events are enabled, then removes its own pane.  A selection made elsewhere is preserved.

`desktop-terminal` detects the client's terminal from `TERMINAL_APP`, `TERM_PROGRAM`, `ITERM_SESSION_ID`, or `WT_SESSION`.  On macOS it tries Ghostty or iTerm2 when identified, then Terminal.app; on Linux it uses `xdg-terminal-exec`, or Windows Terminal from WSL.  The named launchers open new windows and can be selected regardless of the current terminal.  macOS launchers bring the new window to the front so the reviewer notices it.  Ghostty requires its 1.3+ AppleScript API.  Windows Terminal runs the Linux binary in the same WSL distribution and directory; native Windows binaries are not supported.

Built-in GUI launchers are unavailable on positive SSH evidence from the client environment, the resolved tmux session, or validated client-process ancestry (`sshd` or `sshd-session`).  Missing evidence allows GUI use.  Multiplexer and custom launchers remain eligible.  With a single attached tmux client, refreshed session environment supersedes old wrapper SSH hints after reattachment.  Multiple clients cannot supply that override.  A daemon's own environment is not substituted for a marker-bound client.

Platform scripts are embedded under `internal/cli/launchers/` and need no installation.  They illustrate native startup operations; their private argument interface is distinct from the custom JSON contract below.  When the review ends after Submit, macOS launchers request closure of only their created window by its captured ID.  Cleanup is best effort: automatic closure or a terminal confirmation dialog does not invalidate the completed review.  xdg and Windows Terminal rely on the review command exiting; terminal settings may retain the window.  No launcher quits the entire terminal application or restores OS focus unconditionally.

For `custom:EXECUTABLE`, a name containing `/` is an explicit path, relative to the caller's working directory when needed.  Otherwise Critty searches PATH, including the current directory only when PATH explicitly contains `.` or an empty element.  The name is one executable, not a shell expression or an argument list.

Critty invokes the executable with one operation argument:

| Operation | Standard input | Result |
| --- | --- | --- |
| `check` | JSON with `session`, `command`, and `cwd` | Exit 0 when usable; nonzero tries the next launcher. |
| `open` | The same JSON | Return promptly after launching; stdout is the opaque close token. |
| `close` | Exactly the bytes returned by `open` | Close owned resources and restore focus as appropriate; nonzero fails the command. |

`command` is a shell-quoted TUI command; run it in `cwd`.  Each operation has a 10-second deadline.  Write diagnostics to stderr; check diagnostics are reported only if all launchers fail.  Keep `open` stdout exclusively for the token.  If `open` fails after creating resources, the launcher must clean them up itself.  After a successful `open`, Critty calls `close` even if the TUI fails to become ready.

Each operation inherits the caller's environment with stale `TMUX`, `TMUX_PANE`, `HERDR_*`, and `TTY` replaced by the detected terminal context.  Other variables, including the state/config locations, are preserved.  `CRITTY_TERMINAL_APP` carries a detected application name and `CRITTY_SSH=1` reports positive SSH evidence.  Use `critty env` to inspect or reuse the same detection.

## Terminal environment

`critty env` locates the Herdr or tmux pane the same way a review launch does, from the multiplexer environment or from process ancestry, and prints the variables that address it.

```bash
critty env                       # NAME=value lines: TMUX, TMUX_PANE, TTY, or HERDR_*
critty env --json                # the same as one JSON object
critty env tty                   # only the terminal device, such as /dev/ttys003
critty env run <command> [args]  # exec the command with those variables set
critty env -- <command> [args]   # the same
```

Flags go before the mode word.  Without a pane, `TTY` is the controlling terminal of the nearest ancestor process, then `SSH_TTY`; the command fails when no terminal or client hints are found.  Hooks that run detached from the agent's terminal can use `critty env tty` to find it.  `--terminal-request <id>` resolves a prepared marker as a review launch would, so it only applies inside a flow that displays one.
