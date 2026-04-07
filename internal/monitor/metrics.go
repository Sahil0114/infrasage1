package monitor

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	generationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "infrasage_iac_generations_total",
			Help: "Total number of Terraform HCL generation attempts.",
		},
		[]string{"status"},
	)

	modelLatency = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "infrasage_model_latency_seconds",
			Help:    "Time taken for Ollama model inference in seconds.",
			Buckets: []float64{1, 5, 10, 20, 30, 60, 90, 120},
		},
	)

	scanFindings = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "infrasage_scan_findings",
			Help: "Number of security findings by scanner and severity.",
		},
		[]string{"scanner", "severity"},
	)

	driftDetectedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "infrasage_drift_detected_total",
			Help: "Total number of drift detection events by resource type.",
		},
		[]string{"resource_type"},
	)
)

func init() {
	prometheus.MustRegister(
		generationsTotal,
		modelLatency,
		scanFindings,
		driftDetectedTotal,
	)
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
	generationsTotal.WithLabelValues(status).Inc()
}

// RecordModelLatency records the time taken for a model inference call.
func RecordModelLatency(seconds float64) {
	modelLatency.Observe(seconds)
}

// RecordScanFinding increments the scan findings gauge for the given scanner and severity.
// Call once per finding. severity should be CRITICAL, HIGH, MEDIUM, or LOW.
func RecordScanFinding(tool, severity string) {
	scanFindings.WithLabelValues(tool, severity).Add(1)
}

// RecordDrift increments the drift detected counter for the given resource type.
func RecordDrift(resourceType string) {
	driftDetectedTotal.WithLabelValues(resourceType).Inc()
}
