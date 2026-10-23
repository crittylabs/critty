"""Exercise Critty's CLI and TUI in an isolated tmux server without recording."""

import argparse
import concurrent.futures
import json
import os
import re
import shlex
import shutil
import subprocess
import sys
import tempfile
import time
from collections.abc import Callable
from pathlib import Path
from typing import Any

ROOT: Path
BINARY: Path
TMUX: Path
SERVER: str
WORK: Path
STATE: Path
CONFIG: Path


def configure(root: Path, binary: Path, tmux_binary: Path) -> None:
    global ROOT, BINARY, TMUX, SERVER, WORK, STATE, CONFIG
    ROOT, BINARY, TMUX = root, binary, tmux_binary
    SERVER = root.name
    WORK, STATE, CONFIG = root.resolve() / "work", root / "state", root / "config"


def environment() -> dict[str, str]:
    env = {
        k: v
        for k, v in os.environ.items()
        if not k.startswith(("HERDR_", "TMUX", "CRITTY_"))
    }
    env.update(
        XDG_STATE_HOME=str(STATE),
        XDG_CONFIG_HOME=str(CONFIG),
        PATH=f"{ROOT / 'bin'}:{os.environ.get('PATH', '')}",
        TMUX_TMPDIR=str(ROOT),
        TERM="xterm-256color",
        PWD=str(WORK),
    )
    env.pop("NO_COLOR", None)
    return env


def run(args: list[str], check: bool = True) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        args,
        cwd=WORK,
        env=environment(),
        text=True,
        capture_output=True,
        check=check,
        timeout=25,
    )


def tmux(*args: str) -> str:
    return run([str(TMUX), "-L", SERVER, *args]).stdout.strip()


def cli(*args: str, check: bool = True) -> subprocess.CompletedProcess[str]:
    return run([str(BINARY), *args], check=check)


def wait_until(
    condition: Callable[[], Any], description: str, timeout: float = 20
) -> Any:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        value = condition()
        if value:
            return value
        time.sleep(0.1)
    raise AssertionError(f"timeout: {description}")


class Round:
    def __init__(self, name: str, args: list[str]) -> None:
        self.name = name
        self.summary = ROOT / f"{name}.summary.json"
        args = [*args, "--summary", str(self.summary)]
        self.job = ROOT / f"{name}.job.json"
        self.job.write_text(
            json.dumps(
                {
                    "name": name,
                    "args": args,
                    "root": str(ROOT),
                    "binary": str(BINARY),
                    "tmux": str(TMUX),
                }
            )
        )
        self.out = ROOT / f"{name}.stdout"
        self.err = ROOT / f"{name}.stderr"
        self.result = ROOT / f"{name}.exit.json"
        for path in [self.out, self.err, self.result]:
            path.unlink(missing_ok=True)
        command = shlex.join(
            [sys.executable, str(Path(__file__).resolve()), "--child", str(self.job)]
        )
        tmux("new-window", "-t", "scenario", "-n", name, "-c", str(WORK), command)
        self.session = wait_until(self.session_id, f"{name} session")
        self.pane = wait_until(self.tui_pane, f"{name} TUI")
        wait_until(lambda: "Comments" in self.screen(), f"{name} rendered TUI")

    def session_id(self) -> str | None:
        if self.err.exists():
            match = re.search(r"Review session: ([0-9a-f]{12})", self.err.read_text())
            if match:
                return match[1]
        if self.result.exists():
            raise AssertionError(f"launch failed: {self.err.read_text()}")

    def tui_pane(self) -> str | None:
        try:
            panes = tmux(
                "list-panes",
                "-t",
                f"scenario:{self.name}",
                "-F",
                "#{pane_id} #{pane_start_command}",
            ).splitlines()
        except subprocess.CalledProcessError:
            return None
        return next((line.split()[0] for line in panes if "_tui" in line), None)

    def screen(self) -> str:
        return tmux("capture-pane", "-p", "-t", self.pane)

    def keys(self, *keys: str) -> None:
        tmux("send-keys", "-t", self.pane, *keys)
        if keys[-1] == "Escape":
            # Keep a following key from arriving as an Alt combination.
            self.settle()

    def text(self, text: str) -> None:
        tmux("send-keys", "-l", "-t", self.pane, text)

    def settle(self) -> None:
        """Give input time to take effect before asserting that nothing happened."""
        time.sleep(0.3)

    def wait_screen(self, text: str, description: str) -> str:
        def visible() -> str | None:
            screen = self.screen()
            return screen if text in screen else None

        return wait_until(visible, description)

    def click(self, pattern: str, last: bool = False) -> None:
        def target() -> tuple[int, int] | None:
            rows = list(enumerate(self.screen().splitlines(), 1))
            for y, row in reversed(rows) if last else rows:
                if match := re.search(pattern, row):
                    # Button rows contain spaces and single-cell frame/selector glyphs.
                    return match.start() + 1, y
            return None

        x, y = wait_until(target, f"mouse target {pattern}")
        self.text(f"\x1b[<35;{x};{y}M")
        self.text(f"\x1b[<0;{x};{y}M\x1b[<0;{x};{y}m")

    def comment(
        self, body: str, file: bool = True, expected: str | None = None
    ) -> None:
        if file:
            self.keys("f")
        else:
            self.keys("Escape", "Escape", "j", "Enter")
        self.text(body)
        self.keys("C-s")
        try:
            wait_until(
                lambda: any(c["body"] == (expected or body) for c in self.comments()),
                "comment saved",
                timeout=3,
            )
        except AssertionError:
            print(self.screen(), flush=True)
            print(self.comments(), flush=True)
            raise

    def comments(self) -> list[dict[str, Any]]:
        return json.loads(
            cli("comments", "--session", self.session, "--json", "--all").stdout
        )

    def finish(self, approved: bool) -> dict[str, int]:
        needs_confirmation = approved and any(
            not c.get("resolved") for c in self.comments()
        )
        self.keys("q")
        if approved:
            self.keys("a")
        self.keys("Enter")
        if needs_confirmation:
            wait_until(
                lambda: "Resolve all & approve?" in self.screen(),
                "resolve-all confirmation",
            )
            self.keys("y")
        return self.completed(approved)

    def completed(self, approved: bool | None) -> dict[str, int]:
        code = wait_until(
            lambda: (
                json.loads(self.result.read_text()) if self.result.exists() else None
            ),
            f"{self.name} exited",
        )
        err = self.err.read_text()
        if approved is not None:
            assert code["code"] == 0, (code, err)
            assert f"approved: {str(approved).lower()}" in err, err
            summary = json.loads(self.summary.read_text())
            assert summary["session"] == self.session and summary["round"] >= 1
            assert summary["approved"] == approved
            assert summary["verdict"] in ("approve", "reject", "comment")
            assert summary["comments"] == self.comments()
            assert all("content" not in d for d in summary["attachments"])
        return code

    def directory(self) -> Path:
        entries = json.loads(cli("status").stdout)
        return Path(
            next(e["review_path"] for e in entries if e["key"] == self.session)
        ).parent


def child(path: str) -> None:
    job = json.loads(Path(path).read_text())
    configure(Path(job["root"]), Path(job["binary"]), Path(job["tmux"]))
    name = job["name"]
    env = environment()
    env["TMUX"] = os.environ["TMUX"]
    env["TMUX_PANE"] = os.environ["TMUX_PANE"]
    with (
        (ROOT / f"{name}.stdout").open("w") as out,
        (ROOT / f"{name}.stderr").open("w") as err,
    ):
        result = subprocess.run(
            [str(BINARY), *job["args"]],
            cwd=WORK,
            env=env,
            stdout=out,
            stderr=err,
            check=False,
        )
    (ROOT / f"{name}.exit.json").write_text(json.dumps({"code": result.returncode}))


def report(message: str) -> None:
    print(f"PASS {message}", flush=True)


def transient_views() -> None:
    root = ROOT / "dry-view"
    work, state, config = root / "work", root / "state", root / "config"
    for directory in [work, state / "tcrit", config / "tcrit"]:
        directory.mkdir(parents=True)
    (config / "tcrit" / "config.json").write_text('{"author":"Legacy"}')
    (state / "tcrit" / "sentinel").write_text("keep old data")
    env = environment()
    env.update(XDG_STATE_HOME=str(state), XDG_CONFIG_HOME=str(config))
    for args in [
        ["init", "-q"],
        ["config", "user.name", "Test"],
        ["config", "user.email", "test@example.com"],
    ]:
        subprocess.run(["git", *args], cwd=work, env=env, check=True)
    note = work / "notes.md"
    note.write_text("# Notes\n\nOriginal content.\n")
    subprocess.run(["git", "add", "notes.md"], cwd=work, env=env, check=True)
    subprocess.run(["git", "commit", "-qm", "base"], cwd=work, env=env, check=True)
    note.write_text("# Notes\n\nChanged content.\n- [ ] Confirm\n")
    subprocess.run(["git", "add", "notes.md"], cwd=work, env=env, check=True)

    def snapshot() -> dict[str, tuple[int, bytes]]:
        return {
            str(path.relative_to(root)): (path.stat().st_mtime_ns, path.read_bytes())
            for directory in [work, state, config]
            for path in directory.rglob("*")
            if path.is_file()
        }

    form = work / "form.md"
    form.write_text(
        "# Questionnaire\n\n- [ ] Confirm demo <!-- form: id=confirm required -->\n"
    )
    patch = work / "input.diff"
    patch.write_text(
        subprocess.run(
            ["git", "diff", "--cached"],
            cwd=work,
            env=env,
            text=True,
            capture_output=True,
            check=True,
        ).stdout
    )
    before = snapshot()
    for args in [["--help"], ["review", "--help"], ["view", "--help"]]:
        help_text = subprocess.run(
            [str(BINARY), *args],
            cwd=work,
            env=env,
            capture_output=True,
            text=True,
            check=True,
        ).stdout
        assert "--demo" not in help_text and "--dry-run" not in help_text, help_text
    assert snapshot() == before, "help changed files"
    for command in [[], ["review"]]:
        result = subprocess.run(
            [str(BINARY), *command, "--dry-run"],
            cwd=work,
            env=env,
            capture_output=True,
            text=True,
            check=False,
        )
        assert result.returncode != 0 and "unknown flag: --dry-run" in result.stderr, (
            result
        )
        assert snapshot() == before, "removed dry-run option changed files"
    for prefix in [["view"], ["review", "--demo"]]:
        for flag in [
            "session",
            "task",
            "result",
            "summary",
            "terminal-request",
            "launcher",
            "clear-attachments",
        ]:
            option = f"--{flag}" if flag == "clear-attachments" else f"--{flag}=unused"
            result = subprocess.run(
                [str(BINARY), *prefix, option],
                cwd=work,
                env=env,
                text=True,
                capture_output=True,
                check=False,
                timeout=10,
            )
            assert result.returncode != 0 and (
                "cannot be combined" in result.stderr or "unknown flag" in result.stderr
            ), result
            assert snapshot() == before, (
                f"invalid transient invocation changed files: {prefix} {flag}"
            )

    for name, args in [
        ("view-git", ["view"]),
        ("view-staged", ["view", "--staged"]),
        ("view-doc", ["view", "notes.md"]),
        ("view-patch", ["view", "--diff=input.diff"]),
        ("demo-git", ["review", "--demo"]),
        ("demo-doc", ["review", "--demo", "notes.md"]),
        ("demo-forms", ["review", "--demo", "--null", "--attachment=form.md"]),
    ]:
        job = root / f"{name}.job.json"
        job.write_text(
            json.dumps(
                {
                    "name": name,
                    "args": args,
                    "root": str(root),
                    "binary": str(BINARY),
                    "tmux": str(TMUX),
                }
            )
        )
        pane = tmux(
            "new-window",
            "-d",
            "-P",
            "-F",
            "#{pane_id}",
            "-t",
            "scenario",
            "-n",
            name,
            shlex.join(
                [sys.executable, str(Path(__file__).resolve()), "--child", str(job)]
            ),
        )

        def screen(pane: str = pane) -> str:
            return tmux("capture-pane", "-p", "-t", pane)

        demo = name.startswith("demo")
        wait_until(
            lambda demo=demo: ("Submit q" if demo else "Close q") in screen(),
            "transient UI rendered",
        )
        assert ("Demo" if demo else "View") in screen(), screen()
        assert snapshot() == before, "transient mode wrote files during startup"
        if demo:
            if name == "demo-forms":
                tmux("send-keys", "-t", pane, "n", "Space")
                wait_until(
                    lambda: "✓" in screen() and "Confirm demo" in screen(),
                    "demo form answered",
                )
            tmux("send-keys", "-t", pane, "f")
            wait_until(
                lambda: "Add File Comment" in screen(), "demo comment editor opened"
            )
            tmux("send-keys", "-l", "-t", pane, "Disposable demo comment")
            tmux("send-keys", "-t", pane, "C-s")
            wait_until(
                lambda: (
                    "Disposable demo comment" in screen()
                    and "Add File Comment" not in screen()
                ),
                "demo comment saved in memory",
            )
            assert snapshot() == before, "demo persisted edits"
            tmux("send-keys", "-t", pane, "q")
            wait_until(lambda: "Overall comment" in screen(), "demo Submit opened")
            if name == "demo-forms":
                tmux("send-keys", "-t", pane, "a", "Enter")
                wait_until(
                    lambda: "Resolve all & approve?" in screen(),
                    "demo approval confirms unresolved comments",
                )
                tmux("send-keys", "-t", pane, "y")
            elif name == "demo-doc":
                tmux("send-keys", "-t", pane, "q")
                wait_until(
                    lambda: "Abort review?" in screen(), "demo abort confirmation"
                )
                tmux("send-keys", "-t", pane, "y")
            else:
                tmux("send-keys", "-t", pane, "c", "Enter")
        else:
            tmux("send-keys", "-t", pane, "Enter", "f", "Space")
            tmux("send-keys", "-l", "-t", pane, "\x1be")
            wait_until(
                lambda: "editing is disabled" in screen(),
                "view external editor refused",
            )
            assert "Type your" not in screen(), screen()
            assert snapshot() == before, "view wrote files after edit keys"
            if name == "view-doc":
                rows = screen().splitlines()
                y, row = next(
                    (y, row) for y, row in enumerate(rows, 1) if "Close q" in row
                )
                x = row.index("Close q") + 1
                tmux("send-keys", "-l", "-t", pane, f"\x1b[<0;{x};{y}M\x1b[<0;{x};{y}m")
            else:
                tmux("send-keys", "-t", pane, "q")
        result_path = root / f"{name}.exit.json"
        wait_until(result_path.exists, "transient UI closed")
        assert json.loads(result_path.read_text())["code"] == 0
        assert not (root / f"{name}.stdout").read_text()
        assert not (root / f"{name}.stderr").read_text()
        assert snapshot() == before, "transient mode wrote files on exit"
    report(
        "view stays read-only; demo edits comments, answers forms, and submits without review files, exports, or migration"
    )


def scenarios() -> None:
    for path in [WORK, STATE, CONFIG / "tcrit", ROOT / "bin"]:
        path.mkdir(parents=True, exist_ok=True)
    if not (ROOT / "bin" / "tmux").exists():
        (ROOT / "bin" / "tmux").symlink_to(TMUX)
    (CONFIG / "tcrit" / "config.json").write_text(
        json.dumps({"author": "Scenario Reviewer", "cleanup_on_approve": True})
    )
    migrated = cli("status")
    assert json.loads(migrated.stdout) == []
    assert "critty: migrated " in migrated.stderr
    assert (CONFIG / "critty" / "config.toml").is_file()
    assert not (CONFIG / "critty" / "config.json").exists()
    assert not (CONFIG / "tcrit").exists()
    # Once the current path exists, even unusable old data stays unread and quiet.
    (CONFIG / "tcrit").mkdir()
    (CONFIG / "tcrit" / "config.json").write_text("invalid JSON")
    (CONFIG / "critty" / "config.json").write_text("invalid JSON")
    current = cli("status")
    assert json.loads(current.stdout) == []
    assert current.stderr == ""
    report(
        "startup converts old configuration to TOML once; current configuration ignores old data silently"
    )
    (WORK / "plan.md").write_text(
        "# Release plan\n\nDeploy the application.\nConfirm deployment.\n"
    )
    tmux(
        "-f",
        "/dev/null",
        "new-session",
        "-d",
        "-s",
        "scenario",
        "-x",
        "110",
        "-y",
        "38",
        "sleep 600",
    )
    try:
        transient_views()
        paragraph_source = "**日本語 é first\nsecond**\n\n`code\ncontinued`\n"
        (WORK / "paragraph.md").write_text(paragraph_source)
        paragraph = Round("paragraph-markdown", ["paragraph.md"])
        paragraph.wait_screen("continued", "multiline Markdown rendered")
        screen = paragraph.screen()
        assert "second**" not in screen and "`code" not in screen, screen
        assert "continued`" not in screen, screen
        tmux("resize-pane", "-t", paragraph.pane, "-x", "54", "-y", "24")
        paragraph.wait_screen("continued", "multiline Markdown after resize")
        paragraph.click("second")
        paragraph.keys("Enter")
        paragraph.text("Keep source line anchors.")
        paragraph.keys("C-s")
        wait_until(lambda: paragraph.comments(), "multiline Markdown comment saved")
        assert paragraph.comments()[0]["start_line"] == 2, paragraph.comments()
        assert (WORK / "paragraph.md").read_text() == paragraph_source
        paragraph.finish(False)
        report(
            "multiline Markdown keeps formatting, source text and comment lines after resize"
        )
        marker_job = ROOT / "doctor-marker.job.json"
        marker_job.write_text(
            json.dumps(
                {
                    "name": "doctor-marker",
                    "args": ["doctor", "--json"],
                    "root": str(ROOT),
                    "binary": str(BINARY),
                    "tmux": str(TMUX),
                }
            )
        )
        agent_called = ROOT / "unexpected-agent-call"
        for name in ["claude", "codex"]:
            stub = ROOT / "bin" / name
            stub.write_text(
                f"#!/bin/sh\nprintf invoked > {shlex.quote(str(agent_called))}\nexit 99\n"
            )
            stub.chmod(0o755)
        try:
            tmux(
                "new-window",
                "-t",
                "scenario",
                "-n",
                "doctor-marker",
                "-c",
                str(WORK),
                shlex.join(
                    [
                        sys.executable,
                        str(Path(__file__).resolve()),
                        "--child",
                        str(marker_job),
                    ]
                ),
            )
            wait_until(
                lambda: (ROOT / "doctor-marker.exit.json").exists(),
                "local marker diagnosis completed",
            )
            marker_output = (ROOT / "doctor-marker.stdout").read_text()
            diagnosis = json.loads(marker_output)
            marker = next(
                c for c in diagnosis["checks"] if c["check"] == "terminal/marker"
            )
            assert marker["status"] == "ok", marker
            assert "local marker round trip" in marker["message"], marker
            assert "CRITTY-" not in marker_output, marker_output
            assert not agent_called.exists(), "doctor launched an agent CLI"
        finally:
            for name in ["claude", "codex"]:
                (ROOT / "bin" / name).unlink()
        report("doctor checks a local marker without an agent or JSON contamination")

        patch = WORK / "standalone.diff"
        patch.write_text(
            "diff --git a/standalone.go b/standalone.go\n"
            "--- a/standalone.go\n+++ b/standalone.go\n"
            "@@ -10,3 +20,3 @@\n context\n-old call\n+new call\n tail\n"
        )
        formats = Round("diff-formats", [f"--diff={patch}"])
        tmux("resize-pane", "-t", formats.pane, "-x", "100")
        pane_width = int(
            tmux("display-message", "-p", "-t", formats.pane, "#{pane_width}")
        )
        wait_until(
            lambda: (
                len(formats.screen().splitlines()[0].expandtabs(8)) == pane_width - 1
            ),
            "review redrawn at the server's pane width before mouse input",
        )
        formats.click(r"Ignore: none")
        formats.wait_screen("Ignore: WS", "mouse disables whitespace-only changes")
        formats.keys("w")
        formats.wait_screen("Ignore: none", "w restores whitespace changes")
        formats.keys("S")
        formats.wait_screen("Split: on", "side-by-side diff without source files")
        formats.keys("g", "j", "Left", "Enter")
        formats.text("Deleted line remains on its original side.")
        formats.keys("C-s")
        saved = wait_until(formats.comments, "old-side comment saved")
        assert saved[0]["side"] == "old" and saved[0]["start_line"] == 11
        formats.click(r"Split: on")
        formats.wait_screen("Split: off", "mouse disables split diff")
        assert formats.comments() == saved
        assert not (WORK / "standalone.go").exists()
        formats.finish(False)
        report("standalone patch format switching preserves old-side comments")

        history_task = json.loads(
            cli(
                "history", "start", "Review lifecycle", "--project", "Scenario Project"
            ).stdout
        )["task"]["id"]
        default_approval = Round(
            "default-approval", ["plan.md", "--task", history_task]
        )
        version = cli("--version").stdout.strip().removeprefix("critty version ")
        # Keep long development versions visible instead of testing truncation.
        tmux("resize-pane", "-t", default_approval.pane, "-x", "100")
        default_approval.keys("M-?")
        wait_until(
            lambda: f"critty {version}" in default_approval.screen(),
            "Help build version",
        )
        default_approval.keys("Escape")
        default_approval.keys("q", "Enter")
        default_approval.completed(True)
        report(
            "Submit defaults to Approve when no comments are unresolved; Enter sends it"
        )

        unchanged = Round("unchanged-feedback", ["plan.md", "--task", history_task])
        cli(
            "comment",
            "--session",
            unchanged.session,
            "--author",
            "Scenario Agent",
            "plan.md",
            "Existing feedback.",
        )
        unchanged.keys("q", "BTab")
        unchanged.text("Preserve draft.")
        unchanged.keys("Tab", "a", "Enter")
        wait_until(
            lambda: "Resolve all & approve?" in unchanged.screen(),
            "existing feedback confirmation",
        )
        unchanged.keys("n")
        unchanged.settle()
        assert not unchanged.result.exists(), (
            "declining resolve-all submitted the round"
        )
        assert not any(c.get("resolved") for c in unchanged.comments()), (
            "declining resolved feedback"
        )
        unchanged.wait_screen("Preserve draft", "draft after declining resolve-all")
        unchanged.keys("Enter", "y")
        unchanged.completed(True)
        assert all(c.get("resolved") for c in unchanged.comments())
        report(
            "existing feedback defaults to Approve; declining resolve-all keeps the draft; confirming approves"
        )

        task_sessions = {default_approval.session, unchanged.session}
        assert task_sessions <= {
            session["key"] for session in json.loads(cli("status").stdout)
        }, "task reviews were removed by approval cleanup"
        quiet = json.loads(cli("history", "finish", history_task).stdout)
        assert not quiet["decision_required"] and not quiet["task"]["pruned"]
        assert not quiet.get("maintenance") and quiet["storage"]["bytes"] > 0
        assert not json.loads(cli("history", "usage").stdout)["exceeded"]
        retained = Round("retained-inspection", ["--session", default_approval.session])
        retained.keys("q", "Enter")
        retained.completed(True)
        details = next(
            task
            for task in json.loads(cli("history", "list").stdout)
            if task["id"] == history_task
        )
        assert (
            details["completed_at"] and details["updated_at"] and details["bytes"] > 0
        )
        assert all(
            session["task"] == history_task and not session["ungrouped"]
            for session in json.loads(cli("status").stdout)
            if session["key"] in task_sessions
        )
        assert cli(
            "clear", "--session", default_approval.session, check=False
        ).returncode
        deferred = json.loads(cli("history", "defer", history_task).stdout)
        assert deferred["deferred_until"] and deferred["deferred_update"]
        copied = json.loads(cli("history", "export", history_task).stdout)["archive"]
        inventory = json.loads(cli("history", "archives", "list").stdout)["archives"]
        assert any(item["path"] == copied and item["bytes"] > 0 for item in inventory)
        cli("history", "archives", "verify", copied)
        cli("history", "archives", "delete", copied)
        assert not Path(copied).exists()
        assert task_sessions <= {
            session["key"] for session in json.loads(cli("status").stdout)
        }, "archive deletion removed live reviews"
        report(
            "completed reviews resume without reopening; member-safe clear, persistent deferral, and archive commands round-trip"
        )
        decision = json.loads(
            cli("history", "finish", history_task, "--action", "ask").stdout
        )
        assert decision["decision_required"] and not decision["task"]["pruned"]
        organized = json.loads(
            cli("history", "finish", history_task, "--action", "archive").stdout
        )
        archive = Path(organized["archive"])
        saved = json.loads((archive / "manifest.json").read_text())
        assert saved["task"]["project"] == "Scenario Project"
        assert "Scenario Project - Review lifecycle" in archive.name
        assert {session["key"] for session in saved["sessions"]} == task_sessions
        assert "Existing feedback" in (archive / "review.md").read_text()
        assert organized["task"]["pruned"]
        assert task_sessions.isdisjoint(
            session["key"] for session in json.loads(cli("status").stdout)
        )
        (CONFIG / "critty" / "config.toml").write_text(
            'author = "Scenario Reviewer"\ncleanup_on_approve = false\n'
        )
        report(
            "task reviews defer approval cleanup; one boundary decision archives both sessions before removing originals"
        )

        warning_task = json.loads(
            cli("history", "start", "Old retained work", "--action", "keep").stdout
        )["task"]["id"]
        cli("history", "finish", warning_task, "--action", "keep")
        receipt = STATE / "critty" / "history" / f"{warning_task}.json"
        record = json.loads(receipt.read_text())
        record["started_at"] = "1999-12-31T00:00:00Z"
        record["completed_at"] = "2000-01-01T00:00:00Z"
        receipt.write_text(json.dumps(record))
        (CONFIG / "critty" / "config.toml").write_text(
            'author = "Scenario Reviewer"\ncleanup_on_approve = false\n'
            "[history]\nmax_bytes = 1\n"
        )
        diagnosis = json.loads(cli("doctor", "--json", check=False).stdout)
        history_checks = {item["check"]: item for item in diagnosis["checks"]}
        assert history_checks["history/age"]["status"] == "warn"
        assert "critty history list" in history_checks["history/age"]["advice"]
        assert not json.loads(receipt.read_text())["pruned"]
        for verdict, action in [("approve", "a"), ("reject", "r"), ("comment", "c")]:
            submission = Round(f"submit-{verdict}", ["plan.md"])
            if verdict == "approve":
                wait_until(
                    lambda current=submission: (
                        "critty: warning: history:" in current.err.read_text()
                    ),
                    "history warning during ordinary review startup",
                )
                warning = submission.err.read_text()
                assert "above max_bytes=1" in warning and "keep_days=30" in warning
                assert "critty-history" in warning
                assert not json.loads(receipt.read_text())["pruned"]
            else:
                assert "critty: warning: history:" not in submission.err.read_text()
            submission.keys("q", "BTab")
            body = f"Overall comment: {verdict}."
            submission.text(body)
            submission.wait_screen("● Comment", "first input selects Comment")
            submission.keys("C-s")
            submission.settle()
            assert not submission.result.exists(), "C-s submitted the overall comment"
            assert "● Comment" in submission.screen(), (
                "new overall feedback did not select Comment"
            )
            if verdict != "comment":
                submission.keys(action)
                submission.settle()
            assert not submission.result.exists(), (
                "selecting a verdict submitted the review"
            )
            submission.keys("Enter")
            submission.completed(verdict == "approve")
            assert f"verdict: {verdict}" in submission.err.read_text()
            assert body in submission.out.read_text()
            saved = json.loads((submission.directory() / "review.json").read_text())
            comments = saved["review_comments"]
            assert len(comments) == 1 and comments[0]["body"] == body, saved
            assert comments[0].get("resolved", False) == (verdict == "approve"), saved
        report(
            "radio selections require Submit and send overall feedback with distinct verdicts"
        )

        overall = Round("overall-resumed", ["--session", submission.session])
        overall.keys("}")
        wait_until(
            lambda: "Overall comments" in overall.screen(),
            "overall thread navigation",
        )
        overall.keys("r")
        wait_until(
            lambda: all(c.get("resolved") for c in overall.comments()),
            "overall thread resolution",
        )
        overall.finish(True)
        report(
            "previous overall feedback is visible, reachable with }, and individually resolvable"
        )

        draft = Round("submit-draft", ["plan.md"])
        draft.keys("q", "BTab")
        draft.text("Saved draft.")
        draft.keys("Escape")
        draft.keys("q", "q")
        wait_until(lambda: "Abort review?" in draft.screen(), "abort confirmation")
        draft.keys("n")
        draft.settle()
        assert not draft.result.exists(), "declining abort ended the review"
        wait_until(
            lambda: "Saved draft" in draft.screen(), "draft after declining abort"
        )
        draft.keys("q", "y")
        draft.completed(None)
        assert draft.comments() == [], "draft became submitted feedback"
        resumed_draft = Round("draft-resumed", ["--session", draft.session])
        resumed_draft.keys("q")
        wait_until(
            lambda: "Saved draft" in resumed_draft.screen(), "restored overall draft"
        )
        resumed_draft.keys("BTab")
        resumed_draft.text(" Updated draft.")
        resumed_draft.wait_screen("Updated draft", "updated overall draft")
        cli("stop", "--session", draft.session)
        resumed_draft.completed(None)
        externally_resumed = Round("draft-after-stop", ["--session", draft.session])
        externally_resumed.keys("q")
        wait_until(
            lambda: "Updated draft" in externally_resumed.screen(),
            "draft after CLI stop",
        )
        externally_resumed.keys("c", "Enter")
        externally_resumed.completed(False)
        assert "Saved draft. Updated draft." in externally_resumed.out.read_text()
        report("overall comment draft survives Back, stop, and same-session resume")

        first = Round("round1", ["plan.md"])
        first.comment("Add rollback instructions.")
        comment = first.comments()[0]
        with concurrent.futures.ThreadPoolExecutor() as pool:
            futures = [
                pool.submit(
                    cli,
                    "comment",
                    "--session",
                    first.session,
                    "--reply-to",
                    comment["id"],
                    "--author",
                    author,
                    body,
                )
                for author, body in [
                    ("Scenario Agent A", "Preparing rollback."),
                    ("Scenario Agent B", "Checking deployment."),
                ]
            ]
            for future in futures:
                future.result()
        assert len(first.comments()[0]["replies"]) == 2, first.comments()
        first.comment(
            "Keep the deployment explanation.",
            file=False,
            expected="Add rollback instructions.Keep the deployment explanation.",
        )
        comments = first.comments()
        assert len(comments) == 1 and len(comments[0]["replies"]) == 2, comments
        first.finish(False)
        report(
            "document review, concurrent CLI replies, and TUI save preserve all feedback"
        )

        (WORK / "plan.md").write_text(
            "# Release plan\n\nDeploy the application.\nConfirm deployment.\n\nRollback to the previous release if deployment fails.\n"
        )
        second = Round("round2", ["--session", first.session])
        assert second.session == first.session
        assert "Rollback" in second.screen(), second.screen()
        second.finish(True)
        assert all(c.get("resolved") for c in second.comments()), second.comments()
        assert json.loads(second.summary.read_text())["round"] == 2
        report(
            "same-session source refresh and approval preserve threads and resolve them"
        )

        stopped = Round("stop", ["plan.md"])
        stopped.comment("Preserve this unfinished review.")
        cli("stop", "--session", stopped.session)
        stopped.completed(None)
        assert (
            stopped.result.exists() and "approved: true" not in stopped.err.read_text()
        )
        resumed = Round("resume", ["--session", stopped.session])
        assert resumed.comments()[0]["body"] == "Preserve this unfinished review."
        resumed.keys("]", "R", "y")
        resumed.completed(True)
        report("stop returns without approval; restart retains the saved comment")

        (WORK / "approval.md").write_text(
            "# Local scenario only\n- [ ] Confirm <!-- form: id=confirm required -->\n- [ ] Small <!-- form: id=size value=small -->\n- [ ] Large <!-- form: id=size value=large -->\n- [x] Publish <!-- form: id=publish consent -->\n"
        )
        (WORK / "mouse-approval.md").write_text(
            "# Mouse submission\n- [ ] Viewed <!-- form: id=viewed required -->\n"
        )
        mouse = Round("mouse-submit", ["--null", "--attachment", "mouse-approval.md"])
        floating = (
            tmux("display-message", "-p", "-t", mouse.pane, "#{pane_floating_flag}")
            == "1"
        )
        tmux("resize-pane", "-t", mouse.pane, "-x", "56" if floating else "54")
        assert tmux("display-message", "-p", "-t", mouse.pane, "#{pane_width}") == "54"
        mouse.comment("Existing feedback.")
        mouse.keys("r")
        wait_until(
            lambda: all(c.get("resolved") for c in mouse.comments()),
            "mouse thread resolution",
        )
        mouse.keys("q")
        mouse.click(r"Overall comment \(optional\)")
        mouse.text("Both videos look good!")
        mouse.click(r"[○●] Approve")
        mouse.wait_screen("● Approve", "mouse selects Approve")
        mouse.click(r"Submit RET")
        mouse.wait_screen("Required items must be completed", "required items notice")
        assert not mouse.result.exists(), "missing confirmation allowed approval"
        # The notice opened from Submit's cell; clicking that cell again within
        # the TUI's double-click guard (modalClickGuard) is ignored.
        time.sleep(0.5)
        mouse.click(r"Review r")
        screen = mouse.wait_screen("L2/", "Review focuses the first unchecked item")
        assert "Required items must be completed" not in screen, screen
        rows = screen.splitlines()
        viewed_row = next(
            i for i, row in enumerate(rows) if re.search(r"\bViewed\b", row)
        )
        required_row = next(
            i for i, row in enumerate(rows) if re.search(r"\bRequired\b", row)
        )
        assert viewed_row != required_row, screen
        mouse.click(r"\bViewed\b")
        mouse.keys("q")
        mouse.click(r"[○●] Approve")
        mouse.click(r"Submit RET")
        mouse.completed(True)
        assert "Both videos look good!" in mouse.out.read_text()
        report(
            "54-column mouse Approve/Submit: missing confirmation blocks visibly; wrapped confirmation allows retry"
        )

        form = Round(
            "forms",
            [
                "--null",
                "--attachment",
                "approval.md",
                "--result",
                str(ROOT / "form-result.json"),
            ],
        )
        form.comment("Pending form review thread.")
        form.click(r"0/1 💬")
        form.keys("r")
        wait_until(
            lambda: all(c.get("resolved") for c in form.comments()),
            "form thread resolution",
        )
        form.click(r"1/1 💬")
        form.keys("g")
        form.keys("j", "Space", "j", "j", "Space")
        form.keys("q")
        form.click(r"Overall comment \(optional\)")
        form.text("Keep this form review draft.")
        form.wait_screen("Keep this form review draft.", "overall draft input")
        form.click(r"1/1 💬", last=True)
        form.settle()
        assert not form.result.exists(), "counter click submitted the review"
        form.keys("q", "BTab", "C-s", "Tab", "Tab", "Tab", "Enter")
        form.wait_screen("L5/", "Submit counter focuses unchecked consent")
        form.keys("q")
        form.wait_screen("Optional items remain unanswered", "optional items notice")
        form.keys("Escape")
        form.finish(True)
        result = json.loads((ROOT / "form-result.json").read_text())
        assert result["approved"] and result["session"] == form.session, result
        document = result["attachments"][0]
        assert (
            document["answers"]["confirm"]["human"]
            and document["answers"]["size"]["value"] == "large"
        ), result
        assert not any(
            values.get("publish") for values in result["consents"].values()
        ), result
        assert "Keep this form review draft." in form.out.read_text()
        report(
            "header and Submit counters navigate; drafts and human answers export; unchecked consent grants nothing"
        )

        (WORK / "path dirs").mkdir()
        (WORK / "path dirs" / "report.md").write_text("report\n")
        (WORK / "paths.md").write_text(
            '- [ ] Output <!-- form: id=output type=file default="new.md" required -->\n'
            '- [ ] Notes <!-- form: id=where value=notes type=directory default="path dirs/" -->\n'
            '- [ ] Project <!-- form: id=where value=project type=directory default="./" -->\n'
            '- [ ] Copy <!-- form: id=copy checkbox type=file default="path dirs/copy.md" -->\n'
            '- [ ] Source <!-- form: id=source type=file existing default="path dirs/report.md" -->\n'
        )
        paths = Round(
            "path-inputs",
            [
                "--null",
                "--attachment",
                "paths.md",
                "--result",
                str(ROOT / "paths.json"),
            ],
        )
        paths.keys("Space")
        paths.wait_screen("File", "path editor")
        paths.wait_screen("File does not exist", "missing leaf notice")
        paths.keys("C-u")
        paths.text("missing/new.md")
        paths.wait_screen("Parent directory does not exist", "missing parent notice")
        paths.keys("C-s", "Space", "C-u")
        paths.text("path dirs/re")
        paths.wait_screen("report.md", "path completion without @")
        paths.keys("Tab")
        paths.wait_screen("path dirs/report.md", "completed path with spaces")
        paths.keys("C-s")
        paths.keys("j", "Space")
        paths.wait_screen("Directory", "radio directory input")
        paths.keys("C-u")
        paths.text("custom notes/")
        paths.keys("C-s", "j", "Space", "C-s", "k", "Space")
        paths.wait_screen("custom notes/", "per-choice path draft")
        paths.keys("Escape")
        paths.keys("j", "j", "Space")
        paths.wait_screen("File", "checkbox file input")
        paths.keys("C-s", "Space", "j", "Space", "C-u")
        paths.text("missing.md")
        paths.wait_screen("File does not exist", "existing input missing leaf")
        paths.keys("C-s")
        paths.wait_screen("File does not exist", "existing input rejects missing leaf")
        paths.keys("C-u")
        paths.text("missing/source.md")
        paths.keys("C-s")
        paths.wait_screen(
            "Parent directory does not exist", "existing input rejects missing parent"
        )
        paths.keys("C-u")
        paths.text("path dirs/report.md")
        paths.keys("C-s")
        paths.finish(True)
        result = json.loads((ROOT / "paths.json").read_text())
        answers = result["attachments"][0]["answers"]
        assert result["version"] == 3 and result["approved"], result
        assert answers["output"]["value"] == "path dirs/report.md", answers
        assert answers["output"]["human"], answers
        assert answers["source"]["value"] == "path dirs/report.md", answers
        assert answers["source"]["human"], answers
        assert answers["where"]["value"] == "notes", answers
        assert answers["where"]["text"] == "custom notes/", answers
        assert not answers["copy"]["value"] and "text" not in answers["copy"], answers
        assert all("drafts" not in answer for answer in answers.values()), answers
        assert "Interactive form result (version 3)" in paths.out.read_text()
        report(
            "path inputs complete raw names, accept new paths with notices, enforce existing, and export only selected drafts"
        )

        failure = Round(
            "save-failure",
            [
                "--null",
                "--attachment",
                "approval.md",
                "--result",
                str(ROOT / "recovered-result.json"),
            ],
        )
        failure.keys("j", "Space", "j", "j", "j", "Space")
        directory = failure.directory()
        blocker = directory / "result-1.json"
        blocker.mkdir()
        failure.keys("q", "y")
        failure.settle()
        assert not failure.result.exists(), "save failure exited the TUI"
        saved = json.loads((directory / "forms.json").read_text())
        assert saved["attachments"][0]["answers"]["confirm"]["value"], saved
        assert not (ROOT / "recovered-result.json").exists()
        blocker.rmdir()
        failure.keys("y")
        failure.completed(True)
        result = json.loads((ROOT / "recovered-result.json").read_text())
        assert any(values.get("publish") for values in result["consents"].values())
        report(
            "failed approval save keeps TUI and answers; retry exports explicit human consent"
        )

        source = ROOT / "unmanaged"
        source.mkdir()
        (source / "whole.md").write_text("one\ntwo\n")
        (source / ".gitattributes").write_text("* text ident\n")
        prepared = json.loads(
            cli(
                "tmp",
                "prepare",
                "--root",
                str(source),
                "--",
                "whole.md",
                "one.md",
                "two.md",
                ".gitattributes",
                "raw.txt",
                "raw.bin",
            ).stdout
        )
        tree = Path(prepared["tree"])
        (tree / "whole.md").unlink()
        (tree / "one.md").write_text("one\n")
        (tree / "two.md").write_text("two\n")
        (tree / "raw.txt").write_bytes(b"line\r\n$Id: keep $\r\n")
        (tree / "raw.bin").write_bytes(b"\x00\xff\r\n")
        frozen = json.loads(cli("tmp", "snapshot", prepared["id"]).stdout)
        attachment = Path(frozen["attachment"])
        consent_line = next(
            number
            for number, line in enumerate(attachment.read_text().splitlines(), 1)
            if line.startswith("- [ ] ")
            and line.endswith("<!-- form: id=apply consent -->")
        )
        temporary = Round(
            "temporary-tree",
            [
                "--diff",
                frozen["diff"],
                "--attachment",
                str(attachment),
                "--result",
                frozen["result"],
                "--focus",
                f"{attachment}:{consent_line}",
            ],
        )
        temporary.keys("Space")
        temporary.comment("Keep this proposal; confirm in the next round.")
        temporary.finish(False)
        temporary = Round(
            "temporary-tree-approved",
            ["--session", temporary.session, "--result", frozen["result"]],
        )
        temporary.finish(True)
        wrong_root = cli(
            "tmp",
            "apply",
            prepared["id"],
            frozen["snapshot"],
            "--root",
            str(WORK),
            "--result",
            frozen["result"],
            check=False,
        )
        assert wrong_root.returncode != 0 and "root differs" in wrong_root.stderr
        assert (source / "whole.md").exists()
        applied = json.loads(
            cli(
                "tmp",
                "apply",
                prepared["id"],
                frozen["snapshot"],
                "--root",
                str(source),
                "--result",
                frozen["result"],
            ).stdout
        )
        assert applied["complete"], applied
        assert not (source / "whole.md").exists()
        assert (source / "one.md").read_text() == "one\n"
        assert (source / "two.md").read_text() == "two\n"
        assert (source / "raw.txt").read_bytes() == b"line\r\n$Id: keep $\r\n"
        assert (source / "raw.bin").read_bytes() == b"\x00\xff\r\n"
        report(
            "temporary tree: split a file through a frozen diff, human consent, and bounded application"
        )

        export = ROOT / "failed-export.json"
        failed_export = Round(
            "export-failure",
            ["--null", "--attachment", "approval.md", "--result", str(export)],
        )
        failed_export.keys("j", "Space")
        directory = failed_export.directory()
        export.mkdir()
        failed_export.keys("q", "y")
        code = failed_export.completed(None)
        assert (
            code["code"] != 0 and "approved: true" not in failed_export.err.read_text()
        )
        saved_result = json.loads((directory / "result-1.json").read_text())
        assert (
            saved_result["approved"]
            and saved_result["session"] == failed_export.session
        )
        assert saved_result["attachments"][0]["answers"]["confirm"]["human"]
        report(
            "failed result export returns nonzero without printing approval and retains the completed result"
        )
        summary_failure = Round("summary-failure", ["plan.md"])
        directory = summary_failure.directory()
        summary_failure.summary.mkdir()
        summary_failure.keys("q", "Enter")
        code = summary_failure.completed(None)
        assert (
            code["code"] != 0
            and "approved: true" not in summary_failure.err.read_text()
        )
        assert directory.exists(), "summary failure cleaned up the review"
        retry = Round("summary-retry", ["--session", summary_failure.session])
        retry.keys("q", "Enter")
        retry.completed(True)
        assert json.loads(retry.summary.read_text())["round"] == 2
        report(
            "failed summary export grants no approval; same-session retry saves a fresh result"
        )

        launcher = WORK / "custom launcher"
        launcher.write_text(
            f"#!{sys.executable}\n"
            + """import json
import os
from pathlib import Path
import subprocess
import sys

operation = sys.argv[1]
request = json.load(sys.stdin)
if operation == "check":
    assert os.environ["TMUX_PANE"].startswith("%")
    Path("launcher-check.json").write_text(json.dumps(request))
elif operation == "open":
    assert request == json.loads(Path("launcher-check.json").read_text())
    command = 'tmux set-option -p -t "$TMUX_PANE" remain-on-exit on && exec ' + request["command"]
    pane = subprocess.check_output([
        "tmux", "split-window", "-h", "-d", "-P", "-F", "#{pane_id}",
        "-t", os.environ["TMUX_PANE"], "-c", request["cwd"], command,
    ], text=True).strip()
    print(json.dumps({"pane": pane}))
elif operation == "close":
    Path("launcher-close.json").write_text(json.dumps(request))
    subprocess.run(["tmux", "kill-pane", "-t", request["pane"]], check=True)
"""
        )
        launcher.chmod(0o755)
        custom = Round(
            "custom-launcher",
            [
                "plan.md",
                "--launcher",
                "custom:/missing/launcher",
                "--launcher",
                f"custom:{launcher}",
            ],
        )
        custom.finish(True)
        assert "unavailable" not in custom.err.read_text()
        closed = json.loads((WORK / "launcher-close.json").read_text())
        assert closed["pane"] == custom.pane
        assert (
            custom.pane not in tmux("list-panes", "-a", "-F", "#{pane_id}").splitlines()
        )
        project_config = WORK / ".critty.toml"
        project_config.write_text(
            "launchers = "
            + json.dumps([f"custom:{launcher}"], ensure_ascii=False)
            + "\n"
        )
        try:
            (WORK / "launcher-check.json").unlink()
            cli_first = Round("launcher-cli-priority", ["plan.md", "--launcher", "mux"])
            cli_first.finish(True)
            assert not (WORK / "launcher-check.json").exists()
            for name, args in [
                ("launcher-config-only", ["plan.md"]),
                (
                    "launcher-config-fallback",
                    ["plan.md", "--launcher", "custom:/missing/launcher"],
                ),
            ]:
                configured = Round(name, args)
                configured.finish(True)
                assert (
                    json.loads((WORK / "launcher-close.json").read_text())["pane"]
                    == configured.pane
                )
                assert "unavailable" not in configured.err.read_text()
        finally:
            project_config.unlink()
        report("configured launchers follow CLI choices and precede implicit mux")
        fallback = Round(
            "launcher-fallback", ["plan.md", "--launcher", "custom:/missing/launcher"]
        )
        fallback.finish(True)
        assert "unavailable" not in fallback.err.read_text()
        report("custom launcher check/open/close round trip and unavailable fallback")
    finally:
        tmux("kill-server")


if __name__ == "__main__":
    if len(sys.argv) > 1 and sys.argv[1] == "--child":
        child(sys.argv[2])
    else:
        parser = argparse.ArgumentParser(description=__doc__)
        parser.add_argument(
            "--binary",
            type=Path,
            help="test an existing Critty binary instead of building",
        )
        parser.add_argument(
            "--tmux",
            default=shutil.which("tmux"),
            help="tmux executable (default: PATH)",
        )
        parser.add_argument(
            "--keep", action="store_true", help="retain fixtures and logs after the run"
        )
        args = parser.parse_args()
        if args.tmux is None:
            parser.error("tmux is required")
        # Keep Unix socket paths short, including on macOS where TMPDIR is long.
        root = Path(tempfile.mkdtemp(prefix="critty-e2e-", dir="/tmp"))
        try:
            binary = args.binary.resolve() if args.binary else root / "critty"
            if not args.binary:
                repo = Path(__file__).resolve().parent.parent
                subprocess.run(
                    ["go", "build", "-o", str(binary), "./cmd/critty"],
                    cwd=repo,
                    check=True,
                )
            configure(root, binary, Path(args.tmux).resolve())
            scenarios()
            sys.dont_write_bytecode = True
            from tmux_launchers import verify

            verify(binary, Path(args.tmux).resolve())
        finally:
            if args.keep:
                print(f"Fixtures and logs: {root}")
            else:
                shutil.rmtree(root)
