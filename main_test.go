package main

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/backendArchitect/gospect-mcp/internal/scan"
)

// TestParseArgs_Interspersed verifies flags are recognized before, after, and between positionals.
func TestParseArgs_Interspersed(t *testing.T) {
	cases := []struct {
		name       string
		argv       []string
		wantFmt    string
		wantVerb   bool
		wantPosArg []string
	}{
		{"flags before", []string{"g", "scan", "-format", "text", "./dir", "./..."}, "text", false, []string{"./dir", "./..."}},
		{"flags after", []string{"g", "scan", "./dir", "-format", "text"}, "text", false, []string{"./dir"}},
		{"interspersed", []string{"g", "scan", "./dir", "-format", "text", "./...", "-verbose"}, "text", true, []string{"./dir", "./..."}},
		{"none", []string{"g", "scan", "./dir"}, "json", false, []string{"./dir"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			old := os.Args
			defer func() { os.Args = old }()
			os.Args = c.argv

			fs := flag.NewFlagSet("scan", flag.ContinueOnError)
			format := fs.String("format", "json", "")
			verbose := fs.Bool("verbose", false, "")
			got := parseArgs(fs)

			if *format != c.wantFmt || *verbose != c.wantVerb {
				t.Errorf("flags: format=%q verbose=%v; want %q %v", *format, *verbose, c.wantFmt, c.wantVerb)
			}
			if !reflect.DeepEqual(got, c.wantPosArg) {
				t.Errorf("positional=%v; want %v", got, c.wantPosArg)
			}
		})
	}
}

func TestAllowFixEnabled(t *testing.T) {
	t.Setenv("GOSPECT_ALLOW_FIX", "")
	if allowFixEnabled(false) {
		t.Fatal("should be off with no flag and no env")
	}
	if !allowFixEnabled(true) {
		t.Fatal("flag should enable it")
	}
	t.Setenv("GOSPECT_ALLOW_FIX", "1")
	if !allowFixEnabled(false) {
		t.Fatal("env should enable it")
	}
}

// fixToolJSON must reject a call that doesn't identify a finding, before touching git or the module.
func TestFixToolJSON_RequiresDetectorAndFile(t *testing.T) {
	for _, in := range []string{`{}`, `{"detector":"SA1006"}`, `{"file":"app.go"}`} {
		if _, err := fixToolJSON(context.Background(), []byte(in)); err == nil {
			t.Fatalf("expected an error for %s", in)
		}
	}
	if _, err := fixToolJSON(context.Background(), []byte(`{bad json`)); err == nil {
		t.Fatal("expected an error for malformed JSON")
	}
	// A well-formed request with no go.mod above the file fails inside the fixer, not on validation.
	if _, err := fixToolJSON(context.Background(), []byte(`{"detector":"SA1006","file":"/nonexistent/app.go"}`)); err == nil || strings.Contains(err.Error(), "required") {
		t.Fatalf("expected a fixer error (not a validation error), got %v", err)
	}
}

// scanToolJSON validates caller input at the MCP boundary before loading anything.
func TestScanToolJSON_RejectsBadArgs(t *testing.T) {
	for _, in := range []string{
		`{}`,
		`{bad json`,
		`{"path":"testdata/buggy","min_severity":"critical"}`,
		`{"path":"testdata/buggy","min_confidence":"sure"}`,
		`{"path":"testdata/buggy","since":"--output=/tmp/pwned"}`, // must never reach git as an option
	} {
		if _, err := scanToolJSON(nil, "", []byte(in)); err == nil {
			t.Fatalf("expected an error for %s", in)
		}
	}
}

// The MCP scan must honor .gospect.yml like the CLI does (explicit args still win), and apply the
// same filters and diff mode.
func TestScanToolJSON_ConfigFiltersAndSince(t *testing.T) {
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS("testdata/buggy")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gospect.yml"), []byte("pedantic: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run := func(args string) scan.Report {
		t.Helper()
		out, err := scanToolJSON(nil, "", []byte(args))
		if err != nil {
			t.Fatalf("%s: %v", args, err)
		}
		var rep scan.Report
		if err := json.Unmarshal([]byte(out), &rep); err != nil {
			t.Fatal(err)
		}
		return rep
	}
	has := func(rep scan.Report, det string) bool {
		for _, f := range rep.Findings {
			if f.Detector == det {
				return true
			}
		}
		return false
	}

	p := strconv.Quote(dir)
	if rep := run(`{"path":` + p + `}`); !has(rep, "todo") {
		t.Fatal("pedantic: true in .gospect.yml should enable the todo detector")
	}
	if rep := run(`{"path":` + p + `,"pedantic":false}`); has(rep, "todo") {
		t.Fatal("explicit pedantic:false should override .gospect.yml")
	}
	rep := run(`{"path":` + p + `,"min_severity":"high","detector":["nilness"]}`)
	if rep.FindingCount == 0 {
		t.Fatal("expected the fixture's nilness findings to survive the filter")
	}
	for _, f := range rep.Findings {
		if f.Detector != "nilness" || f.Severity != "high" {
			t.Fatalf("filter leaked %s/%s", f.Detector, f.Severity)
		}
	}

	if _, err := scanToolJSON(nil, "", []byte(`{"path":`+p+`,"since":"HEAD"}`)); err == nil {
		t.Fatal("since outside a git repo should be a tool error, not an exit")
	}
	git := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	git("add", ".")
	git("commit", "-qm", "init")
	if rep := run(`{"path":` + p + `,"since":"HEAD"}`); rep.FindingCount != 0 {
		t.Fatalf("no changes since HEAD should yield no findings, got %d", rep.FindingCount)
	}
}
