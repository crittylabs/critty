# Critty

<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/critty-logo-dark-boxed.svg">
    <img src="assets/critty-logo-light-boxed.svg" alt="critty" width="400">
  </picture><br>
  <a href="https://github.com/crittylabs/critty/releases/latest"><img src="https://img.shields.io/github/v/release/crittylabs/critty" alt="Latest release"></a>
  <a href="https://github.com/crittylabs/critty/actions/workflows/ci.yml"><img src="https://github.com/crittylabs/critty/actions/workflows/ci.yml/badge.svg?branch=main" alt="CI status"></a>
</p>

> **critty** is the new name of crit, originally published as `kevindutra/crit`, with the fixes and improvements developed as TCrit; see [CHANGELOG.md](CHANGELOG.md) for every change since crit 0.2.2.

TUI for reviewing AI-generated code and plans — built for human-in-the-loop agentic coding workflows.

Read a plan or review code changes across multiple files, leave inline comments, and let your coding agent address the feedback automatically.

<video controls src="https://github.com/user-attachments/assets/0942656f-1192-455c-a29c-6af171c7138d"></video>

[More videos and screenshots](#gallery).

## Key Features

- **Review in the terminal** — read documents and multi-file diffs with syntax highlighting and inline comments.
- **Round trips with your agent** — review, send feedback, and return for the next round.  Integrations support Claude Code, Codex, OpenCode, and Gemini CLI.
- **Flexible review inputs** — review Git changes, commit ranges, supplied diffs, or versioned plans.
- **Attached documents and forms** — review commit messages and approval notes alongside code, with checkboxes and choices for human decisions.
- **Inline image previews** — compare image changes and view local images in Markdown and comments on supported terminals.  [See an image review](#reviewing-an-image-replacement).
- **Navigation and editing** — browse files, search as you type, reply to comments, and edit in your preferred editor.
- **Mouse and keyboard** — use vim-style keys or clickable tabs, comments, and controls.
- **Scriptable and Crit-compatible** — automate comments with a [Crit](https://crit.md/)-compatible review format and resume saved sessions.

### Image previews

PNG, JPEG, GIF, and WebP previews work on supported terminals, including Ghostty, Kitty, and iTerm2.  In tmux, enable forwarding with `tmux set -g allow-passthrough on`.  Unsupported images and terminals keep a text placeholder; supplied diffs remain text-only.

Press `I` or click **Images** to toggle previews.  Markdown and comment previews use local images; remote URLs are not fetched.  [See local images in an attached document](#attached-checklist-with-an-inline-image).

## Install

Install the `critty` binary, then install the review skills for your agent with `critty install`.

Critty was called TCrit until v0.17.0.  When switching from TCrit, install from `crittylabs/critty`, the new repository and Go module path, update scripts to call `critty`, reinstall the bundled skills, and remove the old `tcrit` and `tcrit-cli` skills.  Existing reviews, configuration, and custom prompts keep their TCrit paths, so they remain available.  If tfil reports terminal markers, update it to 0.4.1 or later.

After upgrading to v0.18.1, rerun `critty install` for your agent to add the new `critty-approval` skill and update the review instructions.

### Command-line binary

With [mise](https://mise.jdx.dev/):

```bash
mise install github:crittylabs/critty
```

With Go:

```bash
go install github.com/crittylabs/critty/cmd/critty@latest
```

Make sure `$GOPATH/bin` (defaults to `~/go/bin`) is in your `PATH`:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

### Review header

Use `--project 'My project'` and `--topic 'Task name'` to customize the review header.  Git reviews show the project and branch by default.

### Codex with a background server

> [!WARNING]
> [Codex 0.157.0](https://github.com/openai/codex/releases/tag/rust-v0.157.0) enabled automatic background-server startup by default.  Its shell tools no longer share the invoking terminal's process ancestry, so Critty cannot recover the tmux or Herdr context from that ancestry.  Start Codex through `critty codex` or the bundled wrapper when using tmux or Herdr.

From your tmux pane or Herdr terminal:

```bash
critty codex
critty codex resume
```

Launch Codex through `critty codex`, or put Critty's bundled `codex` wrapper before the original Codex in PATH.  The wrapper records which tmux or Herdr pane Codex runs in, then starts Codex with its arguments and environment unchanged.  `go install github.com/crittylabs/critty/cmd/...@latest` installs both binaries; the [mise GitHub backend](https://mise.jdx.dev/dev-tools/backends/github.html#bin_path) exposes both from the release archive when `github:crittylabs/critty` comes before the tool providing Codex in `[tools]`.

Keep the one-time marker shown by the Critty skill visible so the review opens in the correct pane.  A terminal filter such as [tfil](https://github.com/knu/tfil) can report it directly; see [terminal-filter integration](docs/terminal-filter.md).

### Agent skills

`critty install` installs skills that match the binary:

- [critty](internal/cli/skill/critty/SKILL.md) — runs the interactive review loop and handles feedback.
- [critty-approval](internal/cli/skill/critty-approval/SKILL.md) — prepares drafts for human approval and checks the result before acting.
- [critty-cli](internal/cli/skill/critty-cli/SKILL.md) — provides the detailed command and review-format reference.

When you submit a round, the agent receives your comments and continues the workflow.  If its wait times out, send `hey` after submitting.

Run the installer from your home directory to install globally, or from a repository root to install for that project only.

#### Claude Code

```bash
cd ~ && critty install claude-code   # ~/.claude/skills/{critty,critty-cli,critty-approval}/
critty install claude-code           # From a repo root: install for that project
```

Then use `/critty [file]`.

#### Codex

```bash
cd ~ && critty install codex        # ~/.agents/skills/{critty,critty-cli,critty-approval}/
critty install codex                # From a repo root: install for that project
```

Then use `$critty`.

#### OpenCode

```bash
cd ~ && critty install opencode     # ~/.config/opencode/commands/critty.md and ~/.config/opencode/skills/{critty-cli,critty-approval}/
critty install opencode             # From a repo root: install for that project
```

Then use `/critty [file]`.

#### Gemini CLI

```bash
cd ~ && critty install gemini        # ~/.gemini/skills/{critty,critty-cli,critty-approval}/
critty install gemini                # From a repo root: install for that project
```

Ask Gemini to use the `critty` skill to review your changes or a document.  Use `/skills reload` if Gemini CLI is already running.

If you used the earlier `@tcrit` subagent integration, remove the old `.gemini/agents/tcrit.md` yourself; the installer leaves it in place.

#### Prompt templates

```bash
cd ~ && critty install prompts        # Install global templates under ~/.config/tcrit/prompts/
critty install prompts                # From a repo root: install under .tcrit/prompts/
critty check                         # Report stale installed integrations
```

### Reviewing actions before execution

Ask your agent to use `critty-approval` when you want to review a push, PR, issue, message, release, or deployment before it happens.  For example:

> Use critty-approval to prepare an issue for example/project.  Let me review its destination, title, and body before publishing it.

In Claude Code, invoke `/critty-approval`; in Codex, use `$critty-approval`.  In OpenCode and Gemini CLI, ask the agent to use the `critty-approval` skill.

The agent prepares drafts for you to comment on and approve, then carries out the authorized actions.  You can answer choices and grant individual permissions in the same review.  Critty records the decisions; the agent executes the actions.

See [Interactive forms](#interactive-forms) for basic controls and [the approval skill](internal/cli/skill/critty-approval/SKILL.md) for the full workflow.

### Building a custom flow

Keep project policy in your `CLAUDE.md` or `AGENTS.md` and delegate review mechanics to the installed skills.  For example:

```markdown
After implementation, use the critty skill to review the staged changes
with the proposed commit message attached.  Address feedback until approved,
then commit with the exact reviewed message.

Before publishing a PR or issue, use the critty-approval skill to review
the destination and final text.  Execute only the approved actions.
```

Watch the [Claude Code](#review-round-trips-with-claude-code) and [Codex](#review-round-trips-with-codex) examples in the Gallery.

## Requirements

- **Go 1.25+** for building from source
- **Herdr or tmux** for automatic agent review workflows. Without either multiplexer, critty can run the TUI directly in an interactive terminal.

### Starting a multiplexer session

For the most spacious review layout, start the agent inside [Herdr](https://herdr.dev/). Critty opens each review in a dedicated tab and returns to the agent tab when the review ends.

Alternatively, start tmux before launching your agent:

```bash
tmux new -s work
# Now launch Claude Code inside this tmux session
claude
```

Without Herdr or tmux, launch critty yourself in an interactive terminal when an agent asks you to review.

## CLI overview

| Command | Purpose |
|---------|---------|
| `critty` | Review current Git changes |
| `critty --staged` | Review staged changes |
| `critty notes.md` | Review a document |
| `critty plan plan.md` | Review a versioned plan |
| `critty --session <id>` | Resume a saved review |
| `critty status` | List saved reviews |
| `critty stop --session <id>` | Stop a review and keep its comments |
| `critty clear --session <id>` | Delete a saved review |

Use `critty notes.md:42` or `critty --focus path/to/file:42` to start at a particular line.

For all commands and options, run `critty --help` or see the [CLI reference skill](internal/cli/skill/critty-cli/SKILL.md).

## Code Review (multi-file)

```bash
critty                            # all current changes, including untracked files
critty --staged                    # staged changes only
critty --unstaged                  # unstaged and untracked changes
critty --scope=main...             # changes since the merge base with main
```

Browse the changed files, leave comments, and press `q` or click **Submit** to finish.  Submit feedback for another round, or approve when the comments are resolved.  The agent reads your feedback, updates the code, and reopens the saved session.

Comments follow source edits across rounds.  If a comment can no longer be matched, a `💬` marker keeps its original context accessible.

### Supplied diffs

Review a diff file with `critty --diff=changes.diff`, or pipe a Git diff:

```bash
git diff main feature | critty --diff
```

This also works outside a Git repository.  To provide an updated diff for the next round, repeat the command with `--session <id>`.

## Stopping and resuming

```bash
critty status                    # list saved sessions in this working directory
critty stop --session <id>        # stop without deleting or approving
critty --session <id>             # reopen from the original directory
critty clear --session <id>       # explicitly delete a stopped review
```

The TUI closes after each submitted round.  Stopping mid-round keeps your feedback for later.  Use the session ID printed by Critty to resume the same review; approval normally cleans up its saved data.

## Plan Review (versioned)

```bash
critty plan docs/plans/my-plan.md
```

Saves numbered versions of the plan in the session directory.  Run `critty plan --session <id> <file>` to submit a revised version (stdin works too), or `critty --session <id>` to reopen the saved one.  Comments carry forward onto the revised text.

### Attached documents

Attach notices, approval notes, or a commit message without staging them:

```bash
critty review --staged --attachment NOTICE.md --attachment COMMIT_MESSAGE.md
critty plan design.md --attachment APPROVAL.md
critty --null --attachment APPROVAL.md
```

Keep review-only drafts outside the index.  Attachments appear before the main review and support comments and replies.  `--null` reviews attachments on their own, including outside Git.

Resuming uses saved snapshots.  To update the attachments, pass the complete list again with `--attachment`; use `--clear-attachments` to remove them.

### Interactive forms

In attached Markdown, press `Space` or click a checkbox to answer.  Answers are saved separately from the source document.  [See an attached checklist with an inline image](#attached-checklist-with-an-inline-image).

Required confirmations must be answered before approval.  Permission checkboxes apply to specific operations; approving the review leaves unchecked operations unauthorized.

Ask your agent to use the [approval skill](internal/cli/skill/critty-approval/SKILL.md) to prepare forms.  Form syntax and exported results are documented in the [CLI reference](internal/cli/skill/critty-cli/SKILL.md).

## Document Review (single file)

```bash
critty review docs/plans/my-plan.md
```

Read the document, leave inline comments, and submit feedback or approve as in a code review.  The agent revises the document and reopens the same session for the next round.

Inside Herdr or tmux, Critty opens the review in a dedicated tab or split pane and waits for your submission.  Without a multiplexer, run it directly in an interactive terminal.  For Codex's background server, use the [Codex wrapper](#codex-with-a-background-server).

## Keybindings

| Key | Action |
|-----|--------|
| `j` / `k` | Move down / up |
| `ctrl+d` / `ctrl+u` | Scroll half a page |
| `g` / `G` | Jump to top / bottom |
| `tab` / `shift+tab` | Switch files |
| `t` | Focus the file tree or comments sidebar |
| `n` / `N` | Jump to the next / previous change or open thread |
| `/` / `?`, `ctrl+s` / `ctrl+r` | Search forward / backward |
| `enter` | Add a comment or open a thread |
| `v` | Select lines for a comment |
| `f` | Comment on the whole file |
| `[` / `]` | Jump between threads |
| `r` / `R` | Toggle resolution / resolve and move to the next thread |
| `w` | Toggle ignoring whitespace in diffs |
| `h` / `H` | Fold resolved threads / hide comments |
| `I` | Toggle image previews |
| `alt+e` | Open the current line in `$EDITOR` |
| `alt+?` | Show all keyboard shortcuts |
| `q` | Open the finish dialog |

### Search

Search updates as you type.  Use `ctrl+s` / `ctrl+r` to move between matches, `enter` to keep the match, and `esc` or `ctrl+g` to return to the starting point.  During search, `alt+s` followed by `c`, `w`, `r`, or `m` toggles case handling, word search, regular expressions, or multi-file search.  The search prompt shows the active modes.

You can also click **Search** and use the on-screen controls.  See [incremental search in the Gallery](#incremental-search) or open Keyboard Help (`alt+?`) for more shortcuts.

### Comments

In a comment or reply, `ctrl+s` saves and `ctrl+o` opens `$EDITOR`.  Emacs-style editing and kill-ring shortcuts are supported.  Use `ctrl+v` to paste text or a clipboard image; image paste needs `wl-paste` or `xclip` on Linux and uses the remote clipboard over SSH.

Link to a file with `@path/to/file L40`, or to a thread with its comment ID.  Typing `@` offers file completion.  Use `alt+w` to copy a source or thread reference and `ctrl+y` to paste it.

### Mouse controls

Click tabs, files, lines, and threads to navigate.  Click or drag in the gutter to comment on a line or range.  Use the mouse wheel to scroll and the buttons on threads and dialogs to reply, resolve, or submit.

## Scriptable CLI

Agents can add comments, reply, and read saved feedback without opening the TUI.  See the [CLI reference skill](internal/cli/skill/critty-cli/SKILL.md) for commands, session targeting, and review-format details.

Shell completions are available through `critty completion <shell>` for Bash, Zsh, Fish, and PowerShell.

## Gallery

### Code review with inline comments

<video controls src="https://github.com/user-attachments/assets/0942656f-1192-455c-a29c-6af171c7138d"></video>

### Review round trips with Claude Code

<video controls src="https://github.com/user-attachments/assets/a8c68b3f-e569-4290-b314-0923f3572dd6"></video>

### Review round trips with Codex

<video controls src="https://github.com/user-attachments/assets/9927c5f6-c7a6-4190-90dd-b20fef75f1e1"></video>

### Reviewing an image replacement

![Before and After image previews in Ghostty](https://github.com/user-attachments/assets/664a9bee-4d88-4467-8326-8360da97a078)

### Attached checklist with an inline image

![Attached review checklist with required confirmations, clickable checkboxes, and an inline image](https://github.com/user-attachments/assets/94076412-3b6d-47e0-a885-dee636686925)

### Incremental search

![Incremental search highlights matches while an inline image remains visible](https://github.com/user-attachments/assets/bfd140a0-3631-4b61-a6b0-ea690ffafd7f)

## Development

```bash
go test ./...
go build ./...
go vet ./...
```

### Regenerating the demo

The README animation and the MP4 clips are recorded with VHS from the tapes in `demo/`.  See [demo/README.md](demo/README.md) for the requirements and the `make -C demo` targets.

## License

MIT
