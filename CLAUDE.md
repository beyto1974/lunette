# CLAUDE.md

Guidance for Claude Code when working in this repository.

## What this is

`marcview`, a Go module (`github.com/beyto1974/marcview`): a Bubble Tea
dual-pane MARC21 browser plus `show`, `export` and `validate` subcommands.
It reads binary MARC21 and MARCXML and converts between them.

It was split out of a harvesting repo, so records typically arrive from
OAI-PMH endpoints — expect real-world damage and mislabeled encodings rather
than clean library exports.

## Commands

```bash
make test        # go test ./...
make race        # go test -race ./...
make cover       # coverage profile and total
make badge       # regenerate docs/coverage.svg
make lint        # go vet + gofmt check
make build       # ./marcview
```

Run a single package: `go test ./internal/render/ -run TestAnnotated -v`.

## House rules

**Tests first.** Every package here was built test-first and the suite is the
contract. New behaviour needs a failing test before the implementation, and the
coverage floor in CI is 70%.

**Verify library APIs before writing against them.** Both major dependencies
are moving targets and published documentation for them is unreliable:

- `gomarc` v0.2.0 has **no** `Record.AsXML()` despite what pkg.go.dev implies;
  single-record XML goes through `marc.NewXMLWriter` into a buffer.
  `Record.String()` is pymarc *breaker* format (`=245  10$aTitle`), not the
  `yaz-marcdump` layout.
- Charm v2 modules live under `charm.land/...`, not `github.com/charmbracelet/...`.
  `tea.Model.View()` returns a `tea.View` struct, and alt screen is
  `View.AltScreen`, not a program option.

Use `go doc` against the module cache. It is the authority; the web is not.

**Layout changes need a golden review.** `internal/tui/testdata/golden/` holds
the whole rendered frame at eight sizes and states. If a change is deliberate,
run `make golden` and read the diff; never regenerate to make a red test green
without looking at what moved.

**Do not silently drop records.** A record that fails to decode becomes a
`marcio.Issue` with its ordinal and byte offset. Anything that reduces the
output count says so on stderr.

**Colour is opt-in.** `render.Options{Color: false}` must produce byte-identical
plain text — export and tests depend on it. A zero-value `lipgloss.Style`
renders unchanged, which is how `plainPalette` works.

## Things already learned the hard way

- Records commonly carry UTF-8 bytes with leader/09 blank, which declares
  MARC-8. `marcio.LooksUTF8` detects this and forces UTF-8; without it
  `données` decodes as `donn©♭es`.
- `gomarc`'s reader panics on a declared record length below 5 (negative slice
  allocation). `safeNext` recovers and ends the walk, since a panic that
  consumed no input would repeat forever.
- lipgloss v2 counts the border inside `Style.Width` and `Style.Height`, unlike
  v1. Passing content width squeezes the content by two cells - which is how
  the list year ended up wrapping onto its own line.
- Test fixtures are committed in both encodings; regenerate `sample.mrc` from
  `sample.marcxml` with `yaz-marcdump`, never by hand.

## Style

Comments explain why, not what. Match the density of the surrounding file:
package docs and non-obvious decisions are commented, mechanical code is not.
Keep `gofmt` clean — CI fails on unformatted files.
