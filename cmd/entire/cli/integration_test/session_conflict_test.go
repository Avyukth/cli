//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"entire.io/cli/cmd/entire/cli/paths"
	"entire.io/cli/cmd/entire/cli/strategy"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// TestSessionIDConflict_DifferentSessionOnShadowBranch tests that starting a new session
// fails when the shadow branch has commits from a different session.
func TestSessionIDConflict_DifferentSessionOnShadowBranch(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Cleanup()

	// Setup
	env.InitRepo()
	env.WriteFile("README.md", "# Test")
	env.GitAdd("README.md")
	env.GitCommit("Initial commit")

	env.GitCheckoutNewBranch("feature/test")
	env.InitEntire(strategy.StrategyNameManualCommit)

	baseHead := env.GetHeadHash()
	shadowBranch := "entire/" + baseHead[:7]

	// Create a session and checkpoint (this creates the shadow branch)
	session1 := env.NewSession()
	if err := env.SimulateUserPromptSubmit(session1.ID); err != nil {
		t.Fatalf("SimulateUserPromptSubmit (session1) failed: %v", err)
	}

	env.WriteFile("test.txt", "content")
	session1.CreateTranscript("Add test file", []FileChange{{Path: "test.txt", Content: "content"}})
	if err := env.SimulateStop(session1.ID, session1.TranscriptPath); err != nil {
		t.Fatalf("SimulateStop (session1) failed: %v", err)
	}

	// Verify shadow branch exists
	if !env.BranchExists(shadowBranch) {
		t.Fatalf("Shadow branch %s should exist after first session", shadowBranch)
	}
	t.Logf("Created shadow branch: %s", shadowBranch)

	// Clear the session state file but keep the shadow branch
	// This simulates an orphaned shadow branch scenario
	sessionStateDir := filepath.Join(env.RepoDir, ".git", "entire-sessions")
	entries, err := os.ReadDir(sessionStateDir)
	if err != nil {
		t.Fatalf("Failed to read session state dir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".json") {
			if err := os.Remove(filepath.Join(sessionStateDir, entry.Name())); err != nil {
				t.Fatalf("Failed to remove session state file: %v", err)
			}
		}
	}

	// Try to start a new session (should fail due to session ID conflict)
	session2 := env.NewSession()
	err = env.SimulateUserPromptSubmit(session2.ID)

	// Expect an error about session ID conflict
	if err == nil {
		t.Error("Expected error when starting new session with existing shadow branch from different session")
	} else {
		t.Logf("Got expected error: %v", err)
		if !strings.Contains(err.Error(), "session ID conflict") {
			t.Errorf("Expected 'session ID conflict' in error message, got: %v", err)
		}
	}
}

// TestSessionIDConflict_NoConflictWithSameSession tests that resuming the same session
// (same session ID) does not trigger a conflict error.
func TestSessionIDConflict_NoConflictWithSameSession(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Cleanup()

	// Setup
	env.InitRepo()
	env.WriteFile("README.md", "# Test")
	env.GitAdd("README.md")
	env.GitCommit("Initial commit")

	env.GitCheckoutNewBranch("feature/test")
	env.InitEntire(strategy.StrategyNameManualCommit)

	// Create a session and checkpoint
	session := env.NewSession()
	if err := env.SimulateUserPromptSubmit(session.ID); err != nil {
		t.Fatalf("SimulateUserPromptSubmit failed: %v", err)
	}

	env.WriteFile("test.txt", "content")
	session.CreateTranscript("Add test file", []FileChange{{Path: "test.txt", Content: "content"}})
	if err := env.SimulateStop(session.ID, session.TranscriptPath); err != nil {
		t.Fatalf("SimulateStop failed: %v", err)
	}

	// Try to "resume" the same session (same ID) - should not error
	// This simulates Claude resuming with the same session ID
	err := env.SimulateUserPromptSubmit(session.ID)
	if err != nil {
		t.Errorf("Resuming same session should not error, got: %v", err)
	}
}

// TestSessionIDConflict_NoShadowBranch tests that starting a new session succeeds
// when no shadow branch exists (fresh start).
func TestSessionIDConflict_NoShadowBranch(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Cleanup()

	// Setup
	env.InitRepo()
	env.WriteFile("README.md", "# Test")
	env.GitAdd("README.md")
	env.GitCommit("Initial commit")

	env.GitCheckoutNewBranch("feature/test")
	env.InitEntire(strategy.StrategyNameManualCommit)

	baseHead := env.GetHeadHash()
	shadowBranch := "entire/" + baseHead[:7]

	// Verify no shadow branch exists
	if env.BranchExists(shadowBranch) {
		t.Fatalf("Shadow branch %s should not exist before first session", shadowBranch)
	}

	// Create a new session - should succeed without conflict
	session := env.NewSession()
	err := env.SimulateUserPromptSubmit(session.ID)
	if err != nil {
		t.Errorf("Starting new session with no shadow branch should succeed, got: %v", err)
	}
}

// TestSessionIDConflict_WithMultipleCheckpoints tests that session ID conflict is detected
// even when the shadow branch has multiple checkpoints.
func TestSessionIDConflict_WithMultipleCheckpoints(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Cleanup()

	// Setup
	env.InitRepo()
	env.WriteFile("README.md", "# Test")
	env.GitAdd("README.md")
	env.GitCommit("Initial commit")

	env.GitCheckoutNewBranch("feature/test")
	env.InitEntire(strategy.StrategyNameManualCommit)

	baseHead := env.GetHeadHash()
	shadowBranch := "entire/" + baseHead[:7]

	// Create a session with multiple checkpoints
	session1 := env.NewSession()
	if err := env.SimulateUserPromptSubmit(session1.ID); err != nil {
		t.Fatalf("SimulateUserPromptSubmit failed: %v", err)
	}

	// First checkpoint
	env.WriteFile("test1.txt", "content1")
	session1.CreateTranscript("Add test1", []FileChange{{Path: "test1.txt", Content: "content1"}})
	if err := env.SimulateStop(session1.ID, session1.TranscriptPath); err != nil {
		t.Fatalf("SimulateStop (checkpoint 1) failed: %v", err)
	}

	// Continue session with second checkpoint
	if err := env.SimulateUserPromptSubmit(session1.ID); err != nil {
		t.Fatalf("SimulateUserPromptSubmit (checkpoint 2) failed: %v", err)
	}

	env.WriteFile("test2.txt", "content2")
	session1.TranscriptBuilder = NewTranscriptBuilder() // Reset transcript builder
	session1.CreateTranscript("Add test2", []FileChange{{Path: "test2.txt", Content: "content2"}})
	if err := env.SimulateStop(session1.ID, session1.TranscriptPath); err != nil {
		t.Fatalf("SimulateStop (checkpoint 2) failed: %v", err)
	}

	// Verify shadow branch exists
	if !env.BranchExists(shadowBranch) {
		t.Fatalf("Shadow branch %s should exist", shadowBranch)
	}

	// Clear session state files
	sessionStateDir := filepath.Join(env.RepoDir, ".git", "entire-sessions")
	entries, err := os.ReadDir(sessionStateDir)
	if err == nil {
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".json") {
				_ = os.Remove(filepath.Join(sessionStateDir, entry.Name()))
			}
		}
	}

	// Try to start a new session - should fail
	session2 := env.NewSession()
	err = env.SimulateUserPromptSubmit(session2.ID)

	if err == nil {
		t.Error("Expected session ID conflict error when shadow branch has multiple checkpoints from different session")
	} else if !strings.Contains(err.Error(), "session ID conflict") {
		t.Errorf("Expected 'session ID conflict' in error, got: %v", err)
	}
}

// TestSessionIDConflict_OrphanedShadowBranch tests detection of orphaned shadow branches
// (shadow branch exists but has no corresponding session state file).
func TestSessionIDConflict_OrphanedShadowBranch(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Cleanup()

	// Setup
	env.InitRepo()
	env.WriteFile("README.md", "# Test")
	env.GitAdd("README.md")
	env.GitCommit("Initial commit")

	env.GitCheckoutNewBranch("feature/test")
	env.InitEntire(strategy.StrategyNameManualCommit)

	baseHead := env.GetHeadHash()
	shadowBranch := "entire/" + baseHead[:7]

	// Manually create a shadow branch with a different session ID
	// This simulates a shadow branch that was left behind
	createOrphanedShadowBranch(t, env.RepoDir, shadowBranch, "orphaned-session-id")

	// Verify shadow branch exists
	if !env.BranchExists(shadowBranch) {
		t.Fatalf("Shadow branch %s should exist after manual creation", shadowBranch)
	}

	// Try to start a new session - should detect conflict from shadow branch trailer
	session := env.NewSession()
	err := env.SimulateUserPromptSubmit(session.ID)

	if err == nil {
		t.Error("Expected session ID conflict error when orphaned shadow branch exists")
	} else {
		t.Logf("Got expected error: %v", err)
		if !strings.Contains(err.Error(), "session ID conflict") {
			t.Errorf("Expected 'session ID conflict' in error, got: %v", err)
		}
	}
}

// createOrphanedShadowBranch creates a shadow branch with a specific session ID
// without creating a corresponding session state file.
func createOrphanedShadowBranch(t *testing.T, repoDir, branchName, sessionID string) {
	t.Helper()

	repo, err := git.PlainOpen(repoDir)
	if err != nil {
		t.Fatalf("Failed to open repo: %v", err)
	}

	// Get HEAD commit to use as parent/tree
	head, err := repo.Head()
	if err != nil {
		t.Fatalf("Failed to get HEAD: %v", err)
	}

	headCommit, err := repo.CommitObject(head.Hash())
	if err != nil {
		t.Fatalf("Failed to get HEAD commit: %v", err)
	}

	// Create an Entire session ID with date prefix
	entireSessionID := paths.EntireSessionID(sessionID)

	// Create commit message with Entire-Session trailer
	commitMsg := "Orphaned checkpoint\n\n" +
		"Entire-Session: " + entireSessionID + "\n" +
		"Entire-Strategy: manual-commit\n"

	// Create the commit
	commit := &object.Commit{
		Author: object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
		Committer: object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
		Message:  commitMsg,
		TreeHash: headCommit.TreeHash,
	}

	obj := repo.Storer.NewEncodedObject()
	if err := commit.Encode(obj); err != nil {
		t.Fatalf("Failed to encode commit: %v", err)
	}

	commitHash, err := repo.Storer.SetEncodedObject(obj)
	if err != nil {
		t.Fatalf("Failed to store commit: %v", err)
	}

	// Create the branch reference
	refName := plumbing.NewBranchReferenceName(branchName)
	ref := plumbing.NewHashReference(refName, commitHash)
	if err := repo.Storer.SetReference(ref); err != nil {
		t.Fatalf("Failed to create branch reference: %v", err)
	}
}

// TestSessionIDConflict_ShadowBranchWithoutTrailer tests that a shadow branch without
// an Entire-Session trailer does not cause a conflict (backwards compatibility).
func TestSessionIDConflict_ShadowBranchWithoutTrailer(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Cleanup()

	// Setup
	env.InitRepo()
	env.WriteFile("README.md", "# Test")
	env.GitAdd("README.md")
	env.GitCommit("Initial commit")

	env.GitCheckoutNewBranch("feature/test")
	env.InitEntire(strategy.StrategyNameManualCommit)

	baseHead := env.GetHeadHash()
	shadowBranch := "entire/" + baseHead[:7]

	// Create a shadow branch without Entire-Session trailer (simulating old format)
	createShadowBranchWithoutTrailer(t, env.RepoDir, shadowBranch)

	// Verify shadow branch exists
	if !env.BranchExists(shadowBranch) {
		t.Fatalf("Shadow branch %s should exist", shadowBranch)
	}

	// Starting a new session should succeed (no trailer = no conflict)
	session := env.NewSession()
	err := env.SimulateUserPromptSubmit(session.ID)

	if err != nil {
		t.Errorf("Starting session with shadow branch without trailer should succeed, got: %v", err)
	}
}

// createShadowBranchWithoutTrailer creates a shadow branch without an Entire-Session trailer.
func createShadowBranchWithoutTrailer(t *testing.T, repoDir, branchName string) {
	t.Helper()

	repo, err := git.PlainOpen(repoDir)
	if err != nil {
		t.Fatalf("Failed to open repo: %v", err)
	}

	head, err := repo.Head()
	if err != nil {
		t.Fatalf("Failed to get HEAD: %v", err)
	}

	headCommit, err := repo.CommitObject(head.Hash())
	if err != nil {
		t.Fatalf("Failed to get HEAD commit: %v", err)
	}

	// Create commit without Entire-Session trailer
	commit := &object.Commit{
		Author: object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
		Committer: object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
		Message:  "Legacy checkpoint without session trailer",
		TreeHash: headCommit.TreeHash,
	}

	obj := repo.Storer.NewEncodedObject()
	if err := commit.Encode(obj); err != nil {
		t.Fatalf("Failed to encode commit: %v", err)
	}

	commitHash, err := repo.Storer.SetEncodedObject(obj)
	if err != nil {
		t.Fatalf("Failed to store commit: %v", err)
	}

	refName := plumbing.NewBranchReferenceName(branchName)
	ref := plumbing.NewHashReference(refName, commitHash)
	if err := repo.Storer.SetReference(ref); err != nil {
		t.Fatalf("Failed to create branch reference: %v", err)
	}
}
