package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/everbright/marco/marcview/internal/marcio"
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
