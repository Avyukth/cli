package session

import (
	"testing"
)

func TestSession_IsSubSession(t *testing.T) {
	tests := []struct {
		name     string
		session  Session
		expected bool
	}{
		{
			name: "top-level session with empty ParentID",
			session: Session{
				ID:       "session-123",
				ParentID: "",
			},
			expected: false,
		},
		{
			name: "sub-session with ParentID set",
			session: Session{
				ID:        "session-456",
				ParentID:  "session-123",
				ToolUseID: "toolu_abc",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.session.IsSubSession()
			if result != tt.expected {
				t.Errorf("IsSubSession() = %v, want %v", result, tt.expected)
			}
		})
	}
}
