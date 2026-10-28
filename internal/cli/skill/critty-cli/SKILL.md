---
name: critty-cli
description: Reference for Critty commands, review data, and local effects. Use for scripted comments, session targeting, review inputs, attachments, form results, or terminal setup. Use the critty skill for the interactive review loop.
user-invocable: false
---

# Critty CLI Reference

Use `/critty` for the interactive review loop, `/critty-approval` for human decisions, and `/critty-history` for task bookkeeping.  This skill supplies their command and data contracts.  Read only the reference needed for the operation below.

## Routine review operations

```sh
critty review --staged --task <task-id> --summary <scratch>/round-1.json
critty sessions resume <session-id> --summary <scratch>/round-2.json
critty comments --session <session-id> --all --json
critty comment --session <session-id> --reply-to <comment-id> --author 'Claude Code' '<reply>'
critty sessions stop <session-id>
```

Create scratch space with `critty tmp create` and use its returned `directory` for `<scratch>`.  Keep each invocation's execution handle, complete stdout/stderr, exit status, and a fresh summary path.  Follow `/critty` for launching, waiting, recovering, and refreshing input between rounds.

- Use the exact session ID returned at startup or completion.  Scope stays fixed within a session; matching paths or scope arguments do not identify a session.
- Exit 0 means a submission, not necessarily approval.  Read `verdict`, `approved`, all threads and replies, and attachment snapshots.  Only `verdict: approve` with `approved: true` approves the reviewed content.  A failed export or nonzero exit grants no approval.
- Verify the summary's session, round, and reviewed input against this invocation.  Keep complete finish output; summaries omit attachment bodies, form answers, and individual consents.  Read [result contracts](references/results.md) before interpreting forms or recovering a result.
- Comments use source coordinates from the reviewed version, which may differ from disk.  Read the complete thread before replying, identify your agent with `--author`, and leave resolution to the human unless explicitly authorized.
- Use literal shell arguments.  For three or more replies, or any body with awkward quoting, use the [bulk JSON interface](references/comments.md#bulk-comments).
- Saved reviews are retained after approval.  Stop preserves state; removal deletes it.  Follow `/critty-history` for task completion and retention.

## Choose the needed reference

| Operation | Read before proceeding |
| --- | --- |
| Select a Git scope, document, plan, or supplied diff; refresh live sources; set labels or focus | [Review inputs and presentation](references/inputs.md) |
| Attach text or images, replace snapshots, or interpret attachment paths | [Attached documents](references/inputs.md#attached-documents) |
| Handle feedback marked `textconv` | [Edit originals from textconv feedback](references/inputs.md#edit-originals-from-textconv-feedback) |
| Interpret finish output, summary JSON, form results, or export failures | [Review output and results](references/results.md) |
| Write forms or interpret field values | [Form widgets](references/forms.md) and `/critty-approval` for authorization |
| Find or resume a session, move between worktrees, stop or remove a review | [Review sessions](references/sessions.md) |
| Read or post comments, replies, bulk JSON, ambiguous IDs, or stored anchors | [Reading and writing comments](references/comments.md) |
| Diagnose setup, bind the invoking terminal, choose a launcher, or reuse terminal environment | [Terminal setup and diagnostics](references/terminals.md) |
| Assess command effects, install integrations, or prepare scratch/application trees | [Storage and installation](references/storage.md) |
| Change settings or prompts | [Configuration](references/configuration.md) |

Critty stores reviews locally and communicates through local Unix sockets.  Review and comment commands do not publish repository changes or upload review contents.  Installation, temporary-tree application, external editors, custom launchers, and `env run` have their own writes or subprocess effects; assess the selected operation using the references above.

## Review launchers

Before selecting or troubleshooting a launcher, read the [launcher requirements and contract](references/terminals.md#review-launchers).
