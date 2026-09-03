# lunette

A terminal browser for MARC21 files: record list on the left, record on the
right. Reads binary MARC21 and MARCXML, and converts between them. Files larger
than memory are the ordinary case: a 3.7 GB harvest of 1.4 million records
opens in a few minutes and half a gigabyte, and `lunette show -n 1` on it
answers instantly.

![lunette browsing a MARC file](docs/screenshot-browse.svg)

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/beyto1974/lunette/main/install.sh | sh
```

That fetches the right binary for your platform, checks it against the
published `checksums.txt`, and puts it in `~/.local/bin`. Set `PREFIX` to
install elsewhere and `LUNETTE_VERSION` to pin a release. To read it before
running it — reasonable, for anything piped into a shell:

```bash
curl -fsSLO https://raw.githubusercontent.com/beyto1974/lunette/main/install.sh
less install.sh && sh install.sh
```

Otherwise: `go install github.com/beyto1974/lunette@latest` (needs Go 1.25.13),
a binary for linux, macOS or Windows from the
[releases](https://github.com/beyto1974/lunette/releases), or
`git clone … && make build`. Try it without installing anything with
`go run github.com/beyto1974/lunette@latest records.mrc`.

Nothing is needed at runtime — it is one static binary.

## Browse

```bash
lunette records.mrc
lunette records.marcxml         # the format is detected, not assumed
lunette part-*.mrc              # several files read as one set
lunette -follow harvest.mrc     # keep reading while a harvest writes the file
```

| Key | |
|---|---|
| `↑` `↓` `j` `k` | Move: records in the list, fields in the record |
| `tab` `enter` | Switch pane · `z` one pane at a time |
| `/` | Filter · `s` cycles where it looks: titles, record, both |
| `n` `N` | Next, previous match |
| `:` | Jump to a record number |
| `a` `c` `r` `J` `X` | Annotated, compact, raw, JSON, XML |
| `f` | Filter by the selected field |
| `o` | Open the field's URL, or the record's 856 |
| `y` | Copy the field, or the whole record |
| `?` | All keys · `q` quit |

The mouse works too: click to select, scroll either pane.

## Command line

```bash
lunette validate records.mrc                       # exit 1 if a record fails to decode
lunette encoding records.mrc                       # what encoding the file really uses
lunette show -mode compact records.mrc             # print records
lunette show -scope record -filter privacy f.mrc   # search inside records, not just titles
lunette export -format xml records.mrc > out.xml   # mrc, xml or json
metha-cat -format marc21 "$URL" | lunette show -   # "-" is standard input
```

`show -mode` takes the same five views as the browser. Colour is on when
output is a terminal and off when it is piped; `-color`, `-no-color` and
`NO_COLOR` override that.

## Notes

- **Encodings.** Repositories often emit UTF-8 while leaving leader/09 blank,
  which claims MARC-8; read literally that turns `données` into `donn©♭es`.
  lunette trusts the bytes, says so in the title bar, and labels exports
  correctly. `lunette encoding` reports the evidence.
- **Large files.** Nothing reads a whole file into memory. `show`, `export` and
  `validate` stream, and `show -n` stops reading once it has what it was asked
  for. MARCXML is cut into blocks of whole records and decoded on every core.
  The browser keeps a list row and a byte offset per record and fetches the
  record itself when the cursor lands on it, so what it holds is measured in
  hundreds of bytes per record rather than kilobytes.
- **Damaged records** are reported with their offset, not skipped silently, and
  a record that would crash the parser is contained. In MARCXML the damage
  costs the record holding it: the rest of the block is read again one record
  at a time, so a harvest cut off mid-record still gives up everything else.
- **Untrusted input.** Control characters in a record are shown as `^[` rather
  than sent to the terminal, where they could rewrite the screen or reach the
  clipboard.
- **Dependencies.** MARC parsing is [gomarc](https://github.com/beyto1974/gomarc),
  which is pre-1.0 and may change under us; `make vuln` runs govulncheck, as
  does CI.

Longer explanations are in [docs/notes.md](docs/notes.md); planned work is in
[TODO.md](TODO.md) and [docs/ROADMAP.md](docs/ROADMAP.md); how to report a
vulnerability is in [SECURITY.md](SECURITY.md).

## Development

```bash
make test          # also: race, cover, lint
make golden        # rewrite the TUI frame snapshots after a layout change
make screenshots   # regenerate the screenshot above
make snapshot      # build the release archives without publishing
```

Releases are cut by pushing a tag: GoReleaser builds the six binaries, stamps
the version into `lunette version`, and publishes archives and checksums.

Built on [gomarc](https://github.com/beyto1974/gomarc) and the
[Charm](https://charm.sh) stack.

## Licence

MIT — see [LICENSE](LICENSE).
