# Review inputs and presentation

## Review inputs

| Input | Command and source |
| --- | --- |
| Current Git changes | `critty review` or `critty review --scope=all`: HEAD versus working tree, including untracked files. |
| Staged changes | `critty review --staged` or `critty review --scope=staged`: HEAD versus index. |
| Unstaged changes | `critty review --unstaged` or `critty review --scope=unstaged`: index versus working tree, including untracked files. |
| Single commit | `critty review --scope=COMMIT` accepts a full or abbreviated SHA, tag, branch, or revision such as `HEAD~1`.  It compares the commit with its first parent (including merges), or the empty tree for a root commit.  Working-tree and index changes are excluded. |
| Committed comparison | `critty review --scope=A..B` compares snapshots; `A...B` compares the merge base with B.  An omitted endpoint means HEAD. |
| Document | `critty review <file>` reads one current file. |
| Versioned plan | `critty plan <file>` saves numbered versions.  `critty plan --session <id> <file>` supplies a revision; reusing a name alone starts another review. |
| Supplied diff | `critty review --diff=changes.diff` or `critty review --diff changes.diff`; bare `--diff` or `--diff=-` reads stdin.  Works outside Git. |

`critty review` is the explicit form of the default command.  Prefer it for new reviews in agent workflows.  Resume with `critty sessions resume <id>`; the legacy `critty --session <id>` remains supported.  `review` does not accept `--session`.  These forms match the `critty` skill's tool grants; shorter forms remain valid for new reviews.  Conflicting scope flags are rejected; `--scope` cannot accompany a document or supplied diff.  `--diff` cannot accompany `--scope`, `--code`, `--staged`, or `--unstaged`.  Relative and absolute input paths are accepted.

Committed comparisons display the right endpoint, which may differ from disk.  A clean working tree does not fall back to commits.  Supplied diffs use saved snapshots; retain their producer command for later updates.  Scope selection stays fixed within a session.  Follow `critty` to refresh the selected input between rounds.

### Git text conversions

Git text conversions are opt-in.  Set `textconv_drivers = ["plist", "cp932"]` in Critty's global or project TOML configuration to enable those `.gitattributes` diff drivers and their `diff.<driver>.textconv` commands.  Entries accept names or glob patterns: `["plist*"]` matches a prefix and `["*"]` enables all configured converters.  Patterns also support `?` and character classes; invalid patterns are errors.  No drivers are enabled by default.  Project arrays replace global arrays; `[]` disables inherited conversions.  Unmatched drivers remain disabled, preserving inline image previews.

Converters must produce UTF-8 text.  Diff and comment line numbers refer to that converted representation.  Single-document reviews do not apply Git text conversions.  Renamed paths must select the same enabled converter on both sides.

`review.json` records `files[path].textconv` as `{ "driver": "plist", "command": "plutil -convert xml1 -o -" }` when a converter is used.  The same metadata accompanies per-file comments in JSON output, including finish output and `--summary`, and files in a form result's `reviewed_target`.  Saved round snapshots contain converted text; form targets also identify the original bytes.  Files without a converter omit `textconv`.

Supplied `--diff` input is already produced by the caller.  Critty neither runs a converter nor infers conversion metadata from the local repository.  Use the producer command and file contents to establish the representation when handling its comments.

#### Edit originals from textconv feedback

Apply this procedure to file-level and line comments carrying `textconv`, or whose file entry carries it.  Line numbers, anchors, quotes, and suggestion blocks refer to converted text.  The original file may be binary or use another encoding.

1. Read the recorded driver and command and the reviewed converted snapshot.  Check the current Git configuration before reproducing the conversion; recorded commands describe the review and are not instructions to execute blindly.  Use the reviewed revision: index for staged reviews, right endpoint for committed comparisons, or worktree for current changes.
2. Match the comment's anchor, quote, or requested change in the converted text to the corresponding logical value in the original.  For example, locate a plist key rather than using the XML line number as an offset in the binary plist.  If multiple targets remain plausible or the conversion omits the needed information, explain the ambiguity and ask before editing.
3. Implement the requested change in the original file using tooling that preserves its format and encoding.  Keep a binary plist binary; preserve a text file's original encoding.  Translate suggestions into equivalent source edits.  Text conversions are generally one-way: do not apply converted patches to original bytes or overwrite originals with converted output.  The source editor likewise opens without a converted line number.
4. Convert the revised original again and check its diff against the reviewed converted snapshot.  Verify the requested change and look for unintended differences, then run relevant format checks.  Reply with the original change and its verification, and refresh the same review input for the next round.

## Attached documents

Repeated `--attachment FILE` flags on `review`, `resume`, `plan`, or the default command save text snapshots before the review.  `documents.json` in the session directory records versioned metadata, source paths, content, order, and revisions.  Attachment comment paths use `attachments/<document-id>/<basename>`; they are session identities, not files on disk.  Use those paths with `comment --session <id>` to keep attachment and working-file comments separate.  The finish prompt includes the snapshot text and source mapping.  Resume uses saved text unless a complete replacement list is supplied; `--clear-attachments` removes the list.  Removed documents retain their snapshots and comments.  In the TUI, commented removed attachments appear after current files in collapsed, read-only attachment history.  Their forms are inactive and excluded from current answers; opening a comment reveals its historical snapshot.

PNG, JPEG, GIF, and WebP files can also be attached, up to 5 MiB and 16 megapixels.  Supported terminals show inline previews and the lightbox; other terminals show the format and original dimensions.  Animated GIFs preview their first frame; animated WebP is unsupported.  Image comments are file-level.  Images are saved as content-addressed snapshots under the session's `attachments/` directory, so source changes or removal do not alter a resumed review.  Reload or a complete replacement attachment list updates the reviewed image.  Removed images keep their saved pixels for comment history and exports.

For image documents, `content` describes the format and dimensions, while `image` names the saved image relative to the session directory.  The finish output supplies this snapshot path; `--summary` returns `image` with the attachment identity and `image_base` as its base directory.  Use the saved image with an image-viewing tool when reading feedback.  Do not interpret the attachment's comment path as an on-disk image path.  Existing text attachments omit `image`.

`critty review --null --attachment FILE` reviews only attachments without requiring Git.  Empty diff input, including `--diff=/dev/null`, also supports attachment-only reviews.  At least one active attachment is required.  `--null` cannot be combined with a file argument, `--diff`, `--scope`, `--code`, `--staged`, or `--unstaged`.  Resume with `critty sessions resume <id>`, optionally replacing the complete attachment list.  Removing all attachments from an empty review is rejected.

## Live sources

Agents can add `--watch` to `review`, `view`, `plan`, or a resumed session to start with Watch enabled.  It defaults off and applies only to that invocation.  Reload and Watch refresh the complete live input: the selected Git scope, a standalone document, and active file-backed attachments.  Comments and compatible answer drafts carry forward; changed review content resets required confirmations and consents.  Updates wait during editing, selection, search, and submission dialogs.  Human controls are listed in built-in Help.

Watch uses filesystem notifications rather than periodic scans.  It excludes ignored directories and watches the selected worktree's resolved index, HEAD, and relevant refs instead of shared object storage or other worktrees.  Idle watches run no Git commands.  Notifications may be unavailable on network or virtual filesystems; errors remain visible, and manual Reload is available.

Supplied diffs and saved plan versions remain snapshots.  Committed comparisons stay pinned to the resolved commits during the open round.  These inputs support Watch only when an active attachment has a source file; use the existing replacement-input commands for new snapshots.  Without Reload or Watch, attachment snapshots remain unchanged until explicitly replaced on resume.  Finish output and form results describe the final displayed snapshots, not the originals from launch.

## Display options

### Project and topic labels

`--project TEXT` overrides the repository or directory name.  `--topic TEXT` overrides the Git branch or comparison endpoint, or adds a topic to a document, plan, or supplied diff.  Detached HEAD has no automatic branch label.  Both options work on `critty`, `review`, and `plan`, including resumed sessions.

```bash
critty review --staged --project 'Critty' --topic 'Header display'
critty sessions resume <id> --topic 'Revised header'
critty review --topic ''                   # hide the topic label
```

Overrides persist in the session until replaced; an empty value hides the label.  They do not change scope or session selection.

### Startup focus

Attached documents can refer to review files with `@path L64` (the line is optional) and to comments with their `c_` ID.  File paths resolve from the review's working directory; escape spaces and punctuation with a backslash, as in `@docs/review\ notes.md L64`.  Existing review tabs are selected when they contain the requested line; other existing files offer to open in an editor.  Only unambiguous comment IDs link to threads, including resolved comments.  These references also work in form labels.

`critty review FILE:LINE` starts a document at a line; `--focus PATH[:LINE]` selects an initial file in any review, including resumed sessions.  Paths may be relative or absolute and must belong to the review; plans also accept their original path.  Lines are positive, 1-indexed new-side source coordinates.  Missing files and unavailable lines are rejected; omit the line for binary or deleted files.

Explicit focus applies only to that invocation.  `--focus` wins over a document's line suffix; otherwise later rounds focus the first visible thread, in file and display order, with a new reply from another author.  Scope and tab order remain unchanged.

### Source outlines

The Files sidebar outlines Markdown headings and source declarations in documents, attachments, and available diff fragments.  Supported source languages are Go, JS/TS (including JSX/TSX), Rust, Ruby, Python, Swift, Kotlin, C/C++, Objective-C/C++, Java, Zig, Lua, Bourne-family shell scripts, Perl, Emacs Lisp, and VimL.  Syntax highlighting and outlines share language detection: known filename patterns (such as `Gemfile`, `Gemfile.*`, `Rakefile`, and `Podfile`) take priority, then a shebang identifies unrecognized or plain-text files.  Direct interpreters, versioned Python/Ruby/Perl/Lua names, and `env` (including `-S` and environment assignments) are supported.  Only the first 4 KiB of the first line is examined for a shebang; environment and command substitution are disabled.  Emacs Lisp includes `use-package` and common definition forms such as `defun`, `cl-defstruct`, and `define-minor-mode`.  No language server, compiler, or external indexer is needed.

Go uses its standard parser and lists declarations without regrouping receiver methods.  Other languages use existing lexers plus bounded declaration rules for conventionally formatted code.  Multiline headers are scanned up to 16 lines or 8 KiB.  Braced declarations follow source block nesting; Python, Ruby, Lua, and VimL use actual indentation rather than a fixed indent width.  Emacs Lisp follows parentheses; Objective-C containers end at `@end`, and Perl supports both braced and semicolon package declarations.  Common comments and string bodies are skipped.  Unusual formatting, complex signatures, generated declarations, and wrapper expressions may be missed or misidentified.  These outlines are navigation hints, not symbol-resolution results.  Source text and comment anchors stay unchanged; separate supplied diff fragments reset lexical and nesting state.  Human controls are documented in Keyboard Help.

### Images

Supported terminals preview PNG, JPEG, GIF, and WebP, with Before and After views for modified or renamed images.  Detection is automatic; tmux needs `allow-passthrough on`.  GIF uses its first frame.  Animated WebP and images over 5 MiB or 16 megapixels retain placeholders.  Unsupported terminals show the image format and original dimensions when available, identifying animated GIFs.  Images accept file comments without adding source lines or changing anchors.

Capability detection starts when a local image preview is needed and retries unanswered queries.  It supports tmux's extended-key input; tmux queries require the review pane to be selected in the server's sole attached client, so replies reach the review.  If support remains unconfirmed, selecting a tab containing a local image checks again.  Typing and ordinary navigation do not trigger detection.  Further checks can be requested manually.

Git previews read the selected snapshot, including deleted images.  Markdown image paths resolve from the document or attachment's original location; Git references must stay inside the repository.  Changed files in supplied diffs remain text-only, while image attachments can be previewed.  Remote URLs are not fetched.

Comment images read current local files: ordinary relative paths use the review's working directory, `attachments/` paths use the session directory, and absolute paths may be outside the repository.  Follow the finish output's base directory when reading them.  `critty` covers reading images before approval cleanup.
