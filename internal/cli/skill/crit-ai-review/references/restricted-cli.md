# Restricted CLI review

The requester prepares inputs, runs tests, and launches the other AI.  The reviewer reads and posts findings; it never edits files or runs checks.  Host bookkeeping such as authentication and logs is separate from reviewer tool permissions.

## Claude Code

Use an isolated launch with `--restricted`, an explicit built-in tool list, no external MCP tools, and the bundled `scripts/claude_review_gate.py` as a `PreToolUse` hook.  The gate requires Python 3.10 or later.  The gate admits `Read`, `Glob`, and `Grep` under authorized read roots.  Bash accepts exact requester-approved read-only commands and comment-list and comment-add commands for one session.  Other tools and commands are denied.

Before launch, the requester creates a policy JSON file containing:

- `cwd`: absolute review working directory.
- `binary`: absolute path to the verified Critty executable.
- `session`: the existing review session ID.
- `author`: the fixed AI reviewer name.
- `locations`: bare permitted file paths from the frozen target, without numeric range suffixes.  The reviewer may append a positive line or ascending line range as `:line[-end]`; review-level comments need no location.
- `read_roots`: resolved absolute directories authorized for investigation, normally the review workspace.  Read/search paths outside these roots are denied.  Supply CLI excerpts when the installed skill lies outside them.
- `read_commands`: lists of exact argument vectors for read-only commands the requester has checked.  Use absolute executables and review their entire arguments; this is trusted permission configuration, not model input.  Include needed Git history and diff commands, with pagers, external diff drivers, text conversion, and filesystem monitors disabled.  Supply their canonical command strings to the reviewer.  Additional investigations can request another checked command; never add a blanket shell allowance.

Create a separate settings JSON file with a `PreToolUse` matcher of `*`.  Its single command hook invokes the gate with the policy path.  Use an absolute Python interpreter and gate path, quote with a shell-argument generator, set a short timeout, and set `onFailure` to `block`.  Give the reviewer neither write access to these files nor tools that can alter them.  Do not add Bash to `permissions.allow`: a missing hook must leave Bash denied by `dontAsk`.

Run `python3 scripts/claude_review_gate.py --describe <policy.json>` from this skill's directory and include its JSON capability summary in the assignment.  It derives allowed roots, exact command strings, posting rules, and denial recovery from the same policy the hook enforces.  If this command fails, correct the policy before dispatch.  The hook returns this summary for denied operations under a valid policy; configuration or malformed-input failures return an error reason.  A denied optional investigation should leave the reviewer working within the available scope, with the limitation in its report.

Choose read commands that cover the actual target: `git diff HEAD` includes staged and unstaged tracked changes; untracked snapshots still require direct reads.  Supply a checked hashing command over the target paths when independent digest verification is needed.  Otherwise, identify hashes as requester-verified and have the requester compare them before and after the review; the reviewer should not claim to have recomputed them.

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
```

The gate accepts only that canonical POSIX quoting, including the `--` separator.  It checks the full command, fixed session and author, and permitted file paths and numeric ranges.  Flags outside the listed command shapes, environment assignments, pipes, redirections, substitutions, and command chaining are not executable through this interface.  Metacharacters inside a canonically quoted body remain literal text.  Replies, resolving, clearing, session creation, and bulk-file preparation remain with the requester.

Validate the installed CLI's options and hook support first.  Confirm the gate denies tests, edits, and reads outside the authorized roots while admitting workspace research, a supplied Git command, and the selected comment command.  `onFailure: block` closes hook failures; `--restricted` also confines built-in file tools to the launch directories.  A missing settings file can remove the hook entirely, so validate it and inspect startup and tool events rather than treating a successful launch as proof.  If the write boundary cannot be enforced, omit Bash entirely and use the returned-payload transport.  Do not weaken the launch to bypass permissions.

The hook constrains model tool calls, not arbitrary host processes.  Use a trusted executable and read-only review tree, keep inputs stable, and inspect tool events and target hashes afterward.  A changed target invalidates the review.

See the official [Claude CLI reference](https://code.claude.com/docs/en/cli-reference) and [hook decision and failure behavior](https://code.claude.com/docs/en/hooks).  The example uses options available in the locally inspected CLI; check them again when adapting it to another installation.

## Other hosts, including Codex

Use equivalent tool restrictions only when the host exposes them and they have been verified.  A filesystem read-only sandbox alone does not prohibit running tests or arbitrary commands.  Codex's `exec --sandbox read-only` therefore does not by itself meet this workflow's no-execution requirement.  Use a verified tool allowlist or return findings for the requester to post; do not grant broad workspace-write access merely to reach Critty storage.
