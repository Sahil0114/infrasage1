package gitops

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"time"
)

const githubAPIBase = "https://api.github.com"

// CreatePR opens a pull request on GitHub for the given branch.
// It requires GITHUB_TOKEN and GITHUB_REPO (e.g. "Sahil0114/infrasage1") to be set.
// Returns the HTML URL of the created PR.
func CreatePR(branch, title, body string) (string, error) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return "", fmt.Errorf("GITHUB_TOKEN environment variable is not set.\nFix: add GITHUB_TOKEN=<your personal access token> to your .env file.")
	}

	repo := os.Getenv("GITHUB_REPO")
	if repo == "" {
		return "", fmt.Errorf("GITHUB_REPO environment variable is not set.\nFix: add GITHUB_REPO=owner/repo-name to your .env file.")
	}

	base := os.Getenv("GITHUB_DEFAULT_BRANCH")
	if base == "" {
		base = "main"
	}

	slog.Debug("creating GitHub PR",
		"repo", repo,
		"head", branch,
		"base", base,
		"title", title,
	)

	payload := map[string]any{
		"title": title,
		"body":  body,
		"head":  branch,
		"base":  base,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("gitops CreatePR: marshaling request body: %w", err)
	}

	url := fmt.Sprintf("%s/repos/%s/pulls", githubAPIBase, repo)

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("gitops CreatePR: building HTTP request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("gitops CreatePR: HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("gitops CreatePR: reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		// Try to extract GitHub's error message for a better user message.
		var ghErr struct {
			Message string `json:"message"`
			Errors  []struct {
				Message string `json:"message"`
			} `json:"errors"`
		}
		if json.Unmarshal(respBody, &ghErr) == nil && ghErr.Message != "" {
			details := ghErr.Message
			for _, e := range ghErr.Errors {
				if e.Message != "" {
					details += ": " + e.Message
				}
			}
			return "", fmt.Errorf("gitops CreatePR: GitHub API returned %d: %s\nFix: ensure GITHUB_TOKEN has 'repo' scope and the branch '%s' has been pushed.", resp.StatusCode, details, branch)
		}
		return "", fmt.Errorf("gitops CreatePR: GitHub API returned HTTP %d\nFix: ensure GITHUB_TOKEN is valid and GITHUB_REPO is correct (format: owner/repo).", resp.StatusCode)
	}

	var pr struct {
		HTMLURL string `json:"html_url"`
		Number  int    `json:"number"`
	}
	if err := json.Unmarshal(respBody, &pr); err != nil {
		return "", fmt.Errorf("gitops CreatePR: parsing GitHub response: %w", err)
	}

	if pr.HTMLURL == "" {
		return "", fmt.Errorf("gitops CreatePR: GitHub response did not contain a PR URL")
	}

	slog.Debug("GitHub PR created", "url", pr.HTMLURL, "number", pr.Number)
	return pr.HTMLURL, nil
}

// PollWorkflowRuns polls GitHub Actions workflow runs for the given head branch,
// streaming human-readable status messages to statusFn.
// It polls every pollInterval for up to maxWait then sends a timeout message.
func PollWorkflowRuns(branch string, maxWait, pollInterval time.Duration, statusFn func(string)) {
	token := os.Getenv("GITHUB_TOKEN")
	repo := os.Getenv("GITHUB_REPO")
	if token == "" || repo == "" {
		statusFn("⚠️  CI polling skipped — GITHUB_TOKEN/GITHUB_REPO not configured")
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}
	deadline := time.Now().Add(maxWait)
	var lastRunID int
	var lastRunStatus string
	started := false

	for time.Now().Before(deadline) {
		time.Sleep(pollInterval)

		apiURL := fmt.Sprintf("%s/repos/%s/actions/runs?branch=%s&per_page=5", githubAPIBase, repo, url.QueryEscape(branch))
		req, err := http.NewRequest(http.MethodGet, apiURL, nil)
		if err != nil {
			continue
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var result struct {
			WorkflowRuns []struct {
				ID         int    `json:"id"`
				Status     string `json:"status"`
				Conclusion string `json:"conclusion"`
				HTMLURL    string `json:"html_url"`
				Name       string `json:"name"`
			} `json:"workflow_runs"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			slog.Debug("PollWorkflowRuns: failed to parse response", "err", err)
			continue
		}
		if len(result.WorkflowRuns) == 0 {
			if !started {
				statusFn("⏳ Waiting for GitHub Actions CI to start...")
				started = true
			}
			continue
		}
		started = true

		run := result.WorkflowRuns[0]

		// Skip if this run + status combination was already reported.
		if run.ID == lastRunID && run.Status == lastRunStatus {
			continue
		}
		lastRunID = run.ID
		lastRunStatus = run.Status

		switch run.Status {
		case "queued":
			statusFn(fmt.Sprintf("⏳ CI queued — %s", run.HTMLURL))
		case "in_progress":
			statusFn(fmt.Sprintf("🔄 CI running (%s) — %s", run.Name, run.HTMLURL))
		case "completed":
			switch run.Conclusion {
			case "success":
				statusFn(fmt.Sprintf("✅ CI passed (%s) — %s", run.Name, run.HTMLURL))
			case "failure":
				statusFn(fmt.Sprintf("❌ CI failed (%s) — check Actions tab: %s", run.Name, run.HTMLURL))
			case "cancelled":
				statusFn(fmt.Sprintf("⏹️  CI cancelled (%s) — %s", run.Name, run.HTMLURL))
			default:
				statusFn(fmt.Sprintf("CI %s (%s) — %s", run.Conclusion, run.Name, run.HTMLURL))
			}
			return
		}
	}

	statusFn("⏱️  CI polling timed out — check GitHub Actions manually")
}
