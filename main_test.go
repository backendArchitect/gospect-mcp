package main

import (
	"context"
	"flag"
	"os"
	"reflect"
	"strings"
	"testing"
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
