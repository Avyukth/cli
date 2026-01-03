package cli

import "encoding/json"

// transcriptLine represents a single line in the transcript
type transcriptLine struct {
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

// toolInput represents the input to various tools
type toolInput struct {
	FilePath     string `json:"file_path,omitempty"`
	NotebookPath string `json:"notebook_path,omitempty"`
	Description  string `json:"description,omitempty"`
	Command      string `json:"command,omitempty"`
	Pattern      string `json:"pattern,omitempty"`
}
