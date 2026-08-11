// Command lunette browses and converts MARC21 files: a dual-pane terminal
// browser by default, plus headless export and validate subcommands.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/beyto1974/lunette/internal/export"
	"github.com/beyto1974/lunette/internal/marcio"
	"github.com/beyto1974/lunette/internal/render"
	"github.com/beyto1974/lunette/internal/tui"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "lunette: "+err.Error())
		os.Exit(1)
	}
}

// run dispatches a command line. Output goes to the given writers so that the
// subcommands are testable without touching the process streams.
func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		usage(stderr)
		return fmt.Errorf("no file given")
	}

	switch args[0] {
	case "export":
		return runExport(args[1:], stdout, stderr)
	case "validate":
		return runValidate(args[1:], stdout)
	case "encoding":
		return runEncoding(args[1:], stdout)
	case "show":
		return runShow(args[1:], stdout)
	case "help", "-h", "--help":
		usage(stdout)
		return nil
	default:
		return runView(args)
	}
}

func usage(w io.Writer) {
	fmt.Fprint(w, `lunette - browse and convert MARC21 files

Usage:
  lunette [-follow] <file>              open the dual-pane browser
  lunette show [flags] <file>           print records to stdout
  lunette export [flags] <file>         convert records to another format
  lunette validate <file>               report records that fail to decode
  lunette encoding <file>               report what encoding the file really uses

Input may be binary MARC21 (.mrc) or MARCXML; the format is detected from the
file's first bytes.

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
  -color                                 force ANSI colour when not a terminal

export flags:
  -format mrc|xml|json                   output encoding (required)
  -o FILE                                output file (default stdout)
  -filter Q                              keep records matching Q
  -scope titles|record|both              where -filter looks (default titles)
  -all                                   shorthand for -scope both
  -tag NNN                               keep records carrying field NNN
`)
}

func runView(args []string) error {
	fs := flag.NewFlagSet("lunette", flag.ContinueOnError)
	follow := fs.Bool("follow", false, "keep reading the file as records are appended")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("want exactly one file, got %d", fs.NArg())
	}

	var opts []tui.Option
	if *follow {
		opts = append(opts, tui.WithFollow())
	}
	return tui.Run(fs.Arg(0), opts...)
}

// searchScope resolves the -scope flag, with -all as a shorthand kept for the
// flag that predates the three-way choice.
func searchScope(name string, all bool) (marcio.Scope, error) {
	if all {
		return marcio.ScopeBoth, nil
	}
	return marcio.ParseScope(name)
}

func runShow(args []string, stdout io.Writer) error {
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
	color := fs.Bool("color", false, "force ANSI colour")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("want exactly one file, got %d", fs.NArg())
	}

	m, err := render.ParseMode(*mode)
	if err != nil {
		return err
	}
	res, err := marcio.LoadFile(fs.Arg(0))
	if err != nil {
		return err
	}

	sc, err := searchScope(*scope, *all)
	if err != nil {
		return err
	}
	recs := export.Filter(res.Records, export.Criteria{Query: *query, Tag: *tag, Scope: sc})
	if *limit > 0 && *limit < len(recs) {
		recs = recs[:*limit]
	}

	out := bufio.NewWriter(stdout)
	defer out.Flush()
	for i, rec := range recs {
		s, err := render.Render(rec, m, render.Options{Color: *color, Width: *width, Indent: *indent})
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

func runExport(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	fs.SetOutput(stderr)
	format := fs.String("format", "", "mrc, xml or json")
	outPath := fs.String("o", "", "output file (default stdout)")
	query := fs.String("filter", "", "keep records matching this text")
	tag := fs.String("tag", "", "keep records carrying this field")
	scope := fs.String("scope", "titles", "where -filter looks: titles, record or both")
	all := fs.Bool("all", false, "shorthand for -scope both")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("want exactly one file, got %d", fs.NArg())
	}
	f, err := export.ParseFormat(*format)
	if err != nil {
		return err
	}

	res, err := marcio.LoadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	sc, err := searchScope(*scope, *all)
	if err != nil {
		return err
	}
	recs := export.Filter(res.Records, export.Criteria{Query: *query, Tag: *tag, Scope: sc})

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

// runEncoding reports what a file's bytes say about its encoding, and whether
// its leaders agree. It exits non-zero on a conflict so a harvest script can
// catch a repository that mislabels its records.
func runEncoding(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("encoding", flag.ContinueOnError)
	fs.SetOutput(stdout)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("want exactly one file, got %d", fs.NArg())
	}

	rep, err := marcio.AnalyzeEncodingFile(fs.Arg(0))
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "%s\n%s", fs.Arg(0), rep)

	if rep.Conflict() {
		return fmt.Errorf("%d record(s) declare MARC-8 but hold UTF-8", rep.MismatchedTotal)
	}
	return nil
}

func runValidate(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(stdout)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("want exactly one file, got %d", fs.NArg())
	}

	res, err := marcio.LoadFile(fs.Arg(0))
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "%s: %s, %d record(s) decoded, %d failed\n",
		fs.Arg(0), res.Format, len(res.Records), len(res.Issues))
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
