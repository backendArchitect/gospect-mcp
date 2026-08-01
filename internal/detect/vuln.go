package detect

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
)

// RunVuln reports known-vulnerability findings by shelling out to govulncheck (golang.org/x/vuln)
// in each module root. It is opt-in: govulncheck is slow and needs the vulnerability database
// (usually a network call), so the scan enables it only on request. If govulncheck isn't installed,
// it returns a single low-severity note rather than failing — the rest of the scan is unaffected.
//
// Only *reachable* vulnerabilities (those govulncheck traces to a called function) are reported —
// the high-signal subset — one finding per (OSV id, module).
func RunVuln(roots []string) ([]Finding, error) {
	if _, err := exec.LookPath("govulncheck"); err != nil {
		return []Finding{{
			Category: "vuln", Detector: "govulncheck", Severity: "low",
			Message: "govulncheck not found on PATH; install it (go install golang.org/x/vuln/cmd/govulncheck@latest) to enable vulnerability scanning",
		}}, nil
	}
	var findings []Finding
	seen := map[string]bool{}
	for _, root := range roots {
		fs, err := govulncheckModule(root, seen)
		if err != nil {
			return nil, err
		}
		findings = append(findings, fs...)
	}
	return findings, nil
}

// govulncheckModule runs govulncheck in one module and parses its streaming JSON. seen dedupes
// findings by "osv\x00module" across all modules. Because an OSV's definition and its findings can
// arrive in either order, findings are buffered and the summary is attached after the full stream.
func govulncheckModule(root string, seen map[string]bool) ([]Finding, error) {
	cmd := exec.Command("govulncheck", "-json", "./...")
	cmd.Dir = root
	var out, errBuf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errBuf
	// Exit status 3 means "vulnerabilities found" — expected, not an error. Anything else with no
	// output is a real failure (e.g. the module doesn't build).
	if err := cmd.Run(); err != nil && out.Len() == 0 {
		return nil, fmt.Errorf("govulncheck in %s: %v: %s", root, err, errBuf.String())
	}

	// A vuln's OSV definition and its findings can arrive in either order, so buffer the raw hits
	// and resolve summaries after the whole stream is read.
	type hit struct {
		id, module, fixed string
		f                 Finding
	}
	summaries := map[string]string{}
	var hits []hit
	dec := json.NewDecoder(&out)
	for {
		var msg govulnMessage
		if err := dec.Decode(&msg); err == io.EOF {
			break
		} else if err != nil {
			break // tolerate a truncated tail rather than dropping everything parsed so far
		}
		if msg.OSV != nil {
			summaries[msg.OSV.ID] = msg.OSV.Summary
		}
		if msg.Finding == nil || len(msg.Finding.Trace) == 0 {
			continue
		}
		t := msg.Finding.Trace[0]
		if t.Function == "" {
			continue // module-only hit, not traced to a call — lower signal, skip
		}
		key := msg.Finding.OSV + "\x00" + t.Module
		if seen[key] {
			continue
		}
		seen[key] = true
		f := Finding{Category: "vuln", Detector: "govulncheck", Severity: "high", Package: t.Package}
		if t.Position != nil {
			f.File, f.Line, f.Col = t.Position.Filename, t.Position.Line, t.Position.Column
		}
		hits = append(hits, hit{id: msg.Finding.OSV, module: t.Module, fixed: msg.Finding.FixedVersion, f: f})
	}

	findings := make([]Finding, 0, len(hits))
	for _, h := range hits {
		h.f.Message = vulnMessage(h.id, summaries[h.id], h.module, h.fixed)
		findings = append(findings, h.f)
	}
	return findings, nil
}

func vulnMessage(id, summary, module, fixed string) string {
	msg := id
	if summary != "" {
		msg += ": " + summary
	}
	msg += fmt.Sprintf(" (module %s", module)
	if fixed != "" {
		msg += ", fixed in " + fixed
	}
	return msg + ")"
}

// --- govulncheck -json message shapes (only the fields we use) ---

type govulnMessage struct {
	OSV     *govulnOSV     `json:"osv"`
	Finding *govulnFinding `json:"finding"`
}

type govulnOSV struct {
	ID      string `json:"id"`
	Summary string `json:"summary"`
}

type govulnFinding struct {
	OSV          string        `json:"osv"`
	FixedVersion string        `json:"fixed_version"`
	Trace        []govulnFrame `json:"trace"`
}

type govulnFrame struct {
	Module   string          `json:"module"`
	Package  string          `json:"package"`
	Function string          `json:"function"`
	Position *govulnPosition `json:"position"`
}

type govulnPosition struct {
	Filename string `json:"filename"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
}
