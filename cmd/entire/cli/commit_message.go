package cli

import (
	"strings"
)

// generateCommitMessage creates a commit message from the user's original prompt
func generateCommitMessage(originalPrompt string) string {
	if originalPrompt != "" {
		cleaned := cleanPromptForCommit(originalPrompt)
		if cleaned != "" {
			return cleaned
		}
	}

	return "Claude Code session updates"
}

// cleanPromptForCommit cleans up a user prompt to make it suitable as a commit message
// Uses a loop to remove all matching prefixes until none remain
func cleanPromptForCommit(prompt string) string {
	cleaned := prompt

	prefixes := []string{
		"Can you ",
		"can you ",
		"Please ",
		"please ",
		"Let's ",
		"let's ",
		"Could you ",
		"could you ",
		"Would you ",
		"would you ",
		"I want you to ",
		"I'd like you to ",
		"I need you to ",
	}

	// Loop until no prefix is found
	for {
		found := false
		for _, prefix := range prefixes {
			if strings.HasPrefix(cleaned, prefix) {
				cleaned = strings.TrimPrefix(cleaned, prefix)
				found = true
				break
			}
		}
		if !found {
			break
		}
	}

	cleaned = strings.TrimSuffix(cleaned, "?")
	cleaned = strings.TrimSpace(cleaned)

	if len(cleaned) > 72 {
		cleaned = strings.TrimSpace(cleaned[:72])
	}

	// Capitalize first letter
	if len(cleaned) > 0 {
		cleaned = strings.ToUpper(string(cleaned[0])) + cleaned[1:]
	}

	return cleaned
}
