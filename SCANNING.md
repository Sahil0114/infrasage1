# SCANNING.md — Security Scanner Integration Instructions

> Read this before writing anything in `internal/scanner/`.

---

## Overview of the Three Tools

| Tool | Made By | What It Checks | Output Format |
|---|---|---|---|
| Checkov | Bridgecrew / Palo Alto | AWS misconfigs, CIS benchmarks | JSON |
| tfsec | Aqua Security | Terraform-specific security rules | JSON |
| Terrascan | Accurics / Tenable | Cloud security policies (OPA-based) | JSON |

**Why three tools instead of one?**
Each tool has a different ruleset. Some findings are caught by one but not others. Using all three gives maximum security coverage and makes the project more impressive for evaluation.

Example of unique findings:
- Checkov finds: S3 bucket missing access logging
- tfsec finds: S3 bucket missing MFA delete
- Terrascan finds: S3 bucket object-level logging not enabled

Together they give a complete picture.

---

## Scan Flow — Detailed

```
infrasage ask "..."
       │
       ▼
infra.tf written to disk
       │
       ▼
internal/scanner/runner.go: RunAll("infra.tf")
       │
       ├── copies infra.tf to /tmp/infrasage-scans/infra.tf
       │
       ├── goroutine 1: internal/scanner/checkov.go: RunCheckov("infra.tf")
       │       │
       │       ├── docker exec infrasage-checkov checkov -f /scans/infra.tf --output json --quiet
       │       ├── parse JSON output
       │       └── return ScanResult{Tool:"checkov", Passed:12, Failed:2, Findings:[...]}
       │
       ├── goroutine 2: internal/scanner/tfsec.go: RunTfsec("infra.tf")
       │       │
       │       ├── docker exec infrasage-tfsec tfsec /scans/infra.tf --format json --no-colour
       │       ├── parse JSON output
       │       └── return ScanResult{Tool:"tfsec", Passed:0, Failed:1, Findings:[...]}
       │
       └── goroutine 3: internal/scanner/terrascan.go: RunTerrascan("infra.tf")
               │
               ├── docker exec infrasage-terrascan terrascan scan -i terraform -f /scans/infra.tf -o json
               ├── parse JSON output
               └── return ScanResult{Tool:"terrascan", Passed:0, Failed:1, Findings:[...]}

       ← all 3 goroutines finish ←
       │
       ▼
Report{File:"infra.tf", Results:[checkov, tfsec, terrascan]}
       │
       ▼
Report.Print() → renders table to stdout
```

---

## Checkov JSON Output — Exact Structure to Parse

Checkov outputs one large JSON object. The relevant fields:

```json
{
  "results": {
    "passed_checks": [
      {
        "check_id": "CKV_AWS_19",
        "check_type": "terraform",
        "resource": "aws_s3_bucket.example",
        "file_path": "/scans/infra.tf",
        "file_line_range": [1, 20]
      }
    ],
    "failed_checks": [
      {
        "check_id": "CKV_AWS_18",
        "check_type": "terraform",
        "resource": "aws_s3_bucket.example",
        "check_result": {
          "result": "FAILED",
          "evaluated_keys": {}
        },
        "file_path": "/scans/infra.tf",
        "file_line_range": [1, 20]
      }
    ]
  }
}
```

**How to map to ScanResult:**
- `Passed = len(passed_checks)`
- `Failed = len(failed_checks)`
- For each failed check: `Finding{ID: check_id, Resource: resource, Line: file_line_range[0]}`
- Severity: Checkov doesn't include severity in the check output by default. Use this mapping:
  ```go
  var checkovSeverity = map[string]string{
      "CKV_AWS_18": "MEDIUM",  // S3 access logging
      "CKV_AWS_19": "HIGH",    // S3 encryption
      "CKV_AWS_20": "HIGH",    // S3 public access
      "CKV_AWS_21": "MEDIUM",  // S3 versioning
      // default: "MEDIUM" for unknown check IDs
  }
  ```

**Watch out:** Checkov sometimes outputs warning lines before the JSON. Strip non-JSON prefix by finding the first `{` character in the output.

---

## tfsec JSON Output — Exact Structure to Parse

tfsec outputs a JSON object with a `results` array:

```json
{
  "results": [
    {
      "rule_id": "AVD-AWS-0086",
      "rule_description": "Bucket does not have MFA delete enabled.",
      "severity": "HIGH",
      "warning": false,
      "location": {
        "filename": "/scans/infra.tf",
        "start_line": 5,
        "end_line": 10
      },
      "affected_resource": "aws_s3_bucket.example"
    }
  ]
}
```

**How to map to ScanResult:**
- `Failed = len(results)`
- `Passed = 0` (tfsec only reports failures)
- Severity is already in the JSON — use it directly
- `Finding{ID: rule_id, Message: rule_description, Severity: severity, Resource: affected_resource, Line: location.start_line}`

**Watch out:** If there are no findings, tfsec may output `{"results": null}` instead of `{"results": []}`. Handle null by treating it as an empty array.

---

## Terrascan JSON Output — Exact Structure to Parse

Terrascan outputs:

```json
{
  "results": {
    "violations": [
      {
        "rule_name": "s3ObjectLevelLogging",
        "rule_id": "AC_AWS_0207",
        "severity": "MEDIUM",
        "category": "LOGGING",
        "description": "Ensure that Object-level logging for write events is enabled for S3 bucket.",
        "resource_name": "aws_s3_bucket.example",
        "resource_type": "aws_s3_bucket",
        "module_name": "root",
        "file": "/scans/infra.tf",
        "line": 1
      }
    ],
    "skipped_violations": [],
    "passed_rules": []
  }
}
```

**How to map to ScanResult:**
- `Failed = len(violations)`
- `Passed = len(passed_rules)` (use this if available)
- `Finding{ID: rule_id, Message: description, Severity: severity, Resource: resource_name, Line: line}`

---

## Report Printer — Exact Visual Format

The `Print()` method on `*Report` must output this exact format.
Use `fmt.Printf` with the exact Unicode box-drawing characters shown.

```
╔══════════════════════════════════════════════════╗
║         InfraSage Security Scan Report           ║
╠══════════════════════════════════════════════════╣
║  File: infra.tf                                  ║
╠══════════╦═════════╦═════════╦═══════════════════╣
║ Scanner  ║  Pass   ║  Fail   ║  Status           ║
╠══════════╬═════════╬═════════╬═══════════════════╣
║ Checkov  ║   12    ║    2    ║  ⚠  WARN         ║
║ tfsec    ║    0    ║    0    ║  ✓  PASS         ║
║ Terrascan║    0    ║    1    ║  ⚠  WARN         ║
╠══════════╩═════════╩═════════╩═══════════════════╣
║  FINDINGS (3 total):                             ║
╠══════════════════════════════════════════════════╣
║  [CKV_AWS_18]  MEDIUM   S3 access logging off   ║
║  [CKV_AWS_21]  MEDIUM   S3 versioning disabled  ║
║  [AC_AWS_0207] MEDIUM   Object-level logging    ║
╚══════════════════════════════════════════════════╝
```

**Status column logic:**
```go
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
```

**Findings section:** Only show if `TotalFailed() > 0`. Sort findings by severity: CRITICAL → HIGH → MEDIUM → LOW.

---

## Handling Scanner Errors Gracefully

If a scanner container is not running, `docker exec` will fail. The CLI must not crash. Handle it:

```
║ Checkov  ║    -    ║    -    ║  ✗  ERROR        ║
```

And below the table:
```
⚠  Checkov error: container infrasage-checkov is not running.
   Fix: run 'infrasage stack up' to start scanner containers.
```

This means the user gets useful output even with partial scanner availability.

---

## Severity Normalization

Each tool uses slightly different severity naming. Normalize to these four values:

| Normalized | Checkov equivalent | tfsec equivalent | Terrascan equivalent |
|---|---|---|---|
| `CRITICAL` | `CRITICAL` | `CRITICAL` | `CRITICAL` |
| `HIGH` | `HIGH` | `HIGH` | `HIGH` |
| `MEDIUM` | `MEDIUM` | `MEDIUM` | `MEDIUM` |
| `LOW` | `LOW` | `LOW` | `LOW` |

If a tool uses a different name (e.g., `WARNING`, `INFO`), map to `LOW`.

---

## Future: OPA Integration (Phase 2)

In Phase 2, a 4th scanner will be added: OPA (Open Policy Agent) with custom `.rego` policy files in `policies/`.

The `runner.go` is designed to support this. The `ScanResult` struct and `Report` struct will work unchanged.

When adding OPA:
1. Add `terrascan` container to compose (Terrascan bundles OPA)
2. Or run a standalone OPA container
3. Create `internal/scanner/opa.go` following the same pattern
4. Add `RunOPA(tfFile string) ScanResult` to the goroutines in `runner.go`

For Phase 1, this is out of scope. Document it but do not implement it.
