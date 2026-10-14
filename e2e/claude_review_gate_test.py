"""Test restricted AI reviewer permissions without third-party dependencies."""

import importlib.util
import json
import shlex
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

SCRIPT = (
    Path(__file__).resolve().parents[1]
    / "internal/cli/skill/critty-ai-review/scripts/claude_review_gate.py"
)
spec = importlib.util.spec_from_file_location("claude_review_gate", SCRIPT)
gate = importlib.util.module_from_spec(spec)
spec.loader.exec_module(gate)
POLICY = {
    "cwd": "/project",
    "binary": "/trusted/critty",
    "session": "0123456789ab",
    "author": "Claude Code Fable (reviewer)",
    "locations": ["src/example.py"],
    "read_roots": ["/project"],
    "read_commands": [["/usr/bin/git", "--no-pager", "log", "--oneline", "-n", "20"]],
}
PREFIX = [
    POLICY["binary"],
    "comment",
    "--session",
    POLICY["session"],
    "--author",
    POLICY["author"],
    "--",
]


def event(command: str) -> dict:
    return {
        "hook_event_name": "PreToolUse",
        "cwd": "/project",
        "tool_name": "Bash",
        "tool_input": {"command": command},
    }


class ClaudeReviewGateTest(unittest.TestCase):
    def test_reply_only_keeps_the_same_thread_boundary(self) -> None:
        target = {"id": "c_123abc", "path": "src/example.py"}
        policy = {**POLICY, "resolvable_comments": [target]}
        argv = gate.resolution_argv(policy, target, False)
        self.assertNotIn("--resolve", argv)
        self.assertTrue(
            gate.permitted(
                event(
                    shlex.join(
                        argv + ["My concern is satisfied; human follow-up remains."]
                    )
                ),
                policy,
            )
        )
        for other in [{**target, "id": "c_ffffff"}, {**target, "path": "other.py"}]:
            self.assertFalse(
                gate.permitted(
                    event(
                        shlex.join(
                            gate.resolution_argv(policy, other, False) + ["Satisfied"]
                        )
                    ),
                    policy,
                )
            )
        self.assertFalse(gate.permitted(event(shlex.join(argv + [" "])), policy))

    def test_only_listed_own_findings_can_be_resolved(self) -> None:
        targets = [
            {"id": "c_123abc", "path": "src/example.py"},
            {"id": "r_123abc", "path": ""},
        ]
        policy = {**POLICY, "resolvable_comments": targets}
        for target in targets:
            argv = gate.resolution_argv(policy, target)
            self.assertTrue(
                gate.permitted(event(shlex.join(argv + ["Verified fix"])), policy)
            )
            self.assertFalse(
                gate.permitted(event(shlex.join(argv + ["Verified fix"])), POLICY)
            )
            self.assertFalse(gate.permitted(event(shlex.join(argv + [" "])), policy))
            for other in [
                {**target, "id": "c_ffffff"},
                {**target, "path": "other.py"},
            ]:
                self.assertFalse(
                    gate.permitted(
                        event(
                            shlex.join(gate.resolution_argv(policy, other) + ["Reason"])
                        ),
                        policy,
                    )
                )
            self.assertFalse(
                gate.permitted(
                    event(shlex.join(argv + ["Reason"]) + "; touch x"), policy
                )
            )
        for targets in [
            None,
            {},
            ["c_123abc"],
            [{"id": "c_123abc", "path": ""}],
            [{"id": "r_123abc", "path": "src/example.py"}],
        ]:
            self.assertFalse(
                gate.valid_policy({**POLICY, "resolvable_comments": targets})
            )

    def test_literal_comments(self) -> None:
        for body in ["Finding", "Don't run $(touch x); `id`\nnext line"]:
            with self.subTest(body=body):
                self.assertTrue(
                    gate.permitted(event(shlex.join(PREFIX + [body])), POLICY)
                )
                self.assertTrue(
                    gate.permitted(
                        event(shlex.join(PREFIX + ["src/example.py:4-6", body])), POLICY
                    )
                )

    def test_commands_outside_scope_are_denied(self) -> None:
        for command in [
            "python3 -m pytest",
            "touch x",
            shlex.join(PREFIX + ["finding"]) + "; touch x",
            shlex.join(PREFIX + ["finding"]) + " > x",
            shlex.join(PREFIX + ["finding"]) + " | sh",
            shlex.join(PREFIX + ["finding"]) + "\ntouch x",
            shlex.join(PREFIX) + ' "$(touch x)"',
            shlex.join(PREFIX) + " `touch x`",
            shlex.join(PREFIX).replace("0123456789ab", "ffffffffffff") + " finding",
            shlex.join(PREFIX[:-1] + ["--clear"]),
            shlex.join(PREFIX[:-1] + ["--resolve", "--", "finding"]),
            shlex.join(PREFIX + ["other.py", "finding"]),
            "env X=1 " + shlex.join(PREFIX + ["finding"]),
        ]:
            with self.subTest(command=command):
                self.assertTrue(not gate.permitted(event(command), POLICY))

    def test_exact_read_and_non_shell_tools(self) -> None:
        command = shlex.join(
            [
                POLICY["binary"],
                "comments",
                "--session",
                POLICY["session"],
                "--all",
                "--json",
            ]
        )
        request = event(command)
        self.assertTrue(gate.permitted(request, POLICY))
        request["tool_input"]["run_in_background"] = True
        self.assertTrue(not gate.permitted(request, POLICY))
        for tool in ["Edit", "Write", "Agent", "NotebookEdit", "mcp__server__write"]:
            request["tool_name"] = tool
            self.assertTrue(not gate.permitted(request, POLICY))
        request["tool_name"] = "Read"
        request["tool_input"] = {"file_path": "/project/src/example.py"}
        self.assertTrue(gate.permitted(request, POLICY))
        request["cwd"] = "/other"
        self.assertTrue(not gate.permitted(request, POLICY))

    def test_workspace_research_is_allowed(self) -> None:
        for tool, field in [("Read", "file_path"), ("Grep", "path")]:
            with self.subTest(tool=tool, field=field):
                request = event("")
                request["tool_name"] = tool
                for path in [
                    "src/example.py",
                    "/project/src/example.py",
                    "/project/AGENTS.md",
                    "/project",
                ]:
                    request["tool_input"] = {field: path}
                    self.assertTrue(gate.permitted(request, POLICY))
                for path in ["", "../secret", "/elsewhere", "/project-other/file"]:
                    request["tool_input"] = {field: path}
                    self.assertTrue(not gate.permitted(request, POLICY))
                request["tool_input"] = {}
                self.assertTrue(gate.permitted(request, POLICY) == (tool == "Grep"))

    def test_symlink_escape_is_denied(self) -> None:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        tmp_path = Path(tmp.name)
        source = tmp_path / "source"
        source.symlink_to(tmp_path.parent / "outside")
        policy = {**POLICY, "cwd": str(tmp_path), "read_roots": [str(tmp_path)]}
        request = {
            "hook_event_name": "PreToolUse",
            "cwd": str(tmp_path),
            "tool_name": "Read",
            "tool_input": {"file_path": str(source)},
        }
        self.assertTrue(not gate.permitted(request, policy))

    def test_search_and_history(self) -> None:
        request = event("")
        request["tool_name"] = "Glob"
        request["tool_input"] = {"pattern": "**/*.py"}
        self.assertTrue(gate.permitted(request, POLICY))
        for pattern in ["../**/*", "/elsewhere/*"]:
            request["tool_input"] = {"pattern": pattern}
            self.assertTrue(not gate.permitted(request, POLICY))
        history = shlex.join(POLICY["read_commands"][0])
        self.assertTrue(gate.permitted(event(history), POLICY))
        self.assertTrue(
            not gate.permitted(event(history + " --output=history.txt"), POLICY)
        )
        self.assertTrue(not gate.permitted(event("/usr/bin/git reset --hard"), POLICY))

    def test_malformed_policy_lists_are_denied(self) -> None:
        for key in ["locations", "read_roots", "read_commands"]:
            with self.subTest(key=key):
                request = event(shlex.join(PREFIX + ["finding"]))
                for value in ["/project", {}, None, [1]]:
                    self.assertTrue(not gate.permitted(request, {**POLICY, key: value}))

    def test_invalid_event_and_locations(self) -> None:
        request = event(shlex.join(PREFIX + ["finding"]))
        del request["cwd"]
        self.assertTrue(not gate.permitted(request, POLICY))
        request = event(shlex.join(PREFIX + ["finding"]))
        request["hook_event_name"] = "PostToolUse"
        self.assertTrue(not gate.permitted(request, POLICY))
        for location in [
            "src/example.py:0",
            "src/example.py:9-3",
            "src/example.py:-1",
            "src/example.py:3-",
            "other.py:2",
        ]:
            self.assertTrue(
                not gate.permitted(
                    event(shlex.join(PREFIX + [location, "finding"])), POLICY
                )
            )
        self.assertTrue(
            gate.permitted(
                event(shlex.join(PREFIX + ["src/example.py:42", "finding"])), POLICY
            )
        )
        self.assertTrue(
            not gate.permitted(
                event(shlex.join(PREFIX + ["src/example.py", " "])), POLICY
            )
        )

    def test_hook_failures_deny_with_valid_json(self) -> None:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        tmp_path = Path(tmp.name)
        for policy_text, input_text in [
            ("{}", "{}"),
            ("{", "{}"),
            (json.dumps(POLICY), "{"),
            (None, "{}"),
        ]:
            with self.subTest(policy_text=policy_text, input_text=input_text):
                path = tmp_path / "policy.json"
                path.unlink(missing_ok=True)
                if policy_text is not None:
                    path.write_text(policy_text)
                result = subprocess.run(
                    [sys.executable, str(SCRIPT), str(path)],
                    input=input_text,
                    text=True,
                    capture_output=True,
                    check=True,
                )
                self.assertTrue(
                    json.loads(result.stdout)["hookSpecificOutput"][
                        "permissionDecision"
                    ]
                    == "deny"
                )

    def test_hook_allow_and_capability_description(self) -> None:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        tmp_path = Path(tmp.name)
        path = tmp_path / "policy.json"
        path.write_text(json.dumps(POLICY))
        command = [sys.executable, str(SCRIPT), str(path)]
        result = subprocess.run(
            command + ["--describe"], text=True, capture_output=True, check=True
        )
        summary = json.loads(result.stdout)
        self.assertTrue(gate.permitted(event(summary["list_comments"]), POLICY))
        for read_command in summary["read_commands"]:
            self.assertTrue(gate.permitted(event(read_command), POLICY))
        result = subprocess.run(
            command,
            input=json.dumps(event(summary["list_comments"])),
            text=True,
            capture_output=True,
            check=True,
        )
        self.assertTrue(
            json.loads(result.stdout)["hookSpecificOutput"]["permissionDecision"]
            == "allow"
        )
        result = subprocess.run(
            command,
            input=json.dumps(event("touch x")),
            text=True,
            capture_output=True,
            check=True,
        )
        decision = json.loads(result.stdout)["hookSpecificOutput"]
        self.assertTrue(decision["permissionDecision"] == "deny")
        self.assertTrue(json.loads(decision["permissionDecisionReason"]) == summary)

    def test_describe_failure_stops_dispatch(self) -> None:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        tmp_path = Path(tmp.name)
        for policy_text in [None, "{", "{}"]:
            with self.subTest(policy_text=policy_text):
                path = tmp_path / "policy.json"
                path.unlink(missing_ok=True)
                if policy_text is not None:
                    path.write_text(policy_text)
                result = subprocess.run(
                    [sys.executable, str(SCRIPT), "--describe", str(path)],
                    text=True,
                    capture_output=True,
                    check=False,
                )
                self.assertTrue(result.returncode == 1)
                self.assertTrue(result.stdout == "")
                self.assertTrue(
                    result.stderr == "invalid or unreadable review policy\n"
                )

    def test_symlinked_read_root(self) -> None:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        tmp_path = Path(tmp.name)
        directory = tmp_path / "real"
        directory.mkdir()
        alias = tmp_path / "alias"
        alias.symlink_to(directory, target_is_directory=True)
        policy = {**POLICY, "cwd": str(alias), "read_roots": [str(alias)]}
        request = {
            "hook_event_name": "PreToolUse",
            "cwd": str(directory),
            "tool_name": "Read",
            "tool_input": {"file_path": str(directory / "source.py")},
        }
        self.assertTrue(gate.permitted(request, policy))

    def test_ambiguous_locations_and_relative_executable_are_denied(self) -> None:
        request = event(shlex.join(PREFIX + ["src/example.py:4-6:9", "finding"]))
        self.assertTrue(
            not gate.permitted(request, {**POLICY, "locations": ["src/example.py:4-6"]})
        )
        self.assertTrue(
            not gate.permitted(
                event(shlex.join(PREFIX + ["finding"])),
                {**POLICY, "read_commands": [["git", "log"]]},
            )
        )
        self.assertTrue(
            gate.permitted(event(shlex.join(PREFIX + ["src/example.py"])), POLICY)
        )


if __name__ == "__main__":
    unittest.main()
