# gospect-mcp

A **Go-only, report-first** code scanner exposed as an **MCP server**. It indexes a Go
module, runs deterministic analyzers, and **reports** findings. It never modifies code —
fixes are a separate, explicitly-invoked step (not yet implemented).

Design doc: see the project design notes. Key principles:

- **Sensor, not oracle.** The server runs deterministic Go tooling and emits findings with
  evidence. The connecting MCP host (Claude Code, or any MCP-capable agent) supplies any AI.
  The server itself uses **no LLM** and detects **no** installed AI.
- **Report-first.** Default output is a report. Fixes only on explicit request.
- **Universal.** Works on any Go module where `go build ./...` succeeds.

## Status — Phase 0 (walking skeleton)

Working today:
- `go/packages` loader (the risky core) with load stats.
- Deterministic detectors, report-only:
  - **bug** — `nilness`, `lostcancel`, `httpresponse`, `unmarshal`, `copylock`, `errorsas`,
    `nilfunc`, `unreachable` (all via `golang.org/x/tools`, no extra deps).
  - **missing** — unimplemented stubs (`panic("not implemented")`), `TODO`/`FIXME` markers,
    unchecked error returns (errcheck-lite, with fmt-printer exclusion).
  - **modernize** — outdated `go.mod` go directive; `loopclosure` (pre-1.22 loop-var capture).
- Findings carry `category`, `detector`, `severity`, `file:line`, `message`; the report includes a
  `by_category` summary.
- Minimal MCP server over stdio (hand-rolled JSON-RPC, no SDK dependency): `initialize`,
  `tools/list`, `tools/call`. A `scan` tool + a standalone CLI mode.

Planned (per design): over-engineering + stale-swagger + untested-exports + routes-with-no-handler
(via codebase-memory composition), deprecated-API detection, `propose_fix` (envelope only), CI mode.

## Run

Standalone CLI (prints a JSON report):

```sh
go run . scan /path/to/go/module ./...
```

As an MCP server over stdio (a host connects and calls the `scan` tool):

```sh
go run .
```

Example MCP client config (Claude Code):

```json
{
  "mcpServers": {
    "gospect-mcp": { "command": "gospect-mcp" }
  }
}
```

## The `scan` tool

Input: `{ "path": "<module dir>", "patterns": ["./..."] }` (patterns optional).
Output: a JSON `Report` — load stats + a flat list of `Finding`s (category, detector,
severity, file, line, message). No mutations.

## Test

```sh
go test ./...
```

`testdata/buggy` is a separate module with a deliberate nil dereference; the test confirms
the scanner loads it and `nilness` reports the bug.
