package strategy

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
)

// shadowBranchPattern matches shadow branch names: entire/<7+ hex chars>
// The pattern requires at least 7 hex characters after "entire/"
var shadowBranchPattern = regexp.MustCompile(`^entire/[0-9a-fA-F]{7,}$`)

// IsShadowBranch returns true if the branch name matches the shadow branch pattern.
// Shadow branches have the format "entire/<commit-hash>" where the commit hash
// is at least 7 hex characters. The "entire/sessions" branch is NOT a shadow branch.
func IsShadowBranch(branchName string) bool {
	// Explicitly exclude entire/sessions
	if branchName == "entire/sessions" {
		return false
	}
	return shadowBranchPattern.MatchString(branchName)
}

// ListShadowBranches returns all shadow branches in the repository.
// Shadow branches match the pattern "entire/<commit-hash>" (7+ hex chars).
// The "entire/sessions" branch is excluded as it stores permanent metadata.
// Returns an empty slice (not nil) if no shadow branches exist.
func ListShadowBranches() ([]string, error) {
	repo, err := OpenRepository()
	if err != nil {
		return nil, fmt.Errorf("failed to open git repository: %w", err)
	}

	refs, err := repo.References()
	if err != nil {
		return nil, fmt.Errorf("failed to get references: %w", err)
	}

	var shadowBranches []string

	err = refs.ForEach(func(ref *plumbing.Reference) error {
		// Only look at branch references
		if !ref.Name().IsBranch() {
			return nil
		}

		// Extract branch name without refs/heads/ prefix
		branchName := strings.TrimPrefix(ref.Name().String(), "refs/heads/")

		if IsShadowBranch(branchName) {
			shadowBranches = append(shadowBranches, branchName)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to iterate references: %w", err)
	}

	// Ensure we return empty slice, not nil
	if shadowBranches == nil {
		shadowBranches = []string{}
	}

	return shadowBranches, nil
}

// DeleteShadowBranches deletes the specified branches from the repository.
// Returns two slices: successfully deleted branches and branches that failed to delete.
// Individual branch deletion failures do not stop the operation - all branches are attempted.
// Returns an error only if the repository cannot be opened.
func DeleteShadowBranches(branches []string) (deleted []string, failed []string, err error) {
	if len(branches) == 0 {
		return []string{}, []string{}, nil
	}

	repo, err := OpenRepository()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open git repository: %w", err)
	}

	for _, branch := range branches {
		refName := plumbing.NewBranchReferenceName(branch)

		// Check if reference exists before trying to delete
		ref, err := repo.Reference(refName, true)
		if err != nil {
			failed = append(failed, branch)
			continue
		}

		// Delete the reference
		if err := repo.Storer.RemoveReference(ref.Name()); err != nil {
			failed = append(failed, branch)
			continue
		}

		deleted = append(deleted, branch)
	}

	return deleted, failed, nil
}
