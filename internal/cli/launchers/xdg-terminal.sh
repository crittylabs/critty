#!/bin/sh
set -eu
case "$1" in
    check) exec xdg-terminal-exec --print-cmd ;;
    open) exec xdg-terminal-exec --dir="$3" -- /bin/sh -c "$2" ;;
    close) exit 0 ;;
    *) exit 2 ;;
esac
