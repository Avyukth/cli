package strategy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"entire.io/cli/cmd/entire/cli/paths"
)

// Hook marker used to identify Entire CLI hooks
const (
	entireHookMarker = "Entire CLI hooks"
	settingsFile     = ".entire/settings.json"
)

// GetGitDir returns the actual git directory path by delegating to git itself.
// This handles both regular repositories and worktrees, and inherits git's
// security validation for gitdir references.
func GetGitDir() (string, error) {
	return getGitDirInPath(".")
}

// getGitDirInPath returns the git directory for a repository at the given path.
// It delegates to `git rev-parse --git-dir` to leverage git's own validation.
func getGitDirInPath(dir string) (string, error) {
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--git-dir")
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return "", errors.New("not a git repository")
	}

	gitDir := strings.TrimSpace(string(output))

	// git rev-parse --git-dir returns relative paths from the working directory,
	// so we need to make it absolute if it isn't already
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(dir, gitDir)
	}

	return filepath.Clean(gitDir), nil
}

// IsGitHookInstalled checks if all generic Entire CLI hooks are installed.
func IsGitHookInstalled() bool {
	gitDir, err := GetGitDir()
	if err != nil {
		return false
	}
	hooks := []string{"prepare-commit-msg", "commit-msg", "post-commit", "pre-push"}
	for _, hook := range hooks {
		hookPath := filepath.Join(gitDir, "hooks", hook)
		data, err := os.ReadFile(hookPath) //nolint:gosec // Path is constructed from constants
		if err != nil {
			return false
		}
		if !strings.Contains(string(data), entireHookMarker) {
			return false
		}
	}
	return true
}

// InstallGitHook installs generic git hooks that delegate to `entire hook` commands.
// These hooks work with any strategy - the strategy is determined at runtime.
// If silent is true, no output is printed.
func InstallGitHook(silent bool) error {
	gitDir, err := GetGitDir()
	if err != nil {
		return err
	}

	hooksDir := filepath.Join(gitDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil { //nolint:gosec // Git hooks require executable permissions
		return fmt.Errorf("failed to create hooks directory: %w", err)
	}

	// Determine command prefix based on local_dev setting
	var cmdPrefix string
	if isLocalDev() {
		cmdPrefix = "go run ./cmd/entire/main.go"
	} else {
		cmdPrefix = "entire"
	}

	// Install prepare-commit-msg hook
	// $1 = commit message file, $2 = source (message, template, merge, squash, commit, or empty)
	prepareCommitMsgPath := filepath.Join(hooksDir, "prepare-commit-msg")
	prepareCommitMsgContent := fmt.Sprintf(`#!/bin/sh
# %s
%s hooks git prepare-commit-msg "$1" "$2" 2>/dev/null || true
`, entireHookMarker, cmdPrefix)

	if err := writeHookFile(prepareCommitMsgPath, prepareCommitMsgContent); err != nil {
		return fmt.Errorf("failed to install prepare-commit-msg hook: %w", err)
	}

	// Install commit-msg hook
	commitMsgPath := filepath.Join(hooksDir, "commit-msg")
	commitMsgContent := fmt.Sprintf(`#!/bin/sh
# %s
# Commit-msg hook: strip trailer if no user content (allows aborting empty commits)
%s hooks git commit-msg "$1" || exit 1
`, entireHookMarker, cmdPrefix)

	if err := writeHookFile(commitMsgPath, commitMsgContent); err != nil {
		return fmt.Errorf("failed to install commit-msg hook: %w", err)
	}

	// Install post-commit hook
	postCommitPath := filepath.Join(hooksDir, "post-commit")
	postCommitContent := fmt.Sprintf(`#!/bin/sh
# %s
# Post-commit hook: condense session data if commit has Entire-Checkpoint trailer
%s hooks git post-commit 2>/dev/null || true
`, entireHookMarker, cmdPrefix)

	if err := writeHookFile(postCommitPath, postCommitContent); err != nil {
		return fmt.Errorf("failed to install post-commit hook: %w", err)
	}

	// Install pre-push hook
	prePushPath := filepath.Join(hooksDir, "pre-push")
	prePushContent := fmt.Sprintf(`#!/bin/sh
# %s
# Pre-push hook: push session logs alongside user's push
# $1 is the remote name (e.g., "origin")
%s hooks git pre-push "$1" || true
`, entireHookMarker, cmdPrefix)

	if err := writeHookFile(prePushPath, prePushContent); err != nil {
		return fmt.Errorf("failed to install pre-push hook: %w", err)
	}

	if !silent {
		fmt.Println("✓ Installed git hooks (prepare-commit-msg, commit-msg, post-commit, pre-push)")
		fmt.Println("  Hooks delegate to the current strategy at runtime")
	}

	return nil
}

// writeHookFile writes a hook file with executable permissions.
func writeHookFile(path, content string) error {
	// Git hooks must be executable (0o755)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil { //nolint:gosec // Git hooks require executable permissions
		return fmt.Errorf("failed to write hook file %s: %w", path, err)
	}
	return nil
}

// isLocalDev reads the local_dev setting from .entire/settings.json
// Works correctly from any subdirectory within the repository.
func isLocalDev() bool {
	settingsFileAbs, err := paths.AbsPath(settingsFile)
	if err != nil {
		settingsFileAbs = settingsFile // Fallback to relative
	}
	data, err := os.ReadFile(settingsFileAbs) //nolint:gosec // path is from AbsPath or constant
	if err != nil {
		return false
	}
	var settings struct {
		LocalDev bool `json:"local_dev"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return false
	}
	return settings.LocalDev
}
