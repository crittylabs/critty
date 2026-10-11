# Changelog

## Unreleased

### Features

- The bundled `critty-approval` skill prepares Markdown approval attachments with YAML frontmatter and interactive confirmations, choices, and consents, then ties execution to the exact human-approved result.  All four agent integrations install it.

- `--null` reviews only attached documents without requiring a Git repository.

- Search the current file incrementally with `/` or `ctrl+s` forward and `?` or `ctrl+r` backward, as in Emacs.  Queries are regular expressions with smart case, matches stay within a line, and visible matches are highlighted.  `alt+s` followed by `c`, `w`, or `r` toggles case folding, word search, and regular expressions, and the prompt names the active modes.  `ctrl+d` and `ctrl+w` add the character or word after the current match to the query.

### Changed

- The keyboard help moves from `?` to `alt+?`, since `?` now starts a backward search.

### Fixes

- Empty diff input, including `--diff=/dev/null`, can be reviewed with attachments and resumed from its saved session.

- Markdown quotes and italic text use the primary text color to stay readable on diff backgrounds.
- `n` and `N` stop at added, deleted, and modified binary files even though they have no diff hunks.

## 0.18.0

Critty is the new name of crit.  This first release from `crittylabs/critty`, the original crit repository, brings in everything developed as the TCrit fork since crit 0.2.2.  The [changelog](https://github.com/crittylabs/critty/blob/main/CHANGELOG.md) lists every change in detail under TCrit 0.3.0 through 0.17.0.

### Moving to Critty

- Install from the new repository and Go module path, `github.com/crittylabs/critty`: `go install github.com/crittylabs/critty/cmd/critty@latest` or `mise install github:crittylabs/critty`.  Release archives are named `critty_*`.
- The command and the bundled agent skills are `critty` and `critty-cli`.  Old `tcrit` commands and skills are not aliases: update scripts, reinstall the skills with `critty install`, and remove the old `tcrit` and `tcrit-cli` skills.
- Reviews, configuration, and custom prompts saved by TCrit stay available.  Their paths, `.tcrit.config.json`, the `tcrit_state` field, and `TCRIT_HOST` are unchanged.
- Codex terminal markers start with `CRITTY-`.  tfil 0.4.1 and later report them with `--critty-notify`; `--tcrit-notify` remains an alias.
- TCrit releases remain available as the `tcrit-v0.3.0` through `tcrit-v0.17.0` tags.
- Critty has its own logo and icon.

### Agent workflow

- Review commands follow the [Crit](https://crit.md/) workflow: they block until the reviewer finishes, print a result for the agent, and continue in later rounds with `critty --session <id>`.  Reviews keep [Crit](https://crit.md/)-compatible `review.json` data, and `critty comment` and `critty comments` script them.
- `critty install` adds the shared `critty` review loop and `critty-cli` reference to Claude Code, Codex, OpenCode, and Gemini CLI.  `critty check` reports stale integrations, and finish prompts can be customized.  The skills describe local effects and the scope of a human approval.
- Reviews open in a full-width Herdr tab or a tmux split, found from process ancestry when the multiplexer environment is missing.  Each round closes its pane or tab.
- Codex reviews find their pane through the bundled `codex` wrapper or `critty codex`, a one-time marker shown by the skill, and optional notifications from terminal filters such as tfil.
- `critty env` prints the terminal variables of the pane a review would use, as `NAME=value` lines or JSON.  `critty env tty` prints only the terminal device, and `critty env run <command>` runs a command with those variables.

### What to review

- `--scope` selects `all`, `staged`, `unstaged`, or a committed comparison such as `main..HEAD` or `main...`; `--staged` and `--unstaged` are shortcuts.  Each scope keeps its own session, shown in the header.
- `git diff <base> <head> | critty --diff` reviews any Git unified diff, including outside a repository.
- `critty plan` saves each revision of a plan and carries comment threads forward.
- Repeated `--attachment FILE` flags show documents such as commit messages before the reviewed files.  Task lists in attached Markdown become forms for required confirmations, choices, and individual permissions, and `--result FILE` exports the approved answers.
- Every review gets its own session ID.  `critty stop --session <id>` keeps it, and `critty --session <id>` resumes it.
- `--focus PATH[:LINE]` and `critty FILE:LINE` start at a file or line.
- The header names the project and the Git branch under review; `--project` and `--topic` replace them.

### Comments

- Threads support replies, resolution, and reopening.  `r` toggles resolution in place, and `R` resolves and moves to the next unresolved thread, or opens the finish dialog after the last one.
- `f` comments on the whole file, and removed lines can be commented on too.
- `ctrl+v` pastes clipboard images into comments as Markdown attachments.
- The comment editor has a kill ring (`ctrl+y`, `alt+y`), GitHub-style suggestions (`alt+s`), `$EDITOR` editing (`ctrl+o`), and `@path` completion.  `alt+w` copies a source or thread reference, and clicking `@path L40` or a comment ID jumps to it.
- Comments follow edits between rounds: anchors match moved text, multi-line ranges resize with insertions and deletions, and comments without a matching line fold into markers at the right edge of the code.
- `h` folds all resolved threads and `H` hides comment boxes for reading the diff.
- A new round focuses the first thread with a fresh reply from another author.
- Each author has a color, unfocused threads shrink to the latest message, and the focused thread has a bright border.

### Navigation and display

- The sidebar has a **Files** tab with a foldable tree that follows the active file.  File tabs use compact paths, follow the tree order, and mark new and deleted files as `(A +12)` and `(D -8)`.
- `n` / `N` visit changes and unresolved comments across files; `[` / `]` and `{` / `}` move between all or unresolved threads.  `alt+p` switches files by fuzzy search.
- The top line names what is under review and ends with a **Submit** button.  The mouse can click tabs, lines, threads, and buttons, scroll, and drag the gutter to select lines.
- `w` ignores whitespace in diffs.
- Text adapts to light terminal backgrounds, approval dialogs are light green, and editors place the real terminal cursor for input methods.

### Diffs

- Inline replacements keep whitespace, long highlighted lines wrap, and syntax highlighting starts faster.
- Paths with spaces or special characters work, and binary files keep their added, deleted, modified, or renamed status.

### Fixed

- Tab-indented code quoted in comment and reply dialogs stays inside the preview border when it wraps.
- The second click of a double-click that opens a dialog no longer activates a control under the pointer.

## TCrit 0.17.0

### Added

- Attach text and Markdown documents to any review with repeatable `--attachment FILE` flags.  Documents appear before the reviewed files, keep their own comments, and survive review restarts as saved snapshots.  Pass the complete attachment list to refresh it, or use `--clear-attachments` to remove it.
- Answer task-list forms in attached Markdown with `Space` or by clicking their checkboxes and labels.  Forms support required human confirmations, single choices, and individual operation permissions.  Answers persist separately from source documents; changes to the review invalidate confirmations and permissions.
- Export self-contained form results with `--result FILE`.  Results include the reviewed content, field definitions, human answers, and permissions explicitly granted on approval.  Overall approval alone grants no individual permission.
- Bundled agent skills explain attachment snapshots, form answers, result verification, and cleanup.

### Changed

- File tabs follow the expanded file tree: directories first, then names within each directory.  Attached documents stay together before project files, including removed documents with saved comments.

## TCrit 0.16.5

### Changed

- File tabs, the file tree, and the file selector show new files as `(A +12)` and deleted files as `(D -8)`.  Binary files show `(A)`, `(D)`, or `(M)` without line counts, including in their content placeholders.  Only the text inside parentheses is colored: green for additions, red for deletions, and yellow for binary modifications.
- Bundled agent skills explain local storage, communication, command side effects, and the scope of human approval.

### Fixed

- Binary detection preserves whether a file was added, deleted, modified, or renamed across review scopes and supplied diffs.

## TCrit 0.16.4

### Added

- The review header shows the project name and Git branch, with `--project` and `--topic` overrides retained across review rounds.

### Changed

- The file tree starts fully expanded under the project name.  Folding or unfolding a folder cascades through single-child directory chains.

## TCrit 0.16.3

### Added

- `--focus PATH[:LINE]` opens a review at a specific file or line, including when resuming a session.  Single-document reviews also accept `FILE:LINE`.
- Bundled agent skills document startup file and line focus.

### Changed

- Submit dialogs for Approve and Resolve All & Approve use light green borders, titles, and messages.
- Update Go dependencies.

## TCrit 0.16.2

### Added

- `tcrit env tty` prints only the terminal device, and `tcrit env run <command>` runs a command with the terminal variables set.  `tcrit env -- <command>` keeps working.
- The `tcrit-cli` skill documents `tcrit env`.

### Changed

- When no Herdr or tmux pane reports a terminal, `tcrit env` takes `TTY` from the controlling terminal of the process or its nearest ancestor, then from `SSH_TTY`, and succeeds with `TTY` alone outside a multiplexer.

## TCrit 0.16.1

### Added

- `tcrit env` prints the `TMUX`, `TMUX_PANE`, and `TTY` or `HERDR_*` variables that identify the current Herdr or tmux pane, as `NAME=value` lines or JSON with `--json`.  `tcrit env -- <command>` runs a command with those variables set.  The pane is found through process ancestry when the variables are missing.

### Changed

- The README embeds the demo recordings as inline video players.
- Automated releases publish an existing draft release, keeping its attached recordings and replacing its notes with the matching `CHANGELOG.md` section.

## TCrit 0.16.0

### Added

- Resolution indicators on displaced comment markers and a clickable resolution control in reply dialogs, also available with `alt+r`.
- Three comment display modes: normal display, `h` to fold all resolved threads, and `H` to hide comments.  Normal display keeps threads resolved in the current round visible.
- A Codex review-and-commit demo alongside refreshed Claude Code and document-review recordings.

### Changed

- Clicking a grouped displaced-comment marker expands or folds all its threads together.  Its checkbox only displays status.
- Resolution changes in reply dialogs apply immediately, independently of saving reply text.
- Unfocused comment borders use dark gray; grouped marker counts use blue.
- Demo recordings use MP4 and are hosted as versioned release assets instead of being tracked in Git.  The upload helper accepts GIF and MP4 files, including attachments to draft releases.
- `make -C demo` records full-color MP4s directly, without GIF palette processing.

### Fixed

- Unresolved displaced threads expand when a new round focuses a fresh agent reply.
- Double-clicking a marker cannot accidentally toggle a dialog control that appears beneath the pointer.
- Displaced threads and reply dialogs start at the latest message, with the original quote available by scrolling up.

## TCrit 0.15.0

### Added

- **Displaced comment markers** — comments whose positions no longer match the source stay accessible through faint `💬` markers.  Focus a marker and press `enter` to expand it, or click it; the thread shows its original text for context.
- **Unresolved thread navigation** — `{` / `}` visits only unresolved comments; `[` / `]` visits all comments.
- A custom review-and-commit flow demo, plus MP4 recording targets for the demos.

### Changed

- Resolved threads stay folded when focused; `enter` opens their contents in the comment editor.  The `h` folding toggle is removed.
- Clicking **Resolve** folds the thread; clicking **Resolved** reopens it with its history visible.
- `]`, `}`, and the thread header's **↓** open the finish dialog at the last eligible thread.  `R` opens it when no unresolved threads remain.
- The whitespace indicator is shortened to `[WS: ignored]` and placed beside **Submit**, remaining visible on narrow terminals.
- Bundled agent skills wait on the review command itself and explain how to keep its execution timeout from interrupting an open review.
- The README focuses on usage, with the full fork comparison in `docs/upstream-differences.md` and recording instructions in `demo/README.md`.
- `tcrit comment --help` describes the fields accepted by bulk JSON import.

### Fixed

- Comment anchors prefer exact matches over nearby similar text and follow edits within a line more reliably.
- Multi-line comment ranges expand or shrink with internal insertions and deletions, preserving their original anchor text across rounds.

### Removed

- The Claude Code plugin.  Use `tcrit install claude-code` to install the bundled review skills.

## TCrit 0.14.0

### Added

- **Review bar** — the top line names what is under review and where.  The button that opens the finish dialog moves there from the footer as **Submit q**.
- **Thread navigation buttons** — **↑** / **↓** in each comment header move to the previous or next thread like `[` / `]`, and the buttons line up across threads.
- **Light terminal backgrounds** — text, the comment editor, and dialog dimming adapt to light backgrounds.

### Changed

- The header and footer are tidied up: the header line shows the path, line position, and comment count, and the footer keeps the key hints and **Help**, disappearing when the terminal is too short for it.
- File tabs abbreviate directories to their first letter, bold the active file, and underline files with unresolved comments.
- Counts use the singular for one.
- The bundled skill returns from the review wait as soon as TCrit finishes.
- The demo recordings are redone with a revised scenario.

### Fixed

- Renamed files are diffed against their previous path, so a rename no longer counts every line as added.
- The last file tab joins the pane border when the tabs fill the width exactly.

## TCrit 0.13.0

### Added

- **File tree sidebar** — the sidebar now has **Files** and **Comments** tabs and opens on **Files**, a directory tree of the reviewed files with foldable folders.  `t` switches views and focuses the sidebar; clicking a tab does the same.  The tree cursor follows the active file tab and vice versa, unfolding folders as needed.  Each level indents by one column, and rows too long for the sidebar scroll sideways with `→` / `l`, returning with `←` / `h` before those keys fold or climb.
- **Help button** — the footer shows a **Help ?** button next to **Approve** / **Finish Review**; clicking it opens the keyboard help.
- **Dialog dismissal** — every dialog has a red `x` at the right edge of its title row, and clicking outside a dialog dismisses it.  Both act like `esc`, so unsaved comment edits still ask before discarding.

### Changed

- `r` resolves or reopens the focused thread without moving.  The new `R` resolves it, or leaves an already resolved thread as is, and advances to the next unresolved thread as `r` used to.  Clicking **☐ Resolve** stays in place.
- The comment sidebar is a separate pane to the right of the tabbed source pane, with the same height and its own tabs, instead of a divider inside the frame.  Hiding comments with `H` keeps the narrow gutter inside the source pane.  The sidebar is at least 26 columns wide.
- Focused comment boxes, the sidebar, and dialogs keep rounded borders; focus is shown by a brighter border color instead of thick borders.
- Footer key hints use bold cyan keys on a gray background with normal-colored descriptions, separated by middle dots, so they are readable and distinct from the clickable buttons.

## TCrit 0.12.0

### Changed

- Codex wrappers register the invoking terminal and preserve all original arguments and environment values.  Remove automatic daemon startup and injected `--remote`, `--cd`, and shell settings, restoring Codex's normal directory, permission, and worktree behavior.

### Added

- One-time terminal markers for Codex reviews.  The skill displays a fresh marker before each round; TCrit searches only the visible text in registered live panes and rejects missing or ambiguous matches.
- `tcrit terminal prepare`, `tcrit terminal notify <id>`, and `--terminal-request <id>`.  [tfil](https://github.com/knu/tfil) 0.4.0 supports marker detection and notification with `--tcrit-notify`, letting TCrit skip pane text searches.  Requests expire and are consumed once; Herdr operations retain the selected server socket.

## TCrit 0.11.1

### Added

- **Codex launch wrappers** — `tcrit codex` and the bundled `codex` binary preserve tmux and Herdr context through per-thread shell settings.  [Codex 0.157.0](https://github.com/openai/codex/releases/tag/rust-v0.157.0) enabled automatic background-server startup by default, separating shell tools from the terminal process ancestry TCrit used for detection.  The wrappers start and connect to the local daemon when supported, pass through older CLIs, and chain with forward-searching PATH wrappers such as tfil.
- When terminal detection fails, suggest correcting PATH order if another `codex` precedes TCrit's bundled wrapper, including `[tools]` ordering for mise.

### Fixed

- Forward terminal context when reconnecting through an explicit `--remote unix://` or `--remote unix://PATH`, including Codex's printed reconnect commands.  Preserve the selected socket without starting a daemon.  Document that running tasks may retain their previous terminal context until Codex accepts configuration overrides.

## TCrit 0.11.0

### Added

- **File path completion in comment editors** — typing `@` and a path character at the start of a line or after whitespace lists the entries of that directory, one level at a time, below the `@` or above it when the screen is short.  Candidates match fuzzily; `tab` accepts the first one, Up/Down with `enter` accept a chosen one, and `esc` closes the list.  A file completes with a trailing space; a directory completes with `/` and lists its entries.
- **Fuzzy file selector** — `alt+p` opens a dialog with a search field over the review's file tabs.  Space-separated words must all match, and basename matches rank first.  Up/Down or `ctrl+p` / `ctrl+n` choose, `enter` switches tabs, and the dialog keeps a fixed size while typing.

### Changed

- File references accept unescaped letters, digits, and combining marks of any script; non-ASCII punctuation still ends a reference.  Prose may follow a path directly without a space, as full-width text commonly does in some languages; the longest leading part naming an existing file is linked.  Copied and completed paths escape underscores at word boundaries so names such as `__init__.py` survive Markdown rendering; both spellings are recognized.
- Added `github.com/sahilm/fuzzy` as a direct dependency for fuzzy matching.

### Removed

- The `/` tab search.  The key is reserved for a future in-source search.

## TCrit 0.10.0

### Added

- **Clipboard images in comments and replies** — `ctrl+v` pastes copied image files or bitmap data on macOS through AppKit, and images on Linux through `wl-paste` or `xclip`.  When no image is available, it falls back to text.  Attachments support PNG, JPEG, GIF, and WebP up to 5 MiB each; Escape cancels a pending paste.
- Image attachments persist across review rounds and stop/resume.  The agent reads images in the final review result before running the printed session cleanup command.  The TUI shows Markdown references rather than inline image previews.

### Changed

- Bundled agent skills reduce excessive token use during human review waits by keeping waits inside tools and avoiding repeated reasoning and status polling.  They now bound the overall wait to ten minutes, then preserve the running review and recoverable output; `hey` resumes result collection when the host cannot resume the agent automatically.
- CI runs formatting checks, vet, and tests on both Linux and macOS, including the AppKit clipboard tests.  The required `test` check succeeds only when both platforms pass.
- Updated Go to 1.27.1, Go dependencies, and Harden-Runner.

## TCrit 0.9.5

### Added

- **File references and source navigation** — comments and replies link existing `@path/to/file` references, with optional ` L40` line numbers and backslash escapes for paths containing spaces.  Click to jump within the review or confirm opening other locations in `$EDITOR`.  `alt+g` asks for a line number, and `alt+e` opens the current file at the cursor line.  Editor navigation supports both `--goto FILE:LINE` and `+LINE FILE`.

### Changed

- Bundled agent skills distinguish recoverable launch failures from interruptions after the TUI opens.  They keep waiting for review submission unless the host can automatically resume the agent and preserve the result, and clarify completion notifications and nested wait handling.
- Check previously unhandled errors in CLI setup, socket cleanup, and tests; simplify test code and clear all lint findings.

### Fixed

- Reply editors omit the reply being edited from the reference thread, so its saved body is not duplicated above the input field.  Other replies and their original numbering remain visible.

## TCrit 0.9.4

### Changed

- New review rounds focus the first visible thread, in file and display order, with a reply added by another author since the previous submission.  The thread expands and scrolls to its latest new reply, including resolved threads and file comments.  Without a new reply, or when resuming an unfinished round, the starting position is unchanged.  `[` / `]` navigation is unchanged.
- File tabs show both added and deleted line counts, such as `(+12 -5)`, omitting zero counts.
- Syntax highlighting reuses the lexer, style, and formatter within each file, substantially reducing startup time for diffs with many deleted lines while preserving rendered output.
- Refactored TUI internals and consolidated overlapping tests without changing their intended behavior.

## TCrit 0.9.3

### Added

- **Kill ring** — comment and reply editors share up to 60 entries during the current TUI run.  Line and word kills (`ctrl+k`, `ctrl+u`, `ctrl+w`, `alt+d`, and their aliases) save deleted text; consecutive kills combine in text order.  `ctrl+y` yanks the latest entry, and `alt+y` immediately after a yank or yank-pop cycles through older entries.  Other input ends the sequence.  The ring is independent of the system clipboard; ordinary Delete/Backspace do not add entries.

### Changed

- File-comment threads appear above the first source or deleted line, directly below the tabs, including for empty, deleted, and binary files.  They remain listed in the sidebar; creating a file comment or navigating to one focuses its inline box.  Inline reply, resolve, delete, folding, and visibility controls apply to file comments too.

### Breaking changes

- Suggestion insertion moves from `ctrl+y` to `alt+s` (`M-s`).  `ctrl+y` now yanks from the kill ring; the Suggest button remains available.

## TCrit 0.9.2

### Changed

- `f` replies to an existing file-comment thread, including resolved threads, and creates a new thread only when none exists.  Saving a reply reopens a resolved thread.
- Inline and sidebar thread headers now have clickable **Resolve / Resolved** toggles and a right-aligned red **x** deletion button.  Deletion remains limited to your comments from the current round with no replies and requires confirmation.
- Resolution controls stay in place when toggled; collapsed resolved threads omit the author name.
- Keyboard help uses distinct colors for section headings and key names, with brighter descriptions.
- Bundled agent skills read saved feedback after a nonzero review exit, then stop work and wait for explicit chat instructions instead of automatically restarting TCrit.

### Fixed

- Keep the right outer frame visible when the comment sidebar is open, with mouse targets aligned to the pane layout.

## TCrit 0.9.1

### Added

- **Ignore whitespace** — `w` toggles whitespace-insensitive diffs across all files in Git and supplied-diff reviews.  Whitespace-only changes are excluded from highlighting and `n` / `N` navigation while source text, line numbers, and comment context remain intact.  Added and deleted blank lines remain visible; the setting lasts for the current TUI run.

### Changed

- `n` / `N` visits change hunks and unresolved comment threads in display order from either pane, including file comments, deleted-line comments, and multiple threads on one line.  Selecting a hidden comment reveals it; resolved threads are skipped.
- `Tab` / `Shift+Tab` switches file tabs from the sidebar as well as the content pane, preserving pane focus.

## TCrit 0.9.0

### Added

- **Saved review sessions** — stop a review with `tcrit stop --session <id>`, discover saved sessions with `tcrit status`, and resume after process exit with `tcrit --session <id>`.  Source snapshots preserve comment context across rounds; `tcrit clear --session <id>` deletes a stopped review.
- **Read diffs without comment boxes** — `H` hides inline comments and replaces the sidebar with a narrow gutter.  Click a comment marker to open its thread; press `H` again to restore comments without changing resolved-comment folding.

### Changed

- Every submitted round closes its TUI and dedicated Herdr tab or tmux pane.  The next round opens a new process and restores saved comments and replies.
- Review results include resolved threads and replies, even on approval, so final instructions survive automatic cleanup.  Bundled skills read all returned threads and reply only to new feedback or substantive updates.
- Focused comments and editors use bright, thick borders; unfocused comments use thin blue borders.  Modal dialogs also use thick borders.
- Saving an empty new comment or reply closes the editor without creating an entry.  Saving an unchanged edit leaves review state unchanged; clearing an existing entry and saving deletes it without another confirmation.
- Legacy review state is ignored without a warning.

### Breaking changes

- Each new review receives an independent session ID, even for the same directory, scope, or plan name.  Continue an existing review explicitly with `--session <id>` instead of repeating its original command.
- Plan revisions are stored inside their review session directory.  Submit revised plans with `tcrit plan --session <id> <file>`; plain `tcrit --session <id>` reopens saved plan or supplied-diff input.
- `tcrit clear` refuses active reviews.  Stop the review before deleting its saved state.

## TCrit 0.8.2

### Changed

- Resolving a thread with `r` now moves focus to the next unresolved thread, wrapping across files and skipping resolved threads even when unfolded with `h`.  When none remain, focus returns to the source.  Reopening a thread keeps its focus.

## TCrit 0.8.1

### Fixed

- `[` / `]` now visit resolved line and file comments when unfolded with `h`, while continuing to skip them when folded

### Documentation

- Clarified comment editing shortcuts, suggestion replacement, and scrolling through code context and thread history

## TCrit 0.8.0

### Added

- **Supplied diff reviews** — `tcrit --diff[=FILE]` accepts Git unified diffs from a file or stdin, including outside a repository; snapshots survive Herdr/tmux launches and can be replaced for another review round
- **Fixed review scopes** — `--scope=all|staged|unstaged|A..B|A...B` keeps each comparison and its comments in a separate session; `--staged` and `--unstaged` are aliases, and committed comparisons display the right-hand commit's content
- **Native input cursor** — comment and reply editors expose the real terminal cursor so IME composition follows the insertion point after wrapping and scrolling
- **Resolved-thread display toggle** — `h` folds or unfolds resolved comments across all files, including their visibility in the sidebar

### Changed

- **Author-based comment colors** — comments and replies share a color per author, with your own messages in white
- Unfocused threads show only the latest message; focused histories use at most 10 wrapped rows, and reply dialogs use up to 16, with older or longer content accessible by scrolling
- The mouse wheel focuses and scrolls comment threads; resolved threads expand on focus and fold again on blur unless folding is disabled
- `[` / `]` skip resolved comments; vertical movement still visits them, while visual range selection skips comment boxes
- Suggest selects the inserted code for immediate deletion or replacement while preserving the fences and existing comment
- Gemini installs the same shared `tcrit` and `tcrit-cli` skills as the other integrations, with review rounds handled by the original agent

### Breaking changes

- Git review defaults to `--scope=all` (staged, unstaged, and untracked changes).  To review against a branch, specify a comparison such as `--scope=main...HEAD`
- Removed `--base`, `--base-branch`, the `base_branch` setting, and automatic base-branch fallback; use `--scope` instead
- Existing installations of the old Gemini subagent may retain `.gemini/agents/tcrit.md`; check for customizations before removing it and reinstall the shared skills with `tcrit install gemini`

## TCrit 0.7.3

### Added

- **Staged-only reviews** — `tcrit --staged` and `tcrit review --staged` read the file list, content, and diffs exclusively from the Git index; bundled agent integrations document the safer scope
- **Visible review scope** — the code-review header always identifies staged changes, the working tree, or the active base ref

## TCrit 0.7.2

### Fixed

- `tcrit install opencode` run from the home directory now installs into OpenCode's global config directory (`$XDG_CONFIG_HOME/opencode`, `~/.config/opencode` by default) instead of `~/.opencode`

## TCrit 0.7.1

### Added

- **OpenCode integration** — `tcrit install opencode` installs the `/tcrit` command and the `tcrit-cli` skill; the `all` target now includes OpenCode
- **`tcrit-cli` reference skill** — documents `tcrit comment`, `tcrit comments`, session and plan targeting, bulk JSON input, the review file format, and clearing, so agents no longer guess at headless usage

### Changed

- **Simpler skill set** — a single `tcrit` skill passes its arguments straight to the CLI, which chooses between git changes, a document, and a plan, instead of asking the user which mode to use; Claude Code, Codex, OpenCode, and Gemini all install the same `tcrit` + `tcrit-cli` pair
- The Claude Code plugin ships the skills under `plugin/tcrit/skills/` and is invoked as `/tcrit:tcrit`

### Fixed

- Files deleted since the base ref now show their removed lines in the code review TUI instead of an empty tab
- A commented file whose addition is reverted in a later round now shows a placeholder instead of its stale content; the comments stay in the sidebar and in `review.json`
- Resolved file comments stay in the sidebar as collapsed headers so they can be reopened with `r`, instead of vanishing
- `[` and `]` now visit file comments too, moving focus to their sidebar entry, and work from the sidebar as well as the content pane

### Removed

- The `tcrit-review`, `tcrit-code-review`, and `tcrit-plan-review` skills, the `tcrit:review`, `tcrit:code-review`, and `tcrit:plan-review` plugin commands, and the legacy root-level plugin manifest inherited from upstream

## TCrit 0.7.0

### Added

- **Native Herdr workflow** — discover the invoking Herdr workspace, tab, and pane from inherited context or process ancestry, open reviews in a dedicated full-width tab, and restore focus to the agent tab between rounds

### Changed

- Multiplexer discovery now supports Herdr and tmux together and selects whichever context is nearest in the process ancestry when they are nested

## TCrit 0.6.3

### Added

- **Codex skill installer** — `tcrit install codex` installs project-local or global review skills with Codex invocation syntax and reply attribution; the `all` target now includes Codex

### Changed

- `Ctrl-PgUp` and `Ctrl-PgDn` scroll through complete code context and thread history while the comment textarea keeps focus

### Fixed

- A reviewer can edit only their latest reply when it is also the thread's latest reply, preventing an earlier response from being overwritten after another participant replies

## TCrit 0.6.2

### Changed

- Agent replies now use cyan text so they remain visually distinct from reviewer comments in inline and sidebar threads
- Dialog shortcut keys use highlighted backgrounds, with labels matching their actual keys
- Reply editors keep complete code context and thread history together in one scrollable region while leaving actions accessible in short terminal panes
- Newly saved comments remain selected so they can be deleted immediately if needed

### Fixed

- Inserting a suggestion leaves the cursor at the end of the final suggested code line and scrolls it into view

## TCrit 0.6.1

### Added

- **Deleted-line comments** — select and comment on removed lines with the keyboard or mouse, including lines in fully deleted files
- **Direct comment deletion** — press `d` to delete the selected comment after confirmation without opening its thread first

### Fixed

- Application headers consistently use TCrit branding
- The code header remains stable while dragging a mouse selection beyond the viewport

## TCrit 0.6.0

### Added

- **Mouse-first TUI controls** — click file tabs, code lines, comment threads, sidebar items, dialog buttons, and the review-finish button; scroll with the mouse wheel, and drag the gutter to select line ranges

## TCrit 0.5.1

### Added

- **Suggestion blocks** — press `Ctrl-Y` to insert a GitHub-compatible `suggestion` block for the selected line or a line comment's anchor, including in replies
- **External editor support** — press `Ctrl-O` to edit comment and reply bodies in `$EDITOR` without leaving the review session

### Fixed

- Changed-line backgrounds remain continuous across Markdown bold and code spans

## TCrit 0.5.0

### Added

- **File-level comments** — press `f` to comment on the active file without attaching feedback to a specific line; file threads appear at the top of the comment sidebar
- **Keyboard help** — press `?` for a compact reference covering navigation, review, search, selection, and dialog controls
- **Native tmux context discovery** — tcrit can find the invoking pane through process ancestry when agent environments do not inherit `TMUX` or `TMUX_PANE`

### Changed

- Resolved comment threads collapse out of the active sidebar while remaining available inline for reopening
- Code review tabs refresh between rounds so newly added and removed files appear without restarting the TUI
- The persistent footer now emphasizes review-specific actions and leaves conventional navigation keys to the help screen

### Fixed

- Concurrent CLI comments and replies no longer overwrite one another
- Long syntax-highlighted source lines wrap instead of being truncated
- Nested Markdown styles preserve ANSI colors and backgrounds correctly

## TCrit 0.4.0

### Changed

- **Crit-compatible review workflow** — review state now uses CritJSON, review commands block until feedback is finished, and `tcrit comment` / `tcrit comments` use crit-compatible syntax for agent replies and automation
- **Installation commands** — `tcrit install` and `tcrit check` replace the former `setup-claude` and `setup-gemini` commands

### Added

- **Versioned plan reviews** — `tcrit plan` stores immutable revisions and carries comments forward as plans change
- **Thread lifecycle controls** — reviewers can resolve or reopen threads, reply from the TUI, and delete only comments or replies they authored in the current round
- **Cross-file navigation** — `[` / `]` traverse comment threads, while `n` / `N` traverse changes without wrapping; `<` / `>` jump to the current file's first or last line
- **Resolve-all approval** — finishing a round without new feedback can resolve every remaining thread and approve in one step

### Fixed

- Comments retain their intended location across edited review rounds, including drift detection when the original text disappears
- Inline replacement diffs preserve whitespace and highlight only the words that actually changed

## TCrit 0.3.1

### Added

- **`--version` flag** — `tcrit --version` reports the build version, resolved from release ldflags or Go build info

### Fixed

- **Release automation** — GoReleaser still pointed at the pre-rebrand `cmd/crit` path, so tagged releases failed to build; releases now ship `tcrit` binaries again
- Release builds are now gated behind a passing CI run (gofmt, `go vet`, `go test`) with hardened, SHA-pinned workflows, and Renovate keeps dependencies and action refs up to date

## TCrit 0.3.0

Forked from [crit](https://github.com/kevindutra/crit) and rebranded to **tcrit**.

### Changed

- **Renamed to tcrit** — the binary, embedded skills, commands, and plugin are now `tcrit`; the Go module moved to `github.com/knu/tcrit`

### Added

- **`tcrit clear`** — clear comments for a document or code review session; `--all` deletes all saved review state (upstream PR #9)
- **Re-review flow** — starting a new review asks whether to keep or clear existing comments
- **Approval confirmation** — quitting a review without new comments asks for confirmation before approving
- **Gemini CLI support** — `tcrit setup-gemini` installs Gemini CLI agents for the review workflow (upstream PR #8)
- **Paging keybindings** — `PgUp`/`PgDn`/`Home`/`End` in the review TUI (upstream PR #10)

### Fixed

- Paths with spaces and special characters are handled correctly in git operations
- `n`/`N` change navigation jumps to the correct offsets in files with long wrapped lines
- Detached reviews open the tmux split from the invoking pane

## 0.2.2

### Changed

- Bumped plugin versions to 1.2.2

## 0.2.1

### Fixed

- `crit review --code --detach --wait` now correctly opens in a tmux split pane instead of failing with a TTY error

## 0.2.0

### Added

- **Multi-file code review** — `crit review --code` detects changed files in your git repo and opens a tabbed TUI with syntax highlighting and diff markers
- **Tabbed file navigation** — `tab`/`shift+tab` to switch files, `/` to search
- **Change navigation** — `n`/`N` to jump between changed lines within a file
- **Aggregate status** — `crit status --code` outputs comments across all reviewed files as JSON
- **Session management** — code review sessions are persisted to `.crit/code-review.yaml`
- **Review router skill** — `/crit:review` asks whether to review code or a document, then routes to the appropriate workflow
- **Code review skill** — `/crit:code-review` runs the full code review workflow in Claude Code
- **Plan review skill** — `/crit:plan-review` routes single-file document reviews

### Changed

- `tab` now switches between file tabs in code review mode (was: switch panes)
- `s` toggles the comment sidebar (was: `tab`)
- Review skill restructured into router pattern with separate code and plan review skills

## 0.1.0

Initial release.

- Interactive TUI for reviewing markdown documents
- Inline comments with visual select mode
- tmux split pane integration (`--detach --wait`)
- Scriptable CLI (`crit comment`, `crit status`)
- Claude Code skill (`/crit-review`)
- Shell completions (bash, zsh, fish)
