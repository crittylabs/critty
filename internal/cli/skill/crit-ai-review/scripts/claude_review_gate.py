#!/usr/bin/env python3
"""Allow review research and narrowly scoped Critty comment writes."""

import argparse
import json
import re
import shlex
import sys
from pathlib import Path


def valid_policy(policy: object) -> bool:
    if not isinstance(policy, dict):
        return False
    for key in ("cwd", "binary", "session", "author"):
        if not isinstance(policy.get(key), str) or not policy[key]:
            return False
    if (
        not Path(policy["cwd"]).is_absolute()
        or not Path(policy["binary"]).is_absolute()
    ):
        return False
    if not re.fullmatch(r"[0-9a-f]{12}", policy["session"]):
        return False
    for key in ("locations", "read_roots"):
        value = policy.get(key)
        if not isinstance(value, list) or not all(
            isinstance(x, str) and x for x in value
        ):
            return False
    if not all(Path(x).is_absolute() for x in policy["read_roots"]):
        return False
    if any(re.search(r":[0-9]+(?:-[0-9]+)?$", path) for path in policy["locations"]):
        return False
    targets = policy.get("resolvable_comments", [])
    if not isinstance(targets, list):
        return False
    for target in targets:
        if (
            not isinstance(target, dict)
            or set(target) != {"id", "path"}
            or not isinstance(target["id"], str)
            or not re.fullmatch(r"[cr]_[0-9a-f]{6}", target["id"])
            or not isinstance(target["path"], str)
            or (target["id"].startswith("r_") != (target["path"] == ""))
        ):
            return False
    commands = policy.get("read_commands")
    return isinstance(commands, list) and all(
        isinstance(command, list)
        and bool(command)
        and all(isinstance(arg, str) and arg for arg in command)
        and Path(command[0]).is_absolute()
        for command in commands
    )


def permitted(event: dict, policy: dict) -> bool:
    if not valid_policy(policy) or not isinstance(event, dict):
        return False
    if event.get("hook_event_name") != "PreToolUse":
        return False
    cwd = event.get("cwd")
    if (
        not isinstance(cwd, str)
        or not cwd
        or Path(cwd).resolve() != Path(policy["cwd"]).resolve()
    ):
        return False
    tool_input = event.get("tool_input")
    if not isinstance(tool_input, dict):
        return False
    if event.get("tool_name") in {"Read", "Glob", "Grep"}:
        if event["tool_name"] == "Glob":
            pattern = tool_input.get("pattern")
            if (
                not isinstance(pattern, str)
                or Path(pattern).is_absolute()
                or ".." in Path(pattern).parts
            ):
                return False
        field = "file_path" if event["tool_name"] == "Read" else "path"
        path = tool_input.get(field, cwd if field == "path" else None)
        if not isinstance(path, str) or not path:
            return False
        candidate = Path(path)
        if not candidate.is_absolute():
            candidate = Path(policy["cwd"]) / candidate
        return any(
            candidate.resolve().is_relative_to(Path(root).resolve())
            for root in policy["read_roots"]
        )
    if event.get("tool_name") != "Bash":
        return False
    if tool_input.get("run_in_background"):
        return False
    command = tool_input.get("command", "")
    if not isinstance(command, str):
        return False
    try:
        args = shlex.split(command, posix=True)
    except ValueError:
        return False
    # Accept only the exact POSIX quoting produced by shlex.join.  Parsing
    # alone would erase quotes around expansions and shell operators.
    if not args or shlex.join(args) != command:
        return False
    if args in policy["read_commands"]:
        return True
    binary, session, author = policy["binary"], policy["session"], policy["author"]
    for target in policy.get("resolvable_comments", []):
        for resolve in (False, True):
            prefix = resolution_argv(policy, target, resolve)
            if (
                args[: len(prefix)] == prefix
                and len(args) == len(prefix) + 1
                and args[-1].strip()
            ):
                return True
    if args == [binary, "comments", "--session", session, "--all", "--json"]:
        return True
    prefix = [binary, "comment", "--session", session, "--author", author, "--"]
    if args[: len(prefix)] != prefix:
        return False
    positional = args[len(prefix) :]
    if len(positional) == 1:
        return bool(positional[0].strip())
    if len(positional) != 2 or not positional[1].strip():
        return False
    location = positional[0]
    if location in policy["locations"]:
        return True
    path, separator, lines = location.rpartition(":")
    if not separator or path not in policy["locations"]:
        return False
    if not re.fullmatch(r"[1-9][0-9]*(?:-[1-9][0-9]*)?", lines):
        return False
    start, _, end = lines.partition("-")
    return not end or int(end) >= int(start)


def resolution_argv(policy: dict, target: dict, resolve: bool = True) -> list[str]:
    args = [
        policy["binary"],
        "comment",
        "--session",
        policy["session"],
        "--author",
        policy["author"],
        "--reply-to",
        target["id"],
    ]
    if resolve:
        args.append("--resolve")
    if target["path"]:
        args.extend(["--path", target["path"]])
    return args + ["--"]


def describe(policy: dict) -> dict:
    return {
        "read_roots": policy["read_roots"],
        "read_tools": ["Read", "Glob", "Grep"],
        "read_commands": [shlex.join(command) for command in policy["read_commands"]],
        "list_comments": shlex.join(
            [
                policy["binary"],
                "comments",
                "--session",
                policy["session"],
                "--all",
                "--json",
            ]
        ),
        "post_prefix": shlex.join(
            [
                policy["binary"],
                "comment",
                "--session",
                policy["session"],
                "--author",
                policy["author"],
                "--",
            ]
        ),
        "comment_paths": policy["locations"],
        "resolve_prefixes": [
            shlex.join(resolution_argv(policy, target))
            for target in policy.get("resolvable_comments", [])
        ],
        "reply_prefixes": [
            shlex.join(resolution_argv(policy, target, False))
            for target in policy.get("resolvable_comments", [])
        ],
        "posting": "Append an optional permitted path[:line[-end]], then the literal body, using canonical POSIX quoting.",
        "denial_recovery": "Continue with allowed research. Report unavailable evidence; ask the requester only when it is essential. Do not repeat denied operations or bypass the gate.",
        "forbidden": ["file edits", "tests", "builds", "other commands", "delegation"],
    }


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("policy", type=Path)
    parser.add_argument("--describe", action="store_true")
    options = parser.parse_args()
    allowed = False
    reason = "Invalid review policy or tool request; ask the requester for corrected configuration."
    try:
        policy = json.loads(options.policy.read_text(encoding="utf-8"))
        if not valid_policy(policy):
            raise ValueError("invalid policy")
        capabilities = describe(policy)
        if options.describe:
            print(json.dumps(capabilities, ensure_ascii=False))
            return
        event = json.load(sys.stdin)
        allowed = permitted(event, policy)
        reason = (
            "Permitted review operation."
            if allowed
            else json.dumps(capabilities, ensure_ascii=False)
        )
    except (OSError, ValueError, KeyError, TypeError, IndexError, AttributeError):
        if options.describe:
            parser.exit(1, "invalid or unreadable review policy\n")
    print(
        json.dumps(
            {
                "hookSpecificOutput": {
                    "hookEventName": "PreToolUse",
                    "permissionDecision": "allow" if allowed else "deny",
                    "permissionDecisionReason": reason,
                }
            }
        )
    )


if __name__ == "__main__":
    main()
