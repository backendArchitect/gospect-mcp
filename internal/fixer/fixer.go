// Package fixer drives a system AI agent to fix ONE finding, then verifies the result and rolls
// back on any regression. gospect never trusts the agent: a fix is kept only if the target finding
// is gone, no new findings appeared, and the module still builds. This is the actuator counterpart
// to the report-first sensor — opt-in, git-checkpointed, and self-verifying.
package fixer

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/backendArchitect/gospect-mcp/internal/agent"
	"github.com/backendArchitect/gospect-mcp/internal/detect"
	"github.com/backendArchitect/gospect-mcp/internal/fix"
	"github.com/backendArchitect/gospect-mcp/internal/loader"
	"github.com/backendArchitect/gospect-mcp/internal/scan"
)

// Options configures a single fix attempt.
type Options struct {
	Finding       detect.Finding
	Agent         agent.Agent
	Deterministic bool          // apply the analyzer's SuggestedFix instead of driving an agent
	RunTests      bool          // also run `go test ./...` as part of verification
	DryRun        bool          // build the prompt and stop (don't invoke the agent)
	Timeout       time.Duration // abort the agent if it runs longer than this (0 = no limit)
	Progress      func(string)  // human-readable progress (stderr)
	Output        io.Writer     // where the agent's live output goes (nil = discard)
}

// Result is the outcome of one fix attempt.
type Result struct {
	Detector    string           `json:"detector"`
	Location    string           `json:"location"`
	Applied     bool             `json:"applied"`      // a verified change was kept
	RolledBack  bool             `json:"rolled_back"`  // the agent changed code but it failed verification
	Reason      string           `json:"reason"`       // why not applied (empty on success)
	NewFindings []detect.Finding `json:"new_findings"` // regressions the fix would have introduced
	Diff        string           `json:"diff,omitempty"`
	Prompt      string           `json:"prompt,omitempty"` // populated for a dry run
}

// Fix attempts to fix one finding under a git repo, verifying and rolling back as needed.
func Fix(ctx context.Context, dir string, o Options) (*Result, error) {
	progress := o.Progress
	if progress == nil {
		progress = func(string) {}
	}
	f := o.Finding
	res := &Result{Detector: f.Detector, Location: loc(f)}

	// A dry run only renders the prompt — no git, no scanning, no edits.
	if o.DryRun {
		res.Prompt = buildPrompt(fix.Build(f))
		return res, nil
	}

	file, err := filepath.Abs(f.File)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", f.File, err)
	}
	root := moduleRoot(file)
	if root == "" {
		return nil, fmt.Errorf("no go.mod found above %s", file)
	}

	// A git repo + clean tree is required so the agent's change is exactly reversible.
	top, err := git(root, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("fix needs a git repository: %w", err)
	}
	if dirty, _ := git(top, "status", "--porcelain"); dirty != "" {
		return nil, fmt.Errorf("working tree has uncommitted changes — commit or stash first so a failed fix can be rolled back cleanly")
	}

	prompt := buildPrompt(fix.Build(f))

	// Reproduce the finding (scoped to its package) and snapshot the baseline finding set.
	scanOpts := scan.Options{DiffMode: true, ChangedFiles: []string{file}, Staticcheck: true, Untested: true}
	before, err := scan.ScanWithOptions(root, scanOpts)
	if err != nil {
		return nil, fmt.Errorf("baseline scan failed: %w", err)
	}
	targetFP := matchFingerprint(before.Findings, f)
	if targetFP == "" {
		res.Reason = "the finding is no longer present (already fixed?) — nothing to do"
		return res, nil
	}
	beforeFPs := fingerprintSet(before.Findings)

	// Actuate: apply a deterministic analyzer fix, or drive the AI agent.
	if o.Deterministic {
		progress(fmt.Sprintf("applying a deterministic fix for %s at %s …", f.Detector, loc(f)))
		pkgs, _, lerr := loader.LoadChanged(root, []string{file}, nil)
		if lerr != nil {
			return nil, fmt.Errorf("load for deterministic fix: %w", lerr)
		}
		edits, ok := detect.SuggestedEdits(pkgs, f)
		if !ok {
			res.Reason = "no deterministic fix is available for this finding — use an agent"
			return res, nil
		}
		if err := applyEdits(edits); err != nil {
			return finishRollback(top, res, "applying the suggested edit failed: "+err.Error(), nil), nil
		}
	} else {
		actx := ctx
		if o.Timeout > 0 {
			var cancel context.CancelFunc
			actx, cancel = context.WithTimeout(ctx, o.Timeout)
			defer cancel()
		}
		progress(fmt.Sprintf("invoking %s to fix %s at %s (streaming its output below; up to %s)…", o.Agent.Name, f.Detector, loc(f), timeoutLabel(o.Timeout)))
		if err := o.Agent.Run(actx, top, prompt, agentOut(o)); err != nil {
			rollback(top) // agent crashed or timed out — revert anything it half-wrote
			if actx.Err() == context.DeadlineExceeded {
				res.Reason = fmt.Sprintf("agent timed out after %s (raise -timeout, or use -safe for deterministic fixes)", o.Timeout)
			} else {
				res.Reason = "agent failed: " + err.Error()
			}
			res.RolledBack = true
			return res, nil
		}
	}

	if changed, _ := git(top, "status", "--porcelain"); changed == "" {
		res.Reason = "the agent made no changes (it may have judged the finding not real)"
		return res, nil
	}

	// Verify: finding gone, no new findings, still builds (and optionally tests pass).
	progress("verifying the fix (re-scan + build)…")
	after, err := scan.ScanWithOptions(root, scanOpts)
	if err != nil {
		return finishRollback(top, res, "the fix broke the scan/load: "+err.Error(), nil), nil
	}
	if _, still := fingerprintSet(after.Findings)[targetFP]; still {
		return finishRollback(top, res, "the finding is still present after the fix", nil), nil
	}
	if regressions := newFindings(before.Findings, after.Findings, beforeFPs); len(regressions) > 0 {
		res.NewFindings = regressions
		return finishRollback(top, res, fmt.Sprintf("the fix introduced %d new finding(s)", len(regressions)), regressions), nil
	}
	if out, err := goCmd(root, "build", "./..."); err != nil {
		return finishRollback(top, res, "the module no longer builds:\n"+out, nil), nil
	}
	if o.RunTests {
		if out, err := goCmd(root, "test", "./..."); err != nil {
			return finishRollback(top, res, "tests failed after the fix:\n"+tail(out), nil), nil
		}
	}

	// Kept.
	res.Applied = true
	res.Diff, _ = git(top, "diff")
	return res, nil
}

func finishRollback(top string, res *Result, reason string, regs []detect.Finding) *Result {
	rollback(top)
	res.RolledBack = true
	res.Reason = reason
	res.NewFindings = regs
	return res
}

// buildPrompt renders a fix envelope into instructions for an editing agent.
func buildPrompt(env fix.Envelope) string {
	f := env.Finding
	var b strings.Builder
	b.WriteString("Fix exactly ONE issue in this Go repository. Make the smallest correct change.\n\n")
	fmt.Fprintf(&b, "Finding: [%s] %s\n  %s\n\n", f.Detector, loc(f), f.Message)
	fmt.Fprintf(&b, "Root cause: %s\nExpected scope: %s\nReuse: %s\n\n", env.RootCause, env.ExpectedScope, env.ReuseHint)
	b.WriteString("Verify it's REAL before editing:\n")
	for _, v := range env.VerifyFirst {
		fmt.Fprintf(&b, "  - %s\n", v)
	}
	b.WriteString("\nConstraints:\n")
	for _, c := range env.Constraints {
		fmt.Fprintf(&b, "  - %s\n", c)
	}
	b.WriteString("  - Edit files in place. Do NOT create a git commit. Do NOT modify unrelated code or files.\n")
	b.WriteString("  - If after verification it is NOT a real issue, make no change at all.\n")
	return b.String()
}

func loc(f detect.Finding) string {
	if f.Line > 0 {
		return fmt.Sprintf("%s:%d", f.File, f.Line)
	}
	return f.File
}

// matchFingerprint finds the finding in set that corresponds to target (same detector + file, and
// line/message when given) and returns its fingerprint.
func matchFingerprint(set []detect.Finding, target detect.Finding) string {
	for _, f := range set {
		if f.Detector != target.Detector {
			continue
		}
		if !sameFile(f.File, target.File) {
			continue
		}
		if target.Message != "" && f.Message != target.Message {
			continue
		}
		if target.Message == "" && target.Line > 0 && f.Line != target.Line {
			continue
		}
		return f.Fingerprint
	}
	return ""
}

func fingerprintSet(fs []detect.Finding) map[string]struct{} {
	m := make(map[string]struct{}, len(fs))
	for _, f := range fs {
		m[f.Fingerprint] = struct{}{}
	}
	return m
}

func newFindings(before, after []detect.Finding, beforeFPs map[string]struct{}) []detect.Finding {
	var out []detect.Finding
	for _, f := range after {
		if _, existed := beforeFPs[f.Fingerprint]; !existed {
			out = append(out, f)
		}
	}
	return out
}

// sameFile matches a finding's file against a candidate. The verify scan is already scoped to the
// finding's package (unique file basenames within a Go package), so basename equality is enough and
// tolerates absolute-vs-relative path differences.
func sameFile(a, b string) bool {
	return a == b || filepath.Base(a) == filepath.Base(b)
}

// moduleRoot walks up from file to the nearest directory containing go.mod.
func moduleRoot(file string) string {
	dir := filepath.Dir(file)
	for {
		if fi, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil && !fi.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// rollback reverts every change under the repo since the (required) clean starting state: tracked
// modifications are restored and untracked files the agent created are removed. Because Fix refuses
// to start on a dirty tree, this reverts exactly the agent's work and nothing of the user's.
func rollback(top string) {
	_, _ = git(top, "checkout", "--", ".") // restore modified tracked files
	_, _ = git(top, "clean", "-fd")        // remove new (untracked) files the agent created
}

// applyEdits writes a set of resolved text edits to disk. Edits within a file are applied
// back-to-front so earlier offsets stay valid.
func applyEdits(edits []detect.TextEdit) error {
	byFile := map[string][]detect.TextEdit{}
	for _, e := range edits {
		byFile[e.File] = append(byFile[e.File], e)
	}
	for file, es := range byFile {
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		sort.Slice(es, func(i, j int) bool { return es[i].Start > es[j].Start })
		for _, e := range es {
			if e.Start < 0 || e.End > len(data) || e.Start > e.End {
				return fmt.Errorf("edit range [%d:%d] out of bounds for %s", e.Start, e.End, file)
			}
			data = append(data[:e.Start], append([]byte(e.NewText), data[e.End:]...)...)
		}
		if err := os.WriteFile(file, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

func goCmd(dir string, args ...string) (string, error) {
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.String(), err
}

// timeoutLabel renders the agent time budget for progress text ("no limit" when unset).
func timeoutLabel(d time.Duration) string {
	if d <= 0 {
		return "no limit"
	}
	return d.String()
}

func agentOut(o Options) io.Writer {
	if o.Output != nil {
		return o.Output
	}
	return io.Discard
}

func tail(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > 20 {
		lines = lines[len(lines)-20:]
	}
	return strings.Join(lines, "\n")
}
