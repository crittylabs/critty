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
| `split` | Boolean enabling side-by-side diffs initially; defaults to `false` (unified). |
| `ignore_patterns` | Array of glob strings excluded from review input. |
| `launchers` | Ordered array of launcher names or custom launcher specifications. |
| `textconv_drivers` | Array of Git diff driver names or glob patterns enabled for text conversion; defaults to `[]`. |
| `quiet` | Boolean controlling finish-output verbosity. |
| `cleanup_on_approve` | Boolean enabling removal of eligible approved reviews. |
| `disable_stats` | Boolean disabling review statistics. |
| `prompts` | Table mapping prompt keys to overrides. |
| `history` | Global-only table controlling task retention. |
| `math_ratex` | Global-only RaTeX selection: `auto` (default), `off`, a command name, or an absolute executable path. |
| `mermaid_merman`, `mermaid_mmdr` | Global-only Mermaid CLI selections with the same `auto`, `off`, command, or absolute path values. |
| `mermaid_fonts` | Global-only array of up to eight absolute font-file paths for Merman; omission enables automatic font selection. |

Global settings are applied first, followed by project settings.  Settings merge as follows:

- Nonempty project `output` and `author` strings override global values.  Empty strings leave the inherited value in place.  An absent author falls back to Git's user name.
- An explicitly supplied project boolean overrides the global boolean, including `false`.  Omission inherits the global value.
- Ignore patterns form a union, preserving first occurrence order.  An empty array adds no patterns and does not clear inherited patterns.
- Project `launchers` replaces the global array.  `[]` clears that array; omission inherits it.  CLI launcher flags still take precedence, and the launcher resolver supplies its normal defaults.
- Project `textconv_drivers` replaces the global array.  `[]` disables inherited conversions; omission inherits the global patterns.  For example, `["plist", "cp932"]` enables those Git diff drivers, `["plist*"]` matches names with that prefix, and `["*"]` enables all configured converters.  Patterns also support `?` and character classes such as `[12]`; invalid patterns are errors.  Unmatched drivers remain disabled, preserving inline image previews.
- Prompt tables merge by key.  A project value replaces the corresponding global value and leaves other keys intact.

Unknown keys are ignored by the current loader.  Omit a key when no override is intended; TOML has no `null`.

The header's `Split: off/on` control or `S` switches between unified and side-by-side diffs for the open review.  Runtime changes do not edit settings or comment coordinates.  Split views show all available source lines, including unchanged lines outside diff hunks.  Narrow panes temporarily use unified rendering and return to two columns when widened.  Both views support supplied unified patches without source files; missing context remains unavailable.

## Math previews

In terminals supporting image previews, Markdown `$...$` and `$$...$$` expressions in documents and comments can be replaced by PNG images.  Inline images share a row with surrounding text and move as a whole when wrapping.  Multiline display expressions hide their source lines while retaining their navigation and review coordinates.  Copying an image keeps its original TeX.  Searching, selecting source lines, or turning Images off reveals the source; comments retain image previews until Images is turned off.  Code blocks also support math, including `suggestion`, `md`, and `markdown` fences; inline code stays literal.  Multiline display expressions must remain in one Markdown paragraph, without blank lines.  Missing tools or rendering failures leave the source visible.  The comment editor keeps editable source and shows previews below it after a typing pause.  Math previews do not implement MathJax extensions.

Critty tries RaTeX for expressions without physical newlines, then falls back to `latex` and `dvipng` on PATH.  Expressions containing newlines go directly to LaTeX.  Line breaks between display delimiters and the expression are excluded; internal line breaks and TeX comments are preserved.  LaTeX needs `amsmath` and `amssymb`.  No renderer is installed or downloaded automatically.  Run `critty doctor` to check executable availability; RaTeX autodetection reads CLI help, but no formulas are rendered and TeX packages are not verified.  Missing optional tools are informational; an unavailable configured RaTeX executable produces a warning.

### Install with mise

Install the [RaTeX](https://github.com/erweixin/RaTeX) PNG CLI for single-line expressions:

```sh
mise use -g 'github:erweixin/RaTeX[matching=ratex-cli]@latest'
```

For multiline expressions and fallback, install LaTeX and dvipng through [TinyTeX](https://yihui.org/tinytex/).  Its mise backend supports macOS and x86_64 Linux.  The postinstall updates the package manager before adding the AMS math packages and PNG converter:

```sh
mise use -g --tool-option 'os=["linux/x64","macos"]' \
  --postinstall 'tlmgr update --self && tlmgr install amsmath amsfonts dvipng' \
  tinytex@latest
```

No Critty configuration is needed.  Check detection, then reopen the document:

```sh
critty doctor
critty view notes.md
```

### Renderer selection

With no configuration, Critty looks for `render` on PATH and checks that its help advertises the RaTeX PNG CLI.  LaTeX and dvipng are also found on PATH.  To disable RaTeX while keeping the LaTeX fallback, set this global option:

```toml
math_ratex = "off"
```

Use `auto` (or omit the setting) for automatic discovery.  To select a wrapper or another installation, set `math_ratex` to its command name or absolute path.  Project files cannot override this selection.  Each renderer command has a three-second deadline.  Expressions are limited to 16 KiB and PNGs to 5 MiB and 16 megapixels.  Results are cached in memory for the open viewer.  LaTeX runs in a temporary directory with shell escape disabled and restrictive TeX file access; dvipng disables Ghostscript.  The subprocesses retain the user's OS permissions, so these controls do not provide an OS sandbox.

## Mermaid previews

In terminals supporting image previews, fenced `mermaid` blocks in documents and comments can appear as diagrams.  Copying keeps the original fence and Mermaid source, and review coordinates stay attached to the source lines.  The comment editor retains editable source and shows the diagram below its closing fence after a typing pause.  Press `I` or `M-i` (`Alt+i`) outside input, use `M-i` in the comment editor, or click **Images** to show or hide images, math, and Mermaid previews together.  Turning previews off does not change the document or editor text.  Searching or selecting document source also reveals its text.

Critty tries [Merman](https://github.com/Latias94/merman), then [mmdr](https://github.com/1jehuang/mermaid-rs-renderer) if it is installed and Merman is missing, fails, or produces an invalid PNG.  If neither renderer succeeds, the source remains visible.  These native CLIs run without a browser.  Their syntax coverage and layout can differ from official Mermaid; mmdr can accept malformed input and treats math labels as literal text.  It is a best-effort preview fallback.  Set `mermaid_mmdr = "off"` to keep the source on Merman failure instead.

### Install with mise

Install Merman, and optionally mmdr for fallback:

```sh
mise use -g 'github:Latias94/merman[matching=merman-cli]@latest'
mise use -g github:1jehuang/mermaid-rs-renderer@latest
critty doctor
```

No Critty configuration is needed when the executables are on PATH.  Global `mermaid_merman` and `mermaid_mmdr` accept `auto` (or omission), `off`, a command name, or an absolute executable path.  Project files cannot override these selections.  Tools are never installed automatically.  `doctor` checks availability, not rendering correctness or complete font coverage.

### Fonts and limits

If Fontconfig's `fc-match` is available, Critty selects a generic sans face and a face matching the diagram's character set.  Merman receives their font files directly, avoiding a full system-font scan that can exceed Merman 0.9.3's limits on font-rich hosts.  mmdr receives the matching installed family through its configuration.  Without Fontconfig, each renderer uses its own system-font defaults.  The result depends on installed fonts; matching does not guarantee every glyph or weight.  Missing glyphs may be blank even when a renderer succeeds.  Install Fontconfig through your OS package manager if automatic discovery fails, or set `mermaid_fonts` globally to explicit font-file paths for Merman.  Those files do not supply fonts to mmdr.

Inputs are limited to 64 KiB.  Each command has a three-second deadline; generated PNGs are limited to 5 MiB, 4096 pixels per side, and 16 megapixels.  Merman applies raster limits before generation; mmdr's reported dimensions are checked before PNG generation.  Results are cached in memory for the open viewer.  Source code and font paths are passed as data, without shell evaluation.  Merman network access remains disabled, and subprocesses retain the user's OS permissions.

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
