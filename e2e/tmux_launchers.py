"""Verify built-in launcher focus and cleanup in an isolated tmux client.

Run with --binary /absolute/path/to/critty and --tmux /absolute/path/to/tmux.
Also runs after scenarios.py.  No user terminal or tmux settings are changed.
"""

import argparse
import fcntl
import os
import pty
import re
import select
import shutil
import signal
import struct
import subprocess
import tempfile
import termios
import time
from collections.abc import Callable
from pathlib import Path


def verify(binary: Path, tmux_binary: Path) -> None:
    with tempfile.TemporaryDirectory(prefix="ct-launch-", dir="/tmp") as directory:
        root = Path(directory)
        socket = str(root / "tmux")
        (root / "bin").mkdir()
        (root / "bin/tmux").symlink_to(tmux_binary)
        env = {
            key: value
            for key, value in os.environ.items()
            if not key.startswith(("TMUX", "HERDR_", "CRITTY_")) and key != "NO_COLOR"
        }
        env.update(
            PATH=f"{root / 'bin'}:{env['PATH']}",
            XDG_STATE_HOME=str(root / "state"),
            XDG_CONFIG_HOME=str(root / "config"),
            TERM="xterm-256color",
        )
        pid, master, process = None, None, None

        def tmux(*args: str) -> str:
            return subprocess.check_output(
                [str(tmux_binary), "-S", socket, *args], env=env, text=True
            ).strip()

        def wait(predicate: Callable[[], object], description: str) -> object:
            deadline = time.monotonic() + 15
            while time.monotonic() < deadline:
                if master is not None:
                    while select.select([master], [], [], 0)[0]:
                        os.read(master, 65536)
                result = predicate()
                if result:
                    return result
                time.sleep(0.05)
            raise AssertionError(f"timeout: {description}")

        def current() -> str:
            return tmux("list-clients", "-F", "#{pane_id}")

        try:
            source = tmux(
                "-f",
                "/dev/null",
                "new-session",
                "-d",
                "-s",
                "launchers",
                "-x",
                "180",
                "-y",
                "60",
                "-P",
                "-F",
                "#{pane_id}",
                "sleep 300",
            )
            tmux("split-window", "-h", "-d", "-t", source, "sleep 300")
            tmux("set-option", "-s", "focus-events", "on")
            usage = next(
                (
                    line
                    for line in tmux("list-commands").splitlines()
                    if line.startswith("new-pane ")
                ),
                "",
            )
            supports_float = (
                "-B border-lines" in usage
                and "-T title" in usage
                and any(
                    "A" in flags for flags in re.findall(r"\[-([A-Za-z]+)\]", usage)
                )
            )
            window = tmux("display-message", "-p", "-t", source, "#{window_id}")
            session = tmux("display-message", "-p", "-t", source, "#{session_id}")
            server_pid = tmux("display-message", "-p", "-t", source, "#{pid}")
            env.update(TMUX=f"{socket},{server_pid},{session[1:]}", TMUX_PANE=source)
            pid, master = pty.fork()
            if pid == 0:
                fcntl.ioctl(0, termios.TIOCSWINSZ, struct.pack("HHHH", 60, 180, 0, 0))
                os.execve(
                    tmux_binary,
                    [
                        str(tmux_binary),
                        "-S",
                        socket,
                        "attach-session",
                        "-t",
                        "launchers",
                    ],
                    env,
                )
            wait(
                lambda: tmux("list-clients", "-F", "#{client_name}"), "attached client"
            )
            plan = root / "plan.md"
            plan.write_text("# Review\n\nLauncher round trip.\n")

            def focus() -> None:
                tmux("select-window", "-t", window)
                tmux("select-pane", "-t", source)
                os.write(master, b"\x1b[I")
                wait(
                    lambda: "focused" in tmux("list-clients", "-F", "#{client_flags}"),
                    "focus in",
                )

            def geometry() -> str:
                return tmux(
                    "display-message",
                    "-p",
                    "-t",
                    source,
                    "#{pane_left},#{pane_top},#{pane_width},#{pane_height}",
                )

            for name, args, state in (
                ("default", [], "source"),
                ("tmux-meta", ["--launcher", "tmux"], "source"),
                ("floating", ["--launcher", "tmux-floating"], "source"),
                ("background-window", [], "other-window"),
                (
                    "background-window-split",
                    ["--launcher", "tmux-split"],
                    "other-window",
                ),
                ("background-terminal", [], "unfocused"),
                ("split", ["--launcher", "tmux-split"], "source"),
            ):
                tmux("select-layout", "-t", window, "even-horizontal")
                focus()
                before = geometry()
                other = ""
                if state == "other-window":
                    other = tmux(
                        "new-window",
                        "-t",
                        "launchers",
                        "-P",
                        "-F",
                        "#{pane_id}",
                        "sleep 300",
                    )
                elif state == "unfocused":
                    os.write(master, b"\x1b[O")
                    wait(
                        lambda: (
                            "focused"
                            not in tmux("list-clients", "-F", "#{client_flags}")
                        ),
                        "focus out",
                    )
                expected = other or source
                out_path, err_path = root / f"{name}.out", root / f"{name}.err"
                with out_path.open("wb") as out, err_path.open("wb") as err:
                    process = subprocess.Popen(
                        [str(binary), str(plan), *args],
                        env=env,
                        stdout=out,
                        stderr=err,
                        cwd=root,
                    )

                    def review_pane(
                        log: Path = err_path, child: subprocess.Popen = process
                    ) -> str:
                        match = re.search(
                            r"Opened review in tmux (?:floating|split) pane (%\d+)",
                            log.read_text(),
                        )
                        if match:
                            return match[1]
                        if child.poll() is not None:
                            raise AssertionError(log.read_text())
                        return ""

                    pane = wait(review_pane, "review pane")
                    wait(
                        lambda pane=pane: (
                            "Comments" in tmux("capture-pane", "-p", "-t", pane)
                        ),
                        "rendered TUI",
                    )
                    if state == "unfocused":
                        assert "focused" not in tmux(
                            "list-clients", "-F", "#{client_flags}"
                        ), "launch activated the terminal"
                        os.write(master, b"\x1b[I")
                        wait(
                            lambda: (
                                "focused"
                                in tmux("list-clients", "-F", "#{client_flags}")
                            ),
                            "focus returned to terminal",
                        )
                        wait(
                            lambda pane=pane: current() == pane,
                            "review selected after terminal focus returns",
                        )
                    else:
                        assert current() == (pane if state == "source" else expected), (
                            err_path.read_text()
                        )
                    if state == "other-window":
                        tmux("select-window", "-t", window)
                        assert current() == pane, (
                            "returning to the source window left input on the agent pane"
                        )
                        tmux(
                            "select-window",
                            "-t",
                            tmux("display-message", "-p", "-t", other, "#{window_id}"),
                        )
                        assert current() == other, "returning changed the other window"
                    floating = (
                        tmux(
                            "display-message", "-p", "-t", pane, "#{pane_floating_flag}"
                        )
                        == "1"
                    )
                    assert floating == (
                        supports_float and args != ["--launcher", "tmux-split"]
                    ), (
                        f"launcher {name}: floating={floating}, capabilities={supports_float}, usage={usage!r}"
                    )
                    if floating:
                        assert geometry() == before, (
                            "floating launch changed source geometry"
                        )
                        assert (
                            tmux(
                                "display-message",
                                "-p",
                                "-t",
                                pane,
                                "#{pane_modal_flag}",
                            )
                            == "0"
                        )
                        if name == "default":
                            tmux("join-pane", "-s", pane, "-t", pane)
                            assert (
                                tmux(
                                    "display-message",
                                    "-p",
                                    "-t",
                                    pane,
                                    "#{pane_floating_flag}",
                                )
                                == "0"
                            )
                    elif name == "split":
                        assert geometry() != before, (
                            "explicit split did not split the source"
                        )
                    tmux("send-keys", "-t", pane, "q")
                    wait(
                        lambda pane=pane: (
                            "Submit" in tmux("capture-pane", "-p", "-t", pane)
                        ),
                        "Submit dialog",
                    )
                    tmux("send-keys", "-t", pane, "Enter")
                    assert process.wait(timeout=15) == 0, err_path.read_text()
                    assert "approved: true" in err_path.read_text(), (
                        err_path.read_text()
                    )
                    assert current() == expected, (
                        "completion changed the user's selection"
                    )
                    if floating:
                        assert geometry() == before, (
                            f"cleanup changed source geometry: {before} -> {geometry()}"
                        )
                    assert (
                        pane
                        not in tmux("list-panes", "-a", "-F", "#{pane_id}").splitlines()
                    ), "review pane leaked"
                if other:
                    other_window = tmux(
                        "display-message", "-p", "-t", other, "#{window_id}"
                    )
                    tmux("kill-window", "-t", other_window)
                print(
                    f"PASS built-in tmux {name}: focus, approval, cleanup and layout",
                    flush=True,
                )
        finally:
            if process is not None and process.poll() is None:
                process.terminate()
                process.wait(timeout=5)
            subprocess.run(
                [str(tmux_binary), "-S", socket, "kill-server"],
                env=env,
                capture_output=True,
                check=False,
            )
            if pid:
                try:
                    os.kill(pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
                os.waitpid(pid, 0)
            if master is not None:
                os.close(master)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path, required=True)
    parser.add_argument("--tmux", type=Path, default=shutil.which("tmux"))
    args = parser.parse_args()
    if args.tmux is None:
        parser.error("tmux is required")
    verify(args.binary.resolve(), args.tmux.resolve())
