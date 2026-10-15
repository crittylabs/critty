#!/bin/sh
set -eu
case "$1" in
    check) command -v wt.exe >/dev/null && command -v wsl.exe >/dev/null ;;
    # The command carries its cwd and is base64-encoded by Go. Windows
    # Terminal's semicolon command separator never sees shell source or paths.
    open)
        # The shell inside WSL expands the payload argument.
        # shellcheck disable=SC2016
        exec wt.exe -w -1 new-tab wsl.exe --distribution "$4" --exec /bin/sh -c 'exec /bin/sh -c "$(printf %s "$1" | base64 -d)"' critty "$2"
        ;;
    close) exit 0 ;;
    *) exit 2 ;;
esac
