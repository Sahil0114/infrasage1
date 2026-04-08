# ARCHITECTURE.md — InfraSage System Architecture

> Read this before creating any file or folder.
> Every decision here was made deliberately for the M2 / 8GB constraint.

---

## System Overview

```
USER TYPES:   infrasage ask "create an S3 bucket"
                          │
                          ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                    Go Binary  (infrasage)                               │
│                                                                         │
│  cmd/           ← Cobra CLI layer. One file per command.               │
│  internal/      ← All business logic. No direct import from cmd/.      │
│  └─ llm/        ← HTTP client to Ollama                                │
│  └─ terraform/  ← exec wrapper around terraform CLI                    │
│  └─ scanner/    ← docker exec into scanner containers                  │
│  └─ gitops/     ← git + GitHub API for deploy command                  │
│  └─ monitor/    ← Prometheus metric registration and push              │
└────────┬──────────────────────────────────────────────────────────────-┘
         │
         │ HTTP POST /api/generate
         ▼
┌─────────────────────────┐
│  Ollama  (NATIVE macOS) │   ← NOT in Docker. Uses Apple Metal GPU.
│  Port: 11434            │   ← Model: qwen2.5-coder:3b (base, no fine-tune)
│  Model file: ~/.ollama  │   ← Managed by Ollama, never committed to repo
└─────────────────────────┘

         │ docker exec
         ▼
┌─────────────────────────────────────────────────────────────────────────┐
│               Docker Compose  (deploy/docker-compose.yml)               │
│                                                                         │
│  ┌─────────────┐   ┌─────────────┐   ┌─────────────────┐              │
│  │   checkov   │   │    tfsec    │   │   terrascan     │              │
│  │  container  │   │  container  │   │   container     │              │
│  │             │   │             │   │                 │              │
│  └──────┬──────┘   └──────┬──────┘   └────────┬────────┘              │
│         └─────────────────┴──────────────────┘                         │
│                            │ shared volume                              │
│                    /tmp/infrasage-scans  (host path)                   │
│                                                                         │
│  ┌──────────────────────┐   ┌──────────────────────────────────────┐  │
│  │     Prometheus       │   │              Grafana                  │  │
│  │     Port: 9090       │◄──│           Port: 3000                  │  │
│  │  scrapes :2112/metrics│  │   Dashboard: infrasage.json           │  │
│  └──────────────────────┘   └──────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────┘

         │ git push + GitHub API
         ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                  GitHub  (Cloud — zero local overhead)                  │
│                                                                         │
│  Push .tf file to branch                                                │
│         │                                                               │
│         ▼                                                               │
│  GitHub Actions triggers (.github/workflows/infrasage-ci.yml)          │
│    ├── Checkov Action  (re-scans in CI)                                │
│    ├── tfsec Action                                                     │
│    ├── Terrascan Action                                                 │
│    └── Terraform dry-run simulation comments on the PR                  │
│                                                                         │
│  Drift check cron (.github/workflows/drift-check.yml)                  │
│    Runs every 6 hours → if drift found → auto-creates new PR           │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## Repository Layout — Every File Explained

The agent must create files in exactly these locations. Do not deviate.

```
infrasage/
│
├── CLAUDE.md              ← Agent master instructions (already exists)
│
├── main.go                ← Entry point. One line: cmd.Execute()
│
├── cmd/                   ← Cobra command definitions only.
│   │                         No business logic here.
│   │                         Each file = one top-level command.
│   │
│   ├── root.go            ← Registers all subcommands. Loads .env.
│   │                         Defines global flags (--debug, --config).
│   │
│   ├── ask.go             ← infrasage ask "..."
│   │                         Calls: llm.Generate → terraform.ExtractHCL
│   │                                → scanner.RunAll → monitor.RecordGeneration
│   │
│   ├── scan.go            ← infrasage scan <file>
│   │                         Calls: scanner.RunAll → report.Print
│   │
│   ├── apply.go           ← infrasage apply <file>
│   │                         Calls: terraform.Validate → terraform.Apply
│   │
│   ├── deploy.go          ← infrasage deploy <file>
│   │                         Calls: gitops.CommitAndPush → gitops.CreatePR
│   │
│   ├── drift.go           ← infrasage drift
│   │                         Calls: terraform.Plan (exit code 2 = drift)
│   │
│   ├── stack.go           ← infrasage stack up|down|status
│   │                         Calls: exec docker compose commands
│   │                         Also checks/starts ollama serve
│   │
│   ├── monitor.go         ← infrasage monitor open|metrics
│   │                         open: exec open http://localhost:3000
│   │                         metrics: curl localhost:2112/metrics
│   │
│   └── model.go           ← infrasage model pull|status
│                              pull: ollama pull <model from env>
│                              status: ollama list
│
├── internal/              ← All business logic. Pure Go, no exec in here
│   │                         except terraform/ and scanner/ which must shell out.
│   │
│   ├── llm/
│   │   └── client.go      ← OllamaClient struct.
│   │                         Methods: Generate(system, user string) (string, error)
│   │                                  IsHealthy() bool
│   │                         Transport: net/http POST to :11434/api/generate
│   │
│   ├── terraform/
│   │   ├── parser.go      ← ExtractHCL(raw string) string
│   │   │                     Strips ```hcl fences from LLM output.
│   │   │                     Returns empty string if no valid HCL found.
│   │   │
│   │   └── wrapper.go     ← Validate(dir string) error
│   │                         Plan(dir string) (exitCode int, err error)
│   │                         Apply(dir string) error
│   │                         All use os/exec to shell out to terraform binary.
│   │
│   ├── scanner/
│   │   ├── runner.go      ← RunAll(tfFile string) (*Report, error)
│   │   │                     Runs checkov, tfsec, terrascan in parallel (goroutines)
│   │   │                     Collects results into Report struct.
│   │   │
│   │   ├── checkov.go     ← Run(tfFile string) (ScanResult, error)
│   │   │                     Copies file to /tmp/infrasage-scans/
│   │   │                     Runs: docker exec infrasage-checkov checkov -f /scans/<file> --output json
│   │   │                     Parses JSON → ScanResult
│   │   │
│   │   ├── tfsec.go       ← Same pattern as checkov.go
│   │   │                     Command: docker exec infrasage-tfsec tfsec /scans/<file> --format json
│   │   │
│   │   ├── terrascan.go   ← Same pattern as checkov.go
│   │   │                     Command: docker exec infrasage-terrascan terrascan scan -i terraform -f /scans/<file> -o json
│   │   │
│   │   └── report.go      ← Report struct with Print() method.
│   │                         Draws the table to stdout.
│   │                         HasCritical() bool method used by scan command.
│   │
│   ├── gitops/
│   │   ├── git.go         ← CommitAndPush(file, branch, message string) error
│   │   │                     Uses os/exec git commands.
│   │   │                     Creates branch, stages file, commits, pushes.
│   │   │
│   │   └── github.go      ← CreatePR(branch, title, body string) (string, error)
│   │                         Uses net/http to call GitHub REST API v3.
│   │                         Requires GITHUB_TOKEN env var.
│   │                         Returns the PR URL.
│   │
│   └── monitor/
│       └── metrics.go     ← Registers all Prometheus metrics at package init.
│                             StartServer(port string) — runs :2112/metrics in background goroutine.
│                             RecordGeneration(status string)
│                             RecordScan(report *scanner.Report)
│                             RecordModelLatency(seconds float64)
│                             RecordDrift(resourceType string)
│
├── deploy/
│   ├── docker-compose.yml ← Defines: checkov, tfsec, terrascan, prometheus, grafana
│   ├── prometheus.yml     ← Scrape config pointing to host.docker.internal:2112
│   └── grafana/
│       ├── provisioning/
│       │   ├── datasources/prometheus.yml
│       │   └── dashboards/dashboard.yml
│       └── dashboards/
│           └── infrasage.json  ← Grafana dashboard definition
│
├── examples/
│   ├── s3-bucket.tf       ← Example for scanner testing
│   └── ec2-instance.tf    ← Example for scanner testing
│
├── .github/
│   └── workflows/
│       ├── infrasage-ci.yml   ← Scan + plan on PR
│       └── drift-check.yml    ← Scheduled drift detection
│
├── .env.example           ← Template. User copies to .env and fills in values.
├── .gitignore
└── go.mod
```

**What is NOT in the repo:**
- No `models/` directory — Ollama manages model files in `~/.ollama/models/`
- No `fine_tune/` directory — fine-tuning is not part of this project
- No `.env` — user creates this from `.env.example`

---

## Data Flow — Exactly What Happens When User Runs `infrasage ask`

```
Step 1: User runs: ./bin/infrasage ask "create an S3 bucket with versioning"

Step 2: cmd/ask.go's RunE function is called
        → args[0] = "create an S3 bucket with versioning"
        → Reads env vars: INFRASAGE_OLLAMA_URL, INFRASAGE_OLLAMA_MODEL

Step 3: cmd/ask.go calls internal/llm.IsHealthy()
        → HTTP GET http://localhost:11434/api/tags
        → If 200 OK: continue
        → If error: print human-readable error and exit 1

Step 4: cmd/ask.go calls internal/llm.Generate(systemPrompt, userPrompt)
        → Builds JSON body:
            { model, prompt, system, stream: false, options: {temperature: 0.1} }
        → HTTP POST http://localhost:11434/api/generate
        → Waits up to 120 seconds for response
        → Decodes: { "response": "terraform {\n  required_providers ..." }
        → Returns the response string

Step 5: cmd/ask.go calls internal/terraform.ExtractHCL(rawResponse)
        → Checks if response contains ```hcl ... ``` fence → strips it
        → Checks if response looks like HCL (contains "terraform {" or "resource \"")
        → Returns clean HCL string, or "" if nothing found

Step 6: cmd/ask.go writes HCL to file (default: infra.tf)
        → os.WriteFile("infra.tf", []byte(hcl), 0644)

Step 7: cmd/ask.go calls internal/scanner.RunAll("infra.tf")
        → Copies infra.tf to /tmp/infrasage-scans/infra.tf
        → Launches 3 goroutines simultaneously:
            goroutine 1: docker exec infrasage-checkov checkov -f /scans/infra.tf --output json
            goroutine 2: docker exec infrasage-tfsec tfsec /scans/infra.tf --format json
            goroutine 3: docker exec infrasage-terrascan terrascan scan -i terraform -f /scans/infra.tf -o json
        → Each goroutine parses its tool's JSON and returns ScanResult
        → All 3 results collected into *Report

Step 8: Report.Print() draws the table to stdout

Step 9: internal/monitor records metrics:
        → infrasage_iac_generations_total{status="success"} += 1
        → infrasage_model_latency_seconds.Observe(elapsed)
        → infrasage_scan_findings{scanner="checkov", severity="HIGH"} = 2
        → etc.

Done. User sees: generated HCL + security report.
```

---

## Key Structs — The Agent Must Use These Exactly

These are the contracts between packages. Do not change field names once established.

```go
// internal/llm/client.go
type OllamaClient struct {
    BaseURL string        // e.g. "http://localhost:11434"
    Model   string        // e.g. "qwen2.5-coder:3b"
    Timeout time.Duration // 120s default
}

// internal/scanner/report.go
type Finding struct {
    ID       string  // CKV_AWS_18, AVD-AWS-0086, etc.
    Severity string  // CRITICAL, HIGH, MEDIUM, LOW
    Message  string  // Human readable description
    Resource string  // aws_s3_bucket.my_bucket
    Line     int     // Line number in .tf file
}

type ScanResult struct {
    Tool     string        // "checkov", "tfsec", "terrascan"
    Passed   int
    Failed   int
    Findings []Finding
    Duration time.Duration
    Error    error         // nil if scan succeeded
}

type Report struct {
    File    string
    Results []ScanResult
}

// Report methods:
// Print() — draws the table to stdout
// HasCritical() bool — true if any finding is CRITICAL severity
// TotalPassed() int
// TotalFailed() int
```

---

## RAM Budget — Keep This In Mind At All Times

| Component | RAM Usage | Notes |
|---|---|---|
| macOS + background | ~2.0 GB | Baseline, unavoidable |
| Ollama + Qwen2.5-3B Q4 | ~2.0 GB | Metal-accelerated, very efficient |
| Docker Desktop | ~0.5 GB | Overhead for the VM |
| Checkov container | ~0.1 GB | Only spikes during scan, returns to near-zero |
| tfsec container | ~0.05 GB | Tiny Go binary |
| Terrascan container | ~0.05 GB | Tiny Go binary |
| Prometheus container | ~0.15 GB | Stable |
| Grafana container | ~0.20 GB | Stable |
| CLI binary running | ~0.01 GB | Negligible |
| **Total estimated** | **~5.05 GB** | **~3 GB headroom on 8GB** |

If RAM pressure becomes an issue:
1. First reduce Docker Desktop memory limit to 3GB in Docker Desktop → Settings → Resources
2. If still tight, stop Grafana when not needed: `docker compose stop grafana`
3. Never reduce Ollama — it needs its full allocation for inference

---

## Approved Docker Images (ARM64 Verified)

These images are confirmed to have `linux/arm64` support.
Do not substitute without checking ARM64 compatibility first.

| Service | Image | Verified ARM64 |
|---|---|---|
| Checkov | `bridgecrew/checkov:latest` | ✅ |
| tfsec | `aquasec/tfsec:latest` | ✅ |
| Terrascan | `accurics/terrascan:latest` | ✅ |
| Prometheus | `prom/prometheus:latest` | ✅ |
| Grafana | `grafana/grafana:latest` | ✅ |

All containers in docker-compose.yml must include:
```yaml
platform: linux/arm64
```

---

## Network — How Services Talk To Each Other

```
Host machine (macOS)
│
├── :11434  ← Ollama (native process, not in container)
│              The Go binary calls this directly.
│
├── :2112   ← Go binary's Prometheus metrics HTTP server
│              Prometheus container scrapes this.
│              This is a background goroutine started by the CLI at startup.
│
├── :9090   ← Prometheus (in Docker, published to host)
│
└── :3000   ← Grafana (in Docker, published to host)

Docker internal network: infrasage-net
  All containers can reach each other by service name.
  Prometheus calls http://host.docker.internal:2112/metrics
  (host.docker.internal resolves to the Mac's host IP from inside Docker)
```

---

## The System Prompt — This Is The Most Important String In The Project

The system prompt sent with every LLM call determines output quality.
It must be stored as a constant in `cmd/ask.go`. Do not hard-code it elsewhere.
Do not let the user override it (Phase 1). Future phase may add `--prompt-file` flag.

```
You are an expert Terraform engineer. Your only job is to output valid Terraform HCL code.

Rules you must follow without exception:
1. Output ONLY raw HCL code. No markdown. No explanations. No commentary.
2. Start your response with the first line of HCL code.
3. Always include a terraform { required_providers { ... } } block.
4. Always include required_version = ">= 1.6" inside the terraform block.
5. Use the AWS provider unless the user explicitly says otherwise.
6. Follow these security defaults:
   - S3 buckets: block_public_acls = true, block_public_policy = true,
                 ignore_public_acls = true, restrict_public_buckets = true
   - S3 buckets: enable server_side_encryption_configuration
   - S3 buckets: enable versioning
   - Security groups: no ingress 0.0.0.0/0 on port 22 unless asked
   - IAM: no wildcard (*) actions or resources unless asked
7. Add these tags to every resource:
   tags = { Project = "infrasage", ManagedBy = "terraform" }
8. Use snake_case for all resource and variable names.
9. The AWS provider version must be ~> 5.0.
10. Do not use deprecated attributes.
```
