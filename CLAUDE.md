# CLAUDE.md — InfraSage Master Agent Instructions

> This file is read first by every IDE agent session.
> It points to the other instruction files and gives the agent its operating rules.

---

## What You Are Building

**InfraSage** — a CLI tool written entirely in Go that:
1. Takes a natural language prompt from the user
2. Sends it to a locally running LLM (Ollama, native on macOS)
3. Gets back valid Terraform HCL code
4. Automatically scans that code with three security tools
5. Can push to GitHub and trigger cloud CI/CD
6. Exposes metrics to Prometheus and Grafana

The entire project is a single Go binary called `infrasage`. Everything else (scanners, monitoring) runs in Docker containers managed by that binary.

---

## Developer Machine Constraints (CRITICAL — read before writing any code)

| Constraint | Value | What This Means For You |
|---|---|---|
| Machine | MacBook Air M2 | ARM64 architecture. All Docker images must support linux/arm64 |
| RAM | 8 GB unified | The LLM cannot run inside Docker. It runs natively via Ollama |
| Storage | 256 GB | Model files are large (~2GB). Never store them in the repo |
| Go version | 1.22+ | Use slog, not log. Use any/comparable, not interface{} |
| Docker | Docker Desktop (Apple Silicon) | Must use `platform: linux/arm64` in compose |

**The single most important constraint:** Ollama must run natively on macOS, not in Docker. Running it in Docker on 8GB RAM would starve the OS and other containers. Ollama uses Apple Metal for GPU acceleration natively, giving much faster inference.

**Model strategy:** The project uses `qwen2.5-coder:3b` via Ollama as-is. There is no fine-tuning step. The system prompt is carefully engineered to maximise output quality from the base model.

---

## Instruction Files — Read These In Order

When starting a new feature or phase, read the relevant file:

| File | When To Read It |
|---|---|
| `CLAUDE.md` (this file) | Every session, first |
| `ARCHITECTURE.md` | Before writing any file or folder structure |
| `PHASES.md` | Before starting any new phase of work |
| `CONVENTIONS.md` | Before writing any Go code |
| `DOCKER.md` | Before touching any container or compose config |
| `LLM.md` | Before writing anything in `internal/llm/` |
| `SCANNING.md` | Before writing anything in `internal/scanner/` |
| `CICD.md` | Before writing GitHub Actions or deploy commands |
| `MONITORING.md` | Before writing anything in `internal/monitor/` |

---

## Agent Operating Rules

These rules apply in every session, no exceptions:

**Rule 1 — Read before writing.**
Before creating any file, re-read the relevant instruction doc. The architecture decisions in these files are final and deliberate. Do not invent alternatives.

**Rule 2 — One phase at a time.**
Only write code for the current phase. Do not scaffold Phase 4 code while working on Phase 1. Unfinished scaffolding causes confusion.

**Rule 3 — Test each step before moving on.**
Every step in `PHASES.md` has a test command. Run it. If it fails, fix it before writing the next piece.

**Rule 4 — Never put secrets in code.**
All API keys, tokens, passwords go in `.env`. The `.env` file is always in `.gitignore`. Use `os.Getenv()` or `godotenv.Load()`.

**Rule 5 — Never commit model files.**
The Ollama model lives in `~/.ollama/models/` (managed by Ollama itself, not in the repo). Never `git add` any `.gguf` or `.bin` files.

**Rule 6 — Ask before adding dependencies.**
The Go module has exactly these approved dependencies. Do not add others without a reason:
- `github.com/spf13/cobra` — CLI framework
- `github.com/joho/godotenv` — .env loading
- `github.com/prometheus/client_golang` — metrics
- Standard library for everything else (net/http, os/exec, encoding/json)

**Rule 7 — Docker images must be ARM64 compatible.**
Before using any Docker image, verify it has a `linux/arm64` manifest. The machine is Apple Silicon. x86-only images will fail silently or run under Rosetta at reduced performance.

**Rule 8 — Error messages must be human readable.**
Every error returned from a command must include what the user should do. Example: not `"ollama: connection refused"` but `"Ollama is not running. Fix: run 'ollama serve' in a terminal"`.

---

## Quick Reference — All Commands The Binary Must Support

```
infrasage stack up                     → Start Docker containers + check Ollama
infrasage stack down                   → Stop Docker containers
infrasage stack status                 → Show all container health

infrasage ask "prompt here"            → Generate infra.tf from natural language
infrasage ask "prompt" --out ec2.tf    → Write to custom filename
infrasage ask "prompt" --no-scan       → Skip auto-scan after generation

infrasage scan infra.tf                → Run all 3 scanners, show unified report

infrasage apply infra.tf               → terraform init + validate + plan + apply

infrasage deploy infra.tf              → git commit + push branch + open GitHub PR

infrasage drift                        → terraform plan --detailed-exitcode

infrasage monitor open                 → Open Grafana in browser
infrasage monitor metrics              → Print Prometheus metrics to stdout

infrasage model pull                   → ollama pull <model>
infrasage model status                 → ollama list
```

---

## Definition of Done (For Each Phase)

A phase is done when:
1. All commands listed in that phase's section of `PHASES.md` run without errors
2. The test commands at the bottom of each phase step all pass
3. No `TODO`, `FIXME`, or `panic("implement me")` remains in that phase's code
4. The code compiles with `go build ./...`
5. `go vet ./...` produces no output
