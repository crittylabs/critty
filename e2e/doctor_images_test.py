"""Test doctor's foreground TTY image probe without a graphics-capable terminal."""

import argparse
import fcntl
import json
import os
import pty
import re
import select
import signal
import struct
import subprocess
import sys
import tempfile
import termios
import time
import unittest
from pathlib import Path

BINARY: Path | None = None


class DoctorImagesTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.temp = tempfile.TemporaryDirectory(prefix="critty-doctor-images-")
        cls.addClassCleanup(cls.temp.cleanup)
        cls.root = Path(cls.temp.name).resolve()
        cls.binary = BINARY or cls.root / "critty"
        if BINARY is None:
            subprocess.run(
                ["go", "build", "-o", str(cls.binary), "./cmd/critty"],
                cwd=Path(__file__).resolve().parents[1],
                check=True,
            )
        (cls.root / "home").mkdir()
        cls.tmux_bin = cls.root / "bin"
        cls.tmux_bin.mkdir()
        fake_tmux = cls.tmux_bin / "tmux"
        fake_tmux.write_text(
            '#!/bin/sh\n[ "$3" = list-clients ] || exit 1\n'
            'printf "%s\\n" "$CRITTY_TEST_CLIENT_PANE"\n'
        )
        fake_tmux.chmod(0o700)
        cls.env = {
            k: v
            for k, v in os.environ.items()
            if not k.startswith(("TMUX", "HERDR_", "CRITTY_"))
        }
        cls.env.update(
            HOME=str(cls.root / "home"),
            XDG_CONFIG_HOME=str(cls.root / "config"),
            TERM="xterm-256color",
        )
        cls.argv = [str(cls.binary), "doctor", "--json", "--python", sys.executable]

    def test_no_tty_skips_without_error(self) -> None:
        result = subprocess.run(
            self.argv,
            cwd=self.root,
            env=self.env,
            start_new_session=True,
            stdin=subprocess.DEVNULL,
            capture_output=True,
            text=True,
            timeout=10,
            check=True,
        )
        item = self.terminal_result(json.loads(result.stdout))
        self.assertEqual(item["status"], "info")
        self.assertTrue(item["message"].startswith("skipped:"))

    @staticmethod
    def terminal_result(report: dict) -> dict:
        return next(x for x in report["checks"] if x["check"] == "images/terminal")

    def test_foreground_tty_round_trips(self) -> None:
        for mode in (
            "supported",
            "moved",
            "rejected",
            "encoded",
            "lost-two",
            "silent",
            "interrupted",
            "signal",
            "busy",
            "background",
            "unselected",
        ):
            with self.subTest(mode=mode):
                self.check_pty(mode)

    def check_pty(self, mode: str) -> None:
        report = self.root / f"{mode}.json"
        pid, master = pty.fork()
        if pid == 0:
            # /dev/tty remains the controlling terminal; stdout is a JSON file.
            fd = os.open(report, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
            os.dup2(fd, 1)
            os.close(fd)
            os.chdir(self.root)
            if mode == "background":
                child = os.fork()
                if child:
                    _, status = os.waitpid(child, 0)
                    os._exit(os.waitstatus_to_exitcode(status))
                os.setpgrp()
            env = dict(self.env)
            if mode in ("encoded", "unselected"):
                env["TMUX"] = "/not-running,123,0"
                env["TMUX_PANE"] = "%1"
                env["PATH"] = str(self.tmux_bin) + os.pathsep + env["PATH"]
                env["CRITTY_TEST_CLIENT_PANE"] = "%2" if mode == "unselected" else "%1"
            os.execve(self.binary, self.argv, env)
        fcntl.ioctl(master, termios.TIOCSWINSZ, struct.pack("HHHH", 24, 80, 0, 0))
        before = termios.tcgetattr(master)
        if mode == "busy":
            os.write(master, b"pending input\n")
        data, seen = b"", set()
        deadline, reaped = time.monotonic() + 12, False
        try:
            while time.monotonic() < deadline:
                done, status = os.waitpid(pid, os.WNOHANG)
                if done:
                    reaped = True
                    break
                if not select.select([master], [], [], 0.05)[0]:
                    continue
                try:
                    data += os.read(master, 65536)
                except OSError:
                    continue
                for match in re.finditer(rb"a=T,[^;]*i=(\d+);", data):
                    image_id = int(match[1])
                    if image_id in seen:
                        continue
                    seen.add(image_id)
                    if mode == "silent" or mode == "lost-two" and len(seen) <= 2:
                        continue
                    if mode == "signal":
                        os.kill(pid, signal.SIGTERM)
                        continue
                    if mode == "interrupted":
                        response = b"\x03"
                    elif mode == "encoded":
                        response = f"\x1b[95;3uGi={image_id};OK\x1b[92;3u\x1b[91;3u2;1R".encode()
                    else:
                        payload = "ENOTSUP" if mode == "rejected" else "OK"
                        column = 2 if mode == "moved" else 1
                        response = f"\x1b_Gi={image_id};{payload}\x1b\\\x1b[2;{column}R".encode()
                    # Split responses across reads, including their ESC terminators.
                    for byte in response:
                        os.write(master, bytes([byte]))
                        time.sleep(0.001)
            self.assertTrue(reaped, f"probe timed out: {mode}")
            self.assertEqual(os.waitstatus_to_exitcode(status), 0)
            self.assertEqual(before, termios.tcgetattr(master), "TTY was not restored")
            output = report.read_bytes()
            self.assertNotIn(b"\x1b", output)
            item = self.terminal_result(json.loads(output))
            expected = "ok" if mode in ("supported", "encoded", "lost-two") else "warn"
            if mode in ("busy", "background", "unselected"):
                expected = "info"
            self.assertEqual(item["status"], expected, item)
            if mode in ("busy", "background", "unselected"):
                self.assertTrue(item["message"].startswith("skipped:"), item)
            if mode == "silent":
                self.assertEqual(len(seen), 3)
        finally:
            if not reaped:
                os.kill(pid, signal.SIGKILL)
                os.waitpid(pid, 0)
            os.close(master)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path, help="test an existing Critty binary")
    args, remaining = parser.parse_known_args()
    BINARY = args.binary.resolve() if args.binary else None
    unittest.main(argv=[sys.argv[0], *remaining])
