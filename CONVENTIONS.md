# CONVENTIONS.md — Go Code Conventions

> Read this before writing any Go code.
> These rules apply to every file in the project. No exceptions.

---

## Package Structure Rules

**Rule:** `cmd/` packages may import `internal/` packages. `internal/` packages may NOT import `cmd/` packages. This is enforced by Go's module system.

**Rule:** `internal/scanner/` may import `internal/monitor/` for recording metrics. No other cross-package imports inside `internal/`.

**Rule:** No circular imports. If you feel you need a circular import, you need a new package or a new interface.

**Dependency graph (top to bottom — only downward imports allowed):**
```
cmd/
  └── internal/llm/
  └── internal/terraform/
  └── internal/scanner/
        └── internal/monitor/
  └── internal/gitops/
  └── internal/monitor/
```

---

## File Naming

- One command = one file in `cmd/`. File named after the command: `ask.go`, `scan.go`.
- One concern = one file in `internal/`. Name describes what it does: `client.go`, `runner.go`, `parser.go`.
- Test files: `<file>_test.go` in the same package.
- No `util.go`, `helpers.go`, or `common.go`. Name files specifically.

---

## Error Handling

**Always wrap errors with context:**
```go
// CORRECT
return fmt.Errorf("scanner RunAll: copying file to scan dir: %w", err)

// WRONG
return err
```

**Commands must return human-readable errors with next steps:**
```go
// CORRECT
return fmt.Errorf("Ollama is not running at %s.\nFix: run 'ollama serve' in a terminal, then retry.", url)

// WRONG
return fmt.Errorf("connection refused")
```

**Never use `log.Fatal` or `os.Exit` inside `internal/` packages.** Only `cmd/` layer may exit the process, and only by returning an error from `RunE`.

**Never use `panic` except for programmer errors during init (e.g., `prometheus.MustRegister`).**

---

## Environment Variables

All configuration comes from environment variables. No hardcoded values except defaults.

Pattern for every config value:
```go
func getEnvOrDefault(key, defaultVal string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return defaultVal
}
```

**Complete list of env vars used by the project:**

| Variable | Default | Used In |
|---|---|---|
| `INFRASAGE_OLLAMA_URL` | `http://localhost:11434` | `cmd/ask.go` |
| `INFRASAGE_OLLAMA_MODEL` | `qwen2.5-coder:3b` | `cmd/ask.go` |
| `INFRASAGE_OLLAMA_TIMEOUT` | `120s` | `internal/llm/client.go` |
| `INFRASAGE_SCAN_DIR` | `/tmp/infrasage-scans` | `internal/scanner/docker.go` |
| `INFRASAGE_COMPOSE_FILE` | `./deploy/docker-compose.yml` | `cmd/stack.go` |
| `INFRASAGE_UI_PORT` | `3000` | `cmd/ui.go` |
| `INFRASAGE_METRICS_PORT` | `2112` | `internal/monitor/metrics.go` |
| `INFRASAGE_GRAFANA_URL` | `http://localhost:3001` | `cmd/monitor.go` |
| `GITHUB_TOKEN` | *(required for deploy)* | `internal/gitops/github.go` |
| `GITHUB_REPO` | *(required for deploy)* | `internal/gitops/github.go` |
| `GITHUB_DEFAULT_BRANCH` | `main` | `internal/gitops/github.go` |
| `AWS_REGION` | `us-east-1` | Referenced in examples only |

---

## Logging

**Use `log/slog` (Go 1.21+). Never use `fmt.Println` for debug output.**

```go
// For debug info (only shown with --debug flag):
slog.Debug("calling ollama", "url", url, "model", model)

// For operational info always shown:
fmt.Printf("⏳ Generating Terraform HCL...\n")

// For errors in cmd layer:
fmt.Fprintf(os.Stderr, "Error: %v\n", err)
```

**The `--debug` global flag** in `cmd/root.go` should set slog level to Debug:
```go
if debug {
    slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))
}
```

---

## External Process Execution

All `os/exec` calls follow this pattern:
```go
ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
defer cancel()

cmd := exec.CommandContext(ctx, "docker", "exec", containerName, "checkov", "-f", filePath)
output, err := cmd.Output()
if err != nil {
    // Check if it's a context timeout
    if ctx.Err() == context.DeadlineExceeded {
        return ScanResult{Error: fmt.Errorf("checkov timed out after 60s")}
    }
    return ScanResult{Error: fmt.Errorf("checkov exec: %w", err)}
}
```

**Never use `cmd.Run()` when you need output. Use `cmd.Output()` for stdout or `cmd.CombinedOutput()` for stdout+stderr.**

**When streaming output to terminal** (for terraform or docker compose commands): connect to os.Stdout/os.Stderr:
```go
cmd.Stdout = os.Stdout
cmd.Stderr = os.Stderr
err := cmd.Run()
```

---

## JSON Parsing

Parse only the fields you need. Use anonymous structs for one-off parsing:
```go
var result struct {
    Results struct {
        FailedChecks []struct {
            CheckID       string `json:"check_id"`
            Resource      string `json:"resource"`
            FileLineRange []int  `json:"file_line_range"`
        } `json:"failed_checks"`
        PassedChecks []struct{} `json:"passed_checks"`
    } `json:"results"`
}
if err := json.Unmarshal(output, &result); err != nil {
    return ScanResult{Error: fmt.Errorf("parsing checkov JSON: %w", err)}
}
```

**Never ignore JSON parse errors.** Scanner tools sometimes output warnings before the JSON — trim non-JSON prefix if necessary.

---

## Goroutines

The scanner runner uses goroutines. Follow this exact pattern:
```go
results := make(chan ScanResult, 3)  // buffered — won't block

go func() { results <- RunCheckov(tfFile) }()
go func() { results <- RunTfsec(tfFile) }()
go func() { results <- RunTerrascan(tfFile) }()

var allResults []ScanResult
for i := 0; i < 3; i++ {
    allResults = append(allResults, <-results)
}
```

**Never launch goroutines that could leak.** Every goroutine must either return or receive from a context cancellation.

---

## Approved Dependencies (Do Not Add Others Without Reason)

```
github.com/spf13/cobra          — CLI framework
github.com/joho/godotenv        — .env loading
github.com/prometheus/client_golang — metrics
```

Standard library covers everything else:
- HTTP: `net/http`
- JSON: `encoding/json`
- Exec: `os/exec`
- Files: `os`, `io`, `path/filepath`
- Logging: `log/slog`
- Context: `context`
- Concurrency: `sync`, channels
- Regex: `regexp`
- Time: `time`
- Strings: `strings`, `fmt`

---

## Makefile Targets (Must All Work)

```makefile
build          # go build -o bin/infrasage .
test           # go test ./...
lint           # go vet ./... (golangci-lint run ./... if installed)
fmt            # go fmt ./...
tidy           # go mod tidy
clean          # rm -rf bin/ /tmp/infrasage-scans
stack-up       # docker compose -f deploy/docker-compose.yml up -d
stack-down     # docker compose -f deploy/docker-compose.yml down
model-pull     # ollama pull qwen2.5-coder:3b
setup          # One-time: checks deps, pulls model, starts stack
```

---

## Commit Message Convention

```
feat(ask): add --no-scan flag to skip security scanning
fix(scanner): handle empty checkov JSON output
docs(readme): add quick start section
chore(deps): update cobra to v1.8.1
```

Pattern: `type(scope): description`
Types: `feat`, `fix`, `docs`, `chore`, `test`, `refactor`
