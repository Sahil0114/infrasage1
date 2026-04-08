package monitor

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/Sahil0114/infrasage1/internal/scanner"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	modelLatency = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "infrasage_model_latency_seconds",
			Help:    "Time taken for Ollama model inference in seconds.",
			Buckets: []float64{1, 5, 10, 20, 30, 60, 90, 120},
		},
	)

	scanDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "infrasage_scan_duration_seconds",
			Help:    "Time taken per security scanner in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"scanner"},
	)
)

func init() {
	prometheus.MustRegister(
		modelLatency,
		scanDurationSeconds,
	)

	prometheus.MustRegister(newPersistentCollector())
}

// StartServer starts the Prometheus metrics HTTP server on the given port.
// It runs in a background goroutine and returns immediately.
func StartServer(port string) {
	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		addr := fmt.Sprintf(":%s", port)
		slog.Info("metrics server started", "addr", addr)
		if err := http.ListenAndServe(addr, mux); err != nil {
			slog.Error("metrics server stopped", "err", err)
		}
	}()
}

// RecordGeneration increments the generation counter with the given status label.
// status should be "success" or "error".
func RecordGeneration(status string) {
	storeMu.Lock()
	defer storeMu.Unlock()

	state, err := loadState()
	if err != nil {
		slog.Error("metrics store load failed", "err", err)
		return
	}

	state.Generations[status]++
	if err := saveState(state); err != nil {
		slog.Error("metrics store save failed", "err", err)
	}
}

// RecordModelLatency records the time taken for a model inference call.
func RecordModelLatency(seconds float64) {
	modelLatency.Observe(seconds)
}

// RecordScanFinding increments the scan findings gauge for the given scanner and severity.
// Call once per finding. severity should be CRITICAL, HIGH, MEDIUM, or LOW.
func RecordScanFinding(tool, severity string) {
	storeMu.Lock()
	defer storeMu.Unlock()

	state, err := loadState()
	if err != nil {
		slog.Error("metrics store load failed", "err", err)
		return
	}

	key := fmt.Sprintf("%s|%s", tool, severity)
	state.ScanFindings[key]++
	if err := saveState(state); err != nil {
		slog.Error("metrics store save failed", "err", err)
	}
}

// RecordScan records aggregate scanner metrics for one report.
func RecordScan(report *scanner.Report) {
	if report == nil {
		return
	}

	for _, res := range report.Results {
		if res.Error != nil {
			continue
		}
		scanDurationSeconds.WithLabelValues(res.Tool).Observe(res.Duration.Seconds())
		for _, f := range res.Findings {
			RecordScanFinding(res.Tool, f.Severity)
		}
	}
}

// RecordDrift increments the drift detected counter for the given resource type.
func RecordDrift(resourceType string) {
	storeMu.Lock()
	defer storeMu.Unlock()

	state, err := loadState()
	if err != nil {
		slog.Error("metrics store load failed", "err", err)
		return
	}

	state.Drift[resourceType]++
	if err := saveState(state); err != nil {
		slog.Error("metrics store save failed", "err", err)
	}
}

type persistedMetrics struct {
	Generations  map[string]float64 `json:"generations"`
	ScanFindings map[string]float64 `json:"scan_findings"`
	Drift        map[string]float64 `json:"drift"`
}

type persistentCollector struct {
	generationsDesc  *prometheus.Desc
	scanFindingsDesc *prometheus.Desc
	driftDesc        *prometheus.Desc
}

var storeMu sync.Mutex

func newPersistentCollector() *persistentCollector {
	return &persistentCollector{
		generationsDesc: prometheus.NewDesc(
			"infrasage_iac_generations_total",
			"Total number of Terraform HCL generation attempts.",
			[]string{"status"}, nil,
		),
		scanFindingsDesc: prometheus.NewDesc(
			"infrasage_scan_findings",
			"Number of security findings by scanner and severity.",
			[]string{"scanner", "severity"}, nil,
		),
		driftDesc: prometheus.NewDesc(
			"infrasage_drift_detected_total",
			"Total number of drift detection events by resource type.",
			[]string{"resource_type"}, nil,
		),
	}
}

func (c *persistentCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.generationsDesc
	ch <- c.scanFindingsDesc
	ch <- c.driftDesc
}

func (c *persistentCollector) Collect(ch chan<- prometheus.Metric) {
	storeMu.Lock()
	defer storeMu.Unlock()

	state, err := loadState()
	if err != nil {
		slog.Error("metrics store load failed during collect", "err", err)
		return
	}

	for status, count := range state.Generations {
		ch <- prometheus.MustNewConstMetric(c.generationsDesc, prometheus.CounterValue, count, status)
	}

	for key, count := range state.ScanFindings {
		parts := splitKey(key)
		ch <- prometheus.MustNewConstMetric(c.scanFindingsDesc, prometheus.GaugeValue, count, parts[0], parts[1])
	}

	for resourceType, count := range state.Drift {
		ch <- prometheus.MustNewConstMetric(c.driftDesc, prometheus.CounterValue, count, resourceType)
	}
}

func metricsStorePath() string {
	if p := os.Getenv("INFRASAGE_METRICS_STORE"); p != "" {
		return p
	}
	return "/tmp/infrasage-metrics.json"
}

func loadState() (*persistedMetrics, error) {
	path := metricsStorePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &persistedMetrics{
				Generations:  map[string]float64{},
				ScanFindings: map[string]float64{},
				Drift:        map[string]float64{},
			}, nil
		}
		return nil, err
	}

	var state persistedMetrics
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	if state.Generations == nil {
		state.Generations = map[string]float64{}
	}
	if state.ScanFindings == nil {
		state.ScanFindings = map[string]float64{}
	}
	if state.Drift == nil {
		state.Drift = map[string]float64{}
	}

	return &state, nil
}

func saveState(state *persistedMetrics) error {
	path := metricsStorePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := json.Marshal(state)
	if err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}

	return os.Rename(tmp, path)
}

func splitKey(key string) [2]string {
	for i := 0; i < len(key); i++ {
		if key[i] == '|' {
			return [2]string{key[:i], key[i+1:]}
		}
	}
	return [2]string{key, "unknown"}
}
