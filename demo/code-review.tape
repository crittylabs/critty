# Critty — staged-change review loop with Claude Code
#
# Run `make` in this directory; it runs setup.sh (builds critty and prepares
# the fixture under $DEMO_DIR) and records an MP4 with VHS.

Output code-review-claude.mp4

Require tmux
Require claude
Require vhs

Set Shell "zsh"
Set FontSize 18
Set FontFamily "Monaspace Argon NF"
Set Width 1800
Set Height 900
Source theme.tape
Set Padding 20
Set Framerate 24
Set TypingSpeed 50ms
Set WindowBar Colorful

Hide

# Detach from the recording host's multiplexer, use the demo build and an
# isolated review state, and start a private tmux server for the recording.
Type "unset TMUX TMUX_PANE NO_COLOR; export COLORTERM=truecolor; unset -m 'HERDR_*'; export TAPE_DIR=$PWD DEMO_DIR=${DEMO_DIR:-${${TMPDIR:-/tmp}%/}/critty-demo}; export XDG_STATE_HOME=$DEMO_DIR/state PATH=$DEMO_DIR/bin:$PATH; cd $DEMO_DIR/work; clear"
Enter
Type "tmux -L demo -f $TAPE_DIR/tmux.conf new -s demo"
Enter
Sleep 1s

# Launch Claude Code in auto mode; set DEMO_PERMISSION_MODE=bypassPermissions
# if a permission prompt stalls the recording.  On a fresh setup Claude Code
# asks to trust the folder, and Bypass Permissions mode asks for acceptance;
# both default to "No, exit".  Down + Enter picks "Yes" for each dialog that
# appears and is harmless on an empty prompt.
Type "claude --model claude-opus-5-5 --effort low --settings .claude/settings.json --permission-mode ${DEMO_PERMISSION_MODE:-auto}"
Enter
Wait+Screen@60s /Yes, I (trust|accept)|auto mode on|bypass permissions on/
Sleep 1s
Down
Sleep 500ms
Enter
Sleep 1s
Wait+Screen@60s /Yes, I (trust|accept)|auto mode on|bypass permissions on/
Sleep 1s
Down
Sleep 500ms
Enter
Wait+Screen@60s /auto mode on|bypass permissions on/
Sleep 1s

Show

# Ask for a review; the critty skill opens the TUI in a tmux split pane.
Type@40ms `Review the staged changes with Critty. Use "$DEMO_DIR/bin/critty" for every Critty command.`
Sleep 500ms
Enter
Wait+Screen@300s /Comments \(\d+\)/
Sleep 2s

# Switch to internal/config/config.go, move to the line that discards the
# parse error, and comment on it.
Tab
Sleep 800ms
Type@60ms "jjjjjjjjjjjjjjjjjjjj"
Sleep 800ms
Enter
Sleep 800ms
Type@40ms "Don't drop the parse error. Return it so main can report a bad GREET_COUNT."
Sleep 800ms
Ctrl+S
Sleep 1.5s

# Finish the round: the TUI closes and Claude Code addresses the comment.
Type "q"
Sleep 800ms
Enter

# The next round opens with Claude's reply focused in the thread.
Wait+Screen@600s /↳ /
Sleep 3s

# Resolve the thread and approve.
Type "R"
Sleep 1.5s
Enter
Sleep 8s
