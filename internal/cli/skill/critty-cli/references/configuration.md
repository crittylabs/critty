# Configuration tasks

Read this reference when editing Critty settings or locating custom prompts.

## Locate and edit

Global settings use `$XDG_CONFIG_HOME/critty/config.toml`, usually `~/.config/critty/config.toml`.  Project settings use `.critty.toml` in the repository root, or the working directory outside a repository.  Custom prompt files use `.critty/prompts/` and the global `critty/prompts/` directory; `critty install prompts` installs these files.

Use a TOML parser and generator.  Preserve unrelated settings and distinguish an omitted value from an explicit `false` or empty array.  Project scalar settings override global values; ignore patterns are unioned, prompt keys are merged, and project `launchers` replaces the global array.  An empty launcher array clears the inherited list.  Follow the caller's authorization requirements before changing live configuration.

```toml
launchers = ["mux", "desktop-terminal"]
cleanup_on_approve = false

[prompts]
"on_finish_approved:diff" = "file:.critty/prompts/approved.md"
```

Quote prompt keys containing `:` or `.`.  These characters are part of the key, not TOML table separators.

## History policy

Retention settings apply only in the global TOML file.  Omitted or empty `start` and `end` mean `auto`: the CLI retains originals quietly, reports any maintenance decision at task start, and rotates completed backups when capacity is exceeded.  This describes command behavior; it does not instruct agents to inspect old records or usage independently.  Project settings cannot authorize history actions.

```toml
[history]
start = "auto"
end = "auto"
keep_days = 30
max_bytes = 1073741824
# archive_dir = "/absolute/path/to/archives"
```

`keep_days` is the desired retention period, not a minimum guarantee: capacity rotation can remove more recent completed backups.  `max_bytes` budgets allocated disk space in registered original-review locations belonging to the current working directory, including directory blocks and counting hard links once, excluding explicit archives.  Rotation stops at 80% of the limit and protects ongoing, busy, ungrouped, and just-finished records.  Report a target that cannot be reached; do not widen deletion scope.

Use `keep`, `archive`, or `delete` for fixed boundary behavior.  `ask` is available for an explicit decision request, usually through `--action ask`.  A one-time maintenance answer does not authorize editing global preferences.  Change settings only when the user requests persistent behavior.

For supported keys and full merge rules, see [configuration](https://github.com/crittylabs/critty/blob/main/docs/configuration.md).
