"""Generate the Critty logo, icon, and social preview in this directory.

Text is converted to outlines, so the SVGs render the same without the font.
Requirements: fontTools, rsvg-convert, and Monaspace Neon SemiBold and
Regular (https://github.com/githubnext/monaspace, SIL OFL).  Pass the
directory holding the fonts as the first argument if it is not
~/Library/Fonts.
"""

import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path

from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.pens.transformPen import TransformPen
from fontTools.ttLib import TTFont

OUT = Path(__file__).resolve().parent
DEFAULT_FONT_DIR = Path.home() / "Library/Fonts"

INK, PAPER = "#111418", "#ffffff"
DARK_INK, DARK_BG = "#e6edf3", "#0d1117"
ACCENT, ACCENT_DARK = "#2ea043", "#3fb950"
DARK_TILE = "#1f2630"

TEXT_SIZE = 200
PAD = 40
TILE = 260
GAP = 50


class Face:
    def __init__(self, path: Path) -> None:
        self.font = TTFont(path)
        self.glyphs = self.font.getGlyphSet()
        self.cmap = self.font.getBestCmap()
        self.upm = self.font["head"].unitsPerEm
        self.cap = self.font["OS/2"].sCapHeight

    def advance(self, size: float) -> float:
        """Return the monospace advance width at the given size."""
        return self.font["hmtx"][self.cmap[ord("c")]][0] * size / self.upm

    def path(self, text: str, size: float, x: float, y: float) -> str:
        """Return SVG path data for text with its baseline at y."""
        scale = size / self.upm
        out = []
        for ch in text:
            glyph = self.cmap[ord(ch)]
            pen = SVGPathPen(self.glyphs)
            self.glyphs[glyph].draw(TransformPen(pen, (scale, 0, 0, -scale, x, y)))
            out.append(pen.getCommands())
            x += self.font["hmtx"][glyph][0] * scale
        return " ".join(out)


@dataclass
class Theme:
    name: str
    ink: str
    accent: str
    tile: str
    paper: str


THEMES = [
    Theme("light", INK, ACCENT, DARK_BG, PAPER),
    Theme("dark", DARK_INK, ACCENT_DARK, DARK_TILE, DARK_BG),
]


def caret(cx: float, top: float, w: float, h: float, stroke: float, color: str) -> str:
    return (
        f'<path d="M{cx - w / 2:.1f} {top + h:.1f} L{cx:.1f} {top:.1f} '
        f'L{cx + w / 2:.1f} {top + h:.1f}" fill="none" stroke="{color}" '
        f'stroke-width="{stroke:.1f}" stroke-linejoin="miter"/>'
    )


def wordmark(face: Face, x: float, y: float, ink: str, accent: str) -> str:
    """Draw "critty" with the caret from crit's logo where "ty" joins."""
    a = face.advance(TEXT_SIZE)
    return (
        f'<path d="{face.path("crit", TEXT_SIZE, x, y)}" fill="{ink}"/>'
        f'<path d="{face.path("ty", TEXT_SIZE, x + 4 * a, y)}" fill="{accent}"/>'
        + caret(
            x + 4 * a,
            y + TEXT_SIZE * 0.1,
            a * 0.55,
            TEXT_SIZE * 0.3,
            TEXT_SIZE * 0.075,
            accent,
        )
    )


def icon(face: Face, x: float, y: float, s: float, bg: str) -> str:
    """Draw a rounded terminal tile with a prompt, cursor, and caret."""
    p = [
        f'<rect x="{x}" y="{y}" width="{s}" height="{s}" rx="{s * 0.22}" fill="{bg}"/>'
    ]
    fs = s * 0.5
    a = face.advance(fs)
    left = x + s * 0.143
    base = y + s * 0.54
    p.append(f'<path d="{face.path(">", fs, left, base)}" fill="{ACCENT_DARK}"/>')
    p.append(
        f'<rect x="{left + a * 1.25:.1f}" y="{base - fs * 0.08:.1f}" '
        f'width="{a * 0.9:.1f}" height="{fs * 0.1:.1f}" fill="{DARK_INK}"/>'
    )
    p.append(
        caret(left + a * 1.7, base + s * 0.08, a * 0.6, s * 0.16, s * 0.05, ACCENT_DARK)
    )
    return "".join(p)


def svg(w: int, h: int, body: str, bg: str | None = None, rx: float = 0) -> str:
    fill = f'<rect width="{w}" height="{h}" rx="{rx:.1f}" fill="{bg}"/>' if bg else ""
    return (
        f'<svg xmlns="http://www.w3.org/2000/svg" width="{w}" height="{h}" '
        f'viewBox="0 0 {w} {h}">{fill}{body}</svg>\n'
    )


def logo(face: Face, theme: Theme, boxed: bool) -> str:
    w = round(PAD * 2 + TILE + GAP + 6 * face.advance(TEXT_SIZE))
    h = PAD * 2 + TILE
    base = PAD + TILE / 2 + face.cap * TEXT_SIZE / face.upm / 2 - 10
    body = icon(face, PAD, PAD, TILE, theme.tile)
    body += wordmark(face, PAD + TILE + GAP, base, theme.ink, theme.accent)
    if boxed:
        # Keep the box corners concentric with the tile corners.
        return svg(w, h, body, theme.paper, TILE * 0.22 + PAD)
    return svg(w, h, body)


PREVIEW_W, PREVIEW_H = 1280, 640
TAGLINE = "Your agents work. You make the decisions."
SUBTITLE = "Reviews and approvals in your terminal."
AGENTS = ["Claude Code", "Codex", "OpenCode", "Antigravity", "Gemini CLI"]
TERMINALS = ["tmux", "Herdr", "Desktop terminals"]
TOOL, MUTED, QUIET = "#79c0ff", "#8b949e", "#6e7681"


def text_line(
    face: Face, parts: list[tuple[str, str | None]], size: float, y: float
) -> str:
    """Center a line of (text, color) parts.

    A part with a None color is a gap whose text gives its width in cells.
    """
    a = face.advance(size)

    def cells(text: str, color: str | None) -> float:
        return len(text) if color else float(text)

    x = (PREVIEW_W - a * sum(cells(t, c) for t, c in parts)) / 2
    out = []
    for text, color in parts:
        if color:
            out.append(f'<path d="{face.path(text, size, x, y)}" fill="{color}"/>')
        x += a * cells(text, color)
    return "".join(out)


def tools(names: list[str]) -> list[tuple[str, str | None]]:
    parts: list[tuple[str, str | None]] = []
    for i, name in enumerate(names):
        if i:
            parts.append((" · ", QUIET))
        parts.append((name, TOOL))
    return parts


def social_preview(mark: Face, body: Face) -> str:
    """Draw the GitHub social preview: the dark logo, tagline, and integrations."""
    theme = next(t for t in THEMES if t.name == "dark")
    scale = 0.85
    w = PAD * 2 + TILE + GAP + 6 * mark.advance(TEXT_SIZE)
    h = PAD * 2 + TILE
    base = PAD + TILE / 2 + mark.cap * TEXT_SIZE / mark.upm / 2 - 10
    logo_body = icon(mark, PAD, PAD, TILE, theme.tile)
    logo_body += wordmark(mark, PAD + TILE + GAP, base, theme.ink, theme.accent)
    x, y = (PREVIEW_W - w * scale) / 2, 235 - h * scale / 2
    out = f'<g transform="translate({x:.1f} {y:.1f}) scale({scale})">{logo_body}</g>'
    out += text_line(body, [(TAGLINE, DARK_INK)], 38, 420)
    out += text_line(body, [(SUBTITLE, MUTED)], 28, 467)
    out += text_line(body, tools(AGENTS), 24, 520)
    # Give "on" a little more room than a single space before the terminals.
    out += text_line(body, [("on", QUIET), ("1.6", None)] + tools(TERMINALS), 24, 560)
    return svg(PREVIEW_W, PREVIEW_H, out, theme.paper)


def write(name: str, text: str) -> None:
    path = OUT / name
    path.write_text(text)
    subprocess.run(["rsvg-convert", path, "-o", path.with_suffix(".png")], check=True)


def main() -> None:
    fonts = Path(sys.argv[1]) if len(sys.argv) > 1 else DEFAULT_FONT_DIR
    face = Face(fonts / "MonaspaceNeon-SemiBold.otf")
    for theme in THEMES:
        write(f"critty-logo-{theme.name}.svg", logo(face, theme, boxed=False))
        write(f"critty-logo-{theme.name}-boxed.svg", logo(face, theme, boxed=True))
    write("critty-icon.svg", svg(512, 512, icon(face, 0, 0, 512, DARK_BG)))
    write(
        "critty-social-preview.svg",
        social_preview(face, Face(fonts / "MonaspaceNeon-Regular.otf")),
    )


if __name__ == "__main__":
    main()
