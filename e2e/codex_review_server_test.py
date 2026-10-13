"""Check Codex review tools and MCP stdio locally, without starting an AI."""

import asyncio
import importlib.util
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

sys.dont_write_bytecode = True
SCRIPTS = (
    Path(__file__).resolve().parents[1] / "internal/cli/skill/crit-ai-review/scripts"
)
sys.path.insert(0, str(SCRIPTS))
from codex_review_server import ReviewTools


class CodexReviewTest(unittest.TestCase):
    def setUp(self) -> None:
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.root = Path(temp.name).resolve()
        self.workspace = self.root / "workspace"
        self.workspace.mkdir()
        self.source = self.workspace / "source.py"
        self.source.write_text("alpha\n日本語 beta\n")
        self.policy = {
            "cwd": str(self.workspace),
            "binary": "/trusted/critty",
            "session": "0123456789ab",
            "author": "Codex (AI review)",
            "locations": ["source.py"],
            "read_roots": [str(self.workspace)],
            "read_commands": [["/usr/bin/git", "--no-pager", "log", "-n", "10"]],
        }
        self.tools = ReviewTools(self.policy)

    def test_research_and_pagination(self) -> None:
        self.assertEqual(
            self.tools.read_file("source.py")["text"], "1: alpha\n2: 日本語 beta"
        )
        self.assertEqual(self.tools.search("日本語")["matches"][0]["line"], 2)
        self.assertEqual(self.tools.list_files()["files"], [str(self.source)])
        self.source.write_text("line\n" * 102)
        page = self.tools.read_file("source.py")
        self.assertEqual(page["next_line"], 101)
        self.assertEqual(self.tools.read_file("source.py", 101)["next_line"], None)
        page = self.tools.search("line")
        self.assertEqual(page["next_offset"], 100)
        self.assertEqual(len(self.tools.search("line", offset=100)["matches"]), 2)

    def test_symlink_and_path_escape(self) -> None:
        outside = self.root / "outside"
        outside.mkdir()
        (outside / "secret").write_text("secret")
        (self.workspace / "link").symlink_to(outside, target_is_directory=True)
        (self.workspace / "filelink").symlink_to(outside / "secret")
        for path in [
            "../outside/secret",
            str(outside / "secret"),
            "link/secret",
            "filelink",
        ]:
            with self.subTest(path=path), self.assertRaises(ValueError):
                self.tools.read_file(path)
        self.assertEqual(self.tools.list_files()["files"], [str(self.source)])
        self.assertEqual(self.tools.search("secret")["matches"], [])

    def test_search_reports_omissions(self) -> None:
        (self.workspace / "binary").write_bytes(b"\0test")
        (self.workspace / "large").write_bytes(b"x" * (2 * 1024 * 1024 + 1))
        self.assertEqual(self.tools.search("alpha")["skipped_files"], 2)
        with self.assertRaises(ValueError):
            self.tools.read_file("large")

    def test_commands_and_findings_do_not_use_a_shell(self) -> None:
        with patch("codex_review_server.subprocess.run") as run:
            run.return_value = subprocess.CompletedProcess([], 0, "ok", "")
            self.tools.read_command(0)
            self.assertEqual(run.call_args.args[0], self.policy["read_commands"][0])
            self.assertEqual(run.call_args.kwargs["cwd"], self.workspace)
            self.assertEqual(run.call_args.kwargs["stdin"], subprocess.DEVNULL)
            self.assertNotIn("shell", run.call_args.kwargs)
            body = "Finding: $(touch bad); `id`\nquoted 'text'"
            self.tools.post_finding(body, "source.py:1-2")
            self.assertEqual(
                run.call_args.args[0],
                [
                    "/trusted/critty",
                    "comment",
                    "--session",
                    "0123456789ab",
                    "--author",
                    "Codex (AI review)",
                    "--",
                    "source.py:1-2",
                    body,
                ],
            )
            for location in ["elsewhere", "source.py:0", "source.py:9-2"]:
                with self.subTest(location=location), self.assertRaises(ValueError):
                    self.tools.post_finding("finding", location)
            for index in [-1, 1]:
                with self.subTest(index=index), self.assertRaises(ValueError):
                    self.tools.read_command(index)
            self.assertEqual(run.call_count, 2)

    def test_comments_filter_avoids_large_readback(self) -> None:
        comments = [
            {"id": "c_old", "body": "REQUEST old"},
            {"id": "c_new", "body": "REQUEST current"},
        ]
        with patch.object(self.tools, "run", return_value=json.dumps(comments)):
            self.assertEqual(
                self.tools.comments("REQUEST current")["comments"], [comments[1]]
            )

    def test_describe_without_sdk_or_model(self) -> None:
        policy = self.root / "policy.json"
        policy.write_text(json.dumps(self.policy))
        command = [
            sys.executable,
            "-B",
            str(SCRIPTS / "codex_review_server.py"),
            "--describe",
            str(policy),
        ]
        result = subprocess.run(command, capture_output=True, text=True, check=True)
        self.assertEqual(
            json.loads(result.stdout)["comment_tools"], ["comments", "post_finding"]
        )
        policy.write_text("{}")
        result = subprocess.run(command, capture_output=True, text=True, check=False)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(result.stdout, "")

    @unittest.skipUnless(
        importlib.util.find_spec("mcp"), "install pinned MCP SDK for stdio test"
    )
    def test_stdio_protocol_and_denial_recovery(self) -> None:
        from mcp import Client, StdioServerParameters

        policy = self.root / "policy.json"
        policy.write_text(json.dumps(self.policy))

        async def exercise() -> None:
            params = StdioServerParameters(
                command=sys.executable,
                args=["-B", str(SCRIPTS / "codex_review_server.py"), str(policy)],
            )
            async with Client(params) as client:
                listing = await client.list_tools()
                self.assertEqual(
                    {t.name for t in listing.tools},
                    {
                        "capabilities",
                        "read_file",
                        "list_files",
                        "search",
                        "read_command",
                        "comments",
                        "post_finding",
                    },
                )
                denied = await client.call_tool("read_file", {"path": "../policy.json"})
                self.assertTrue(denied.is_error)
                self.assertIn("outside permitted read roots", str(denied))
                self.assertIn("Continue with permitted alternatives", str(denied))
                allowed = await client.call_tool("read_file", {"path": "source.py"})
                self.assertFalse(allowed.is_error)
                self.assertIn("alpha", str(allowed))

        asyncio.run(exercise())


if __name__ == "__main__":
    unittest.main()
