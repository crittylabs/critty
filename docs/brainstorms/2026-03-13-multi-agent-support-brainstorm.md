# Multi-Agent Support for Crit

**Date:** 2026-03-13
**Status:** Brainstorm

## What We're Building

Expand crit to support CLI coding agents beyond Claude Code — starting with **Codex CLI** (OpenAI, Rust) and **OpenCode** (Go/Bubble Tea). Each agent gets a first-class, fully orchestrated review experience (pause agent -> open TUI -> deliver comments -> resume) optimized for its capabilities, without degrading the existing Claude Code flow.

## Why This Approach

Crit's review pattern (human-in-the-loop code review for agentic coding) is agent-agnostic in concept but currently Claude Code-specific in implementation. The coupling is light — tmux + filesystem + natural language skill files — but expanding to other agents requires:

1. Different pause/resume mechanisms per agent
2. Different instruction file formats per agent
3. Agent detection to pick the right behavior automatically

Rather than forcing all agents through a lowest-common-denominator path, we preserve Claude Code's proven `tmux wait-for` flow and add best-fit paths for each new agent.

## Key Decisions

### 1. Full Orchestration Per Agent
Each supported agent gets the full loop: pause -> review in TUI -> deliver comments back -> agent addresses them -> offer re-review. No "lite" mode.

### 2. Agent Detection: Auto-detect + Flag Override
Crit auto-detects which agent is running (via env vars, process inspection) and picks the right adapter. Users can override with `--agent codex` if detection fails.

### 3. Light Adapter Pattern in Go
A small Go interface encapsulates per-agent differences. The core TUI and review logic stays shared.

```go
type AgentAdapter interface {
    Name() string              // "Claude Code", "Codex", "OpenCode"
    Detect() bool              // Can we detect this agent is running?
    BannerText() string        // TUI banner when agent is paused
    WaitMechanism() WaitType   // TmuxWaitFor, FileSignal, etc.
    SetupPath() string         // Where to write instruction files
    SetupContent() string      // What instruction content to write
}
```

### 4. Preserve Claude Code Path Unchanged
The existing `tmux wait-for` mechanism, SKILL.md files, "Claude Code is paused" banner — all stay exactly as they are. New agents get their own adapters, not modifications to the Claude Code flow.

### 5. File-Based Signal for Non-Claude Agents
Codex and OpenCode use a file-based pause mechanism (`.crit/review.lock` or similar). Their instruction files tell them to watch for / poll this file. This is universal and doesn't require agent-specific IPC.

### 6. Agent-Specific Instruction Files
Each agent's setup generates the right file format:

| Agent | File | User-level | Project-level |
|---|---|---|---|
| Claude Code | SKILL.md | `~/.claude/skills/crit/` | `.claude/skills/crit/` |
| Codex | SKILL.md / AGENTS.md | `~/.codex/skills/crit/` | `AGENTS.md` in project root |
| OpenCode | SKILL.md / AGENTS.md | `~/.config/opencode/skills/crit/` | `AGENTS.md` in project root |

Note: OpenCode falls back to Claude Code paths (`~/.claude/skills/`), so a Claude Code user-level install may work for OpenCode out of the box.

### 7. Unified `crit setup` Command
Replace `crit setup-claude` with `crit setup [--agent <name>]`. Auto-detects the agent or accepts explicit flag. Writes the appropriate instruction file.

## Agent Research Summary

### Codex CLI (OpenAI)
- **Language:** Rust (rewritten from TypeScript/Ink)
- **Non-interactive mode:** `codex exec "prompt"` with `--json` for JSONL streaming
- **No native tmux support** — but `tmux send-keys` works (proven by third-party tools)
- **Instruction files:** Full AGENTS.md hierarchy — global (`~/.codex/AGENTS.md`), project root, and subdirectory levels. Also supports a skills system (`~/.codex/skills/`) analogous to Claude Code's.
- **User-level config:** `~/.codex/config.toml` (overridable via `CODEX_HOME`). Global instructions and skills can be installed user-wide, not just per-project.
- **Pause approach:** File-based signal. The AGENTS.md file instructs Codex to run `crit review --detach --wait` which blocks on the file signal.

### OpenCode
- **Language:** Go, Bubble Tea TUI
- **Non-interactive mode:** `opencode -p "prompt"` or `opencode run`
- **IPC options:** ACP (JSON-RPC 2.0 over stdio), HTTP server (`opencode serve`), JS/TS SDK
- **No native tmux support** — but send-keys works
- **Instruction files:** Global rules at `~/.config/opencode/AGENTS.md` or `~/.config/opencode/rules/` directory. Also supports skills (`~/.config/opencode/skills/*/SKILL.md`). Falls back to Claude Code paths (`~/.claude/CLAUDE.md`, `~/.claude/skills/`) if no OpenCode-specific config exists.
- **User-level config:** `~/.config/opencode/opencode.json` (XDG conventions).
- **Pause approach:** File-based signal, same as Codex. Could also explore ACP integration later.

## Scope

### In Scope
- Go adapter interface + implementations for Claude Code, Codex, OpenCode
- Agent auto-detection logic
- `crit setup` command (replacing `crit setup-claude`, with backwards compat)
- Instruction files (AGENTS.md for Codex, RULES.md for OpenCode)
- File-based signal mechanism for non-Claude agents
- Generalized TUI banner text per agent

### Stretch Goals (evaluate during planning)
- **Codex `exec --json` integration** (~0.5-1 day effort): Parse the JSONL event stream from `codex exec` to provide richer feedback delivery. The protocol is unidirectional and simple — just `bufio.Scanner` + `json.Unmarshal`. Risk: the JSONL schema has broken backward compat before and has no formal spec.
- **OpenCode ACP integration** (~2-3 days effort): Full bidirectional JSON-RPC 2.0 over stdio. Richer than file-based signals — supports cancel/abort, streaming updates, structured message delivery. Protocol is well-specified with a published schema, but spec is still at v1 and evolving. No official Go SDK exists (only TS, Python, Kotlin).

### Out of Scope
- Web UI or non-tmux workflows
- Community plugin system for adding new agents

## Open Questions

*None — all key questions resolved during brainstorming.*
