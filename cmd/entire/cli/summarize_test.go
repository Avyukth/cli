package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// gitCheckout is defined in git_operations_test.go

func TestValidateSummarize(t *testing.T) {
	// Create temp directory for test repo
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	// Initialize repo
	repo, err := git.PlainInit(tmpDir, false)
	if err != nil {
		t.Fatalf("Failed to init repo: %v", err)
	}

	w, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Failed to get worktree: %v", err)
	}

	// Create initial commit on main
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("initial"), 0o644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}
	if _, err := w.Add("test.txt"); err != nil {
		t.Fatalf("Failed to add test file: %v", err)
	}
	baseCommit, err := w.Commit("initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
		},
	})
	if err != nil {
		t.Fatalf("Failed to create initial commit: %v", err)
	}

	// Create main branch
	mainRef := plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), baseCommit)
	if err := repo.Storer.SetReference(mainRef); err != nil {
		t.Fatalf("Failed to create main branch: %v", err)
	}

	t.Run("error when on main branch", func(t *testing.T) {
		gitCheckout(t, tmpDir, "main")

		err := ValidateSummarize("main", false)
		if err == nil {
			t.Error("ValidateSummarize() expected error for main branch, got nil")
		}
	})

	t.Run("error when target branch doesn't exist", func(t *testing.T) {
		// Create and checkout feature branch
		featureRef := plumbing.NewHashReference(plumbing.NewBranchReferenceName("feature"), baseCommit)
		if err := repo.Storer.SetReference(featureRef); err != nil {
			t.Fatalf("Failed to create feature branch: %v", err)
		}
		gitCheckout(t, tmpDir, "feature")

		err := ValidateSummarize("nonexistent", false)
		if err == nil {
			t.Error("ValidateSummarize() expected error for nonexistent target, got nil")
		}
	})

	t.Run("error when uncommitted changes exist", func(t *testing.T) {
		// Make sure we're on feature branch
		gitCheckout(t, tmpDir, "feature")

		// Create uncommitted change
		if err := os.WriteFile(testFile, []byte("modified"), 0o644); err != nil {
			t.Fatalf("Failed to write test file: %v", err)
		}

		err := ValidateSummarize("main", false)
		if err == nil {
			t.Error("ValidateSummarize() expected error for uncommitted changes, got nil")
		}

		// Clean up
		if _, err := w.Add("test.txt"); err != nil {
			t.Fatalf("Failed to add test file: %v", err)
		}
		if _, err := w.Commit("cleanup", &git.CommitOptions{
			Author: &object.Signature{
				Name:  "Test",
				Email: "test@example.com",
			},
		}); err != nil {
			t.Fatalf("Failed to commit cleanup: %v", err)
		}
	})

	t.Run("error when no commits since target", func(t *testing.T) {
		// Create branch with no new commits
		emptyFeature := plumbing.NewHashReference(plumbing.NewBranchReferenceName("empty-feature"), baseCommit)
		if err := repo.Storer.SetReference(emptyFeature); err != nil {
			t.Fatalf("Failed to create empty-feature branch: %v", err)
		}
		gitCheckout(t, tmpDir, "empty-feature")

		err := ValidateSummarize("main", false)
		if err == nil {
			t.Error("ValidateSummarize() expected error when no commits to summarize, got nil")
		}
	})

	t.Run("success when all validations pass", func(t *testing.T) {
		// Create feature branch with commits
		featureRef := plumbing.NewHashReference(plumbing.NewBranchReferenceName("feature2"), baseCommit)
		if err := repo.Storer.SetReference(featureRef); err != nil {
			t.Fatalf("Failed to create feature2 branch: %v", err)
		}
		gitCheckout(t, tmpDir, "feature2")

		// Make a commit
		if err := os.WriteFile(testFile, []byte("feature work"), 0o644); err != nil {
			t.Fatalf("Failed to write test file: %v", err)
		}
		if _, err := w.Add("test.txt"); err != nil {
			t.Fatalf("Failed to add test file: %v", err)
		}
		if _, err := w.Commit("feature commit", &git.CommitOptions{
			Author: &object.Signature{
				Name:  "Test",
				Email: "test@example.com",
			},
		}); err != nil {
			t.Fatalf("Failed to commit: %v", err)
		}

		err := ValidateSummarize("main", false)
		if err != nil {
			t.Errorf("ValidateSummarize() unexpected error: %v", err)
		}
	})
}

func TestBranchExists(t *testing.T) {
	// Create temp directory for test repo
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	// Initialize repo
	repo, err := git.PlainInit(tmpDir, false)
	if err != nil {
		t.Fatalf("Failed to init repo: %v", err)
	}

	w, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Failed to get worktree: %v", err)
	}

	// Create initial commit
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0o644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}
	if _, err := w.Add("test.txt"); err != nil {
		t.Fatalf("Failed to add test file: %v", err)
	}
	commit, err := w.Commit("initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
		},
	})
	if err != nil {
		t.Fatalf("Failed to create initial commit: %v", err)
	}

	// Create main branch
	mainRef := plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), commit)
	if err := repo.Storer.SetReference(mainRef); err != nil {
		t.Fatalf("Failed to create main branch: %v", err)
	}

	t.Run("returns true for existing branch", func(t *testing.T) {
		exists, err := BranchExists("main")
		if err != nil {
			t.Fatalf("BranchExists() error = %v", err)
		}
		if !exists {
			t.Error("BranchExists() = false, want true for existing branch")
		}
	})

	t.Run("returns false for non-existent branch", func(t *testing.T) {
		exists, err := BranchExists("nonexistent")
		if err != nil {
			t.Fatalf("BranchExists() error = %v", err)
		}
		if exists {
			t.Error("BranchExists() = true, want false for non-existent branch")
		}
	})
}

func TestCountCommitsBetween(t *testing.T) {
	// Create temp directory for test repo
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	// Initialize repo
	repo, err := git.PlainInit(tmpDir, false)
	if err != nil {
		t.Fatalf("Failed to init repo: %v", err)
	}

	w, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Failed to get worktree: %v", err)
	}

	// Create base commit
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("initial"), 0o644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}
	if _, err := w.Add("test.txt"); err != nil {
		t.Fatalf("Failed to add test file: %v", err)
	}
	baseCommit, err := w.Commit("initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
		},
	})
	if err != nil {
		t.Fatalf("Failed to create initial commit: %v", err)
	}

	// Make 3 more commits with different content
	for i := 1; i <= 3; i++ {
		content := fmt.Sprintf("content %d", i)
		if err := os.WriteFile(testFile, []byte(content), 0o644); err != nil {
			t.Fatalf("Failed to write test file: %v", err)
		}
		if _, err := w.Add("test.txt"); err != nil {
			t.Fatalf("Failed to add test file: %v", err)
		}
		if _, err := w.Commit(fmt.Sprintf("commit %d", i), &git.CommitOptions{
			Author: &object.Signature{
				Name:  "Test",
				Email: "test@example.com",
			},
		}); err != nil {
			t.Fatalf("Failed to commit: %v", err)
		}
	}

	head, err := repo.Head()
	if err != nil {
		t.Fatalf("Failed to get HEAD: %v", err)
	}
	headHash := head.Hash()

	// Count commits from base to head
	count, err := CountCommitsBetween(&baseCommit, &headHash)
	if err != nil {
		t.Fatalf("CountCommitsBetween() error = %v", err)
	}

	// Should be 3 commits (not counting base)
	if count != 3 {
		t.Errorf("CountCommitsBetween() = %d, want 3", count)
	}
}

func TestCreateSquashedBranch(t *testing.T) {
	// Create temp directory for test repo
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	// Initialize repo
	repo, err := git.PlainInit(tmpDir, false)
	if err != nil {
		t.Fatalf("Failed to init repo: %v", err)
	}

	w, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Failed to get worktree: %v", err)
	}

	// Create base commit on main
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("initial"), 0o644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}
	if _, err := w.Add("test.txt"); err != nil {
		t.Fatalf("Failed to add test file: %v", err)
	}
	baseCommit, err := w.Commit("initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
		},
	})
	if err != nil {
		t.Fatalf("Failed to create initial commit: %v", err)
	}

	// Create main branch
	mainRef := plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), baseCommit)
	if err := repo.Storer.SetReference(mainRef); err != nil {
		t.Fatalf("Failed to create main branch: %v", err)
	}

	// Create feature branch
	featureRef := plumbing.NewHashReference(plumbing.NewBranchReferenceName("feature"), baseCommit)
	if err := repo.Storer.SetReference(featureRef); err != nil {
		t.Fatalf("Failed to create feature branch: %v", err)
	}
	gitCheckout(t, tmpDir, "feature")

	// Make 3 commits on feature
	for i := 1; i <= 3; i++ {
		content := fmt.Sprintf("feature %d", i)
		if err := os.WriteFile(testFile, []byte(content), 0o644); err != nil {
			t.Fatalf("Failed to write test file: %v", err)
		}
		if _, err := w.Add("test.txt"); err != nil {
			t.Fatalf("Failed to add test file: %v", err)
		}
		if _, err := w.Commit(fmt.Sprintf("feature commit %d", i), &git.CommitOptions{
			Author: &object.Signature{
				Name:  "Test",
				Email: "test@example.com",
			},
		}); err != nil {
			t.Fatalf("Failed to commit: %v", err)
		}
	}

	// Create squashed branch
	author := &GitAuthor{Name: "Test", Email: "test@example.com"}
	message := "feat: squashed feature work"
	err = CreateSquashedBranch("feature", "main", "entire/pr/feature", message, nil, author)
	if err != nil {
		t.Fatalf("CreateSquashedBranch() error = %v", err)
	}

	// Verify squashed branch exists
	exists, err := BranchExists("entire/pr/feature")
	if err != nil {
		t.Fatalf("Failed to check branch existence: %v", err)
	}
	if !exists {
		t.Error("CreateSquashedBranch() should create branch entire/pr/feature")
	}

	// Verify squashed branch has only 1 commit from merge-base
	squashRef, err := repo.Reference(plumbing.NewBranchReferenceName("entire/pr/feature"), true)
	if err != nil {
		t.Fatalf("Failed to get squashed branch ref: %v", err)
	}
	squashHash := squashRef.Hash()
	count, err := CountCommitsBetween(&baseCommit, &squashHash)
	if err != nil {
		t.Fatalf("Failed to count commits: %v", err)
	}
	if count != 1 {
		t.Errorf("Squashed branch should have 1 commit from base, got %d", count)
	}

	// Verify commit message
	squashCommit, err := repo.CommitObject(squashHash)
	if err != nil {
		t.Fatalf("Failed to get squashed commit: %v", err)
	}
	if !strings.Contains(squashCommit.Message, message) {
		t.Errorf("Squashed commit should contain message %q, got %q", message, squashCommit.Message)
	}

	// Verify original branch unchanged (get the hash before we call CreateSquashedBranch)
	currentFeature, err := repo.Reference(plumbing.NewBranchReferenceName("feature"), true)
	if err != nil {
		t.Fatalf("Failed to get feature branch ref: %v", err)
	}
	// Just check the last commit is one of our feature commits
	if currentFeature.Hash() == baseCommit {
		t.Error("Original feature branch should have commits, not be at base")
	}

	// Verify tree contents match HEAD
	featureCommit, err := repo.CommitObject(currentFeature.Hash())
	if err != nil {
		t.Fatalf("Failed to get feature commit: %v", err)
	}
	featureTree, err := featureCommit.Tree()
	if err != nil {
		t.Fatalf("Failed to get feature tree: %v", err)
	}
	squashTree, err := squashCommit.Tree()
	if err != nil {
		t.Fatalf("Failed to get squash tree: %v", err)
	}

	featureFile, err := featureTree.File("test.txt")
	if err != nil {
		t.Fatalf("Failed to get feature file: %v", err)
	}
	squashFile, err := squashTree.File("test.txt")
	if err != nil {
		t.Fatalf("Failed to get squash file: %v", err)
	}

	featureContent, err := featureFile.Contents()
	if err != nil {
		t.Fatalf("Failed to get feature file contents: %v", err)
	}
	squashContent, err := squashFile.Contents()
	if err != nil {
		t.Fatalf("Failed to get squash file contents: %v", err)
	}

	if featureContent != squashContent {
		t.Errorf("Squashed tree should match HEAD tree, got %q vs %q", squashContent, featureContent)
	}
}

func TestCreateSquashedBranch_TrailersAlphabeticalOrder(t *testing.T) {
	// Create temp directory for test repo
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	// Initialize repo
	repo, err := git.PlainInit(tmpDir, false)
	if err != nil {
		t.Fatalf("Failed to init repo: %v", err)
	}

	w, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Failed to get worktree: %v", err)
	}

	// Create base commit on main
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("initial"), 0o644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}
	if _, err := w.Add("test.txt"); err != nil {
		t.Fatalf("Failed to add test file: %v", err)
	}
	baseCommit, err := w.Commit("initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
		},
	})
	if err != nil {
		t.Fatalf("Failed to create initial commit: %v", err)
	}

	// Create main branch
	mainRef := plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), baseCommit)
	if err := repo.Storer.SetReference(mainRef); err != nil {
		t.Fatalf("Failed to create main branch: %v", err)
	}

	// Create feature branch
	featureRef := plumbing.NewHashReference(plumbing.NewBranchReferenceName("feature"), baseCommit)
	if err := repo.Storer.SetReference(featureRef); err != nil {
		t.Fatalf("Failed to create feature branch: %v", err)
	}
	gitCheckout(t, tmpDir, "feature")

	// Make a commit on feature
	if err := os.WriteFile(testFile, []byte("feature work"), 0o644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}
	if _, err := w.Add("test.txt"); err != nil {
		t.Fatalf("Failed to add test file: %v", err)
	}
	if _, err := w.Commit("feature commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
		},
	}); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Create squashed branch with trailers in non-alphabetical order
	author := &GitAuthor{Name: "Test", Email: "test@example.com"}
	message := "feat: test trailer ordering"
	trailers := map[string]string{
		"Zebra-Trailer":  "z",
		"Alpha-Trailer":  "a",
		"Middle-Trailer": "m",
		"Beta-Trailer":   "b",
	}
	err = CreateSquashedBranch("feature", "main", "test-branch", message, trailers, author)
	if err != nil {
		t.Fatalf("CreateSquashedBranch() error = %v", err)
	}

	// Get the squashed commit
	branchRef, err := repo.Reference(plumbing.NewBranchReferenceName("test-branch"), true)
	if err != nil {
		t.Fatalf("Failed to get branch ref: %v", err)
	}
	commit, err := repo.CommitObject(branchRef.Hash())
	if err != nil {
		t.Fatalf("Failed to get commit: %v", err)
	}

	// Parse commit message to extract trailer lines
	lines := strings.Split(commit.Message, "\n")
	var trailerLines []string
	inTrailers := false
	for _, line := range lines {
		if line == "" {
			inTrailers = true
			continue
		}
		if inTrailers && strings.Contains(line, ":") {
			trailerLines = append(trailerLines, line)
		}
	}

	// Verify we found all trailers
	if len(trailerLines) != len(trailers) {
		t.Fatalf("Expected %d trailer lines, got %d", len(trailers), len(trailerLines))
	}

	// Verify trailers are in alphabetical order
	for i := 1; i < len(trailerLines); i++ {
		prevKey := strings.Split(trailerLines[i-1], ":")[0]
		currKey := strings.Split(trailerLines[i], ":")[0]
		if prevKey > currKey {
			t.Errorf("Trailers not in alphabetical order: %s comes before %s", prevKey, currKey)
		}
	}

	// Verify expected order (Alpha, Beta, Middle, Zebra)
	expectedOrder := []string{"Alpha-Trailer", "Beta-Trailer", "Middle-Trailer", "Zebra-Trailer"}
	for i, expected := range expectedOrder {
		actualKey := strings.Split(trailerLines[i], ":")[0]
		if actualKey != expected {
			t.Errorf("Trailer at position %d: expected %s, got %s", i, expected, actualKey)
		}
	}
}

func TestSourceRangeTrailerFormat(t *testing.T) {
	// Create temp directory for test repo
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	// Initialize repo
	repo, err := git.PlainInit(tmpDir, false)
	if err != nil {
		t.Fatalf("Failed to init repo: %v", err)
	}

	w, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Failed to get worktree: %v", err)
	}

	// Create base commit on main
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("initial"), 0o644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}
	if _, err := w.Add("test.txt"); err != nil {
		t.Fatalf("Failed to add test file: %v", err)
	}
	baseCommit, err := w.Commit("initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
		},
	})
	if err != nil {
		t.Fatalf("Failed to create initial commit: %v", err)
	}

	// Create main branch
	mainRef := plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), baseCommit)
	if err := repo.Storer.SetReference(mainRef); err != nil {
		t.Fatalf("Failed to create main branch: %v", err)
	}

	// Create feature branch
	featureRef := plumbing.NewHashReference(plumbing.NewBranchReferenceName("feature"), baseCommit)
	if err := repo.Storer.SetReference(featureRef); err != nil {
		t.Fatalf("Failed to create feature branch: %v", err)
	}
	gitCheckout(t, tmpDir, "feature")

	// Make a commit on feature
	if err := os.WriteFile(testFile, []byte("feature work"), 0o644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}
	if _, err := w.Add("test.txt"); err != nil {
		t.Fatalf("Failed to add test file: %v", err)
	}
	featureCommit, err := w.Commit("feature commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
		},
	})
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Get merge base
	mergeBase, err := GetMergeBase("feature", "main")
	if err != nil {
		t.Fatalf("Failed to get merge base: %v", err)
	}

	// Build the Entire-Source-Range value the same way runSummarize does
	headHash := featureCommit
	sourceRange := mergeBase.String()[:7] + ".." + headHash.String()[:7]

	// Verify it doesn't contain "HEAD"
	if strings.Contains(sourceRange, "HEAD") {
		t.Errorf("Entire-Source-Range should not contain 'HEAD', got: %s", sourceRange)
	}

	// Verify it contains two short SHAs separated by ..
	parts := strings.Split(sourceRange, "..")
	if len(parts) != 2 {
		t.Fatalf("Expected format 'sha..sha', got: %s", sourceRange)
	}

	// Verify each part is a 7-char hex string
	for i, part := range parts {
		if len(part) != 7 {
			t.Errorf("Part %d should be 7 chars, got %d: %s", i, len(part), part)
		}
		for _, c := range part {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Errorf("Part %d contains non-hex char: %s", i, part)
				break
			}
		}
	}
}
