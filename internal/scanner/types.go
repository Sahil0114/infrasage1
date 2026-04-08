package scanner

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Finding represents a single security finding from a scanner tool.
type Finding struct {
	ID       string // e.g. CKV_AWS_18, AVD-AWS-0086, AC_AWS_0207
	Severity string // CRITICAL, HIGH, MEDIUM, LOW
	Message  string // Human-readable description
	Resource string // e.g. aws_s3_bucket.my_bucket
	Line     int    // Line number in .tf file
}

// ScanResult holds the output of one scanner tool run.
type ScanResult struct {
	Tool     string
	Passed   int
	Failed   int
	Findings []Finding
	Duration time.Duration
	Error    error // nil if scan succeeded
	RawJSON  []byte // Raw JSON output from the scanner (for Checkov)
}

// Report aggregates results from all scanner tools for a single file.
type Report struct {
	File    string
	Results []ScanResult
}

// TotalPassed returns the sum of passed checks across all scanners.
func (r *Report) TotalPassed() int {
	total := 0
	for _, res := range r.Results {
		total += res.Passed
	}
	return total
}

// TotalFailed returns the sum of failed checks across all scanners.
func (r *Report) TotalFailed() int {
	total := 0
	for _, res := range r.Results {
		total += res.Failed
	}
	return total
}

// HasCritical returns true if any finding has CRITICAL severity.
func (r *Report) HasCritical() bool {
	for _, res := range r.Results {
		for _, f := range res.Findings {
			if f.Severity == "CRITICAL" {
				return true
			}
		}
	}
	return false
}

// Print renders the scan report table to stdout.
func (r *Report) Print() {
	const width = 54
	bar := strings.Repeat("═", width-2)

	fmt.Printf("\n╔%s╗\n", bar)
	fmt.Printf("║%s║\n", centerStr("InfraSage Security Scan Report", width-2))
	fmt.Printf("╠%s╣\n", bar)
	fmt.Printf("║  File: %-*s║\n", width-10, r.File)
	fmt.Printf("╠══════════╦══════════╦══════════╦%s╣\n", strings.Repeat("═", width-34))
	fmt.Printf("║ %-8s ║ %-8s ║ %-8s ║ %-*s║\n", "Scanner", "Pass", "Fail", width-35, "Status")
	fmt.Printf("╠══════════╬══════════╬══════════╬%s╣\n", strings.Repeat("═", width-34))

	for _, res := range r.Results {
		status := statusString(res)
		passStr := "-"
		failStr := "-"
		if res.Error == nil {
			passStr = fmt.Sprintf("%d", res.Passed)
			failStr = fmt.Sprintf("%d", res.Failed)
		}
		fmt.Printf("║ %-8s ║ %-8s ║ %-8s ║ %-*s║\n",
			truncate(res.Tool, 8), passStr, failStr, width-35, status)
	}

	totalFailed := r.TotalFailed()
	fmt.Printf("╠══════════╩══════════╩══════════╩%s╣\n", strings.Repeat("═", width-34))

	if totalFailed > 0 {
		header := fmt.Sprintf("  FINDINGS (%d total):", totalFailed)
		fmt.Printf("║%-*s║\n", width-2, header)
		fmt.Printf("╠%s╣\n", bar)

		// Collect all findings and sort by severity (CRITICAL first)
		var all []Finding
		for _, res := range r.Results {
			all = append(all, res.Findings...)
		}
		sort.Slice(all, func(i, j int) bool {
			return severityOrder(all[i].Severity) < severityOrder(all[j].Severity)
		})

		for _, f := range all {
			msg := f.Message
			if msg == "" {
				msg = f.ID
			}
			line := fmt.Sprintf("  [%-12s] %-8s %s", truncate(f.ID, 12), f.Severity, msg)
			if len(line) > width-2 {
				line = line[:width-5] + "..."
			}
			fmt.Printf("║%-*s║\n", width-2, line)
		}
	} else {
		fmt.Printf("║  %-*s║\n", width-4, "✅ No findings — all checks passed!")
	}

	fmt.Printf("╚%s╝\n\n", bar)

	// Print per-scanner errors below the table
	for _, res := range r.Results {
		if res.Error != nil {
			fmt.Printf("⚠  %s error: %v\n", res.Tool, res.Error)
			fmt.Printf("   Fix: run 'infrasage stack up' to start scanner containers.\n\n")
		}
	}
}

// statusString returns the display status for a scan result.
func statusString(result ScanResult) string {
	if result.Error != nil {
		return "✗  ERROR"
	}
	if result.Failed == 0 {
		return "✓  PASS"
	}
	for _, f := range result.Findings {
		if f.Severity == "CRITICAL" {
			return "✗  FAIL"
		}
	}
	return "⚠  WARN"
}

// centerStr centers s within a field of given width using spaces.
func centerStr(s string, width int) string {
	if len(s) >= width {
		return s
	}
	left := (width - len(s)) / 2
	right := width - len(s) - left
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", right)
}

// severityOrder returns sort priority for a severity string (lower = higher priority).
func severityOrder(s string) int {
	switch strings.ToUpper(s) {
	case "CRITICAL":
		return 0
	case "HIGH":
		return 1
	case "MEDIUM":
		return 2
	case "LOW":
		return 3
	default:
		return 4
	}
}

// truncate shortens s to at most n runes, padding with spaces to exactly n.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// normalizeSeverity maps tool-specific severity names to the four canonical values.
func normalizeSeverity(s string) string {
	switch strings.ToUpper(s) {
	case "CRITICAL":
		return "CRITICAL"
	case "HIGH", "ERROR":
		return "HIGH"
	case "MEDIUM", "WARNING", "WARN":
		return "MEDIUM"
	case "LOW", "INFO", "NOTICE":
		return "LOW"
	default:
		return "MEDIUM"
	}
}
