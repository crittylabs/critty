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
        if not k.startswith(("HERDR_", "TMUX", "CRITTY_", "TCRIT_"))
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
        time.sleep(0.15)

    def text(self, text: str) -> None:
        tmux("send-keys", "-l", "-t", self.pane, text)
        time.sleep(0.1)

    def click(self, pattern: str, last: bool = False) -> None:
        rows = list(enumerate(self.screen().splitlines(), 1))
        for y, row in reversed(rows) if last else rows:
            match = re.search(pattern, row)
            if match:
                # Button rows contain spaces and single-cell frame/selector glyphs.
                x = match.start() + 1
                self.text(f"\x1b[<35;{x};{y}M")
                self.text(f"\x1b[<0;{x};{y}M\x1b[<0;{x};{y}m")
                return
        raise AssertionError(f"mouse target not visible: {pattern}\n{self.screen()}")

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


def scenarios() -> None:
    for path in [WORK, STATE, CONFIG / "critty", ROOT / "bin"]:
        path.mkdir(parents=True, exist_ok=True)
    if not (ROOT / "bin" / "tmux").exists():
        (ROOT / "bin" / "tmux").symlink_to(TMUX)
    (CONFIG / "critty" / "config.json").write_text(
        json.dumps({"author": "Scenario Reviewer", "cleanup_on_approve": False})
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
        default_approval = Round("default-approval", ["plan.md"])
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

        unchanged = Round("unchanged-feedback", ["plan.md"])
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
        assert not unchanged.result.exists(), (
            "declining resolve-all submitted the round"
        )
        assert not any(c.get("resolved") for c in unchanged.comments()), (
            "declining resolved feedback"
        )
        assert "Preserve draft" in unchanged.screen(), "declining discarded the draft"
        unchanged.keys("Enter", "y")
        unchanged.completed(True)
        assert all(c.get("resolved") for c in unchanged.comments())
        report(
            "existing feedback defaults to Approve; declining resolve-all keeps the draft; confirming approves"
        )

        for verdict, action in [("approve", "a"), ("reject", "r"), ("comment", "c")]:
            submission = Round(f"submit-{verdict}", ["plan.md"])
            submission.keys("q", "BTab")
            body = f"Overall comment: {verdict}."
            submission.text(body)
            assert "● Comment" in submission.screen(), (
                "first input did not select Comment"
            )
            submission.keys("C-s")
            assert not submission.result.exists(), "C-s submitted the overall comment"
            assert "● Comment" in submission.screen(), (
                "new overall feedback did not select Comment"
            )
            if verdict != "comment":
                submission.keys(action)
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
        assert all(c.get("resolved") for c in mouse.comments())
        mouse.keys("q")
        mouse.click(r"Overall comment \(optional\)")
        mouse.text("Both videos look good!")
        mouse.click(r"[○●] Approve")
        assert "● Approve" in mouse.screen(), "mouse did not select Approve"
        mouse.click(r"Submit RET")
        assert not mouse.result.exists(), "missing confirmation allowed approval"
        assert "Required items must be completed" in mouse.screen(), mouse.screen()
        assert "Proceed anyway" not in mouse.screen(), mouse.screen()
        mouse.keys("f")
        assert not mouse.result.exists(), "Proceed bypassed required confirmation"
        mouse.click(r"Review r")
        screen = wait_until(
            lambda: mouse.screen() if "Required" in mouse.screen() else None,
            "wrapped required confirmation",
        )
        rows = screen.splitlines()
        assert "L2/" in screen, "Review did not focus the first unchecked item"
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
        assert all(c.get("resolved") for c in form.comments())
        form.click(r"1/1 💬")
        form.keys("g")
        form.keys("j", "Space", "j", "j", "Space")
        form.keys("q")
        form.click(r"Overall comment \(optional\)")
        form.text("Keep this form review draft.")
        form.click(r"1/1 💬", last=True)
        assert not form.result.exists(), "counter click submitted the review"
        form.keys("q", "Tab", "Tab", "Tab", "Tab", "Tab", "Enter")
        assert "L5/" in form.screen(), "Submit counter did not focus unchecked consent"
        form.keys("q")
        assert "Optional items remain unanswered" in form.screen(), form.screen()
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
        time.sleep(0.3)
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
        project_config = WORK / ".critty.config.json"
        project_config.write_text(json.dumps({"launchers": [f"custom:{launcher}"]}))
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
