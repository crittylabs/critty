# Agent instructions

This file is read by coding agents such as Claude Code and Codex.

## Local notes

If `AGENTS.local.md` exists next to this file, read it before starting work and follow it.  It holds a developer's local additions, is not committed, and takes precedence over this file where they differ.

## How we work

- Create task scratch directories with `critty tmp create` and use the returned paths for temporary documents and results.
- Write skills, tests, and other repository text in English regardless of the language used in development instructions.  Use other languages only where the content requires them, such as multilingual test cases.
- Update `README.md` in the release preparation commit.  For major visual changes or headline features, propose recording new demos.
- Upload screenshots, demo videos, and other binary files that will be replaced later to GitHub, such as issue or PR attachments and release assets, and reference their URLs instead of committing them.  Keep only stable brand assets such as logos under `assets/`.
- Research [Crit](https://github.com/tomasz-tomczyk/crit)'s current implementation and documentation when changing the review format or workflow, and use the findings to inform specification and compatibility decisions.
- In commit messages, put key names such as `R` or `ctrl+v` and command names in backquotes.  Never write a bare `@` in commit subjects; goreleaser copies them into release notes, where they become mentions.

## User-facing documentation

- Keep `README.md` focused on feature benefits, installation, basic workflows, and important limitations for human users.  Link to bundled skills under `internal/cli/skill/` and built-in help for details; update affected skills alongside CLI or workflow changes.
- Keep bundled skills focused on agent workflows and CLI contracts.  Document human TUI controls in built-in help.  In bundled skills, refer to agents generically, naming specific tools only as examples, and use the returned scratch directory in path examples.
- In README text and release notes, identify who uses the feature and what they can accomplish.  Lead with the user's goal, such as checking their answers before submitting, then explain the controls.  Use examples that make sense beyond this project's own workflow.
- Distinguish human TUI features from commands used by AI agents and machine-readable results.  For an agent-facing option, name the actor, explain its visible effect, and clarify relevant returned values.  Keep detailed CLI and JSON contracts in the bundled skills.
- Group related UI improvements, form elements, and other changes by the user purpose they serve; use child items when they clarify individual additions.  For forms, explain when an agent uses them to ask for the user's preferences or decisions.
- Describe fixes through the incorrect behavior they remove, including defaults when they explain the failure.  State practical limits that affect use, such as formatting stopping at blank lines; give the tradeoff when it explains the limit.

## Changelog maintenance

Treat `Unreleased` as a compact description of the final user-visible differences from the latest published release.  Update it as a whole, rather than adding a record for each commit or task.

Before every changelog update:

1. Verify the latest published release and its tag.  Compare that tag with the final changes being prepared, including in-scope uncommitted work; the previous commit is not the baseline.
2. Read all of `Unreleased` and find the entries covering the affected features.  Rewrite those entries to describe the final behavior, merging related additions, refinements, and fixes into one entry per user-visible change.  Add a separate entry only for an independent difference from the release baseline.
3. Remove superseded behavior, duplicate explanations, and fixes for problems introduced and resolved after the baseline.  Retain fixes for problems present in a published release, including prereleases; verify publication history before removing them.
4. Check the complete section against the release comparison.  Every entry must describe a remaining user-visible difference, and every such difference, including small fixes, must be covered.  Keep only the wording needed to explain those differences and their important constraints.

- Name new commands, skills, and integrations and explain their roles; link to details instead of accumulating workflow steps.
- Move the consolidated section to a versioned section during release preparation.  Keep published sections unchanged unless correcting an error.
- Keep implementation mechanics, detailed dialog layouts, and exhaustive key bindings in code or Help.  Include controls only when they explain the change or help users adapt established usage; point to updated Help for full instructions.
- State concrete compatibility requirements when they affect adoption or existing workflows, before listing supported implementations as examples.  Describe layered integrations in terms of their host requirements.  Restrict platforms only when the implementation requires it.
- A release's section becomes its GitHub Release notes, so use absolute URLs for links there.

## TUI design

- Keep text, diffs, selections, focus indicators, and dialogs readable in both light and dark color schemes.
- Make basic operations available through both keyboard and mouse.  Provide clickable buttons for actions and closing dialogs, and let users click input fields and tabs to focus them.
- Support multilingual input and Unicode text, including wide and combining characters.  Keep rendering, selection, mouse hit areas, and the terminal cursor aligned through wrapping and scrolling so IME input appears at the correct position.
- Keep dialogs usable in small terminals and after resizing.  Keep confirm and cancel controls reachable, let users scroll content that does not fit, and restore focus appropriately when dialogs close.
- Keep editing behavior consistent across input fields.  Preserve existing Emacs-style editing and paste operations when adding shortcuts, and document new key bindings in the help.
- For visual experiments, show alternatives together in a preview and get feedback before staging the final implementation.
- Use the hidden `critty review --demo` option for interactive demos in the current terminal.  Review controls work normally, but comments, answers, and submission stay in memory and are discarded on exit.  It creates no saved review, history, IPC server, or agent result.  Temporary files for external editors and pasted images are allowed; demo image files are removed on exit.  Do not combine it with session, task, export, terminal-request, or launcher options.  Use `critty view` for read-only inspection.
- Critty detects the terminal background, so check light and dark schemes in separate native windows titled with the scheme and build, and keep the previous build open for comparison.
- Diagnose color and background glitches by inspecting the emitted ANSI sequences before changing the palette.  In tmux, `tmux capture-pane -e` can reveal the actual terminal output.

## Review

- Default to reviewing changes with Critty itself using a build of the current checkout when possible.  If dogfooding is unavailable, explain the limitation and use another review method.  If you replace the review build while a round is open, stop the review and start it again.
- Before reviewing, verify the binary version and check for active review sessions.  Do not replace a build used by another session.
- Preserve comments, replies, attachments, and form answers across concurrent TUI and CLI updates, stop/resume, and review rounds.  Report save failures instead of presenting them as success.
- Keep display options separate from reviewed content and approval state.  Whitespace filtering, folding, and display order must preserve source text and comment anchors.  Show when a comment cannot be reliably matched to the current source, and keep its original context accessible.
- Treat stdout, stderr, JSON output, exit codes, and continuation commands as contracts for agents.  When changing them, verify a complete review round trip.
- When a change alters review UI behavior, attach a small document that exercises it and say in the round summary what to try.

### Review build

With the user's permission, make the review build available as `critty` on PATH.  For example, copy it into a directory early on PATH, or copy only the binary into a temporary directory outside the checkout and add that directory to PATH.  Keep the selected build available through all review rounds, then restore any replaced executable or remove the temporary directory.

## Development

Build, test, and vet with the standard Go tools:

```sh
go build ./...
go test ./...
go vet ./...
```

Run `python3 e2e/scenarios.py` before pushing to test the final changes together.  While iterating on UI feedback, run targeted package tests and defer `e2e/scenarios.py` until the change is ready for commit or push.  Reuse a passing result while the tested code remains unchanged.

CI installs released tmux (apt on Ubuntu, Homebrew on macOS), which may be older and narrower than the local build.  Detect tmux features from the server, give E2E panes an explicit size when matching long text, and reproduce CI-only failures with an official tmux release built in a temporary directory.

Use `e2e/native_terminal.py` inside the native terminal for isolated checks with saved stdout, stderr, and exit results.  Read its command help before launching; window closure alone does not indicate failure.

Agent tool calls may implicitly set `NO_COLOR`; do not propagate it to user terminals unless intentional.  For manual previews and captures, reuse the environment setup in `e2e/native_terminal.py` or `e2e/tmux_launchers.py`.  Ad-hoc launches must drop `TMUX*`, `HERDR_*`, `CRITTY_*`, and `NO_COLOR`, keep `XDG_STATE_HOME` under a short `/tmp` directory because macOS limits socket paths to about 104 bytes, and pass `--launcher mux` when Critty must render in the current PTY.

See the README for installing the binary and the agent skills, and `CHANGELOG.md` for changes by release.

When recording demos, verify that the recording runs the intended binary.  Login shells may reorder PATH, so setting PATH in the launcher alone does not establish which build runs in the recording pane.

After a tag's CI passes, `release.yml` starts as a separate `workflow_run` run; find and watch it by workflow name.
