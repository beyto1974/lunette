# Notes

Longer explanations that would crowd the README.

## The five views

- **Annotated** (default) — decoded leader, MARC21 field labels, blank
  indicators shown as `#`, one subfield per line, and 880 vernacular fields
  rendered beside the field they translate rather than stranded at the end.
- **Compact** — one field per line with subfields inline,
  `245 10 $a Title $b subtitle`. Most records fit on one screen; this is the
  form to grep.
- **Raw** — pymarc breaker format, `=245  10$aTitle`.
- **JSON** — MARC-in-JSON, indented in the browser, compact from `export`.
- **XML** — MARCXML, likewise.

Tags, indicators, subfield codes, labels and the leader each get their own
colour, and the active filter term is highlighted — including in the JSON and
XML views, where the match is marked by inverting the cell so that the syntax
colour underneath survives. Colours are ANSI-256 indices, so they follow the
terminal theme rather than fighting it.

Long values wrap to the pane with continuation lines indented under the value,
not under the tag, and over-long tokens such as 856 URLs are broken rather than
left to overflow. Piped output is unwrapped unless `show -width` says otherwise.

## Searching

A search has a scope, because the two panes hold different text:

| Scope | Reads | Browser | Command line |
|---|---|---|---|
| `titles` (default) | Control number, title, author, year | no prefix | `-scope titles` |
| `record` | Every subfield and control field | `rec:` prefix | `-scope record` |
| `both` | Either | `all:` prefix | `-scope both`, or `-all` |

`s` cycles the scope and re-runs the filter. Titles is the default because the
two indexes cost different amounts: the list key is built during load, the
record index walks every field of every record and is built only when a scope
asks for it. The difference is not academic — on a 5572-record harvest,
`privacy` matches 21 records by title and 54 by record body, because most of
those hits are 650 subject headings.

`tag:856` narrows by field presence and combines with either scope.

## Encodings

MARC21 says what encoding a record uses in leader/09: `a` for UTF-8, blank for
MARC-8. Many OAI-PMH repositories emit UTF-8 bytes and leave the leader blank,
so a reader that believes the leader turns `données` into `donn©♭es`.

lunette samples the file, and when the bytes are valid multi-byte UTF-8 with no
MARC-8 escape sequences it decodes them as UTF-8 whatever the leader says, then
reports the override in the title bar and in `validate`. `lunette encoding`
gives the full picture: the leader/09 distribution, how many records hold
non-ASCII, how many carry real MARC-8 escape sequences, how many hold bytes
that are neither ASCII nor valid UTF-8, and which records are mislabelled. It
exits non-zero on a conflict, so a harvest script can gate on it.

It reads raw record bytes rather than decoded records, because escape sequences
and invalid UTF-8 are exactly what a decoder removes.

Exports are labelled correctly on the way out: everything lunette writes is
UTF-8, so every exported record carries leader/09 = `a`. Passing a blank leader
through is how the problem spreads.

## Following a file

`-follow` watches a binary MARC21 file while it grows, through inotify on Linux
and kqueue on BSD and macOS, so records appear as they are written rather than
at the end of a polling interval. A burst of writes is coalesced into one read.

Only whole records are read: a half-written one at the end of the file is left
for the next read rather than reported as damage, and the initial load is
bounded the same way. A file that shrinks was replaced rather than appended to,
so following stops and says so. Where a watch cannot be established the browser
falls back to a one-second timer and says so, and a slow re-check runs alongside
the watch regardless, since a watch on a network filesystem can miss writes made
by another host.

Following is binary-only: a MARCXML document is not a document until its
closing tag.

## Untrusted input

MARC files come from other people's repositories, so record content is treated
as untrusted:

- **Control characters** are rewritten as caret notation before anything is
  displayed or copied. A record carrying `ESC [ 2 J` would otherwise clear the
  screen, and one carrying an OSC 52 sequence would write to the user's
  clipboard. `testdata/escapes.mrc` is such a record, and the tests use it.
- **Links** are handed to the desktop only if they are http or https, carry no
  control characters, and are of sane length.
- **Damaged records** are reported with their ordinal and byte offset rather
  than dropped, and a record short enough to make the parser panic is contained
  rather than taking the run down.
- **Reads are bounded**: a single incremental read is capped, and the file walk
  that finds record boundaries seeks rather than loading the file.
- **`export -o`** refuses to write over its own input, and over any existing
  file unless `-force` is given.

## Layout and tests

The TUI layout is covered by golden frames in `internal/tui/testdata/golden/`:
the whole rendered view at eight terminal sizes and states, stripped of ANSI so
the diffs stay readable. Layout arithmetic is where this package has gone wrong
before — panes two rows too tall, list rows two cells too wide — and property
assertions kept missing it. Change the layout deliberately, run `make golden`,
and read the diff.

The screenshots in the README are produced the same way: `make screenshots`
drives the real binary in a pty, replays the output through a small terminal
emulator to get one whole frame, and renders that to SVG. Only
`testdata/sample.mrc` is used, so the images contain no third-party records.
