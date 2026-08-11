"""Rebuild escapes.mrc, the terminal-injection fixture. See escapes.md."""

FIELD_END, RECORD_END, SUBFIELD = b"\x1e", b"\x1d", b"\x1f"
ESC = b"\x1b"

payload = b"Harmless Title" + ESC + b"[2J" + ESC + b"]52;c;aGFja2Vk\x07" + ESC + b"[31m"
fields = [(b"001", b"evil-001"), (b"245", b"10" + SUBFIELD + b"a" + payload)]

data, directory = b"", b""
for tag, content in fields:
    body = content + FIELD_END
    directory += tag + b"%04d" % len(body) + b"%05d" % len(data)
    data += body
directory += FIELD_END

base = 24 + len(directory)
total = base + len(data) + 1
leader = b"%05d" % total + b"nam" + b" a22" + b"%05d" % base + b"a  4500"
assert len(leader) == 24

with open("testdata/escapes.mrc", "wb") as f:
    f.write(leader + directory + data + RECORD_END)
