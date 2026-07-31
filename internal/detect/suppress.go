package detect

import (
	"bufio"
	"os"
	"strings"
)

// suppressDirective is the inline marker that tells gospect a finding is intentional, mirroring
// the familiar //nolint idiom. Put it on the flagged line or the line directly above it:
//
//	risky() //gospect:ignore              — suppress any finding on this line
//	risky() //gospect:ignore nilness      — suppress only these detectors
const suppressDirective = "//gospect:ignore"

// ApplySuppressions drops findings the author marked intentional with a //gospect:ignore comment
// on (or directly above) the flagged line. It returns the surviving findings and how many were
// suppressed. Files are read once and cached; an unreadable file leaves its findings untouched.
func ApplySuppressions(findings []Finding) (kept []Finding, suppressed int) {
	cache := map[string][]string{}
	getLines := func(file string) []string {
		if lines, ok := cache[file]; ok {
			return lines
		}
		lines := readLines(file)
		cache[file] = lines
		return lines
	}

	for _, f := range findings {
		if f.File != "" && f.Line > 0 && isSuppressed(getLines(f.File), f.Line, f.Detector) {
			suppressed++
			continue
		}
		kept = append(kept, f)
	}
	return kept, suppressed
}

// isSuppressed reports whether line (1-indexed) or the line above it carries a //gospect:ignore
// directive covering detector. A bare directive covers everything; one with names covers only
// those detectors.
func isSuppressed(lines []string, line int, detector string) bool {
	for _, n := range []int{line, line - 1} {
		if n >= 1 && n <= len(lines) && directiveCovers(lines[n-1], detector) {
			return true
		}
	}
	return false
}

func directiveCovers(text, detector string) bool {
	i := strings.Index(text, suppressDirective)
	if i < 0 {
		return false
	}
	rest := strings.TrimSpace(text[i+len(suppressDirective):])
	if rest == "" {
		return true // bare directive suppresses any detector
	}
	for _, name := range strings.FieldsFunc(rest, func(r rune) bool { return r == ',' || r == ' ' }) {
		if name == detector {
			return true
		}
	}
	return false
}

func readLines(file string) []string {
	f, err := os.Open(file)
	if err != nil {
		return nil
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024) // tolerate long lines (generated code)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines
}
