# Restricted CLI review

The reviewee (requester) prepares inputs, runs empirical checks, and launches the other AI.  The reviewer gathers information and makes static findings.  Read and search tools are appropriate; running code, tests, measurements, reproductions, or diagnostic probes belongs to the reviewee.  Host bookkeeping such as authentication and logs is separate from reviewer tool permissions.

## Claude Code

### Static review with relayed findings

Use this path by default.  Launch with `--restricted`, only `Read`, `Glob`, and `Grep`, and no MCP servers.  The reviewee supplies snapshots, instructions, existing comments, and any needed history excerpts.  The reviewer returns findings, replies, its own resolution decisions, and a completion report as bulk JSON; the reviewee posts them under the reviewer author and reads them back under the shared skill's relay contract.  This avoids granting shell or storage-write access to the reviewer and needs no Python gate.

Check the installed CLI's options first.  Prepare an empty MCP file containing `{"mcpServers": {}}` and the request in the returned scratch directory.  Replace `<directory>` and `<selected-model>` below with their actual values:

```sh
claude --restricted --print --model '<selected-model>' \
  --tools Read,Glob,Grep --allowedTools Read,Glob,Grep \
  --permission-mode dontAsk --permission-prompts none \
  --strict-mcp-config --mcp-config '<directory>/empty-mcp.json' \
  --disable-slash-commands --no-session-persistence \
  --output-format stream-json --verbose < '<directory>/request.txt'
```

Verify that startup reports only these three tools and no MCP servers.  `--restricted` confines reads to the working directories; launch from the authorized review workspace and check its symlinks before granting additional directories.  If an information source is inaccessible, the reviewer reports the missing information and affected conclusion to the reviewee and continues permitted investigation.  The reviewee can supply an excerpt or arrange appropriately scoped access; it does not enable test or probe execution for the reviewer.

### Direct Critty posting

When direct posting or additional static read commands are needed, use an isolated launch with `--restricted`, an explicit built-in tool list, no external MCP tools, and the bundled `scripts/claude_review_gate.py` as a `PreToolUse` hook.  The gate requires Python 3.10 or later.  It admits `Read`, `Glob`, and `Grep` under authorized read roots.  Bash accepts exact requester-approved static read commands and scoped Critty comment operations for one session.  Other tools and commands are denied.

Before launch, the requester creates a policy JSON file containing:

- `cwd`: absolute review working directory.
- `binary`: absolute path to the verified Critty executable.
- `session`: the existing review session ID.
- `author`: the fixed reviewer comment author selected under the shared skill's naming rule.
- `resolvable_comments`: optional list of `{"id": "c_123abc", "path": "src/example.py"}` entries verified by the requester as this reviewer's own findings.  These threads permit replies with or without resolution.  Use an empty path for review-level IDs (`r_…`).  Match both ID and path to disambiguate file comment IDs.  Omission permits neither replies nor resolution.  Keep this list fixed during an assignment.
- `locations`: bare permitted file paths from the frozen target, without numeric range suffixes.  The reviewer may append a positive line or ascending line range as `:line[-end]`; review-level comments need no location.
- `read_roots`: resolved absolute directories authorized for investigation, normally the review workspace.  Read/search paths outside these roots are denied.  Supply CLI excerpts when the installed skill lies outside them.
- `read_commands`: start with `[]`; add only exact argument vectors for necessary static reads, such as Git history or diffs.  Use absolute executables and review their entire arguments; this is trusted permission configuration, not model input.  Disable pagers, external diff drivers, text conversion, and filesystem monitors.  Exclude code execution, tests, benchmarks, reproductions, and diagnostic probes even if they appear read-only.  Supply canonical command strings to the reviewer; missing empirical evidence goes to the reviewee, never into this allowlist.

Create a separate settings JSON file with a `PreToolUse` matcher of `*`.  Its single command hook invokes the gate with the policy path.  Use an absolute Python interpreter and gate path, quote with a shell-argument generator, set a short timeout, and set `onFailure` to `block`.  Give the reviewer neither write access to these files nor tools that can alter them.  Do not add Bash to `permissions.allow`: a missing hook must leave Bash denied by `dontAsk`.

Run `python3 scripts/claude_review_gate.py --describe <policy.json>` from this skill's directory and include its JSON capability summary in the assignment.  It derives allowed roots, exact command strings, posting rules, and denial recovery from the same policy the hook enforces.  If this command fails, correct the policy before dispatch.  The hook returns this summary for denied operations under a valid policy; configuration or malformed-input failures return an error reason.  A denied optional investigation should leave the reviewer working within the available scope, with the limitation in its report.

Choose static read commands that cover the actual target: `git diff HEAD` includes staged and unstaged tracked changes; untracked snapshots still require direct reads.  Have the reviewee check source hashes before and after review and identify them as reviewee-verified.  Reading existing checksums is permitted information gathering; it does not establish that the reviewer recomputed them.

The `.tmp/` directory in the command example below is illustrative.  Replace it consistently with the scratch directory used in your environment, and prepare the referenced files there before launch.

From that working directory, a launch using caller-prepared files is:

```sh
claude --restricted --print --model fable \
  --tools Read,Glob,Grep,Bash --allowedTools Read,Glob,Grep \
  --permission-mode dontAsk --permission-prompts none \
  --strict-mcp-config --mcp-config .tmp/ai-review/empty-mcp.json \
  --disable-slash-commands --no-session-persistence \
  --settings .tmp/ai-review/settings.json \
  --output-format stream-json --verbose < .tmp/ai-review/request.txt
```

The empty MCP file contains `{"mcpServers": {}}`.  Replace the model with the user's selected model.  Capture the process output and exit status using the caller's execution tool.  Inspect startup events to confirm the actual model and tools; collect the final result and any permission denials.

The requester supplies exact command examples generated with Python's `shlex.join`:

```text
<binary> comments --session <session> --all --json
<binary> comment --session <session> --author <author> -- <body>
<binary> comment --session <session> --author <author> -- <location> <body>
<binary> comment --session <session> --author <author> --reply-to <own-id> --resolve --path <path> -- <reason>
<binary> comment --session <session> --author <author> --reply-to <own-id> --path <path> -- <reason>
```

For review-level replies, omit `--path`.  Generate allowed reply and resolution prefixes from the gate's capability summary; only the policy's exact ID/path pairs are accepted, and the reason must be nonempty.  Use the reply-only form while another reviewer has an unresolved or unanswered concern about the same issue.

The gate accepts only that canonical POSIX quoting, including the `--` separator.  It checks the full command, fixed session and author, permitted file paths and numeric ranges, and the own-finding resolution list.  Flags outside the listed command shapes, environment assignments, pipes, redirections, substitutions, and command chaining are not executable through this interface.  Metacharacters inside a canonically quoted body remain literal text.  Other replies, clearing, session creation, and bulk-file preparation remain with the requester.

Validate the installed CLI's options and hook support first.  Confirm the gate denies tests, edits, and reads outside the authorized roots while admitting workspace research, a supplied Git command, and the selected comment command.  `onFailure: block` closes hook failures; `--restricted` also confines built-in file tools to the launch directories.  A missing settings file can remove the hook entirely, so validate it and inspect startup and tool events rather than treating a successful launch as proof.  If the write boundary cannot be enforced, omit Bash entirely and use the returned-payload transport.  Do not weaken the launch to bypass permissions.

The hook constrains model tool calls, not arbitrary host processes.  Use a trusted executable and read-only review tree, keep inputs stable, and inspect tool events and target hashes afterward.  A changed target invalidates the review.

See the official [Claude CLI reference](https://code.claude.com/docs/en/cli-reference) and [hook decision and failure behavior](https://code.claude.com/docs/en/hooks).  The example uses options available in the locally inspected CLI; check them again when adapting it to another installation.

## Codex

Use the [restricted Codex launch](codex-cli.md) when Codex is the reviewer.  It disables native shell execution and exposes bounded workspace research and Critty posting through a policy-controlled MCP server.  The requester prepares its Python environment and validates the resulting tool surface before dispatch.

## Other hosts

Use equivalent tool restrictions only when the host exposes them and they have been verified.  A filesystem read-only sandbox alone does not prohibit running tests or arbitrary commands.  Use a verified tool allowlist or return findings for the requester to post; do not grant broad workspace-write access merely to reach Critty storage.  Permission denials are information for the reviewee: report the attempted read, missing evidence, and affected conclusion, continue available investigation, and let the reviewee arrange any additional access or empirical checks.
