// Package agent locates and drives a system AI coding CLI (Claude Code, aider, …) to apply a fix.
// gospect only *invokes* the agent; it never trusts the result — the caller re-scans and rolls back
// on any regression. Invocation conventions are best-effort (these CLIs evolve); the generic
// --agent template is the reliable escape hatch.
package agent

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

// waitDelay bounds how long we wait for the agent's output pipe to drain after its context is
// cancelled. Without it, a killed agent whose orphaned child processes still hold the pipe open
// would block cmd.Wait indefinitely — defeating the caller's timeout.
const waitDelay = 3 * time.Second

// promptToken is the placeholder replaced by the fix prompt in a command template.
const promptToken = "{prompt}"

// Agent is a resolved, runnable AI coding CLI.
type Agent struct {
	Name string   // display name, e.g. "claude"
	Args []string // full argv; if it contains {prompt} the prompt is substituted there, else piped to stdin
}

// spec is a known agent and how to invoke it headlessly so it edits files in the working directory.
type spec struct {
	name string
	bin  string
	args []string // may contain {prompt}
}

// known lists the agents gospect can auto-detect. The invocation aims to (a) run non-interactively
// and (b) let the tool edit files in place without committing (gospect owns the git checkpoint).
// These are best-effort defaults — override with --agent if a CLI changes.
var known = []spec{
	{"claude", "claude", []string{"-p", promptToken, "--permission-mode", "acceptEdits"}},
	{"aider", "aider", []string{"--yes", "--no-auto-commit", "--message", promptToken}},
	{"cursor-agent", "cursor-agent", []string{"-p", promptToken}},
	{"gemini", "gemini", []string{"-p", promptToken}},
	{"opencode", "opencode", []string{"run", promptToken}},
}

// Detect returns the known agents currently on PATH, in registry order (most preferred first).
func Detect() []Agent {
	var found []Agent
	for _, s := range known {
		if path, err := exec.LookPath(s.bin); err == nil {
			found = append(found, Agent{Name: s.name, Args: append([]string{path}, s.args...)})
		}
	}
	return found
}

// ByName returns the named known agent if it's installed.
func ByName(name string) (Agent, bool) {
	for _, s := range known {
		if s.name == name {
			if path, err := exec.LookPath(s.bin); err == nil {
				return Agent{Name: s.name, Args: append([]string{path}, s.args...)}, true
			}
			return Agent{}, false
		}
	}
	return Agent{}, false
}

// Parse builds an Agent from a user-supplied command template, e.g. `mycli --edit {prompt}`. If the
// template contains {prompt} the prompt is substituted there; otherwise it's piped to the command's
// stdin. Fields are whitespace-split (no shell quoting) — keep it simple.
func Parse(template string) (Agent, error) {
	fields := strings.Fields(template)
	if len(fields) == 0 {
		return Agent{}, fmt.Errorf("empty --agent template")
	}
	return Agent{Name: fields[0], Args: fields}, nil
}

// KnownNames returns the auto-detectable agent names, for help text and errors.
func KnownNames() []string {
	names := make([]string, len(known))
	for i, s := range known {
		names[i] = s.name
	}
	return names
}

// Run executes the agent in dir with the given prompt, streaming its output to out. The prompt is
// substituted for {prompt} in the args, or piped to stdin when no such token exists.
func (a Agent) Run(ctx context.Context, dir, prompt string, out io.Writer) error {
	args, viaStdin := substitute(a.Args, prompt)
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = out, out
	cmd.WaitDelay = waitDelay // don't let a killed agent's orphaned children block Wait forever
	if viaStdin {
		cmd.Stdin = strings.NewReader(prompt)
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("agent %q failed: %w", a.Name, err)
	}
	return nil
}

// substitute replaces {prompt} in args and reports whether the prompt still needs piping via stdin
// (i.e. no {prompt} token was present).
func substitute(args []string, prompt string) (out []string, viaStdin bool) {
	viaStdin = true
	out = make([]string, len(args))
	for i, a := range args {
		if strings.Contains(a, promptToken) {
			out[i] = strings.ReplaceAll(a, promptToken, prompt)
			viaStdin = false
		} else {
			out[i] = a
		}
	}
	return out, viaStdin
}
