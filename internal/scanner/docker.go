package scanner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// scanDir returns the host-side directory used to share files with scanner containers.
// Defaults to /tmp/infrasage-scans but can be overridden via INFRASAGE_SCAN_DIR.
func scanDir() string {
	if v := os.Getenv("INFRASAGE_SCAN_DIR"); v != "" {
		return v
	}
	return "/tmp/infrasage-scans"
}

// copyToScanDir copies the given Terraform file into the shared scan directory
// so that scanner containers (which mount the directory as /scans) can read it.
// Returns the path of the copied file inside the scan directory.
func copyToScanDir(tfFile string) (string, error) {
	dir := scanDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("scanner copyToScanDir: creating scan dir %s: %w", dir, err)
	}

	src, err := os.Open(tfFile)
	if err != nil {
		return "", fmt.Errorf("scanner copyToScanDir: opening source file %s: %w", tfFile, err)
	}
	defer src.Close()

	destPath := filepath.Join(dir, filepath.Base(tfFile))
	dst, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("scanner copyToScanDir: creating destination %s: %w", destPath, err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return "", fmt.Errorf("scanner copyToScanDir: copying file: %w", err)
	}

	return destPath, nil
}

// containerPath converts a host-side scan-dir path to the equivalent path
// inside the scanner containers, where the scan dir is mounted as /scans.
func containerPath(hostPath string) string {
	base := filepath.Base(hostPath)
	return "/scans/" + base
}

// dockerExec runs a command inside the named container using `docker exec`.
// It captures stdout and returns it along with any error.
//
// Important: security scanner tools exit with code 1 when they find violations.
// This is expected behaviour, not an error. dockerExec handles this by returning
// the captured stdout even when the exit code is non-zero — as long as there is
// output to parse. The caller is responsible for distinguishing a parse-able
// non-zero exit from a genuine failure (e.g. container not found).
func dockerExec(containerName string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	fullArgs := append([]string{"exec", containerName}, args...)
	cmd := exec.CommandContext(ctx, "docker", fullArgs...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("docker exec %s: timed out after 60s", containerName)
		}

		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// Non-zero exit is expected when scanner tools find violations.
			// If we have stdout output, return it so the caller can parse it.
			if stdout.Len() > 0 {
				return stdout.Bytes(), nil
			}
			// No stdout means a real failure (container not running, command not found, etc.)
			stderrMsg := stderr.String()
			if stderrMsg == "" {
				stderrMsg = exitErr.Error()
			}
			return nil, fmt.Errorf("docker exec %s: exit code %d: %s",
				containerName, exitErr.ExitCode(), stderrMsg)
		}

		// Non-ExitError — likely the docker binary itself failed or container not found.
		return nil, fmt.Errorf("docker exec %s: %w\nFix: run 'infrasage stack up' to start scanner containers.", containerName, err)
	}

	return stdout.Bytes(), nil
}

// findJSONStart trims any non-JSON prefix from output.
// Some tools (e.g. Checkov) print warning lines before the JSON object.
func findJSONStart(data []byte) []byte {
	idx := bytes.IndexByte(data, '{')
	if idx < 0 {
		return data
	}
	return data[idx:]
}
