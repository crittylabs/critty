"""Check native macOS launcher ownership and best-effort cleanup.

Opens two temporary windows in the selected application. Only validated
created IDs are closed. Use --expect-closed when the terminal configuration
allows windows to close without confirmation; otherwise pending is normal.
"""

import argparse
import shlex
import subprocess
import tempfile
import time
from pathlib import Path

APPS = {
    "ghostty-window": "com.mitchellh.ghostty",
    "iterm-window": "com.googlecode.iterm2",
    "terminal-app-window": "com.apple.Terminal",
}
ROOT = Path(__file__).resolve().parents[1]


def window_ids(app: str) -> set[str]:
    result = subprocess.check_output(
        [
            "/usr/bin/osascript",
            "-e",
            f'tell application id "{app}" to get id of windows',
        ],
        text=True,
        timeout=10,
    )
    return {value.strip() for value in result.strip().split(",") if value.strip()}


def call(script: str, operation: str, value: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["/usr/bin/osascript", "-", operation, value, str(ROOT)],
        input=script,
        text=True,
        capture_output=True,
        check=False,
        timeout=10,
    )


def verify(launcher: str, expect_closed: bool) -> None:
    app = APPS[launcher]
    script = (ROOT / "internal/cli/launchers" / f"{launcher}.applescript").read_text()
    scratch = ROOT / ".tmp"
    scratch.mkdir(exist_ok=True)
    owned: list[tuple[str, Path]] = []
    with tempfile.TemporaryDirectory(
        prefix="desktop-launch-", dir=scratch
    ) as directory:
        try:
            for index in range(2):
                release = Path(directory) / f"release-{index}"
                before = window_ids(app)
                command = "/bin/sh -c " + shlex.quote(
                    f"while ! test -e {shlex.quote(str(release))}; do /bin/sleep 0.01; done"
                )
                opened = call(script, "open", command)
                token = opened.stdout.strip()
                created = window_ids(app) - before
                if opened.returncode or created != {token}:
                    release.touch()
                    raise AssertionError(
                        f"Cannot verify window ownership: {opened.stderr}"
                    )
                owned.append((token, release))
            token, release = owned[-1]
            others = window_ids(app) - {token}
            release.touch()
            time.sleep(0.2)  # Allow the fixture process to exit, as on Submit.
            result = call(script, "close", token)
            deadline = time.monotonic() + 3
            while token in window_ids(app) and time.monotonic() < deadline:
                time.sleep(0.05)
            remaining = window_ids(app)
            if not others <= remaining:
                raise AssertionError("Cleanup closed another window")
            if token not in remaining:
                # Automatic closure and repeated cleanup must both be harmless.
                again = call(script, "close", token)
                if again.returncode:
                    raise AssertionError(again.stderr)
                print(
                    f"PASS {launcher}: identity, cleanup, repeated cleanup, other windows"
                )
            elif expect_closed:
                raise AssertionError(
                    f"Created window remains: {token}; {result.stderr}"
                )
            else:
                print(
                    f"PENDING {launcher}: window {token}; terminal may require confirmation"
                )
        finally:
            for token, release in owned:
                release.touch()
            time.sleep(0.2)
            for token, _ in owned:
                call(script, "close", token)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--launcher", choices=APPS, required=True)
    parser.add_argument("--expect-closed", action="store_true")
    args = parser.parse_args()
    verify(args.launcher, args.expect_closed)


if __name__ == "__main__":
    main()
