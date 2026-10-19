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

When your agent needs your input, it prepares the request and opens the review in Critty.  You can comment on individual lines, use checkboxes to confirm details or allow actions, and choose options with radio buttons.  Your comments and answers go back to the agent.

For example:

- Review code changes alongside screenshots and questions from your agent.
- Leave inline feedback on a plan or document.
- Review updates to notes, reports, or configuration files outside Git before applying them.
- Choose an implementation approach before coding starts.
- Check a proposed PR's text and destination before authorizing publication.

![An approval checklist with required confirmations and an inline image](https://github.com/user-attachments/assets/d22e2123-92f9-400d-8b35-f20f4c010db2)

[More videos and screenshots](#gallery).

> **critty** is the new name of crit, originally published as `kevindutra/crit`, with the fixes and improvements developed as TCrit; see [CHANGELOG.md](CHANGELOG.md) for every change since crit 0.2.2.

## Key Features

- **Code and document reviews** — read multi-file diffs and documents with syntax highlighting and inline comments.
- **Approvals and choices** — read requests in attached documents, confirm details or allow actions with checkboxes, and choose between alternatives with radio buttons.  These requests can accompany a code review or stand on their own.
- **Round trips with your agent** — send feedback and answers, let the agent act on them, and return for the next round.  Integrations support Claude Code, Codex, OpenCode, and Antigravity CLI; legacy Gemini CLI is also available.
- **Flexible review inputs** — review Git changes, commit ranges, supplied diffs, or versioned plans.
- **Review changes outside Git** — review proposed edits to documents and configuration files before updating the originals.  Create, split, rename, or delete files through the same feedback and approval workflow.
- **Inline image previews** — compare image changes, preview images while writing comments, and inspect local images in a zoomable lightbox.  [See an image review](#reviewing-an-image-replacement).
- **Navigation and editing** — browse files, search as you type, reply to comments, and edit in your preferred editor.
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

After installing the binary, run:

```bash
critty doctor
```

Run it in the terminal where you plan to use Critty.  It checks installed agent integrations, review launchers, image settings, and Python/MCP prerequisites for optional AI reviews.  Follow its suggestions for the features you plan to use, then rerun it after setup.

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

Rerun the installer after upgrading.  In v1.2.0, Codex skills use `.codex/skills` in projects and `~/.codex/skills` for users; remove obsolete copies from `.agents/skills` if needed.  When migrating from TCrit, use the new binary and skill names; see the [changelog](CHANGELOG.md) for migration details.

After installing or upgrading the skills, run `critty doctor` from your project to check for missing or outdated integration files.

### Terminals and multiplexers

Floating panes require tmux 3.8 or later; older servers use split panes.  Desktop terminal windows are not opened over SSH, so use tmux or Herdr for agent reviews, or run Critty directly.

After setting up or switching your terminal or multiplexer, run `critty doctor` there to check review launchers and local image forwarding settings.  See the [launcher reference](internal/cli/skill/critty-cli/SKILL.md#review-launchers) for configuration and supported terminals.

#### Image previews

Image previews need a terminal with Kitty graphics protocol support and Unicode image placeholders, such as Kitty, Ghostty, or iTerm2.  Through tmux or Herdr, enable image passthrough; for tmux, run `tmux set -g allow-passthrough on`.

After enabling passthrough, run `critty doctor` in the foreground of that terminal to probe image support and check forwarding settings.  Herdr checks read local configuration; running server and remote client settings may differ.

PNG, JPEG, GIF, and WebP are supported.  Large or unsupported images and changed images in supplied diffs remain text placeholders; remote image URLs are not fetched.

<a id="codex-with-a-background-server"></a>

### Using Codex in tmux or Herdr

When using Codex in tmux or Herdr, start it through Critty so reviews can find the correct terminal:

```bash
critty codex
```

Alternatively, put the bundled `codex` wrapper ahead of Codex on your `PATH`.  Run `critty doctor` to inspect wrapper and terminal-filter detection.  For an end-to-end binding test, use the [marker-based doctor check](internal/cli/skill/critty-cli/SKILL.md#environment-diagnostics).  See [terminal-filter integration](docs/terminal-filter.md) for setup.

## Basic workflow

### Review code and documents

Ask your agent to use the `critty` skill to review its changes, a plan, or a document.  For example:

> Use critty to review these changes before committing.

Critty opens in a tmux floating pane, a Herdr tab, or a new window of a supported terminal emulator when neither is running.  Read the diff or document, leave inline comments on the relevant lines, and submit Approve, Reject, or Comment.

The agent reads your verdict and comments, makes revisions, and reopens the same session.  You can read its replies and the updated changes, add feedback, and continue until the work is ready.

<video controls src="https://github.com/user-attachments/assets/2842e136-b0bd-4a4f-9c66-d5a22ed52ea3"></video>

For a manual review:

```bash
critty                    # Current Git changes
critty --staged            # Staged changes
critty notes.md            # A document
critty plan design.md      # A versioned plan
critty status              # Saved reviews
critty --session <id>      # Resume a saved review
```

<a id="review-navigation"></a>

Reviews support mouse and keyboard navigation, search, inline comments, and your preferred editor.  Progress indicators highlight unresolved feedback and unanswered items.  Use the in-app Help for controls and `critty --help` for commands.  The [CLI reference](internal/cli/skill/critty-cli/SKILL.md) covers review inputs, attachments, saved sessions, configuration, and scripted comments.

### Review changes outside Git

Use Critty for personal notes, reports, configuration files, and other text files that are not in a repository.  Your agent can propose edits, create files, split a long document, or rename and remove files while keeping the originals unchanged until you approve.

For example:

> Use critty to review updates to my notes before applying them.  Split the long meeting notes into a summary and action items, and keep the originals unchanged until I approve.

The agent prepares copies with `critty tmp`, edits them, and opens the proposed diff with a request to apply it.  Leave comments and review revisions as usual.  After you approve the review and select the permission checkbox, the agent applies the reviewed changes.  Critty checks for changes to the originals and retains backups for recovery.

See the [workflow for changes outside source control](internal/cli/skill/critty/references/temporary-tree.md) and `critty tmp --help` for details.

<a id="approvals-and-choices"></a>

### Review changes and approve the next actions

Agree with your agent on which actions need your approval, such as committing, pushing a branch, or deploying.  Keep your project rules for reviews, approvals, and choices in `CLAUDE.md` or `AGENTS.md`.

When it needs your permission or a decision, the agent can use [critty-approval](internal/cli/skill/critty-approval/SKILL.md) to prepare a document with the details and open it in Critty.  That document can be attached to the same session as the diff, so you can review the changes and next steps together.  It can include the proposed commit message, push destination, or deployment plan.

For example:

> Use critty-approval to review these changes and the proposed push together.  Include the commit message and exact repository and branch destination, and wait for my approval before pushing.

For a deployment, include the target environment and exact deployment actions alongside the changes.  Review the code and proposed operations together, leave feedback where needed, and approve the operations you want the agent to carry out.

Required confirmations must be complete before approval.  Approving the review does not authorize operations whose permission checkboxes remain unchecked.  Critty records your decisions; the agent executes the authorized actions.

### Approvals and choices without a code diff

The agent can also open a request document on its own, without a code diff.  Use the same `critty-approval` skill to choose an approach before implementation starts or approve an action that has no code changes to review:

> Use critty-approval to let me choose between these implementation approaches.
>
> Use critty-approval to show me the PR text and destination before publishing it.

Read the request and supporting material, leave comments, and use the checkboxes or radio buttons to give your answers.  Submit your decision, and the agent continues from your feedback and answers.  The installed skills handle the interaction with Critty.

### AI review and collaboration (Experimental)

Request a second opinion with [critty-ai-review](internal/cli/skill/critty-ai-review/SKILL.md), or use [critty-ai-cowork](internal/cli/skill/critty-ai-cowork/SKILL.md) to let your agent address findings with another AI before your final review.  Findings return to the same Critty comment threads.

For example:

> Use critty-ai-review with Codex Astra to review these changes in the current session.

AI review requires an existing Critty session and authorized access to the chosen reviewer.  The bundled restricted CLI setups support Claude Code with Python 3.10+, or Codex with Python 3.11+ and the MCP Python SDK.  See the linked skills for setup and dependencies.

Before your first AI review, run `critty doctor` to check Python and MCP prerequisites.  If the reviewer uses a dedicated Python environment, check that interpreter explicitly:

```bash
critty doctor --python /absolute/path/to/venv/bin/python
```

If you installed the earlier `crit-ai-review` or `crit-ai-cowork` skills, reinstall with `critty install --force <agent>` to update them.

## Gallery

Videos recorded with the v1.2.0 candidate.

### Code review with inline comments

<video controls src="https://github.com/user-attachments/assets/2842e136-b0bd-4a4f-9c66-d5a22ed52ea3"></video>

### Side-by-side code review

![Split diff view in Ghostty with an inline review comment and agent reply](https://github.com/user-attachments/assets/e5d6b0c3-47c4-431d-a020-058f6cf9b414)

### Review round trips with Claude Code

<video controls src="https://github.com/user-attachments/assets/b78ebf6f-c406-44cd-b56d-b1793031f5b9"></video>

### Review round trips with Codex

<video controls src="https://github.com/user-attachments/assets/4e86ed2e-65c1-4aed-a499-e4d57e3736e2"></video>

### Asking Codex Astra for a review from Claude Code

A comment in a Claude Code review requests a second opinion from Codex Astra.  Claude starts the reviewer, and Astra's inline findings and overall report return to the same Critty session.

<video controls src="https://github.com/user-attachments/assets/1e8f2467-27fb-4a83-9685-9aedc40e1c8d"></video>

### Reviewing a document

<video controls src="https://github.com/user-attachments/assets/d8cf18f7-aa1d-4985-a993-f4c522eaf28c"></video>

### Previewing an image while writing a comment

![Image preview beneath a comment draft in Ghostty](https://github.com/user-attachments/assets/5576979d-7943-4ae4-9578-37a9e01d8508)

### Inspecting an attached image in the lightbox

![Lightbox showing a local image from an attached Markdown document](https://github.com/user-attachments/assets/af8b4612-b968-49d1-b18a-e0b3d3a6350a)

### Reviewing an image replacement

![Before and After image previews in Ghostty](https://github.com/user-attachments/assets/76064e2d-e9f1-4c07-ae5d-4f7f3f1a16a7)

### Attached checklist with an inline image

![Attached review checklist with required confirmations, clickable checkboxes, and an inline image](https://github.com/user-attachments/assets/d22e2123-92f9-400d-8b35-f20f4c010db2)

### Review progress and pending items

![Actionable counters, tab markers, and a thread on an attached checklist in Ghostty](https://github.com/user-attachments/assets/8e52ed1c-3854-4786-9a9a-950efe4e79a7)

### Submitting with unfinished items

![Submit dialog warning about unresolved threads and unanswered required and optional items](https://github.com/user-attachments/assets/67093884-2089-4044-8b7e-2b9d2fbed91b)

### Incremental search

![Incremental search highlights matches while an inline image remains visible](https://github.com/user-attachments/assets/c87a24e4-dbea-4c1f-996b-0cad6844d306)

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
