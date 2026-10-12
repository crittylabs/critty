# Agent instructions

This file is read by coding agents such as Claude Code and Codex.

## Local notes

If `AGENTS.local.md` exists next to this file, read it before starting work and follow it.  It holds a developer's local additions, is not committed, and takes precedence over this file where they differ.

## How we work

- Describe new features in `README.md` at the level of how to use them, not their internal rules.
- Keep README Key Features focused on benefits for first-time readers.  Put detailed usage in the relevant instructions.
- Update `README.md` for a release in the release preparation commit.
- When preparing a release with major visual changes or headline features, propose recording new demo videos to show them.
- When changing CLI behavior or review workflows, update the affected bundled skills under `internal/cli/skill/` in the same change so agents receive the current usage and constraints.
- Follow [crit](https://github.com/tomasz-tomczyk/crit) for specification and compatibility decisions, and check its current implementation and documentation when a change touches the review format or workflow.
- In commit messages, put key names such as `R` or `ctrl+v` and command names in backquotes.

## Changelog maintenance

- Record user-visible differences from the latest published release under `Unreleased`, including small fixes.  Describe the final behavior that will ship, and move entries to a versioned section during release preparation.
- Omit fixes for problems introduced and resolved between releases.  Fold refinements of unreleased features into their feature entries, and keep fixes for problems that existed in a published release, including prereleases.  Verify release tags and publication history before removing a past fix.
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

See the README for installing the binary and the agent skills, and `CHANGELOG.md` for changes by release.

When recording demos, verify that the recording runs the intended binary.  Login shells may reorder PATH, so setting PATH in the launcher alone does not establish which build runs in the recording pane.
