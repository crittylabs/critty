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

[prompts]
"on_finish_approved:diff" = "file:.critty/prompts/approved.md"
```

Quote prompt keys containing `:` or `.`.  These characters are part of the key, not TOML table separators.

`textconv_drivers` enables Git text conversion for matching diff driver names or glob patterns.  `["plist*"]` matches a prefix; `["*"]` enables all configured converters.  `?` and character classes are also supported; invalid patterns are errors.  It defaults to no drivers.  Project arrays replace global arrays, and `[]` disables inherited drivers.  Unmatched image drivers retain inline image previews.  See [Git text conversions](../SKILL.md#git-text-conversions) for coordinates and JSON metadata.

`split` chooses the initial diff view and follows boolean precedence.  Runtime view changes are display-only: source text, comment sides and coordinates, and saved review data remain independent of the view.  Supplied unified patches support both views without original files; unavailable context is never filled from unrelated worktree content.

`math_ratex` is a global-only RaTeX selection: `auto` (the default, also when empty), `off`, a command name, or an absolute path.  Automatic discovery finds `render` on PATH and checks its PNG CLI help.  `off` skips RaTeX while retaining LaTeX fallback.  Project files cannot override this selection.  For Markdown math previews, single-line expressions try the selected or detected executable first; multiline expressions and RaTeX failures use `latex` plus `dvipng` on PATH.  Math delimiters also work inside code blocks, including `suggestion`, `md`, and `markdown`; inline code stays literal.  Missing tools or failed rendering leave the source visible.  No tools are installed automatically.  Use `critty doctor` to check the configured executable and PATH tools; automatic RaTeX detection reads CLI help, but no formulas are rendered and TeX packages are not verified.  See [math previews](https://github.com/crittylabs/critty/blob/main/docs/configuration.md#math-previews) for syntax and limits.

`mermaid_merman` and `mermaid_mmdr` are global-only selections accepting `auto` (default), `off`, a command name, or an absolute path.  Mermaid fences try `merman-cli` on PATH, then `mmdr`; unavailable or failed renderers leave source visible.  Fontconfig's optional `fc-match` selects host fonts automatically.  Global `mermaid_fonts` supplies up to eight absolute font-file paths to Merman only.  Project files cannot choose these executables or fonts, and no tools are installed automatically.  `critty doctor` checks availability without rendering diagrams.  See [Mermaid setup and limits](https://github.com/crittylabs/critty/blob/main/docs/configuration.md#mermaid-previews) for installation, font behavior, and fallback compatibility.

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
