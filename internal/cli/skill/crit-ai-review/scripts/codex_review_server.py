"""Expose bounded workspace research and Critty findings over MCP stdio."""

import argparse
import json
import os
import shlex
import subprocess
from collections.abc import Callable, Iterator
from functools import wraps
from pathlib import Path

from claude_review_gate import describe, permitted, valid_policy

MAX_FILE_BYTES = 2 * 1024 * 1024
MAX_OUTPUT = 32000
PAGE_SIZE = 100


class ReviewTools:
    def __init__(self, policy: dict) -> None:
        if not valid_policy(policy):
            raise ValueError("invalid review policy")
        self.policy = policy
        self.cwd = Path(policy["cwd"]).resolve(strict=True)
        self.roots = [Path(p).resolve(strict=True) for p in policy["read_roots"]]
        if not self.cwd.is_dir() or not all(p.is_dir() for p in self.roots):
            raise ValueError("review directories must exist")

    def capabilities(self) -> dict:
        """Describe the actual permitted tools and how to recover from denials."""
        result = describe(self.policy)
        del result["list_comments"], result["post_prefix"]
        result["comment_tools"] = ["comments", "post_finding"]
        result["session"] = self.policy["session"]
        result["author"] = self.policy["author"]
        result["read_tools"] = ["read_file", "list_files", "search", "read_command"]
        result["posting"] = (
            "Use post_finding(body, location) and comments; the server fixes the session and author."
        )
        result["read_commands"] = [
            {"index": i, "argv": args}
            for i, args in enumerate(self.policy["read_commands"])
        ]
        result["limits"] = {"file_bytes": MAX_FILE_BYTES, "page_size": PAGE_SIZE}
        return result

    def path(self, value: str) -> Path:
        path = Path(value)
        if not path.is_absolute():
            path = self.cwd / path
        path = path.resolve(strict=True)
        if not any(path.is_relative_to(root) for root in self.roots):
            raise ValueError(
                "outside permitted read roots; use capabilities for alternatives"
            )
        return path

    def text(self, path: Path) -> str:
        if not path.is_file():
            raise ValueError("not a regular file")
        with path.open("rb") as stream:
            data = stream.read(MAX_FILE_BYTES + 1)
        if len(data) > MAX_FILE_BYTES:
            raise ValueError(
                "file too large; ask the requester for an excerpt if essential"
            )
        if b"\0" in data:
            raise ValueError("not a text file")
        return data.decode("utf-8")

    def read_file(self, path: str, start_line: int = 1) -> dict:
        """Read up to 100 source lines; use next_line to continue."""
        if start_line < 1:
            raise ValueError("start_line must be positive")
        lines = self.text(self.path(path)).splitlines()
        selected = lines[start_line - 1 : start_line - 1 + PAGE_SIZE]
        end = start_line - 1 + len(selected)
        text = "\n".join(f"{i}: {line}" for i, line in enumerate(selected, start_line))
        return {
            "text": text[:MAX_OUTPUT],
            "next_line": end + 1 if end < len(lines) else None,
            "truncated": len(text) > MAX_OUTPUT,
        }

    def files(self, value: str) -> Iterator[Path]:
        root = self.path(value)
        if root.is_file():
            yield root
            return
        for directory, dirs, names in os.walk(root, followlinks=False):
            dirs[:] = sorted(
                d for d in dirs if d != ".git" and not Path(directory, d).is_symlink()
            )
            for name in sorted(names):
                try:
                    path = self.path(str(Path(directory, name)))
                except (OSError, ValueError):
                    continue
                if path.is_file():
                    yield path

    def list_files(self, path: str = ".", offset: int = 0) -> dict:
        """List a page of files under an allowed path; .git directories are omitted."""
        if offset < 0:
            raise ValueError("offset must be non-negative")
        page = []
        for i, file in enumerate(self.files(path)):
            if i < offset:
                continue
            if len(page) == PAGE_SIZE:
                return {"files": page, "next_offset": offset + PAGE_SIZE}
            page.append(str(file))
        return {"files": page, "next_offset": None}

    def search(self, query: str, path: str = ".", offset: int = 0) -> dict:
        """Find literal text in UTF-8 files, with source lines and pagination."""
        if not query or offset < 0:
            raise ValueError("query must be non-empty and offset non-negative")
        matches, skipped = [], 0
        seen = 0
        for file in self.files(path):
            try:
                lines = self.text(file).splitlines()
            except (OSError, ValueError):
                skipped += 1
                continue
            for number, line in enumerate(lines, 1):
                if query not in line:
                    continue
                seen += 1
                if seen <= offset:
                    continue
                if len(matches) == PAGE_SIZE:
                    return {
                        "matches": matches,
                        "next_offset": offset + PAGE_SIZE,
                        "skipped_files": skipped,
                    }
                matches.append({"path": str(file), "line": number, "text": line[:500]})
        return {"matches": matches, "next_offset": None, "skipped_files": skipped}

    def run(self, argv: list[str]) -> str:
        result = subprocess.run(
            argv,
            cwd=self.cwd,
            stdin=subprocess.DEVNULL,
            capture_output=True,
            text=True,
            timeout=30,
            check=False,
        )
        if result.returncode:
            raise ValueError(
                f"command failed ({result.returncode}): {result.stderr[:2000]}"
            )
        return result.stdout

    def read_command(self, index: int) -> dict:
        """Run one exact requester-approved read command by its capabilities index."""
        commands = self.policy["read_commands"]
        if index < 0 or index >= len(commands):
            raise ValueError(
                "unknown read command; use capabilities for available indices"
            )
        output = self.run(commands[index])
        return {"text": output[:MAX_OUTPUT], "truncated": len(output) > MAX_OUTPUT}

    def comments(self, request_id: str = "", offset: int = 0) -> dict:
        """Read saved comments, optionally filtering bodies by request ID."""
        if offset < 0:
            raise ValueError("offset must be non-negative")
        output = self.run(
            [
                self.policy["binary"],
                "comments",
                "--session",
                self.policy["session"],
                "--all",
                "--json",
            ]
        )
        items = json.loads(output)
        if not isinstance(items, list):
            raise TypeError("unexpected Critty comments response")
        items = [c for c in items if request_id in c.get("body", "")]
        end = offset + PAGE_SIZE
        return {
            "comments": items[offset:end],
            "next_offset": end if end < len(items) else None,
        }

    def post_finding(self, body: str, location: str = "") -> str:
        """Post a finding to the fixed session and author; optional path[:line[-end]]."""
        argv = [
            self.policy["binary"],
            "comment",
            "--session",
            self.policy["session"],
            "--author",
            self.policy["author"],
            "--",
        ]
        if location:
            argv.append(location)
        argv.append(body)
        event = {
            "hook_event_name": "PreToolUse",
            "cwd": str(self.cwd),
            "tool_name": "Bash",
            "tool_input": {"command": shlex.join(argv)},
        }
        if not permitted(event, self.policy):
            raise ValueError(
                "finding outside permitted scope; use capabilities for alternatives"
            )
        return self.run(argv)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("policy", type=Path)
    parser.add_argument("--describe", action="store_true")
    args = parser.parse_args()
    try:
        tools = ReviewTools(json.loads(args.policy.read_text(encoding="utf-8")))
    except (OSError, ValueError, TypeError):
        parser.exit(1, "invalid or unreadable review policy\n")
    if args.describe:
        print(json.dumps(tools.capabilities(), ensure_ascii=False))
        return
    from mcp.server import MCPServer
    from mcp.server.mcpserver.exceptions import ToolError

    def recoverable(method: Callable[..., object]) -> Callable[..., object]:
        @wraps(method)
        def call(*args, **kwargs) -> object:
            try:
                return method(*args, **kwargs)
            except (OSError, ValueError, TypeError, subprocess.SubprocessError) as exc:
                raise ToolError(
                    f"{exc}. Continue with permitted alternatives; consult capabilities."
                ) from exc

        return call

    server = MCPServer("critty_review", instructions=json.dumps(tools.capabilities()))
    for method in [
        tools.capabilities,
        tools.read_file,
        tools.list_files,
        tools.search,
        tools.read_command,
        tools.comments,
        tools.post_finding,
    ]:
        server.tool()(recoverable(method))
    server.run(transport="stdio")


if __name__ == "__main__":
    main()
