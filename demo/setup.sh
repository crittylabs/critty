#!/bin/sh
# Prepare everything the tapes in this directory need under $DEMO_DIR
# (default: $TMPDIR/critty-demo):
#   bin/critty  the critty build to record
#   work        a Git repository with staged changes (code-review.tape)
#   doc         a Markdown document to review (demo.tape)
#   state       an isolated XDG_STATE_HOME for review sessions
# The fixture contents live in assets/: go.mod, CLAUDE.md, and the base/
# tree form the first commit, and the staged/ tree is copied on top and
# staged.  The directory
# sits outside the checkout because Claude Code reads CLAUDE.md from every
# parent directory, and this repository's file must not leak into the demo.
set -eu

cd "$(dirname "$0")"
assets=$(pwd)/assets
root=$(cd .. && pwd)
tmp=${TMPDIR:-/tmp}
demo=${DEMO_DIR:-${tmp%/}/critty-demo}
work=$demo/work

tmux -L demo kill-server 2>/dev/null || true
rm -rf "$work" "$demo/doc" "$demo/state"
mkdir -p "$demo/bin" "$work" "$demo/doc" "$demo/state"
go build -ldflags "-X github.com/crittylabs/critty/internal/cli.version=${DEMO_VERSION:-}" -o "$demo/bin/critty" "$root/cmd/critty"

cp "$assets/plan.md" "$demo/doc/plan.md"

cd "$work"
git init -q -b main
git config user.name "Demo Reviewer"
git config user.email "reviewer@example.com"
git config commit.gpgsign false

cp "$assets/go.mod" "$assets/CLAUDE.md" . && cp -R "$assets"/base/. .
git add .
git commit -q -m "Greet by name"

cp -R "$assets"/staged/. .
git add .

"$demo/bin/critty" install claude-code >/dev/null
printf '{"theme": "dark", "fastMode": false}\n' >.claude/settings.json
echo "demo fixtures ready under $demo"
