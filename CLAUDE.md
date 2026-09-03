# CLAUDE.md

Guidance for Claude Code when working in this repository.

## What this is

`lunette`, a Go module (`github.com/beyto1974/lunette`): a Bubble Tea
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
make audit       # everything to check before pushing
make hooks       # run that audit automatically on every push
make build       # ./lunette
```

Run a single package: `go test ./internal/render/ -run TestAnnotated -v`.

## Before pushing

This repository is public, and a push is permanent: history can be rewritten
only until someone has fetched it. Run `make audit` before pushing anything,
and read what it says rather than watching the exit code. `make hooks` points
`core.hooksPath` at `scripts/hooks`, so the audit then runs on every push
without being remembered; a push that has to go out regardless takes
`--no-verify`. Hooks are not copied by a clone, so each checkout opts in once.

It checks what a machine can decide: no `Claude-Session:` trailers in the
commit messages, every author a noreply address, no secret-shaped strings, no
email addresses, no local paths like `/opt/…` or `/home/…`, no unexpectedly
large file, a clean tree, gofmt, vet, the tests, shellcheck, govulncheck
reachability, the release config, and every relative link in the README.

Four things it cannot decide, which stay a judgement each time:

- **Whose data is in the fixtures.** `testdata/sample.marcxml` once carried real
  people's names and URLs on a real institution's domain, copied from a
  catalogue. Invented names cost nothing; someone else's do not belong here.
- **What the screenshots show.** They are generated from `testdata/`, and
  regenerating them from a real harvest would publish that harvest.
- **What a new dependency brings.** govulncheck reports known advisories, not
  whether a library deserves trust.
- **Whether an error message leaks a path** a user did not supply.

If a session URL or anything else slips into a commit, it can still be stripped
while the branch is unpushed - the recipe is below. Afterwards it cannot.

## Commits

**No `Claude-Session:` trailers.** This history is public and a session link
means nothing to anyone but the person who made it. `.claude/settings.json`
sets `attribution.sessionUrl: false` to stop them being written; if one appears
anyway — a stale session that started before that file existed — strip it
before pushing. `Co-Authored-By:` stays.

**Conventional prefixes**: `feat:`, `fix:`, `test:`, `docs:`, `build:`,
`refactor:`. `.goreleaser.yaml` groups the changelog by them and drops `test:`,
`chore:` and `docs:`, so the prefix decides whether a commit reaches the
release notes.

The 38 commits made before that setting existed were cleaned in one pass, which
is why every hash changed. Only safe before the first push:

```bash
git filter-branch -f --msg-filter 'sed "/^Claude-Session:/d"' -- --all
git update-ref -d refs/original/refs/heads/main
git reflog expire --expire=now --all && git gc --prune=now
```

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
- Everything the exporter writes is UTF-8, so `export.Write` stamps leader/09
  as 'a'. gomarc's `AsMARC` does this for binary output already; MARCXML and
  MARC-in-JSON needed it doing explicitly.
- Test fixtures are committed in both encodings; regenerate `sample.mrc` from
  `sample.marcxml` with `yaz-marcdump`, never by hand.

## Reading files too big to hold

The harvests this reads run to gigabytes - 3.7 GB and 1.4 million records is a
real one - so nothing loads a whole file any more. Four rules follow from that,
and breaking any of them is what makes the tool feel broken:

- **Nothing holds every record.** `show`, `export` and `validate` walk with
  `marcio.StreamReader` / `StreamFiles` and keep one batch at a time;
  `export.Writer` serialises as it goes. `Load` and `LoadFiles` still exist for
  small inputs and tests. A caller that has seen enough returns `marcio.ErrStop`,
  which is what makes `show -n 1` instant on a file of a million records.
- **The browser keeps rows, not records.** `internal/tui/store.go`: a title, a
  year, a search key, the field tags, and a `marcio.Extent`. The record is
  fetched back from the file when the cursor lands on it. A decoded record is
  several kilobytes; a row is a few hundred bytes. Adding anything to the model
  that holds records undoes this.
- **The list is appended to, never rebuilt, as records load.** Rebuilding is
  O(n) and a load is thousands of batches, so rebuilding per batch is
  quadratic. `appendItems` is for loading, `rebuildItems` for a changed filter.
- **MARCXML is cut into blocks and decoded on every core.** `xmlblocks.go`
  finds record boundaries by scanning - the literal text `</record>` can sit
  inside a comment or a CDATA section, where `bytes.LastIndex` would tear a
  record in half - and `xmlparallel.go` gives each block a decoder.

That last one also carries the only way past damaged MARCXML: an
`encoding/xml` decoder returns the same syntax error for ever once it has seen
one, so a walk that "records the issue and continues" spins for ever. A block
that fails is read again one record at a time, each with a fresh decoder, which
costs the damaged record and nothing else and keeps the ordinals true.

## Following a file

`-follow` watches the file's *directory*, not the file: a writer that replaces
a file by renaming over it leaves a watch on the old inode pointing at nothing.
Reads are tagged with what triggered them, because the safety tick runs its own
timer - arming a watch waiter from it as well would leave a reader queued on
the channel after every tick.

## Style

Comments explain why, not what. Match the density of the surrounding file:
package docs and non-obvious decisions are commented, mechanical code is not.
Keep `gofmt` clean — CI fails on unformatted files.
