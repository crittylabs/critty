# Demo recordings

The tapes in this directory record a code review (`code-review.tape`) and a document review without an agent (`demo.tape`) using [VHS](https://github.com/charmbracelet/vhs).  It needs `vhs`, `tmux`, `claude`, and GNU make on `PATH`.

Run these from this directory, or from the repository root with `make -C demo`:

```bash
make            # record MP4s whose tape or scripts changed
make rebuild    # record them again from scratch
make clean      # delete the MP4s
```

Each recording runs `setup.sh`, then `vhs`, then `clean-recording.sh`.  The setup script builds the current checkout and creates the fixtures from `assets` under `$DEMO_DIR` (default `$TMPDIR/critty-demo`): a throwaway Git repository with staged changes, a Markdown document, the Claude Code skills with an English-only `CLAUDE.md` and the full-color `dark` theme, and an isolated `XDG_STATE_HOME` for review sessions.  The fixtures live outside the checkout because Claude Code reads `CLAUDE.md` from every parent directory.  The tapes clear inherited `NO_COLOR` and enable truecolor so agent-host settings do not remove the recording colors.  The tape starts a private tmux server inside the recording so Critty opens its pane there rather than in the multiplexer running VHS.  Both tapes source `theme.tape`, which selects the `Pro` palette shipped with macOS Terminal so the recordings show Critty on a stock theme.  Claude Code runs as Opus 5.5 with low effort and fast mode disabled, in auto permission mode inside the fixture directory; set `DEMO_PERMISSION_MODE=bypassPermissions` before `vhs` if a permission prompt stalls the recording.  The cleanup script moves MP4 playback metadata to the front of the file for streaming.  It preserves every frame without re-encoding or reducing colors.

Set `DEMO_VERSION` to label the recording build, for example `DEMO_VERSION=v1.8.0 make -C demo rebuild` from the repository root.  The tapes put the fixture's build first on `PATH`; verify `command -v critty` and `critty --version` inside the actual recording pane, since login shells can reorder `PATH`.  Put the Go toolchain on the pane's `PATH` as well so recorded agents can run tests.

Codex review-flow recordings use `assets/codex-config.toml`: GPT-6.1 Sol, medium reasoning, and Fast mode.  Preparation scripts copy this file into the recording's isolated `CODEX_HOME` before adding fixture trust settings, so they do not depend on personal Codex defaults.  Claude Code remains on Opus 5.5 with low effort and fast mode disabled.

MP4s are local build outputs ignored by Git.  Published recordings are hosted as release assets.  After the first recording, MP4s only rebuild when a tape, script, or fixture is newer.

For inline playback in the GitHub README, upload the MP4s as GitHub video attachments and put each `https://github.com/user-attachments/assets/...` URL on its own line.  GitHub generates the player.  Handwritten `<video>` elements can fail playback for signed-in readers.  Verify playback and seeking in the README and hosting issue, both signed in and signed out.  Keep the versioned release assets for downloads.

With GitHub CLI 2.103.2 or newer, publish attachments and their hosting issue together:

```sh
gh issue create --repo crittylabs/critty --title 'Demo media for v1.8.0' \
  --body-file MEDIA.md --attach ./review-flow-codex-v1.8.0.mp4
```

Write local references such as `![](./review-flow-codex-v1.8.0.mp4)` in `MEDIA.md`; paths are relative to that file.  GitHub CLI replaces them with uploaded URLs and native video players.  Repeat `--attach` for each file.  If some uploads fail, the issue is still created with the successful attachments and its URL is printed; inspect it before retrying to avoid duplicates.  Download each URL without credentials and compare it with the local file.

## Uploading recordings

Use an existing draft or published release to host recordings without adding them to Git:

For an upcoming release, create a draft with both its tag and title set to the version, such as `v1.8.0`.  Upload the recordings before pushing the release tag.  After tag CI passes, GoReleaser reuses the draft, keeps its attached recordings, replaces its notes with the matching `CHANGELOG.md` section, and publishes it.  If no matching draft exists, it creates a new release.

```sh
./demo/release.sh -t v1.8.0 list
./demo/release.sh -t v1.8.0 upload demo/review-flow-codex.mp4 v1.8.0
```

The script requires `gh`, `jq`, and `sha256sum` or `shasum`.  Authenticate `gh` with write access to the repository first.  The repository defaults to the current repository reported by `gh repo view`.  The default release tag is `demo-assets`.  Use `-r OWNER/REPO` and `-t TAG` to select another repository or release.  The release must already exist; the script does not create tags or releases.

`list` prints tab-separated asset names, SHA-256 digests, and download URLs.  `upload FILE VERSION` accepts GIF or MP4 and preserves the extension, for example `review-flow-codex-v1.8.0.mp4`.  The version must have the form `vX.Y.Z`.  An existing asset with that name is reused only when its SHA-256 and size match; different content under the same name is rejected.  Existing assets are never overwritten.  Draft releases are supported; their download links become public when the release is published.

v1.8.0 recordings:

- [Outlines and image attachments](https://github.com/crittylabs/critty/releases/download/v1.8.0/outlines-v1.8.0.mp4)
- [Code review](https://github.com/crittylabs/critty/releases/download/v1.8.0/code-review-claude-v1.8.0.mp4)
- [Document review](https://github.com/crittylabs/critty/releases/download/v1.8.0/demo-v1.8.0.mp4)
- [Review and commit with Claude Code](https://github.com/crittylabs/critty/releases/download/v1.8.0/review-flow-claude-v1.8.0.mp4)
- [Review and commit with Codex](https://github.com/crittylabs/critty/releases/download/v1.8.0/review-flow-codex-v1.8.0.mp4)
- [Claude Code requesting a Codex Astra review](https://github.com/crittylabs/critty/releases/download/v1.8.0/ai-review-claude-codex-astra-v1.8.0.mp4)
- [Answering a questionnaire](https://github.com/crittylabs/critty/releases/download/v1.8.0/answering-questions-v1.8.0.mp4)
- [Live review updates and references](https://github.com/crittylabs/critty/releases/download/v1.8.0/updates-v1.8.0.mp4)

## Sending Alt keys and mouse events from a tape

VHS accepts `Alt+p`, but its headless Chrome runs xterm.js on macOS, where Alt with a letter is not turned into an escape sequence, so the TUI receives a bare `p` (charmbracelet/vhs issue #442; `macOptionIsMeta` cannot be set from a tape).  Send the escape byte yourself instead:

```
Escape
Type "p"
```

`Type` waits after each character, not before, so the letter follows the escape within a few milliseconds and tmux delivers both in one read even with the `escape-time 10` in `tmux.conf`.  Critty then sees `alt+p`.

Mouse events work the same way, as SGR sequences: `Escape` then `Type@1ms "[<0;COL;ROWM"` presses the left button at 1-based column and row, the same with a trailing `m` releases it, `[<64;…M` and `[<65;…M` scroll the wheel up and down, and `[<32;…M` drags.  tmux forwards them to the pane that asked for mouse reporting and converts the coordinates, so in a split add the pane's left offset or zoom the pane first.  Hit areas are exact and column widths follow the font and `Set Width`, so check a coordinate with a `Screenshot` before relying on it.
