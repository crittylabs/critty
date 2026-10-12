"""Check Kitty virtual placements in the current terminal without showing an image."""

import argparse
import json
import os
import re
import secrets
import select
import sys
import termios
import time
import tty


def probe(timeout: float = 3) -> dict[str, object]:
    input_fd, output_fd = sys.stdin.fileno(), sys.stdout.fileno()
    if not os.isatty(input_fd) or not os.isatty(output_fd):
        raise RuntimeError("run in an interactive terminal")
    image_id = secrets.randbelow(0xFFFFFF) + 1
    request = (
        "\x1b7\x1b[1;1H"
        f"\x1b_Ga=T,f=32,s=1,v=1,U=1,c=1,r=1,i={image_id};AAAAAA==\x1b\\"
        "\x1b[6n"
        f"\x1b_Ga=d,d=I,q=2,i={image_id}\x1b\\\x1b8"
    ).encode()
    if os.environ.get("TMUX"):
        request = b"\x1bPtmux;" + request.replace(b"\x1b", b"\x1b\x1b") + b"\x1b\\"
    reply = re.compile(rb"\x1b_Gi=" + str(image_id).encode() + rb";([^\x1b]*)\x1b\\")
    cursor = re.compile(rb"\x1b\[(\d+);(\d+)R")
    data = b""
    previous = termios.tcgetattr(input_fd)
    try:
        tty.setraw(input_fd)
        os.write(output_fd, request)
        deadline = time.monotonic() + timeout
        while (remaining := deadline - time.monotonic()) > 0:
            ready, _, _ = select.select([input_fd], [], [], remaining)
            if not ready:
                break
            chunk = os.read(input_fd, 4096)
            if not chunk:
                break
            data += chunk
            if reply.search(data) and cursor.search(data):
                break
    finally:
        termios.tcsetattr(input_fd, termios.TCSANOW, previous)
    response, position = reply.search(data), cursor.search(data)
    accepted = response is not None and response[1] == b"OK"
    coordinates = [int(position[1]), int(position[2])] if position else None
    return {
        "accepted": accepted,
        "cursor": coordinates,
        "virtual_placement": accepted and coordinates == [1, 1],
    }


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--expect", choices=("supported", "unsupported"))
    args = parser.parse_args()
    try:
        result = probe()
    except (OSError, RuntimeError) as error:
        parser.exit(1, f"probe_images: {error}\n")
    print(json.dumps(result))
    if args.expect:
        sys.exit(result["virtual_placement"] != (args.expect == "supported"))
