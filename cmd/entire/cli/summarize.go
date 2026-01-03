package cli

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/spf13/cobra"
)

// ValidateSummarize validates that summarize can proceed safely.
// It checks:
// - Current branch is not main/master (unless skipDefaultBranchCheck is true)
// - Target branch exists
// - No uncommitted changes
// - Current branch has commits since target
func ValidateSummarize(targetBranch string, skipDefaultBranchCheck bool) error {
	// Check if on default branch (unless skip flag is set)
	if !skipDefaultBranchCheck {
		isDefault, branchName, err := IsOnDefaultBranch()
		if err != nil {
			return fmt.Errorf("failed to check current branch: %w", err)
		}
		if isDefault {
			return fmt.Errorf("cannot summarize '%s' branch - create a feature branch first", branchName)
		}
	}

	// Check target branch exists
	exists, err := BranchExists(targetBranch)
	if err != nil {
		return fmt.Errorf("failed to check target branch: %w", err)
	}
	if !exists {
		return fmt.Errorf("target branch '%s' not found", targetBranch)
	}

	// Check for uncommitted changes
	hasChanges, err := HasUncommittedChanges()
	if err != nil {
		return fmt.Errorf("failed to check for uncommitted changes: %w", err)
	}
	if hasChanges {
		return errors.New("uncommitted changes detected - commit or stash them before summarizing")
	}

	// Get current branch
	currentBranch, err := GetCurrentBranch()
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}

	// Find merge base
	mergeBase, err := GetMergeBase(currentBranch, targetBranch)
	if err != nil {
		return fmt.Errorf("failed to find common ancestor with %s: %w", targetBranch, err)
	}

	// Count commits since merge base
	repo, err := openRepository()
	if err != nil {
		return fmt.Errorf("failed to open git repository: %w", err)
	}

	head, err := repo.Head()
	if err != nil {
		return fmt.Errorf("failed to get HEAD: %w", err)
	}

	headHash := head.Hash()
	count, err := CountCommitsBetween(mergeBase, &headHash)
	if err != nil {
		return fmt.Errorf("failed to count commits: %w", err)
	}

	if count == 0 {
		return fmt.Errorf("no commits to summarize - branch is at same commit as %s", targetBranch)
	}

	return nil
}

// BranchExists checks if a branch exists in the repository.
func BranchExists(branchName string) (bool, error) {
	repo, err := openRepository()
	if err != nil {
		return false, fmt.Errorf("failed to open git repository: %w", err)
	}

	_, err = repo.Reference(plumbing.NewBranchReferenceName(branchName), true)
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check branch: %w", err)
	}

	return true, nil
}

// CountCommitsBetween counts the number of commits between two commits.
// Returns the count of commits from (exclusive) to (inclusive).
func CountCommitsBetween(from, to *plumbing.Hash) (int, error) {
	repo, err := openRepository()
	if err != nil {
		return 0, fmt.Errorf("failed to open git repository: %w", err)
	}

	// Get the commit object for 'to'
	toCommit, err := repo.CommitObject(*to)
	if err != nil {
		return 0, fmt.Errorf("failed to get 'to' commit: %w", err)
	}

	// Iterate through commit history from 'to' backwards
	iter := object.NewCommitPreorderIter(toCommit, nil, nil)
	count := 0
	err = iter.ForEach(func(c *object.Commit) error {
		// Stop when we reach 'from'
		if c.Hash == *from {
			return errors.New("stop")
		}
		count++
		return nil
	})

	// ForEach returns the error that stopped iteration
	// Our "stop" error is expected, so ignore it
	if err != nil && err.Error() != "stop" {
		return 0, fmt.Errorf("failed to iterate commits: %w", err)
	}

	return count, nil
}

// CreateSquashedBranch creates a new branch with a single squashed commit.
// The commit contains all changes from sourceBranch since it diverged from targetBranch.
// Trailers can optionally be added to the commit message.
func CreateSquashedBranch(sourceBranch, targetBranch, newBranchName, message string, trailers map[string]string, author *GitAuthor) error {
	repo, err := openRepository()
	if err != nil {
		return fmt.Errorf("failed to open git repository: %w", err)
	}

	// Find merge base
	mergeBase, err := GetMergeBase(sourceBranch, targetBranch)
	if err != nil {
		return fmt.Errorf("failed to find merge base: %w", err)
	}

	// Get the tree from source branch HEAD
	sourceRef, err := repo.Reference(plumbing.NewBranchReferenceName(sourceBranch), true)
	if err != nil {
		return fmt.Errorf("failed to resolve source branch: %w", err)
	}

	sourceCommit, err := repo.CommitObject(sourceRef.Hash())
	if err != nil {
		return fmt.Errorf("failed to get source commit: %w", err)
	}

	// Use the tree from source HEAD (contains all the accumulated changes)
	tree := sourceCommit.TreeHash

	// Build commit message with trailers (sorted for deterministic order)
	fullMessage := message
	if len(trailers) > 0 {
		fullMessage += "\n"

		// Sort trailer keys for deterministic ordering
		keys := make([]string, 0, len(trailers))
		for k := range trailers {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		var sb strings.Builder
		for _, key := range keys {
			sb.WriteString(fmt.Sprintf("\n%s: %s", key, trailers[key]))
		}
		fullMessage += sb.String()
	}

	// Create the squashed commit with merge-base as parent
	commit := &object.Commit{
		Author: object.Signature{
			Name:  author.Name,
			Email: author.Email,
			When:  time.Now(),
		},
		Committer: object.Signature{
			Name:  author.Name,
			Email: author.Email,
			When:  time.Now(),
		},
		Message:      fullMessage,
		TreeHash:     tree,
		ParentHashes: []plumbing.Hash{*mergeBase},
	}

	// Store the commit object
	obj := repo.Storer.NewEncodedObject()
	if err := commit.Encode(obj); err != nil {
		return fmt.Errorf("failed to encode commit: %w", err)
	}
	commitHash, err := repo.Storer.SetEncodedObject(obj)
	if err != nil {
		return fmt.Errorf("failed to store commit: %w", err)
	}

	// Create/update the branch reference
	ref := plumbing.NewHashReference(plumbing.NewBranchReferenceName(newBranchName), commitHash)
	if err := repo.Storer.SetReference(ref); err != nil {
		return fmt.Errorf("failed to create branch reference: %w", err)
	}

	return nil
}

func newSummarizeCmd() *cobra.Command {
	var messageFlag string
	var targetFlag string
	var branchFlag string
	var forceFlag bool
	cmd := &cobra.Command{
		Use:   "summarize",
		Short: "Create a PR-ready branch with squashed commits",
		Long: `Squashes all commits from your feature branch into a single commit on a
summary branch (entire/pr/<branch>). The original branch remains unchanged.

This is useful for creating clean pull requests while preserving your detailed
commit history for debugging and attribution.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runSummarize(messageFlag, targetFlag, branchFlag, forceFlag)
		},
	}

	cmd.Flags().StringVarP(&messageFlag, "message", "m", "", "Commit message for squashed commit (required)")
	cmd.Flags().StringVarP(&targetFlag, "target", "t", "main", "PR target branch")
	cmd.Flags().StringVarP(&branchFlag, "branch", "b", "", "Custom summary branch name (default: entire/pr/<current-branch>)")
	cmd.Flags().BoolVarP(&forceFlag, "force", "f", false, "Overwrite summary branch even if it exists on remote")

	// Mark message flag as required. This should never fail since the flag exists.
	//nolint:errcheck,gosec // Cannot return error from command builder; failure means programming error
	cmd.MarkFlagRequired("message")

	return cmd
}

func runSummarize(message, target, summaryBranchName string, force bool) error {
	// Validation
	if err := ValidateSummarize(target, false); err != nil {
		return err
	}

	// Get current branch
	currentBranch, err := GetCurrentBranch()
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}

	// Determine summary branch name
	if summaryBranchName == "" {
		summaryBranchName = "entire/pr/" + currentBranch
	}

	// Check if summary branch exists on remote (unless --force is used)
	if !force {
		existsOnRemote, err := BranchExistsOnRemote(summaryBranchName)
		if err != nil {
			return fmt.Errorf("failed to check remote branch: %w", err)
		}
		if existsOnRemote {
			return fmt.Errorf("summary branch '%s' exists on remote. Use --force to overwrite", summaryBranchName)
		}
	}

	// Get merge base for commit range trailer
	mergeBase, err := GetMergeBase(currentBranch, target)
	if err != nil {
		return fmt.Errorf("failed to find merge base: %w", err)
	}

	// Get HEAD commit for trailer and commit counting
	repo, err := openRepository()
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}
	head, err := repo.Head()
	if err != nil {
		return fmt.Errorf("failed to get HEAD: %w", err)
	}
	headHash := head.Hash()

	// Build trailers
	trailers := make(map[string]string)

	// Try to get session info (may not exist, that's ok)
	start := GetStrategy()
	info, infoErr := start.GetSessionInfo()
	if infoErr == nil {
		trailers["Entire-Session"] = info.SessionID
		trailers["Entire-Strategy"] = start.Name()
	}

	// Always include these trailers
	trailers["Entire-Source-Branch"] = currentBranch
	trailers["Entire-Source-Range"] = mergeBase.String()[:7] + ".." + headHash.String()[:7]
	trailers["Generated-By"] = "entire-cli"

	// Get git author
	author, err := GetGitAuthor()
	if err != nil {
		return fmt.Errorf("failed to get git author: %w", err)
	}

	// Create squashed branch
	if err := CreateSquashedBranch(currentBranch, target, summaryBranchName, message, trailers, author); err != nil {
		return fmt.Errorf("failed to create squashed branch: %w", err)
	}

	// Count commits that were squashed
	commitCount, err := CountCommitsBetween(mergeBase, &headHash)
	if err != nil {
		// Log warning but don't fail - commit count is informational
		commitCount = 0
	}

	// Success message
	fmt.Printf("\nCreated summary branch: %s\n", summaryBranchName)
	fmt.Printf("Source branch: %s (%d commits squashed)\n", currentBranch, commitCount)
	fmt.Printf("\nNext steps:\n")
	fmt.Printf("  1. Push to remote:    git push origin %s\n", summaryBranchName)
	fmt.Printf("  2. Create PR:         gh pr create --base %s --head %s\n", target, summaryBranchName)
	fmt.Printf("\nYour original branch '%s' remains unchanged for reference.\n", currentBranch)

	return nil
}
