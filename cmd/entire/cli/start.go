package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"entire.io/cli/cmd/entire/cli/strategy"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/spf13/cobra"
)

// featureNameRegex matches valid feature names: alphanumeric, hyphens, underscores only
var featureNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// ValidateFeatureName validates that the feature name contains only valid characters.
// Valid characters are: alphanumeric, hyphens, and underscores.
func ValidateFeatureName(name string) error {
	if name == "" {
		return errors.New("feature name is required")
	}

	if !featureNameRegex.MatchString(name) {
		return fmt.Errorf("invalid feature name '%s': use alphanumeric characters, hyphens, and underscores only", name)
	}

	return nil
}

// GetWorktreePath returns the path where the worktree will be created.
// Worktrees are always created in the .worktrees directory at the repo root.
func GetWorktreePath(name string) string {
	return filepath.Join(".worktrees", name)
}

// GetBranchName returns the full branch name with the configured prefix.
// If prefix is empty, defaults to "feature/".
func GetBranchName(name string, prefix string) string {
	if prefix == "" {
		prefix = "feature/"
	}
	return prefix + name
}

// worktreesEntryRegex matches .worktrees entries in .gitignore
// Matches: .worktrees, .worktrees/, .worktrees/*
var worktreesEntryRegex = regexp.MustCompile(`(?m)^\.worktrees(?:/\*?)?$`)

// hasWorktreesEntry checks if the .gitignore content contains a .worktrees entry.
func hasWorktreesEntry(content string) bool {
	return worktreesEntryRegex.MatchString(content)
}

// EnsureWorktreesInGitignore ensures that .worktrees is in the root .gitignore file.
// Creates the file if it doesn't exist, appends if .worktrees is not present.
func EnsureWorktreesInGitignore() error {
	return ensureWorktreesInGitignoreAt(".gitignore")
}

// ensureWorktreesInGitignoreAt ensures .worktrees is in the specified .gitignore file.
func ensureWorktreesInGitignoreAt(gitignorePath string) error {
	content, err := os.ReadFile(gitignorePath) //nolint:gosec // path is controlled by caller
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read .gitignore: %w", err)
	}

	contentStr := string(content)
	if hasWorktreesEntry(contentStr) {
		return nil
	}

	var newContent string
	switch {
	case len(content) == 0:
		newContent = ".worktrees\n"
	case strings.HasSuffix(contentStr, "\n"):
		newContent = contentStr + ".worktrees\n"
	default:
		newContent = contentStr + "\n.worktrees\n"
	}

	if err := os.WriteFile(gitignorePath, []byte(newContent), 0o600); err != nil {
		return fmt.Errorf("failed to write .gitignore: %w", err)
	}

	return nil
}

// newStartCmd creates the start command
func newStartCmd() *cobra.Command {
	var baseBranch string
	var scaffold bool
	var openEditor bool
	var useWorktree bool

	cmd := &cobra.Command{
		Use:   "start <name>",
		Short: "Start a new feature branch",
		Long: `Create a new feature branch and check it out.

By default, creates a branch and checks it out in the current directory.
Use --worktree to create an isolated worktree for parallel feature development.`,
		Example: `  # Basic usage - create and checkout feature branch
  entire start feature-x

  # Branch from develop instead of main
  entire start feature-x --base develop

  # Create in a worktree for parallel work
  entire start feature-x --worktree

  # With requirements scaffolding (requires configured template)
  entire start feature-x --scaffold

  # Open in editor after creation (with worktree)
  entire start feature-x --worktree --open`,
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				return errors.New("feature name required: entire start <name>")
			}
			if len(args) > 1 {
				return fmt.Errorf("expected 1 feature name, got %d", len(args))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			startSettings := GetStartSettings()

			// Resolve base branch at runtime if not explicitly provided
			resolvedBaseBranch := baseBranch
			if !cmd.Flags().Changed("base") {
				resolvedBaseBranch = startSettings.BaseBranch
			}

			// Resolve worktree mode: flag overrides setting
			resolvedUseWorktree := useWorktree
			if !cmd.Flags().Changed("worktree") {
				resolvedUseWorktree = startSettings.UseWorktrees
			}

			return runStart(name, resolvedBaseBranch, scaffold, openEditor, resolvedUseWorktree, cmd)
		},
	}

	// Use empty default for --base flag; actual default is resolved at runtime
	// from settings to ensure we read current settings, not settings at init time
	cmd.Flags().StringVar(&baseBranch, "base", "", "Base branch to create feature from (default: from settings or 'main')")
	cmd.Flags().BoolVar(&scaffold, "scaffold", false, "Create requirements doc from configured template")
	cmd.Flags().BoolVar(&openEditor, "open", false, "Open in editor ($EDITOR) - only with --worktree")
	cmd.Flags().BoolVar(&useWorktree, "worktree", false, "Create in a separate worktree for parallel work")

	return cmd
}

// runStart executes the start command logic
func runStart(name, baseBranch string, scaffold, openEditor, useWorktree bool, cmd *cobra.Command) error {
	// Validate feature name
	if err := ValidateFeatureName(name); err != nil {
		return err
	}

	repo, err := strategy.OpenRepository()
	if err != nil {
		return fmt.Errorf("not in a git repository: %w", err)
	}

	// Get settings
	startSettings := GetStartSettings()

	// Generate branch name
	branchName := GetBranchName(name, startSettings.BranchPrefix)

	// Validate branch doesn't already exist
	if branchExists(repo, branchName) {
		return fmt.Errorf("branch '%s' already exists", branchName)
	}

	// Validate base branch exists and get its hash
	baseHash, err := resolveBaseBranch(repo, baseBranch)
	if err != nil {
		return err
	}

	// Handle scaffold flag validation
	if scaffold {
		if startSettings.RequirementsTemplate == "" {
			return errors.New("no requirements template configured. Set 'start.requirementsTemplate' in .entire/settings.json")
		}
		if _, err := os.Stat(startSettings.RequirementsTemplate); os.IsNotExist(err) {
			return fmt.Errorf("requirements template not found: %s", startSettings.RequirementsTemplate)
		}
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	if useWorktree {
		return runStartWithWorktree(ctx, repo, name, branchName, baseBranch, baseHash, scaffold, openEditor, startSettings, cmd)
	}
	return runStartWithCheckout(repo, name, branchName, baseBranch, baseHash, scaffold, startSettings, cmd)
}

// resolveBaseBranch resolves the base branch to a commit hash.
// Checks local branches first, then remote branches.
func resolveBaseBranch(repo *git.Repository, baseBranch string) (plumbing.Hash, error) {
	// Try local branch first
	localRef := plumbing.NewBranchReferenceName(baseBranch)
	ref, err := repo.Reference(localRef, true)
	if err == nil {
		return ref.Hash(), nil
	}

	// Try remote branch (origin)
	remoteRef := plumbing.NewRemoteReferenceName("origin", baseBranch)
	ref, err = repo.Reference(remoteRef, true)
	if err == nil {
		return ref.Hash(), nil
	}

	return plumbing.ZeroHash, fmt.Errorf("base branch '%s' not found", baseBranch)
}

// runStartWithCheckout creates a branch and checks it out in the current directory
func runStartWithCheckout(repo *git.Repository, name, branchName, baseBranch string, baseHash plumbing.Hash, scaffold bool, settings *StartSettings, cmd *cobra.Command) error {
	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Check for uncommitted changes before creating branch
	// This prevents leaving the repo in a broken state if checkout fails
	status, err := worktree.Status()
	if err != nil {
		return fmt.Errorf("failed to get worktree status: %w", err)
	}
	if !status.IsClean() {
		return errors.New("working directory has uncommitted changes; commit or stash them first")
	}

	// Create new branch reference pointing to base commit
	branchRef := plumbing.NewBranchReferenceName(branchName)
	ref := plumbing.NewHashReference(branchRef, baseHash)
	if err := repo.Storer.SetReference(ref); err != nil {
		return fmt.Errorf("failed to create branch: %w", err)
	}

	// Checkout the new branch using git CLI instead of go-git to work around
	// go-git v5 bug where Checkout deletes untracked files
	// (see https://github.com/go-git/go-git/issues/970)
	checkoutCmd := exec.CommandContext(context.Background(), "git", "checkout", branchName)
	if output, err := checkoutCmd.CombinedOutput(); err != nil {
		// Rollback: delete the branch we just created
		_ = repo.Storer.RemoveReference(branchRef) //nolint:errcheck // best-effort rollback
		return fmt.Errorf("failed to checkout branch: %s: %w", strings.TrimSpace(string(output)), err)
	}

	// Handle scaffold if requested (in current directory)
	if scaffold {
		if err := scaffoldRequirements(".", name, settings.RequirementsTemplate); err != nil {
			return fmt.Errorf("failed to scaffold requirements: %w", err)
		}
	}

	// Print success message
	fmt.Fprintf(cmd.OutOrStdout(), "Created and checked out branch: %s (from %s)\n", branchName, baseBranch)
	if scaffold {
		fmt.Fprintf(cmd.OutOrStdout(), "Requirements: docs/requirements/%s/README.md\n", name)
	}
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), "Next steps:")
	fmt.Fprintln(cmd.OutOrStdout(), "  claude")

	return nil
}

// runStartWithWorktree creates a branch in a separate worktree
func runStartWithWorktree(ctx context.Context, repo *git.Repository, name, branchName, baseBranch string, baseHash plumbing.Hash, scaffold, openEditor bool, settings *StartSettings, cmd *cobra.Command) error {
	// Suppress unused parameter warning - baseHash reserved for future go-git worktree support
	_ = repo
	_ = baseHash

	// Get main repo root (allows creating sibling worktrees from inside a worktree)
	mainRoot, err := strategy.GetMainRepoRoot()
	if err != nil {
		return fmt.Errorf("failed to find main repository: %w", err)
	}

	// Worktree path is relative to main repo root
	worktreePath := filepath.Join(mainRoot, GetWorktreePath(name))

	// Validate worktree doesn't already exist
	if _, err := os.Stat(worktreePath); err == nil {
		return fmt.Errorf("worktree '%s' already exists at %s", name, worktreePath)
	}

	// Ensure .worktrees is in .gitignore (in main repo)
	if err := ensureWorktreesInGitignoreAt(filepath.Join(mainRoot, ".gitignore")); err != nil {
		return fmt.Errorf("failed to update .gitignore: %w", err)
	}

	// Create the worktree directory
	if err := os.MkdirAll(filepath.Dir(worktreePath), 0o755); err != nil { //nolint:gosec // worktree needs standard dir permissions
		return fmt.Errorf("failed to create .worktrees directory: %w", err)
	}

	// Create worktree with new branch using git command
	// Note: go-git doesn't support `git worktree add`, so we use native git
	gitCmd := exec.CommandContext(ctx, "git", "worktree", "add", "-b", branchName, worktreePath, baseBranch) //nolint:gosec // args are validated
	gitCmd.Dir = mainRoot
	if output, err := gitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create worktree: %s", strings.TrimSpace(string(output)))
	}

	// Handle scaffold if requested (in worktree)
	if scaffold {
		if err := scaffoldRequirements(worktreePath, name, settings.RequirementsTemplate); err != nil {
			return fmt.Errorf("failed to scaffold requirements: %w", err)
		}
	}

	// Print success message
	fmt.Fprintf(cmd.OutOrStdout(), "Created worktree at %s\n", worktreePath)
	fmt.Fprintf(cmd.OutOrStdout(), "Branch: %s (from %s)\n", branchName, baseBranch)
	if scaffold {
		fmt.Fprintf(cmd.OutOrStdout(), "Requirements: docs/requirements/%s/README.md\n", name)
	}
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), "Next steps:")
	fmt.Fprintf(cmd.OutOrStdout(), "  cd %s\n", worktreePath)
	fmt.Fprintln(cmd.OutOrStdout(), "  claude")

	// Open in editor if requested
	if openEditor {
		editor := os.Getenv("EDITOR")
		if editor == "" {
			fmt.Fprintln(cmd.OutOrStdout(), "\nNote: $EDITOR not set, skipping --open")
		} else {
			editorCmd := exec.CommandContext(ctx, editor, worktreePath) //nolint:gosec // EDITOR is user-configured
			editorCmd.Stdout = os.Stdout
			editorCmd.Stderr = os.Stderr
			editorCmd.Stdin = os.Stdin
			// Intentionally using Start() instead of Run() to allow the editor to run
			// in parallel with the terminal. The process becomes orphaned when we exit,
			// which is expected behavior for GUI editors (VS Code, Sublime, etc.).
			if err := editorCmd.Start(); err != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "\nNote: failed to open editor: %v\n", err)
			}
		}
	}

	return nil
}

// branchExists checks if a local branch exists
func branchExists(repo *git.Repository, branchName string) bool {
	refName := plumbing.NewBranchReferenceName(branchName)
	_, err := repo.Reference(refName, true)
	return err == nil
}

// scaffoldRequirements creates the requirements documentation from template
func scaffoldRequirements(worktreePath, name, templatePath string) error {
	// Read template (path is validated before calling this function)
	templateContent, err := os.ReadFile(templatePath) //nolint:gosec // path validated in caller
	if err != nil {
		return fmt.Errorf("failed to read template: %w", err)
	}

	// Create requirements directory in worktree
	reqDir := filepath.Join(worktreePath, "docs", "requirements", name)
	if err := os.MkdirAll(reqDir, 0o755); err != nil { //nolint:gosec // standard dir permissions
		return fmt.Errorf("failed to create requirements directory: %w", err)
	}

	// Write README.md
	readmePath := filepath.Join(reqDir, "README.md")
	if err := os.WriteFile(readmePath, templateContent, 0o644); err != nil { //nolint:gosec // standard file permissions
		return fmt.Errorf("failed to write requirements file: %w", err)
	}

	return nil
}
