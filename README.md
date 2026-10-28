# Critty

<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/critty-logo-dark-boxed.svg">
    <img src="assets/critty-logo-light-boxed.svg" alt="critty" width="400">
  </picture><br>
  <a href="https://github.com/crittylabs/critty/releases/latest"><img src="https://img.shields.io/github/v/release/crittylabs/critty" alt="Latest release"></a>
  <a href="https://github.com/crittylabs/critty/actions/workflows/ci.yml"><img src="https://github.com/crittylabs/critty/actions/workflows/ci.yml/badge.svg?branch=main" alt="CI status"></a>
</p>

Review changes, refine documents, make choices, and approve actions with your agent in the terminal.

You can review a diff, file, or document on its own.  Your agent can also attach a document asking for your permission or a decision.  You can agree in advance on which actions need your approval, such as pushing code or deploying.

When your agent needs your input, it prepares the request and opens a review in Critty.  Comment on individual lines, answer questions, confirm details, or authorize actions.  Your comments and answers go back to the agent.

If you chat with your agent in a terminal multiplexer, such as tmux, Herdr, or cmux, the review opens in a pane or tab alongside the chat.  Review the request, send your feedback, and return to the conversation while your agent handles the next step.  This works with both TUI and GUI multiplexers; without one, Critty opens a new terminal emulator window when available.  Local desktop reviews prefer terminals with image previews and try other available terminals if needed.  A failed attempt to identify the invoking pane can still fall back to a desktop or custom launcher; your configured launcher order takes precedence.  See the [supported launchers and requirements](internal/cli/skill/critty-cli/references/terminals.md#review-launchers) for the full list.

Work through a questionnaire: check related items together, write answers, choose paths, and rank priorities.  Check the **Answers** list before sending your choices back to the agent.

https://github.com/user-attachments/assets/8bce362f-f53f-4711-b6d3-47cf93df0c84

For example:

- Review code changes alongside screenshots and questions from your agent.
- Leave inline feedback on a plan or document.
- Review updates to notes, reports, or configuration files outside Git before applying them.
- Choose an implementation approach before coding starts.
- Check a proposed PR's text and destination before authorizing publication.

[More videos and screenshots](#gallery).

> **critty** is the new name of crit, originally published as `kevindutra/crit`, with the fixes and improvements developed as TCrit; see [CHANGELOG.md](CHANGELOG.md) for every change since crit 0.2.2.

## Key Features

- **Code and document reviews** — read multi-file diffs in unified or side-by-side view, with syntax highlighting and inline comments.
- **Approvals and questionnaires** — confirm details, allow actions, choose alternatives, write answers, select paths, and rank options in attached documents.  These requests can accompany a code review or stand on their own.
- **Design interviews** — work through questions with your agent using [critty-grill](internal/cli/skill/critty-grill/SKILL.md), then save an agreed decision summary, glossary, or ADR.
- **Round trips with your agent** — send feedback and answers, let the agent act on them, and return for the next round.  Integrations support Claude Code, Codex, OpenCode, and Antigravity CLI; legacy Gemini CLI is also available.
- **Flexible review inputs** — review Git changes, a single commit, commit ranges, supplied diffs, or versioned plans.  Use `critty view` for a read-only inspection in the current terminal.
- **Live Review Updates** — refresh an open review with Reload, or enable Watch to follow edits.  Unread dots mark files that changed while you were reading elsewhere.
- **Per-project Session Management** — find saved reviews across a project's worktrees and continue from another checkout, even after the original worktree is removed.
- **Plists and other encodings** — review binary plists and encoded text through your configured Git text converters.  Enable the converters you need in Critty's configuration.
- **Review before editing** — review proposed changes to documents and configuration files outside Git while the originals stay untouched.  Create, split, rename, or delete files after approval.
- **Inline image previews** — compare image changes, attach images alongside review documents, preview images while writing comments, and inspect local images in a zoomable lightbox.  [See an image review](#reviewing-an-image-replacement).
- **History management** — group reviews by task, retain feedback, and export portable archives with attachments and images.  See the [history workflow](internal/cli/skill/critty-history/SKILL.md).
- **Source outlines** — jump to Markdown headings and declarations in code or attached source files from the Files sidebar.
- **Navigation and editing** — search across files as you type, follow file and comment references, and return to each file where you left off.  Compact tabs leave more room for the document.
- **Text Selection and Copying** — select text from documents and diffs to reuse it, preserving Markdown formatting and link targets.
- **Mouse and keyboard** — use vim-style keys or clickable tabs, comments, and controls.
- **Scriptable and Crit-compatible** — automate comments with a [Crit](https://crit.md/)-compatible review format and resume saved sessions.

## Install

Install the binary with [mise](https://mise.jdx.dev/):

```bash
mise install github:crittylabs/critty
```

Or with Go, with `$GOPATH/bin` on your `PATH`:

```bash
go install github.com/crittylabs/critty/cmd/critty@latest
```

Release binaries are available for macOS and Linux on amd64 and arm64.  Building from source requires Go 1.27.2 or later.

### Check prerequisites

Git 2.31.0 or later is required for repository reviews and the `critty tmp` workflow for applying changes outside Git.  Standalone documents, attachment-only reviews, and scratch directories created with `critty tmp create` can run without Git.  Prefer a maintained Git package with current security fixes.

After installing the binary, run:

```bash
critty doctor
```

Run it in the terminal where you plan to use Critty.  It checks installed agent integrations, review launchers, image settings, and Python/MCP prerequisites for optional AI reviews.  It also tests a local marker round trip with a three-second wait, without launching an agent; unsupported terminal contexts are reported as skipped.  Follow its suggestions for the features you plan to use, then rerun it after setup.

### Agent skills

<a id="antigravity-cli"></a>

Install the skills for your agent.  The table shows how to start a review; use `critty-approval` for approvals and choices:

| Agent | Install for your user | Start a review |
| --- | --- | --- |
| Claude Code | `critty install --user claude-code` | `/critty` |
| Codex | `critty install --user codex` | `$critty` |
| OpenCode | `critty install --user opencode` | `/critty` |
| Antigravity CLI | `critty install --user agy` | `/critty` |
| Gemini CLI | `critty install --user gemini` | Ask it to use the `critty` skill |

The `agy` target also accepts `antigravity`.  Omit `--user` to install for the current project.  Use `critty install agents` for shared skills, or `critty install all` for all agent-specific integrations except legacy Gemini CLI.  Choose shared skills or agent-specific variants to avoid duplicate skill names.

Rerun the installer with `--force` after upgrading to refresh the bundled skills and commands.  For Antigravity CLI, run `critty install --user --force agy`; v1.8.1 splits detailed instructions into linked reference files so the skills fit its size limit.  In v1.2.0, Codex skills use `.codex/skills` in projects and `~/.codex/skills` for users; remove obsolete copies from `.agents/skills` if needed.  When migrating from TCrit, use the new binary and skill names; see the [changelog](CHANGELOG.md) for migration details.

After installing or upgrading the skills, run `critty doctor` from your project to check for missing or outdated integration files.

In v1.3.0, user settings move to `~/.config/critty/config.toml` and project `.critty.toml`.  Startup converts older JSON settings when the TOML destination is absent.  Local state and prompt paths also use Critty names; update custom launchers to use `CRITTY_HOST`.  See [configuration](docs/configuration.md) and the [release notes](CHANGELOG.md#130) for migration details.

### Terminals and multiplexers

In tmux, floating panes require version 3.8 or later; older servers use split panes.  Desktop terminal windows are not opened over SSH, so use a supported multiplexer for agent reviews, or run Critty directly.

After setting up or switching your terminal or multiplexer, run `critty doctor` there to check review launchers and local image forwarding settings.  See the [launcher reference](internal/cli/skill/critty-cli/references/terminals.md#review-launchers) for configuration and supported terminals.

#### Image previews

Image previews need a terminal with Kitty graphics protocol support and Unicode image placeholders, such as Kitty, Ghostty, or iTerm2.  Through tmux or Herdr, enable image passthrough; for tmux, run `tmux set -g allow-passthrough on`.

After enabling passthrough, run `critty doctor` in the foreground of that terminal to probe image support and check forwarding settings.  Herdr checks read local configuration; running server and remote client settings may differ.

PNG, JPEG, GIF, and WebP are supported.  Large or unsupported images and changed images in supplied diffs remain text placeholders; remote image URLs are not fetched.

Attach images alongside review documents with `--attachment image.png`, including in attachment-only reviews.  Images are saved with the review, so resuming keeps the reviewed version even if the original changes or disappears.  Supported terminals show the image inline and in the lightbox; other terminals show its format and dimensions.  Image attachments are limited to 5 MiB and 16 megapixels.  Animated GIFs display their first frame.

<a id="codex-with-a-background-server"></a>

### Using Codex in a multiplexer

When using Codex in a supported multiplexer, start it through Critty so reviews can find the correct terminal:

```bash
critty codex
```

Alternatively, put the bundled `codex` wrapper ahead of Codex on your `PATH`.  Run `critty doctor` to inspect wrapper and terminal-filter detection.  For an end-to-end binding test, use the [marker-based doctor check](internal/cli/skill/critty-cli/references/terminals.md#environment-diagnostics).  See [terminal-filter integration](docs/terminal-filter.md) for setup.

## Basic workflow

### Review code and documents

Ask your agent to use the `critty` skill to review its changes, a plan, or a document.  For example:

> Use critty to review these changes before committing.

Critty opens a review pane or tab in the detected multiplexer, or a new window of a supported terminal emulator.  Read the diff or document, leave inline comments on the relevant lines, and submit Approve, Reject, or Comment.

The agent reads your verdict and comments, makes revisions, and reopens the same session.  You can read its replies and the updated changes, add feedback, and continue until the work is ready.

https://github.com/user-attachments/assets/8afbb65f-e001-4a67-ab34-7c3afac65a4e

For a manual review:

```bash
critty review                 # Current Git changes
critty review --staged        # Staged changes
critty review --scope=HEAD    # One commit
critty review notes.md        # A document
critty plan design.md         # A versioned plan
```

<a id="review-navigation"></a>

Reviews support mouse and keyboard navigation, search, inline comments, and your preferred editor.  Progress indicators highlight unresolved feedback and unanswered items.  Use the in-app Help for controls and `critty --help` for commands.  The [CLI reference](internal/cli/skill/critty-cli/SKILL.md) covers review inputs, attachments, saved sessions, configuration, and scripted comments.

Compact tabs leave two more rows for the document.  Each tab remembers your reading position in the document and Comments sidebar.  The Files tree shows pending comments and unanswered items.  Use `V` for full-width reading with all comments reduced to bubble markers.

Word-level highlights estimate corresponding lines even when a change also adds or removes lines.  Side-by-side view uses the same matches.  Very long lines and expensive searches keep full-line highlighting so reviews stay responsive.

### Per-project Session Management

Find a saved review and continue where you left off:

```bash
critty sessions                  # Reviews for this project, across worktrees
critty sessions --all            # Reviews from every project
critty sessions resume <id>      # Continue a saved review
```

The list shows each review's status and local start time.  Resume from any checkout of the same Git project, even after deleting the original worktree.  Git reviews use the current checkout, while comments, replies, attachments, and answers carry forward.  Changed inputs require fresh confirmations and consents.

Saved text remains available when an original document is missing or a file is outside the current changes.  Plans and supplied diffs keep their saved snapshots; unavailable binary data cannot be restored.  Older sessions need the original checkout or its Git registration to identify the project.  See `critty sessions --help` for sorting, stopping, and removing sessions, and the [session reference](internal/cli/skill/critty-cli/references/sessions.md#targeting-a-session-or-plan) for details.

### Live Review Updates

Keep a review open while files change.  Click **Reload** to refresh the current Git scope, document, and file-backed attachments, or turn on **Watch** to follow edits automatically.  Watch starts off; agents can enable it with `--watch`.  Updates wait while you edit, select text, search, or use a dialog.  Updated background tabs show an unread dot until opened.

Saved plans, supplied diffs, and committed comparisons keep their snapshots.  File-backed attachments can still update.  Changed review inputs require fresh confirmations and consents.  See the [live source reference](internal/cli/skill/critty-cli/references/inputs.md#live-sources) for limits.

### Text Selection and Copying

Drag across text in a document or diff, then press `M-w` or click **Copy text**.  Copies contain the source text, with wrapped rows joined and gutters and annotations left out.  Markdown retains its formatting and link targets, even when selecting part of a link label.  Text goes to the kill ring and system clipboard; system clipboard access requires terminal OSC 52 support.

Without a text selection, `M-w` copies a file reference or comment ID.  Follow these references in comments and attached documents by clicking them.  In an attachment, `M-o` follows a reference on the cursor line.  References such as `@src/main.go L64` select a review tab or offer to open the file in an editor; comment IDs select their thread.

Search covers all review files by default.  Use the **Files** button or `M-s m` during search to restrict it to the current file.  Selecting lines keeps search in that file.

The outline below the Files tree lets you jump to Markdown headings or source declarations, including in attachments.  Source outlines support Go, JS/TS, Rust, Ruby, Python, Swift, Kotlin, C/C++, Objective-C/C++, Java, Zig, Lua, Bourne-family shell scripts, Perl, Emacs Lisp, and VimL.  Known file names and shebangs such as `#!/usr/bin/env ruby` identify scripts without a language suffix for both syntax highlighting and outlines; `env -S` is also supported.  Go uses its standard parser; other languages use lightweight rules for conventionally formatted code and may miss or misidentify unusual syntax.  Entries stay in source order, with nesting where available.  See Keyboard Help for navigation and resizing.

### Review plists and other encodings

Review binary plists and text in other encodings as readable UTF-8 through your existing Git `textconv` drivers.  Enable their names in Critty's global or project TOML configuration, for example:

```toml
textconv_drivers = ["plist", "cp932"]
```

Conversions are disabled by default.  Comments on converted files refer to the converted text and its line numbers.  Agents receive the converter's name and command with the feedback to help locate the original content and preserve its format when editing.  See [configuration](docs/configuration.md) and [Git text conversions](internal/cli/skill/critty-cli/references/inputs.md#git-text-conversions) for driver setup and limitations.

### Review changes outside Git

Use Critty for personal notes, reports, configuration files, and other text files that are not in a repository.  Your agent can propose edits, create files, split a long document, or rename and remove files while keeping the originals unchanged until you approve.

For example:

> Use critty to review updates to my notes before applying them.  Split the long meeting notes into a summary and action items, and keep the originals unchanged until I approve.

The agent prepares copies with `critty tmp`, edits them, and opens the proposed diff with a request to apply it.  Leave comments and review revisions as usual.  After you approve the review and select the permission checkbox, the agent applies the reviewed changes.  Critty checks for changes to the originals and retains backups for recovery.

The application request lists the destination and files to create, update, or delete before the permission checkbox.  Exact file states remain available as JSON at the end.

See the [workflow for changes outside source control](internal/cli/skill/critty/references/temporary-tree.md) and `critty tmp --help` for details.

<a id="approvals-and-choices"></a>

### Review changes and approve the next actions

Agree with your agent on which actions need your approval, such as committing, pushing a branch, or deploying.  Keep your project rules for reviews, approvals, and choices in `CLAUDE.md` or `AGENTS.md`.

When it needs your permission or a decision, the agent can use [critty-approval](internal/cli/skill/critty-approval/SKILL.md) to prepare a document with the details and open it in Critty.  That document can be attached to the same session as the diff, so you can review the changes and next steps together.  It can include the proposed commit message, push destination, or deployment plan.

For example:

> Use critty-approval to review these changes and the proposed push together.  Include the commit message and exact repository and branch destination, and wait for my approval before pushing.

For a deployment, include the target environment and exact deployment actions alongside the changes.  Review the code and proposed operations together, leave feedback where needed, and approve the operations you want the agent to carry out.

Required confirmations must be complete before approval.  Approving the review does not authorize operations whose permission checkboxes remain unchecked.  Critty records your decisions; the agent executes the authorized actions.

You can send Comment or Reject with unfinished form items or unresolved threads without extra confirmation.  Completion checks apply when approving.

Ordinary Markdown checklists keep their written checkmarks and are display-only.  Agents add a `<!-- form: ... -->` annotation to each item that should collect an answer.  Preference answers use separate fields from execution permissions.  Changed execution requests require renewed consent.

### Approvals and choices without a code diff

The agent can also open a request document on its own, without a code diff.  Use the same `critty-approval` skill to choose an approach before implementation starts or approve an action that has no code changes to review:

> Use critty-approval to let me choose between these implementation approaches.
>
> Use critty-approval to show me the PR text and destination before publishing it.

Read the request and supporting material, leave comments, and use the checkboxes or radio buttons to give your answers.  Submit your decision, and the agent continues from your feedback and answers.  The installed skills handle the interaction with Critty.

### Answer questionnaires and refine ideas

Use attachment forms to give written answers, select file or directory paths, rank options, or select related items together.  Open **Answers** to check your selections and written answers before submitting, find unanswered fields, and jump back to revise them.  For questionnaire rounds, your agent can make **Confirm answers** the default Submit action.  Choosing it returns your answers to the agent without approving the whole design.  See [attachment forms](internal/cli/skill/critty-cli/references/forms.md).

Use [critty-grill](internal/cli/skill/critty-grill/SKILL.md) to work through a design with your agent:

> Use critty-grill to stress-test this plan.  Agree on which decisions to record and where to save them.

The agent asks questions through comments and form choices, then confirms the shared understanding before saving a decision summary, glossary, ADRs, or a combination.  You choose the destinations and languages together.

### AI review and collaboration (Experimental)

Request a second opinion with [critty-ai-review](internal/cli/skill/critty-ai-review/SKILL.md), or use [critty-ai-cowork](internal/cli/skill/critty-ai-cowork/SKILL.md) to let your agent address findings with another AI before your final review.  Findings return to the same Critty comment threads.

For example:

> Use critty-ai-review with Codex Astra to review these changes in the current session.

Ask your agent to "Have Fable review this and finish it" to delegate revisions and follow-up through `critty-ai-cowork`.  Say "I'd like Fable's opinion first" to receive findings through `critty-ai-review` before deciding what to change.  The same distinction applies to other reviewers and languages; explicitly naming a skill selects that workflow.

You can make that request in a Critty comment, including a Comment submission.  It authorizes sending the current review's necessary diff, documents, and surrounding source to the chosen model's configured service, with results returned to the local session.  That scope carries through follow-up rounds; separate host permission checks can still require direct confirmation.

Your agent owns edits, tests, measurements, and reproductions.  Claude can request a Codex review, and Codex can request a Claude review; the requesting agent keeps those responsibilities.  The reviewer gathers permitted information and makes static findings.  If it needs more evidence or a read is denied, it tells your agent what is missing and continues with the available material.  Your agent obtains the information or runs the necessary check.

In cowork, both agents agree on how to address each finding, and the reviewer resolves satisfied findings after checking the response.  Remaining disagreements come to you for a decision.

AI review requires an existing Critty session and authorized access to the chosen reviewer.  Claude Code can review with only read and search tools, returning comments for your agent to post.  Direct posting through the Claude gate requires Python 3.10+; the restricted Codex MCP setup requires Python 3.11+ and the MCP Python SDK.  See the linked skills for setup and dependencies.

For a Python gate or MCP setup, run `critty doctor` to check its prerequisites.  If it uses a dedicated Python environment, check that interpreter explicitly:

```bash
critty doctor --python /absolute/path/to/venv/bin/python
```

If you installed the earlier `crit-ai-review` or `crit-ai-cowork` skills, reinstall with `critty install --force <agent>` to update them.

## Gallery

Videos and screenshots captured with v1.7.0 candidates.

### Navigating outlines and image attachments

Jump through source declarations and Markdown headings, then inspect an attached image.  `M-G` focuses the outline; Enter returns to the source.

https://github.com/user-attachments/assets/b647a70b-8206-4402-857b-330ee6a214c6

![Source declarations and their outline in Ghostty](https://github.com/user-attachments/assets/22e5987a-53d3-4f32-91a1-792d610bc3d5)

![An image attached directly to a review in Ghostty](https://github.com/user-attachments/assets/91781b73-e5d8-47a1-be89-3b822c70badb)

### Code review with inline comments

https://github.com/user-attachments/assets/8afbb65f-e001-4a67-ab34-7c3afac65a4e

### Side-by-side code review

![Split diff view in Ghostty with an inline review comment and agent reply](https://github.com/user-attachments/assets/1d93a68b-a4d2-4a0f-91da-f51928cbf018)

### Review round trips with Claude Code

https://github.com/user-attachments/assets/56724c09-7ffa-4521-bfe6-9452be423e4e

### Review round trips with Codex

https://github.com/user-attachments/assets/ecc84f8b-3ab9-4d22-80e7-f1e2e854e992

### Asking Codex Astra for a review from Claude Code

A comment in a Claude Code review requests a second opinion from Codex Astra.  Claude starts the reviewer, and Astra's inline findings and overall report return to the same Critty session.

https://github.com/user-attachments/assets/418bb5b9-e31e-42ab-9abd-075127fe643c

### Following live review updates

Watch follows file edits and marks changed background tabs.  Follow a file reference from an attachment to inspect the updated source.

https://github.com/user-attachments/assets/a01e9427-1963-4771-ac00-b42e6de94900

### Reviewing a document

https://github.com/user-attachments/assets/1b66b800-df61-4ee0-8424-0b39fb68bba7

### Previewing an image while writing a comment

![Image preview beneath a comment draft in Ghostty](https://github.com/user-attachments/assets/3268a1ce-0a8a-4a32-ac7d-39d803673cef)

### Inspecting an attached image in the lightbox

![Lightbox showing a local image from an attached Markdown document](https://github.com/user-attachments/assets/07954969-1d1b-452b-853f-9dd834c57208)

### Reviewing an image replacement

![Before and After image previews in Ghostty](https://github.com/user-attachments/assets/c93ac410-3def-4f3c-a7b5-c72581e7c98c)

### Attached checklist with an inline image

![Attached review checklist with required confirmations, clickable checkboxes, and an inline image](https://github.com/user-attachments/assets/4ae61333-45d5-42b1-9e8b-d698cb0cc9de)

### Answering a questionnaire with form widgets

See the [questionnaire walkthrough above](#critty) for select-all controls, written answers, path completion, rankings, and the Answers list.

![A release questionnaire in Critty with a mixed select-all control, checked and required items, a choice with a note, a text answer, and a reordered ranking](https://github.com/user-attachments/assets/28bd80aa-e323-4198-9c8b-47be3c760708)

### Review progress and pending items

![Actionable counters, tab markers, and a thread on an attached checklist in Ghostty](https://github.com/user-attachments/assets/db36474c-6ebe-4554-af8c-c2e497bd0f21)

### Approval checks for unfinished items

![Approval dialog warning about unresolved threads and unanswered required and optional items](https://github.com/user-attachments/assets/7fab4e15-c81e-4062-9720-e19f3bf718aa)

### Incremental search

![Incremental search finds matches across attached files by default](https://github.com/user-attachments/assets/19fbf244-9aef-47fa-b26f-cd2bdfe46045)

## Contributing

Contributions and ideas are welcome: support for more agent CLIs and multiplexers, terminal compatibility, keyboard and mouse interaction, UI improvements, and workflows that use AI.  [Open an issue](https://github.com/crittylabs/critty/issues) to suggest an improvement or send a [pull request](https://github.com/crittylabs/critty/pulls).

## Development

```bash
go test ./...
go build ./...
go vet ./...
```

Run `python3 e2e/scenarios.py` for isolated CLI and TUI scenarios, and `python3 e2e/doctor_images_test.py` for image diagnostics.  These checks need Python and tmux, but no AI accounts.

See [demo/README.md](demo/README.md) for recording and publishing demos.

## License

[MIT](LICENSE)
