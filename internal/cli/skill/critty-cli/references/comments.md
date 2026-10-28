# Reading and writing comments

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
      "author": "Reviewer",
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
          "author": "Reviewer",
          "review_round": 1,
          "replies": [
            { "id": "rp_9b21d4", "body": "Removed the old names.", "author": "Agent", "review_round": 1 }
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
critty comment --session <id> --author '<agent-name>' '<body>'                     # review-level
critty comment --session <id> --author '<agent-name>' <path> '<body>'              # file-level
critty comment --session <id> --author '<agent-name>' <path>:<line> '<body>'       # one line
critty comment --session <id> --author '<agent-name>' <path>:<start>-<end> '<body>' # line range
critty comment --session <id> --reply-to <comment-id> --author '<agent-name>' '<body>'
```

Rules:

- Always pass `--author` with your agent name so the thread shows who wrote what.
- Read the whole thread before replying; `critty` defines how to identify new feedback and avoid repeated replies.
- Use a single-quoted argument for a short body without apostrophes.  For bodies containing apostrophes, multiple lines, or complex quoting, use a JSON file even for one comment.  Double quotes let the shell interpret backticks and `$`.
- Line numbers are 1-indexed source coordinates, not positions in the diff text.  Staged reviews use index contents, committed comparisons use the right endpoint, and supplied diffs use the saved snapshot.  Files on disk may differ or be absent.  Use comment anchors when content changes between rounds.
- Bodies are Markdown; code fences and inline code render in the TUI.
- Do not pass `--resolve` unless the user explicitly asks for it.  The same applies to the `resolve` field in bulk JSON.

## Bulk comments

Use a JSON array for three or more comments or replies, or for any body that is awkward to quote safely:

```bash
critty comment --session <id> --json --file comments.json --author '<agent-name>'
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
