"""Capture one frame of the browser as ANSI text, for the README screenshots.

The browser draws incrementally: the pty carries a stream of partial updates,
cursor jumps and erases, not a picture. Stripping the control sequences out of
that stream does not leave a frame behind - the sequences are where much of the
layout lives. So this replays the stream into a grid of cells the way a
terminal would, then prints the grid.

Usage: capture.py <output.txt> <binary> <file> [keys ...]
"""

import fcntl
import os
import pty
import re
import signal
import struct
import sys
import termios
import time

ROWS, COLS = 26, 98
SETTLE = 1.5

CSI = re.compile(r"\x1b\[([0-9;?]*)([@-~])")
OSC = re.compile(r"\x1b[\]P][^\x07\x1b]*(?:\x07|\x1b\\)?")


class Screen:
    """As much of a terminal as these frames need."""

    def __init__(self, rows, cols):
        self.rows, self.cols = rows, cols
        self.cells = [[(" ", "") for _ in range(cols)] for _ in range(rows)]
        self.row = self.col = 0
        self.sgr = ""

    def put(self, ch):
        if self.col >= self.cols:
            self.col = 0
            self.row += 1
        if 0 <= self.row < self.rows and 0 <= self.col < self.cols:
            self.cells[self.row][self.col] = (ch, self.sgr)
        self.col += 1

    def erase(self, count):
        for i in range(count):
            col = self.col + i
            if 0 <= self.row < self.rows and col < self.cols:
                self.cells[self.row][col] = (" ", "")

    def erase_line(self, mode):
        start, end = {0: (self.col, self.cols), 1: (0, self.col + 1), 2: (0, self.cols)}[mode]
        for col in range(start, min(end, self.cols)):
            if 0 <= self.row < self.rows:
                self.cells[self.row][col] = (" ", "")

    def clear(self):
        self.cells = [[(" ", "") for _ in range(self.cols)] for _ in range(self.rows)]

    def feed(self, text):
        text = OSC.sub("", text)
        i = 0
        while i < len(text):
            ch = text[i]
            if ch == "\x1b":
                match = CSI.match(text, i)
                if not match:
                    i += 1
                    continue
                self.control(match.group(1), match.group(2))
                i = match.end()
                continue
            if ch == "\n":
                self.row, self.col = self.row + 1, 0
            elif ch == "\r":
                self.col = 0
            elif ch == "\t":
                self.col = min(self.col + 8 - self.col % 8, self.cols)
            elif ch >= " ":
                self.put(ch)
            i += 1

    def control(self, params, final):
        if params.startswith("?"):
            return  # private mode set/reset: alt screen, mouse, cursor
        numbers = [int(p) if p else 0 for p in params.split(";")] if params else []
        first = numbers[0] if numbers else 0

        if final == "m":
            self.sgr = "" if not params or params == "0" else "\x1b[" + params + "m"
        elif final in "Hf":
            self.row = (numbers[0] - 1) if numbers and numbers[0] else 0
            self.col = (numbers[1] - 1) if len(numbers) > 1 and numbers[1] else 0
        elif final == "G":
            self.col = max(first - 1, 0)
        elif final == "A":
            self.row -= max(first, 1)
        elif final == "B":
            self.row += max(first, 1)
        elif final == "C":
            self.col += max(first, 1)
        elif final == "D":
            self.col -= max(first, 1)
        elif final == "X":
            self.erase(max(first, 1))
        elif final == "K":
            self.erase_line(first)
        elif final == "J":
            self.clear()
        self.row = max(0, min(self.row, self.rows - 1))
        self.col = max(0, self.col)

    def render(self):
        """The grid as ANSI text, one escape per run of like-styled cells."""
        lines = []
        for row in self.cells:
            out, current = [], ""
            for ch, sgr in row:
                if sgr != current:
                    if current:
                        out.append("\x1b[0m")
                    if sgr:
                        out.append(sgr)
                    current = sgr

                out.append(ch)
            if current:
                out.append("\x1b[0m")
            lines.append("".join(out).rstrip())
        while lines and not lines[-1].strip():
            lines.pop()
        return "\n".join(lines) + "\n"


def resize(fd, pid, rows, cols):
    fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", rows, cols, 0, 0))
    os.kill(pid, signal.SIGWINCH)


def drain(fd, seconds):
    chunks = []
    end = time.time() + seconds
    while time.time() < end:
        try:
            chunks.append(os.read(fd, 65536))
        except (BlockingIOError, OSError):
            time.sleep(0.02)
    return b"".join(chunks)


def main():
    output, binary, marc_file, *keys = sys.argv[1:]

    pid, fd = pty.fork()
    if pid == 0:
        os.environ["TERM"] = "xterm-256color"
        os.environ["COLORTERM"] = "truecolor"
        os.execv(binary, [binary, marc_file])

    fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", ROWS, COLS, 0, 0))
    os.set_blocking(fd, False)

    screen = Screen(ROWS, COLS)
    screen.feed(drain(fd, SETTLE).decode("utf-8", "replace"))

    for chunk in keys:
        os.write(fd, chunk.encode().decode("unicode_escape").encode("latin-1"))
        screen.feed(drain(fd, 0.5).decode("utf-8", "replace"))

    # Typing leaves the screen half-redrawn, so ask for a full repaint and
    # read it into a clean grid: a resize to the same size changes nothing and
    # repaints nothing, hence the detour through one column narrower.
    resize(fd, pid, ROWS, COLS - 1)
    drain(fd, 0.6)

    screen = Screen(ROWS, COLS)
    resize(fd, pid, ROWS, COLS)
    screen.feed(drain(fd, 1.0).decode("utf-8", "replace"))

    os.write(fd, b"q")
    time.sleep(0.3)
    try:
        os.close(fd)
    except OSError:
        pass

    with open(output, "w", encoding="utf-8") as f:
        f.write(screen.render())
    print(f"{output}: captured")


if __name__ == "__main__":
    main()
