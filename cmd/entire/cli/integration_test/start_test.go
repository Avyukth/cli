//go:build integration

package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"entire.io/cli/cmd/entire/cli/paths"
	"entire.io/cli/cmd/entire/cli/strategy"
)

// TestStartDefaultMode tests creating a branch and checking it out (default behavior).
func TestStartDefaultMode(t *testing.T) {
	t.Parallel()
	env := NewTestEnv(t)
	env.InitRepo()
	env.InitEntire("manual-commit")

	env.WriteFile("README.md", "# Test")
	env.GitAdd("README.md", ".entire")
	env.GitCommit("Initial commit")

	// Run entire start test-feature (use master as base since go-git creates "master" not "main")
	output, err := env.runEntireCmd("start", "test-feature", "--base", "master")
	if err != nil {
		t.Fatalf("start command failed: %v\nOutput: %s", err, output)
	}

	// Verify we're on the new branch
	currentBranch := env.GetCurrentBranch()
	if currentBranch != "feature/test-feature" {
		t.Errorf("expected branch feature/test-feature, got %s", currentBranch)
	}

	// Verify branch exists
	if !env.BranchExists("feature/test-feature") {
		t.Error("branch feature/test-feature should exist")
	}
}

// TestStartWorktreeFromMainRepo tests creating a worktree from the main repo.
func TestStartWorktreeFromMainRepo(t *testing.T) {
	t.Parallel()
	env := NewTestEnv(t)
	env.InitRepo()
	env.InitEntire("manual-commit")

	env.WriteFile("README.md", "# Test")
	env.GitAdd("README.md", ".entire")
	env.GitCommit("Initial commit")

	worktreePath := filepath.Join(env.RepoDir, ".worktrees", "worktree-feature")

	// Clean up worktree at end
	t.Cleanup(func() {
		cmd := exec.Command("git", "worktree", "remove", worktreePath, "--force")
		cmd.Dir = env.RepoDir
		_ = cmd.Run() //nolint:errcheck
	})

	// Run entire start --worktree (use master as base since go-git creates "master" not "main")
	output, err := env.runEntireCmd("start", "worktree-feature", "--worktree", "--base", "master")
	if err != nil {
		t.Fatalf("start --worktree failed: %v\nOutput: %s", err, output)
	}

	// Verify worktree directory exists
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		t.Errorf("worktree should exist at %s", worktreePath)
	}

	// Verify .gitignore contains .worktrees
	gitignore := env.ReadFile(".gitignore")
	if !strings.Contains(gitignore, ".worktrees") {
		t.Error(".gitignore should contain .worktrees")
	}

	// Verify branch exists
	if !env.BranchExists("feature/worktree-feature") {
		t.Error("branch feature/worktree-feature should exist")
	}
}

// TestStartSiblingWorktreeFromWorktree tests creating a sibling worktree
// from inside an existing worktree.
//
// NOTE: This test uses os.Chdir() because it tests the strategy.IsInsideWorktree()
// function which reads the current working directory. This is intentional and
// the test cannot be parallelized. The CLI subprocess execution uses cmd.Dir
// so it's not affected by the chdir.
func TestStartSiblingWorktreeFromWorktree(t *testing.T) {
	env := NewTestEnv(t)
	env.InitRepo()
	env.InitEntire("manual-commit")

	env.WriteFile("README.md", "# Test")
	env.GitAdd("README.md", ".entire")
	env.GitCommit("Initial commit")

	// Create first worktree using native git
	worktree1 := filepath.Join(env.RepoDir, ".worktrees", "feature-one")
	worktree2 := filepath.Join(env.RepoDir, ".worktrees", "feature-two")

	if err := os.MkdirAll(filepath.Dir(worktree1), 0o755); err != nil {
		t.Fatalf("failed to create .worktrees dir: %v", err)
	}

	cmd := exec.Command("git", "worktree", "add", "-b", "feature/feature-one", worktree1, "HEAD")
	cmd.Dir = env.RepoDir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to create first worktree: %v\nOutput: %s", err, output)
	}

	// Clean up worktrees at end
	t.Cleanup(func() {
		cmd := exec.Command("git", "worktree", "remove", worktree1, "--force")
		cmd.Dir = env.RepoDir
		_ = cmd.Run() //nolint:errcheck

		cmd = exec.Command("git", "worktree", "remove", worktree2, "--force")
		cmd.Dir = env.RepoDir
		_ = cmd.Run() //nolint:errcheck
	})

	// Verify we're inside a worktree when we chdir there
	originalWd, _ := os.Getwd()
	if err := os.Chdir(worktree1); err != nil {
		t.Fatalf("failed to chdir to worktree: %v", err)
	}
	defer func() { _ = os.Chdir(originalWd) }()

	if !strategy.IsInsideWorktree() {
		t.Fatal("expected IsInsideWorktree() to return true")
	}

	// Copy .entire settings to worktree
	worktreeEntireDir := filepath.Join(worktree1, ".entire")
	if err := os.MkdirAll(worktreeEntireDir, 0o755); err != nil {
		t.Fatalf("failed to create .entire in worktree: %v", err)
	}
	settingsSrc := filepath.Join(env.RepoDir, ".entire", paths.SettingsFileName)
	settingsDst := filepath.Join(worktreeEntireDir, paths.SettingsFileName)
	settingsData, err := os.ReadFile(settingsSrc)
	if err != nil {
		t.Fatalf("failed to read settings: %v", err)
	}
	if err := os.WriteFile(settingsDst, settingsData, 0o644); err != nil {
		t.Fatalf("failed to write settings to worktree: %v", err)
	}

	// Create sibling worktree using the start command FROM INSIDE first worktree
	output, err := env.runEntireCmdInDir(worktree1, "start", "feature-two", "--worktree", "--base", "master")
	if err != nil {
		t.Fatalf("start --worktree from inside worktree failed: %v\nOutput: %s", err, output)
	}

	// Verify sibling was created in main repo's .worktrees (not nested)
	if _, err := os.Stat(worktree2); os.IsNotExist(err) {
		t.Errorf("sibling worktree should exist at %s", worktree2)
	}

	// Verify branch exists
	if !env.BranchExists("feature/feature-two") {
		t.Error("branch feature/feature-two should exist")
	}
}

// TestStartMissingArgument tests that the command errors without a feature name.
func TestStartMissingArgument(t *testing.T) {
	t.Parallel()
	env := NewTestEnv(t)
	env.InitRepo()
	env.InitEntire("manual-commit")

	env.WriteFile("README.md", "# Test")
	env.GitAdd("README.md", ".entire")
	env.GitCommit("Initial commit")

	// Run entire start (no name)
	_, err := env.runEntireCmd("start")
	if err == nil {
		t.Error("expected error when no feature name provided")
	}
}

// runEntireCmd builds and runs the entire CLI with the given arguments.
func (env *TestEnv) runEntireCmd(args ...string) (string, error) {
	return env.runEntireCmdInDir(env.RepoDir, args...)
}

// runEntireCmdInDir runs the entire CLI in a specific directory using the shared test binary.
func (env *TestEnv) runEntireCmdInDir(dir string, args ...string) (string, error) {
	env.T.Helper()

	cmd := exec.Command(getTestBinary(), args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"ENTIRE_TEST_CLAUDE_PROJECT_DIR="+env.ClaudeProjectDir,
	)

	output, err := cmd.CombinedOutput()
	return string(output), err
}
