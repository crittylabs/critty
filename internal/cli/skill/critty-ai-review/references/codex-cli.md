# Restricted Codex review

Use the bundled `scripts/codex_review_server.py` to expose research and finding tools over MCP stdio.  It uses the same policy JSON as the Claude gate.  The server executes checked argument vectors directly, fixes their working directory, and accepts no shell, environment, executable, or session overrides from the reviewer.  Its write operations call Critty to add a finding, reply to a policy-listed own finding, or resolve it with a reason.

Codex's Bash hook input contains the command string, not every execution option.  A hook alone cannot validate a caller-selected shell or all tool paths.  Disable native shell execution, external tools, and delegation; retain a read-only sandbox for native file tools.  The MCP server supplies the permitted operations.  Do not grant workspace-write access to make posting work: the server owns the narrow posting operation outside the model's sandbox.

## When Claude requests a Codex review

Claude remains the reviewee and uses its normal, authorized tools to prepare inputs, run checks, launch Codex, and collect results.  The restricted Claude launch in the sibling reference is for a Claude reviewer; it does not apply to this requesting role.  Codex receives the shared skill's reviewer instructions and only the bounded research and findings tools below.  Launch preparation, tests, and empirical checks stay with Claude.

Resolve the user's selected Codex model and configured service, retaining the task's transmission authorization and applicable limits.  Obtain any launch permission from Claude's host using that scope.  The child setting `approval_policy="never"` prevents Codex from requesting broader permissions; it neither grants the parent permission to launch nor changes the parent's approval policy.  The MCP tool approval setting covers only the prepared server's scoped operations.

Resolve and verify the non-interactive Codex CLI executable.  PATH may select a wrapper that registers an interactive terminal, including Critty's Codex wrapper.  Use a supported non-interactive route or the verified CLI executable when registration is inappropriate, retaining required host security controls and authentication.  Record its absolute path and version; inspect its `exec --help` rather than assuming a command named `codex` is the intended binary.

## Prepare

1. Use Python 3.11 or later and install `scripts/codex-review-requirements.txt` into a dedicated environment.  This pins the official MCP Python SDK.  `critty install` copies these resources but does not install Python packages or register an MCP server globally.
2. Prepare the policy described in [restricted CLI review](restricted-cli.md), starting with `read_commands: []`.  Add only necessary static read commands, such as Git history with pagers, external diff, text conversion, and filesystem monitors disabled.  Entries are requester-controlled executable argument vectors, not reviewer input.  The server runs them with stdin closed and a fixed cwd.  Tests, measurements, reproductions, and diagnostic probes remain with the reviewee, including when they would not write files.
3. Run `<python> <skill-dir>/scripts/codex_review_server.py --describe <policy.json>` and include the result in the assignment.  This validates directories and policy without importing MCP or contacting a model.  Fix preparation errors before dispatch.
4. Use a dedicated, empty launch directory.  Inspect its ancestors for project `.codex` configuration and hooks, and use a location without those layers.  Keep policy, server code, and source snapshots unchanged during review.  Supply the task's applicable instructions in the request rather than inheriting unrelated instructions from the launch directory.

## Launch

Check the installed Codex CLI options first.  Use the verified Codex executable, an absolute Python executable from the prepared environment, an absolute server path, and an absolute policy path.  The following template uses the locally inspected Codex 0.163.0 options.  Serialize configuration strings and arrays as TOML values, and use an argument generator for the final shell command rather than interpolating paths into shell or TOML syntax:

```sh
'<absolute-codex>' exec --strict-config --ignore-user-config --ignore-rules \
  --sandbox read-only --ephemeral --skip-git-repo-check --json \
  --model '<selected-model>' -C '<empty-launch-directory>' \
  --output-last-message '<directory>/final-1.txt' \
  -c 'approval_policy="never"' \
  -c 'project_doc_max_bytes=0' \
  -c 'web_search="disabled"' \
  -c 'features.shell_tool=false' \
  -c 'features.shell_snapshot=false' \
  -c 'agents.enabled=false' \
  -c 'features.multi_agent=false' \
  -c 'features.multi_agent_v2=false' \
  -c 'features.apps=false' \
  -c 'features.plugins=false' \
  -c 'features.hooks=false' \
  -c 'features.tool_suggest=false' \
  -c 'features.browser_use=false' \
  -c 'features.computer_use=false' \
  -c 'features.view_image=false' \
  -c 'features.code_mode=false' \
  -c 'features.code_mode_host=true' \
  -c 'mcp_servers.critty_review.command="<absolute-python>"' \
  -c 'mcp_servers.critty_review.args=["-B", "<absolute-server.py>", "<absolute-policy.json>"]' \
  -c 'mcp_servers.critty_review.required=true' \
  -c 'mcp_servers.critty_review.default_tools_approval_mode="approve"' \
  - < '<directory>/request-1.txt'
```

The per-server approval setting applies only to this requester-approved, policy-enforcing server, so permitted research and findings do not prompt mid-review.  Use the user's selected model and existing authentication.  Do not print or copy credentials into the request.  Managed requirements remain authoritative: if they reject the requested restrictions or require additional execution tools, report the incompatibility and use the shared skill's returned-findings fallback.  Do not bypass managed policy.

Before trusting an installation, verify the resulting tool surface: no `exec_command`, shell, `write_stdin`, delegation, browser, or unrelated external service may remain.  Some model catalogs require Code Mode regardless of its feature flag.  Keep the isolated Code Mode host available and inspect its nested tool inventory too; JavaScript orchestration must expose no filesystem, network, or process API of its own.  `agents.enabled=false` also suppresses catalog-selected delegation.  Native file editing must fail under the read-only sandbox.  The permitted external tools must come only from `critty_review`.  If startup reports ignored settings, another MCP server, or failure to start the required server, stop the run rather than treating it as a review.  Configuration and protocol checks can use a local test provider without calling an AI service.

## Reviewer operations

- `capabilities`: current roots, exact read-command indices, comment paths, destination, and denial recovery.
- `read_file`, `list_files`, `search`: bounded UTF-8 reads, paged directory listings, and literal-text search under authorized roots.  Directory traversal omits `.git` and does not follow directory symlinks; resolved file paths must stay within a permitted root.  Search reports skipped non-text, unreadable, or oversized files.  Use `read_command` for approved Git history queries.
- `read_command`: execute one exact requester-approved static read argv by index.  It cannot add arguments, input, shell syntax, or a different cwd.  This interface is not an empirical verification tool.
- `post_finding`: supply a literal body and optional permitted path or source range.  Omit `location` for the overall completion comment required by the shared skill.  Session and author are fixed by policy.  Shell metacharacters in the body remain data.
- `resolve_finding`: supply a policy-listed own comment ID, its exact path (empty for review-level comments), and a nonempty reason.  The server posts the reason as a reply and resolves that thread using the fixed session and author.  Read it back with `comments` to verify persistence.
- `reply_finding`: supply the same ID, path, and reason to reply without changing resolution state.  Use this when your concern is satisfied but another reviewer has an unresolved or unanswered concern about the same issue.
- `comments`: read comments with optional request-ID filtering and pagination.  Read back the intended bodies and IDs after posting.

Follow pagination and report truncation or skipped evidence.  After a rejected read, consult `capabilities`, use an available alternative, and continue.  Tell the reviewee what information is missing, which operation was denied, and how the gap affects the conclusion.  Request needed excerpts or scoped access; the reviewee owns permission changes and empirical verification.  A posting timeout has an uncertain outcome: check `comments` before retrying.

The requester runs tests, checks source hashes before and after, verifies persistence, and collects the final completion report.  Starting the server and running its local tests do not start Codex or incur model usage.

## Collect and continue

Launch through the reviewee's background task or resumable process handle, preserving complete JSONL stdout, stderr, exit status, and the final-message file at fresh paths for each assignment.  In Claude Code, use its background Bash task and completion mechanism; a timeout or tool yield is not process completion.  Keep the handle and frozen inputs until the process has finished, and collect that result before starting another Codex reviewer.

Parse the JSONL events and final report.  Require successful process completion and a completed assessment for the assigned snapshots; `turn.completed` or exit 0 alone does not establish coverage.  An absent final report, `turn.failed`, or a nonzero exit leaves the assignment incomplete.  The Codex thread ID is an execution identifier, separate from the Critty session and review round.  Match the report and saved findings to the latter, using the shared skill's comment readback and relay procedures.  AI completion alone supplies no human approval or commit authorization.

For a follow-up, prepare a new request ID and fresh output paths, supply the current snapshots, prior assessment, comments and reviewee replies, and update the policy's `resolvable_comments` with verified own findings.  Start a fresh ephemeral Codex execution; do not resume an unrelated or implicit last session.  Codex statically verifies the response and decides whether its own finding is satisfied and should resolve.  Claude checks the saved result and records its own agreement.  In cowork, continue this exchange until the shared completion conditions hold; in second-opinion mode, return to the human checkpoint before fixes or another AI assignment.

Sources: [Codex hooks and tool coverage](https://learn.chatgpt.com/docs/hooks), [configuration reference](https://learn.chatgpt.com/docs/config-file/config-reference), and the [official MCP Python SDK](https://github.com/modelcontextprotocol/python-sdk).
