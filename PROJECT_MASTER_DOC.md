# PROJECT_MASTER_DOC.md — InfraSage Comprehensive Technical Documentation

> Generated: 2026-04-28  
> Repository: `github.com/Sahil0114/infrasage1`  
> Branch analysed: `copilot/project-master-doc`

---

## Table of Contents

1. [Project Overview](#1-project-overview)
2. [Tech Stack](#2-tech-stack)
3. [Folder & File Structure](#3-folder--file-structure)
4. [End-to-End Data Flow](#4-end-to-end-data-flow)
5. [Implemented Features](#5-implemented-features)
6. [Issues: TODOs, Dead Code, Bugs, Security & Performance](#6-issues-todos-dead-code-bugs-security--performance)
7. [Database Design](#7-database-design)
8. [API Routes & Endpoints (External Integrations)](#8-api-routes--endpoints-external-integrations)
9. [Local Setup Guide](#9-local-setup-guide)
10. [Current Status Assessment](#10-current-status-assessment)

---

## 1. Project Overview

**InfraSage** is a single Go binary CLI tool that:

1. Accepts a natural language prompt from the user
2. Sends it to a locally running Ollama LLM (native on macOS, model `qwen2.5-coder:3b`)
3. Receives back Terraform HCL code
4. Automatically scans the generated HCL with three security scanners (Checkov, tfsec, Terrascan) running in Docker containers
5. Can commit, push to a new git branch, and open a GitHub Pull Request
6. Can run `terraform init / validate / plan / apply` against the generated HCL
7. Detects infrastructure drift via `terraform plan --detailed-exitcode`
8. Exposes Prometheus metrics (generation count, latency, scan findings, drift events)
9. Connects to Grafana dashboards for observability

The project is described as a final-year university DevSecOps demonstration project targeting a MacBook Air M2 with 8 GB RAM.

---

## 2. Tech Stack

### Language & Runtime
| Component | Version / Detail | Source |
|---|---|---|
| Go | 1.23.0 (toolchain 1.24.13) | `go.mod:3-5` |
| Module path | `github.com/Sahil0114/infrasage1` | `go.mod:1` |

### Direct Go Dependencies
| Package | Version | Purpose | Source |
|---|---|---|---|
| `github.com/spf13/cobra` | v1.10.2 | CLI framework — command tree, flags, help text | `go.mod:8` |
| `github.com/joho/godotenv` | v1.5.1 | Loads `.env` file into `os.Getenv()` at startup | `go.mod:8` |
| `github.com/prometheus/client_golang` | v1.23.2 | Prometheus metric registration and `/metrics` HTTP handler | `go.mod:9` |

### Indirect Go Dependencies
| Package | Version | Role |
|---|---|---|
| `github.com/beorn7/perks` | v1.0.1 | Prometheus histogram internals |
| `github.com/cespare/xxhash/v2` | v2.3.0 | Prometheus hash internals |
| `github.com/inconshreveable/mousetrap` | v1.1.0 | Cobra Windows compatibility |
| `github.com/prometheus/client_model` | v0.6.2 | Prometheus protobuf model |
| `github.com/prometheus/common` | v0.66.1 | Prometheus common utilities |
| `github.com/prometheus/procfs` | v0.16.1 | Prometheus process metrics |
| `github.com/spf13/pflag` | v1.0.9 | Cobra flag parsing |
| `go.yaml.in/yaml/v2` | v2.4.2 | YAML parsing (Prometheus dependency) |
| `golang.org/x/sys` | v0.35.0 | System calls |
| `google.golang.org/protobuf` | v1.36.8 | Protobuf (Prometheus dependency) |

> Standard library packages heavily used: `net/http`, `encoding/json`, `os/exec`, `os`, `io`, `path/filepath`, `regexp`, `strings`, `fmt`, `log/slog`, `context`, `time`

### External Tools (must be installed on host, not in Go module)
| Tool | Install | Role |
|---|---|---|
| Ollama | `brew install ollama` | Runs LLM inference natively on macOS with Metal GPU |
| Terraform | `brew install terraform` (≥ 1.6) | HCL validation, plan, apply, drift detection |
| Docker Desktop | docker.com | Hosts scanner containers and monitoring stack |
| git | `xcode-select --install` | Branch management for `deploy` command |

### LLM Model
| Attribute | Value |
|---|---|
| Model name | `qwen2.5-coder:3b` |
| Registry | Ollama (pulled via `ollama pull qwen2.5-coder:3b`) |
| Storage | `~/.ollama/models/` — managed by Ollama, never in repo |
| Inference parameters | temperature=0.1, num_ctx=4096, top_p=0.9 (`internal/llm/client.go:38-42`) |
| Timeout | 120 seconds hardcoded (`internal/llm/client.go:27`) |

### Docker Stack (managed via `deploy/docker-compose.yml`)
| Container name | Image | Port | Role |
|---|---|---|---|
| `infrasage-checkov` | `bridgecrew/checkov:latest` | — | IaC security scanner; kept alive with `tail -f /dev/null` |
| `infrasage-tfsec` | `aquasec/tfsec:latest` | — | IaC security scanner |
| `infrasage-terrascan` | `tenable/terrascan:latest` | — | IaC security scanner |
| `infrasage-prometheus` | `prom/prometheus:latest` | 9091→9090 | Metrics collection |
| `infrasage-grafana` | `grafana/grafana:latest` | 3000→3000 | Metrics visualisation |

All scanner containers share a volume mount: host `/tmp/infrasage-scans` → container `/scans`.

### CI/CD
| File | Trigger | Purpose |
|---|---|---|
| `.github/workflows/infrasage-ci.yml` | PR to `main` (on `**.tf` changes); push to `main` | Runs Checkov, tfsec, Terrascan; runs `terraform plan`; comments on PR; runs `terraform apply` on merge |
| `.github/workflows/drift-check.yml` | Cron `0 */6 * * *` + manual dispatch | Runs `terraform plan --detailed-exitcode`; creates drift-remediation PR if drift detected |

---

## 3. Folder & File Structure

```
infrasage1/                           ← repo root
│
├── main.go                           ← Entry point; one line: cmd.Execute()
│
├── go.mod                            ← Go module: github.com/Sahil0114/infrasage1
├── go.sum                            ← Dependency checksums
├── Makefile                          ← Build/test/lint/stack targets
│
├── .env.example                      ← Environment variable template (copy to .env)
├── .gitignore                        ← Ignores .env, bin/, .terraform/, *.tfstate, models
│
├── ci_trigger.tf                     ← Minimal Terraform file (terraform { required_version = ">= 1.6.0" })
│                                       Purpose: triggers CI pipeline; acts as placeholder
│
├── infrasage-agent-docs.zip          ← Binary artifact committed to repo (agent documentation)
│
├── cmd/                              ← Cobra command layer; no business logic
│   ├── root.go                       ← rootCmd; loads .env; global --debug flag; Execute()
│   ├── ask.go                        ← `infrasage ask "..."` — main generation command
│   ├── scan.go                       ← `infrasage scan <file>` — standalone scan
│   ├── apply.go                      ← `infrasage apply <file>` — terraform workflow
│   ├── deploy.go                     ← `infrasage deploy <file>` — git + PR
│   ├── drift.go                      ← `infrasage drift` — drift detection
│   ├── stack.go                      ← `infrasage stack up|down|status`
│   ├── monitor.go                    ← `infrasage monitor open|metrics`
│   └── model.go                      ← `infrasage model pull|status`
│
├── internal/                         ← All business logic packages
│   ├── llm/
│   │   ├── client.go                 ← OllamaClient struct; Generate(); IsHealthy()
│   │   └── client_test.go            ← Unit + integration tests for OllamaClient
│   │
│   ├── terraform/
│   │   ├── parser.go                 ← ExtractHCL() — strips markdown fences from LLM output
│   │   ├── parser_test.go            ← 11 unit tests for ExtractHCL
│   │   └── wrapper.go                ← Init(), Validate(), Plan(), Apply() — shells out to terraform
│   │
│   ├── scanner/
│   │   ├── types.go                  ← Finding, ScanResult, Report structs; Print(); HasCritical()
│   │   ├── runner.go                 ← RunAll() — runs 3 scanners in parallel goroutines
│   │   ├── docker.go                 ← scanDir(), copyToScanDir(), dockerExec(); dead code: containerPath(), findJSONStart()
│   │   ├── checkov.go                ← RunCheckov(); parseCheckovOutput(); severity/message lookup tables
│   │   ├── tfsec.go                  ← RunTfsec(); tfsec JSON parsing
│   │   └── terrascan.go              ← RunTerrascan(); terrascan JSON parsing
│   │
│   ├── gitops/
│   │   ├── git.go                    ← CommitAndPush() — git checkout/add/commit/push
│   │   └── github.go                 ← CreatePR() — GitHub REST API v3 PR creation
│   │
│   └── monitor/
│       └── metrics.go                ← Prometheus metric registration; StartServer(); Record* helpers
│
├── deploy/
│   ├── docker-compose.yml            ← 5-service stack (3 scanners + Prometheus + Grafana)
│   ├── prometheus.yml                ← Scrape config; targets host.docker.internal:2112
│   └── grafana/
│       ├── provisioning/
│       │   ├── datasources/
│       │   │   └── prometheus.yml    ← Auto-provisions Prometheus datasource pointing to infrasage-prometheus:9090
│       │   └── dashboards/
│       │       └── dashboard.yml     ← Auto-loads dashboards from /var/lib/grafana/dashboards
│       └── dashboards/
│           └── infrasage.json        ← Grafana dashboard JSON (stat, timeseries, bargauge panels)
│
├── examples/
│   ├── s3-bucket.tf                  ← Full S3 example with versioning + encryption + public access block
│   └── ec2-instance.tf               ← Full EC2 example with VPC, SG, IMDSv2, monitoring, encrypted EBS
│
├── .github/
│   └── workflows/
│       ├── infrasage-ci.yml          ← PR scan + plan + merge apply CI pipeline
│       └── drift-check.yml           ← Scheduled drift detection (every 6 hours)
│
└── Agent instruction docs (not code):
    ├── ARCHITECTURE.md
    ├── CICD.md
    ├── CLAUDE.md
    ├── CONVENTIONS.md
    ├── DOCKER.md
    ├── LLM.md
    ├── MONITORING.md
    ├── PHASES.md
    └── SCANNING.md
```

---

## 4. End-to-End Data Flow

### 4.1 `infrasage ask "create an S3 bucket"` — Primary Flow

```
User input
   │
   ▼ cmd/ask.go:runAsk()
1. Read env: INFRASAGE_OLLAMA_URL (default http://localhost:11434)
              INFRASAGE_OLLAMA_MODEL (default qwen2.5-coder:3b)

2. internal/llm.NewOllamaClient(url, model)
   → OllamaClient{BaseURL, Model, Timeout: 120s}

3. client.IsHealthy()          [internal/llm/client.go:103]
   → HTTP GET http://localhost:11434/api/tags  (3s timeout)
   → If non-200 or error: return human-readable error, exit 1

4. client.Generate(systemPrompt, userPrompt)  [internal/llm/client.go:32]
   → JSON body: { model, prompt, system, stream:false,
                  options: {temperature:0.1, num_ctx:4096, top_p:0.9} }
   → HTTP POST http://localhost:11434/api/generate  (120s timeout)
   → Decode: { "response": "<raw LLM text>", "done": true }
   → Return raw string

5. tf.ExtractHCL(response)     [internal/terraform/parser.go:21]
   → Try regex: ```hcl/terraform/tf ... ```  (fenced code block)
   → Fallback: search for "terraform {", "resource \"", "provider \""
   → Return clean HCL string, or "" if nothing found

6. If hcl == "": error + exit 1

7. os.WriteFile(askOutFile, hcl+"\n", 0644)
   Default file: infra.tf

8. Print: "✅ Generated in X.Xs → infra.tf"
   Print: HCL content to stdout

9. scanner.RunAll(askOutFile)   [internal/scanner/runner.go:14]
   → filepath.Abs(tfFile)
   → 3 goroutines send to buffered channel (cap 3):
       goroutine A: RunCheckov(absPath)   [internal/scanner/checkov.go:81]
       goroutine B: RunTfsec(absPath)     [internal/scanner/tfsec.go:15]
       goroutine C: RunTerrascan(absPath) [internal/scanner/terrascan.go:14]

   Each goroutine:
     a. copyToScanDir(tfFile)   [internal/scanner/docker.go:27]
        → mkdir -p /tmp/infrasage-scans
        → io.Copy src → /tmp/infrasage-scans/<filename>
     b. dockerExec("infrasage-checkov",  [internal/scanner/docker.go:68]
          "checkov", "-f", "/scans/<filename>", "--output", "json", "--quiet")
        → exec.CommandContext with 60s timeout
        → docker exec infrasage-checkov <args>
        → Return stdout bytes (even on non-zero exit; scanners exit 1 on findings)
     c. Parse JSON output → ScanResult{Tool, Passed, Failed, Findings, Duration}

10. Collect 3 ScanResults into *Report{File, Results}
    report.Print()  →  Unicode box table to stdout

11. For each finding:
    monitor.RecordScanFinding(tool, severity)  [internal/monitor/metrics.go:82]
    → scanFindings gauge: Add(1) with labels {scanner, severity}

12. monitor.RecordGeneration("success")        [internal/monitor/metrics.go:71]
    → generationsTotal counter: Inc() with label {status="success"}

13. monitor.RecordModelLatency(elapsed)        [internal/monitor/metrics.go:76]
    → modelLatency histogram: Observe(seconds)
```

### 4.2 `infrasage scan <file>` Flow

```
cmd/scan.go:runScan()
→ os.Stat(tfFile) — verify file exists
→ scanner.RunAll(tfFile) — identical to ask step 9 above
→ report.Print()
→ for each finding: monitor.RecordScanFinding()
→ if report.HasCritical(): fmt.Fprintf(os.Stderr, ...), os.Exit(1)
```

### 4.3 `infrasage apply <file>` Flow

```
cmd/apply.go:runApply()
→ filepath.Abs(tfFile) → dir = filepath.Dir(absPath)
→ tf.Init(dir)        → exec: terraform init            (120s timeout)
→ tf.Validate(dir)    → exec: terraform validate        (60s timeout)
→ tf.Plan(dir)        → exec: terraform plan -detailed-exitcode  (300s timeout)
   exitCode 0 → "No changes", return
   exitCode 2 → proceed to apply
   other      → error
→ tf.Apply(dir)       → exec: terraform apply -auto-approve  (600s timeout)
```

### 4.4 `infrasage deploy <file>` Flow

```
cmd/deploy.go:runDeploy()
→ Derive branch name: "infrasage/<basename>-<timestamp>"   if not --branch
→ Derive commit msg:  "chore(infrasage): add <filename>"   if not --message

→ gitops.CommitAndPush(file, branch, message)   [internal/gitops/git.go:22]
   → exec: git checkout -b <branch>   (60s)
   → exec: git add <file>
   → exec: git commit -m <message>
   → exec: git push --set-upstream origin <branch>

→ gitops.CreatePR(branch, title, body)          [internal/gitops/github.go:19]
   → Read GITHUB_TOKEN, GITHUB_REPO, GITHUB_DEFAULT_BRANCH from env
   → HTTP POST https://api.github.com/repos/<owner/repo>/pulls  (15s timeout)
     Headers: Authorization: Bearer <token>
              Accept: application/vnd.github+json
              X-GitHub-Api-Version: 2022-11-28
   → Return pr.HTMLURL
```

### 4.5 `infrasage drift` Flow

```
cmd/drift.go:runDrift()
→ filepath.Abs(".")       current working directory
→ filepath.Glob("*.tf")  ensure .tf files exist
→ tf.Init(dir)
→ tf.Plan(dir)  → exitCode 0: no drift; exitCode 2: drift → os.Exit(2)
```
> **Note:** `monitor.RecordDrift()` is defined but never called from `cmd/drift.go`. Drift events are not recorded in Prometheus.

### 4.6 `infrasage stack up` Flow

```
cmd/stack.go:runStackUp()
→ os.MkdirAll(INFRASAGE_SCAN_DIR, 0755)
→ exec.LookPath("ollama")   — check ollama binary exists
→ isOllamaRunning():  exec curl -sf --max-time 2 http://localhost:11434/api/tags
→ If not running: exec ollama serve (detached goroutine)
→ exec docker compose -f <INFRASAGE_COMPOSE_FILE> up -d
```

### 4.7 Prometheus Metrics Flow (intended; partially broken — see §6)

```
infrasage CLI binary (host :2112)
   │  monitor.StartServer() should start here (NOT currently called — see §6.1)
   │  GET /metrics → Prometheus exposition format
   │
   ▼ scrape every 10s (deploy/prometheus.yml)
infrasage-prometheus container
   │  host.docker.internal:2112 → host machine :2112
   │
   ▼ PromQL queries every 15s
infrasage-grafana container
   │  datasource: http://infrasage-prometheus:9090
   │
   ▼
User → http://localhost:3000  (admin / infrasage)
```

---

## 5. Implemented Features

| Feature | Command | Status | Key Files |
|---|---|---|---|
| LLM-based Terraform HCL generation | `infrasage ask "..."` | ✅ Implemented | `cmd/ask.go`, `internal/llm/client.go`, `internal/terraform/parser.go` |
| Custom output filename | `infrasage ask "..." --out ec2.tf` | ✅ Implemented | `cmd/ask.go:62-65` |
| Skip auto-scan | `infrasage ask "..." --no-scan` | ✅ Implemented | `cmd/ask.go:66-69` |
| Security scan (Checkov) | `infrasage scan <file>` | ✅ Implemented | `internal/scanner/checkov.go` |
| Security scan (tfsec) | `infrasage scan <file>` | ✅ Implemented | `internal/scanner/tfsec.go` |
| Security scan (Terrascan) | `infrasage scan <file>` | ✅ Implemented | `internal/scanner/terrascan.go` |
| Parallel scanner execution | Automatic in `ask`/`scan` | ✅ Implemented | `internal/scanner/runner.go:26-28` |
| Unified scan report table | Auto-printed after scan | ✅ Implemented | `internal/scanner/types.go:66-131` |
| CRITICAL exit code (CI gate) | `infrasage scan` exits 1 on CRITICAL | ✅ Implemented | `cmd/scan.go:56-59` |
| Terraform init/validate/plan/apply | `infrasage apply <file>` | ✅ Implemented | `cmd/apply.go`, `internal/terraform/wrapper.go` |
| Drift detection | `infrasage drift` | ✅ Implemented | `cmd/drift.go`, `internal/terraform/wrapper.go:56` |
| Git commit + push branch | `infrasage deploy <file>` | ✅ Implemented | `internal/gitops/git.go` |
| GitHub PR creation | `infrasage deploy <file>` | ✅ Implemented | `internal/gitops/github.go` |
| Docker stack management | `infrasage stack up/down/status` | ✅ Implemented | `cmd/stack.go` |
| Ollama health check + auto-start | part of `stack up` | ✅ Implemented | `cmd/stack.go:89-102` |
| Model pull / status | `infrasage model pull/status` | ✅ Implemented | `cmd/model.go` |
| Open Grafana in browser | `infrasage monitor open` | ✅ Implemented | `cmd/monitor.go:43-71` |
| Print Prometheus metrics | `infrasage monitor metrics` | ✅ Implemented | `cmd/monitor.go:73-99` |
| Prometheus metric registration | auto on any command | ✅ Implemented | `internal/monitor/metrics.go` |
| Prometheus HTTP metrics server | `:2112/metrics` | ❌ **BROKEN** — server never started | See §6.1 |
| Grafana dashboard | auto-provisioned | ✅ Provisioned | `deploy/grafana/dashboards/infrasage.json` |
| CI security scanning | on PR to main | ✅ Implemented | `.github/workflows/infrasage-ci.yml` |
| CI terraform plan + PR comment | on PR to main | ✅ Implemented | `.github/workflows/infrasage-ci.yml:86-158` |
| CI terraform apply on merge | on push to main | ✅ Implemented | `.github/workflows/infrasage-ci.yml:159-184` |
| Scheduled drift detection | every 6 hours | ✅ Implemented | `.github/workflows/drift-check.yml` |
| Auto drift-remediation PR | when drift detected | ✅ Implemented | `.github/workflows/drift-check.yml:86-127` |
| Debug logging (`--debug` flag) | global flag | ✅ Implemented | `cmd/root.go:39-43` |
| `.env` loading | auto on startup | ✅ Implemented | `cmd/root.go:35-38` |
| Human-readable error messages with Fix: hints | all commands | ✅ Implemented | all `cmd/*.go` |

---

## 6. Issues: TODOs, Dead Code, Bugs, Security & Performance

### 6.1 **BUG (Critical)** — `monitor.StartServer()` is Never Called

**File:** `internal/monitor/metrics.go:57` defines `StartServer(port string)`  
**Expected caller:** `cmd/root.go` — `PersistentPreRunE` or a new `PersistentPreRun`  
**Actual:** `cmd/root.go:19-29` only configures debug logging. `StartServer` is never imported or called from any `cmd/` file.

**Impact:**  
- `curl http://localhost:2112/metrics` returns "connection refused" — the endpoint does not exist
- Prometheus cannot scrape the CLI binary
- `infrasage monitor metrics` will always fail with "cannot reach metrics server"
- All `monitor.Record*()` calls succeed (they update in-memory counters), but the data is never exposed

**Fix referenced in `MONITORING.md:58-63`:**
```go
// In cmd/root.go PersistentPreRunE, after debug setup:
port := getEnvOrDefault("INFRASAGE_METRICS_PORT", "2112")
monitor.StartServer(port)
```

---

### 6.2 **BUG (Medium)** — `RecordDrift()` is Never Called

**File:** `internal/monitor/metrics.go:87-89` defines `RecordDrift(resourceType string)`  
**Expected caller:** `cmd/drift.go`  
**Actual:** `cmd/drift.go` calls `tf.Plan()` and uses `os.Exit(2)` on drift detection, but never calls `monitor.RecordDrift()`

**Impact:** `infrasage_drift_detected_total` metric always stays at 0, making the Grafana drift panel useless.

---

### 6.3 **Dead Code** — `containerPath()` in `internal/scanner/docker.go`

**File:** `internal/scanner/docker.go:55-58`
```go
func containerPath(hostPath string) string {
    base := filepath.Base(hostPath)
    return "/scans/" + base
}
```
This function is defined but never called. Each scanner file (`checkov.go`, `tfsec.go`, `terrascan.go`) constructs the container path inline:
- `checkov.go:93`: `containerPath := "/scans/" + filename`
- `tfsec.go:28`: `inContainerPath := "/scans/" + filepath.Base(destPath)`
- `terrascan.go:25`: `inContainerPath := "/scans/" + filepath.Base(destPath)`

**Impact:** No runtime impact; code clutter. `go vet` does not flag unused functions.

---

### 6.4 **Dead Code** — `findJSONStart()` in `internal/scanner/docker.go`

**File:** `internal/scanner/docker.go:110-116`
```go
func findJSONStart(data []byte) []byte {
    idx := bytes.IndexByte(data, '{')
    if idx < 0 {
        return data
    }
    return data[idx:]
}
```
Defined but never called. Each scanner file does its own inline `bytes.IndexByte()` call:
- `checkov.go:119`: `jsonStart := bytes.IndexByte(output, '{')`
- `tfsec.go:43`: `jsonStart := bytes.IndexByte(output, '{')`
- `terrascan.go:42`: `jsonStart := bytes.IndexByte(output, '{')`

Additionally, the existing `findJSONStart` returns the original `data` on failure (`idx < 0`), whereas the inline code returns early — a subtle difference that could cause a JSON parse error downstream if used. Not a safety issue here since it's dead, but shows inconsistency.

---

### 6.5 **Bug (Minor)** — `INFRASAGE_OLLAMA_TIMEOUT` Env Var is Not Consumed

**Documented in:** `.env.example:20`, `CONVENTIONS.md:84`  
**Expected file:** `internal/llm/client.go`  
**Actual code:** `internal/llm/client.go:27` hardcodes `Timeout: 120 * time.Second`

The env var `INFRASAGE_OLLAMA_TIMEOUT` is advertised but has no effect. Users who set it expecting a longer timeout (for larger models) will be surprised.

---

### 6.6 **Bug (Minor)** — Scan Findings Metric Uses `Add()` Instead of `Set()`

**File:** `internal/monitor/metrics.go:83`
```go
func RecordScanFinding(tool, severity string) {
    scanFindings.WithLabelValues(tool, severity).Add(1)
}
```
`MONITORING.md:119` explicitly says _"Use `Set()` not `Inc()` because this represents the current state, not a running total"_.

**Impact:** After running 3 `infrasage ask` commands that each find 5 HIGH findings from Checkov, the gauge will show `15` rather than `5` (the count from the last scan). The value grows monotonically and never resets. This defeats the purpose of a Gauge.

---

### 6.7 **Missing Metric** — `infrasage_scan_duration_seconds` Not Implemented

**Documented in:** `MONITORING.md:162-173`, referenced in Grafana dashboard Panel 5  
**Actual:** The metric is not registered in `internal/monitor/metrics.go`. Only 4 metrics are registered (`generationsTotal`, `modelLatency`, `scanFindings`, `driftDetectedTotal`).  
**Impact:** Grafana Panel 5 ("Scan Duration by Tool") will show "No data."

---

### 6.8 **Inconsistency** — Drift Metric Name Mismatch

| Location | Metric name |
|---|---|
| `internal/monitor/metrics.go:37` | `infrasage_drift_detected_total` |
| `MONITORING.md:146` | `infrasage_drift_events_total` |
| Grafana dashboard panel 6 query | `rate(infrasage_drift_events_total[1h])` |

The Prometheus metric registered in code differs from both the docs and the Grafana dashboard query. The Grafana drift panel will always show "No data" even if drift recording were fixed.

---

### 6.9 **DRY Violation** — Duplicate `getEnvOrDefault` Helpers

`cmd/ask.go:173-178` defines `getEnvOrDefault(key, defaultVal string) string`  
`cmd/stack.go:21-26` defines `getEnvOrDefaultStack(key, defaultVal string) string`  
`cmd/monitor.go:74` calls `getEnvOrDefault()` from `ask.go` (works because same package)  
`cmd/model.go:45, 73` also calls `getEnvOrDefault()` from `ask.go`

Both functions are identical in behaviour. Having two copies in the same package is unnecessary but not harmful since Go merges a single `cmd` package.

---

### 6.10 **Terrascan ARM64 Gap**

**File:** `deploy/docker-compose.yml:44-52`  
Checkov and tfsec have `platform: linux/arm64`. Terrascan (`tenable/terrascan:latest`) does **not** have a platform constraint:
```yaml
terrascan:
    image: tenable/terrascan:latest
    container_name: infrasage-terrascan
    # No platform: line
```
On Apple Silicon this will either fail to pull (if `tenable/terrascan` has no arm64 manifest) or run under Rosetta 2 emulation. The ARCHITECTURE.md comment at line 42 acknowledges this: _"if linux/arm64 is unavailable it runs under Rosetta 2 on Apple Silicon, which is acceptable for a scan-only workload."_

---

### 6.11 **Image Name Discrepancy (ARCHITECTURE.md vs docker-compose.yml)**

`ARCHITECTURE.md:341` lists the Terrascan image as `accurics/terrascan:latest`  
`deploy/docker-compose.yml:47` uses `tenable/terrascan:latest`

Tenable acquired Accurics, so both images are the same product, but the architecture doc is outdated.

---

### 6.12 **Grafana Datasource URL Discrepancy**

`MONITORING.md:207` specifies `url: http://prometheus:9090`  
`deploy/grafana/provisioning/datasources/prometheus.yml:8` uses `url: http://infrasage-prometheus:9090`

The actual file is correct (using the container name defined in docker-compose.yml as `container_name: infrasage-prometheus`). The doc is wrong. Not a runtime issue.

---

### 6.13 **`ci_trigger.tf` at Repo Root**

**File:** `ci_trigger.tf`
```hcl
terraform {
  required_version = ">= 1.6.0"
}
```
This minimal file triggers CI workflows on all `.tf` changes. It will also be scanned by Checkov/tfsec/Terrascan in CI (all scanners will likely pass since it contains no resources). The purpose appears to be to demonstrate that CI triggers work even without a real generated `infra.tf`. **It is safe but could cause confusion** since `terraform init` at the repo root will use this file.

---

### 6.14 **`infrasage-agent-docs.zip` Binary in Repo**

**File:** `infrasage-agent-docs.zip` (binary, ~size unknown)  
Committed to the repository root. Binary blobs in git increase clone size and are not version-diffable. This appears to be agent documentation. Consider using a release asset or documentation site instead.

---

### 6.15 **tfsec `Passed` Count Always 0**

**File:** `internal/scanner/tfsec.go:92`
```go
return ScanResult{
    Tool:   "tfsec",
    Passed: 0, // tfsec only reports failures, not passes
    ...
}
```
This is an acknowledged limitation — tfsec's JSON output format does not include passing checks. The `TotalPassed()` count in the report will undercount when tfsec is included.

---

### 6.16 **Security** — Grafana Anonymous Access Enabled

**File:** `deploy/docker-compose.yml:81`
```yaml
- GF_AUTH_ANONYMOUS_ENABLED=true
- GF_AUTH_ANONYMOUS_ORG_ROLE=Viewer
```
Anonymous access allows anyone on the same network as `localhost:3000` to view all dashboards without credentials. Acceptable for a local dev environment; must be disabled before any deployment that is network-accessible.

---

### 6.17 **Security** — GitHub Token in `.env` (Expected; Documented)

`GITHUB_TOKEN` must be set in `.env` for the `deploy` command to work. The `.gitignore` correctly excludes `.env`. This is acceptable but users should be warned that a PAT with `repo` scope has broad repository access.

---

### 6.18 **Performance** — No Context Cancellation Propagation in Scanner Goroutines

**File:** `internal/scanner/runner.go:26-28`  
The three scanner goroutines are launched without any cancellation mechanism. If one scanner hangs, the other two will still complete, but the calling goroutine waits for all three. The per-scanner `dockerExec()` has a 60-second timeout (`docker.go:69`), which mitigates this, but there is no global scan timeout propagated from the caller.

---

### 6.19 **Digger Workflow Without Digger Configuration**

**File:** `.github/workflows/infrasage-ci.yml:159-184`  
The workflow job is named "Digger (Terraform Apply on Merge)" and comments reference Digger, but the implementation is a plain `terraform apply -auto-approve` step — no Digger action or `digger.yml` config file is present. This means the expected Digger PR orchestration (plan comments, manual approval gates) is not actually wired up.

---

## 7. Database Design

**InfraSage has no database.** There is no persistent data store (SQL, NoSQL, file-based) in this project. All state is:

| State | Storage | Persistence |
|---|---|---|
| Generated HCL files | Local filesystem (user's working directory) | Until `git add` / manual delete |
| Prometheus metrics | In-memory counters in the running CLI process | Lost on process exit |
| Terraform state | `.tfstate` files (gitignored; expected in S3 backend for real use) | On disk; not in repo |
| Grafana dashboard state | `grafana-storage` Docker named volume | Until `docker compose down -v` |
| Prometheus scraped data | Prometheus Docker container (ephemeral) | Until container restart |
| Ollama model files | `~/.ollama/models/` on host | Persistent; managed by Ollama |
| Git history | Local `.git/` + remote GitHub | Permanent |

---

## 8. API Routes & Endpoints (External Integrations)

This section documents every HTTP API the binary calls or exposes.

### 8.1 Endpoints Exposed by the CLI Binary

| Method | Path | Port | Handler | Purpose |
|---|---|---|---|---|
| `GET` | `/metrics` | `:2112` | `promhttp.Handler()` | Prometheus exposition format; scraped by Prometheus container |

> **Note:** This endpoint is currently broken — `monitor.StartServer()` is never called. See §6.1.

---

### 8.2 Ollama REST API (called by `internal/llm/client.go`)

| Method | URL | Called From | Request | Response |
|---|---|---|---|---|
| `GET` | `{INFRASAGE_OLLAMA_URL}/api/tags` | `client.IsHealthy()` L107 | none | `{ models: [...] }` — 200 = healthy |
| `POST` | `{INFRASAGE_OLLAMA_URL}/api/generate` | `client.Generate()` L59 | `{ model, prompt, system, stream:false, options:{temperature,num_ctx,top_p} }` | `{ response: "<HCL text>", done: bool, error: "" }` |

Default base URL: `http://localhost:11434`

---

### 8.3 GitHub REST API (called by `internal/gitops/github.go`)

| Method | URL | Called From | Request Body | Response |
|---|---|---|---|---|
| `POST` | `https://api.github.com/repos/{owner}/{repo}/pulls` | `CreatePR()` L54 | `{ title, body, head, base }` | `{ html_url, number }` (HTTP 201) |

Headers sent:
- `Authorization: Bearer <GITHUB_TOKEN>`
- `Accept: application/vnd.github+json`
- `X-GitHub-Api-Version: 2022-11-28`

---

### 8.4 External HTTP Clients in `cmd/monitor.go`

| Method | URL | Purpose |
|---|---|---|
| `GET` | `http://localhost:{INFRASAGE_METRICS_PORT}/metrics` | `monitor metrics` command fetches and prints the metrics page |

---

### 8.5 GitHub Actions — External Actions Used

| Action | Version | Used In | Purpose |
|---|---|---|---|
| `actions/checkout` | v4 | both workflows | Checkout repository |
| `hashicorp/setup-terraform` | v3 | both workflows | Install Terraform 1.6.0 |
| `bridgecrewio/checkov-action` | master | infrasage-ci.yml | Run Checkov |
| `aquasecurity/tfsec-action` | v1.0.0 | infrasage-ci.yml | Run tfsec |
| `tenable/terrascan-action` | main | infrasage-ci.yml | Run Terrascan |
| `actions/github-script` | v7 | infrasage-ci.yml | Post PR comments |
| `aws-actions/configure-aws-credentials` | v4 | drift-check.yml | Configure AWS credentials |
| `peter-evans/create-pull-request` | v6 | drift-check.yml | Create drift-remediation PR |

---

## 9. Local Setup Guide

### 9.1 Prerequisites

Install all required tools on macOS (Apple Silicon):

```bash
# Install Homebrew if not present
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"

# Required tools
brew install go          # Go 1.23+
brew install ollama      # LLM runtime (native Metal GPU)
brew install terraform   # Infrastructure provisioning
brew install git         # Version control

# Install Docker Desktop from https://docker.com
# Enable "Use Rosetta 2 for x86/amd64 emulation" in Docker Desktop settings
```

### 9.2 Clone and Configure

```bash
git clone https://github.com/Sahil0114/infrasage1.git
cd infrasage1

# Copy environment template
cp .env.example .env

# Edit .env — minimum required values:
# GITHUB_TOKEN=<your-PAT-with-repo-scope>   # Only needed for 'infrasage deploy'
# AWS_ACCESS_KEY_ID=...                      # Only needed for 'infrasage apply'
# AWS_SECRET_ACCESS_KEY=...
```

### 9.3 Environment Variables Reference

| Variable | Default | Required For | Description |
|---|---|---|---|
| `INFRASAGE_OLLAMA_URL` | `http://localhost:11434` | `ask`, `stack up` | Ollama API base URL |
| `INFRASAGE_OLLAMA_MODEL` | `qwen2.5-coder:3b` | `ask`, `model pull/status` | Model identifier |
| `INFRASAGE_OLLAMA_TIMEOUT` | `120s` | *(documented; not consumed — see §6.5)* | LLM generation timeout |
| `INFRASAGE_SCAN_DIR` | `/tmp/infrasage-scans` | `scan`, `ask` | Shared scan directory host path |
| `INFRASAGE_COMPOSE_FILE` | `./deploy/docker-compose.yml` | `stack *` | Docker Compose file path |
| `INFRASAGE_METRICS_PORT` | `2112` | `monitor metrics` | Prometheus metrics server port |
| `INFRASAGE_GRAFANA_URL` | `http://localhost:3000` | `monitor open` | Grafana dashboard URL |
| `GITHUB_TOKEN` | *(none)* | `deploy` | GitHub PAT with `repo` scope |
| `GITHUB_REPO` | `Sahil0114/infrasage1` | `deploy` | Target repo in `owner/repo` format |
| `GITHUB_DEFAULT_BRANCH` | `main` | `deploy` | PR base branch |
| `AWS_REGION` | `us-east-1` | `apply`, `drift` | AWS region for Terraform |
| `AWS_ACCESS_KEY_ID` | *(none)* | `apply`, `drift` | AWS credentials |
| `AWS_SECRET_ACCESS_KEY` | *(none)* | `apply`, `drift` | AWS credentials |

### 9.4 One-Time Setup

```bash
# Step 1: Pull the LLM model (~2 GB download)
ollama pull qwen2.5-coder:3b

# Step 2: Start Ollama serve (keep running, or use brew services)
ollama serve
# OR: brew services start ollama

# Step 3: Build the binary
make build
# Output: bin/infrasage

# Step 4: Start Docker stack (scanners + monitoring)
./bin/infrasage stack up
# OR: make stack-up

# Verify Ollama: curl http://localhost:11434/api/tags
# Verify Prometheus: http://localhost:9091
# Verify Grafana: http://localhost:3000  (admin / infrasage)
```

Alternatively, run the combined setup target:
```bash
make setup   # checks deps + model-pull + stack-up + build
```

### 9.5 Daily Usage

```bash
# Generate Terraform HCL and auto-scan
./bin/infrasage ask "create an S3 bucket with versioning and encryption"

# Generate to custom file, skip scan
./bin/infrasage ask "create an EC2 instance" --out ec2.tf --no-scan

# Standalone scan of any .tf file
./bin/infrasage scan examples/s3-bucket.tf

# Terraform workflow (requires AWS credentials)
./bin/infrasage apply infra.tf

# Commit + push branch + open GitHub PR
./bin/infrasage deploy infra.tf

# Drift detection (cd to directory with .tf files first)
./bin/infrasage drift

# Monitoring
./bin/infrasage monitor open     # opens Grafana
./bin/infrasage monitor metrics  # prints Prometheus metrics (requires §6.1 fix)

# Model management
./bin/infrasage model pull        # download/update model
./bin/infrasage model status      # list local models

# Debug mode (verbose logging to stderr)
./bin/infrasage --debug ask "..."
```

### 9.6 CI/CD Secrets (GitHub Repository)

For CI workflows to function, add these repository secrets at  
`Settings → Secrets and variables → Actions`:

| Secret | Required For |
|---|---|
| `AWS_ACCESS_KEY_ID` | drift-check.yml, infrasage-ci.yml terraform steps |
| `AWS_SECRET_ACCESS_KEY` | drift-check.yml, infrasage-ci.yml terraform steps |
| `AWS_REGION` | drift-check.yml (optional; defaults to `us-east-1`) |

`GITHUB_TOKEN` is automatically provided by GitHub Actions.

### 9.7 Makefile Quick Reference

```bash
make build        # go build -o bin/infrasage .
make test         # go test ./... -v -count=1
make lint         # go vet ./... (+ golangci-lint if installed)
make fmt          # go fmt ./...
make tidy         # go mod tidy
make clean        # rm -rf bin/ /tmp/infrasage-scans
make stack-up     # docker compose up -d
make stack-down   # docker compose down
make stack-status # docker compose ps
make model-pull   # ollama pull qwen2.5-coder:3b
make model-status # ollama list
make setup        # one-time full setup
```

---

## 10. Current Status Assessment

### 10.1 Build Status

```
go build ./...  → EXIT 0  ✅  (compiles cleanly)
go vet ./...    → EXIT 0  ✅  (no vet issues)
go test ./...   → EXIT 0  ✅  (all tests pass)
```

Test breakdown:
| Package | Tests | Result |
|---|---|---|
| `internal/terraform` | 11 (ExtractHCL variants) | PASS |
| `internal/llm` | 4 (3 pass; 1 live test skipped — requires Ollama) | PASS |
| `cmd/` | 0 test files | — |
| `internal/scanner` | 0 test files | — |
| `internal/gitops` | 0 test files | — |
| `internal/monitor` | 0 test files | — |

### 10.2 Feature Status Summary

| Feature Category | Status | Demo Blocker? |
|---|---|---|
| LLM HCL generation (`ask`) | ✅ **Works** — requires Ollama running + model pulled | Yes — needs Ollama + model |
| Security scanning (`scan`) | ✅ **Works** — requires Docker stack running | Yes — needs `stack up` |
| Terraform apply (`apply`) | ✅ **Works** — requires terraform + AWS credentials | Yes — needs AWS creds |
| Deploy to GitHub (`deploy`) | ✅ **Works** — requires GITHUB_TOKEN | Yes — needs GITHUB_TOKEN |
| Drift detection (`drift`) | ✅ **Works** — requires terraform + AWS credentials | Yes — needs AWS creds |
| Stack management (`stack`) | ✅ **Works** — requires Docker Desktop | Yes — needs Docker |
| Model management (`model`) | ✅ **Works** — requires Ollama | Yes — needs Ollama |
| Prometheus metrics exposure | ❌ **BROKEN** — `StartServer()` never called | Yes — §6.1 |
| `monitor metrics` command | ❌ **BROKEN** — depends on broken metrics server | Yes — §6.1 |
| `monitor open` command | ✅ **Works** — opens browser (Grafana data absent until §6.1 fixed) | Partial |
| Grafana drift panel | ❌ **BROKEN** — metric name mismatch + RecordDrift() never called | Yes — §6.7, §6.8 |
| Grafana scan duration panel | ❌ **BROKEN** — metric not registered | Yes — §6.7 |
| CI scan pipeline | ✅ **Works** on PR with `.tf` changes | Needs secrets |
| CI drift detection | ✅ **Works** on schedule | Needs AWS secrets |

### 10.3 Demo Blockers

To get a fully working demo of all features:

1. **Ollama must be running natively** with `qwen2.5-coder:3b` pulled
2. **Docker Desktop must be running** and `infrasage stack up` must succeed
3. **AWS credentials** required for `apply`, `drift`, and CI workflows
4. **GITHUB_TOKEN** required for `deploy` command
5. **Fix §6.1** (call `monitor.StartServer()` in `cmd/root.go`) to enable Prometheus metrics exposure
6. **Fix §6.2** (call `monitor.RecordDrift()` in `cmd/drift.go`) for drift metrics
7. **Fix §6.8** (align metric name `infrasage_drift_detected_total` → `infrasage_drift_events_total` or vice versa) for Grafana
8. **Implement §6.7** (`infrasage_scan_duration_seconds` HistogramVec) for Grafana Panel 5

### 10.4 Overall Assessment

The core pipeline — **NLP prompt → LLM → HCL generation → security scan → report** — is correctly and cleanly implemented. The CLI architecture follows the planned design closely: `cmd/` layer purely wires Cobra commands to `internal/` packages; all business logic lives in `internal/`; error messages are human-readable with actionable fix suggestions.

The main gap is in the **observability layer**: the Prometheus metrics server is architecturally complete but never started, making the entire Grafana/Prometheus stack produce no data from the CLI binary. This is a single missing function call. The secondary gaps (RecordDrift, metric name mismatch, scan duration histogram) are similarly small in scope.

The Go codebase is well-structured, passes `go vet`, and all unit tests pass. It is a solid foundation for the described DevSecOps demonstration project.
