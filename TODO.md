# TODO

Near-term, actionable items for marcview. Larger feature proposals live in
[docs/ROADMAP.md](docs/ROADMAP.md).

## Correctness

- [ ] Report the file byte offset for MARCXML records; `Issue.Offset` is `-1`
      there because `XMLReader` does not expose the decoder's position.
- [ ] Decode the 008 fixed field in the annotated view (dates, place, language,
      material-specific bytes 18-34). Only the leader is decoded today.
- [ ] Handle `-n` in `show` before loading the whole file; it currently loads
      everything and then truncates.
- [ ] Check `Record.GetLinkedFields` behaviour on records with repeated `$6`
      occurrence numbers; the 880 pairing assumes it returns only true partners.

## Views

- [ ] Act on the selected field: filter by this subject, open this 856. The
      field cursor exists now, so these are small additions.

## Testing


- [ ] Golden-file tests for the annotated renderer, so layout changes are
      visible in review rather than asserted line by line.
- [ ] Add a terminal-level TUI test with `teatest` to cover key handling; the
      current tests drive the model directly and leave `handleKey` at 0%.
- [ ] Fixture with MARC-8 records (not just UTF-8) to prove the encoding
      detector leaves genuine MARC-8 alone.
- [ ] Fuzz `marcio.Load` against random byte strings; the parser panics on some
      malformed lengths and only the recover in `safeNext` contains it.

## Packaging

- [ ] Tag a release and add a `goreleaser` config; CI builds the four
      OS/arch binaries but nothing publishes them.
- [ ] Push to `github.com/beyto1974/marcview` so the module path resolves; it
      is set but the remote does not exist yet.
- [ ] Consider vendoring or pinning `gomarc`; it is pre-1.0 (v0.2.0) and its
      API may move.

## Documentation

- [ ] Record a short asciinema of the browser for the README.
- [ ] Document the `tag:` filter syntax inside the app's full help, not just in
      the README.
