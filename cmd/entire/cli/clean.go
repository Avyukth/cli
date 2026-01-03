package cli

import (
	"fmt"
	"io"

	"entire.io/cli/cmd/entire/cli/strategy"
	"github.com/spf13/cobra"
)

func newCleanCmd() *cobra.Command {
	var forceFlag bool

	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Clean up shadow branches",
		Long: `Remove ephemeral shadow branches created by the manual-commit strategy.

Shadow branches (entire/<commit-hash>) store checkpoint metadata and can
accumulate over time. This command helps clean them up.

Without --force, shows a preview of branches that would be deleted.
With --force, actually deletes the branches.

The entire/sessions branch is never deleted as it contains permanent
session metadata.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runClean(cmd.OutOrStdout(), forceFlag)
		},
	}

	cmd.Flags().BoolVarP(&forceFlag, "force", "f", false, "Actually delete branches (otherwise just preview)")

	return cmd
}

func runClean(w io.Writer, force bool) error {
	// List all shadow branches
	branches, err := strategy.ListShadowBranches()
	if err != nil {
		return fmt.Errorf("failed to list shadow branches: %w", err)
	}

	return runCleanWithBranches(w, force, branches)
}

// runCleanWithBranches is the core logic for cleaning branches.
// Separated for testability.
func runCleanWithBranches(w io.Writer, force bool, branches []string) error {
	// Handle no branches case
	if len(branches) == 0 {
		fmt.Fprintln(w, "No shadow branches to clean up.")
		return nil
	}

	// Preview mode (default)
	if !force {
		fmt.Fprintf(w, "%d shadow branches found:\n", len(branches))
		for _, branch := range branches {
			fmt.Fprintf(w, "  %s\n", branch)
		}
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "Run with --force to delete these branches.")
		return nil
	}

	// Force mode - delete branches
	deleted, failed, err := strategy.DeleteShadowBranches(branches)
	if err != nil {
		return fmt.Errorf("failed to delete shadow branches: %w", err)
	}

	// Report results
	if len(deleted) > 0 {
		fmt.Fprintf(w, "Deleted %d shadow branches:\n", len(deleted))
		for _, branch := range deleted {
			fmt.Fprintf(w, "  %s\n", branch)
		}
	}

	if len(failed) > 0 {
		fmt.Fprintf(w, "\nFailed to delete %d branches:\n", len(failed))
		for _, branch := range failed {
			fmt.Fprintf(w, "  %s\n", branch)
		}
		return fmt.Errorf("failed to delete %d branches", len(failed))
	}

	return nil
}
