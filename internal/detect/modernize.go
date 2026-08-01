package detect

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/mod/modfile"
)

// recommendedGoMinor is the oldest Go minor version we consider "current enough". A go.mod
// declaring an older version is flagged as a (low-severity) modernization opportunity.
const recommendedGoMinor = 21

// RunModernize inspects the module's go.mod for an outdated go directive. It is deliberately
// deterministic and dependency-light; deeper modernization (deprecated APIs, unadopted features)
// comes later. A missing/unparseable go.mod is not an error — we just have nothing to report.
func RunModernize(dir string) ([]Finding, error) {
	gomodPath := filepath.Join(dir, "go.mod")
	data, err := os.ReadFile(gomodPath)
	if err != nil {
		return nil, nil
	}
	f, err := modfile.Parse(gomodPath, data, nil)
	if err != nil || f.Go == nil {
		return nil, nil
	}

	major, minor := parseGoVersion(f.Go.Version)
	if major > 1 || (major == 1 && minor >= recommendedGoMinor) {
		return nil, nil
	}

	line := 0
	if f.Go.Syntax != nil {
		line = f.Go.Syntax.Start.Line
	}
	return []Finding{{
		Category:   "modernize",
		Detector:   "go-version",
		Severity:   "low",
		Confidence: "high",
		File:       gomodPath,
		Line:       line,
		Message: fmt.Sprintf("go.mod declares Go %s (older than recommended 1.%d); consider updating the go directive and adopting newer language features",
			f.Go.Version, recommendedGoMinor),
	}}, nil
}

func parseGoVersion(v string) (major, minor int) {
	parts := strings.Split(v, ".")
	if len(parts) >= 1 {
		major, _ = strconv.Atoi(parts[0])
	}
	if len(parts) >= 2 {
		minor, _ = strconv.Atoi(parts[1])
	}
	return major, minor
}
