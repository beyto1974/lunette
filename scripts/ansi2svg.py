"""Render a captured terminal frame as SVG.

Takes the ANSI text that capture.py produces and writes an SVG that any browser
can display, using its own monospace face rather than an embedded font - which
keeps the file a few kilobytes rather than a third of a megabyte.

Usage: ansi2svg.py <frame.txt> <out.svg> [title]
"""

import html
import re
import sys

FONT_SIZE = 13.0
CELL_W = FONT_SIZE * 0.6009  # DejaVu Sans Mono advance width
CELL_H = FONT_SIZE * 1.35
PAD = 14.0
TITLEBAR = 26.0
BACKGROUND = "#171717"
FOREGROUND = "#c6c6c6"
RADIUS = 6.0

SGR = re.compile(r"\x1b\[([0-9;]*)m")


def palette(n):
    """The xterm 256-colour palette as hex."""
    if n < 16:
        base = [
            (0, 0, 0), (205, 49, 49), (13, 188, 121), (229, 229, 16),
            (36, 114, 200), (188, 63, 188), (17, 168, 205), (229, 229, 229),
            (102, 102, 102), (241, 76, 76), (35, 209, 139), (245, 245, 67),
            (59, 142, 234), (214, 112, 214), (41, 184, 219), (255, 255, 255),
        ]
        r, g, b = base[n]
    elif n < 232:
        n -= 16
        levels = [0, 95, 135, 175, 215, 255]
        r, g, b = levels[n // 36], levels[(n // 6) % 6], levels[n % 6]
    else:
        v = 8 + (n - 232) * 10
        r = g = b = v
    return f"#{r:02x}{g:02x}{b:02x}"


class Style:
    __slots__ = ("fg", "bg", "bold", "inverse")

    def __init__(self):
        self.fg = self.bg = None
        self.bold = self.inverse = False

    def copy(self):
        s = Style()
        s.fg, s.bg, s.bold, s.inverse = self.fg, self.bg, self.bold, self.inverse
        return s

    def apply(self, params):
        codes = [int(p) if p else 0 for p in params.split(";")] if params else [0]
        i = 0
        while i < len(codes):
            c = codes[i]
            if c == 0:
                self.fg = self.bg = None
                self.bold = self.inverse = False
            elif c == 1:
                self.bold = True
            elif c == 7:
                self.inverse = True
            elif c == 22:
                self.bold = False
            elif c == 27:
                self.inverse = False
            elif c == 39:
                self.fg = None
            elif c == 49:
                self.bg = None
            elif 30 <= c <= 37:
                self.fg = palette(c - 30)
            elif 90 <= c <= 97:
                self.fg = palette(c - 90 + 8)
            elif 40 <= c <= 47:
                self.bg = palette(c - 40)
            elif 100 <= c <= 107:
                self.bg = palette(c - 100 + 8)
            elif c in (38, 48) and i + 2 < len(codes) and codes[i + 1] == 5:
                colour = palette(codes[i + 2])
                if c == 38:
                    self.fg = colour
                else:
                    self.bg = colour
                i += 2
            elif c in (38, 48) and i + 4 < len(codes) and codes[i + 1] == 2:
                colour = "#%02x%02x%02x" % tuple(codes[i + 2:i + 5])
                if c == 38:
                    self.fg = colour
                else:
                    self.bg = colour
                i += 4
            i += 1

    def colours(self):
        fg = self.fg or FOREGROUND
        bg = self.bg
        if self.inverse:
            fg, bg = bg or BACKGROUND, fg
        return fg, bg


def runs(line):
    """Split a line into (text, style) runs."""
    out, style, pos = [], Style(), 0
    for match in SGR.finditer(line):
        if match.start() > pos:
            out.append((line[pos:match.start()], style.copy()))
        style.apply(match.group(1))
        pos = match.end()
    if pos < len(line):
        out.append((line[pos:], style.copy()))
    return out


def main():
    source, target, *rest = sys.argv[1:]
    title = rest[0] if rest else ""

    lines = open(source, encoding="utf-8").read().split("\n")
    while lines and not lines[-1].strip():
        lines.pop()

    columns = max((len(SGR.sub("", line)) for line in lines), default=0)
    width = columns * CELL_W + PAD * 2
    height = len(lines) * CELL_H + PAD * 2 + TITLEBAR

    parts = [
        f'<svg xmlns="http://www.w3.org/2000/svg" width="{width:.0f}" height="{height:.0f}" '
        f'viewBox="0 0 {width:.0f} {height:.0f}" role="img" aria-label="{html.escape(title or source)}">',
        f'<rect width="{width:.0f}" height="{height:.0f}" rx="{RADIUS}" fill="{BACKGROUND}"/>',
    ]
    for i, colour in enumerate(("#ff5f56", "#ffbd2e", "#27c93f")):
        parts.append(f'<circle cx="{PAD + 6 + i * 15:.0f}" cy="{TITLEBAR / 2 + 3:.0f}" r="5" fill="{colour}"/>')

    parts.append(
        f'<g font-family="ui-monospace,SFMono-Regular,Menlo,DejaVu Sans Mono,monospace" '
        f'font-size="{FONT_SIZE}px" xml:space="preserve">'
    )

    for row, line in enumerate(lines):
        y = PAD + TITLEBAR + row * CELL_H + FONT_SIZE
        column = 0
        for text, style in runs(line):
            if not text:
                continue
            fg, bg = style.colours()
            x = PAD + column * CELL_W
            span = len(text) * CELL_W

            if bg:
                parts.append(
                    f'<rect x="{x:.2f}" y="{y - FONT_SIZE + 2:.2f}" '
                    f'width="{span:.2f}" height="{CELL_H:.2f}" fill="{bg}"/>'
                )

            # Give every glyph its own x. SVG takes a list of coordinates,
            # one per character, which puts each cell exactly where the
            # terminal had it whatever monospace face the viewer resolves and
            # without depending on textLength, which renderers implement
            # unevenly - one of them drops it entirely and tears holes in the
            # box-drawing rules.
            weight = ' font-weight="bold"' if style.bold else ""
            positions = " ".join(f"{x + i * CELL_W:.1f}" for i in range(len(text)))
            parts.append(
                f'<text x="{positions}" y="{y:.2f}" fill="{fg}"{weight}>'
                f'{html.escape(text)}</text>'
            )
            column += len(text)

    parts.append("</g></svg>")
    with open(target, "w", encoding="utf-8") as f:
        f.write("\n".join(parts) + "\n")
    print(f"==> {target} ({columns}x{len(lines)})")


if __name__ == "__main__":
    main()
