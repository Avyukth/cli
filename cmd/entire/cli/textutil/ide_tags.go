package textutil

import (
	"regexp"
	"strings"
)

// ideContextTagRegex matches IDE-injected context tags like <ide_opened_file>...</ide_opened_file>
// and <ide_selection>...</ide_selection>. These are injected by the VSCode extension.
var ideContextTagRegex = regexp.MustCompile(`(?s)<ide_[^>]*>.*?</ide_[^>]*>`)

// StripIDEContextTags removes IDE-injected context tags from prompt text.
// The VSCode extension injects tags like:
//   - <ide_opened_file>...</ide_opened_file> - currently open file
//   - <ide_selection>...</ide_selection> - selected code in editor
//
// These shouldn't appear in commit messages or session descriptions.
func StripIDEContextTags(text string) string {
	result := ideContextTagRegex.ReplaceAllString(text, "")
	return strings.TrimSpace(result)
}
