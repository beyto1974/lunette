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

### 1.2 Repository-wide statistics view — S

A `stats` subcommand and an in-app panel: field frequency (how many records
carry 856, 100, 650), average field count, records per year, encoding-level
distribution, language codes. Cataloguers ask "what does this harvest actually
contain?" far more often than they ask about a single record, and it is a
by-product of a load we already perform.

### 1.3 MARC validation rules — M

`validate` currently only reports records that fail to *decode*. Records that
decode fine can still be wrong: missing 245, no 008, indicators outside the
allowed set, subfields not permitted for the tag, malformed dates in 008/07-10.
A small rule table over the fields already parsed would catch these, with
`--strict` for a non-zero exit. This is the difference between "the bytes
parsed" and "the record is usable".

### 1.4 Diff two records or two files — M

Compare the same control number across two harvests, or two records in one
file, field by field. Anyone re-harvesting a set periodically needs to see what
changed; today that means exporting to XML twice and running `diff`, which
reports byte noise rather than field changes.

---

## 2. Nice to have

### 2.1 Saved views and bookmarks — S

Mark records while browsing, then export only the marked set. Pairs naturally
with the existing `export --filter`.

### 2.2 Filter history and better query syntax — S

`year:2020`, `has:856`, `-tag:650` (negation), and recall of previous filters
with `↑` in the prompt. The current syntax handles text plus one `tag:`.

### 2.3 Configurable colours — S

A small TOML or JSON theme file mapping the six semantic roles (tag, indicator,
subfield code, label, leader, match) to colours, plus a `--no-color` flag.
Terminal themes vary enough that the fixed ANSI-256 choices will not suit
everyone.

### 2.4 Export the current filter from the TUI — S

`e` in the browser writes the filtered set to a file using the export package
that already exists. Closes the loop between browsing and extracting.

### 2.5 Follow 856 URLs — S

`o` opens the electronic-location URL in the system browser. Trivial with
`xdg-open`/`open`, genuinely useful for a repository of links.

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
