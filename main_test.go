package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/beyto1974/lunette/internal/marcio"
)

const sample = "testdata/sample.mrc"

// exec runs a command line and returns what it wrote.
func exec(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	return execStdin(t, nil, args...)
}

// execStdin runs a command line with something on standard input.
func execStdin(t *testing.T, stdin io.Reader, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	var out, errb bytes.Buffer
	err = run(args, stdin, &out, &errb)
	return out.String(), errb.String(), err
}

// piped is the fixture as it would arrive through a pipe.
func piped(t *testing.T) io.Reader {
	t.Helper()
	data, err := os.ReadFile(sample)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	return bytes.NewReader(data)
}

// A dash means standard input, so lunette can join a shell pipeline instead of
// demanding a temporary file.
func TestReadsStdin(t *testing.T) {
	out, _, err := execStdin(t, piped(t), "validate", "-")
	if err != nil {
		t.Fatalf("validate -: %v", err)
	}
	if !strings.Contains(out, "3 record(s) decoded") {
		t.Errorf("validate did not read the pipe: %q", out)
	}
	if !strings.Contains(out, "standard input") {
		t.Errorf("report should name the source: %q", out)
	}

	out, _, err = execStdin(t, piped(t), "show", "-mode", "raw", "-")
	if err != nil {
		t.Fatalf("show -: %v", err)
	}
	if n := strings.Count(out, "=LDR"); n != 3 {
		t.Errorf("show read %d records from the pipe, want 3", n)
	}

	out, _, err = execStdin(t, piped(t), "encoding", "-")
	if err != nil {
		t.Fatalf("encoding -: %v", err)
	}
	if !strings.Contains(out, "records:") {
		t.Errorf("encoding did not read the pipe: %q", out)
	}

	out, _, err = execStdin(t, piped(t), "export", "-format", "json", "-")
	if err != nil {
		t.Fatalf("export -: %v", err)
	}
	var v []any
	if err := json.Unmarshal([]byte(out), &v); err != nil || len(v) != 3 {
		t.Errorf("export from a pipe gave %d records: %v", len(v), err)
	}
}

// The browser needs a file it can re-open and seek, so a pipe has to be
// refused rather than left hanging.
func TestBrowserRefusesStdin(t *testing.T) {
	_, _, err := execStdin(t, piped(t), "-")
	if err == nil {
		t.Fatal("the browser accepted a pipe")
	}
	if !strings.Contains(err.Error(), "cannot browse") {
		t.Errorf("error = %q, want it to explain why", err)
	}
}

func TestValidateClean(t *testing.T) {
	out, _, err := exec(t, "validate", sample)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !strings.Contains(out, "3 record(s) decoded, 0 failed") {
		t.Errorf("unexpected report: %q", out)
	}
	if !strings.Contains(out, "MARC21") {
		t.Errorf("report does not name the format: %q", out)
	}
}

// validate exits non-zero on damaged input so it can gate a harvest.
func TestValidateReportsFailures(t *testing.T) {
	good, err := os.ReadFile(sample)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	corrupt := append([]byte(nil), good...)
	copy(corrupt[297:302], "XXXXX")

	path := filepath.Join(t.TempDir(), "corrupt.mrc")
	if err := os.WriteFile(path, corrupt, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	out, _, err := exec(t, "validate", path)
	if err == nil {
		t.Error("validate returned nil error for a damaged file")
	}
	if !strings.Contains(out, "failed") {
		t.Errorf("report does not mention the failure: %q", out)
	}
}

func TestShowModes(t *testing.T) {
	tests := map[string]string{
		"annotated": "Title Statement",
		"compact":   "245 10 $a Identification",
		"raw":       "=245",
		"json":      "\"leader\"",
		"xml":       "<record",
	}
	for mode, want := range tests {
		t.Run(mode, func(t *testing.T) {
			out, _, err := exec(t, "show", "-mode", mode, "-n", "1", sample)
			if err != nil {
				t.Fatalf("show: %v", err)
			}
			if !strings.Contains(out, want) {
				t.Errorf("mode %s output missing %q:\n%s", mode, want, out)
			}
		})
	}
}

func TestShowLimitAndFilter(t *testing.T) {
	out, _, err := exec(t, "show", "-mode", "raw", "-filter", "vandaele", sample)
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	if n := strings.Count(out, "=LDR"); n != 1 {
		t.Errorf("filter returned %d records, want 1", n)
	}

	out, _, err = exec(t, "show", "-mode", "raw", "-n", "2", sample)
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	if n := strings.Count(out, "=LDR"); n != 2 {
		t.Errorf("-n 2 returned %d records, want 2", n)
	}
}

// -all widens the filter from the list key to every subfield.
func TestShowFullTextFilter(t *testing.T) {
	out, _, err := exec(t, "show", "-mode", "raw", "-filter", "privacy", sample)
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	if n := strings.Count(out, "=LDR"); n != 0 {
		t.Errorf("key filter matched %d records for a 650 subject, want 0", n)
	}

	for _, flag := range [][]string{{"-all"}, {"-scope", "record"}, {"-scope", "both"}} {
		args := append([]string{"show", "-mode", "raw"}, flag...)
		args = append(args, "-filter", "privacy", sample)
		out, _, err = exec(t, args...)
		if err != nil {
			t.Fatalf("show %v: %v", flag, err)
		}
		if n := strings.Count(out, "=LDR"); n != 1 {
			t.Errorf("show %v matched %d records, want 1", flag, n)
		}
	}

	if _, _, err := exec(t, "show", "-scope", "everything", sample); err == nil {
		t.Error("show accepted an unknown scope")
	}
}

func TestExportFullTextFilter(t *testing.T) {
	out, _, err := exec(t, "export", "-format", "json", "-scope", "record", "-filter", "privacy", sample)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	var v []any
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("export did not produce JSON: %v", err)
	}
	if len(v) != 1 {
		t.Errorf("exported %d records, want 1", len(v))
	}
}

func TestShowWidth(t *testing.T) {
	out, _, err := exec(t, "show", "-mode", "compact", "-width", "40", "-n", "1", sample)
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	for i, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if len(line) > 40 {
			t.Errorf("line %d is %d columns wide, want at most 40: %q", i, len(line), line)
		}
	}
}

func TestShowIndent(t *testing.T) {
	flat, _, err := exec(t, "show", "-mode", "json", "-n", "1", sample)
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	pretty, _, err := exec(t, "show", "-mode", "json", "-indent", "-n", "1", sample)
	if err != nil {
		t.Fatalf("show -indent: %v", err)
	}
	if strings.Count(pretty, "\n") <= strings.Count(flat, "\n") {
		t.Error("-indent did not add line breaks")
	}
	var v any
	if err := json.Unmarshal([]byte(pretty), &v); err != nil {
		t.Fatalf("indented output is not valid JSON: %v", err)
	}
}

// Colour follows the usual conventions rather than needing a flag every time.
func TestColourPrecedence(t *testing.T) {
	// Not a terminal in a test, so the automatic answer is "no colour".
	out, _, err := exec(t, "show", "-n", "1", sample)
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	if strings.Contains(out, "\x1b[") {
		t.Error("piped output is coloured")
	}

	out, _, _ = exec(t, "show", "-n", "1", "-color", sample)
	if !strings.Contains(out, "\x1b[") {
		t.Error("-color did not force colour on")
	}

	// -no-color beats -color.
	out, _, _ = exec(t, "show", "-n", "1", "-color", "-no-color", sample)
	if strings.Contains(out, "\x1b[") {
		t.Error("-no-color did not beat -color")
	}

	// NO_COLOR beats the automatic choice but not an explicit -color.
	t.Setenv("NO_COLOR", "1")
	out, _, _ = exec(t, "show", "-n", "1", "-color", sample)
	if !strings.Contains(out, "\x1b[") {
		t.Error("-color should beat NO_COLOR: it was asked for explicitly")
	}
}

func TestUseColour(t *testing.T) {
	tests := []struct {
		name              string
		force, off, noEnv bool
		terminal          bool
		want              bool
	}{
		{name: "piped, nothing set", want: false},
		{name: "terminal", terminal: true, want: true},
		{name: "terminal with NO_COLOR", terminal: true, noEnv: true, want: false},
		{name: "piped with -color", force: true, want: true},
		{name: "terminal with -no-color", terminal: true, off: true, want: false},
		{name: "-no-color beats -color", force: true, off: true, want: false},
		{name: "-color beats NO_COLOR", force: true, noEnv: true, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.noEnv {
				t.Setenv("NO_COLOR", "1")
			} else {
				t.Setenv("NO_COLOR", "")
			}
			if got := useColour(tt.force, tt.off, tt.terminal); got != tt.want {
				t.Errorf("useColour(force=%v, off=%v, terminal=%v) = %v, want %v",
					tt.force, tt.off, tt.terminal, got, tt.want)
			}
		})
	}
}

func TestShowRejectsUnknownMode(t *testing.T) {
	if _, _, err := exec(t, "show", "-mode", "yaml", sample); err == nil {
		t.Error("show accepted an unknown mode")
	}
}

func TestExportToStdout(t *testing.T) {
	out, _, err := exec(t, "export", "-format", "json", sample)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	var v []any
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("export did not produce JSON: %v", err)
	}
	if len(v) != 3 {
		t.Errorf("exported %d records, want 3", len(v))
	}
}

func TestExportToFileWithTagFilter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.mrc")
	if _, _, err := exec(t, "export", "-format", "mrc", "-tag", "856", "-o", path, sample); err != nil {
		t.Fatalf("export: %v", err)
	}

	res, err := marcio.LoadFile(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(res.Records) != 1 {
		t.Fatalf("exported %d records, want the 1 carrying an 856", len(res.Records))
	}
	if got := marcio.Title(res.Records[0]); !strings.Contains(got, "Identification") {
		t.Errorf("exported the wrong record: %q", got)
	}
}

// -o must not quietly destroy something, least of all the file being read.
func TestExportRefusesToOverwriteTheInput(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "records.mrc")
	data, err := os.ReadFile(sample)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if err := os.WriteFile(input, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, _, err = exec(t, "export", "-format", "mrc", "-o", input, input)
	if err == nil {
		t.Fatal("export overwrote its own input")
	}
	if !strings.Contains(err.Error(), "input") {
		t.Errorf("error = %q, want it to explain the clash", err)
	}
	if after, _ := os.ReadFile(input); len(after) != len(data) {
		t.Error("the input file was modified")
	}

	// The same file reached by a different path is still the same file.
	if _, _, err := exec(t, "export", "-format", "mrc", "-o",
		filepath.Join(dir, ".", "records.mrc"), input); err == nil {
		t.Error("a path spelled differently slipped through")
	}
}

func TestExportRefusesAnExistingFile(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.mrc")
	if err := os.WriteFile(out, []byte("precious"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, _, err := exec(t, "export", "-format", "mrc", "-o", out, sample)
	if err == nil {
		t.Fatal("export overwrote an existing file")
	}
	if !strings.Contains(err.Error(), "-force") {
		t.Errorf("error = %q, want it to name the flag that allows this", err)
	}
	if got, _ := os.ReadFile(out); string(got) != "precious" {
		t.Error("the existing file was overwritten anyway")
	}

	// -force is the way through.
	if _, _, err := exec(t, "export", "-format", "mrc", "-force", "-o", out, sample); err != nil {
		t.Fatalf("export -force: %v", err)
	}
	res, err := marcio.LoadFile(out)
	if err != nil || len(res.Records) != 3 {
		t.Errorf("-force did not write the records: %v", err)
	}
}

func TestExportRequiresFormat(t *testing.T) {
	if _, _, err := exec(t, "export", sample); err == nil {
		t.Error("export ran without -format")
	}
}

func TestEncodingReport(t *testing.T) {
	out, _, err := exec(t, "encoding", sample)
	if err != nil {
		t.Fatalf("encoding: %v", err)
	}
	for _, want := range []string{"MARC21", "records:", "leader/09", "non-ascii", "consistent"} {
		if !strings.Contains(out, want) {
			t.Errorf("report does not mention %q:\n%s", want, out)
		}
	}
}

// A file whose bytes contradict its leaders is what the report exists for, and
// it exits non-zero so a harvest script can act on it.
func TestEncodingReportFlagsConflict(t *testing.T) {
	data, err := os.ReadFile(sample)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	for off := 0; off+24 <= len(data); {
		length := 0
		for i := 0; i < 5; i++ {
			length = length*10 + int(data[off+i]-'0')
		}
		if length <= 0 {
			break
		}
		data[off+9] = ' '
		off += length
	}
	path := filepath.Join(t.TempDir(), "mislabelled.mrc")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	out, _, err := exec(t, "encoding", path)
	if err == nil {
		t.Error("encoding returned nil error for a mislabelled file")
	}
	if !strings.Contains(out, "mislabelled records") {
		t.Errorf("report does not list the mislabelled records:\n%s", out)
	}
	if !strings.Contains(out, "UTF-8 bytes behind a MARC-8 leader") {
		t.Errorf("report does not explain the conflict:\n%s", out)
	}
	// The error must quote the real count, not the example cap.
	if !strings.Contains(err.Error(), "1 record(s)") {
		t.Errorf("error message = %q, want the true count", err.Error())
	}
}

func TestArgumentErrors(t *testing.T) {
	if _, _, err := exec(t); err == nil {
		t.Error("no arguments should be an error")
	}
	if _, _, err := exec(t, "validate"); err == nil {
		t.Error("validate without a file should be an error")
	}
	if _, _, err := exec(t, "validate", "a.mrc", "b.mrc"); err == nil {
		t.Error("validate with two files should be an error")
	}
	if _, _, err := exec(t, "validate", "does-not-exist.mrc"); err == nil {
		t.Error("a missing file should be an error")
	}
	if _, _, err := exec(t, "encoding"); err == nil {
		t.Error("encoding without a file should be an error")
	}
	// An unknown first argument is treated as a file to open in the browser.
	if _, _, err := exec(t, "does-not-exist.mrc"); err == nil {
		t.Error("opening a missing file should be an error")
	}
}

// A released binary has to be able to say what it is: a bug report that names
// a version is worth several that do not.
func TestVersion(t *testing.T) {
	out, _, err := exec(t, "version")
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if !strings.HasPrefix(out, "lunette ") {
		t.Errorf("version output = %q, want it to start with the program name", out)
	}
	// Unstamped builds say "dev"; a release stamps the tag in.
	if !strings.Contains(out, version) {
		t.Errorf("version output = %q, want it to contain %q", out, version)
	}
	if !strings.Contains(out, runtime.Version()) {
		t.Errorf("version output = %q, want the Go version too", out)
	}
}

// The stamp is set with -ldflags at build time, so the variables have to be
// package-level strings in main.
func TestVersionDefaults(t *testing.T) {
	if version == "" {
		t.Error("version is empty; -ldflags has nothing to overwrite")
	}
}

func TestHelp(t *testing.T) {
	out, _, err := exec(t, "help")
	if err != nil {
		t.Fatalf("help: %v", err)
	}
	for _, want := range []string{"lunette", "show", "export", "validate"} {
		if !strings.Contains(out, want) {
			t.Errorf("help does not mention %q", want)
		}
	}
}
