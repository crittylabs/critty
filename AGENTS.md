# Agent instructions

This file is read by coding agents such as Claude Code and Codex.

## Local notes

If `AGENTS.local.md` exists next to this file, read it before starting work and follow it.  It holds a developer's local additions, is not committed, and takes precedence over this file where they differ.

## How we work

- Unless `AGENTS.local.md` says otherwise, review changes with Critty itself, running a build of the current checkout so the review dogfoods the work.  If you replace that build while a review round is open, stop the review and start it again.
- Describe new features in `README.md` at the level of how to use them, not their internal rules, and record every user-visible change, including small fixes, in `CHANGELOG.md` for the next release.
- Update `README.md` for a release in the release preparation commit.
- Follow [crit](https://github.com/tomasz-tomczyk/crit) for specification and compatibility decisions, and check its current implementation and documentation when a change touches the review format or workflow.
- In commit messages, put key names such as `R` or `ctrl+v` and command names in backquotes.

## Development

Build, test, and vet with the standard Go tools:

```bash
go build ./...
go test ./...
go vet ./...
```

See the README for installing the binary and the agent skills, `CHANGELOG.md` for changes by release, and `docs/crit-to-critty.md` for the historical list of changes from upstream crit up to v0.18.0.
