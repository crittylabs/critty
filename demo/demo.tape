# Critty — reviewing a Markdown document
#
# Run `make` in this directory; it runs setup.sh (builds critty and copies the
# document under $DEMO_DIR) and records an MP4 with VHS.

Output demo.mp4

Require tmux
Require vhs

Set Shell "zsh"
Set FontSize 18
Set FontFamily "Monaspace Argon NF"
Set Width 1400
Set Height 800
Source theme.tape
Set Padding 20
Set Framerate 24
Set TypingSpeed 50ms
Set WindowBar Colorful

Hide

# Detach from the recording host's multiplexer, use the demo build and an
# isolated review state, and start a private tmux server for the recording.
Type "unset TMUX TMUX_PANE NO_COLOR; export COLORTERM=truecolor; unset -m 'HERDR_*'; export TAPE_DIR=$PWD DEMO_DIR=${DEMO_DIR:-${${TMPDIR:-/tmp}%/}/critty-demo}; export XDG_STATE_HOME=$DEMO_DIR/state PATH=$DEMO_DIR/bin:$PATH; cd $DEMO_DIR/doc; clear"
Enter
Type "tmux -L demo -f $TAPE_DIR/tmux.conf new -s demo"
Enter
Sleep 1s

# Open the document review; Critty opens the TUI in a split pane, which the
# tmux zoom (prefix z) expands to the whole window for the recording.
Type `"$DEMO_DIR/bin/critty" plan.md`
Enter
Wait+Screen@30s /Comments \(\d+\)/
Ctrl+B
Type "z"
Sleep 1s

Show

# Move to the ```json line (line 12) and select the block through line 20.
Type@80ms "jjjjjjjjjjj"
Sleep 800ms
Type "v"
Sleep 400ms
Type@100ms "jjjjjjjj"
Sleep 800ms

# Comment on the selection.
Enter
Sleep 1s
Type@40ms "Add a status field. Consumers need to know whether the deploy succeeded or failed."
Sleep 800ms
Ctrl+S
Sleep 2.5s

Hide

# Finish the review so the session is cleaned up.
Type "q"
Sleep 500ms
Enter
Sleep 1s
