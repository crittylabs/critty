# Storage and installation

## Local storage and command effects

Critty is a local CLI and human review TUI, not a hosted review service.  Review data lives under `$XDG_STATE_HOME/critty` (default `~/.local/state/critty`).  Review processes communicate through local Unix sockets.  The built-in review and comment commands do not upload review contents or publish repository changes.

For editing settings or locating custom prompts, read [configuration](configuration.md).  It covers TOML paths, precedence, and prompt overrides.

| Command | Effects |
| --- | --- |
| `critty view` | View current Git changes, a document, a supplied patch, or attachments in the foreground terminal.  No session, history entry, exports, or configuration migration; review edits and external applications are disabled.  Cannot combine with `--session`, `--task`, `--result`, `--summary`, `--terminal-request`, `--launcher`, or `--clear-attachments`.  Closing exits without an approval result. |
| `comments`, `status` | Read local review state. |
| `doctor` (`check` alias) | Compare installed integrations with embedded files and inspect local Python/MCP prerequisites and image forwarding settings, plus desktop launcher availability, Codex wrapper selection, and tfil detection for the current platform.  `--terminal-request ID` consumes a prepared request to test terminal binding without opening a review.  Desktop checks do not open windows or test automation permission.  Runs bounded Python imports and an image capability probe on its foreground controlling TTY.  Temporarily uses raw terminal input, then restores it; sends a transparent virtual placement, deletes it, and restores the cursor.  Does not start a reviewer, contact a model or package registry, or change stored settings. |
| `terminal prepare` | Remove expired local marker requests, save a random marker with a two-minute expiry, and print its JSON.  No review content is read and no external service is contacted. |
| `terminal notify` | Record a local terminal destination supplied by the terminal filter. |
| `review`, `plan`, `--session` | Read the selected input, save local review state, and open the human TUI, using the detected multiplexer when applicable.  Terminal-marker resolution can read visible text in registered panes. |
| `comment` | Add or update comments in local review state; it does not post a GitHub or GitLab comment. |
| `stop`, `clear` | `stop` closes the review TUI and preserves saved state.  `clear` deletes the selected saved review and attachments. |
| `tmp create` | Create a unique private scratch directory under the current directory's `.critty/tmp/`, returning its absolute path as the JSON `directory` field.  Requires no Git repository and creates no application manifest.  Keep documents and results there until no longer needed, then remove only the returned directory; retain `.critty/tmp/.gitignore`. |
| `tmp prepare`, `tmp snapshot` | Preparation reads explicit regular-file targets beneath `--root` into `.critty/tmp/`; snapshot reads those copies.  Both initialize isolated Git baselines with hooks and content conversions disabled.  Snapshot writes the frozen diff and application-consent attachment.  Originals stay unchanged. |
| `tmp apply` | Verify the saved review and consent, then write or delete registered destinations beneath the prepared root, also required as `--root`.  This can write outside the working directory.  Detect concurrent changes, preserve backups, and journal partial application for retry.  Runs no Git subprocess.  Requires the caller's permission to modify those targets; see the [temporary-tree workflow](../../critty/references/temporary-tree.md). |
| `install` | Write integration files to the selected project or user configuration directories. |
| `env`, `env tty` | Inspect local terminal/process information and print the requested environment or device. |
| `env run`, `env --` | Execute the supplied command with the detected terminal environment.  Assess that command's own effects; the wrapper does not make it read-only or local-only. |

External editors launched from the TUI likewise have their own behavior.  Describe the selected command's actual reads, writes, and subprocesses when an approval request needs clarification; do not treat all Critty commands as read-only or as external publication.

`install --user <target>` selects user scope from any directory; running from the home directory also selects user scope.  Otherwise, installation is relative to the current project root.

| Target | Project | User |
| --- | --- | --- |
| `codex` | `.codex/skills` | `~/.codex/skills` |
| `opencode` | `.opencode` | OpenCode's XDG config directory |
| `antigravity` (`agy`) | `.agents/skills` | `~/.gemini/antigravity-cli/skills` |
| `agents` | `.agents/skills` | `~/.agents/skills` |

`agents` provides shared, host-neutral skill references and comment attribution.  Antigravity skills use the ordinary `critty` names.  Prefer a shared installation or selected host variants, because hosts may discover duplicate names across directories.  Existing installations in old locations are left untouched.  `install all` includes Antigravity; legacy Gemini CLI skills remain available through `install gemini`.
