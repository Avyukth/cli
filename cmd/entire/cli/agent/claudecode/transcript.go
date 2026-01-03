package claudecode

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Transcript parsing types - Claude Code uses JSONL format

// TranscriptLine represents a single line in Claude's JSONL transcript
type TranscriptLine struct {
	Type    string          `json:"type"`
	UUID    string          `json:"uuid"`
	Message json.RawMessage `json:"message"`
}

// userMessage represents a user message in the transcript
type userMessage struct {
	Content interface{} `json:"content"`
}

// assistantMessage represents an assistant message in the transcript
type assistantMessage struct {
	Content []contentBlock `json:"content"`
}

// contentBlock represents a block within an assistant message
type contentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

// toolInput represents the input to file modification tools
type toolInput struct {
	FilePath     string `json:"file_path,omitempty"`
	NotebookPath string `json:"notebook_path,omitempty"`
}

// Scanner buffer size for large transcript files (10MB)
const scannerBufferSize = 10 * 1024 * 1024

// ParseTranscript parses raw JSONL content into transcript lines
func ParseTranscript(data []byte) ([]TranscriptLine, error) {
	var lines []TranscriptLine
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, scannerBufferSize), scannerBufferSize)

	for scanner.Scan() {
		var line TranscriptLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue // Skip malformed lines
		}
		lines = append(lines, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan transcript: %w", err)
	}
	return lines, nil
}

// SerializeTranscript converts transcript lines back to JSONL bytes
func SerializeTranscript(lines []TranscriptLine) ([]byte, error) {
	var buf bytes.Buffer
	for _, line := range lines {
		data, err := json.Marshal(line)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal line: %w", err)
		}
		buf.Write(data)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}

// ExtractModifiedFiles extracts files modified by tool calls from transcript
func ExtractModifiedFiles(lines []TranscriptLine) []string {
	fileSet := make(map[string]bool)
	var files []string

	for _, line := range lines {
		if line.Type != "assistant" {
			continue
		}

		var msg assistantMessage
		if err := json.Unmarshal(line.Message, &msg); err != nil {
			continue
		}

		for _, block := range msg.Content {
			if block.Type != "tool_use" {
				continue
			}

			// Check if it's a file modification tool
			isModifyTool := false
			for _, name := range FileModificationTools {
				if block.Name == name {
					isModifyTool = true
					break
				}
			}

			if !isModifyTool {
				continue
			}

			var input toolInput
			if err := json.Unmarshal(block.Input, &input); err != nil {
				continue
			}

			file := input.FilePath
			if file == "" {
				file = input.NotebookPath
			}

			if file != "" && !fileSet[file] {
				fileSet[file] = true
				files = append(files, file)
			}
		}
	}

	return files
}

// ExtractLastUserPrompt extracts the last user message from transcript
func ExtractLastUserPrompt(lines []TranscriptLine) string {
	for i := len(lines) - 1; i >= 0; i-- {
		if lines[i].Type != "user" {
			continue
		}

		var msg userMessage
		if err := json.Unmarshal(lines[i].Message, &msg); err != nil {
			continue
		}

		// Handle string content
		if str, ok := msg.Content.(string); ok {
			return str
		}

		// Handle array content (text blocks)
		if arr, ok := msg.Content.([]interface{}); ok {
			var texts []string
			for _, item := range arr {
				if m, ok := item.(map[string]interface{}); ok {
					if m["type"] == "text" {
						if text, ok := m["text"].(string); ok {
							texts = append(texts, text)
						}
					}
				}
			}
			if len(texts) > 0 {
				return strings.Join(texts, "\n\n")
			}
		}
	}
	return ""
}

// TruncateAtUUID returns transcript lines up to and including the line with given UUID
func TruncateAtUUID(lines []TranscriptLine, uuid string) []TranscriptLine {
	if uuid == "" {
		return lines
	}

	for i, line := range lines {
		if line.UUID == uuid {
			return lines[:i+1]
		}
	}

	// UUID not found, return full transcript
	return lines
}

// toolResultBlock represents a tool_result in a user message
type toolResultBlock struct {
	Type      string `json:"type"`
	ToolUseID string `json:"tool_use_id"`
}

// userMessageWithToolResults represents a user message that may contain tool results
type userMessageWithToolResults struct {
	Content []toolResultBlock `json:"content"`
}

// FindCheckpointUUID finds the UUID of the message containing the tool_result
// for the given tool_use_id
func FindCheckpointUUID(lines []TranscriptLine, toolUseID string) (string, bool) {
	for _, line := range lines {
		if line.Type != "user" {
			continue
		}

		var msg userMessageWithToolResults
		if err := json.Unmarshal(line.Message, &msg); err != nil {
			continue
		}

		for _, block := range msg.Content {
			if block.Type == "tool_result" && block.ToolUseID == toolUseID {
				return line.UUID, true
			}
		}
	}
	return "", false
}
