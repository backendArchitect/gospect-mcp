# gospect-mcp

![Go](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white)
[![License](https://img.shields.io/badge/License-MIT-green)](LICENSE)
![MCP](https://img.shields.io/badge/MCP-server-8A2BE2)
[![GitHub Marketplace](https://img.shields.io/badge/Marketplace-gospect--mcp-2ea44f?logo=github)](https://github.com/marketplace/actions/gospect-mcp)
[![CI](https://github.com/backendArchitect/gospect-mcp/actions/workflows/ci.yml/badge.svg)](https://github.com/backendArchitect/gospect-mcp/actions/workflows/ci.yml)
[![Release](https://github.com/backendArchitect/gospect-mcp/actions/workflows/release.yml/badge.svg)](https://github.com/backendArchitect/gospect-mcp/actions/workflows/release.yml)

<img width="1024" height="559" alt="gospect-mcp" src="https://github.com/user-attachments/assets/c302ec7c-ce66-4429-b276-50c473d4c15c" />

### Find the real bugs in your Go code — and only the real ones.

**gospect** reads your code with the actual Go compiler, points at genuine problems — nil crashes,
leaks, unchecked errors — and **never changes anything unless you ask**.

Think of it as an **MRI for your Go code**: a deep, non-invasive scan that gives you a diagnosis, not
a tool that quietly rewrites things.

🌐 **[See it in action → backendarchitect.github.io/gospect-mcp](https://backendarchitect.github.io/gospect-mcp/)**

**Why people use it**

- ✅ **Real, not guessed.** Every finding is backed by the Go toolchain — a fact, not an AI's hunch.
- 🩺 **Read-only by default.** It reports problems; it never edits your code unless you run a fix on purpose.
- 🔌 **Works everywhere.** A command-line tool, a GitHub Action for CI, and an MCP server your AI editor (Claude Code, Cursor, …) can call.

---

## Try it in 30 seconds

```sh
# 1. install (macOS / Linux)
brew install backendArchitect/tap/gospect-mcp

# 2. scan any Go project
gospect-mcp scan /path/to/your/project ./...
```

You get a list of problems, **most important first** — each with the exact file and line. No config,
no setup, no changes to your code.

> Don't want to install anything? Run it in Docker:
> ```sh
> docker run --rm -v "$PWD":/work ghcr.io/backendarchitect/gospect-mcp scan ./...
> ```
> More install options are [below](#install).

---

## What it finds

**By default, only genuine bugs** — the kind that crash or leak:

- 🐞 nil pointer dereferences, forgotten `context` cancels, unclosed HTTP bodies, `Printf`
  format mistakes, lost `sync/atomic` updates, `sort.Slice` misuse, and ~15 more — plus
  `panic("not implemented")` stubs.

That's deliberate: run it on real code and you get a short list of real problems, not hundreds of
style nits. Want the opinionated hygiene checks too — unchecked errors, TODOs, complexity hotspots,
an outdated `go.mod`? Add **`-pedantic`**. Go even deeper with **`-staticcheck`** (~100 more checks).

Everything is ranked by severity (bugs first). The [full detector list is here →](USAGE.md#what-it-detects-full-list)

---

## Three ways to use it

**1. As a command** — scan your project and read the report:

```sh
gospect-mcp scan ./my-project                 # full report (JSON)
gospect-mcp scan -format text -min-severity high ./my-project   # just the serious stuff, readable
```

**2. In your AI editor** — let Claude Code, Cursor, and friends call it:

```sh
claude mcp add gospect gospect-mcp
```

Your assistant runs the scan and reasons over the results. Because gospect is read-only, it **can't
change your code** unless you explicitly ask. ([Other editors →](USAGE.md#connect-an-ai-editor-mcp))

**3. In CI** — fail a pull request that introduces a real bug, and comment the findings on the PR:

```yaml
# .github/workflows/gospect.yml
- uses: backendArchitect/gospect-mcp@v1
  with:
    fail-on: high     # fail the build on any high-severity bug
    comment: true     # post the findings as a PR comment
```

Copy-paste workflow: [examples/gospect.yml](examples/gospect.yml) · more CI options: [USAGE.md](USAGE.md#use-it-in-ci)

---

## Fixing (optional, always opt-in)

gospect finds problems; it doesn't fix them — unless you run the separate `fix` command. Even then
it's careful: it makes **one** change, re-checks that the bug is gone and your code still builds, and
**undoes the change** if anything's off.

```sh
gospect-mcp fix -safe .               # safe, mechanical fixes — no AI involved
gospect-mcp fix -detector nilness .   # or let your AI agent fix it, then verify
```

How the safety net works, agent options, and the guarded MCP fix tool: [USAGE.md → Fixing](USAGE.md#fixing-the-full-story).

---

## Install

| Method | Command |
|---|---|
| **Homebrew** (macOS/Linux) | `brew install backendArchitect/tap/gospect-mcp` |
| **Script** (macOS/Linux) | `curl -fsSL https://raw.githubusercontent.com/backendArchitect/gospect-mcp/main/install.sh \| bash` |
| **Windows** (PowerShell) | `irm https://raw.githubusercontent.com/backendArchitect/gospect-mcp/main/install.ps1 \| iex` |
| **Docker** | `docker run --rm -v "$PWD":/work ghcr.io/backendarchitect/gospect-mcp scan ./...` |
| **Go** | `go install github.com/backendArchitect/gospect-mcp@latest` |
| **pre-commit** | add the repo to `.pre-commit-config.yaml` with `- id: gospect` |

> **No Go toolchain? No problem.** The **prebuilt binaries** and the **Docker image** need nothing
> installed. Only the build-from-source paths — `go install`, and Homebrew/pre-commit (which compile
> it) — need **Go 1.25+** (Homebrew installs its own Go automatically).

Prebuilt binaries (Linux/macOS/Windows · amd64 & arm64) are on the
[Releases page](https://github.com/backendArchitect/gospect-mcp/releases). Run `gospect-mcp help`
for every command and flag. Building from source and all distribution channels:
[packaging/](packaging/README.md).

---

## Learn more

- **[USAGE.md](USAGE.md)** — the full reference: every flag, monorepos, CI, filtering, staticcheck, fixing, MCP setup, and more.
- **[Website](https://backendarchitect.github.io/gospect-mcp/)** — the quick visual tour.
- **[CONTRIBUTING.md](CONTRIBUTING.md)** — build, test, and send a change (please read the [Code of Conduct](CODE_OF_CONDUCT.md)).

**In one line:** gospect is a report-first, Go-only code scanner — a command, a CI gate, and an MCP
server. It reports genuine bugs backed by the real Go toolchain, and never touches your code unless
you ask.

## License

[MIT](LICENSE)
