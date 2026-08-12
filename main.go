// Command lunette browses and converts MARC21 files: a dual-pane terminal
// browser by default, plus headless export and validate subcommands.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"

	marc "github.com/beyto1974/gomarc"
	"github.com/charmbracelet/x/term"

	"github.com/beyto1974/lunette/internal/export"
	"github.com/beyto1974/lunette/internal/marcio"
	"github.com/beyto1974/lunette/internal/render"
	"github.com/beyto1974/lunette/internal/tui"
)

// Stamped at build time with -ldflags; see .goreleaser.yaml. A build straight
// from source says "dev", which is honest.
var (
	version = "dev"
	commit  = ""
	date    = ""
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "lunette: "+err.Error())
		os.Exit(1)
	}
}

// run dispatches a command line. Output goes to the given writers so that the
// subcommands are testable without touching the process streams.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		usage(stderr)
		return fmt.Errorf("no file given")
	}

	switch args[0] {
	case "export":
		return runExport(args[1:], stdin, stdout, stderr)
	case "validate":
		return runValidate(args[1:], stdin, stdout)
	case "encoding":
		return runEncoding(args[1:], stdin, stdout)
	case "show":
		return runShow(args[1:], stdin, stdout)
	case "version", "-version", "--version":
		fmt.Fprintln(stdout, versionLine())
		return nil
	case "help", "-h", "--help":
		usage(stdout)
		return nil
	default:
		return runView(args)
	}
}

// versionLine names the build precisely enough to reproduce it.
func versionLine() string {
	line := "lunette " + version
	if commit != "" {
		line += " (" + commit + ")"
	}
	if date != "" {
		line += " built " + date
	}
	return line + ", " + runtime.Version() + " " + runtime.GOOS + "/" + runtime.GOARCH
}

func usage(w io.Writer) {
	fmt.Fprint(w, `lunette - browse and convert MARC21 files

Usage:
  lunette [-follow] <file>...           open the dual-pane browser
  lunette show [flags] <file>...        print records to stdout
  lunette export [flags] <file>...      convert records to another format
  lunette validate <file>...            report records that fail to decode
  lunette encoding <file>               report what encoding the file really uses

Input may be binary MARC21 (.mrc) or MARCXML; the format is detected from the
first bytes. Several files are read as one set. A file of "-" means standard
input, for every subcommand except the browser, which needs a file it can seek.

browser flags:
  -follow                                keep reading a binary MARC21 file as a
                                         harvest appends records to it

show flags:
  -mode annotated|compact|raw|json|xml   rendering (default annotated)
  -n N                                   stop after N records (0 = all)
  -filter Q                              keep records matching Q
  -scope titles|record|both              where -filter looks (default titles)
  -all                                   shorthand for -scope both
  -tag NNN                               keep records carrying field NNN
  -width N                               wrap long fields at N columns
  -indent                                pretty-print the json and xml modes
  -color / -no-color                     force colour on or off (default: on
                                         when stdout is a terminal, and off
                                         when NO_COLOR is set)

export flags:
  -format mrc|xml|json                   output encoding (required)
  -o FILE                                output file (default stdout)
  -filter Q                              keep records matching Q
  -scope titles|record|both              where -filter looks (default titles)
  -all                                   shorthand for -scope both
  -tag NNN                               keep records carrying field NNN
  -force                                 overwrite the output file if it exists
`)
}

func runView(args []string) error {
	fs := flag.NewFlagSet("lunette", flag.ContinueOnError)
	follow := fs.Bool("follow", false, "keep reading the file as records are appended")
	if err := fs.Parse(args); err != nil {
		return err
	}
	paths, err := inputs(fs)
	if err != nil {
		return err
	}
	if *follow && len(paths) > 1 {
		return fmt.Errorf("-follow reads one growing file, got %d", len(paths))
	}
	if slices.Contains(paths, stdinName) {
		// The browser re-reads the file as the cursor moves, and -follow seeks
		// back into it; neither works on a pipe.
		return fmt.Errorf("cannot browse standard input: give a file, or use `lunette show -`")
	}

	var opts []tui.Option
	if *follow {
		opts = append(opts, tui.WithFollow())
	}
	return tui.Run(paths, opts...)
}

// stdinName is the conventional argument for "read standard input", so that
// lunette can sit in a shell pipeline rather than requiring a temporary file.
const stdinName = "-"

// describe names the sources for a report: a path, the pipe, or the first of
// several files and a count.
func describe(paths []string) string {
	if len(paths) == 1 && paths[0] == stdinName {
		return "standard input"
	}
	return marcio.Describe(paths)
}

// load reads records from the given paths, or from stdin when the only path
// is "-".
func load(paths []string, stdin io.Reader) (*marcio.Result, error) {
	if len(paths) == 1 && paths[0] == stdinName {
		return marcio.Load(stdin)
	}
	if slices.Contains(paths, stdinName) {
		return nil, fmt.Errorf("standard input cannot be mixed with files")
	}
	return marcio.LoadFiles(paths)
}

// selection is what show and export both need: the records from the given
// inputs, narrowed by the shared -filter, -tag and -scope flags. Keeping it in
// one place stops the two commands drifting apart, which they had already
// started to do.
type selection struct {
	query, tag, scope string
	all               bool
}

func (sel selection) apply(paths []string, stdin io.Reader) ([]*marc.Record, *marcio.Result, error) {
	res, err := load(paths, stdin)
	if err != nil {
		return nil, nil, err
	}
	sc, err := searchScope(sel.scope, sel.all)
	if err != nil {
		return nil, nil, err
	}
	criteria := export.Criteria{Query: sel.query, Tag: sel.tag, Scope: sc}
	return export.Filter(res.Records, criteria), res, nil
}

// inputs returns the file arguments, insisting on at least one.
func inputs(fs *flag.FlagSet) ([]string, error) {
	if fs.NArg() == 0 {
		return nil, fmt.Errorf("want at least one file")
	}
	return fs.Args(), nil
}

// useColour decides whether to emit ANSI, highest precedence first: an
// explicit -no-color, an explicit -color, the NO_COLOR convention
// (https://no-color.org), and finally whether output is going to a terminal at
// all. Piping into a file should not fill it with escapes; piping into less -R
// should not need a flag.
func useColour(force, off, terminal bool) bool {
	switch {
	case off:
		return false
	case force:
		return true
	case os.Getenv("NO_COLOR") != "":
		return false
	default:
		return terminal
	}
}

// isTerminal reports whether w is a terminal. Anything that is not an *os.File
// - a buffer in a test, a pipe - is not.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(f.Fd())
}

// searchScope resolves the -scope flag, with -all as a shorthand kept for the
// flag that predates the three-way choice.
func searchScope(name string, all bool) (marcio.Scope, error) {
	if all {
		return marcio.ScopeBoth, nil
	}
	return marcio.ParseScope(name)
}

func runShow(args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("show", flag.ContinueOnError)
	fs.SetOutput(stdout)
	mode := fs.String("mode", "annotated", "annotated, raw, json or xml")
	limit := fs.Int("n", 0, "stop after N records (0 = all)")
	query := fs.String("filter", "", "keep records matching this text")
	tag := fs.String("tag", "", "keep records carrying this field")
	scope := fs.String("scope", "titles", "where -filter looks: titles, record or both")
	all := fs.Bool("all", false, "shorthand for -scope both")
	width := fs.Int("width", 0, "wrap long fields at this many columns (0 = no wrapping)")
	indent := fs.Bool("indent", false, "pretty-print the json and xml modes")
	color := fs.Bool("color", false, "force ANSI colour on")
	noColour := fs.Bool("no-color", false, "force ANSI colour off")
	if err := fs.Parse(args); err != nil {
		return err
	}
	paths, err := inputs(fs)
	if err != nil {
		return err
	}

	m, err := render.ParseMode(*mode)
	if err != nil {
		return err
	}
	recs, _, err := selection{query: *query, tag: *tag, scope: *scope, all: *all}.apply(paths, stdin)
	if err != nil {
		return err
	}
	if *limit > 0 && *limit < len(recs) {
		recs = recs[:*limit]
	}

	colour := useColour(*color, *noColour, isTerminal(stdout))

	out := bufio.NewWriter(stdout)
	defer out.Flush()
	for i, rec := range recs {
		s, err := render.Render(rec, m, render.Options{Color: colour, Width: *width, Indent: *indent})
		if err != nil {
			return err
		}
		if i > 0 {
			fmt.Fprintln(out)
		}
		fmt.Fprintln(out, s)
	}
	return nil
}

func runExport(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	fs.SetOutput(stderr)
	format := fs.String("format", "", "mrc, xml or json")
	outPath := fs.String("o", "", "output file (default stdout)")
	query := fs.String("filter", "", "keep records matching this text")
	tag := fs.String("tag", "", "keep records carrying this field")
	scope := fs.String("scope", "titles", "where -filter looks: titles, record or both")
	all := fs.Bool("all", false, "shorthand for -scope both")
	force := fs.Bool("force", false, "overwrite the output file if it exists")
	if err := fs.Parse(args); err != nil {
		return err
	}
	paths, err := inputs(fs)
	if err != nil {
		return err
	}
	for _, path := range paths {
		if err := checkOutput(*outPath, path, *force); err != nil {
			return err
		}
	}
	f, err := export.ParseFormat(*format)
	if err != nil {
		return err
	}

	recs, res, err := selection{query: *query, tag: *tag, scope: *scope, all: *all}.apply(paths, stdin)
	if err != nil {
		return err
	}

	w := stdout
	if *outPath != "" {
		file, err := os.Create(*outPath)
		if err != nil {
			return err
		}
		defer file.Close()
		w = file
	}
	bw := bufio.NewWriter(w)
	if err := export.Write(bw, recs, f); err != nil {
		return err
	}
	if err := bw.Flush(); err != nil {
		return err
	}

	// Skipped records would silently shrink the output, so say so.
	if n := len(res.Issues); n > 0 {
		fmt.Fprintf(stderr, "lunette: %d record(s) could not be decoded and were not exported\n", n)
	}
	return nil
}

// checkOutput refuses an export that would destroy something. Writing over the
// input truncates the file mid-read and loses both copies; writing over any
// other existing file is at least worth asking about, so it needs -force.
func checkOutput(outPath, inputPath string, force bool) error {
	if outPath == "" {
		return nil // stdout
	}

	out, err := filepath.Abs(outPath)
	if err != nil {
		return err
	}
	in, err := filepath.Abs(inputPath)
	if err != nil {
		return err
	}
	if out == in {
		return fmt.Errorf("refusing to write over the input file %s", inputPath)
	}

	if _, err := os.Stat(out); err == nil {
		if !force {
			return fmt.Errorf("%s exists; pass -force to overwrite it", outPath)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// runEncoding reports what a file's bytes say about its encoding, and whether
// its leaders agree. It exits non-zero on a conflict so a harvest script can
// catch a repository that mislabels its records.
func runEncoding(args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("encoding", flag.ContinueOnError)
	fs.SetOutput(stdout)
	if err := fs.Parse(args); err != nil {
		return err
	}
	paths, err := inputs(fs)
	if err != nil {
		return err
	}
	if len(paths) != 1 {
		// The report is about one file's bytes; several would have to be
		// summed, and the leader distribution would stop meaning anything.
		return fmt.Errorf("encoding reads one file at a time, got %d", len(paths))
	}

	rep, err := analyzeEncoding(paths[0], stdin)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "%s\n%s", describe(paths), rep)

	if rep.Conflict() {
		return fmt.Errorf("%d record(s) declare MARC-8 but hold UTF-8", rep.MismatchedTotal)
	}
	return nil
}

// analyzeEncoding reports on a path, or on stdin when the path is "-".
func analyzeEncoding(path string, stdin io.Reader) (*marcio.EncodingReport, error) {
	if path == stdinName {
		return marcio.AnalyzeEncoding(stdin)
	}
	return marcio.AnalyzeEncodingFile(path)
}

func runValidate(args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(stdout)
	if err := fs.Parse(args); err != nil {
		return err
	}
	paths, err := inputs(fs)
	if err != nil {
		return err
	}

	res, err := load(paths, stdin)
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "%s: %s, %d record(s) decoded, %d failed\n",
		describe(paths), marcio.DescribeFormat(res), len(res.Records), len(res.Issues))
	if res.ForcedUTF8 {
		fmt.Fprintln(stdout, "  note: records hold UTF-8 bytes but leader/09 claims MARC-8; decoded as UTF-8")
	}
	for _, issue := range res.Issues {
		fmt.Fprintln(stdout, "  "+issue.String())
	}
	if len(res.Issues) > 0 {
		return fmt.Errorf("%d record(s) failed to decode", len(res.Issues))
	}
	return nil
}
