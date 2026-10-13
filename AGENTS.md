# Agent instructions

This file is read by coding agents such as Claude Code and Codex.

## Local notes

If `AGENTS.local.md` exists next to this file, read it before starting work and follow it.  It holds a developer's local additions, is not committed, and takes precedence over this file where they differ.

## How we work

- Keep `README.md` focused on feature benefits, installation, basic workflows and controls, and important limitations for human users.  Link to bundled skills under `internal/cli/skill/` and built-in help for details; update affected skills alongside CLI or workflow changes.
- Keep bundled skills focused on agent workflows and CLI contracts.  Document human TUI controls in built-in help and `README.md`.
- Update `README.md` in the release preparation commit.  For major visual changes or headline features, propose recording new demos.
- Research [Crit](https://github.com/tomasz-tomczyk/crit)'s current implementation and documentation when changing the review format or workflow, and use the findings to inform specification and compatibility decisions.
- In commit messages, put key names such as `R` or `ctrl+v` and command names in backquotes.

## Changelog maintenance

Treat `Unreleased` as a compact description of the final user-visible differences from the latest published release.  Update it as a whole, rather than adding a record for each commit or task.

Before every changelog update:

1. Verify the latest published release and its tag.  Compare that tag with the final changes being prepared, including in-scope uncommitted work; the previous commit is not the baseline.
2. Read all of `Unreleased` and find the entries covering the affected features.  Rewrite those entries to describe the final behavior, merging related additions, refinements, and fixes into one entry per user-visible change.  Add a separate entry only for an independent difference from the release baseline.
3. Remove superseded behavior, duplicate explanations, and fixes for problems introduced and resolved after the baseline.  Retain fixes for problems present in a published release, including prereleases; verify publication history before removing them.
4. Check the complete section against the release comparison.  Every entry must describe a remaining user-visible difference, and every such difference, including small fixes, must be covered.  Keep only the wording needed to explain those differences and their important constraints.

- Write entries as release notes for users.  Lead with what a feature lets them do and its main use cases.  Name new commands, skills, and integrations and explain their roles; link to details instead of accumulating workflow steps.
- Move the consolidated section to a versioned section during release preparation.  Keep published sections unchanged unless correcting an error.
- Keep implementation mechanics, detailed dialog layouts, and exhaustive key bindings in code, Help, or the README.  Include controls only when they explain the change or help users adapt established usage; point to updated Help for full instructions.
- State compatibility requirements before listing supported implementations as examples.  Describe layered integrations in terms of their host requirements.  Restrict platforms only when the implementation requires it.
- A release's section becomes its GitHub Release notes, so use absolute URLs for links there.

## TUI design

- Keep text, diffs, selections, focus indicators, and dialogs readable in both light and dark color schemes.
- Make basic operations available through both keyboard and mouse.  Provide clickable buttons for actions and closing dialogs, and let users click input fields and tabs to focus them.
- Support multilingual input and Unicode text, including wide and combining characters.  Keep rendering, selection, mouse hit areas, and the terminal cursor aligned through wrapping and scrolling so IME input appears at the correct position.
- Keep dialogs usable in small terminals and after resizing.  Keep confirm and cancel controls reachable, let users scroll content that does not fit, and restore focus appropriately when dialogs close.
- Keep editing behavior consistent across input fields.  Preserve existing Emacs-style editing and paste operations when adding shortcuts, and document new key bindings in the help.
- For visual experiments, show alternatives together in a preview and get feedback before staging the final implementation.
- Diagnose color and background glitches by inspecting the emitted ANSI sequences before changing the palette.  In tmux, `tmux capture-pane -e` can reveal the actual terminal output.

## Review

- Default to reviewing changes with Critty itself using a build of the current checkout when possible.  If dogfooding is unavailable, explain the limitation and use another review method.  If you replace the review build while a round is open, stop the review and start it again.
- Before reviewing, verify the binary version and check for active review sessions.  Do not replace a build used by another session.
- Preserve comments, replies, attachments, and form answers across concurrent TUI and CLI updates, stop/resume, and review rounds.  Report save failures instead of presenting them as success.
- Keep display options separate from reviewed content and approval state.  Whitespace filtering, folding, and display order must preserve source text and comment anchors.  Show when a comment cannot be reliably matched to the current source, and keep its original context accessible.
- Treat stdout, stderr, JSON output, exit codes, and continuation commands as contracts for agents.  When changing them, verify a complete review round trip.

### Review build

With the user's permission, make the review build available as `critty` on PATH.  For example, copy it into a directory early on PATH, or copy only the binary into a temporary directory outside the checkout and add that directory to PATH.  Keep the selected build available through all review rounds, then restore any replaced executable or remove the temporary directory.

## Development

Build, test, and vet with the standard Go tools:

```sh
go build ./...
go test ./...
go vet ./...
```

Run `python3 e2e/scenarios.py` before pushing to test the final changes together.  Also run it when a change is known to affect its scenarios.  Reuse a passing result while the tested code remains unchanged.

Use `e2e/native_terminal.py` inside the native terminal for isolated checks with saved stdout, stderr, and exit results.  Read its command help before launching; window closure alone does not indicate failure.

Agent tool calls may implicitly set `NO_COLOR`; do not propagate it to user terminals unless intentional.

See the README for installing the binary and the agent skills, and `CHANGELOG.md` for changes by release.

When recording demos, verify that the recording runs the intended binary.  Login shells may reorder PATH, so setting PATH in the launcher alone does not establish which build runs in the recording pane.
