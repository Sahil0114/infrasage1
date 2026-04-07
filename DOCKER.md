# DOCKER.md — Container Architecture Instructions

> Read this before touching any container config, docker-compose.yml, or any code that calls docker exec.

---

## The Golden Rule: Ollama Is NOT In Docker

On this machine (M2, 8GB RAM), Ollama must run natively on macOS. Reasons:

1. **Metal GPU acceleration:** Ollama on macOS uses Apple Metal. Inside Docker, it can only use CPU. Inference is 4–8x slower on CPU with this model size.
2. **RAM:** Docker Desktop itself consumes ~0.5GB. Adding the Ollama container and model inside Docker would push total RAM past safe limits.
3. **Simplicity:** Ollama has a native macOS arm64 binary installed via Homebrew. It just works.

**What this means for the compose file:** There is no `ollama` service in `docker-compose.yml`. The Go CLI talks to Ollama on the host via `http://localhost:11434`.

---

## How The Shared Scan Volume Works

All three scanner containers share a volume mounted from the host:

```
Host filesystem:  /tmp/infrasage-scans/
                       │
         ┌─────────────┼─────────────┐
         ▼             ▼             ▼
   checkov         tfsec         terrascan
  container       container      container
  /scans/         /scans/        /scans/
```

**Flow when a scan runs:**
1. Go CLI copies `infra.tf` from current directory to `/tmp/infrasage-scans/infra.tf` on the host
2. CLI runs `docker exec infrasage-checkov checkov -f /scans/infra.tf --output json`
3. The container reads `/scans/infra.tf` (which is the same file as host `/tmp/infrasage-scans/infra.tf`)
4. CLI captures stdout (the JSON result)
5. CLI deletes the temp file after all scans complete (optional but clean)

**Why `tail -f /dev/null` as entrypoint:**
The scanner containers have no long-running process. If we ran `checkov` directly as the entrypoint, the container would exit after the scan. We keep the container alive with `tail -f /dev/null` and use `docker exec` to run commands against the already-running container. This avoids the overhead of starting a new container for each scan.

---

## Docker Compose File — Complete Specification

**Location:** `deploy/docker-compose.yml`

**Every service must have these fields:**
```yaml
platform: linux/arm64    # Critical for M2
restart: unless-stopped  # Restart on docker daemon restart
```

**Scanner services entrypoint pattern:**
```yaml
entrypoint: ["tail", "-f", "/dev/null"]
```
This keeps the container alive indefinitely. The CLI uses `docker exec` to run tools.

**Volume mount for scanners:**
```yaml
volumes:
  - /tmp/infrasage-scans:/scans
```
The left side is the absolute host path. The right side is the container path. This must match what `internal/scanner/docker.go` uses.

**Prometheus must reach the CLI's metrics port:**
The CLI runs on the host, not in Docker. Prometheus (in Docker) must reach it via:
```yaml
# In prometheus.yml scrape config:
targets: ['host.docker.internal:2112']
```
`host.docker.internal` is Docker Desktop's hostname that routes from inside a container to the host machine. This works on macOS automatically.

**Grafana auto-provisioning:**
Grafana loads datasources and dashboards from mounted directories at startup. The compose volumes must mount:
```yaml
volumes:
  - ./grafana/provisioning:/etc/grafana/provisioning
  - ./grafana/dashboards:/var/lib/grafana/dashboards
```
The provisioning YAML files tell Grafana where to find things. The dashboard JSON is the actual dashboard. Grafana reads these on every startup.

---

## Docker Exec Patterns — How The CLI Talks To Containers

**Pattern 1 — Checkov:**
```bash
docker exec infrasage-checkov \
  checkov -f /scans/infra.tf --output json --quiet
```
- `--quiet` suppresses progress bars that would corrupt JSON output
- Stdout: JSON result
- Exit code: 0 = all passed, 1 = some failed (this is normal — parse regardless)

**Pattern 2 — tfsec:**
```bash
docker exec infrasage-tfsec \
  tfsec /scans/infra.tf --format json --no-colour
```
- `--no-colour` prevents ANSI escape codes in JSON
- tfsec scans the directory, not a single file — provide the directory if multiple .tf files

**Pattern 3 — Terrascan:**
```bash
docker exec infrasage-terrascan \
  terrascan scan -i terraform -f /scans/infra.tf -o json
```
- `-i terraform` tells it the IaC type
- `-o json` for machine-readable output

**Handling non-zero exit codes:**
All three tools exit with code 1 when they find violations. In Go:
```go
output, err := cmd.Output()
// err will be non-nil if exit code != 0
// But we still have output! Parse it.
var exitErr *exec.ExitError
if errors.As(err, &exitErr) {
    // Non-zero exit code is expected when violations found
    // Still parse output
} else if err != nil {
    // Actual error (container not running, docker not found, etc.)
    return ScanResult{Error: err}
}
// Parse output regardless
```

---

## Container Health Checks

The `stack status` command runs `docker compose ps`. Each container shows its status.

For the three scanner containers, "healthy" means the container is running — even though they're just running `tail -f /dev/null`. They're ready to accept `docker exec` commands.

**If a container is not running when a scan is attempted:**
The `docker exec` command will fail with a readable error. The scanner function must catch this and return a `ScanResult{Error: ...}` rather than panicking. The report printer must handle `ScanResult.Error != nil` by showing `"✗ ERROR"` in the status column.

---

## Memory Limits in Compose

Do NOT set memory limits on scanner containers. They need burst capacity during active scanning. The scanner containers use near-zero RAM when idle (just the `tail` process).

DO set a memory limit on Grafana if RAM pressure becomes an issue:
```yaml
grafana:
  deploy:
    resources:
      limits:
        memory: 256M
```

---

## Common Docker Problems on M2 and Their Fixes

**Problem:** Container pulls x86 image, runs under Rosetta, crashes.
**Fix:** Add `platform: linux/arm64` to the service in compose.

**Problem:** `host.docker.internal` not resolving inside container.
**Fix:** Docker Desktop for Mac resolves this automatically. If it fails, add to compose:
```yaml
extra_hosts:
  - "host.docker.internal:host-gateway"
```

**Problem:** `/tmp/infrasage-scans` not writable by container.
**Fix:** `chmod 777 /tmp/infrasage-scans` on the host. The CLI creates this directory in `stack up`.

**Problem:** Checkov container logs show "no checks found".
**Fix:** The `-f` flag needs a specific file, not a directory. Verify the file path is `/scans/infra.tf` not `/scans/`.

**Problem:** Grafana shows "No data" in panels.
**Fix:** 
1. Verify `bin/infrasage` has been run at least once (to generate metrics)
2. Verify Prometheus is scraping: open http://localhost:9090/targets, should show `infrasage-cli` as UP
3. Verify the CLI's metrics server is running: `curl http://localhost:2112/metrics`
