"""Record a command in a native terminal with isolated Critty state.

Run inside the terminal being tested, outside tmux or Herdr.  For example:
  python3 e2e/native_terminal.py --output .tmp/native-probe -- \
python3 e2e/probe_images.py --expect supported

Pass an absolute Critty binary path to verify that build.  Output includes
separate raw stdout/stderr logs and result.json.  Temporary state is removed
when the command finishes; use --keep-state to inspect saved reviews.
"""

import argparse
import errno
import fcntl
import json
import os
import pty
import select
import shutil
import signal
import sys
import tempfile
import termios
import time
import tty
from pathlib import Path


class Interrupted(Exception):
    def __init__(self, signum: int) -> None:
        self.signum = signum


def environment(root: Path) -> dict[str, str]:
    env = {
        key: value
        for key, value in os.environ.items()
        if not key.startswith(("HERDR_", "TMUX", "CRITTY_"))
        and key not in ("NO_COLOR", "CLICOLOR", "CLICOLOR_FORCE", "FORCE_COLOR")
    }
    env.update(
        XDG_STATE_HOME=str(root / "state"),
        XDG_CONFIG_HOME=str(root / "config"),
        PWD=str(Path.cwd()),
    )
    return env


def write_all(fd: int, data: bytes) -> None:
    while data:
        data = data[os.write(fd, data) :]


def record(command: list[str], output: Path, env: dict[str, str]) -> int:
    """Forward terminal input and record both output streams without pipe buffering."""
    input_fd = sys.stdin.fileno()
    if not os.isatty(input_fd) or not os.isatty(sys.stdout.fileno()):
        raise RuntimeError("run inside the native terminal being tested")
    output.mkdir(parents=True, exist_ok=False)
    err_master, err_slave = pty.openpty()
    pid, master = pty.fork()
    if pid == 0:
        os.close(err_master)
        os.dup2(err_slave, 2)
        os.close(err_slave)
        try:
            os.execvpe(command[0], command, env)
        except OSError as error:
            print(f"native_terminal: {error}", file=sys.stderr, flush=True)
            os._exit(127)
    os.close(err_slave)
    previous = termios.tcgetattr(input_fd)
    handlers = {}
    status = None
    interrupted = None

    def resize(*_: object) -> None:
        size = fcntl.ioctl(input_fd, termios.TIOCGWINSZ, b"\0" * 8)
        for fd in (master, err_master):
            fcntl.ioctl(fd, termios.TIOCSWINSZ, size)

    def interrupt(signum: int, _: object) -> None:
        raise Interrupted(signum)

    try:
        for signum, handler in (
            (signal.SIGWINCH, resize),
            (signal.SIGHUP, interrupt),
            (signal.SIGTERM, interrupt),
        ):
            handlers[signum] = signal.signal(signum, handler)
        resize()
        tty.setraw(input_fd)
        with (
            (output / "stdout.ansi").open("wb") as out,
            (output / "stderr.ansi").open("wb") as err,
        ):
            streams = {master: (out, 1), err_master: (err, 2)}
            inputs = [input_fd, *streams]
            while streams:
                ready, _, _ = select.select(inputs, [], [])
                for fd in ready:
                    try:
                        data = os.read(fd, 65536)
                    except OSError as error:
                        if error.errno != errno.EIO:
                            raise
                        data = b""
                    if not data:
                        inputs.remove(fd)
                        streams.pop(fd, None)
                    elif fd == input_fd:
                        write_all(master, data)
                    else:
                        log, destination = streams[fd]
                        log.write(data)
                        log.flush()
                        write_all(destination, data)
    except Interrupted as error:
        interrupted = error.signum
    finally:
        # Preserve the result even if the terminal closes or rendering fails.
        try:
            termios.tcsetattr(input_fd, termios.TCSANOW, previous)
        except termios.error:
            pass  # The terminal may have disappeared.
        for signum, handler in handlers.items():
            signal.signal(signum, handler)
        if interrupted is not None or sys.exc_info()[0] is not None:
            try:
                os.killpg(pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
        _, status = os.waitpid(pid, 0)
        os.close(master)
        os.close(err_master)
        result = {
            "command": command,
            "cwd": str(Path.cwd()),
            "returncode": os.waitstatus_to_exitcode(status),
            "interrupted": interrupted,
            "state": env["XDG_STATE_HOME"],
            "terminal": {
                key: env[key]
                for key in ("TERM", "TERM_PROGRAM", "COLORTERM")
                if key in env
            },
        }
        (output / "result.json").write_text(json.dumps(result, indent=2) + "\n")
    return os.waitstatus_to_exitcode(status)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--keep-state", action="store_true")
    parser.add_argument("--cwd", type=Path, default=Path.cwd())
    parser.add_argument(
        "--warmup", type=float, default=2, help="terminal startup delay"
    )
    parser.add_argument("command", nargs=argparse.REMAINDER)
    args = parser.parse_args()
    command = args.command
    if command[:1] == ["--"]:
        command = command[1:]
    if not command or args.warmup < 0:
        parser.error("provide a command and a nonnegative warmup")
    # macOS Unix socket paths have a small limit; checkout paths can be too long.
    root = Path(tempfile.mkdtemp(prefix="ct-", dir="/tmp"))
    try:
        output = args.output.resolve()
        os.chdir(args.cwd)
        time.sleep(args.warmup)
        result = record(command, output, environment(root))
    except (OSError, RuntimeError) as error:
        parser.exit(1, f"native_terminal: {error}\n")
    finally:
        if not args.keep_state:
            shutil.rmtree(root)
    sys.exit(result if result >= 0 else 128 - result)


if __name__ == "__main__":
    main()
