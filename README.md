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
marcview records.marcxml    # the format is sniffed from the first bytes
```

| Key | Action |
|---|---|
| `↑` `↓` `j` `k` | Move through records |
| `g` / `G` | First / last record |
| `tab`, `enter` | Switch focus between the panes |
| `ctrl+u` / `ctrl+d` | Half-page scroll in the record pane |
| `/` | Filter (`tag:856 brussels` narrows by field and text; `all:` searches every subfield) |
| `n` / `N` | Next / previous match inside the record |
| `esc` | Clear the filter |
| `:` | Jump to a record number |
| `a` `c` `r` `J` `X` | Annotated, compact, raw breaker, JSON, XML views |
| `y` | Copy the current record to the clipboard (OSC 52, works over SSH) |
| `?` | Toggle full help |
| `q`, `ctrl+c` | Quit |

Records stream in as they load, so a large file is browsable immediately.
Below about 58 columns the browser shows one pane at a time, following focus.

## Command line

```bash
marcview validate records.mrc                          # counts, failures, exit 1 on damage
marcview show -mode compact -n 5 records.mrc           # print records
marcview show -filter brussels -tag 856 records.mrc    # same criteria as the TUI
marcview show -all -filter privacy records.mrc         # search every subfield
marcview export -format xml records.mrc > out.marcxml  # mrc | xml | json
marcview export -format mrc -tag 856 -o links.mrc records.mrc
```

`show` prints plain text when piped; add `-color` to force ANSI.

By default a filter matches the record's control number, title, author and
year. `-all` (or the `all:` prefix in the browser) searches every subfield
instead, which finds terms that only appear in a subject or a note — on a
5572-record harvest, `privacy` matches 21 records by key and 54 in full text.

## Views and highlighting

Five ways to look at a record, switched live:

- **Annotated** (default) — decoded leader, MARC21 field labels, blank
  indicators shown as `#`, one subfield per line, and 880 vernacular fields
  rendered beside the field they translate.
- **Compact** — one field per line with subfields inline,
  `245 10 $a Title $b subtitle`. Keeps the labels off and most records on one
  screen; the form to reach for when comparing records or grepping output.
- **Raw** — pymarc breaker form, `=245  10$aTitle`.
- **JSON** — MARC-in-JSON.
- **XML** — MARCXML.

MARC is highlighted natively: tags, indicators, subfield codes, field labels
and the leader each get their own colour. With a filter active the matched term
is highlighted in both panes — in the record and in the list titles — and `n`
and `N` step between matches inside the record, with the position shown in the
pane header. JSON and XML are highlighted with
[chroma](https://github.com/alecthomas/chroma). Colours are ANSI-256 indices,
so they follow the terminal theme rather than fighting it.

## Two things it handles that trip up other readers

**Mislabeled encodings.** Many OAI-PMH repositories emit UTF-8 record bytes
while leaving leader/09 blank, which declares MARC-8. Decoding those as MARC-8
turns `données` into `donn©♭es`. marcview samples the file, and when the bytes
are valid multi-byte UTF-8 with no MARC-8 escape sequences it trusts the bytes
over the leader, then says so in the title bar and in `validate`.

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
```

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
