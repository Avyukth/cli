package claudecode

import (
	"testing"
)

// Transcript type constants for tests
const (
	testTypeUser      = "user"
	testTypeAssistant = "assistant"
)

func TestParseTranscript(t *testing.T) {
	t.Parallel()

	data := []byte(`{"type":"user","uuid":"u1","message":{"content":"hello"}}
{"type":"assistant","uuid":"a1","message":{"content":[{"type":"text","text":"hi"}]}}
`)

	lines, err := ParseTranscript(data)
	if err != nil {
		t.Fatalf("ParseTranscript() error = %v", err)
	}

	if len(lines) != 2 {
		t.Errorf("ParseTranscript() got %d lines, want 2", len(lines))
	}

	if lines[0].Type != testTypeUser || lines[0].UUID != "u1" {
		t.Errorf("First line = %+v, want type=user, uuid=u1", lines[0])
	}

	if lines[1].Type != testTypeAssistant || lines[1].UUID != "a1" {
		t.Errorf("Second line = %+v, want type=assistant, uuid=a1", lines[1])
	}
}

func TestParseTranscript_SkipsMalformed(t *testing.T) {
	t.Parallel()

	data := []byte(`{"type":"user","uuid":"u1","message":{"content":"hello"}}
not valid json
{"type":"assistant","uuid":"a1","message":{"content":[]}}
`)

	lines, err := ParseTranscript(data)
	if err != nil {
		t.Fatalf("ParseTranscript() error = %v", err)
	}

	// Should skip the malformed line
	if len(lines) != 2 {
		t.Errorf("ParseTranscript() got %d lines, want 2 (skipping malformed)", len(lines))
	}
}

func TestSerializeTranscript(t *testing.T) {
	t.Parallel()

	lines := []TranscriptLine{
		{Type: "user", UUID: "u1"},
		{Type: "assistant", UUID: "a1"},
	}

	data, err := SerializeTranscript(lines)
	if err != nil {
		t.Fatalf("SerializeTranscript() error = %v", err)
	}

	// Parse back to verify round-trip
	parsed, err := ParseTranscript(data)
	if err != nil {
		t.Fatalf("ParseTranscript(serialized) error = %v", err)
	}

	if len(parsed) != 2 {
		t.Errorf("Round-trip got %d lines, want 2", len(parsed))
	}
}

func TestExtractModifiedFiles(t *testing.T) {
	t.Parallel()

	data := []byte(`{"type":"assistant","uuid":"a1","message":{"content":[{"type":"tool_use","name":"Write","input":{"file_path":"foo.go"}}]}}
{"type":"assistant","uuid":"a2","message":{"content":[{"type":"tool_use","name":"Edit","input":{"file_path":"bar.go"}}]}}
{"type":"assistant","uuid":"a3","message":{"content":[{"type":"tool_use","name":"Bash","input":{"command":"ls"}}]}}
{"type":"assistant","uuid":"a4","message":{"content":[{"type":"tool_use","name":"Write","input":{"file_path":"foo.go"}}]}}
`)

	lines, err := ParseTranscript(data)
	if err != nil {
		t.Fatalf("ParseTranscript() error = %v", err)
	}
	files := ExtractModifiedFiles(lines)

	// Should have foo.go and bar.go (deduplicated, Bash not included)
	if len(files) != 2 {
		t.Errorf("ExtractModifiedFiles() got %d files, want 2", len(files))
	}

	hasFile := func(name string) bool {
		for _, f := range files {
			if f == name {
				return true
			}
		}
		return false
	}

	if !hasFile("foo.go") {
		t.Error("ExtractModifiedFiles() missing foo.go")
	}
	if !hasFile("bar.go") {
		t.Error("ExtractModifiedFiles() missing bar.go")
	}
}

func TestExtractLastUserPrompt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data string
		want string
	}{
		{
			name: "string content",
			data: `{"type":"user","uuid":"u1","message":{"content":"first"}}
{"type":"assistant","uuid":"a1","message":{"content":[]}}
{"type":"user","uuid":"u2","message":{"content":"second"}}`,
			want: "second",
		},
		{
			name: "array content with text block",
			data: `{"type":"user","uuid":"u1","message":{"content":[{"type":"text","text":"hello world"}]}}`,
			want: "hello world",
		},
		{
			name: "empty transcript",
			data: ``,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			lines, err := ParseTranscript([]byte(tt.data))
			if err != nil && tt.data != "" {
				t.Fatalf("ParseTranscript() error = %v", err)
			}
			got := ExtractLastUserPrompt(lines)
			if got != tt.want {
				t.Errorf("ExtractLastUserPrompt() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTruncateAtUUID(t *testing.T) {
	t.Parallel()

	data := []byte(`{"type":"user","uuid":"u1","message":{}}
{"type":"assistant","uuid":"a1","message":{}}
{"type":"user","uuid":"u2","message":{}}
{"type":"assistant","uuid":"a2","message":{}}
`)

	lines, err := ParseTranscript(data)
	if err != nil {
		t.Fatalf("ParseTranscript() error = %v", err)
	}

	tests := []struct {
		name     string
		uuid     string
		wantLen  int
		lastUUID string
	}{
		{"truncate at u1", "u1", 1, "u1"},
		{"truncate at a1", "a1", 2, "a1"},
		{"truncate at u2", "u2", 3, "u2"},
		{"truncate at a2", "a2", 4, "a2"},
		{"empty uuid returns all", "", 4, "a2"},
		{"unknown uuid returns all", "unknown", 4, "a2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			truncated := TruncateAtUUID(lines, tt.uuid)
			if len(truncated) != tt.wantLen {
				t.Errorf("TruncateAtUUID(%q) got %d lines, want %d", tt.uuid, len(truncated), tt.wantLen)
			}
			if len(truncated) > 0 && truncated[len(truncated)-1].UUID != tt.lastUUID {
				t.Errorf("TruncateAtUUID(%q) last UUID = %q, want %q", tt.uuid, truncated[len(truncated)-1].UUID, tt.lastUUID)
			}
		})
	}
}

func TestFindCheckpointUUID(t *testing.T) {
	t.Parallel()

	data := []byte(`{"type":"assistant","uuid":"a1","message":{"content":[{"type":"tool_use","id":"tool1"}]}}
{"type":"user","uuid":"u1","message":{"content":[{"type":"tool_result","tool_use_id":"tool1"}]}}
{"type":"assistant","uuid":"a2","message":{"content":[{"type":"tool_use","id":"tool2"}]}}
{"type":"user","uuid":"u2","message":{"content":[{"type":"tool_result","tool_use_id":"tool2"}]}}
`)

	lines, err := ParseTranscript(data)
	if err != nil {
		t.Fatalf("ParseTranscript() error = %v", err)
	}

	tests := []struct {
		toolUseID string
		wantUUID  string
		wantFound bool
	}{
		{"tool1", "u1", true},
		{"tool2", "u2", true},
		{"unknown", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.toolUseID, func(t *testing.T) {
			t.Parallel()
			uuid, found := FindCheckpointUUID(lines, tt.toolUseID)
			if found != tt.wantFound {
				t.Errorf("FindCheckpointUUID(%q) found = %v, want %v", tt.toolUseID, found, tt.wantFound)
			}
			if uuid != tt.wantUUID {
				t.Errorf("FindCheckpointUUID(%q) uuid = %q, want %q", tt.toolUseID, uuid, tt.wantUUID)
			}
		})
	}
}
