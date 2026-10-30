# Changelog

## Unreleased

### Changed

- **Clearer file paths**: Headers and tabs abbreviate your home directory as `~` and show directories in cyan with muted slashes.  Headers highlight paths from the Git or worktree root onward in purple.  The Files tree and tabs omit redundant ancestor directories for files opened by absolute path.  Tabs omit the root for a single project and retain distinguishable root names when multiple projects or worktrees are open.  Tab paths expand to use their share of the pane: one tab can use the full width, two or three share it, and four or more use about a quarter each.  Narrow tabs keep the filename's end visible, while pending review indicators take priority.

## 1.8.1

### Fixed

- **Antigravity CLI reviews**: Skills now load within the agent's size limit, with detailed instructions in linked references.  After upgrading, run `critty install --user --force agy` to refresh them.
- **Noninteractive terminal handling**: Agent command terminals such as Antigravity CLI's no longer cause unnecessary response waits or misleading warnings in `critty doctor`.  Reviews avoid opening directly in these terminals, and terminal discovery can use the parent terminal instead.

## 1.8.0

### Changed

- **UI and UX refinements**:
  - **Compact tab bar**: Gain two rows for documents and code.  Tabs no longer jump sideways when many files are open, and colors are easier to read.  Unread dots flag updated tabs.
  - **More informative Files tree**: Entries show the same review indicators as tab titles, making pending comments and unanswered form items easier to find.
  - **Clearer navigation**: `n`/`N` visits all hunks and review items, including resolved threads and answered items.  Help and footer hints are easier to follow.
  - **Fold comments with arrow keys**: Use left/right arrows to fold or unfold comments.  Enter now opens comments whose original source location cannot be tracked.
  - **Focus on reading**: View mode (`V`) minimizes every comment to a speech-bubble marker so you can focus on documents and code.
  - **Per-tab scroll positions**: Each tab remembers its document and Comments sidebar scroll positions.
- **Launcher refinements**:
  - **More desktop terminals**: Desktop reviews prefer terminals with image previews.  New launchers support kitty, WezTerm, and Alacritty; Ghostty also works on Linux.  WezTerm and Alacritty also work with Windows terminals from WSL; xterm is the final Linux fallback after `xdg-terminal-exec`.  Explicit launcher order takes precedence.  See [launcher requirements](https://github.com/crittylabs/critty/blob/main/internal/cli/skill/critty-cli/SKILL.md#review-launchers).
  - **Reliable fallback**: Marker lookup failures, including ambiguous matches, no longer stop reviews from trying desktop or custom launchers.  SSH restrictions still apply.
  - **Steadier launcher selection**: Updated review skills help Critty consistently select a suitable launcher and guide users to configure a preferred terminal.  Startup messages and review summaries explain launcher choices and fallback failures.

## 1.7.0

### Added

- **Source outlines**: jump to Markdown headings and source declarations from the Files sidebar, including in attachments.  Supports Go, JS/TS, Rust, Ruby, Python, Swift, Kotlin, C/C++, Objective-C/C++, Java, Zig, Lua, Bourne-family shell scripts, Perl, Emacs Lisp, and VimL.  Entries follow source order, with nesting where available; Go methods include their receiver type in a flat list.  Go uses its standard parser, while other source languages use lightweight heuristics and may miss unusual syntax.  The outline follows the cursor, and short outlines leave more room for files.  `M-G` focuses the outline directly; contextual footer hints show how to reach it and switch between the file tree and outline.  See Keyboard Help for navigation and resizing.
- **Image attachments**: attach PNG, JPEG, GIF, and WebP images alongside documents or in attachment-only reviews.  Preview them inline and in the lightbox on supported terminals, or read their format and dimensions otherwise.  Saved images survive source changes and history exports.  The limit is 5 MiB and 16 megapixels; GIFs preview their first frame, and animated WebP is unsupported.

### Changed

- Read at full width with `V`, which hides the sidebar and folds comments, then restores the previous expansion setting when leaving View.  `O` opens all comments or returns to the default display, which folds resolved threads.  These replace the former `h`/`H` controls; see Keyboard Help.
- Open the image, link, or reference on the current line with `o`; `M-o` remains available.  Focused Resolve checkboxes are bold and accept Space as well as `r`.  The search shortcut now appears with footer hints.
- Image placeholders show the format and original dimensions when previews are unavailable or hidden, including Before/After details.  Animated GIFs are identified.

### Fixed

- Markdown image previews remain available when browsing documents with many large images.  Offscreen previews release terminal image storage and reload on return, keeping their layout space.
- Attached source files keep their syntax highlighting.  Highlighting and outlines share filename-pattern and shebang detection, including plain `env` and `env -S`, for scripts without language suffixes.  Source code, including comments, leaves URLs and review references as source text; text documents and review comments retain links.
- Empty Markdown link and image labels show their destination instead of disappearing or leaving only an image marker.  Image references to other file types, such as videos, open in the default external application.
- Markdown tables retain links, emphasis, and inline code.  Short columns keep their natural widths, long cells wrap at 40 display columns, and wide tables scroll horizontally.  Search reveals hidden matches; copying retains Markdown source across wrapped and scrolled cells.
- Incremental search highlights visible labels or text when matches fall in hidden Markdown URLs, titles, or formatting.  Multiline matches follow source order and retain text styles and links.
- Codex review workflows can fall back to the user's usual terminal window when multiplexer binding is unavailable.  Desktop launches no longer require a terminal marker.

## 1.6.0

### Added

- **Per-project Session Management**: find and continue saved reviews with `critty sessions`.  Reviews from linked Git worktrees appear together, with status and local start time.  Humans and agents can list, resume, stop, or remove sessions through the same command group.
  - Resume from another checkout of the same project, even after the original worktree is removed.  Git reviews use the current checkout; comments, replies, attachments, and answers carry forward.  Changed inputs require fresh confirmations and consents.
  - Saved text remains readable when a document is missing or a file is outside the current changes.  Saved plans and supplied diffs retain their snapshots; unavailable binary data cannot be reconstructed.  Older sessions need the original checkout or its Git registration to identify the project.  Existing `--session`, `stop`, and `clear` forms remain supported.  See the [session reference](https://github.com/crittylabs/critty/blob/main/internal/cli/skill/critty-cli/SKILL.md#targeting-a-session-or-plan).
- **Live Review Updates**: use Reload to refresh Git changes, documents, and file-backed attachments without reopening the review, or enable Watch to follow edits automatically.  Updated background tabs show an unread dot until opened; updates to the visible tab briefly show a notice.
  - Watch defaults off, and agents can enable it with `--watch`.  Updates wait during editing, selection, search, and dialogs.  Supplied diffs, saved plans, and committed comparisons keep their snapshots; file-backed attachments can still refresh.  See the [live source reference](https://github.com/crittylabs/critty/blob/main/internal/cli/skill/critty-cli/SKILL.md#live-sources).
- **Text Selection and Copying**: drag across a document or code diff, then press `M-w` or click Copy text to reuse the source text.  Copies exclude gutters and annotations, join wrapped rows, and preserve Markdown formatting and link targets, including partial link labels.  Text goes to the kill ring and system clipboard; system clipboard access requires terminal OSC 52 support.  Without a selection, `M-w` still copies a reference.
- **Clickable References in Attachments**: follow file references such as `@src/main.go L64` and comment IDs by clicking them or pressing `M-o` on their line, including in form labels.  File references select an existing review tab or offer to open the file in an editor; comment references select the thread, including resolved comments.
- Review a single commit with `--scope=COMMIT`, such as `--scope=HEAD`, in `critty review` or `critty view`.  Merge commits compare against their first parent; root commits show all files as additions.
- Herdr, Zellij, and cmux users can find their host's default pane and tab controls in Keyboard Help, including in read-only views.

### Changed

- Incremental search covers all review files by default.  Use `M-s m` or the Files button to search only the current file; the choice lasts for the current run.  Selecting lines confines search to their file.
- Send comments or reject a review without warnings or extra confirmation for unanswered form items and unresolved threads.  Approval retains its completion checks and confirmations.
- The Files tab highlights the current file across the full row, including when scrolling long filenames or reading in the content pane.
- Copying a reference shows a brief confirmation before restoring the footer hints.

### Fixed

- Word-level highlights exclude matching spaces and tabs around changed words or phrases, while keeping each changed phrase continuous.  Changes to whitespace itself remain highlighted.
- Keyboard Help hides its scroll controls when all shortcuts fit the window, including the shorter list in `critty view`.

## 1.5.1

### Added

- cmux and Zellij users can open reviews beside their agent and return to the invoking terminal afterward.  cmux opens a terminal tab in the invoking pane; Zellij 0.45+ opens a floating pane in the invoking tab.  The default launcher detects both; agents can select `--launcher cmux` or `--launcher zellij`, reuse their context with `critty env`, and locate them through terminal markers.  Zellij detection requires its inherited session and pane variables.
- Rio users can open reviews in a dedicated window with `--launcher rio-window` on macOS and Linux, or from Critty running in WSL using Windows Rio.  The default desktop launcher prefers Rio when client terminal hints identify it; `doctor` checks its availability.  See the [launcher reference](https://github.com/crittylabs/critty/blob/main/internal/cli/skill/critty-cli/SKILL.md#review-launchers) for requirements.

### Changed

- Review setup guidance shares common multiplexer behavior and keeps terminal-specific requirements in the launcher reference.
- Focused dialog actions stand out more clearly, with stronger blue backgrounds in both color schemes and a clearer red in light terminals.

### Fixed

- Shortcuts using Alt with shifted punctuation work in terminals that report Shift separately: `M-?` opens and closes Help, and `M-<` / `M->` move to the beginning or end of comment and answer text.

## 1.5.0

### Added

- Review binary plists and text in other encodings by listing Git diff driver names or glob patterns in Critty's `textconv_drivers` setting; `["*"]` enables all configured converters.  Conversions are disabled by default, preserving inline image previews.  Comments refer to the converted UTF-8 text; review and comment JSON records the driver and command so agents can locate and edit the original content.  See the [CLI reference](https://github.com/crittylabs/critty/blob/main/internal/cli/skill/critty-cli/SKILL.md#git-text-conversions).
- Word-level highlights now estimate line alignment when changes increase or decrease the line count.  Side-by-side diffs use the same matches, leaving unmatched additions and deletions on their own side.  Very long lines and expensive searches retain full-line highlighting to keep review startup responsive.

### Changed

- **Agent skills and AI collaboration** clarify how agents act on your answers, permissions, and review feedback:
  - [critty-ai-review](https://github.com/crittylabs/critty/blob/main/internal/cli/skill/critty-ai-review/SKILL.md) and [critty-ai-cowork](https://github.com/crittylabs/critty/blob/main/internal/cli/skill/critty-ai-cowork/SKILL.md) keep reviewers focused on static findings and missing evidence.  Your agent owns edits, tests, measurements, and reproductions, including requests between Claude and Codex.
  - Cowork requires both agents to agree on each finding's disposition and the reviewer to resolve satisfied findings.  Substantiated disagreements remain open for your decision.
  - Naming a reviewer in a human Critty request authorizes necessary review material and follow-up rounds.  Host permission checks remain separate.
  - Claude Code can return findings using read/search tools without Python.  `doctor` clarifies the optional posting gate's requirements; the Codex guide covers requests from Claude, result collection, and follow-up rounds.
  - [critty-approval](https://github.com/crittylabs/critty/blob/main/internal/cli/skill/critty-approval/SKILL.md) keeps preference answers in stable fields, separate from execution permissions.  Changed execution requests require renewed consent.
  - Ordinary Markdown checklists remain display-only.  Agents add a `<!-- form: ... -->` annotation to each interactive item, including simple checkboxes, to collect your answer.  See [attachment forms](https://github.com/crittylabs/critty/blob/main/internal/cli/skill/critty-cli/references/forms.md).

### Fixed

- README videos now play for signed-in GitHub readers.

## 1.4.0

### Added

- **Design interviews**: use [critty-grill](https://github.com/crittylabs/critty/blob/main/internal/cli/skill/critty-grill/SKILL.md) to stress-test an idea or document with your agent.  Choose decision summaries, glossaries, ADRs, or a combination, and agree on their destinations and languages before saving.
- **New attachment form widgets** let AI agents ask for your preferences and collect your answers in attached documents.  See [attachment forms](https://github.com/crittylabs/critty/blob/main/internal/cli/skill/critty-cli/references/forms.md).
  - **Select-all checkboxes**: select or clear related items together, such as every check in a section or every recommended answer.  Mixed selections are shown, and each affected item records the human answer.
  - **Text and path inputs**: collect written answers and file or directory paths, on their own or alongside choices and checkboxes.  Path fields offer completion and optional defaults; require an existing path when needed.  Only selected inputs are exported.
  - **Rankings**: reorder list items with the keyboard or drag handle, for example to decide task order or priorities.  Comments stay attached to their original items.
- Review your selections and written answers before submitting with the new **Answers** list.  Fields are grouped by section; filter unanswered items or jump to a field to complete or revise it.  Exported history archives also include form answers and their images.
- The CLI option `--answers` lets agents open questionnaires with **Confirm answers** selected in Submit.  Submitting that choice returns answers to the agent without approving the reviewed design; the JSON result records `"verdict": "comment"` and `"approved": false`.
- Create private scratch directories for review documents and results with `critty tmp create`, including outside Git repositories.

### Changed

- Use `critty view` to inspect changes, documents, patches, and attachments without starting or saving a review.  It replaces `--dry-run`; search and display controls remain available in the current terminal.
- Dialogs make the primary action easier to find, with blue buttons at the right and red warnings for Abort and destructive actions.  Submit starts on the verdict choices: choose a verdict and press Enter, or Shift-Tab to write a comment and save it to return to the verdict.  See in-app Help for controls.
- The Images control highlights its `I` shortcut like Split highlights `S`.

### Fixed

- Word-level diff highlights remain visible inside Markdown code spans.
- Markdown code spans, bold, italic, and inline links now continue across source lines in documents and diffs.  Formatting is limited to a paragraph and stops at blank lines or other block boundaries to keep rendering responsive.  Source lines and comment locations are preserved; incomplete patch context stays literal.
- `critty doctor` no longer treats an unset Herdr image setting as disabled; Herdr enables image rendering by default.

## 1.3.2

### Changed

- File application requests put destinations, creations, updates, deletions, and the consent checkbox before the machine-readable details.  Exact file states remain available as JSON at the end of the request.

- Bundled skills use explicit review commands that match their narrow tool grants.  AI review and cowork reuse task authorization for in-scope revisions, distinguish each assignment from host execution permission, and ask only for missing decisions or expanded scope. Requests to have another AI review and finish the work select cowork; requests to hear its opinion first return findings for the human to decide.

### Fixed

- `critty doctor` reports optional tfil installation and detected process ancestry together as information.  Missing or uninspectable ancestry no longer appears as a separate session check suggesting setup changes; marker tests remain the check for terminal binding.

## 1.3.1

### Fixed

- OpenCode’s `/critty` command now links to the installed supporting skills and review references.  `critty doctor` recognizes installations with these corrected links as current.

## 1.3.0

### Added

- **History management** — group reviews by task with `critty history` and the bundled [critty-history skill](https://github.com/crittylabs/critty/blob/main/internal/cli/skill/critty-history/SKILL.md).  Save portable Markdown archives with attachments and images, inspect task storage, and defer maintenance across conversations.  Reviews are retained by default.  Automatic retention rotates eligible completed backups within a per-directory disk budget, while protecting ongoing reviews and explicit archives.  Ordinary reviews report history warnings; `critty doctor` provides fresh checks and remedies.  See [history settings](https://github.com/crittylabs/critty/blob/main/docs/configuration.md#history-settings).

- **Review before editing** — review proposed updates to notes, reports, configuration files, and other text outside Git while the originals stay untouched.  Use `critty tmp prepare`, `snapshot`, and `apply` to create, split, rename, or delete files through reviewed copies.  Originals stay unchanged until approval and explicit application consent.  Concurrent-edit checks, backups, and saved progress support safe retries; application is atomic per file, not across the batch.  See the [workflow](https://github.com/crittylabs/critty/blob/main/internal/cli/skill/critty/references/temporary-tree.md).

- **Side-by-side diffs** — compare code in unified or side-by-side diffs.  Switch with the `Split: off/on` header control or `S`, and set `split = true` to start with two columns.  Both views preserve comment anchors and support supplied patches without original files; narrow panes temporarily use unified rendering.  See [configuration](https://github.com/crittylabs/critty/blob/main/docs/configuration.md).

- Check Git prerequisites and local marker delivery with `critty doctor`.  It reports missing Git or versions below 2.31.0 and tests marker delivery in a foreground terminal without launching an agent.  Unsupported terminal contexts are skipped; standalone documents and attachment-only reviews remain available without Git.  See [setup](https://github.com/crittylabs/critty#check-prerequisites).

- Inspect changes, documents, patches, and attachments with `critty --dry-run` in the current terminal.  Search and display controls remain available without saving a review or history, migrating configuration, or allowing edits and external applications.

### Changed

- User settings now use TOML: `$XDG_CONFIG_HOME/critty/config.toml` globally and `.critty.toml` in projects.  Existing JSON settings are converted once when the TOML destination is absent; remove or replace JSON `null` values before migration.  Review records and JSON command output keep their formats.  See [configuration](https://github.com/crittylabs/critty/blob/main/docs/configuration.md).

- Local state, configuration, custom prompts, and launcher variables use Critty names.  Startup migrates old paths only when the corresponding Critty destination is absent, without merging directories or retaining compatibility aliases.  Update launchers to use `CRITTY_HOST`.

- Approval requests use a short description and ordinary approval for a single action.  The [approval skill](https://github.com/crittylabs/critty/blob/main/internal/cli/skill/critty-approval/SKILL.md) covers publication details and separate permissions or choices.  AI review and cowork skills carry authorization across covered task revisions while tying each review to its exact snapshot; new services, broader access, and publication require separate authorization.

### Fixed

- Paste text with `C-v` in comment and reply editors.  Prefer plain text on Linux and macOS, with HTML text extraction when needed and an additional RTF fallback on macOS.  Image paste remains available.

- Replaced attachments appear in collapsed, read-only history after current attachments.  Their checkboxes are inactive, and answers belong to current forms.

- Markdown formatting and link click targets survive wrapped rows and deleted lines.  Links in attachment choices open without changing the answer; use the checkbox or surrounding space to select it.

- `critty review --help` explains desktop terminal launchers outside a multiplexer, replacing the removed `auto` launcher description.

## 1.2.0

### Added

- Inspect local images in a lightbox from documents, comments, and image diffs.  Click a preview or press `M-o` to open it, zoom with the keyboard or mouse wheel, and use the arrow keys to browse images on the same line or labeled Before/After revisions.  Comments show image previews while you write.  `M-o` also opens Markdown links, HTML links, and plain web URLs with the system browser or viewer; external applications do not open over SSH.  See [image previews](https://github.com/crittylabs/critty#image-previews).

- Open reviews in a floating pane on capable tmux 3.8+ servers, a split on older servers, or a desktop terminal window without a multiplexer.  Choose built-in or custom launchers with `--launcher` or configuration.  Desktop launchers support macOS, Linux with `xdg-terminal-exec`, and WSL with Windows Terminal; they skip known SSH connections.  See [review launchers](https://github.com/crittylabs/critty/blob/main/internal/cli/skill/critty-cli/SKILL.md#review-launchers).

- Review changes and approval requests from Antigravity CLI (`agy`) with `critty install agy` (or `antigravity`), using the ordinary `/critty` and `/critty-approval` skill names.  `install all` includes Antigravity; legacy Gemini CLI remains available explicitly.  Restricted AI reviewer launches continue to support Claude Code and Codex.  See [Antigravity setup](https://github.com/crittylabs/critty#antigravity-cli).

- Install user integrations from any directory with `critty install --user <target>`.  Codex skills now use `.codex/skills` in projects and `~/.codex/skills` for users.  Choose `install agents` for shared skills in `.agents/skills` or `~/.agents/skills`.  Existing copies in old locations remain in place; remove obsolete copies to avoid duplicate skill names.  See [agent skill installation](https://github.com/crittylabs/critty#agent-skills).

### Changed

- `critty --version` shows a colored brand mark on terminals, and Help opens in the pager selected by `PAGER` (`less` by default).  Colors adapt to the terminal background.  Redirected output, `NO_COLOR`, and agent tools retain plain output.

- The Submit dialog starts on one Approve/Comment/Reject choice group, with keyboard navigation between choices, the overall comment, and action buttons.  Bracket navigation continues through the dialog and wraps between matching threads and form items.

- `critty doctor` checks supported desktop launchers without opening windows, identifies Critty and tfil Codex wrappers, and can test terminal marker binding and notifications.  Its paged report groups checks by area, highlights status and advice, wraps to the terminal width, and ends with a summary.

### Fixed

- Delayed image capability replies and mouse reports no longer spill into text fields when sequences arrive in separate reads, including tmux extended-key input.  Image detection starts only when needed or explicitly requested, retries when the review pane becomes visible, and rechecks unconfirmed support on image tabs.  Ordinary typing no longer triggers detection.  The Images control shows `N/A` while previews are inactive.

## 1.1.3

### Fixed

- Files containing a bare carriage return, such as shell scripts, no longer show the lines after it shifted against their line numbers and diff markers.  Carriage returns and other control characters in source lines now appear in caret notation such as `^M`.

## 1.1.2

### Changed

- Diagnose installed integrations, AI review Python/MCP prerequisites, and image forwarding settings with `critty doctor`; `critty check` remains an alias.  Partial installations report missing resources, `--python` selects a dedicated review environment, and color-coded text or `--json` reports present the results.  tmux pane settings and local Herdr configuration include advice without changing stored settings.  On macOS and Linux, image support is probed on the current foreground TTY; an unavailable TTY reports a skipped check without an error.

- AI review and collaboration skills are now named `critty-ai-review` and `critty-ai-cowork`, matching the other Critty skills.  Reinstall with `critty install --force <agent>` to update references and automatically remove recognized old `crit-ai-review` and `crit-ai-cowork` skill folders after installing their replacements.  See the [upgrade instructions](https://github.com/crittylabs/critty#ai-review-and-collaboration-experimental).

- Checkboxes and choices in attachments stand out with colored badges and highlighted labels.  Submitting asks for confirmation when an unanswered item has never scrolled into view, so items below the visible area are not missed.  You can review the item, submit anyway, or cancel.

- Review data now uses the `critty` state directory, with a compatibility link preserving old paths and running sessions.  If the directories conflict or cannot be linked, Critty keeps using the legacy directory.  Saved reviews use `critty_state`, with the old `tcrit_state` key retained for compatibility.  Review launchers use `CRITTY_HOST`, with `TCRIT_HOST` supported as a fallback.

### Fixed

- Improved readability on light terminal backgrounds.  Text that used pale ANSI colors, such as cursor markers, item counts, warnings, attachment names, Markdown code, links, and headings, now uses darker shades.  Code is highlighted with a light color scheme, line numbers are a subdued gray, and button keys are white on dark yellow.  Item counts in dialogs share the gray background of the header counts.

## 1.1.1

### Fixed

- Improved detection of inline image support, which could incorrectly report images as unavailable on supported terminals, especially through tmux.  Detection handles tmux's extended-key input and retries unanswered startup queries.  If support remains unconfirmed, the first keypress checks again after the terminal becomes visible.  Press `I` or click Images to restart an unsuccessful or ongoing check as often as needed.  The Images control shows when a check is in progress.

## 1.1.0

### Features

- **Experimental AI review:** Get second opinions with `crit-ai-review`, or refine work with another AI using `crit-ai-cowork` before final human review.  Both require an existing Critty session and authorized reviewer access.  Reviewers post findings and manage their own threads; your agent makes edits and runs checks.  All agent install targets include these skills, with restricted CLI setups for Claude Code (Python 3.10+) and Codex (Python 3.11+ with the MCP SDK).  See the [README](https://github.com/crittylabs/critty#ai-review-and-collaboration-experimental) for usage and setup.

- Navigate attachment checkboxes and choices alongside review threads.  `[` / `]` visit every thread and form item; `{` / `}` visit unresolved threads and unanswered required items.  `n` / `N` visit diff hunks, unresolved threads, unanswered required items, and optional form items.  A required choice group has one unresolved stop until answered.  See the [navigation table](https://github.com/crittylabs/critty#review-navigation).

- Added actionable counters to the main screen and decision dialogs to show item counts and completion status.  Click a counter to jump to an item.  Tab titles flag unresolved threads and unanswered checkboxes and choices instead of underlining filenames.  Prominent messages and colors highlight unresolved threads and unanswered items to help prevent omissions.  Required answers block approval; optional answers remain optional, and unchecked consents stay unauthorized.

- Switch to the previous or next file tab with `M-{` or `M-}`, alongside `S-TAB` and `TAB`.

- Export verdicts, feedback, and attachment identities as compact JSON with `--summary FILE`.  Bundled skills use it to read results and retain original human feedback for host permission checks.

### Changed

- Configuration uses `~/.config/critty/config.json` and `.critty.config.json`, falling back to the legacy TCrit file only when the new file is absent at that level.  Custom prompts use the corresponding `critty/prompts/` and `.critty/prompts/` directories, with TCrit paths as fallbacks.  Project settings still override global settings, and review storage stays unchanged.

- **Resolve** and **Images** controls underline their shortcut keys, `r` and `I`; the Images button uses less footer space.

## 1.0.0

Critty 1.0 brings code and document reviews, approvals, and user choices into one terminal session.  Review proposed changes alongside an approval checklist or a choice of next steps, or use the same workflow for a standalone decision.  Your agent receives both your review feedback and explicit answers.

### Features

- Submit reviews with Approve, Reject, or Comment and an optional overall comment thread, as on GitHub.  Individual threads' resolution state informs the suggested decision, and approval asks before resolving remaining threads.

- Preview images referenced by relative paths in Markdown documents, image files in diff reviews, and images pasted into comments and replies.  PNG, JPEG, GIF, and WebP are supported.

- Image previews require a terminal that implements the [Kitty graphics protocol](https://sw.kovidgoyal.net/kitty/graphics-protocol/) with Unicode image placeholders, such as Kitty, Ghostty, and iTerm2.  Within these terminals, previews work through tmux or Herdr with passthrough enabled.  Press `I` or click the Images button to toggle image previews.

- Incremental search supports regular expressions, smart case, Unicode word search, and searching across files.  Matching text is highlighted as you type.  Keyboard and clickable controls let you change search modes, move between matches across files, keep the result, or cancel.

- Search supports pasted text, the shared kill ring, and multilingual IME input.  Long queries keep their input end visible.  Controls stay available in short terminals and wrap after resizing.

### Changed

- `n` / `N` also stop on whole-file reviews, attachments, and files whose diffs have disappeared, even without unresolved comments.

- Buttons without a background now use the same subtle gray background as the Submit dialog, including close, comment navigation, and resolution controls.

- Keyboard Help shows the running build's version beside a colored logo.  Help and the review banner share the logo, with a terminal prompt and green accents.  Help keeps its scroll controls on the bottom row, beside the version when space allows.

- Building from source requires Go 1.27.2 or later.

- The bundled `critty`, `critty-cli`, and `critty-approval` skills cover interactive reviews, CLI operations, and approval requests, with input and display rules in the CLI reference.

- Keyboard Help moves to `M-?`, leaving `?` for incremental search.

- Updated Keyboard Help includes the search-mode shortcuts and scrolls with keys, the mouse wheel, and Up/Down buttons when it does not fit the terminal.

### Fixes

- Dialog buttons remain clickable when submission or clipboard status messages wrap.

- `critty env` and `critty env tty` report output write failures instead of returning success.

- Unresolved drifted comments show their threads by default.  Resolved threads expand while focused with `[`, `]`, or the arrow keys, without requiring Enter.

- TUI saves preserve comments and replies added or changed by concurrent CLI commands.  Conflicting edits to the same field report a save error instead of overwriting another writer's changes.

- Demo recordings select the build prepared by `demo/setup.sh`, even when a tmux pane's login shell changes PATH.

- Bundled skills refresh the selected review input after edits, distinguish ordinary approval from interactive forms, and preserve complete output for manually launched reviews.  Comment examples consistently target a session and handle bodies that need JSON input.

## 0.18.1

### Features

- The bundled `critty-approval` skill prepares Markdown approval attachments with YAML frontmatter and interactive confirmations, choices, and consents, then ties execution to the exact human-approved result.  All four agent integrations install it.

- `--null` reviews only attached documents without requiring a Git repository.

### Fixes

- Empty diff input, including `--diff=/dev/null`, can be reviewed with attachments and resumed from its saved session.

- Markdown quotes and italic text use the primary text color to stay readable on diff backgrounds.
- `n` and `N` stop at added, deleted, and modified binary files even though they have no diff hunks.
- `demo/release.sh` can upload recordings to a draft release before its tag exists.

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

- **Codex launch wrappers** — `tcrit codex` and the bundled `codex` binary preserve tmux and Herdr context through per-thread shell settings.  [Codex 0.157.0](https://github.com/openai/codex/releases/tag/rust-v0.157.0) enabled automatic background-server startup by default, separating shell tools from the terminal process ancestry TCrit used for detection.  The wrappers start and connect to the local daemon when supported, pass through older CLIs, and chain with forward-searching PATH wrappers such as tfil.  Explicit local `--remote` reconnects preserve the selected Unix socket and terminal context.
- When terminal detection fails, suggest correcting PATH order if another `codex` precedes TCrit's bundled wrapper, including `[tools]` ordering for mise.

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
