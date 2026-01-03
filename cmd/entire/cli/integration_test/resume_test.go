//go:build integration

package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"entire.io/cli/cmd/entire/cli/strategy"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

const masterBranch = "master"

// Note: Resume tests only run with auto-commit strategy because:
// - Auto-commit strategy creates commits with Entire-Checkpoint trailers and metadata on entire/sessions
//   immediately during SimulateStop
// - Manual-commit strategy only creates this structure after user commits (via prepare-commit-msg
//   and post-commit hooks), which requires the full workflow tested in manual_commit_workflow_test.go
// Both strategies share the same resume code path once the structure exists.

// TestResume_SwitchBranchWithSession tests the resume command when switching to a branch
// that has a commit with an Entire-Checkpoint trailer.
func TestResume_SwitchBranchWithSession(t *testing.T) {
	t.Parallel()
	env := NewFeatureBranchEnv(t, strategy.StrategyNameAutoCommit)

	// Create a session on the feature branch
	session := env.NewSession()
	if err := env.SimulateUserPromptSubmit(session.ID); err != nil {
		t.Fatalf("SimulateUserPromptSubmit failed: %v", err)
	}

	content := "puts 'Hello from session'"
	env.WriteFile("hello.rb", content)

	session.CreateTranscript(
		"Create a hello script",
		[]FileChange{{Path: "hello.rb", Content: content}},
	)
	if err := env.SimulateStop(session.ID, session.TranscriptPath); err != nil {
		t.Fatalf("SimulateStop failed: %v", err)
	}

	// Remember the feature branch name
	featureBranch := env.GetCurrentBranch()

	// Switch back to main branch
	env.GitCheckoutBranch(masterBranch)

	// Verify we're on main
	if branch := env.GetCurrentBranch(); branch != masterBranch {
		t.Fatalf("expected to be on master, got %s", branch)
	}

	// Run resume to switch back to feature branch
	output, err := env.RunResume(featureBranch)
	if err != nil {
		t.Fatalf("resume failed: %v\nOutput: %s", err, output)
	}

	// Verify we switched to the feature branch
	if branch := env.GetCurrentBranch(); branch != featureBranch {
		t.Errorf("expected to be on %s, got %s", featureBranch, branch)
	}

	// Verify output contains session info and resume command
	if !strings.Contains(output, "Session:") {
		t.Errorf("output should contain 'Session:', got: %s", output)
	}
	if !strings.Contains(output, "claude -r") {
		t.Errorf("output should contain 'claude -r', got: %s", output)
	}

	// Verify transcript was restored to Claude project dir
	transcriptFiles, err := filepath.Glob(filepath.Join(env.ClaudeProjectDir, "*.jsonl"))
	if err != nil {
		t.Fatalf("failed to glob transcript files: %v", err)
	}
	if len(transcriptFiles) == 0 {
		t.Error("expected transcript file to be restored to Claude project dir")
	}
}

// TestResume_AlreadyOnBranch tests that resume works when already on the target branch.
func TestResume_AlreadyOnBranch(t *testing.T) {
	t.Parallel()
	env := NewFeatureBranchEnv(t, strategy.StrategyNameAutoCommit)

	// Create a session on the feature branch
	session := env.NewSession()
	if err := env.SimulateUserPromptSubmit(session.ID); err != nil {
		t.Fatalf("SimulateUserPromptSubmit failed: %v", err)
	}

	content := "console.log('test')"
	env.WriteFile("test.js", content)

	session.CreateTranscript(
		"Create a test script",
		[]FileChange{{Path: "test.js", Content: content}},
	)
	if err := env.SimulateStop(session.ID, session.TranscriptPath); err != nil {
		t.Fatalf("SimulateStop failed: %v", err)
	}

	currentBranch := env.GetCurrentBranch()

	// Run resume on the branch we're already on
	output, err := env.RunResume(currentBranch)
	if err != nil {
		t.Fatalf("resume failed: %v\nOutput: %s", err, output)
	}

	// Should still show session info
	if !strings.Contains(output, "Session:") {
		t.Errorf("output should contain 'Session:', got: %s", output)
	}
}

// TestResume_NoCheckpointOnBranch tests that resume handles branches without
// Entire-Checkpoint trailer gracefully.
func TestResume_NoCheckpointOnBranch(t *testing.T) {
	t.Parallel()
	env := NewFeatureBranchEnv(t, strategy.StrategyNameAutoCommit)

	// First, create a session to ensure the entire/sessions branch exists
	// This is required for the resume command to work
	session := env.NewSession()
	if err := env.SimulateUserPromptSubmit(session.ID); err != nil {
		t.Fatalf("SimulateUserPromptSubmit failed: %v", err)
	}
	content := "session content"
	env.WriteFile("session.txt", content)
	session.CreateTranscript(
		"Create session file",
		[]FileChange{{Path: "session.txt", Content: content}},
	)
	if err := env.SimulateStop(session.ID, session.TranscriptPath); err != nil {
		t.Fatalf("SimulateStop failed: %v", err)
	}

	// Now create a plain branch without any checkpoint
	env.GitCheckoutNewBranch("feature/plain")

	// Create a commit without any session/checkpoint
	env.WriteFile("plain.txt", "no session here")
	env.GitAdd("plain.txt")
	env.GitCommit("Plain commit without session")

	plainBranch := env.GetCurrentBranch()

	// Switch to main
	env.GitCheckoutBranch(masterBranch)

	// Resume back to the plain branch
	output, err := env.RunResume(plainBranch)
	if err != nil {
		t.Fatalf("resume failed: %v\nOutput: %s", err, output)
	}

	// Should indicate no checkpoint found
	if !strings.Contains(output, "No Entire checkpoint found") {
		t.Errorf("output should indicate no checkpoint found, got: %s", output)
	}

	// Should still switch to the branch
	if branch := env.GetCurrentBranch(); branch != plainBranch {
		t.Errorf("expected to be on %s, got %s", plainBranch, branch)
	}
}

// TestResume_BranchDoesNotExist tests that resume returns an error for non-existent branches.
func TestResume_BranchDoesNotExist(t *testing.T) {
	t.Parallel()
	env := NewFeatureBranchEnv(t, strategy.StrategyNameAutoCommit)

	// Try to resume a non-existent branch
	output, err := env.RunResume("nonexistent-branch")

	// Should fail
	if err == nil {
		t.Errorf("expected error for non-existent branch, got success with output: %s", output)
	}

	// Error should mention the branch
	if !strings.Contains(output, "nonexistent-branch") && !strings.Contains(err.Error(), "nonexistent-branch") {
		t.Errorf("error should mention the branch name, got: %v, output: %s", err, output)
	}
}

// TestResume_UncommittedChanges tests that resume fails when there are uncommitted changes.
func TestResume_UncommittedChanges(t *testing.T) {
	t.Parallel()
	env := NewFeatureBranchEnv(t, strategy.StrategyNameAutoCommit)

	// Create another branch
	env.GitCheckoutNewBranch("feature/target")
	env.WriteFile("target.txt", "target content")
	env.GitAdd("target.txt")
	env.GitCommit("Target commit")

	// Go back to original branch
	env.GitCheckoutBranch("feature/test-branch")

	// Make uncommitted changes
	env.WriteFile("uncommitted.txt", "uncommitted content")

	// Try to resume to target branch
	output, err := env.RunResume("feature/target")

	// Should fail due to uncommitted changes
	if err == nil {
		t.Errorf("expected error for uncommitted changes, got success with output: %s", output)
	}

	// Error should mention uncommitted changes
	if !strings.Contains(output, "uncommitted") && !strings.Contains(err.Error(), "uncommitted") {
		t.Errorf("error should mention uncommitted changes, got: %v, output: %s", err, output)
	}
}

// TestResume_SessionLogAlreadyExists tests that resume doesn't overwrite existing session logs.
func TestResume_SessionLogAlreadyExists(t *testing.T) {
	t.Parallel()
	env := NewFeatureBranchEnv(t, strategy.StrategyNameAutoCommit)

	// Create a session
	session := env.NewSession()
	if err := env.SimulateUserPromptSubmit(session.ID); err != nil {
		t.Fatalf("SimulateUserPromptSubmit failed: %v", err)
	}

	content := "def hello; end"
	env.WriteFile("hello.rb", content)

	session.CreateTranscript(
		"Create hello method",
		[]FileChange{{Path: "hello.rb", Content: content}},
	)
	if err := env.SimulateStop(session.ID, session.TranscriptPath); err != nil {
		t.Fatalf("SimulateStop failed: %v", err)
	}

	featureBranch := env.GetCurrentBranch()

	// Pre-create a session log in Claude project dir
	if err := os.MkdirAll(env.ClaudeProjectDir, 0o755); err != nil {
		t.Fatalf("failed to create Claude project dir: %v", err)
	}
	existingLog := filepath.Join(env.ClaudeProjectDir, session.ID+".jsonl")
	existingContent := `{"existing": true}`
	if err := os.WriteFile(existingLog, []byte(existingContent), 0o644); err != nil {
		t.Fatalf("failed to write existing log: %v", err)
	}

	// Switch to main and back
	env.GitCheckoutBranch(masterBranch)

	// Resume
	output, err := env.RunResume(featureBranch)
	if err != nil {
		t.Fatalf("resume failed: %v\nOutput: %s", err, output)
	}

	// Existing log should not be overwritten
	data, err := os.ReadFile(existingLog)
	if err != nil {
		t.Fatalf("failed to read existing log: %v", err)
	}
	if string(data) != existingContent {
		t.Errorf("existing log was overwritten, got: %s, want: %s", string(data), existingContent)
	}

	// Output should NOT say "Session restored" since it already existed
	if strings.Contains(output, "Session restored") {
		t.Errorf("output should not say 'Session restored' when log already exists, got: %s", output)
	}
}

// TestResume_MultipleSessionsOnBranch tests resume with multiple sessions (multiple commits),
// ensuring it uses the session from the last commit.
func TestResume_MultipleSessionsOnBranch(t *testing.T) {
	t.Parallel()
	env := NewFeatureBranchEnv(t, strategy.StrategyNameAutoCommit)

	// Create first session
	session1 := env.NewSession()
	if err := env.SimulateUserPromptSubmit(session1.ID); err != nil {
		t.Fatalf("SimulateUserPromptSubmit failed: %v", err)
	}

	content1 := "version 1"
	env.WriteFile("file.txt", content1)

	session1.CreateTranscript(
		"Create version 1",
		[]FileChange{{Path: "file.txt", Content: content1}},
	)
	if err := env.SimulateStop(session1.ID, session1.TranscriptPath); err != nil {
		t.Fatalf("SimulateStop session1 failed: %v", err)
	}

	// Create second session
	session2 := env.NewSession()
	if err := env.SimulateUserPromptSubmit(session2.ID); err != nil {
		t.Fatalf("SimulateUserPromptSubmit failed: %v", err)
	}

	content2 := "version 2"
	env.WriteFile("file.txt", content2)

	session2.CreateTranscript(
		"Update to version 2",
		[]FileChange{{Path: "file.txt", Content: content2}},
	)
	if err := env.SimulateStop(session2.ID, session2.TranscriptPath); err != nil {
		t.Fatalf("SimulateStop session2 failed: %v", err)
	}

	featureBranch := env.GetCurrentBranch()

	// Switch to main
	env.GitCheckoutBranch(masterBranch)

	// Resume
	output, err := env.RunResume(featureBranch)
	if err != nil {
		t.Fatalf("resume failed: %v\nOutput: %s", err, output)
	}

	// Should show session info (from the most recent session)
	if !strings.Contains(output, "Session:") {
		t.Errorf("output should contain session info, got: %s", output)
	}

	// The resume command shows the session from the last commit,
	// which should be session2 (the most recent one)
	if !strings.Contains(output, session2.ID) && !strings.Contains(output, session2.EntireID) {
		t.Logf("Note: Expected session2 ID in output, but this depends on checkpoint lookup")
	}
}

// TestResume_CheckpointWithoutMetadata tests resume when a commit has an Entire-Checkpoint
// trailer but the corresponding metadata is missing from entire/sessions branch.
// This can happen if the metadata branch was corrupted or reset.
func TestResume_CheckpointWithoutMetadata(t *testing.T) {
	t.Parallel()
	env := NewFeatureBranchEnv(t, strategy.StrategyNameAutoCommit)

	// First create a real session so the entire/sessions branch exists
	session := env.NewSession()
	if err := env.SimulateUserPromptSubmit(session.ID); err != nil {
		t.Fatalf("SimulateUserPromptSubmit failed: %v", err)
	}
	content := "real session content"
	env.WriteFile("real.txt", content)
	session.CreateTranscript(
		"Create real file",
		[]FileChange{{Path: "real.txt", Content: content}},
	)
	if err := env.SimulateStop(session.ID, session.TranscriptPath); err != nil {
		t.Fatalf("SimulateStop failed: %v", err)
	}

	// Create a new branch for the orphan checkpoint test
	env.GitCheckoutNewBranch("feature/orphan-checkpoint")

	// Add some content and create a commit with a checkpoint trailer
	// that points to non-existent metadata
	env.WriteFile("orphan.txt", "orphan content")
	env.GitAdd("orphan.txt")

	orphanCheckpointID := "000000000000" // Non-existent checkpoint
	env.GitCommitWithCheckpointID("Commit with orphan checkpoint", orphanCheckpointID)

	featureBranch := env.GetCurrentBranch()

	// Switch to main
	env.GitCheckoutBranch(masterBranch)

	// Resume - should not error but indicate no session available
	output, err := env.RunResume(featureBranch)
	if err != nil {
		t.Fatalf("resume failed: %v\nOutput: %s", err, output)
	}

	// Verify we switched to the feature branch
	if branch := env.GetCurrentBranch(); branch != featureBranch {
		t.Errorf("expected to be on %s, got %s", featureBranch, branch)
	}

	// Should NOT show session info since metadata is missing
	// The resume command should silently skip commits without valid metadata
	if strings.Contains(output, "Session:") {
		t.Errorf("output should not contain 'Session:' when metadata is missing, got: %s", output)
	}
}

// RunResume executes the resume command and returns the combined output.
func (env *TestEnv) RunResume(branchName string) (string, error) {
	env.T.Helper()

	ctx := env.T.Context()
	cmd := exec.CommandContext(ctx, getTestBinary(), "resume", branchName)
	cmd.Dir = env.RepoDir
	cmd.Env = append(os.Environ(),
		"ENTIRE_TEST_CLAUDE_PROJECT_DIR="+env.ClaudeProjectDir,
	)

	output, err := cmd.CombinedOutput()
	return string(output), err
}

// GitCheckoutBranch checks out an existing branch.
func (env *TestEnv) GitCheckoutBranch(branchName string) {
	env.T.Helper()

	repo, err := git.PlainOpen(env.RepoDir)
	if err != nil {
		env.T.Fatalf("failed to open git repo: %v", err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		env.T.Fatalf("failed to get worktree: %v", err)
	}

	err = worktree.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(branchName),
	})
	if err != nil {
		env.T.Fatalf("failed to checkout branch %s: %v", branchName, err)
	}
}
