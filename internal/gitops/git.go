package gitops

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"time"
)

// CommitAndPush creates a new git branch, stages the given file, commits it,
// and pushes the branch to the remote origin.
//
// Parameters:
//   - file:    path to the file to stage and commit (relative to the repo root)
//   - branch:  name of the branch to create and push (e.g. "infrasage/add-s3-bucket")
//   - message: git commit message
//
// The working directory for all git commands is determined by the file's location.
// Requires git to be installed and the current directory to be inside a git repository.
func CommitAndPush(file, branch, message string, force bool) error {
	slog.Debug("gitops CommitAndPush", "file", file, "branch", branch)

	// Ensure git is available on PATH.
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git not found in PATH.\nFix: install git with 'brew install git' or 'xcode-select --install'")
	}

	// 1. Create and checkout a new branch.
	if err := gitRun("checkout", "-b", branch); err != nil {
		// Branch might already exist — try checking it out instead.
		slog.Debug("checkout -b failed, trying checkout", "branch", branch, "err", err)
		if err2 := gitRun("checkout", branch); err2 != nil {
			return fmt.Errorf("gitops CommitAndPush: creating branch %q: %w\nFix: ensure you are inside a git repository and the branch does not already point to a conflicting commit.", branch, err)
		}
	}

	// 2. Stage the file.
	ignored, err := gitIsIgnored(file)
	if err != nil {
		return fmt.Errorf("gitops CommitAndPush: checking gitignore for %q: %w\nFix: run 'git check-ignore -v %s' to see why it is ignored.", file, err, file)
	}
	if ignored && !force {
		return fmt.Errorf("gitops CommitAndPush: %q is ignored by git.\nFix: remove it from .gitignore or re-run deploy with --force.", file)
	}

	addArgs := []string{"add", file}
	if force {
		addArgs = []string{"add", "-f", file}
	}
	if err := gitRun(addArgs...); err != nil {
		return fmt.Errorf("gitops CommitAndPush: staging file %q: %w", file, err)
	}

	// 3. Commit.
	if err := gitRun("commit", "-m", message); err != nil {
		return fmt.Errorf("gitops CommitAndPush: committing: %w\nFix: ensure git user.name and user.email are configured ('git config --global user.email you@example.com').", err)
	}

	// 4. Push to origin.
	if err := gitRun("push", "--set-upstream", "origin", branch); err != nil {
		return fmt.Errorf("gitops CommitAndPush: pushing branch %q to origin: %w\nFix: ensure the remote 'origin' is set and you have push access (check GITHUB_TOKEN if using HTTPS).", branch, err)
	}

	slog.Debug("gitops CommitAndPush: success", "branch", branch)
	return nil
}

func gitIsIgnored(file string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "check-ignore", "-q", file)
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return false, fmt.Errorf("git check-ignore -q %s timed out after 10s", file)
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			switch exitErr.ExitCode() {
			case 1:
				return false, nil
			case 0:
				return true, nil
			default:
				return false, fmt.Errorf("git check-ignore -q %s failed: %w", file, err)
			}
		}
		return false, fmt.Errorf("git check-ignore -q %s failed: %w", file, err)
	}

	return true, nil
}

// gitRun executes a git sub-command with a 60-second timeout, streaming
// stdout and stderr to the terminal so the user can see git's output.
func gitRun(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("git %v timed out after 60s", args)
		}
		return fmt.Errorf("git %v: %w", args, err)
	}
	return nil
}
