# Configuration

Critty uses TOML for user-edited settings.  Comments, tables, and literal strings make launcher lists, prompt overrides, and future grouped policies easier to maintain.  Review records remain CritJSON; session metadata, manifests, and JSON command output retain their existing formats.

The loader is [internal/config](../internal/config).  The [agent configuration reference](../internal/cli/skill/critty-cli/references/configuration.md) covers editing settings.

## Paths and syntax

| Scope | Canonical file |
| --- | --- |
| Global | `$XDG_CONFIG_HOME/critty/config.toml`, usually `~/.config/critty/config.toml` |
| Project | `.critty.toml` at the Git repository root, or the current directory outside a repository |

Missing settings files are allowed.  An empty TOML file supplies no overrides.

```toml
# Try these launchers in order.
launchers = ["mux", "desktop-terminal"]
author = "Reviewer"
ignore_patterns = ["*.lock", "vendor/**"]
cleanup_on_approve = false

[prompts]
"on_finish_approved:diff" = "file:.critty/prompts/approved.md"
```

Prompt keys with `:` or `.` must be quoted.  The name is a single map key; an unquoted dot defines nested TOML tables.  Custom prompt files use project `.critty/prompts/` and global `critty/prompts/` directories.

## Supported settings and precedence

| Key | Type and role |
| --- | --- |
| `output` | String selecting review storage. |
| `author` | String used for comment attribution. |
| `ignore_patterns` | Array of glob strings excluded from review input. |
| `launchers` | Ordered array of launcher names or custom launcher specifications. |
| `quiet` | Boolean controlling finish-output verbosity. |
| `cleanup_on_approve` | Boolean enabling removal of eligible approved reviews. |
| `disable_stats` | Boolean disabling review statistics. |
| `prompts` | Table mapping prompt keys to overrides. |

Global settings are applied first, followed by project settings.  Settings merge as follows:

- Nonempty project `output` and `author` strings override global values.  Empty strings leave the inherited value in place.  An absent author falls back to Git's user name.
- An explicitly supplied project boolean overrides the global boolean, including `false`.  Omission inherits the global value.
- Ignore patterns form a union, preserving first occurrence order.  An empty array adds no patterns and does not clear inherited patterns.
- Project `launchers` replaces the global array.  `[]` clears that array; omission inherits it.  CLI launcher flags still take precedence, and the launcher resolver supplies its normal defaults.
- Prompt tables merge by key.  A project value replaces the corresponding global value and leaves other keys intact.

Unknown keys are ignored by the current loader.  Omit a key when no override is intended; TOML has no `null`.
