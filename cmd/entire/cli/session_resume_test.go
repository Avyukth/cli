package cli

import (
	"os"
	"path/filepath"
	"testing"

	"entire.io/cli/cmd/entire/cli/paths"
	"entire.io/cli/cmd/entire/cli/strategy"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestRunSessionResume_WithValidSession(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	// Set up a fake Claude project directory for testing
	claudeDir := filepath.Join(tmpDir, "claude-projects")
	t.Setenv("ENTIRE_TEST_CLAUDE_PROJECT_DIR", claudeDir)

	// Initialize a git repository
	repo, err := git.PlainInit(tmpDir, false)
	if err != nil {
		t.Fatalf("Failed to init repo: %v", err)
	}

	// Create initial commit
	w, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Failed to get worktree: %v", err)
	}

	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0o644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}
	if _, err := w.Add("test.txt"); err != nil {
		t.Fatalf("Failed to add test file: %v", err)
	}
	if _, err := w.Commit("initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
		},
	}); err != nil {
		t.Fatalf("Failed to create initial commit: %v", err)
	}

	// Ensure entire/sessions branch exists
	if err := strategy.EnsureMetadataBranch(repo); err != nil {
		t.Fatalf("Failed to create metadata branch: %v", err)
	}

	// Set up the auto-commit strategy and create checkpoint metadata
	strat := strategy.NewDualStrategy()
	if err := strat.EnsureSetup(); err != nil {
		t.Fatalf("Failed to ensure setup: %v", err)
	}

	// Create a session with metadata
	sessionID := "2025-12-19-abc123"
	sessionLogContent := `{"type":"test"}`
	metadataDir := filepath.Join(tmpDir, paths.EntireMetadataDir, sessionID)
	if err := os.MkdirAll(metadataDir, 0o755); err != nil {
		t.Fatalf("Failed to create metadata dir: %v", err)
	}
	logFile := filepath.Join(metadataDir, paths.TranscriptFileName)
	if err := os.WriteFile(logFile, []byte(sessionLogContent), 0o644); err != nil {
		t.Fatalf("Failed to write log file: %v", err)
	}

	// Create a file change to commit
	if err := os.WriteFile(testFile, []byte("modified content"), 0o644); err != nil {
		t.Fatalf("Failed to modify test file: %v", err)
	}

	// Use SaveChanges to create a commit with checkpoint metadata
	ctx := strategy.SaveContext{
		CommitMessage:  "test commit with checkpoint",
		MetadataDir:    filepath.Join(paths.EntireMetadataDir, sessionID),
		MetadataDirAbs: metadataDir,
		NewFiles:       []string{},
		ModifiedFiles:  []string{"test.txt"},
		DeletedFiles:   []string{},
		AuthorName:     "Test User",
		AuthorEmail:    "test@example.com",
	}
	if err := strat.SaveChanges(ctx); err != nil {
		t.Fatalf("Failed to save changes: %v", err)
	}

	// Now test the session resume functionality
	err = runSessionResume(sessionID)
	if err != nil {
		t.Fatalf("runSessionResume() returned error: %v", err)
	}

	// Verify current session was set
	currentSession, err := paths.ReadCurrentSession()
	if err != nil {
		t.Fatalf("Failed to read current session: %v", err)
	}
	if currentSession != sessionID {
		t.Errorf("Current session = %q, want %q", currentSession, sessionID)
	}

	// Verify Claude memory was restored
	claudeSessionID := paths.ModelSessionID(sessionID)
	expectedLogPath := filepath.Join(claudeDir, claudeSessionID+".jsonl")

	content, err := os.ReadFile(expectedLogPath)
	if err != nil {
		t.Fatalf("Failed to read session log from Claude project dir: %v", err)
	}

	if string(content) != sessionLogContent {
		t.Errorf("Session log content = %q, want %q", string(content), sessionLogContent)
	}
}

func TestRunSessionResume_SessionNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	// Initialize a git repository
	repo, err := git.PlainInit(tmpDir, false)
	if err != nil {
		t.Fatalf("Failed to init repo: %v", err)
	}

	// Create initial commit
	w, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Failed to get worktree: %v", err)
	}

	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0o644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}
	if _, err := w.Add("test.txt"); err != nil {
		t.Fatalf("Failed to add test file: %v", err)
	}
	if _, err := w.Commit("initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
		},
	}); err != nil {
		t.Fatalf("Failed to create initial commit: %v", err)
	}

	// Ensure entire/sessions branch exists
	if err := strategy.EnsureMetadataBranch(repo); err != nil {
		t.Fatalf("Failed to create metadata branch: %v", err)
	}

	// Try to resume a non-existent session
	err = runSessionResume("nonexistent-session")
	if err == nil {
		t.Error("runSessionResume() expected error for non-existent session, got nil")
	}
}

func TestBuildSessionOptions(t *testing.T) {
	sessions := []strategy.Session{
		{
			ID:          "2025-12-19-session1",
			Description: "First session description",
			Checkpoints: []strategy.Checkpoint{
				{CheckpointID: "abc123"},
			},
		},
		{
			ID:          "2025-12-18-session2",
			Description: "Second session",
			Checkpoints: []strategy.Checkpoint{},
		},
	}

	options := buildSessionOptions(sessions)

	// Should have sessions + cancel option
	if len(options) != 3 {
		t.Errorf("Expected 3 options, got %d", len(options))
	}

	// Verify first option is the first session
	if options[0].Value != "2025-12-19-session1" {
		t.Errorf("First option value = %q, want %q", options[0].Value, "2025-12-19-session1")
	}

	// Verify last option is cancel
	if options[2].Value != sessionPickerCancelValue {
		t.Errorf("Last option should be cancel, got %q", options[2].Value)
	}
}
