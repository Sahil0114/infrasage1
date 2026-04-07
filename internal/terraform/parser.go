package terraform

import (
	"regexp"
	"strings"
)

// hclFencePattern matches markdown code fences with optional language hints:
// ```hcl, ```terraform, ```tf, or plain ```
var hclFencePattern = regexp.MustCompile("(?s)```(?:hcl|terraform|tf)?\\n?(.*?)```")

// ExtractHCL extracts clean Terraform HCL from a raw LLM response string.
// It handles five cases:
//  1. Fenced with language hint: ```hcl ... ```
//  2. Fenced without hint:       ``` ... ```
//  3. Raw HCL (no fencing, starts with an HCL keyword)
//  4. Explanation then raw HCL  (prose prefix, HCL follows)
//  5. Explanation then fenced HCL
//
// Returns the extracted HCL string, or "" if no valid HCL could be found.
func ExtractHCL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	// Cases 1, 2, 5 — response contains markdown code fences.
	// The regex captures everything between the first opening fence and the
	// first closing fence, regardless of language hint.
	if matches := hclFencePattern.FindStringSubmatch(trimmed); len(matches) > 1 {
		content := strings.TrimSpace(matches[1])
		if content != "" {
			return content
		}
	}

	// Cases 3 and 4 — no fences present.
	// Search for the first occurrence of a recognised HCL keyword and return
	// everything from that point forward. This correctly handles both:
	//   - Case 3: the string starts with HCL (idx == 0, returns whole string)
	//   - Case 4: the string has an explanation prefix (idx > 0, trims prose)
	lower := strings.ToLower(trimmed)
	for _, keyword := range []string{"terraform {", "resource \"", "provider \""} {
		idx := strings.Index(lower, keyword)
		if idx >= 0 {
			return strings.TrimSpace(trimmed[idx:])
		}
	}

	// Nothing recognisable found — the model returned pure prose.
	return ""
}
