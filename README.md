# gospect-mcp

![Go](https://img.shields.io/badge/Go-1.21%2B-00ADD8?logo=go&logoColor=white)
![License](https://img.shields.io/badge/License-GPL--3.0-green)
![MCP](https://img.shields.io/badge/MCP-server-8A2BE2)
[![CI](https://github.com/backendArchitect/gospect-mcp/actions/workflows/ci.yml/badge.svg)](https://github.com/backendArchitect/gospect-mcp/actions/workflows/ci.yml)
[![Release](https://github.com/backendArchitect/gospect-mcp/actions/workflows/release.yml/badge.svg)](https://github.com/backendArchitect/gospect-mcp/actions/workflows/release.yml)

**A Go-only, report-first code scanner exposed as an MCP server.** It indexes a Go module,
runs deterministic analyzers, and **reports** genuine bugs, dead code, stale docs and outdated
APIs. It **never modifies your code** — fixes are a separate, explicitly-invoked step.

Think of it as an *MRI for your Go code*: a deep, non-invasive scan that produces a diagnosis
your editor or AI agent acts on — not a tool that silently rewrites things.

---

## Quick start

Install the latest version straight from source (needs Go 1.21+):

```sh
go install github.com/backendArchitect/gospect-mcp@latest
```

This drops a `gospect-mcp` binary in your `$GOBIN` (usually `~/go/bin` — make sure it's on your
`PATH`). Or grab a prebuilt binary (Linux/macOS/Windows, amd64/arm64) from the
[Releases page](https://github.com/backendArchitect/gospect-mcp/releases). Then scan any Go module:

```sh
gospect-mcp scan /path/to/your/module ./...
```

You'll get a JSON report of findings. That's it — no config, no services, no code changes.

> Prefer building it yourself? See [From source](#from-source).

### Keeping up to date

```sh
gospect-mcp version   # print the installed version
gospect-mcp update    # check GitHub for a newer release; update if one exists, else "up to date"
```

`update` checks the latest GitHub release and, when a newer one exists, reinstalls via
`go install …@<tag>`. If no release is newer it prints that you're up to date; if none are
published yet it says so.

---

## Why gospect-mcp

- **Report-first.** The default output is a *report*. It will not touch your code. Fixes only
  happen when you explicitly ask for them.
- **Sensor, not oracle.** The server runs pure Go tooling and emits findings with evidence. It
  uses **no LLM** and detects **no** installed AI — the MCP host you connect it to (Claude Code,
  Cursor, any MCP client) supplies the intelligence. That makes it model- and vendor-agnostic.
- **Genuine over noisy.** It builds on the real Go toolchain (`go/packages`, `go/analysis`,
  SSA) so candidates are semantically backed, not grep guesses.
- **Universal.** Works on any Go module where `go build ./...` succeeds — single- or
  multi-module.

---

## Use it from your AI agent (MCP)

`gospect-mcp` speaks the Model Context Protocol over stdio. Point any MCP host at the binary.

**Claude Code**

```sh
claude mcp add gospect gospect-mcp
```

**Manual config** (`~/.claude.json`, a project `.mcp.json`, Claude Desktop, Cursor, etc.):

```json
{
  "mcpServers": {
    "gospect": {
      "command": "gospect-mcp",
      "args": []
    }
  }
}
```

Then ask your agent to scan a module. It calls the `scan` tool and reasons over the report —
and, because the server is report-only, it can't change your code unless you tell it to.

---

## What it detects

All detectors are deterministic and report-only. Findings carry `category`, `detector`,
`severity`, `file:line`, and `message`; the report includes a `by_category` summary.

| Category | Detectors |
|---|---|
| **bug** | `nilness` (SSA nil-deref), `lostcancel` (leaked `context.CancelFunc`), `httpresponse`, `unmarshal`, `copylock`, `errorsas`, `nilfunc`, `unreachable` |
| **missing** | unimplemented stubs (`panic("not implemented")`), `TODO`/`FIXME` markers, unchecked error returns (errcheck-lite) |
| **modernize** | outdated `go.mod` go directive, `loopclosure` (pre-1.22 loop-var capture) |

Built entirely on `golang.org/x/tools` — no heavyweight dependencies.

**Planned** (see the design notes): over-engineering, stale swagger/OpenAPI vs. routes,
untested exported code, routes-with-no-handler, deprecated-API detection, and a `propose_fix`
tool that emits a *fix envelope* (still never applies code) — all via optional composition with
a code graph such as [codebase-memory-mcp](https://github.com/DeusData/codebase-memory-mcp).

---

## Optional: compose with a code graph

Some detectors need whole-repo relationships (call graph, routes, test coverage) that
single-package analysis can't see. `gospect-mcp` gets these by acting as an MCP **client** of a
code-intelligence graph such as [codebase-memory-mcp](https://github.com/DeusData/codebase-memory-mcp)
— no graph of its own, no duplicated index.

Enable it with three env vars (all optional; unset = graph disabled, local detectors still run):

```sh
export GOSPECT_GRAPH_CMD="codebase-memory-mcp"   # command to launch the graph MCP server
export GOSPECT_GRAPH_PROJECT="my-project"         # project name to query
export GOSPECT_GRAPH_SCOPE="internal/"            # optional file-path substring to scope queries
gospect-mcp scan /path/to/module
```

When configured, the report gains graph-backed findings:
- **untested-exports** — exported functions with no incoming test.
- **over-engineered / high-complexity** — functions/methods whose cyclomatic **or** cognitive
  complexity exceeds conservative thresholds.
- **unhandled-route** — HTTP routes registered with no handler.
- **stale-doc / swagger-drift** — endpoints documented in an OpenAPI/Swagger spec (JSON or YAML)
  with no matching registered route (heuristic path matching; report-first).

A graph connection failure never fails the scan; it's recorded in the report's `graph_error`
field and the local findings are still returned.

## The `scan` tool

**Input**

| field | type | required | default | description |
|---|---|---|---|---|
| `path` | string | ✅ | — | filesystem dir of the Go module |
| `patterns` | string[] | | `["./..."]` | package patterns to scan |

**Output** — a JSON `Report`: load stats + a flat, sorted list of `Finding`s. No mutations.

### Example

```sh
gospect-mcp scan ./testdata/buggy
```

```json
{
  "path": "./testdata/buggy",
  "packages_loaded": 1,
  "load_errors": 0,
  "load_millis": 8,
  "finding_count": 5,
  "by_category": { "bug": 1, "missing": 3, "modernize": 1 },
  "findings": [
    {
      "category": "bug",
      "detector": "nilness",
      "severity": "high",
      "file": ".../buggy.go",
      "line": 8,
      "message": "nil dereference in load"
    },
    {
      "category": "missing",
      "detector": "unchecked-error",
      "severity": "medium",
      "file": ".../stubs.go",
      "line": 14,
      "message": "error return value is not checked"
    }
  ]
}
```

---

## How it works

```
  Go module ──► go/packages (load + type-check) ──► detectors ──► Report (JSON)
                     the risky core                  bug / missing / modernize
```

1. **Load** the target packages with full types + syntax via `go/packages`.
2. **Detect** — run curated `go/analysis` passes plus lightweight AST checks. Each diagnostic
   becomes a `Finding`.
3. **Report** — aggregate, de-dupe, sort, summarize. Never edit.

The MCP layer is a hand-rolled JSON-RPC 2.0 stdio server (`initialize`, `tools/list`,
`tools/call`) — no SDK dependency.

---

## From source

```sh
git clone git@github.com:backendArchitect/gospect-mcp.git
cd gospect-mcp
go build -o gospect-mcp .     # build the binary
go test ./...                 # run the tests
./gospect-mcp scan ./testdata/buggy
```

`testdata/buggy` is a separate module with deliberate issues (a nil-deref, a stub, a TODO, an
unchecked error, an old `go.mod`) that the test suite asserts each detector catches.

---

## Releases & CI

- **PRs** run `go vet` + `go test` + `go build` (`ci.yml`).
- **Pushes to `main`** run the tests and then auto-cut a release (`release.yml`): patch-bump a
  `vX.Y.Z` tag and publish cross-platform binaries + checksums to GitHub Releases. Add
  `[skip release]` to a commit message to skip releasing.

## Roadmap

- [x] Phase 0 — loader, bug/missing/modernize detectors, MCP stdio server, CLI
- [x] Compose with a code graph — MCP **client** + a codebase-memory adapter; graph detectors:
  **untested-exports**, **over-engineering**, **missing-handler**, **stale-swagger**
- [ ] `propose_fix` (emits a fix envelope; still report-first)
- [ ] CI mode with a fix-envelope gate

---

## License

[GPL-3.0](LICENSE).
