# Security

## Reporting

Report a vulnerability through
[private security advisories](https://github.com/beyto1974/lunette/security/advisories/new)
rather than a public issue. I will confirm receipt within a week.

## What this program is exposed to

lunette reads MARC files, which usually come from someone else's repository, so
record content is treated as untrusted input:

- **Control characters** in a record are rewritten as caret notation before
  anything is displayed or copied. Written to a terminal unfiltered they can
  rewrite the screen, retitle the window, or copy attacker-chosen text into the
  clipboard through OSC 52. `testdata/escapes.mrc` is a record that tries all
  three, and the tests use it.
- **Links** reach the desktop only if they are http or https, carry no control
  characters and are of sane length.
- **Malformed records** are reported rather than trusted, and a record that
  would crash the parser is contained rather than allowed to end the run.
- **Reads are bounded**, so a very large or endlessly growing file cannot
  decide how much memory to take.
- **`export -o`** refuses to overwrite its input, and any existing file unless
  `-force` is given.

It opens no network connections, reads no configuration, and writes only where
it is told to.

## Checks that run

- `govulncheck` on every push, and with `make vuln` — it reports advisories
  this code can actually reach, not everything in the module graph.
- CodeQL, weekly and on every push.
- Dependabot for dependency and action updates.
- The test suite under the race detector.

## Scope

Bugs that let record content act on the terminal, escape the file being read,
or crash the program in a way callers cannot contain are in scope. A repository
serving records that decode into nonsense is not: reporting bad data clearly is
what `lunette validate` and `lunette encoding` are for.
