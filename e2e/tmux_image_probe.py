"""Check image probing with isolated tmux clients and simulated Kitty replies."""

import argparse
import fcntl
import json
import os
import pty
import re
import select
import shlex
import shutil
import struct
import subprocess
import sys
import tempfile
import termios
import time
from collections.abc import Callable
from pathlib import Path

sys.dont_write_bytecode = True

from native_terminal import environment


def verify(binary: Path, tmux_binary: Path) -> None:
    with tempfile.TemporaryDirectory(prefix="ct-images-", dir="/tmp") as directory:
        root = Path(directory)
        env = environment(root)
        env["TERM"] = "xterm-256color"
        socket = str(root / "tmux")
        clients: list[tuple[subprocess.Popen[bytes], int]] = []
        pending: list[bytes] = []
        queries: list[int] = []
        (root / "bin").mkdir()
        (root / "bin/tmux").symlink_to(tmux_binary)
        env["PATH"] = f"{root / 'bin'}:{env['PATH']}"

        def tmux(*args: str) -> str:
            return subprocess.check_output(
                [str(tmux_binary), "-S", socket, *args], env=env, text=True
            ).strip()

        def pump() -> None:
            for i, (_, master) in enumerate(clients):
                while select.select([master], [], [], 0)[0]:
                    chunk = os.read(master, 65536)
                    pending[i] += chunk
                    # Reply only to completed virtual-placement requests.  The
                    # cursor report follows in the same passthrough envelope.
                    pattern = rb"\x1b_Ga=T,[^\x1b]*i=(\d+);[^\x1b]*\x1b\\\x1b\[6n"
                    end = 0
                    for match in re.finditer(pattern, pending[i]):
                        queries[i] += 1
                        os.write(master, b"\x1b_Gi=" + match[1] + b";OK\x1b\\\x1b[2;1R")
                        end = match.end()
                    pending[i] = pending[i][end:][-65536:]

        def wait(predicate: Callable[[], bool], description: str) -> None:
            deadline = time.monotonic() + 10
            while time.monotonic() < deadline:
                pump()
                if predicate():
                    return
                time.sleep(0.02)
            raise AssertionError(f"timeout: {description}")

        def attach(session: str) -> None:
            master, slave = pty.openpty()
            fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 120, 0, 0))
            process = subprocess.Popen(
                [str(tmux_binary), "-S", socket, "attach-session", "-t", session],
                stdin=slave,
                stdout=slave,
                stderr=slave,
                env=env,
                start_new_session=True,
            )
            os.close(slave)
            clients.append((process, master))
            pending.append(b"")
            queries.append(0)
            wait(
                lambda: (
                    len(tmux("list-clients", "-F", "#{client_name}").splitlines())
                    == len(clients)
                ),
                "attached client",
            )

        def check(name: str, supported: bool) -> None:
            before = queries.copy()
            output = root / f"{name}.json"
            command = shlex.join([str(binary), "doctor", "--json"])
            tmux(
                "send-keys",
                "-t",
                source,
                f"{command} > {shlex.quote(str(output))}",
                "Enter",
            )

            def complete() -> bool:
                try:
                    json.loads(output.read_text())
                    return True
                except (FileNotFoundError, json.JSONDecodeError):
                    return False

            wait(complete, f"{name} doctor report")
            report = json.loads(output.read_text())
            result = next(
                item for item in report["checks"] if item["check"] == "images/terminal"
            )
            assert (result["status"] == "ok") == supported, (name, result)
            if not supported:
                assert result["message"] == (
                    "skipped: current tmux window is not owned by one attached client "
                    "with this pane selected"
                ), (name, result)
            assert queries == [
                count + int(supported and i == 0) for i, count in enumerate(before)
            ], (name, before, queries)
            print(f"PASS tmux image probe: {name}", flush=True)

        try:
            source = tmux(
                "-f",
                "/dev/null",
                "new-session",
                "-d",
                "-s",
                "source",
                "-x",
                "120",
                "-y",
                "40",
                "-P",
                "-F",
                "#{pane_id}",
                "env PS1= sh -i",
            )
            tmux("set-option", "-g", "allow-passthrough", "on")
            attach("source")
            check("single client", True)
            background = tmux(
                "split-window",
                "-d",
                "-t",
                source,
                "-P",
                "-F",
                "#{pane_id}",
                "sleep 300",
            )
            tmux("select-pane", "-t", background)
            check("background pane", False)
            tmux("select-pane", "-t", source)
            other = tmux(
                "new-session",
                "-d",
                "-s",
                "other",
                "-P",
                "-F",
                "#{pane_id}",
                "env PS1= sh -i",
            )
            attach("other")
            check("unrelated session", True)
            window = tmux("display-message", "-p", "-t", source, "#{window_id}")
            tmux("link-window", "-s", window, "-t", "other:")
            check("linked window", False)
            tmux("set-option", "-g", "allow-passthrough", "all")
            check("hidden linked window with passthrough all", False)
            tmux("select-window", "-t", f"other:{window}")
            check("shared pane", False)

            # Bypass Critty's guard once to verify why a hidden linked window
            # must count: passthrough=all sends replies into the other pane.
            other_window = tmux("display-message", "-p", "-t", other, "#{window_id}")
            tmux("select-window", "-t", f"other:{other_window}")
            sinks = [root / "source.reply", root / "other.reply"]
            tmux(
                "send-keys",
                "-t",
                other,
                f"stty raw -echo; cat > {shlex.quote(str(sinks[1]))}",
                "Enter",
            )
            wait(
                lambda: (
                    tmux(
                        "display-message", "-p", "-t", other, "#{pane_current_command}"
                    )
                    == "cat"
                ),
                "other reply sink",
            )
            request = b"\x1b_Ga=T,f=32,s=1,v=1,U=1,c=1,r=1,i=123;AAAAAA==\x1b\\\x1b[6n"
            wrapped = b"\x1bPtmux;" + request.replace(b"\x1b", b"\x1b\x1b") + b"\x1b\\"
            emit = shlex.join(
                ["printf", "%b", "".join(f"\\0{byte:03o}" for byte in wrapped)]
            )
            tmux(
                "send-keys",
                "-t",
                source,
                f"stty raw -echo; {emit}; cat > {shlex.quote(str(sinks[0]))}",
                "Enter",
            )
            wait(
                lambda: all(
                    path.exists() and b"Gi=123;OK" in path.read_bytes()
                    for path in sinks
                ),
                "passthrough replies in both selected panes",
            )
            print(
                "PASS tmux image probe: hidden linked window replies reach another pane",
                flush=True,
            )
        finally:
            subprocess.run(
                [str(tmux_binary), "-S", socket, "kill-server"], env=env, check=False
            )
            for process, master in clients:
                process.wait(timeout=5)
                os.close(master)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path, required=True)
    parser.add_argument("--tmux", type=Path, default=shutil.which("tmux"))
    args = parser.parse_args()
    if args.tmux is None:
        parser.error("tmux is required")
    verify(args.binary.resolve(), args.tmux.resolve())
