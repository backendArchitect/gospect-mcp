// Package fix builds a "fix envelope" for a finding: root cause, an adversarial verify-first
// checklist, expected scope, and ponytail constraints. It EMITS guidance; it never edits code.
package fix

import "github.com/backendArchitect/gospect-mcp/internal/detect"

// Envelope is the report-first fix guidance for one finding.
type Envelope struct {
	Finding       detect.Finding `json:"finding"`
	RootCause     string         `json:"root_cause"`
	ReuseHint     string         `json:"reuse_hint"`
	ExpectedScope string         `json:"expected_scope"`
	VerifyFirst   []string       `json:"verify_first"`
	Constraints   []string       `json:"constraints"`
}

// reuseHint and constraints are universal (the ponytail discipline), independent of detector.
const reuseHint = "Before adding new code, search this package — and the code graph — for an " +
	"existing helper that already does this. Reuse over addition."

func constraints() []string {
	return []string{
		"Smallest root-cause fix — no unrelated changes, no refactors you weren't asked for.",
		"No new abstractions unless the fix genuinely requires one.",
		"Match the surrounding naming, idioms, and comment density.",
		"Add or update one runnable check for any non-trivial logic.",
		"This tool does not apply the fix — you do, only after verifying it's real.",
	}
}

type tmpl struct {
	rootCause string
	scope     string
	verify    []string
}

// byDetector holds per-detector guidance. Anything unknown falls back to a generic envelope.
var byDetector = map[string]tmpl{
	"nilness": {
		"A value that can be nil is dereferenced at this point.",
		"1 function, ~1–5 lines: guard the nil case, or fix the producer so it can't be nil here.",
		[]string{"Is the nil branch actually reachable given real callers and upstream validation?", "Is it already guarded before this point?", "Is fixing the producer more correct than guarding at the use site?"},
	},
	"lostcancel": {
		"A context CancelFunc is never called, leaking the context/timer.",
		"~1–3 lines: `defer cancel()` (or call it on every return path).",
		[]string{"Is cancel truly never called on any path (including error returns)?"},
	},
	"httpresponse": {
		"An HTTP response is used before its error is checked, or its Body isn't closed.",
		"~1–5 lines: check err first; `defer resp.Body.Close()`.",
		[]string{"Can err actually be non-nil with a non-nil resp here?"},
	},
	"unmarshal": {
		"A non-pointer is passed to Unmarshal, so decoding silently does nothing.",
		"1 line: pass the address (&v) instead of the value.",
		[]string{"Is the target really a value (not already a pointer/interface holding one)?"},
	},
	"copylock": {
		"A value containing a sync lock is copied, which corrupts the lock.",
		"Pass or store a pointer instead of copying the value.",
		[]string{"Does the copied type really contain a lock (directly or embedded)?"},
	},
	"errorsas": {
		"errors.As target is not a pointer to a type implementing error.",
		"1 line: pass a pointer to the concrete error type.",
		nil,
	},
	"unreachable": {
		"This code is unreachable.",
		"Delete the dead code (or fix the control flow that made it unreachable).",
		[]string{"Is it unreachable in all build tags/configurations?"},
	},
	"unchecked-error": {
		"An error return value is discarded.",
		"~1–3 lines: check the error and handle or propagate it.",
		[]string{"Is ignoring this error genuinely intentional, or does it hide a real failure?"},
	},
	"stub": {
		"An unimplemented stub (panic/TODO) stands in for real behavior.",
		"Implement the function — or delete it if nothing depends on it.",
		[]string{"Is this stub reachable in production, or dead scaffolding?"},
	},
	"todo": {
		"A TODO/FIXME marks incomplete work.",
		"Resolve the TODO, or convert it into a tracked issue and remove the marker.",
		nil,
	},
	"go-version": {
		"The go.mod go directive is older than recommended.",
		"1 line in go.mod: bump the go directive; then adopt applicable newer features incrementally.",
		[]string{"Do any dependencies or the toolchain constrain the version you can move to?"},
	},
	"loopclosure": {
		"Pre-1.22 loop-variable capture in a closure/goroutine.",
		"On Go 1.22+ this is already safe; otherwise copy the loop variable inside the loop body.",
		[]string{"What go directive does this module declare — is the capture actually a bug here?"},
	},
	"untested-export": {
		"An exported function has no test.",
		"Add a focused unit test covering the main behavior plus one edge case.",
		[]string{"Is it truly untested, or covered indirectly / under a different name the graph missed?"},
	},
	"high-complexity": {
		"This function's cyclomatic/cognitive complexity exceeds the threshold.",
		"Extract cohesive sub-steps into small helpers — reuse existing helpers before writing new ones.",
		[]string{"Is the complexity essential (a real state machine) or accidental (could collapse duplicated branches)?"},
	},
	"unhandled-route": {
		"A route is registered with no handler.",
		"Wire a handler, or remove the route registration if it's obsolete.",
		[]string{"Is the handler linked somewhere the graph didn't capture (dynamic registration)?"},
	},
	"swagger-drift": {
		"A documented endpoint has no matching registered route.",
		"Update the spec (remove/rename the endpoint) or restore the route if it should exist.",
		[]string{"Is the path merely formatted differently (base path or param syntax) rather than truly missing?"},
	},
}

// Build returns the fix envelope for a finding.
func Build(f detect.Finding) Envelope {
	t, ok := byDetector[f.Detector]
	if !ok {
		t = tmpl{
			rootCause: "See the finding message.",
			scope:     "Smallest change that resolves the finding at its root.",
		}
	}
	verify := []string{"Adversarial check FIRST — default to \"not a real issue\" unless you can point to a concrete, reachable case."}
	verify = append(verify, t.verify...)

	return Envelope{
		Finding:       f,
		RootCause:     t.rootCause,
		ReuseHint:     reuseHint,
		ExpectedScope: t.scope,
		VerifyFirst:   verify,
		Constraints:   constraints(),
	}
}
