package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"entire.io/cli/cmd/entire/cli/agent"
	"entire.io/cli/cmd/entire/cli/paths"
	"entire.io/cli/cmd/entire/cli/strategy"

	"github.com/charmbracelet/huh"
	"github.com/go-git/go-git/v5"
	"github.com/spf13/cobra"
)

func newResumeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resume <branch>",
		Short: "Switch to a branch and resume its session",
		Long: `Switch to a local branch and resume the agent session from its last commit.

This command:
1. Checks out the specified branch
2. Finds the session ID from the last commit's trailers
3. Restores the session log if it doesn't exist locally
4. Shows the command to resume the session

If the branch doesn't exist locally but exists on origin, you'll be prompted
to fetch it.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if checkDisabledGuard(cmd.OutOrStdout()) {
				return nil
			}
			return runResume(args[0])
		},
	}

	return cmd
}

func runResume(branchName string) error {
	// Check if we're already on this branch
	currentBranch, err := GetCurrentBranch()
	if err == nil && currentBranch == branchName {
		// Already on the branch, skip checkout
		return resumeFromCurrentBranch(branchName)
	}

	// Check if branch exists locally
	exists, err := BranchExistsLocally(branchName)
	if err != nil {
		return fmt.Errorf("failed to check branch: %w", err)
	}

	if !exists {
		// Branch doesn't exist locally, check if it exists on remote
		remoteExists, err := BranchExistsOnRemote(branchName)
		if err != nil {
			return fmt.Errorf("failed to check remote branch: %w", err)
		}

		if !remoteExists {
			return fmt.Errorf("branch '%s' not found locally or on origin", branchName)
		}

		// Ask user if they want to fetch from remote
		shouldFetch, err := promptFetchFromRemote(branchName)
		if err != nil {
			return err
		}
		if !shouldFetch {
			return nil
		}

		// Fetch and checkout the remote branch
		fmt.Fprintf(os.Stderr, "Fetching branch '%s' from origin...\n", branchName)
		if err := FetchAndCheckoutRemoteBranch(branchName); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Switched to branch '%s'\n", branchName)
	} else {
		// Branch exists locally, check for uncommitted changes before checkout
		hasChanges, err := HasUncommittedChanges()
		if err != nil {
			return fmt.Errorf("failed to check for uncommitted changes: %w", err)
		}
		if hasChanges {
			return errors.New("you have uncommitted changes. Please commit or stash them first")
		}

		// Checkout the branch
		if err := CheckoutBranch(branchName); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Switched to branch '%s'\n", branchName)
	}

	return resumeFromCurrentBranch(branchName)
}

func resumeFromCurrentBranch(branchName string) error {
	repo, err := openRepository()
	if err != nil {
		return fmt.Errorf("not a git repository: %w", err)
	}

	// Get the HEAD commit
	head, err := repo.Head()
	if err != nil {
		return fmt.Errorf("failed to get HEAD: %w", err)
	}

	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return fmt.Errorf("failed to get commit: %w", err)
	}

	// Extract checkpoint from last commit
	checkpointID, found := paths.ParseCheckpointTrailer(commit.Message)
	if !found {
		fmt.Fprintf(os.Stderr, "No Entire checkpoint found for the last commit on branch '%s'\n", branchName)
		fmt.Fprintf(os.Stderr, "Commit: %s %s\n", head.Hash().String()[:7], firstLine(commit.Message))
		return nil
	}

	// Get metadata branch tree for lookups
	metadataTree, err := strategy.GetMetadataBranchTree(repo)
	if err != nil {
		// No local metadata branch, check if remote has it
		return checkRemoteMetadata(repo, checkpointID)
	}

	// Look up metadata from sharded path
	metadata, err := strategy.ReadCheckpointMetadata(metadataTree, paths.CheckpointPath(checkpointID))
	if err != nil {
		// Checkpoint exists in commit but no local metadata - check remote
		return checkRemoteMetadata(repo, checkpointID)
	}

	return resumeSession(metadata.SessionID, checkpointID)
}

// checkRemoteMetadata checks if checkpoint metadata exists on origin/entire/sessions
// and provides guidance to the user.
func checkRemoteMetadata(repo *git.Repository, checkpointID string) error {
	// Try to get remote metadata branch tree
	remoteTree, err := strategy.GetRemoteMetadataBranchTree(repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Checkpoint '%s' found in commit but session metadata not available\n", checkpointID)
		fmt.Fprintf(os.Stderr, "The entire/sessions branch may not exist locally or on the remote.\n")
		return nil //nolint:nilerr // Informational message, not a fatal error
	}

	// Check if the checkpoint exists on the remote
	_, err = strategy.ReadCheckpointMetadata(remoteTree, paths.CheckpointPath(checkpointID))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Checkpoint '%s' found in commit but session metadata not available\n", checkpointID)
		return nil //nolint:nilerr // Informational message, not a fatal error
	}

	// Metadata exists on remote but not locally
	fmt.Fprintf(os.Stderr, "Checkpoint '%s' found in commit but session metadata not available locally\n", checkpointID)
	fmt.Fprintf(os.Stderr, "The metadata exists on origin. To fetch it, run:\n")
	fmt.Fprintf(os.Stderr, "  git fetch origin entire/sessions:entire/sessions\n")
	fmt.Fprintf(os.Stderr, "\nThen run this command again.\n")
	return nil
}

// resumeSession restores and displays the resume command for a specific session.
func resumeSession(sessionID, checkpointID string) error {
	// Get the current agent (auto-detect or use default)
	ag, err := agent.Detect()
	if err != nil {
		ag = agent.Default()
		if ag == nil {
			return fmt.Errorf("no agent available: %w", err)
		}
	}

	// Get session directory for this agent
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	sessionDir, err := ag.GetSessionDir(cwd)
	if err != nil {
		return fmt.Errorf("failed to determine session directory: %w", err)
	}

	// Extract agent-specific session ID from Entire session ID
	agentSessionID := ag.ExtractAgentSessionID(sessionID)
	sessionLogPath := filepath.Join(sessionDir, agentSessionID+".jsonl")

	// Check if session log already exists
	if !fileExists(sessionLogPath) {
		// Restore the session log
		strat := GetStrategy()

		logContent, _, err := strat.GetSessionLog(checkpointID)
		if err != nil {
			if errors.Is(err, strategy.ErrNoMetadata) {
				fmt.Fprintf(os.Stderr, "Session '%s' found in commit trailer but session log not available\n", sessionID)
				fmt.Fprintf(os.Stderr, "\nTo continue this session, run:\n")
				fmt.Fprintf(os.Stderr, "  %s\n", ag.FormatResumeCommand(agentSessionID))
				return nil
			}
			return fmt.Errorf("failed to get session log: %w", err)
		}

		// Create an AgentSession with the native data
		agentSession := &agent.AgentSession{
			SessionID:  agentSessionID,
			AgentName:  ag.Name(),
			RepoPath:   cwd,
			SessionRef: sessionLogPath,
			NativeData: logContent,
		}

		// Create directory if it doesn't exist
		if err := os.MkdirAll(sessionDir, 0o700); err != nil {
			return fmt.Errorf("failed to create session directory: %w", err)
		}

		// Write the session using the agent's WriteSession method
		if err := ag.WriteSession(agentSession); err != nil {
			return fmt.Errorf("failed to write session: %w", err)
		}

		fmt.Fprintf(os.Stderr, "Session restored to: %s\n", sessionLogPath)
	}

	fmt.Fprintf(os.Stderr, "Session: %s\n", sessionID)
	fmt.Fprintf(os.Stderr, "\nTo continue this session, run:\n")
	fmt.Fprintf(os.Stderr, "  %s\n", ag.FormatResumeCommand(agentSessionID))

	return nil
}

func promptFetchFromRemote(branchName string) (bool, error) {
	var confirmed bool

	form := NewAccessibleForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("Branch '%s' not found locally. Fetch from origin?", branchName)).
				Value(&confirmed),
		),
	)

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get confirmation: %w", err)
	}

	return confirmed, nil
}

// firstLine returns the first line of a string
func firstLine(s string) string {
	for i, c := range s {
		if c == '\n' {
			return s[:i]
		}
	}
	return s
}
