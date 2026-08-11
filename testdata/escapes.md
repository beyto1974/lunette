# escapes.mrc

A single record whose 245 $a carries terminal control sequences:

    Harmless Title \e[2J \e]52;c;aGFja2Vk\a \e[31m

That is: clear the screen, write "hacked" to the user's clipboard through
OSC 52, then turn the text red. Its leader declares UTF-8, and ESC is valid
UTF-8, so the bytes survive decoding untouched.

MARC files come from other people's repositories, so this is input the viewer
has to expect. Everything that renders record text runs it through
`render.Sanitize` first; the tests in `internal/render/sanitize_test.go` and
`internal/tui/escape_test.go` use this file to prove it.

Rebuild it with `python3 testdata/make-escapes.py`.
