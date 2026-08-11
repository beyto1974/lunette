# marcview

A dual-pane terminal browser for MARC21 files: record list on the left, the
selected record on the right. Reads binary MARC21 (`.mrc`) and MARCXML, and
converts between them from the command line.

![coverage](docs/coverage.svg)

```
┌─ records (5572) ────┬─ 245 $a Identification of Transmission Lines ──┐
│ ▸ 00001 Identifi…   │ LEADER 01045nab  2200181uu 4500                │
│   00002 Gegevens…   │        status=n(new)  type=a(language material)│
│   00003 Sans aut…   │                                                │
│                     │ 100 1# Main Entry - Personal Name              │
│                     │        $a De Block, Wim                        │
│                     │ 245 10 Title Statement                         │
│                     │        $a Identification of Transmission Lines │
└─────────────────────┴────────────────────────────────────────────────┘
 / filter   n next match   : jump   tab pane   c compact   y copy   ? help   q quit
```

## Install

Needs **Go 1.25** or newer (a requirement inherited from `gomarc`; older
toolchains download it automatically).

```bash
go install github.com/beyto1974/marcview@latest
```

Or from a clone:

```bash
make build      # ./marcview
make install    # into GOBIN
```

No runtime dependencies — a single static binary.

## Browse

```bash
marcview records.mrc
marcview records.marcxml            # the format is sniffed from the first bytes
marcview -follow harvest.mrc        # keep reading as a harvest appends records
```

| Key | Action |
|---|---|
| `↑` `↓` `j` `k` | Move — through records in the list, field by field in the record |
| `g` / `G` | First / last record, or first / last field |
| `tab`, `enter` | Switch focus between the panes |
| `z` | One pane at a time; `enter` opens the record, `esc` goes back |
| `ctrl+u` / `ctrl+d` | Half-page scroll in the record pane |
| `/` | Filter (`tag:856 brussels` narrows by field and text) |
| `s` | Cycle where the search looks: titles → record → both |
| `n` / `N` | Next / previous match — between records with the list focused, within the record with the record focused |
| `esc` | Clear the filter |
| `:` | Jump to a record number |
| `a` `c` `r` `J` `X` | Annotated, compact, raw breaker, JSON, XML views |
| `f` | Filter by the selected field — same tag, same text |
| `o` | Open the selected field's URL, or the record's 856, in a browser |
| `y` | Copy — the selected field with the record focused, the whole record otherwise (OSC 52, works over SSH) |
| `?` | Toggle full help (the short row is always visible) |
| `q`, `ctrl+c` | Quit |

The record pane has a cursor of its own: with it focused, `↑`/`↓` step from
field to field rather than line to line, the selected field is marked in the
gutter across every line it wraps onto, and `y` copies just that field. The
structured views have no field structure, so there the same keys scroll.

`-follow` watches a binary MARC21 file that is still being written and picks up
records as they land, which is what you want while `harvest-marc21.sh` is
running. Only whole records are read: a half-written one at the end of the file
is left for the next poll rather than reported as damage. A file that shrinks
has been replaced rather than appended to, so following stops and says so.

`z` collapses the browser to a single pane on any terminal — `enter` opens the
record, `esc` returns to the list — which is also what happens automatically
below about 58 columns.

Each pane carries a header: the list shows the cursor position and how far
through the file it is (`1204/5572 · 22%`, plus `of 5572` when a filter is
hiding records), the record pane shows the record number, its control number,
the view mode and the scroll position.

The mouse works too: click a record to select it, click a pane to focus it, and
scroll the wheel over either pane — the wheel moves what is under the pointer
without stealing focus, so you can skim the record list while reading a record.

Records stream in as they load, so a large file is browsable immediately.
Below about 58 columns the browser shows one pane at a time, following focus.

Long values wrap to the pane width, with continuation lines indented under the
value rather than under the tag, so the tag column stays scannable. Over-long
tokens such as 856 URLs are broken rather than left to overflow. Piped output
is unwrapped unless you pass `-width`.

## Command line

```bash
marcview validate records.mrc                          # counts, failures, exit 1 on damage
marcview encoding records.mrc                          # what encoding the file really uses
marcview show -mode compact -n 5 records.mrc           # print records
marcview show -filter brussels -tag 856 records.mrc    # same criteria as the TUI
marcview show -all -filter privacy records.mrc         # search every subfield
marcview show -width 80 records.mrc                    # wrap long fields at 80 columns
marcview show -mode json -indent records.mrc           # pretty-print json or xml
marcview export -format xml records.mrc > out.marcxml  # mrc | xml | json
marcview export -format mrc -tag 856 -o links.mrc records.mrc
```

`show` prints plain text when piped; add `-color` to force ANSI.

A search has a scope, because the two panes hold different text:

| Scope | Reads | In the browser | On the command line |
|---|---|---|---|
| `titles` (default) | Control number, title, author, year — what the left pane shows | no prefix | `-scope titles` |
| `record` | Every subfield and control field — what the right pane shows | `rec:` prefix | `-scope record` |
| `both` | Either | `all:` prefix | `-scope both`, or `-all` |

`s` cycles the scope in the browser and re-runs the current filter. The default
is the cheap one: the titles index is built at load, the record index walks
every field and is only built when a scope needs it. The difference is not
academic — on a 5572-record harvest, `privacy` matches 21 records by title and
54 by record body, because most of those hits are 650 subject headings.

## Views and highlighting

Five ways to look at a record, switched live:

- **Annotated** (default) — decoded leader, MARC21 field labels, blank
  indicators shown as `#`, one subfield per line, and 880 vernacular fields
  rendered beside the field they translate.
- **Compact** — one field per line with subfields inline,
  `245 10 $a Title $b subtitle`. Keeps the labels off and most records on one
  screen; the form to reach for when comparing records or grepping output.
- **Raw** — pymarc breaker form, `=245  10$aTitle`.
- **JSON** — MARC-in-JSON, indented in the browser (`-indent` on the command
  line; export stays compact).
- **XML** — MARCXML, likewise.

MARC is highlighted natively: tags, indicators, subfield codes, field labels
and the leader each get their own colour. With a filter active the matched term
is highlighted in both panes — in the record and in the list titles — and `n`
and `N` step between matches inside the record, with the position shown in the
pane header — including in the JSON and XML views, where the match is marked by
inverting the cell so that chroma's syntax colour underneath survives. JSON and
XML are highlighted with [chroma](https://github.com/alecthomas/chroma). Colours are ANSI-256 indices,
so they follow the terminal theme rather than fighting it.

## Two things it handles that trip up other readers

**Mislabeled encodings.** Many OAI-PMH repositories emit UTF-8 record bytes
while leaving leader/09 blank, which declares MARC-8. Decoding those as MARC-8
turns `données` into `donn©♭es`. marcview samples the file, and when the bytes
are valid multi-byte UTF-8 with no MARC-8 escape sequences it trusts the bytes
over the leader, then says so in the title bar and in `validate`.

`marcview encoding` reports the evidence in full and exits non-zero on a
conflict, so a harvest script can catch a repository that mislabels its records:

```
format:                     MARC21
records:                    5572
leader/09 says utf-8:       0
leader/09 says marc-8:      5572
records w/ non-ascii:       1517
records w/ marc-8 escapes:  0
records w/ invalid utf-8:   0
mislabelled records:        1517
  for example:              2, 3, 7, 13, 18, 27, … (1507 more)

UTF-8 bytes behind a MARC-8 leader in 1517 record(s); marcview decodes them as
UTF-8, other tools will not
```

It reads raw record bytes rather than decoded records, because MARC-8 escape
sequences and invalid UTF-8 are exactly what a decoder removes.

Export corrects the label. Everything marcview writes is UTF-8 — MARCXML and
MARC-in-JSON are UTF-8 by definition, and binary MARC21 is written as UTF-8 too
— so every exported record carries leader/09 = `a`. Passing a blank leader
through would mislabel the output for whoever reads it next, which is how the
problem spreads in the first place.

**Damaged records.** A bad record is reported with its ordinal and byte offset
rather than silently dropped, and a record short enough to make the underlying
parser panic is contained instead of taking the run down.

## Development

```bash
make test     # go test ./...
make race     # go test -race ./...
make cover    # coverage profile + total
make badge    # regenerate docs/coverage.svg
make lint     # go vet + gofmt check
make golden   # rewrite the golden TUI frames after an intended layout change
```

The TUI layout is covered by golden frames: `internal/tui/testdata/golden/`
holds the whole rendered view at eight terminal sizes and states, stripped of
ANSI so the diffs stay readable. Layout arithmetic is where this package has
gone wrong before — panes two rows too tall, list rows two cells too wide — and
property assertions kept missing it. Change the layout deliberately, run
`make golden`, and read the diff.

Layout:

```
main.go                 subcommand dispatch
internal/marcio/        format sniffing, encoding detection, streaming load
internal/render/        annotated / compact / raw / json / xml renderers, tag labels
internal/export/        mrc / xml / json writers, filtering
internal/tui/           Bubble Tea model, panes, keys, styles
testdata/               3-record fixtures in both encodings
```

`testdata/sample.mrc` is generated from the committed MARCXML:

```bash
yaz-marcdump -i marcxml -o marc -f utf-8 -t utf-8 \
  testdata/sample.marcxml > testdata/sample.mrc
```

Planned work is in [TODO.md](TODO.md) and [docs/ROADMAP.md](docs/ROADMAP.md).

Built on [gomarc](https://github.com/beyto1974/gomarc) for MARC parsing and the
[Charm](https://charm.sh) v2 stack (Bubble Tea, Bubbles, Lip Gloss) for the UI.

## Licence

MIT — see [LICENSE](LICENSE).
