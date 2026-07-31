package scan

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// IgnoreFileName is the optional repo-level ignore file, checked into the scanned module root. It
// suppresses noise for everyone (unlike the per-line //gospect:ignore directive), e.g. generated
// code or a whole detector.
const IgnoreFileName = ".gospectignore"

// loadIgnoreFile reads <dir>/.gospectignore into FilterOptions. Format (one rule per line):
//
//	# comments and blank lines are ignored
//	*.pb.go              — drop findings whose path matches this glob/substring
//	internal/gen/        — same; a trailing-slash substring reads naturally as "this directory"
//	detector:todo        — drop all findings from this detector
//
// A missing file is not an error (returns an empty, no-op FilterOptions).
func loadIgnoreFile(dir string) FilterOptions {
	f, err := os.Open(filepath.Join(dir, IgnoreFileName))
	if err != nil {
		return FilterOptions{}
	}
	defer f.Close()

	var opt FilterOptions
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if d, ok := strings.CutPrefix(line, "detector:"); ok {
			if d = strings.TrimSpace(d); d != "" {
				opt.ExcludeDetectors = append(opt.ExcludeDetectors, d)
			}
			continue
		}
		opt.ExcludeGlob = append(opt.ExcludeGlob, line)
	}
	return opt
}
