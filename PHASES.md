# PHASES.md — Step-by-Step Build Instructions

> Each step has exactly one deliverable and one test command.
> Do not move to the next step until the test passes.
> Steps within a phase are strictly ordered. Phases are sequential.

---

## Phase 1 — Project Scaffold + Model + First Working Command

**Goal at end of Phase 1:** `infrasage ask "create an S3 bucket"` generates a valid `infra.tf` file.

---

### Step 1.1 — Go Module Initialisation

**What to create:**
- `go.mod` with module name `github.com/YOURUSERNAME/infrasage` (replace YOURUSERNAME with actual GitHub username)
- `main.go` — single file, calls `cmd.Execute()`

**Exact module dependencies to add:**
```
go get github.com/spf13/cobra@latest
go get github.com/joho/godotenv@latest
go get github.com/prometheus/client_golang@latest
```

**Test:** `go build ./...` produces no errors.

---

### Step 1.2 — Root Command

**What to create:** `cmd/root.go`

**What it must do:**
- Define `rootCmd` as a `*cobra.Command`
- Set `Use: "infrasage"`, `Short: "AI-powered DevSecOps CLI"`, `Version: "0.1.0"`
- In `init()`: call `godotenv.Load()` — silently ignore error if `.env` not found
- Export `Execute()` function called by `main.go`
- Do NOT register subcommands yet — each cmd file registers itself

**Test:** `go run . --version` prints `infrasage version 0.1.0`

---

### Step 1.3 — Environment File

**What to create:**
- `.env.example` with all variables shown in `CONVENTIONS.md` env section
- `.gitignore` that ignores `.env`, `bin/`, `.terraform/`, `*.tfstate`, `*.tfstate.backup`

**What NOT to create:** Do not create `.env` itself — the user must copy `.env.example` to `.env` and fill in their values.

**Test:** `cat .gitignore | grep ".env"` shows the entry exists.

---

### Step 1.4 — Ollama Setup (Manual, Not Code)

This step is done by the human, not the agent. Document it in a comment at the top of `cmd/stack.go`:

```
// HUMAN SETUP REQUIRED (one-time, before running this binary):
// 1. brew install ollama
// 2. brew install terraform
// 3. ollama pull qwen2.5-coder:3b
// 4. ollama serve   (keep this terminal open, or it auto-starts as a service)
```

**Test (human runs):** `curl http://localhost:11434/api/tags` returns JSON with a models list.

---

### Step 1.5 — LLM Client Package

**What to create:** `internal/llm/client.go`

**Exact struct to define:**
```go
type OllamaClient struct {
    BaseURL string
    Model   string
    Timeout time.Duration
}
```

**Exact methods to implement:**

`NewOllamaClient(baseURL, model string) *OllamaClient`
- Sets Timeout to 120 seconds

`Generate(systemPrompt, userPrompt string) (string, error)`
- Builds the request body as a Go map (not a separate struct — keep it simple)
- The JSON shape must be:
  ```json
  {
    "model": "<model>",
    "prompt": "<userPrompt>",
    "system": "<systemPrompt>",
    "stream": false,
    "options": { "temperature": 0.1, "num_ctx": 4096 }
  }
  ```
- HTTP POST to `<BaseURL>/api/generate`
- Use `net/http` only — no third-party HTTP libraries
- Decode response, extract `response` field
- Return error if status != 200 or response is empty

`IsHealthy() bool`
- HTTP GET to `<BaseURL>/api/tags`
- Timeout 3 seconds
- Returns true only if status == 200

**Test:** Write a small `_test.go` that calls `IsHealthy()` — it should return `true` when Ollama is running. Run: `go test ./internal/llm/...`

---

### Step 1.6 — Terraform HCL Parser

**What to create:** `internal/terraform/parser.go`

**Exact function to implement:**

`ExtractHCL(raw string) string`
- Trim whitespace from raw input
- Check for presence of markdown code fences using regexp:
  Pattern: `` (?s)```(?:hcl|terraform|tf)?\n?(.*?)``` ``
- If fence found: return the captured group (the content inside the fences), trimmed
- If no fence found: check if raw contains `terraform {` OR `resource "` OR `provider "`
- If it looks like HCL: return raw trimmed
- Otherwise: return empty string `""`

**Critical detail:** The LLM sometimes adds ` ```hcl ` fences, sometimes ` ``` ` fences, sometimes no fences. The parser must handle all three cases.

**Test:**
```go
// in parser_test.go
input1 := "```hcl\nterraform {\n}\n```"           // should return "terraform {\n}"
input2 := "terraform {\n  required_providers {}\n}" // should return same
input3 := "Sure, here is your code: bla bla"        // should return ""
```

---

### Step 1.7 — Ask Command

**What to create:** `cmd/ask.go`

**Flags to define:**
- `--out` / `-o` (string, default `"infra.tf"`) — output filename
- `--no-scan` (bool, default false) — skip scanner after generation

**What the RunE function must do, in this exact order:**
1. Create `OllamaClient` using env vars `INFRASAGE_OLLAMA_URL` and `INFRASAGE_OLLAMA_MODEL`
2. Call `client.IsHealthy()`. If false: return a readable error telling user to run `ollama serve`
3. Print `⏳ Generating Terraform HCL...` to stdout
4. Record start time with `time.Now()`
5. Call `client.Generate(systemPrompt, args[0])` — the system prompt is the constant defined in `ARCHITECTURE.md`
6. If error: return readable error
7. Call `terraform.ExtractHCL(response)`
8. If empty string returned: return error `"model did not return valid HCL — try rephrasing your prompt"`
9. Write HCL to the `--out` file using `os.WriteFile`
10. Print the elapsed time and filename
11. Print the generated HCL to stdout (so user can see it)
12. If `--no-scan` is false: call `scanner.RunAll(outFile)` and print the report
13. Call `monitor.RecordGeneration("success")` and `monitor.RecordModelLatency(elapsed.Seconds())`

**The system prompt** must be stored as an unexported package-level constant `const systemPrompt = \`...\`` directly in `cmd/ask.go`. Use the exact text from `ARCHITECTURE.md`.

**Test:**
```bash
go run . ask "create an S3 bucket" --no-scan
# Must: print timing, write infra.tf, print HCL content
cat infra.tf | head -5
# Must: start with "terraform {" or "resource "
```

---

### Step 1.8 — Stack Command (Phase 1 version — starts containers)

**What to create:** `cmd/stack.go`

**Subcommands to register:** `up`, `down`, `status`

**What `stack up` must do, in this exact order:**
1. Create `/tmp/infrasage-scans` directory if it doesn't exist (`os.MkdirAll`)
2. Check if `ollama` is in PATH using `exec.LookPath("ollama")`
   - If not found: print warning with install instructions, continue (don't exit)
3. Check if Ollama is already running by calling `curl -sf http://localhost:11434/api/tags` via exec
   - If not running: run `ollama serve` as a background process (`cmd.Start()` not `cmd.Run()`)
   - Print the PID: `"🤖 Ollama started (PID: XXXX)"`
   - If already running: print `"✅ Ollama already running"`
4. Run `docker compose -f ./deploy/docker-compose.yml up -d`
   - Connect stdout/stderr of this command to os.Stdout/os.Stderr so user sees output
5. Print success message with URLs:
   ```
   ✅ InfraSage stack is up!
      Prometheus: http://localhost:9090
      Grafana:    http://localhost:3000  (admin / infrasage)
      Ollama:     http://localhost:11434
   ```

**What `stack down` must do:**
- Run `docker compose -f ./deploy/docker-compose.yml down`

**What `stack status` must do:**
- Run `docker compose -f ./deploy/docker-compose.yml ps`

**Test (Phase 1 — docker-compose.yml doesn't exist yet, so skip docker step):**
```bash
go run . stack --help
go run . stack up --help
```
Full test happens in Step 1.10.

---

### Step 1.9 — Docker Compose File

**What to create:** `deploy/docker-compose.yml`

**Services to define:**

`checkov`:
- Image: `bridgecrew/checkov:latest`
- Container name: `infrasage-checkov`
- Platform: `linux/arm64`
- Volumes: `/tmp/infrasage-scans:/scans`
- Entrypoint: `["tail", "-f", "/dev/null"]`  ← keeps container alive between scans
- Restart: `unless-stopped`

`tfsec`:
- Image: `aquasec/tfsec:latest`
- Container name: `infrasage-tfsec`
- Platform: `linux/arm64`
- Volumes: `/tmp/infrasage-scans:/scans`
- Entrypoint: `["tail", "-f", "/dev/null"]`
- Restart: `unless-stopped`

`terrascan`:
- Image: `accurics/terrascan:latest`
- Container name: `infrasage-terrascan`
- Platform: `linux/arm64`
- Volumes: `/tmp/infrasage-scans:/scans`
- Entrypoint: `["tail", "-f", "/dev/null"]`
- Restart: `unless-stopped`

`prometheus`:
- Image: `prom/prometheus:latest`
- Container name: `infrasage-prometheus`
- Platform: `linux/arm64`
- Ports: `9090:9090`
- Volumes: `./prometheus.yml:/etc/prometheus/prometheus.yml`
- Restart: `unless-stopped`

`grafana`:
- Image: `grafana/grafana:latest`
- Container name: `infrasage-grafana`
- Platform: `linux/arm64`
- Ports: `3000:3000`
- Environment:
  - `GF_SECURITY_ADMIN_PASSWORD=infrasage`
  - `GF_AUTH_ANONYMOUS_ENABLED=true`
  - `GF_AUTH_ANONYMOUS_ORG_ROLE=Viewer`
- Volumes:
  - `./grafana/dashboards:/var/lib/grafana/dashboards`
  - `./grafana/provisioning:/etc/grafana/provisioning`
- Depends on: `prometheus`
- Restart: `unless-stopped`

**Network:** Define one network at the bottom called `infrasage-net`. Attach all services to it.

**Test:**
```bash
cd deploy && docker compose up -d
docker compose ps
# All 5 containers should show status "Up"
docker compose down
```

---

### Step 1.10 — End-to-End Phase 1 Test

Run all of these in order. All must pass before Phase 2.

```bash
# 1. Build the binary
go build -o bin/infrasage .

# 2. Check binary works
./bin/infrasage --version
./bin/infrasage --help

# 3. Start the stack
./bin/infrasage stack up

# 4. Verify containers
./bin/infrasage stack status

# 5. Generate HCL (skip scan for now — scanner containers need Phase 2)
./bin/infrasage ask "create an S3 bucket with versioning" --no-scan

# 6. Check the output file
cat infra.tf

# 7. Verify it's valid HCL structure (not empty, starts correctly)
head -3 infra.tf
```

Expected output of step 5 includes timing like `(12.3s)` and the HCL content.
Expected output of step 6 starts with `terraform {`.

---

## Phase 2 — Security Scanner Integration

**Goal at end of Phase 2:** `infrasage scan infra.tf` runs all three scanners and prints a unified table.

---

### Step 2.1 — Scanner Types (Shared Types)

**What to create:** `internal/scanner/types.go`

Define these types (no methods yet):
```go
type Finding struct {
    ID       string
    Severity string
    Message  string
    Resource string
    Line     int
}

type ScanResult struct {
    Tool     string
    Passed   int
    Failed   int
    Findings []Finding
    Duration time.Duration
    Error    error
}

type Report struct {
    File    string
    Results []ScanResult
}
```

**Test:** `go build ./internal/scanner/...` compiles.

---

### Step 2.2 — Shared Scanner Helper

**What to create:** `internal/scanner/docker.go`

**Function to implement:**

`copyToScanDir(tfFile string) (string, error)`
- Reads the file at `tfFile`
- Gets the scan dir from env `INFRASAGE_SCAN_DIR`, default `/tmp/infrasage-scans`
- Creates the scan dir if it doesn't exist
- Copies the file into the scan dir, keeping the same filename
- Returns the path inside the scan dir (e.g., `/tmp/infrasage-scans/infra.tf`)

`dockerExec(containerName string, args ...string) ([]byte, error)`
- Builds a command: `docker exec <containerName> <args...>`
- Runs it and captures stdout
- Returns stdout bytes and error
- Timeout: 60 seconds (use context with cancel)

**Test:** None yet — tested in subsequent steps.

---

### Step 2.3 — Checkov Integration

**What to create:** `internal/scanner/checkov.go`

**Function to implement:**

`RunCheckov(tfFile string) ScanResult`
- Call `copyToScanDir(tfFile)` to get the in-container path
- The in-container path replaces `/tmp/infrasage-scans` with `/scans` (because that's the volume mount)
- Run: `docker exec infrasage-checkov checkov -f /scans/<filename> --output json --quiet`
- Parse the JSON output

**Checkov JSON output shape to parse:**
```json
{
  "results": {
    "passed_checks": [ { "check_id": "...", "resource": "..." } ],
    "failed_checks": [
      {
        "check_id": "CKV_AWS_18",
        "check_result": { "result": "FAILED" },
        "resource": "aws_s3_bucket.my_bucket",
        "file_line_range": [10, 20]
      }
    ]
  }
}
```
- `Passed` = `len(results.passed_checks)`
- `Failed` = `len(results.failed_checks)`
- For each failed check: create a `Finding` with:
  - `ID` = `check_id`
  - `Severity` = look up severity from a hardcoded map (CKV_AWS_18 → HIGH, etc. — use "MEDIUM" as default if unknown)
  - `Message` = `check_type` or `check_id` if no message available
  - `Resource` = `resource`
  - `Line` = first element of `file_line_range`

**If docker exec fails** (container not running): return `ScanResult{Tool: "checkov", Error: err}`

**Test:**
```bash
# Ensure stack is up, then:
go test ./internal/scanner/... -run TestRunCheckov -v
```

---

### Step 2.4 — tfsec Integration

**What to create:** `internal/scanner/tfsec.go`

**Function to implement:** `RunTfsec(tfFile string) ScanResult`

**Command to run:**
```
docker exec infrasage-tfsec tfsec /scans/<filename> --format json --no-colour
```

**tfsec JSON output shape:**
```json
{
  "results": [
    {
      "rule_id": "AVD-AWS-0086",
      "severity": "HIGH",
      "description": "Bucket does not have MFA delete enabled.",
      "location": { "filename": "...", "start_line": 5 },
      "affected_resource": "aws_s3_bucket.example"
    }
  ]
}
```
- `Passed` cannot be determined from tfsec JSON (it only reports failures) — set to `0`
- `Failed` = `len(results)`
- Map each result to a `Finding` directly

**Test:** Same as Step 2.3 but run the tfsec test.

---

### Step 2.5 — Terrascan Integration

**What to create:** `internal/scanner/terrascan.go`

**Function to implement:** `RunTerrascan(tfFile string) ScanResult`

**Command to run:**
```
docker exec infrasage-terrascan terrascan scan -i terraform -f /scans/<filename> -o json
```

**Terrascan JSON output shape:**
```json
{
  "results": {
    "violations": [
      {
        "rule_id": "AC_AWS_0207",
        "severity": "MEDIUM",
        "description": "...",
        "line": 10,
        "resource_name": "aws_s3_bucket.example"
      }
    ]
  }
}
```
- Map `violations` to `Finding` list
- `Failed` = `len(violations)`
- `Passed` = 0 (terrascan also only reports failures)

**Test:** Same pattern.

---

### Step 2.6 — Parallel Runner

**What to create:** `internal/scanner/runner.go`

**Function to implement:** `RunAll(tfFile string) (*Report, error)`

**Exact logic:**
1. Create a `Report{File: tfFile}`
2. Create a channel `results := make(chan ScanResult, 3)`
3. Launch 3 goroutines:
   - `go func() { results <- RunCheckov(tfFile) }()`
   - `go func() { results <- RunTfsec(tfFile) }()`
   - `go func() { results <- RunTerrascan(tfFile) }()`
4. Collect all 3 results from the channel in a for loop
5. Attach to `Report.Results`
6. Return `&report, nil`

**Why parallel:** All three scanners are independent. Running them sequentially wastes time. Parallel reduces total scan time from ~30s to ~10s.

**Test:**
```bash
./bin/infrasage scan examples/s3-bucket.tf
# Must show table with results from all 3 scanners
```

---

### Step 2.7 — Unified Report Printer

**What to create:** `internal/scanner/report.go`

**Methods to implement on `*Report`:**

`Print()` — writes to os.Stdout this exact format:
```
╔════════════════════════════════════════════╗
║      InfraSage Security Scan Report        ║
╠════════════════════════════════════════════╣
║ File: infra.tf                             ║
╠══════════╦════════╦════════╦══════════════╣
║ Scanner  ║  Pass  ║  Fail  ║   Status     ║
╠══════════╬════════╬════════╬══════════════╣
║ Checkov  ║   12   ║    2   ║ ⚠  WARN     ║
║ tfsec    ║    0   ║    0   ║ ✓  PASS     ║
║ Terrascan║   0    ║    1   ║ ⚠  WARN     ║
╠══════════╩════════╩════════╩══════════════╣
║ FINDINGS:                                  ║
║ [CKV_AWS_18]  HIGH    S3 access logging   ║
║ [AC_AWS_0207] MEDIUM  No MFA delete       ║
╚════════════════════════════════════════════╝
```

Status logic:
- `✓  PASS` if `Failed == 0`
- `⚠  WARN` if `Failed > 0` but no CRITICAL findings
- `✗  FAIL` if any CRITICAL findings

`HasCritical() bool` — returns true if any Finding.Severity == "CRITICAL"

`TotalPassed() int` — sum of all Result.Passed

`TotalFailed() int` — sum of all Result.Failed

**Test:**
```bash
./bin/infrasage scan examples/s3-bucket.tf
# Visually verify table renders correctly
```

---

### Step 2.8 — Wire Scan Into Ask Command

**What to modify:** `cmd/ask.go`

At the end of `runAsk()`, add:
```go
if !askNoScan {
    fmt.Printf("\n🔍 Auto-scanning %s...\n", outFile)
    report, err := scanner.RunAll(outFile)
    if err != nil {
        fmt.Fprintf(os.Stderr, "⚠️  Scan error (non-fatal): %v\n", err)
    } else {
        report.Print()
        monitor.RecordScan(report)
    }
}
```

**Test:**
```bash
./bin/infrasage ask "create an insecure S3 bucket with public access"
# Must: generate HCL, then automatically show scan report
# The insecure prompt should trigger checkov warnings
```

---

## Phase 3 — CI/CD Integration

**Goal at end of Phase 3:** `infrasage deploy infra.tf` creates a GitHub PR, Digger comments the plan.

---

### Step 3.1 — GitHub Actions CI Workflow

**What to create:** `.github/workflows/infrasage-ci.yml`

**Trigger conditions:**
- `on: push` to `main` branch, but only when files matching `**.tf` changed
- `on: pull_request` to `main`, only for `**.tf` files

**Jobs to define — run sequentially in this order:**

Job 1 `scan`:
- Runner: `ubuntu-latest`
- Steps:
  1. `actions/checkout@v4`
  2. Checkov: use `bridgecrewio/checkov-action@master` with `file: infra.tf`
  3. tfsec: use `aquasecurity/tfsec-action@v1.0.0` with `working_directory: .`
  4. Terrascan: use `accurics/terrascan-action@main` with `iac_type: terraform`

Job 2 `plan`:
- Runner: `ubuntu-latest`
- `needs: [scan]` — only runs if scan passes
- Steps:
  1. `actions/checkout@v4`
  2. `hashicorp/setup-terraform@v3`
  3. Digger action: `diggerhq/digger@v0.3.0`
     - Needs secrets: `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `GITHUB_TOKEN`

**Important:** The workflow must not have `aws_access_key_id` or any secret hardcoded. Use `${{ secrets.AWS_ACCESS_KEY_ID }}` syntax only.

---

### Step 3.2 — Digger Configuration File

**What to create:** `digger.yml` in repo root

```yaml
projects:
  - name: infrasage
    dir: .
    workflow: default
    workspace: default
    apply_after_merge: true
    generate_projects:
      files_changed:
        patterns:
          - "**/*.tf"
```

This tells Digger: run terraform plan on PR, run apply on merge.

---

### Step 3.3 — Drift Detection Workflow

**What to create:** `.github/workflows/drift-check.yml`

**Trigger:** `schedule: cron: "0 */6 * * *"` (every 6 hours)

**What the job must do:**
1. `actions/checkout@v4`
2. `hashicorp/setup-terraform@v3` with `aws-access-key-id` and `aws-secret-access-key` from secrets
3. `terraform init`
4. `terraform plan -detailed-exitcode`
   - Exit code 0: no changes (no drift) — job succeeds
   - Exit code 1: terraform error — job fails
   - Exit code 2: drift detected
5. If exit code 2: use `peter-evans/create-pull-request@v5` action to auto-create a PR titled `"chore: drift detected — remediation required"`

---

### Step 3.4 — Deploy Command (Go)

**What to create:**
- `internal/gitops/git.go`
- `internal/gitops/github.go`
- `cmd/deploy.go`

**`internal/gitops/git.go` functions:**

`CreateBranch(branchName string) error`
- Runs: `git checkout -b <branchName>`
- Branch naming: `infrasage/deploy-<sanitized-filename>-<timestamp>`

`StageAndCommit(file, message string) error`
- Runs: `git add <file>`, then `git commit -m <message>`

`Push(branchName string) error`
- Runs: `git push -u origin <branchName>`

**`internal/gitops/github.go` functions:**

`CreatePR(branch, title, body string) (string, error)`
- Uses `net/http` to POST to `https://api.github.com/repos/<GITHUB_REPO>/pulls`
- Auth header: `Authorization: token <GITHUB_TOKEN>`
- Returns the HTML URL of the created PR

**`cmd/deploy.go` RunE:**
1. Scan the file first (abort if CRITICAL findings)
2. Call git functions in order
3. Call CreatePR
4. Print the PR URL
5. Print: "Digger will comment the terraform plan on your PR. Merge to apply."

---

## Phase 4 — Monitoring

**Goal at end of Phase 4:** Grafana shows live metrics after running `infrasage ask` a few times.

---

### Step 4.1 — Metrics Package

**What to create:** `internal/monitor/metrics.go`

**Metrics to register (use `prometheus.MustRegister` in package `init()`):**

```go
IaCGenerationsTotal = prometheus.NewCounterVec(
    prometheus.CounterOpts{
        Name: "infrasage_iac_generations_total",
        Help: "Total number of IaC generation requests",
    },
    []string{"status"},   // labels: "success" or "failed"
)

ScanFindings = prometheus.NewGaugeVec(
    prometheus.GaugeOpts{
        Name: "infrasage_scan_findings",
        Help: "Current count of security findings by scanner and severity",
    },
    []string{"scanner", "severity"},
)

ModelLatencySeconds = prometheus.NewHistogram(
    prometheus.HistogramOpts{
        Name:    "infrasage_model_latency_seconds",
        Help:    "LLM inference latency in seconds",
        Buckets: []float64{1, 5, 10, 20, 30, 60, 90, 120},
    },
)

DriftEventsTotal = prometheus.NewCounterVec(
    prometheus.CounterOpts{
        Name: "infrasage_drift_events_total",
        Help: "Total infrastructure drift detection events",
    },
    []string{"resource_type"},
)

ScanDurationSeconds = prometheus.NewHistogramVec(
    prometheus.HistogramOpts{
        Name:    "infrasage_scan_duration_seconds",
        Help:    "Time taken per security scanner",
        Buckets: prometheus.DefBuckets,
    },
    []string{"scanner"},
)
```

**Functions to implement:**

`StartServer(port string)`
- Runs `http.ListenAndServe(":"+port, promhttp.Handler())` in a goroutine
- Called from `cmd/root.go` `PersistentPreRun`

`RecordGeneration(status string)` — increments `IaCGenerationsTotal{status}`

`RecordModelLatency(seconds float64)` — observes `ModelLatencySeconds`

`RecordScan(report *scanner.Report)` — for each result, for each finding, calls `ScanFindings.With(labels).Set(float64(count))` and `ScanDurationSeconds.With(labels).Observe(duration)`

`RecordDrift(resourceType string)` — increments `DriftEventsTotal{resourceType}`

---

### Step 4.2 — Prometheus Config

**What to create:** `deploy/prometheus.yml`

```yaml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: 'infrasage-cli'
    static_configs:
      - targets: ['host.docker.internal:2112']
    # host.docker.internal resolves to the Mac host from inside Docker
```

**Why `host.docker.internal`:** The CLI runs on the host (not in Docker). Prometheus runs in Docker. `host.docker.internal` is Docker Desktop's special hostname that routes to the host machine. This avoids any networking complexity.

---

### Step 4.3 — Grafana Provisioning

**What to create:**

`deploy/grafana/provisioning/datasources/prometheus.yml`:
```yaml
apiVersion: 1
datasources:
  - name: Prometheus
    type: prometheus
    url: http://prometheus:9090
    isDefault: true
    access: proxy
```

`deploy/grafana/provisioning/dashboards/dashboard.yml`:
```yaml
apiVersion: 1
providers:
  - name: InfraSage
    folder: InfraSage
    type: file
    options:
      path: /var/lib/grafana/dashboards
```

**What to create:** `deploy/grafana/dashboards/infrasage.json`

This is a Grafana dashboard JSON. It must define these 5 panels:
1. `IaC Generation Rate` — stat panel showing `infrasage_iac_generations_total`
2. `Security Findings by Scanner` — bar gauge showing `infrasage_scan_findings` grouped by scanner
3. `Model Inference Latency` — histogram panel using `infrasage_model_latency_seconds`
4. `Scan Duration` — stat panels for each scanner using `infrasage_scan_duration_seconds`
5. `Drift Events` — time series showing `infrasage_drift_events_total` rate over time

**The easiest way to create this JSON:** Start Grafana (`stack up`), manually create the dashboard in the UI, then export it via Dashboard → Share → Export → Save to file → copy to `deploy/grafana/dashboards/infrasage.json`.

---

## Phase 5 — Polish and Evaluation

**Goal:** Project is demo-ready and all metrics are visible.

### Step 5.1 — README.md

Must contain:
- One-paragraph description
- Architecture diagram (ASCII, copy from ARCHITECTURE.md)
- Prerequisites section
- Quick start (5 commands from clone to first IaC generation)
- All commands reference table
- Screenshot or terminal recording link

### Step 5.2 — Example Terraform Files

**What to create:**

`examples/s3-bucket.tf` — A simple S3 bucket with:
- Versioning enabled
- Server-side encryption
- Public access block
- Correct tags
- Required providers block

`examples/ec2-instance.tf` — An EC2 t2.micro with:
- Security group (no port 22 open to world)
- Required providers block
- Correct tags

These are used for scanner testing and demo purposes.

### Step 5.3 — Model Quality Evaluation Script

**What to create:** `evaluate.py`

This script tests `qwen2.5-coder:3b` via the Ollama API on a standard set of prompts and scores the output quality.

**What it must do:**
1. Load 20 test prompts covering varied AWS infrastructure (S3, EC2, VPC, IAM, RDS)
2. For each prompt, call the Ollama API (`http://localhost:11434/api/generate`) with the same model and system prompt used by the CLI
3. Check generated HCL for:
   - Contains `terraform {` block: +1
   - Contains `required_providers`: +1
   - Contains `required_version`: +1
   - Contains security tags: +1
   - Can be parsed as valid HCL (use `python-hcl2` library): +2
4. Print a results table: `prompt | score | notes`
5. Print overall average score and highlight any prompts that scored below 3/6

**Run with:**
```bash
pip install requests python-hcl2
python evaluate.py
```

**Purpose:** Gives a baseline quality score for the model + system prompt combination. If average score is below 4/6, the system prompt in `cmd/ask.go` should be refined before finalising the project.
