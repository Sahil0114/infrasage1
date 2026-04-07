# MONITORING.md — Prometheus and Grafana Instructions

> Read this before writing anything in `internal/monitor/` or `deploy/grafana/`.

---

## Why Monitoring In A CLI Tool?

Most CLIs don't expose Prometheus metrics. InfraSage does this to demonstrate DevOps observability practices — a key evaluation criterion for the final year project.

In a real DevSecOps platform, you'd want to track:
- How often is IaC being generated? Is the tool being used?
- Are security findings increasing or decreasing over time?
- Is the model getting slower? (memory fragmentation, model drift)
- How often is drift detected?

These are production-grade questions. Answering them with a Grafana dashboard shows maturity.

---

## Architecture: How Metrics Flow

```
Go CLI binary (running on host)
       │
       │ registers counters/histograms at startup
       │ increments them as commands run
       │
       ▼
internal/monitor/metrics.go
  - Starts HTTP server on :2112 in background goroutine
  - Serves GET /metrics (Prometheus text format)
       │
       │ scrape every 15 seconds
       ▼
Prometheus container (in Docker)
  - Configured via prometheus.yml
  - target: host.docker.internal:2112
  - Stores time-series data
       │
       │ query via PromQL
       ▼
Grafana container (in Docker)
  - Datasource: Prometheus (auto-provisioned)
  - Dashboard: infrasage.json (auto-provisioned)
  - Shows panels for each metric
       │
       ▼
Human opens http://localhost:3000
```

---

## Metrics HTTP Server — How It Works

The Go CLI starts a tiny HTTP server in the background that serves Prometheus metrics. This is NOT the same as an API server. It's a read-only endpoint that Prometheus scrapes.

**Where to start it:** In `cmd/root.go`, in `PersistentPreRun` (runs before every command):
```go
rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
    port := getEnvOrDefault("INFRASAGE_METRICS_PORT", "2112")
    monitor.StartServer(port)
}
```

**What `StartServer` does:**
```go
func StartServer(port string) {
    go func() {
        mux := http.NewServeMux()
        mux.Handle("/metrics", promhttp.Handler())
        _ = http.ListenAndServe(":"+port, mux)
    }()
}
```

The goroutine means it doesn't block the CLI. The `_` discards the error from `ListenAndServe` because in a goroutine we can't return it — if the port is busy, the metrics just won't be available (acceptable).

**Testing the metrics server:**
After running any `infrasage` command, check:
```bash
curl http://localhost:2112/metrics | grep infrasage
```
Should show all registered metrics.

---

## Complete Metrics Specification

### 1. IaC Generations Counter

```go
Name: "infrasage_iac_generations_total"
Type: CounterVec
Labels: ["status"]
Label values: "success" | "failed"
```

- Incremented in `cmd/ask.go` after generation
- "success" = HCL was extracted and written to file
- "failed" = model error or empty response

**Grafana panel:** Stat panel showing total count and rate.

---

### 2. Scan Findings Gauge

```go
Name: "infrasage_scan_findings"
Type: GaugeVec
Labels: ["scanner", "severity"]
Label values:
  scanner: "checkov" | "tfsec" | "terrascan"
  severity: "CRITICAL" | "HIGH" | "MEDIUM" | "LOW"
```

- Set (not incremented) after each scan run
- Use `Set()` not `Inc()` because this represents the current state, not a running total
- Set each label combination to the count from the last scan

**Why Gauge not Counter:** A counter only goes up. Findings go up and down as you generate different files or fix issues. Gauge is the right type.

**Grafana panel:** Bar gauge grouped by scanner, coloured by severity.

---

### 3. Model Latency Histogram

```go
Name: "infrasage_model_latency_seconds"
Type: Histogram
Buckets: [1, 5, 10, 20, 30, 60, 90, 120]
```

- Observed in `cmd/ask.go` after `client.Generate()` returns
- Measures wall clock time from request to response

**Grafana panel:** Heatmap or histogram showing latency distribution.

---

### 4. Drift Events Counter

```go
Name: "infrasage_drift_events_total"
Type: CounterVec
Labels: ["resource_type"]
Label values: "aws_s3_bucket" | "aws_instance" | "unknown"
```

- Incremented in `cmd/drift.go` when drift is detected
- Parse the terraform plan output to find which resource types drifted
- Use "unknown" if parsing fails

**Grafana panel:** Time series showing drift events over time.

---

### 5. Scan Duration Histogram

```go
Name: "infrasage_scan_duration_seconds"
Type: HistogramVec
Labels: ["scanner"]
Buckets: prometheus.DefBuckets (0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10)
```

- Each scanner goroutine records its own duration
- Measured inside each `Run<Scanner>` function using `time.Since(start)`

**Grafana panel:** Bar chart comparing scan times across tools.

---

## Prometheus Configuration

**File:** `deploy/prometheus.yml`

```yaml
global:
  scrape_interval: 15s      # Scrape every 15 seconds
  evaluation_interval: 15s  # Evaluate alerting rules every 15 seconds

scrape_configs:
  - job_name: 'infrasage-cli'
    static_configs:
      - targets: ['host.docker.internal:2112']
    metrics_path: '/metrics'
```

**`host.docker.internal`:** This is Docker Desktop's special hostname. From inside the Prometheus container, `host.docker.internal` resolves to the Mac's host IP address. This is how the containerized Prometheus reaches the CLI running natively on the host.

---

## Grafana Provisioning — Auto-Configuration

Grafana supports automatic provisioning of datasources and dashboards on startup. This means the dashboard is ready to view immediately after `infrasage stack up`, without any manual Grafana setup.

### Datasource Provisioning

**File:** `deploy/grafana/provisioning/datasources/prometheus.yml`

```yaml
apiVersion: 1

datasources:
  - name: Prometheus
    type: prometheus
    uid: prometheus
    url: http://prometheus:9090
    access: proxy
    isDefault: true
    editable: false
    jsonData:
      timeInterval: "15s"
```

**`http://prometheus:9090`:** Inside Docker's network, Grafana reaches Prometheus by service name. This works because both are in the same Docker Compose network (`infrasage-net`).

### Dashboard Provisioning

**File:** `deploy/grafana/provisioning/dashboards/dashboard.yml`

```yaml
apiVersion: 1

providers:
  - name: InfraSage Dashboards
    orgId: 1
    folder: InfraSage
    folderUid: infrasage
    type: file
    disableDeletion: false
    editable: true
    updateIntervalSeconds: 30
    allowUiUpdates: true
    options:
      path: /var/lib/grafana/dashboards
```

---

## Grafana Dashboard JSON — Panel Specifications

**File:** `deploy/grafana/dashboards/infrasage.json`

The easiest approach: build the dashboard manually in Grafana UI first, then export it.

**Steps to build and export:**
1. `infrasage stack up`
2. Run a few `infrasage ask` commands (to generate metric data)
3. Open http://localhost:3000 (admin / infrasage)
4. Create New Dashboard
5. Add each panel (specifications below)
6. When done: Dashboard settings → JSON Model → Copy all
7. Save to `deploy/grafana/dashboards/infrasage.json`

### Panel 1: IaC Generation Rate

- **Type:** Stat
- **Title:** "Generations"
- **Query:** `sum(infrasage_iac_generations_total)`
- **Calculation:** Last (not mean)
- **Colour mode:** Gradient from green to blue

### Panel 2: Success Rate

- **Type:** Stat
- **Title:** "Success Rate"
- **Query:** `sum(infrasage_iac_generations_total{status="success"}) / sum(infrasage_iac_generations_total) * 100`
- **Unit:** Percent (0-100)
- **Thresholds:** <80 = red, 80-95 = yellow, >95 = green

### Panel 3: Security Findings by Scanner

- **Type:** Bar gauge
- **Title:** "Security Findings"
- **Query:** `infrasage_scan_findings`
- **Legend:** `{{scanner}} - {{severity}}`
- **Max:** Auto

### Panel 4: Model Latency

- **Type:** Histogram (or Gauge)
- **Title:** "Model Latency (p50)"
- **Query:** `histogram_quantile(0.5, rate(infrasage_model_latency_seconds_bucket[10m]))`
- **Unit:** Seconds

### Panel 5: Scan Duration by Tool

- **Type:** Bar chart
- **Title:** "Scan Duration by Tool"
- **Query:** `rate(infrasage_scan_duration_seconds_sum[10m]) / rate(infrasage_scan_duration_seconds_count[10m])`
- **Legend:** `{{scanner}}`

### Panel 6: Drift Events Timeline

- **Type:** Time series
- **Title:** "Drift Events"
- **Query:** `rate(infrasage_drift_events_total[1h])`
- **Legend:** `{{resource_type}}`

---

## Testing the Monitoring Stack

After Phase 5 is implemented:

```bash
# 1. Start everything
./bin/infrasage stack up

# 2. Generate some data
./bin/infrasage ask "create an S3 bucket" --no-scan
./bin/infrasage ask "create an EC2 instance" --no-scan
./bin/infrasage ask "create a VPC"

# 3. Verify CLI metrics endpoint
curl http://localhost:2112/metrics | grep infrasage
# Should show: infrasage_iac_generations_total{status="success"} 3
#              infrasage_model_latency_seconds_...

# 4. Verify Prometheus is scraping
open http://localhost:9090/targets
# Should show: infrasage-cli UP

# 5. Verify Prometheus has data
# Go to http://localhost:9090/graph
# Query: infrasage_iac_generations_total
# Should show value of 3

# 6. Open Grafana
./bin/infrasage monitor open
# Dashboard should show panels with data
```
