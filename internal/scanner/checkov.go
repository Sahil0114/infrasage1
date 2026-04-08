package scanner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"
)

// checkovSeverity maps known Checkov check IDs to their canonical severity level.
// Checkov's JSON output does not include severity by default, so we maintain
// a best-effort lookup table. Unknown IDs default to MEDIUM.
var checkovSeverity = map[string]string{
	// S3
	"CKV_AWS_18":  "MEDIUM", // S3 access logging
	"CKV_AWS_19":  "HIGH",   // S3 encryption
	"CKV_AWS_20":  "HIGH",   // S3 public ACL
	"CKV_AWS_21":  "MEDIUM", // S3 versioning
	"CKV_AWS_52":  "HIGH",   // S3 MFA delete
	"CKV_AWS_144": "HIGH",   // S3 replication encrypted
	"CKV_AWS_145": "HIGH",   // S3 KMS encryption (CMK)
	"CKV2_AWS_6":  "HIGH",   // S3 public access block
	"CKV2_AWS_61": "MEDIUM", // S3 access logging (v2)
	"CKV2_AWS_62": "MEDIUM", // S3 event notifications
	// EC2
	"CKV_AWS_8":   "HIGH",   // EC2 IMDSv2
	"CKV_AWS_79":  "HIGH",   // EC2 IMDSv2 required
	"CKV_AWS_126": "MEDIUM", // EC2 detailed monitoring
	// IAM
	"CKV_AWS_40": "HIGH",   // IAM admin policy
	"CKV_AWS_1":  "MEDIUM", // IAM password policy
	// Security Groups
	"CKV_AWS_24": "CRITICAL", // SG unrestricted SSH
	"CKV_AWS_25": "CRITICAL", // SG unrestricted RDP
	// RDS
	"CKV_AWS_16": "HIGH",   // RDS encryption
	"CKV_AWS_17": "HIGH",   // RDS public access
	"CKV_AWS_23": "MEDIUM", // RDS minor version upgrade
	// General
	"CKV2_AWS_5": "MEDIUM", // SG not attached
}

// checkovMessages maps Checkov check IDs to human-readable descriptions.
// Used to display meaningful messages in the scan report instead of raw check IDs.
var checkovMessages = map[string]string{
	// S3
	"CKV_AWS_18":  "S3 bucket does not have access logging enabled",
	"CKV_AWS_19":  "S3 bucket does not have server-side encryption enabled",
	"CKV_AWS_20":  "S3 bucket has public READ ACL",
	"CKV_AWS_21":  "S3 bucket does not have versioning enabled",
	"CKV_AWS_52":  "S3 bucket does not have MFA delete enabled",
	"CKV_AWS_144": "S3 bucket replication is not using encrypted transfer",
	"CKV_AWS_145": "S3 bucket is not encrypted with customer-managed KMS key",
	"CKV2_AWS_6":  "S3 bucket does not have public access block configured",
	"CKV2_AWS_61": "S3 bucket does not have access control policy attached",
	"CKV2_AWS_62": "S3 bucket does not have event notifications enabled",
	// EC2
	"CKV_AWS_8":   "EC2 instance metadata service (IMDS) does not require IMDSv2",
	"CKV_AWS_79":  "EC2 launch template does not enforce IMDSv2",
	"CKV_AWS_126": "EC2 instance does not have detailed monitoring enabled",
	// IAM
	"CKV_AWS_40": "IAM policy allows admin privileges (*:*)",
	"CKV_AWS_1":  "IAM password policy does not meet minimum requirements",
	// Security Groups
	"CKV_AWS_24": "Security group allows unrestricted SSH access (0.0.0.0/0:22)",
	"CKV_AWS_25": "Security group allows unrestricted RDP access (0.0.0.0/0:3389)",
	// RDS
	"CKV_AWS_16": "RDS instance is not encrypted at rest",
	"CKV_AWS_17": "RDS instance is publicly accessible",
	"CKV_AWS_23": "RDS instance does not have auto minor version upgrade enabled",
	// General
	"CKV2_AWS_5": "Security group is not attached to any resource",
}

// RunCheckov runs the Checkov scanner against tfFile using the infrasage-checkov container.
// It returns a ScanResult regardless of whether findings were found — a non-nil Error
// field means the scanner itself failed (container not running, docker not found, etc.).
func RunCheckov(tfFile string) ScanResult {
	start := time.Now()

	hostPath, err := copyToScanDir(tfFile)
	if err != nil {
		return ScanResult{
			Tool:  "checkov",
			Error: fmt.Errorf("checkov: copying file to scan dir: %w", err),
		}
	}

	// Convert the host scan dir path to the container mount path /scans/
	filename := filepath.Base(hostPath)
	containerPath := "/scans/" + filename

	slog.Debug("running checkov", "container_path", containerPath)

	output, err := dockerExec("infrasage-checkov",
		"checkov", "-f", containerPath, "--output", "json", "--quiet")
	if err != nil {
		return ScanResult{
			Tool:  "checkov",
			Error: fmt.Errorf("checkov: docker exec failed: %w\nFix: run 'infrasage stack up' to start scanner containers.", err),
		}
	}

	result := parseCheckovOutput(output, start)
	result.RawJSON = output
	return result
}

// parseCheckovOutput parses Checkov's JSON output into a ScanResult.
func parseCheckovOutput(output []byte, start time.Time) ScanResult {
	result := ScanResult{
		Tool:     "checkov",
		Duration: time.Since(start),
	}

	// Checkov sometimes prints non-JSON lines before the JSON blob.
	// Find the first '{' to trim any leading warnings or progress output.
	jsonStart := bytes.IndexByte(output, '{')
	if jsonStart < 0 {
		// No JSON found — checkov may have printed nothing (all passed with no output)
		result.Passed = 0
		result.Failed = 0
		return result
	}
	output = output[jsonStart:]

	var raw struct {
		Results struct {
			PassedChecks []struct {
				CheckID string `json:"check_id"`
			} `json:"passed_checks"`
			FailedChecks []struct {
				CheckID       string `json:"check_id"`
				CheckType     string `json:"check_type"`
				Resource      string `json:"resource"`
				FileLineRange []int  `json:"file_line_range"`
			} `json:"failed_checks"`
		} `json:"results"`
	}

	if err := json.Unmarshal(output, &raw); err != nil {
		// Checkov can output a JSON array when scanning multiple files.
		// Try unwrapping an array with one element.
		var arr []json.RawMessage
		if json.Unmarshal(output, &arr) == nil && len(arr) > 0 {
			return parseCheckovOutput(arr[0], start)
		}
		result.Error = fmt.Errorf("checkov: parsing JSON output: %w", err)
		return result
	}

	result.Passed = len(raw.Results.PassedChecks)
	result.Failed = len(raw.Results.FailedChecks)

	for _, fc := range raw.Results.FailedChecks {
		severity, ok := checkovSeverity[fc.CheckID]
		if !ok {
			// Check prefix patterns for unknown IDs
			upperID := strings.ToUpper(fc.CheckID)
			switch {
			case strings.HasPrefix(upperID, "CKV_AWS_"):
				severity = "MEDIUM"
			case strings.HasPrefix(upperID, "CKV2_AWS_"):
				severity = "MEDIUM"
			default:
				severity = "MEDIUM"
			}
		}

		line := 0
		if len(fc.FileLineRange) > 0 {
			line = fc.FileLineRange[0]
		}

		// Look up a human-readable message; fall back to the check ID if unknown.
		msg, ok := checkovMessages[fc.CheckID]
		if !ok {
			msg = fc.CheckID
		}

		result.Findings = append(result.Findings, Finding{
			ID:       fc.CheckID,
			Severity: severity,
			Message:  msg,
			Resource: fc.Resource,
			Line:     line,
		})
	}

	return result
}
