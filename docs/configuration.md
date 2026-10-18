# Configuration

Critty uses TOML for user-edited settings.  Comments, tables, and literal strings make launcher lists, prompt overrides, and task retention policies easier to maintain.  Review records remain CritJSON; session metadata, manifests, and JSON command output retain their existing formats.

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
| `history` | Global-only table controlling task retention. |

Global settings are applied first, followed by project settings.  Settings merge as follows:

- Nonempty project `output` and `author` strings override global values.  Empty strings leave the inherited value in place.  An absent author falls back to Git's user name.
- An explicitly supplied project boolean overrides the global boolean, including `false`.  Omission inherits the global value.
- Ignore patterns form a union, preserving first occurrence order.  An empty array adds no patterns and does not clear inherited patterns.
- Project `launchers` replaces the global array.  `[]` clears that array; omission inherits it.  CLI launcher flags still take precedence, and the launcher resolver supplies its normal defaults.
- Prompt tables merge by key.  A project value replaces the corresponding global value and leaves other keys intact.

Unknown keys are ignored by the current loader.  Omit a key when no override is intended; TOML has no `null`.

## History settings

Only the global `config.toml` supplies history policy.  Project history settings do not override it or authorize removing records.  See the [review history design](review-history.md) for task boundaries, archive layout, and retention decisions.

```toml
[history]
start = "auto"
end = "auto"
keep_days = 30
max_bytes = 1073741824
# archive_dir = "/absolute/path/to/archives"
```

Omitted or empty actions mean `auto`.  Routine completion retains originals quietly.  At task start, completed tasks beyond `keep_days` are offered for maintenance.  When allocated backup disk usage in this working directory exceeds `max_bytes`, auto removes the oldest eligible completed tasks in this working directory until usage reaches 80% of the limit, even within `keep_days`.  Ongoing and busy reviews, ungrouped sessions, explicit archives, and the task being finished are protected.  Usage counts allocated blocks, including review directories and counting hard links once.  It is an estimate of registered original-review locations in this working directory, not a disk quota; insufficient eligible records produce `limited` and a reason instead of expanding deletion scope or failing a successful completion.

Set `start` or `end` to `keep`, `archive`, or `delete` only for fixed behavior.  `keep` retains originals at that boundary regardless of capacity.  Fixed start actions use `keep_days`; fixed end actions apply to the named task.  `ask` is also accepted for callers that explicitly request a decision.  `keep_days` and `max_bytes` are positive integers; zero or omission uses 30 days and 1 GiB.  `archive_dir`, when set, must be absolute.  Invocation-level flags affect only that command, and one-time maintenance answers do not update settings.

Ordinary review start/resume emits history warnings on stderr without requiring a separate check.  Checks run at most hourly per working directory and policy, and repeated warning categories are suppressed for seven days.  These diagnostics retain records; retention actions run at explicit history boundaries.  Agents handle command-reported warnings/results or explicit maintenance requests through Critty commands; boundaries alone do not call for independent capacity or age checks.
