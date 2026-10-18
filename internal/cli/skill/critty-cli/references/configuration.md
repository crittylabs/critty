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

For supported keys and full merge rules, see [configuration](https://github.com/crittylabs/critty/blob/main/docs/configuration.md).
