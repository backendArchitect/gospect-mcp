package scan

import (
	"bufio"
	"os"
	"regexp"

	"github.com/backendArchitect/gospect-mcp/internal/detect"
)

// generatedMarker matches the standard "generated code" line Go tools emit (see
// https://pkg.go.dev/cmd/go#hdr-Generate_Go_files). Findings in such files are noise — the fix
// belongs in the generator, not the output — so they are dropped by default.
var generatedMarker = regexp.MustCompile(`^// Code generated .* DO NOT EDIT\.$`)

// dropGenerated removes findings whose file is generated code, returning the survivors and how many
// were dropped. Whether a file is generated is decided once per file and cached.
func dropGenerated(findings []detect.Finding) (kept []detect.Finding, dropped int) {
	isGen := map[string]bool{}
	check := func(file string) bool {
		if v, ok := isGen[file]; ok {
			return v
		}
		v := isGeneratedFile(file)
		isGen[file] = v
		return v
	}
	for _, f := range findings {
		if f.File != "" && check(f.File) {
			dropped++
			continue
		}
		kept = append(kept, f)
	}
	return kept, dropped
}

// isGeneratedFile reports whether file carries the generated-code marker. Per the convention the
// marker must appear before any non-comment, non-blank line (i.e. before the package clause), so we
// stop scanning once real code begins.
func isGeneratedFile(file string) bool {
	f, err := os.Open(file)
	if err != nil {
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if generatedMarker.MatchString(line) {
			return true
		}
		if trimmed := trimSpace(line); trimmed != "" && !isComment(trimmed) {
			return false // reached real code without seeing the marker
		}
	}
	return false
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

func isComment(s string) bool {
	return len(s) >= 2 && s[0] == '/' && (s[1] == '/' || s[1] == '*')
}
