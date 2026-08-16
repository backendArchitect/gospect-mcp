# Security Policy

## Reporting a vulnerability

Please report security issues **privately** — do not open a public issue.

- Preferred: GitHub's [private vulnerability reporting](https://github.com/backendArchitect/gospect-mcp/security/advisories/new).
- Or email **chauhanvatsal55@gmail.com**.

Include what you found, how to reproduce it, and the impact. You'll get an
acknowledgement within a few days, and a fix or mitigation plan after triage.

## Scope — what matters most

gospect is report-only by default, but two areas are security-sensitive:

- **`fix` (the actuator).** It edits your working tree and can drive an external
  AI agent. The verify-and-rollback harness is what keeps a bad fix from being
  kept — if you find a way for a fix to be applied, or to leave the repo broken,
  when it should have been rolled back, that's a security issue.
- **The MCP server.** By default it exposes only read-only tools. A way to make
  it mutate code without `--allow-fix`, or to escape the intended scope, is a
  security issue.

## Supported versions

Fixes land on `main` and ship in the next release. Please test against the
[latest release](https://github.com/backendArchitect/gospect-mcp/releases)
before reporting.
