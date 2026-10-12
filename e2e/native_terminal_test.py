"""Verify native-terminal recording with real PTYs and subprocesses."""

import json
import os
import pty
import signal
import subprocess
import sys
import tempfile
import time
import unittest
from pathlib import Path

SCRIPT = Path(__file__).with_name("native_terminal.py")


class NativeTerminalTest(unittest.TestCase):
    def launch(self, command: str | list[str], output: Path) -> subprocess.Popen[bytes]:
        master, slave = pty.openpty()
        self.addCleanup(os.close, master)
        env = dict(os.environ)
        env.update(
            NO_COLOR="1",
            FORCE_COLOR="1",
            HERDR_TEST="inherited",
            TMUX="inherited",
            CRITTY_TEST="inherited",
            TERM="native-test",
            TERM_PROGRAM="native-test",
        )
        try:
            proc = subprocess.Popen(
                [
                    sys.executable,
                    str(SCRIPT),
                    "--warmup",
                    "0",
                    "--output",
                    str(output),
                    "--cwd",
                    str(output.parent),
                    "--",
                    *(
                        [sys.executable, "-c", command]
                        if isinstance(command, str)
                        else command
                    ),
                ],
                stdin=slave,
                stdout=slave,
                stderr=subprocess.DEVNULL,
                env=env,
            )
        finally:
            os.close(slave)
        self.addCleanup(self.stop, proc)
        return proc

    @staticmethod
    def stop(proc: subprocess.Popen[bytes]) -> None:
        if proc.poll() is None:
            proc.kill()
        proc.wait(timeout=5)

    def test_streams_environment_and_failure_status(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            output = Path(tmp) / "result"
            proc = self.launch(
                "import os,sys,json; "
                "print(json.dumps({'tty':[os.isatty(i) for i in range(3)],"
                "'env':{k:v for k,v in os.environ.items() if k in "
                "['NO_COLOR','FORCE_COLOR','HERDR_TEST','TMUX','CRITTY_TEST',"
                "'TERM','TERM_PROGRAM','XDG_STATE_HOME']}}),flush=True); "
                "print('日本語のstderr',file=sys.stderr,flush=True); sys.exit(3)",
                output,
            )
            self.assertEqual(proc.wait(timeout=5), 3)
            stdout = json.loads((output / "stdout.ansi").read_text())
            self.assertEqual(stdout["tty"], [True, True, True])
            self.assertEqual(
                set(stdout["env"]), {"TERM", "TERM_PROGRAM", "XDG_STATE_HOME"}
            )
            self.assertEqual(stdout["env"]["TERM"], "native-test")
            result = json.loads((output / "result.json").read_text())
            self.assertEqual(result["cwd"], str(output.parent.resolve()))
            state = Path(stdout["env"]["XDG_STATE_HOME"])
            self.assertLess(len(str(state)), 40)
            self.assertFalse(state.parent.exists())
            self.assertEqual(
                (output / "stderr.ansi").read_text().strip(), "日本語のstderr"
            )
            self.assertEqual(
                json.loads((output / "result.json").read_text())["returncode"], 3
            )

    def test_completed_output_survives_terminal_process_exit(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            output = Path(tmp) / "result"
            proc = self.launch(
                "import sys; print('approved: true',file=sys.stderr); "
                "print('Review approved with no comments')",
                output,
            )
            self.assertEqual(proc.wait(timeout=5), 0)
            self.assertIn("approved: true", (output / "stderr.ansi").read_text())
            self.assertIn("no comments", (output / "stdout.ansi").read_text())
            self.assertEqual(
                json.loads((output / "result.json").read_text())["returncode"], 0
            )

    def test_hangup_retains_partial_output_and_exit_status(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            output = Path(tmp) / "result"
            proc = self.launch(
                "import time; print('ready',flush=True); time.sleep(60)", output
            )
            deadline = time.monotonic() + 5
            while (
                not (output / "stdout.ansi").exists()
                or not (output / "stdout.ansi").stat().st_size
            ):
                if time.monotonic() > deadline:
                    self.fail("child did not start")
                time.sleep(0.01)
            proc.send_signal(signal.SIGHUP)
            self.assertEqual(proc.wait(timeout=5), 137)
            self.assertIn("ready", (output / "stdout.ansi").read_text())
            result = json.loads((output / "result.json").read_text())
            self.assertEqual(result["interrupted"], signal.SIGHUP)
            self.assertEqual(result["returncode"], -signal.SIGKILL)

    def test_missing_executable_is_recorded(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            output = Path(tmp) / "result"
            proc = self.launch(["/no-such-native-test"], output)
            self.assertNotEqual(proc.wait(timeout=5), 0)
            self.assertIn("native_terminal:", (output / "stderr.ansi").read_text())
            self.assertNotEqual(
                json.loads((output / "result.json").read_text())["returncode"], 0
            )


if __name__ == "__main__":
    unittest.main()
