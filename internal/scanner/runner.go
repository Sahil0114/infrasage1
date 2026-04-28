package scanner

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"time"
)

// RunAll runs all three security scanners against the given Terraform file in parallel.
// Each scanner copies the file to the shared scan directory independently.
// Individual scanner failures are captured in ScanResult.Error — RunAll itself only
// returns an error for path resolution failures.
func RunAll(tfFile string) (*Report, error) {
	return RunAllWithCallback(tfFile, nil)
}

// RunAllWithCallback runs all scanners and invokes onResult as each completes.
// onResult may be nil.
func RunAllWithCallback(tfFile string, onResult func(ScanResult)) (*Report, error) {
	absPath, err := filepath.Abs(tfFile)
	if err != nil {
		return nil, fmt.Errorf("scanner RunAll: resolving absolute path for %q: %w", tfFile, err)
	}

	slog.Debug("scanner RunAll: starting", "file", absPath)
	overall := time.Now()

	// Buffer of 3 so goroutines never block on send.
	results := make(chan ScanResult, 3)

	go func() { results <- RunCheckov(absPath) }()
	go func() { results <- RunTfsec(absPath) }()
	go func() { results <- RunTerrascan(absPath) }()

	report := &Report{
		File: filepath.Base(absPath),
	}

	for i := 0; i < 3; i++ {
		res := <-results
		report.Results = append(report.Results, res)
		if onResult != nil {
			onResult(res)
		}
		if res.Error != nil {
			slog.Debug("scanner result with error", "tool", res.Tool, "err", res.Error)
		} else {
			slog.Debug("scanner result",
				"tool", res.Tool,
				"passed", res.Passed,
				"failed", res.Failed,
				"duration", res.Duration,
			)
		}
	}

	slog.Debug("scanner RunAll: complete",
		"total_elapsed", time.Since(overall),
		"total_passed", report.TotalPassed(),
		"total_failed", report.TotalFailed(),
	)

	return report, nil
}
