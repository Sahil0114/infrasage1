package gitops

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
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
