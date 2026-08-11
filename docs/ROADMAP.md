# Roadmap — suggested improvements

Feature proposals for marcview, ordered by value against effort. Nothing here is
committed work; each entry says what it buys and what it costs. Small
housekeeping items live in [../TODO.md](../TODO.md).

Effort is rough: **S** under a day, **M** a few days, **L** a week or more.

---

## 1. Worth doing next

### 1.1 Field-level cursor in the record pane — M

Today the right pane is a scrolling block of text. Making individual fields
selectable turns it into a structure you can act on: copy one field, filter the
list by "records sharing this subject", follow an 856 URL, or expand a linked
880 pair. This is the single change that most increases what the tool can do,
because every later action ("filter by this", "copy this") needs a notion of
"this".

Implementation: keep the rendered record as `[]fieldLine` with a field index per
line rather than one string, and drive the viewport from that.

### ~~1.2 Compact record view~~ — done

Shipped as the `compact` render mode, bound to `c` and available as
`show -mode compact`.

<details><summary>Original proposal</summary>



A fifth render mode, bound to `c`, that puts one field per line with its
subfields inline: `245 10 $a Title $b subtitle`. The annotated view spends
three or four lines on a field with two subfields, so a 30-field record needs
scrolling to see at all; compact fits most records on one screen, which is what
you want when comparing records rather than reading one.

It sits between raw and annotated: raw is dense but shows no labels and no
`#` for blank indicators, annotated is readable but sparse. Compact keeps the
indicator convention and the colour scheme, drops the label line and the
per-subfield line breaks. A `--compact` flag on `show` gives the same density
to piped output, which is also the form worth grepping.

Implementation: a fourth branch in `render.Render` reusing `palette`; the mode
list, key map and help entries all extend by one. Roughly 60 lines plus tests.
Worth pairing with a width-aware fold so long fields wrap under an indent
rather than being cut off.

</details>

The width-aware fold from that entry is done too: values wrap to the pane
width, continuations indent under the value, and over-long tokens are broken.

### ~~1.3 Search highlighting across both panes~~ — done

All three parts shipped: matches highlight in the list as well as the record,
`n` and `N` step between hits with a `match 3/7` counter in the pane header,
and the `all:` prefix (`-all` on the command line) searches every subfield.

<details><summary>Original proposal</summary>



Filter matches are highlighted in the record pane today, but not in the list,
so with a filter active you can see *that* a record matched without seeing
*where*. Three gaps worth closing:

1. **Highlight in the list.** The delegate already truncates titles; wrapping
   the matched span in the match style before truncation is the same work
   `palette.value` does for field values. Truncation must not cut a span
   mid-escape — measure on the plain string, style afterwards.
2. **Match navigation inside a record.** `n` and `N` to jump the viewport
   between hits, with a `3/7 matches` counter in the pane header. The viewport
   bubble already has `SetHighlights` and `HighlightNext`/`HighlightPrevious`,
   so this is mostly plumbing the match offsets in.
3. **Match beyond the search key.** Filtering matches against the precomputed
   key (control number, title, author, year), so a term appearing only in a 520
   summary or a 650 subject does not match at all. An opt-in `all:` prefix that
   searches the full field text — precomputed once per record at load — makes
   the filter honest, at the cost of a larger index.

Together these turn the filter from "which records" into "where in them", which
is what a search is for.

</details>

### 1.4 Two-file mode and record pinning — M

Open two files at once, or pin a record while browsing others, and compare them
side by side. Re-harvesting the same set is routine; answering "what changed in
this record" currently means exporting twice and diffing XML, which reports
byte noise rather than field changes. Pinning is the cheap half: keep one
record in the right pane while the cursor moves, so the comparison is manual
but immediate. See also 1.6.

### 1.5 Repository-wide statistics view — S

A `stats` subcommand and an in-app panel: field frequency (how many records
carry 856, 100, 650), average field count, records per year, encoding-level
distribution, language codes. Cataloguers ask "what does this harvest actually
contain?" far more often than they ask about a single record, and it is a
by-product of a load we already perform.

### 1.6 MARC validation rules — M

`validate` currently only reports records that fail to *decode*. Records that
decode fine can still be wrong: missing 245, no 008, indicators outside the
allowed set, subfields not permitted for the tag, malformed dates in 008/07-10.
A small rule table over the fields already parsed would catch these, with
`--strict` for a non-zero exit. This is the difference between "the bytes
parsed" and "the record is usable".

### 1.7 Diff two records or two files — M

Compare the same control number across two harvests, or two records in one
file, field by field. Anyone re-harvesting a set periodically needs to see what
changed; today that means exporting to XML twice and running `diff`, which
reports byte noise rather than field changes.

---

## 2. Nice to have

### ~~2.1 Mouse support~~ — done

Click to select a record or focus a pane, wheel to scroll whichever pane the
pointer is over. Bubble Tea v2 asks for this through `View.MouseMode`.

### ~~2.2 A golden-frame test for the layout~~ — done

Eight frames under `internal/tui/testdata/golden/`, rewritten with
`make golden`. They found two more defects on their first run: the title bar
truncating "MARC21" to "M" on a narrow terminal, and an empty result showing
the bubble's "No items." with no mention of the filter that caused it.

<details><summary>Original proposal</summary>



Three layout bugs shipped in this repository and all three were invisible to
the test suite until someone looked at a real terminal: panes two rows too
tall, list rows two cells too wide (the year wrapping onto its own line), and
the full help ellipsised off the right edge. Each was a sizing arithmetic error
that a rendered frame would have caught instantly.

A golden test that renders the whole view at a few fixed sizes and compares
against committed text files would turn every one of those into a failing diff.
It also makes deliberate layout changes visible in review, which per-property
assertions do not.

</details>

### 2.3 Saved views and bookmarks — S

Mark records while browsing, then export only the marked set. Pairs naturally
with the existing `export --filter`.

### 2.4 Filter history and better query syntax — S

`year:2020`, `has:856`, `-tag:650` (negation), and recall of previous filters
with `↑` in the prompt. The current syntax handles text plus one `tag:`.

### 2.5 Configurable colours — S

A small TOML or JSON theme file mapping the six semantic roles (tag, indicator,
subfield code, label, leader, match) to colours, plus a `--no-color` flag.
Terminal themes vary enough that the fixed ANSI-256 choices will not suit
everyone.

### 2.6 Export the current filter from the TUI — S

`e` in the browser writes the filtered set to a file using the export package
that already exists. Closes the loop between browsing and extracting.

### 2.7 Follow 856 URLs — S

`o` opens the electronic-location URL in the system browser. Trivial with
`xdg-open`/`open`, genuinely useful for a repository of links.

### ~~2.8 Encoding diagnostics~~ — done

`marcview encoding` reports leader/09 distribution, records holding non-ASCII,
real MARC-8 escape sequences, invalid UTF-8 and truncation, then names the
verdict and exits non-zero on a conflict.

<details><summary>Original proposal</summary>



The loader already decides whether a file's bytes contradict its leader. It
could say more: which records hold non-ASCII, whether any genuinely use MARC-8
escape sequences, and what the leader/09 distribution across the file is. A
`marcview encoding file.mrc` report would have turned this session's "données
became donn©♭es" from a discovery into a one-line answer, and it is the first
thing worth running on a harvest from an unfamiliar repository.

</details>

### 2.9 Fix the leader on export — S

The records this tool was built for claim MARC-8 and hold UTF-8. `export
--fix-encoding` would set leader/09 to 'a' on the way out, so downstream tools
that trust the leader — most of them — stop guessing. It is a one-byte change
per record and the loader already knows when it applies.

---

## 3. Larger bets

### 3.1 Lazy index for very large files — M

Records are held in memory. At the observed scale (5572 records, 6 MB) that is
irrelevant, but a national-library dump of several GB would not fit. The fix is
an offset index built by walking record lengths without parsing, plus an LRU
cache of decoded records. Worth doing only when someone actually hits it — the
current design is simpler and faster below the threshold.

### 3.2 Editing and write-back — L

Deliberately out of scope so far. Doing it properly means undo, dirty-state
tracking, atomic writes, validation before save, and a confirmation flow.
Doing it badly means corrupting catalogue data. If it happens, it should start
read-only-with-patch-export: edit in the UI, emit a patch file, never touch the
original.

### 3.3 Direct OAI-PMH harvesting in Go — L

`marcview harvest` would fold record fetching into this binary — resumption
tokens, retries and rate limiting handled internally — so a user needs neither
metha nor yaz to get from an endpoint to a browsable file. The shell pipeline
in the marco repo already does this; the question is whether marcview should
stand alone.

### 3.4 Authority and linked-data lookups — L

Resolve 100/650 entries against VIAF or LCSH and show the authorised form
alongside. High value for cataloguers, but it introduces network calls,
caching, and offline behaviour into a tool that is currently pure local I/O.

---

## 4. Explicitly not planned

- **A GUI.** The terminal is where these files already live.
- **Format conversion beyond MARC.** Dublin Core, MODS and friends are a
  different problem; `metha` already harvests them.
- **A general MARC library.** `gomarc` is that; this stays an application.
