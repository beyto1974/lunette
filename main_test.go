package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/beyto1974/marcview/internal/marcio"
)

const sample = "testdata/sample.mrc"

// exec runs a command line and returns what it wrote.
func exec(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errb bytes.Buffer
	err = run(args, &out, &errb)
	return out.String(), errb.String(), err
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
	out, _, err := exec(t, "show", "-mode", "raw", "-filter", "kloza", sample)
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

func TestHelp(t *testing.T) {
	out, _, err := exec(t, "help")
	if err != nil {
		t.Fatalf("help: %v", err)
	}
	for _, want := range []string{"marcview", "show", "export", "validate"} {
		if !strings.Contains(out, want) {
			t.Errorf("help does not mention %q", want)
		}
	}
}
