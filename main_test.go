package main

import (
	"flag"
	"os"
	"reflect"
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
