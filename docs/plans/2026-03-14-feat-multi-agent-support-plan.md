---
title: "feat: Multi-Agent Support for Crit"
type: feat
status: completed
date: 2026-03-14
origin: docs/brainstorms/2026-03-13-multi-agent-support-brainstorm.md
---

# feat: Multi-Agent Support for Crit

## Overview

Expand crit to support CLI coding agents beyond Claude Code — starting with Codex CLI (OpenAI) and OpenCode. Each agent gets a fully orchestrated review experience (pause agent -> open TUI -> deliver comments -> resume) using the best available mechanism for its environment, without degrading the existing Claude Code flow.

## Problem Statement

Crit currently works exclusively with Claude Code. The human-in-the-loop review pattern is valuable for any agentic coding workflow, but the implementation is coupled to Claude Code through:
- Hard-coded "Claude Code is paused" banner text (`internal/tui/app.go:1578`)
- `claudeStatusBar` style variable (`internal/tui/styles.go:37`)
- `setup-claude` command writing only to `~/.claude/skills/` (`internal/cli/setup_claude.go`)
- SKILL.md files in Claude Code's format (`internal/cli/skill/*/SKILL.md`)
- tmux as the only wait mechanism (`internal/cli/review.go:151-152`)

The actual coupling is light — tmux + filesystem + natural language instructions — making multi-agent support achievable with focused changes.

## Proposed Solution

Introduce a light **agent adapter pattern** and a **file-based signal fallback** for environments without tmux.

**Core principle:** The wait mechanism is determined by the **environment** (tmux available or not), while the **agent type** determines banner text, instruction file format, and setup paths. All agents run the same crit CLI commands (`crit review`, `crit status`). (See brainstorm: docs/brainstorms/2026-03-13-multi-agent-support-brainstorm.md — Decision #4: preserve Claude Code path unchanged.)

### Architecture

```
┌─────────────────────────────────────────────────────┐
│                    crit CLI                          │
│                                                     │
│  ┌─────────────┐  ┌──────────────────────────────┐  │
│  │ crit setup   │  │ crit review --detach --wait  │  │
│  │              │  │                              │  │
│  │ Writes       │  │  ┌─────────┐ ┌───────────┐  │  │
│  │ instruction  │  │  │  tmux   │ │   file    │  │  │
│  │ files per    │  │  │  path   │ │  signal   │  │  │
│  │ agent type   │  │  │ (if $TMUX)│ │ (fallback)│  │  │
│  └──────┬───────┘  │  └────┬────┘ └─────┬─────┘  │  │
│         │          │       │            │         │  │
│  ┌──────┴───────┐  │  Same TUI, same   │         │  │
│  │AgentAdapter  │  │  comment storage   │         │  │
│  │ .Name()      │  │  same crit status  │         │  │
│  │ .BannerText()│  │                    │         │  │
│  │ .SetupFiles()│  └──────────────────────────────┘  │
│  └──────────────┘                                    │
└─────────────────────────────────────────────────────┘
```

## Technical Approach

### Phase 1: Agent Adapter Interface + Generalized TUI

**Goal:** Decouple crit's Go code from Claude Code without changing any user-facing behavior yet.

#### 1.1 Create `internal/agent/` package

New file: `internal/agent/agent.go`

```go
package agent

// SetupScope controls whether instruction files are installed globally or per-project.
type SetupScope int

const (
    ScopeGlobal  SetupScope = iota
    ScopeProject
)

// SetupFile represents a single instruction file to write during setup.
type SetupFile struct {
    RelPath string // Relative path from the scope root (e.g., "skills/crit-code-review/SKILL.md")
    Content []byte
}

// Adapter encapsulates per-agent differences.
type Adapter interface {
    Name() string                             // "Claude Code", "Codex", "OpenCode"
    Detect() bool                             // Is this agent likely running?
    BannerText() string                       // TUI banner when paused
    SetupFiles(scope SetupScope) []SetupFile  // Files to write during setup
    SetupBasePath(scope SetupScope) string    // Root directory for setup files
}
```

The interface is deliberately narrow. The wait mechanism is NOT part of the adapter — it's determined by the environment (tmux vs file signal).

#### 1.2 Implement Claude Code adapter

New file: `internal/agent/claude.go`

```go
package agent

import "os"

type ClaudeCode struct{}

func (c ClaudeCode) Name() string        { return "Claude Code" }
func (c ClaudeCode) BannerText() string  { return "Claude Code is paused — review the document, then press q to submit" }
func (c ClaudeCode) Detect() bool        { return os.Getenv("CLAUDE_CODE") != "" || os.Getenv("CLAUDE_CODE_ENTRYPOINT") != "" }

func (c ClaudeCode) SetupBasePath(scope SetupScope) string {
    if scope == ScopeProject {
        return ".claude"
    }
    return filepath.Join(home(), ".claude")
}

func (c ClaudeCode) SetupFiles(scope SetupScope) []SetupFile {
    // Return the three existing SKILL.md files with their paths
    return []SetupFile{
        {RelPath: "skills/crit-review/SKILL.md", Content: skillCritReview},
        {RelPath: "skills/crit-plan-review/SKILL.md", Content: skillCritPlanReview},
        {RelPath: "skills/crit-code-review/SKILL.md", Content: skillCritCodeReview},
    }
}
```

#### 1.3 Stub Codex and OpenCode adapters

New files: `internal/agent/codex.go`, `internal/agent/opencode.go`

```go
// codex.go
type Codex struct{}

func (c Codex) Name() string       { return "Codex" }
func (c Codex) BannerText() string { return "Codex is paused — review the document, then press q to submit" }
func (c Codex) Detect() bool       { return os.Getenv("CODEX_HOME") != "" }

func (c Codex) SetupBasePath(scope SetupScope) string {
    if scope == ScopeProject {
        return "." // AGENTS.md lives at project root
    }
    return filepath.Join(home(), ".codex")
}

func (c Codex) SetupFiles(scope SetupScope) []SetupFile {
    if scope == ScopeProject {
        return []SetupFile{
            {RelPath: "AGENTS.md", Content: codexAgentsMD, Mode: AppendSection},
        }
    }
    return []SetupFile{
        {RelPath: "skills/crit-review/SKILL.md", Content: codexSkillReview},
        {RelPath: "skills/crit-plan-review/SKILL.md", Content: codexSkillPlanReview},
        {RelPath: "skills/crit-code-review/SKILL.md", Content: codexSkillCodeReview},
    }
}
```

```go
// opencode.go
type OpenCode struct{}

func (c OpenCode) Name() string       { return "OpenCode" }
func (c OpenCode) BannerText() string { return "OpenCode is paused — review the document, then press q to submit" }
func (c OpenCode) Detect() bool       { return os.Getenv("OPENCODE") != "" }

func (c OpenCode) SetupBasePath(scope SetupScope) string {
    if scope == ScopeProject {
        return "."
    }
    return filepath.Join(xdgConfigHome(), "opencode")
}

func (c OpenCode) SetupFiles(scope SetupScope) []SetupFile {
    if scope == ScopeProject {
        return []SetupFile{
            {RelPath: "AGENTS.md", Content: opencodeAgentsMD, Mode: AppendSection},
        }
    }
    return []SetupFile{
        {RelPath: "skills/crit-review/SKILL.md", Content: opencodeSkillReview},
        {RelPath: "skills/crit-plan-review/SKILL.md", Content: opencodeSkillPlanReview},
        {RelPath: "skills/crit-code-review/SKILL.md", Content: opencodeSkillCodeReview},
    }
}
```

#### 1.4 Agent detection + registry

New file: `internal/agent/detect.go`

```go
package agent

// registry is the ordered list of known agents, checked in priority order.
var registry = []Adapter{
    ClaudeCode{},
    Codex{},
    OpenCode{},
}

// Detect returns the first adapter whose Detect() returns true, or nil.
func Detect() Adapter {
    for _, a := range registry {
        if a.Detect() {
            return a
        }
    }
    return nil
}

// ByName returns the adapter matching the given name (case-insensitive), or nil.
func ByName(name string) Adapter {
    // ... lookup by name
}

// All returns all registered adapters.
func All() []Adapter {
    return registry
}
```

#### 1.5 Generalize TUI banner

In `internal/tui/app.go`:

- Add `agentName string` field to `AppModel`
- Pass agent name via new env var `CRIT_AGENT_NAME` (set by the detach mechanism)
- Change `NewApp()` and `NewCodeReviewApp()` to read `CRIT_AGENT_NAME`
- Replace hard-coded banner at line 1578:

```go
// Before:
claudeBanner := claudeStatusBar.Width(m.width).Render(" Claude Code is paused — review the document, then press q to submit")

// After:
bannerText := fmt.Sprintf(" %s is paused — review the document, then press q to submit", m.agentName)
agentBanner := agentStatusBar.Width(m.width).Render(bannerText)
```

In `internal/tui/styles.go`:
- Rename `claudeStatusBar` → `agentStatusBar` (line 37)

#### 1.6 Refactor `crit setup`

Replace `setup-claude` with a unified `crit setup` command:

```go
var setupCmd = &cobra.Command{
    Use:   "setup",
    Short: "Install crit skills for your coding agent",
    RunE: func(cmd *cobra.Command, args []string) error {
        var adapter agent.Adapter

        if agentFlag != "" {
            adapter = agent.ByName(agentFlag)
            if adapter == nil {
                return fmt.Errorf("unknown agent: %s (available: %s)", agentFlag, agent.Names())
            }
        } else {
            adapter = agent.Detect()
            if adapter == nil {
                return fmt.Errorf("could not detect agent; specify with --agent (available: %s)", agent.Names())
            }
        }

        scope := agent.ScopeGlobal
        if setupProject {
            scope = agent.ScopeProject
        }

        // Write all setup files for this agent
        basePath := adapter.SetupBasePath(scope)
        for _, f := range adapter.SetupFiles(scope) {
            // ... write file (with --force check, section append for AGENTS.md)
        }
    },
}
```

Keep `setup-claude` as a hidden alias that prints a deprecation warning and delegates to `crit setup --agent claude`.

---

### Phase 2: File-Based Signal Mechanism

**Goal:** Allow `--detach --wait` to work without tmux, enabling Codex and OpenCode support even outside tmux sessions.

#### 2.1 Signal file design

**Location:** `.crit/review.signal` (not `.lock` — avoids confusion with the existing YAML file locking in `store.go`)

**Contents:**
```json
{"pid": 12345, "created_at": "2026-03-14T10:30:00Z"}
```

**Lifecycle:**
1. `crit review --detach --wait` (without tmux) creates `.crit/review.signal`
2. Prints message: `"Waiting for review... Run 'crit review --code' in another terminal"`
3. Polls for signal file removal (500ms interval)
4. In another terminal, human runs `crit review --code`
5. TUI detects `.crit/review.signal` on startup → sets `detached = true`, shows banner
6. Human reviews, presses `q` → TUI removes `.crit/review.signal`, exits
7. Waiting process detects removal → prints "Review complete", exits 0
8. Agent reads `crit status --code`

**Race condition prevention:** The signal file is created BEFORE the waiting message is printed. The TUI checks for signal file existence, not creation. If the TUI starts before the waiting process, it won't see the signal file and will run in normal (non-detached) mode — which is fine.

**Stale signal cleanup:** Before creating a new signal, check if one exists. If it does, check if the PID is still alive. If not, remove the stale signal and proceed.

#### 2.2 Modify `--detach --wait` to support file signal

In `internal/cli/review.go`, change `runDetachedCodeReview()` and `runDetachedReview()`:

```go
func runDetachedCodeReview() error {
    if os.Getenv("TMUX") != "" {
        // Existing tmux path — UNCHANGED
        return runDetachedCodeReviewTmux()
    }
    // New file-signal path
    return runDetachedCodeReviewSignal()
}

func runDetachedCodeReviewSignal() error {
    sig, err := signal.Create() // Creates .crit/review.signal with PID
    if err != nil {
        return err
    }

    fmt.Fprintln(os.Stderr, "Waiting for review... Run 'crit review --code' in another terminal")

    if reviewWait {
        if err := sig.WaitForRemoval(500 * time.Millisecond); err != nil {
            return err
        }
        fmt.Fprintln(os.Stdout, "Code review complete.")
    }
    return nil
}
```

#### 2.3 TUI signal detection + cleanup on exit

In `internal/tui/app.go`:

- On `NewApp()` / `NewCodeReviewApp()`: check for `.crit/review.signal`
- If found, set `detached = true` and read agent name from signal or `CRIT_AGENT_NAME`
- On quit (the `q` key handler): if signal file exists, remove it

#### 2.4 New package: `internal/signal/`

```go
package signal

// SignalFile represents an active review signal.
type SignalFile struct {
    Path string
    PID  int
}

func Create() (*SignalFile, error)              // Create .crit/review.signal
func Exists() bool                              // Check if signal file exists
func Remove() error                             // Remove signal file
func (s *SignalFile) WaitForRemoval(interval time.Duration) error  // Poll until removed
func CleanStale() error                         // Remove if PID is dead
```

---

### Phase 3: Agent Instruction Files

**Goal:** Write the actual instruction content that teaches each agent the crit review workflow.

#### 3.1 Claude Code SKILL.md files

**No changes.** The existing three SKILL.md files in `internal/cli/skill/` remain exactly as they are. They are embedded and distributed via `crit setup --agent claude`.

#### 3.2 Codex SKILL.md files (user-level)

Codex supports skills at `~/.codex/skills/crit/SKILL.md`. The content is nearly identical to Claude Code's SKILL.md but adapted for Codex's conventions:

- Remove Claude Code-specific frontmatter (`allowed-tools`)
- Adjust the workflow for file-signal blocking when not in tmux
- Same core flow: run `crit review --code --detach --wait`, read `crit status --code`, address comments

New embedded files: `internal/cli/skill/codex/crit-{review,code-review,plan-review}/SKILL.md`

#### 3.3 Codex AGENTS.md section (project-level)

For project-level setup, crit appends a delimited section to `AGENTS.md`:

```markdown
<!-- crit:start -->
## Crit Code Review

When asked to review code or when a review is needed, use the `crit` CLI tool:

1. Run `crit review --code --detach --wait` (blocks until the human finishes reviewing)
2. Run `crit status --code` to read review comments as JSON
3. Address each comment by editing the relevant files
4. Offer to re-review: run step 1 again if the user wants another pass

For plan/document review: `crit review <file> --detach --wait` then `crit status <file>`
<!-- crit:end -->
```

The `<!-- crit:start -->` / `<!-- crit:end -->` markers enable idempotent updates. `crit setup` replaces the section if it exists, appends if it doesn't.

#### 3.4 OpenCode SKILL.md files (user-level)

OpenCode supports skills at `~/.config/opencode/skills/crit/SKILL.md`. Same pattern as Codex, adapted for OpenCode's conventions.

New embedded files: `internal/cli/skill/opencode/crit-{review,code-review,plan-review}/SKILL.md`

#### 3.5 OpenCode AGENTS.md section (project-level)

Same delimited-section pattern as Codex. OpenCode reads `AGENTS.md` at project root.

#### 3.6 OpenCode/Claude Code path overlap

OpenCode falls back to `~/.claude/skills/` if no OpenCode-specific config exists. To prevent Claude Code's tmux-specific instructions from being used by OpenCode, the OpenCode instruction files should be installed to `~/.config/opencode/skills/` explicitly. Document this in `crit setup` output:

```
Installed crit skills for OpenCode to ~/.config/opencode/skills/
Note: OpenCode also reads ~/.claude/skills/ — if you use both agents,
install crit for each: crit setup --agent opencode && crit setup --agent claude
```

---

### Phase 4: Testing + Polish

#### 4.1 Unit tests for agent detection

Test `Detect()` for each adapter by setting/unsetting env vars. Test `ByName()` lookup. Test detection priority order.

#### 4.2 Unit tests for file signal

Test signal file lifecycle: create, exists, remove, stale cleanup. Test `WaitForRemoval` with mock filesystem. Test race conditions (signal removed before polling starts).

#### 4.3 Integration tests for `--detach --wait` with file signal

Extend existing `review_test.go` tests. The current tests use function variables (`runCommand`, `lookPath`) for mocking — extend this pattern for the file-signal path.

#### 4.4 `crit setup` tests

Test each agent's setup files are written to correct locations. Test `--force` overwrites. Test `--project` scope. Test AGENTS.md section append/replace.

#### 4.5 Update README

- Rewrite to be agent-agnostic, with per-agent setup sections
- Keep Claude Code as the primary/recommended flow
- Add Codex and OpenCode sections

#### 4.6 Backwards compatibility

- `crit setup-claude` → hidden alias with deprecation warning
- `CRIT_DETACHED=1` env var → still works (backwards compat)
- `CRIT_AGENT_NAME` env var → new, optional (defaults to generic "Agent" if not set)
- Existing `.claude/skills/` files → still work, not touched

## System-Wide Impact

### Interaction Graph

`crit review --detach --wait` → detects environment (tmux or not) → creates tmux split OR signal file → TUI opens (same code path) → comments saved to `.crit/reviews/` → TUI exits → tmux wait-for OR signal file removed → `crit status` reads comments as JSON. No callbacks, middleware, or observers. The interaction is linear and file-mediated.

### Error Propagation

- Signal file creation failure → `runDetachedCodeReviewSignal` returns error → agent sees non-zero exit
- TUI crash with signal file → stale signal → next `crit review --detach --wait` detects stale PID and cleans up
- tmux path errors → unchanged, existing error handling

### State Lifecycle Risks

- **Stale signal file:** Mitigated by PID-based cleanup before creating new signals
- **Concurrent reviews:** Signal file includes PID for uniqueness. Could extend to `review-<pid>.signal` if needed, but single-signal is simpler for now
- **Shared `.crit/` directory:** Already gitignored via `*` wildcard, signal files are excluded from git automatically

### API Surface Parity

All three agents use the same CLI commands:
- `crit review --code --detach --wait`
- `crit review <file> --detach --wait`
- `crit status --code`
- `crit status <file>`

No agent-specific CLI flags beyond `crit setup --agent <name>`.

## Acceptance Criteria

### Functional Requirements

- [x] `crit setup --agent claude` writes SKILL.md files to `~/.claude/skills/` (identical to current `setup-claude`)
- [x] `crit setup --agent codex` writes SKILL.md files to `~/.codex/skills/crit/`
- [x] `crit setup --agent codex --project` appends crit section to `AGENTS.md`
- [x] `crit setup --agent opencode` writes SKILL.md files to `~/.config/opencode/skills/crit/`
- [x] `crit setup --agent opencode --project` appends crit section to `AGENTS.md`
- [x] `crit setup` without `--agent` auto-detects and writes appropriate files
- [x] `crit setup` without `--agent` and no detection shows helpful error with available agents
- [x] `crit setup-claude` still works (deprecation warning + delegates to `crit setup --agent claude`)
- [x] `crit review --code --detach --wait` in tmux works exactly as before (tmux wait-for)
- [x] `crit review --code --detach --wait` without tmux creates signal file and blocks
- [x] TUI shows correct agent name in banner when `CRIT_AGENT_NAME` is set
- [x] TUI detects and removes `.crit/review.signal` on quit
- [x] Stale signal files (dead PID) are cleaned up automatically
- [x] AGENTS.md section append is idempotent (re-running `crit setup --project` updates, doesn't duplicate)

### Non-Functional Requirements

- [x] Existing Claude Code + tmux flow has zero behavior changes
- [x] No new dependencies added
- [x] All new code has unit test coverage

### Quality Gates

- [x] Existing `review_test.go` tests continue to pass
- [x] New agent detection tests cover all adapters
- [x] New signal file tests cover lifecycle + stale cleanup
- [x] `crit setup` tests cover all agents + both scopes

## Dependencies & Prerequisites

- No external dependencies — all changes are internal Go code
- Codex and OpenCode env var names need verification (run each agent and check `env`)
- Instruction file content for Codex and OpenCode needs testing with actual agents

## Risk Analysis & Mitigation

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Codex/OpenCode env vars change | Medium | Low | Detection also supports `--agent` flag override |
| File signal race conditions | Low | Medium | Signal created before message printed; stale PID cleanup |
| AGENTS.md section markers conflict with user content | Low | Low | Use HTML comments which are invisible in rendered markdown |
| OpenCode reads Claude Code's SKILL.md via fallback | Medium | Medium | Document that users should install for each agent separately |
| Instruction files don't work well with Codex/OpenCode | Medium | High | Test with actual agents before release; iterate on instruction content |

## Future Considerations

- **ACP integration for OpenCode** (~2-3 days): Bidirectional JSON-RPC 2.0 for richer interaction. Defer until file-signal approach proves insufficient.
- **Codex `exec --json` integration** (~0.5-1 day): Parse JSONL event stream for structured feedback delivery. Low effort but unstable schema.
- **Additional agents:** The adapter interface makes adding new agents straightforward — implement the interface, add instruction files, register in the detection list.

## Files to Create/Modify

### New Files
- `internal/agent/agent.go` — Adapter interface + registry
- `internal/agent/claude.go` — Claude Code adapter
- `internal/agent/codex.go` — Codex adapter
- `internal/agent/opencode.go` — OpenCode adapter
- `internal/agent/detect.go` — Detection logic
- `internal/agent/agent_test.go` — Detection + adapter tests
- `internal/signal/signal.go` — File-based signal mechanism
- `internal/signal/signal_test.go` — Signal lifecycle tests
- `internal/cli/setup.go` — New unified setup command
- `internal/cli/skill/codex/crit-code-review/SKILL.md` — Codex code review instructions
- `internal/cli/skill/codex/crit-plan-review/SKILL.md` — Codex plan review instructions
- `internal/cli/skill/codex/crit-review/SKILL.md` — Codex review router instructions
- `internal/cli/skill/opencode/crit-code-review/SKILL.md` — OpenCode code review instructions
- `internal/cli/skill/opencode/crit-plan-review/SKILL.md` — OpenCode plan review instructions
- `internal/cli/skill/opencode/crit-review/SKILL.md` — OpenCode review router instructions

### Modified Files
- `internal/tui/app.go` — Read `CRIT_AGENT_NAME`, parameterize banner text, signal file cleanup on quit
- `internal/tui/styles.go` — Rename `claudeStatusBar` → `agentStatusBar`
- `internal/cli/review.go` — Add file-signal fallback path in `runDetachedReview` / `runDetachedCodeReview`
- `internal/cli/setup_claude.go` — Deprecate, delegate to new `crit setup --agent claude`
- `internal/cli/root.go` — Register new `setup` command
- `README.md` — Update for multi-agent support

### Unchanged
- `internal/review/` — Comment storage, session management (agent-agnostic already)
- `internal/document/` — Document loading (agent-agnostic already)
- `internal/git/` — Git operations (agent-agnostic already)
- `plugin/crit/` — Claude Code plugin distribution (stays Claude Code-specific)
- `.claude-plugin/` — Claude Code marketplace metadata (stays Claude Code-specific)

## Sources & References

### Origin

- **Brainstorm document:** [docs/brainstorms/2026-03-13-multi-agent-support-brainstorm.md](docs/brainstorms/2026-03-13-multi-agent-support-brainstorm.md) — Key decisions carried forward: preserve Claude Code path unchanged, light adapter pattern, file-based signals for non-Claude agents, auto-detect + flag override, unified `crit setup` command.

### Internal References

- tmux detach mechanism: `internal/cli/review.go:150-195` (code review), `internal/cli/review.go:221-270` (single file)
- Claude banner: `internal/tui/app.go:1577-1579`
- Setup command: `internal/cli/setup_claude.go`
- SKILL.md (code review): `internal/cli/skill/crit-code-review/SKILL.md`
- Review storage: `internal/review/store.go`
- Session manifest: `internal/review/session.go`

### External References

- Codex CLI skills: https://developers.openai.com/codex/skills/
- Codex AGENTS.md: https://developers.openai.com/codex/guides/agents-md/
- OpenCode skills: https://opencode.ai/docs/skills/
- OpenCode rules: https://opencode.ai/docs/rules/
- OpenCode ACP: https://opencode.ai/docs/acp/
