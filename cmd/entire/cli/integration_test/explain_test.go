//go:build integration

package integration

import (
	"strings"
	"testing"
)

func TestExplain_NoCurrentSession(t *testing.T) {
	t.Parallel()
	RunForAllStrategies(t, func(t *testing.T, env *TestEnv, strategyName string) {
		// Try to explain without a current session
		output, err := env.RunCLIWithError("explain")

		if err == nil {
			t.Errorf("expected error when no current session, got output: %s", output)
			return
		}

		if !strings.Contains(output, "no active session") {
			t.Errorf("expected 'no active session' error, got: %s", output)
		}
	})
}

func TestExplain_SessionNotFound(t *testing.T) {
	t.Parallel()
	RunForAllStrategies(t, func(t *testing.T, env *TestEnv, strategyName string) {
		// Try to explain a non-existent session
		output, err := env.RunCLIWithError("explain", "--session", "nonexistent-session-id")

		if err == nil {
			t.Errorf("expected error for nonexistent session, got output: %s", output)
			return
		}

		if !strings.Contains(output, "session not found") {
			t.Errorf("expected 'session not found' error, got: %s", output)
		}
	})
}

func TestExplain_BothFlagsError(t *testing.T) {
	t.Parallel()
	RunForAllStrategies(t, func(t *testing.T, env *TestEnv, strategyName string) {
		// Try to provide both --session and --commit flags
		output, err := env.RunCLIWithError("explain", "--session", "test-session", "--commit", "abc123")

		if err == nil {
			t.Errorf("expected error when both flags provided, got output: %s", output)
			return
		}

		if !strings.Contains(strings.ToLower(output), "cannot specify both") {
			t.Errorf("expected 'cannot specify both' error, got: %s", output)
		}
	})
}
