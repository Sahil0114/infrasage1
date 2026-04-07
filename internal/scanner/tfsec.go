package scanner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"
)

// RunTfsec runs tfsec against the given Terraform file via docker exec.
// tfsec exits with code 1 when it finds violations — dockerExec handles this
// transparently by returning stdout even on non-zero exit codes.
func RunTfsec(tfFile string) ScanResult {
	start := time.Now()

	destPath, err := copyToScanDir(tfFile)
	if err != nil {
		return ScanResult{
			Tool:     "tfsec",
			Duration: time.Since(start),
			Error:    fmt.Errorf("tfsec: copying file to scan dir: %w", err),
		}
	}

	// Build the in-container path: /scans/<filename>
	inContainerPath := "/scans/" + filepath.Base(destPath)
	slog.Debug("running tfsec", "container_path", inContainerPath)

	output, err := dockerExec("infrasage-tfsec",
		"tfsec", inContainerPath, "--format", "json", "--no-colour",
	)
	if err != nil {
		return ScanResult{
			Tool:     "tfsec",
			Duration: time.Since(start),
			Error:    fmt.Errorf("tfsec: docker exec failed: %w\nFix: run 'infrasage stack up' to start scanner containers.", err),
		}
	}

	// tfsec sometimes prints warnings before the JSON — find the first '{'.
	jsonStart := bytes.IndexByte(output, '{')
	if jsonStart < 0 {
		// No JSON object in output — treat as zero findings.
		return ScanResult{
			Tool:     "tfsec",
			Passed:   0,
			Failed:   0,
			Duration: time.Since(start),
		}
	}
	output = output[jsonStart:]

	// tfsec JSON shape:
	// { "results": [ { "rule_id": "...", "severity": "HIGH", ... } ] }
	// Note: results may be null when there are no findings — handle both null and [].
	var raw struct {
		Results []struct {
			RuleID          string `json:"rule_id"`
			RuleDescription string `json:"rule_description"`
			Severity        string `json:"severity"`
			Location        struct {
				Filename  string `json:"filename"`
				StartLine int    `json:"start_line"`
			} `json:"location"`
			AffectedResource string `json:"affected_resource"`
		} `json:"results"`
	}

	if err := json.Unmarshal(output, &raw); err != nil {
		return ScanResult{
			Tool:     "tfsec",
			Duration: time.Since(start),
			Error:    fmt.Errorf("tfsec: parsing JSON output: %w", err),
		}
	}

	var findings []Finding
	for _, r := range raw.Results {
		findings = append(findings, Finding{
			ID:       r.RuleID,
			Severity: normalizeSeverity(r.Severity),
			Message:  r.RuleDescription,
			Resource: r.AffectedResource,
			Line:     r.Location.StartLine,
		})
	}

	return ScanResult{
		Tool:     "tfsec",
		Passed:   0, // tfsec only reports failures, not passes
		Failed:   len(findings),
		Findings: findings,
		Duration: time.Since(start),
	}
}
