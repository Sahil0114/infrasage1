package scanner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"
)

// RunTerrascan runs terrascan against tfFile using the infrasage-terrascan container.
// Terrascan exits with code 3 when violations are found — this is handled gracefully
// by dockerExec, which returns stdout even on non-zero exit codes.
func RunTerrascan(tfFile string) ScanResult {
	start := time.Now()
	result := ScanResult{Tool: "terrascan"}

	destPath, err := copyToScanDir(tfFile)
	if err != nil {
		result.Error = fmt.Errorf("scanner RunTerrascan: copying file to scan dir: %w", err)
		return result
	}

	// The container mounts the scan dir as /scans — build the in-container path.
	inContainerPath := "/scans/" + filepath.Base(destPath)

	output, err := dockerExec(
		"infrasage-terrascan",
		"terrascan", "scan",
		"-i", "terraform",
		"-f", inContainerPath,
		"-o", "json",
	)
	if err != nil {
		result.Error = fmt.Errorf("scanner RunTerrascan: %w\nFix: run 'infrasage stack up' to start the terrascan container.", err)
		result.Duration = time.Since(start)
		return result
	}

	// Terrascan sometimes emits log lines before the JSON object — trim them.
	jsonStart := bytes.IndexByte(output, '{')
	if jsonStart > 0 {
		output = output[jsonStart:]
	}

	if len(output) == 0 {
		// No output at all — treat as zero findings.
		result.Duration = time.Since(start)
		return result
	}

	var raw struct {
		Results struct {
			Violations []struct {
				RuleName     string `json:"rule_name"`
				RuleID       string `json:"rule_id"`
				Severity     string `json:"severity"`
				Description  string `json:"description"`
				ResourceName string `json:"resource_name"`
				ResourceType string `json:"resource_type"`
				File         string `json:"file"`
				Line         int    `json:"line"`
			} `json:"violations"`
			PassedRules []struct{} `json:"passed_rules"`
		} `json:"results"`
	}

	if err := json.Unmarshal(output, &raw); err != nil {
		result.Error = fmt.Errorf("scanner RunTerrascan: parsing JSON output: %w", err)
		result.Duration = time.Since(start)
		return result
	}

	result.Passed = len(raw.Results.PassedRules)
	result.Failed = len(raw.Results.Violations)
	result.Duration = time.Since(start)

	for _, v := range raw.Results.Violations {
		id := v.RuleID
		if id == "" {
			id = v.RuleName
		}
		msg := v.Description
		if msg == "" {
			msg = id
		}
		result.Findings = append(result.Findings, Finding{
			ID:       id,
			Severity: normalizeSeverity(v.Severity),
			Message:  msg,
			Resource: v.ResourceName,
			Line:     v.Line,
		})
	}

	return result
}
