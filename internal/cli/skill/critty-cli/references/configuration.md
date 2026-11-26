# Configuration tasks

Read this reference when editing Critty settings or locating custom prompts.

## Locate and edit

Global settings use `$XDG_CONFIG_HOME/critty/config.toml`, usually `~/.config/critty/config.toml`.  Project settings use `.critty.toml` in the repository root, or the working directory outside a repository.  Custom prompt files use `.critty/prompts/` and the global `critty/prompts/` directory; `critty install prompts` installs these files.

Use a TOML parser and generator.  Preserve unrelated settings and distinguish an omitted value from an explicit `false` or empty array.  Project scalar settings override global values; ignore patterns are unioned, prompt keys are merged, and project `launchers` replaces the global array.  An empty launcher array clears the inherited list.  Follow the caller's authorization requirements before changing live configuration.

```toml
launchers = ["mux", "desktop-terminal"]
textconv_drivers = ["plist", "cp932"] # opt in by Git diff driver name
cleanup_on_approve = false
split = false # true starts diff reviews in side-by-side view
max_wrap_width = 0 # unlimited document text wrapping width
# line_numbers = true # overrides the mode default (view: false, review: true)

[prompts]
"on_finish_approved:diff" = "file:.critty/prompts/approved.md"
```

Quote prompt keys containing `:` or `.`.  These characters are part of the key, not TOML table separators.

`textconv_drivers` is an allowlist of Git diff driver names or glob patterns.  It defaults to `[]`.  A project array replaces the global array, including an empty list that disables inherited drivers.  See [Git text conversions](inputs.md#git-text-conversions) for the reviewed representation and comment metadata.

`split` selects the initial diff layout.  Switching layouts preserves the reviewed source and comment coordinates.

`max_wrap_width` limits document wrapping in terminal columns.  Use `0` for unlimited width or `--max-wrap-width N` to override the setting for one invocation.  `line_numbers` overrides the mode default (off in view, on in review).

`syntax.aliases` maps case-insensitive fence language names to supported languages or other aliases.  `syntax.files` is an ordered array of tables with `pattern` and `language`; patterns match case-sensitive basenames.  Project aliases override global keys, and project file rules precede global rules, with the first match winning.  These settings affect highlighting, supported outlines, and Markdown rendering in reviews and views, including comment fences.  Use `text` to force plain text.  Invalid patterns, unknown languages, and alias cycles fail configuration loading.  See [syntax associations](https://github.com/crittylabs/critty/blob/main/docs/configuration.md#syntax-associations) for examples and limits.

`math_ratex`, `mermaid_merman`, and `mermaid_mmdr` select preview executables in global settings.  Each accepts `auto` (default), `off`, a command name, or an absolute executable path.  `mermaid_fonts` accepts up to eight absolute font paths for Merman.  Check tool availability with `critty doctor`.  See [math previews](https://github.com/crittylabs/critty/blob/main/docs/configuration.md#math-previews) and [Mermaid previews](https://github.com/crittylabs/critty/blob/main/docs/configuration.md#mermaid-previews) for installation and fallbacks.


`nerd_fonts` accepts `auto` (default), `on`, or `off`.  Project values override global values.  Check the detection result with `critty doctor`.  See [font detection and overrides](https://github.com/crittylabs/critty/blob/main/docs/configuration.md#nerd-font-detection).

`remote_image_hosts` is a global-only allowlist for HTTPS image previews in documents, attachments, comments, comment editors, and the lightbox.  It accepts exact DNS hostnames, dot-prefixed suffixes for subdomains, and `*` wildcards within a DNS label.  It defaults to `[]`, so opening review content makes no image requests by default.  Each redirect host must match the allowlist.  See [remote image previews](https://github.com/crittylabs/critty/blob/main/docs/configuration.md#remote-image-previews) for matching examples and connection limits.

`remote_image_cache_max_bytes` is a global-only disk cache limit.  It defaults to 256 MiB.  Set it to `0` to bypass the cache.

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
