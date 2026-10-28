# Terminal setup and diagnostics

## Environment diagnostics

Run `critty doctor` before troubleshooting integration or image setup.  Missing files in a partially installed integration and stale or unreadable files are errors.  Absent integrations and optional reviewers are informational.  Desktop launcher availability is checked independently of launch priorities and SSH restrictions: the platform's built-in desktop candidates, plus `xdg-terminal-exec` under WSL.  Missing or incompatible desktop launchers, Python/MCP problems, and image setup problems are warnings; they do not block unrelated review modes.  Errors return exit status 1; warnings alone return 0.  The text report groups checks by area under a version header, marks each line with its status (`ok`, `error`, `warn`, or `info`), indents advice beneath the check, and ends with a summary count; color-capable terminals color the statuses, and `NO_COLOR`, redirected output, and JSON retain plain text.

Doctor distinguishes PATH selection from current-session behavior.  It identifies the bundled Codex wrapper from Go build metadata, tfil-generated wrappers from their exact `#!/bin/sh` and `# tfil-wrapper` header, and tfil in process ancestry.  Filter installation and detected ancestry appear only as optional information, not as a separate session check or a request to change the setup; neither verifies agent output notification.  Shims and custom wrappers remain unverified, not broken.  Without a marker request, doctor tests the current foreground terminal locally with a three-second wait, or reports why it was skipped.  Local success does not test agent output.  For an end-to-end check, run `critty terminal prepare`, display its marker verbatim in agent output, then run `critty doctor --terminal-request <id>` within two minutes.  This resolves and consumes the request without opening a review.  It uses a terminal filter notification or the visible screen of a live wrapper registration.  For daemon-backed agents, a resolved marker also supplies the client process ancestry for tfil detection.  Add `--terminal-notify-only` to require a terminal-filter callback instead of reading registered pane screens.  This verifies the notification path even inside a supported multiplexer.  With `tfil --critty-notify`, a missing callback reports a warning rather than succeeding through a registered pane.  Without this option, a successful binding test can also use registered multiplexer panes.

Use `--python /absolute/path/to/venv/bin/python` to inspect the interpreter selected for restricted AI review; the default is `python3` on PATH.  Codex requires Python 3.11+ and the bundled MCP SDK pin and server API; Claude requires Python 3.10+.  MCP advice includes a quoted install command using an absolute path to a matching installed requirements file, or the bundled package pin when no current copy exists.  CLI availability does not certify authentication, supported flags, or the effective restricted tool surface.  Validate those at reviewer launch.

`--json` returns an object with `version` and `checks`.  Each check has `check`, `status` (`ok`, `info`, `warn`, or `error`), `message`, and optional `advice`.  A report is still written when errors cause a nonzero exit.

Image diagnostics read tmux pane settings, including inherited options, when its environment identifies the pane; otherwise they label a global setting or report unknown context.  Herdr diagnostics parse local config with the current `terminal.kitty_graphics` key taking precedence over legacy `experimental.kitty_graphics`.  Unset settings report the current Herdr default of enabled; only an explicit false value prompts advice to enable images.  Configuration checks do not verify a running server or remote client.  On macOS and Linux, `images/terminal` probes the current foreground controlling TTY using the same virtual-placement and cursor-position test as the TUI, including tmux extended-key replies.  A missing, inaccessible, busy, background, or dumb TTY reports `info` with a `skipped:` message and adds no error.  Negative or incomplete replies are warnings; an unanswered probe retries up to three times.  Probe traffic uses the TTY directly, so redirected text and JSON contain only the diagnostic report.  Run from the actual review terminal; the TUI Images control can retry support checks later.

## Terminal marker requests

The interactive `critty` skill starts with automatic launcher selection.  Codex and hosts using a notifying terminal filter send a fresh marker; other hosts use normal terminal detection without a marker.  After observing a successful launcher that needs no marker, it skips markers for the rest of that agent session, unless the user explicitly requests a multiplexer launcher.  An interrupted and resumed agent session starts discovery again.  See its launcher selection procedure for explicit user overrides and configuration preferences.  `critty terminal prepare` returns JSON with `id`, `marker`, and `expires_at`; pass its ID as `--terminal-request <id>` on `critty review`, `critty sessions resume`, `critty plan`, or the default command.  Requests expire after two minutes and are consumed at destination selection.  Review session IDs and terminal request IDs are separate.

Desktop launchers open new windows without marker binding.  Omit `--terminal-request` when launching directly in a desktop terminal.  When a supplied request cannot be resolved, including ambiguous matches, reviews skip multiplexer launchers and try desktop or custom candidates in the configured order.  They use the caller's terminal and SSH hints for this unbound fallback; unrelated inherited multiplexer targets remain excluded.  If every candidate fails, the error includes both the marker and launcher failures instead of opening a TUI in the caller's terminal.  `critty env --terminal-request` still fails when the requested terminal cannot be identified.  Follow the `critty` skill's launcher selection procedure to decide whether later rounds need markers; a successful selection does not pin `--launcher`.  Positive SSH restrictions and explicit requirements to use the existing terminal still apply.

`critty terminal notify <id>` is a terminal-filter callback, not an agent-side workaround.  It reads the filter's original terminal environment and records the destination and client hints, including without a multiplexer before or during review startup.  Duplicate notifications from one pane are accepted; different panes are ambiguous.  Expired or consumed requests fail.  Without a notification, Critty reads only the visible text in live registered panes.  It never broadens a failed search to all panes or scrollback.

## Review launchers

`--launcher` applies to code, document, plan, and resumed reviews.  It is not saved in the review session; repeat an explicit user override when resuming.  Agent workflows otherwise omit it and use configured or automatic selection.

| Kind | Launcher names |
| --- | --- |
| Automatic selection | `mux`, `desktop-terminal` |
| Multiplexer | `tmux`, `tmux-floating`, `tmux-split`, `herdr`, `cmux`, `zellij` |
| Desktop terminal | `xdg-terminal`, `ghostty-window`, `kitty-window`, `iterm-window`, `rio-window`, `wezterm-window`, `alacritty-window`, `terminal-app-window`, `windows-terminal-wsl`, `xterm-window` |
| Custom executable | `custom:EXECUTABLE` |

`mux` selects the nearest detected supported multiplexer, retaining current-terminal operation when none is detected.  Repeat `--launcher` to try launchers in the given order.  The defaults are `mux`, then `desktop-terminal`.  Missing defaults are appended in that order; explicit positions are preserved.  Availability and marker failures stay silent when a later candidate succeeds; `--summary` records their reasons in `launch`.  Once the launched TUI is ready, stderr reports `launcher: NAME` with the concrete launcher, such as `tmux-split`, `ghostty-window`, or `custom:EXECUTABLE`, rather than the `mux` or `desktop-terminal` group.  Direct current-terminal operation reports `launcher: foreground` before starting the TUI; `foreground` is a reported mode, not a value accepted by `--launcher`.  If all launchers are unavailable, the final error reports their failures.  Opening or readiness errors identify the launcher and stop the review without reporting a successful launcher or trying another candidate.

Set `launchers` to an ordered TOML array in the global or project configuration, for example `launchers = ["mux", "custom:my-terminal"]`.  Configured entries follow all CLI `--launcher` entries, before missing defaults are added.  With no CLI entries, the configured list is tried first.  A project array replaces the global array; `[]` clears the global list.  Explicit default placement across the combined list is preserved.

`tmux` tries `tmux-floating` first, then `tmux-split`.  Floating panes require the tmux 3.8+ `new-pane` controls; Critty checks the server's command capabilities.  They are nonmodal and centred against the whole window at 90% width and 85% height.  `tmux-split` explicitly selects the traditional split.  An unavailable `tmux-floating` also falls back through `mux`, allowing a split on older servers.  It selects the review within the source window when that window still selects the source pane and the session has one attached client.  This also applies while the client views another window or the terminal is inactive; the displayed window and OS focus stay unchanged.  On finish, it restores the source only while the review remains selected in the sole attached client, that client is focused, and focus events are enabled, then removes its own pane.  A selection made elsewhere is preserved.

`desktop-terminal` prioritizes terminals that support Critty's image previews (Kitty Unicode placeholders), then other installed terminals, then the platform fallback:

| Platform | Default desktop order |
|----------|-----------------------|
| macOS | Ghostty, kitty, iTerm2, Rio, WezTerm, Alacritty, Terminal.app |
| Linux | Ghostty, kitty, Rio, WezTerm, Alacritty, `xdg-terminal-exec`, xterm |
| WSL | Windows Rio, WezTerm, Alacritty, Windows Terminal |

The client's terminal is preferred within its image-capability tier; system fallbacks stay last.  Detection uses `TERMINAL_APP`, `TERM_PROGRAM`, and terminal-specific environment hints.  Explicit launcher order overrides this preference.  The named launchers open new windows regardless of the current terminal.  General graphics support, such as Sixel, does not guarantee Critty image previews; the lower-priority terminals remain useful for text review.

macOS scripted launchers bring the new window to the front.  Ghostty on macOS requires its 1.3+ AppleScript API; Linux uses its CLI.  Rio, WezTerm, Alacritty, and kitty use their CLIs on PATH, with a fallback to their application bundles under `/Applications` and `~/Applications` on macOS.  They require no remote-control setting.  CLI launchers leave window cleanup to the terminal; settings may retain the window after the review exits, and cleanup never quits an existing application.

Under WSL, Rio, WezTerm, and Alacritty require their `.exe` CLI and `wsl.exe` on PATH.  Windows Terminal uses `wt.exe` and `wsl.exe`.  These launchers run the Linux binary in the originating distribution and directory; native Windows Critty binaries are not supported.  Linux `xdg-terminal` delegates terminal selection to `xdg-terminal-exec`.  `xterm-window` requires both xterm on PATH and `DISPLAY`; Wayland sessions need Xwayland.  Neither xterm nor Xwayland is assumed to be installed.

Built-in GUI launchers are unavailable on positive SSH evidence from the client environment, the resolved tmux session, or validated client-process ancestry (`sshd` or `sshd-session`).  Missing evidence allows GUI use.  Multiplexer and custom launchers remain eligible.  With a single attached tmux client, refreshed session environment supersedes old wrapper SSH hints after reattachment.  Multiple clients cannot supply that override.  A daemon's own environment is not substituted for a marker-bound client.

Platform scripts are embedded under `internal/cli/launchers/` and need no installation.  They illustrate native startup operations; their private argument interface is distinct from the custom JSON contract below.  When the review ends after Submit, macOS launchers request closure of only their created window by its captured ID.  Cleanup is best effort: automatic closure or a terminal confirmation dialog does not invalidate the completed review.  xdg and Windows Terminal rely on the review command exiting; terminal settings may retain the window.  No launcher quits the entire terminal application or restores OS focus unconditionally.

For `custom:EXECUTABLE`, a name containing `/` is an explicit path, relative to the caller's working directory when needed.  Otherwise Critty searches PATH, including the current directory only when PATH explicitly contains `.` or an empty element.  The name is one executable, not a shell expression or an argument list.

Critty invokes the executable with one operation argument:

| Operation | Standard input | Result |
| --- | --- | --- |
| `check` | JSON with `session`, `command`, and `cwd` | Exit 0 when usable; nonzero tries the next launcher. |
| `open` | The same JSON | Return promptly after launching; stdout is the opaque close token. |
| `close` | Exactly the bytes returned by `open` | Close owned resources and restore focus as appropriate; nonzero fails the command. |

`command` is a shell-quoted TUI command; run it in `cwd`.  Each operation has a 10-second deadline.  Write diagnostics to stderr; check diagnostics are reported only if all launchers fail.  Keep `open` stdout exclusively for the token.  If `open` fails after creating resources, the launcher must clean them up itself.  After a successful `open`, Critty calls `close` even if the TUI fails to become ready.

Each operation inherits the caller's environment with stale `TMUX`, `TMUX_PANE`, `HERDR_*`, `CMUX_*` (except the socket password), `ZELLIJ*`, and `TTY` replaced by the detected terminal context.  Other variables, including the state/config locations, are preserved.  `CRITTY_TERMINAL_APP` carries a detected application name and `CRITTY_SSH=1` reports positive SSH evidence.  Use `critty env` to inspect or reuse the same detection.

The `cmux` launcher requires the cmux CLI and socket access from the agent process.  It creates a terminal surface in the invoking surface's pane, closes it when the review ends, and restores the invoking surface's focus.  Requests pin the socket, workspace, and surface instead of following the currently focused tab.  Set `CMUX_SOCKET_PATH` for a nondefault socket.  Detection uses `cmux top --all --processes` to match process ancestry, including nested multiplexers; a single inherited cmux context remains usable when process metadata is unavailable.  `critty env` returns `CMUX_SOCKET_PATH`, `CMUX_WORKSPACE_ID`, and `CMUX_SURFACE_ID`.  Terminal markers support both visible-screen lookup through the wrapper and notifying terminal filters.

The `zellij` launcher requires Zellij 0.45+ on PATH.  It opens a floating review pane at 90% width and height in the invoking pane's tab, closes it on exit, and restores focus to the invoking pane.  Session and pane IDs are pinned, including when another tab is selected.  Detection requires inherited `ZELLIJ_SESSION_NAME` and `ZELLIJ_PANE_ID`; ancestry selects the nearest supported multiplexer when nested.  Zellij does not expose pane process IDs, so detection without inherited identity is unavailable.  The wrapper and notifying terminal filters preserve the identity for terminal-marker lookup.  `critty env` returns `ZELLIJ=0`, the session name, pane ID, and `ZELLIJ_SOCKET_DIR` when set.  A custom socket directory is preserved in marker registrations and launcher requests.

## Terminal environment

`critty env` locates the review terminal the same way a review launch does, from the multiplexer environment or from process ancestry, and prints the variables that address it.  See the [launcher requirements](#review-launchers) for each integration's variables and detection limits.

```bash
critty env                       # NAME=value lines for the detected terminal
critty env --json                # the same as one JSON object
critty env tty                   # only the terminal device, such as /dev/ttys003
critty env run <command> [args]  # exec the command with those variables set
critty env -- <command> [args]   # the same
```

Flags go before the mode word.  Without a pane, `TTY` is the controlling terminal of the nearest ancestor process, then `SSH_TTY`; the command fails when no terminal or client hints are found.  Hooks that run detached from the agent's terminal can use `critty env tty` to find it.  `--terminal-request <id>` resolves a prepared marker as a review launch would, so it only applies inside a flow that displays one.
