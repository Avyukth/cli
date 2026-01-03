package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateFeatureName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
	}{
		// Valid names
		{name: "simple lowercase", input: "feature", wantErr: false},
		{name: "with hyphen", input: "my-feature", wantErr: false},
		{name: "with underscore", input: "my_feature", wantErr: false},
		{name: "with numbers", input: "feature123", wantErr: false},
		{name: "uppercase", input: "Feature", wantErr: false},
		{name: "mixed case with hyphens", input: "My-Feature-2", wantErr: false},
		{name: "all valid chars", input: "abc_123-XYZ", wantErr: false},

		// Invalid names
		{name: "empty", input: "", wantErr: true, errMsg: "feature name is required"},
		{name: "with space", input: "my feature", wantErr: true, errMsg: "invalid feature name 'my feature': use alphanumeric characters, hyphens, and underscores only"},
		{name: "with dot", input: "my.feature", wantErr: true, errMsg: "invalid feature name 'my.feature': use alphanumeric characters, hyphens, and underscores only"},
		{name: "with slash", input: "my/feature", wantErr: true, errMsg: "invalid feature name 'my/feature': use alphanumeric characters, hyphens, and underscores only"},
		{name: "with special char", input: "my@feature", wantErr: true, errMsg: "invalid feature name 'my@feature': use alphanumeric characters, hyphens, and underscores only"},
		{name: "with exclamation", input: "feature!", wantErr: true, errMsg: "invalid feature name 'feature!': use alphanumeric characters, hyphens, and underscores only"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFeatureName(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateFeatureName(%q) expected error, got nil", tt.input)
					return
				}
				if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("ValidateFeatureName(%q) error = %q, want %q", tt.input, err.Error(), tt.errMsg)
				}
			} else if err != nil {
				t.Errorf("ValidateFeatureName(%q) unexpected error: %v", tt.input, err)
			}
		})
	}
}

func TestGetWorktreePath(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{name: "simple", want: ".worktrees/simple"},
		{name: "my-feature", want: ".worktrees/my-feature"},
		{name: "feature_123", want: ".worktrees/feature_123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetWorktreePath(tt.name)
			if got != tt.want {
				t.Errorf("GetWorktreePath(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestGetBranchName(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		want   string
	}{
		{name: "simple", prefix: "feature/", want: "feature/simple"},
		{name: "my-feature", prefix: "feature/", want: "feature/my-feature"},
		{name: "bugfix", prefix: "fix/", want: "fix/bugfix"},
		{name: "test", prefix: "", want: "feature/test"}, // empty prefix uses default
	}

	for _, tt := range tests {
		t.Run(tt.name+"_"+tt.prefix, func(t *testing.T) {
			got := GetBranchName(tt.name, tt.prefix)
			if got != tt.want {
				t.Errorf("GetBranchName(%q, %q) = %q, want %q", tt.name, tt.prefix, got, tt.want)
			}
		})
	}
}

func TestEnsureWorktreesInGitignore(t *testing.T) {
	tests := []struct {
		name            string
		createFile      bool   // whether to create the gitignore file before the test
		existingContent string // content to write if createFile is true
		wantContains    string
		shouldModify    bool
	}{
		{
			name:            "no gitignore file",
			createFile:      false,
			existingContent: "",
			wantContains:    ".worktrees",
			shouldModify:    true,
		},
		{
			name:            "empty gitignore",
			createFile:      true,
			existingContent: "",
			wantContains:    ".worktrees",
			shouldModify:    true,
		},
		{
			name:            "gitignore without worktrees",
			createFile:      true,
			existingContent: "node_modules/\n.env\n",
			wantContains:    ".worktrees",
			shouldModify:    true,
		},
		{
			name:            "gitignore with worktrees already",
			createFile:      true,
			existingContent: "node_modules/\n.worktrees\n.env\n",
			wantContains:    ".worktrees",
			shouldModify:    false,
		},
		{
			name:            "gitignore with worktrees/ variant",
			createFile:      true,
			existingContent: "node_modules/\n.worktrees/\n",
			wantContains:    ".worktrees/",
			shouldModify:    false,
		},
		{
			name:            "gitignore without trailing newline",
			createFile:      true,
			existingContent: "node_modules/",
			wantContains:    ".worktrees",
			shouldModify:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			t.Chdir(tmpDir)

			gitignorePath := filepath.Join(tmpDir, ".gitignore")

			// Create existing gitignore if specified
			if tt.createFile {
				if err := os.WriteFile(gitignorePath, []byte(tt.existingContent), 0o644); err != nil {
					t.Fatalf("Failed to write initial .gitignore: %v", err)
				}
			}

			// Run the function
			err := EnsureWorktreesInGitignore()
			if err != nil {
				t.Fatalf("EnsureWorktreesInGitignore() error = %v", err)
			}

			// Read the result
			content, err := os.ReadFile(gitignorePath)
			if err != nil {
				t.Fatalf("Failed to read .gitignore: %v", err)
			}

			contentStr := string(content)
			if !strings.Contains(contentStr, tt.wantContains) {
				t.Errorf("gitignore content = %q, want to contain %q", contentStr, tt.wantContains)
			}

			// Check that worktrees appears only once (not duplicated)
			count := strings.Count(contentStr, ".worktrees")
			if count > 1 {
				t.Errorf("gitignore has %d occurrences of .worktrees, want 1", count)
			}
		})
	}
}

func TestHasWorktreesEntry(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{name: "empty", content: "", want: false},
		{name: "no worktrees", content: "node_modules/\n.env\n", want: false},
		{name: "has .worktrees", content: ".worktrees\n", want: true},
		{name: "has .worktrees/", content: ".worktrees/\n", want: true},
		{name: "has .worktrees/*", content: ".worktrees/*\n", want: true},
		{name: "worktrees in middle", content: "a\n.worktrees\nb\n", want: true},
		{name: "worktrees with comment", content: "# worktrees\n.worktrees\n", want: true},
		{name: "partial match - not worktrees", content: "worktrees\n", want: false},
		{name: "partial match - my.worktrees", content: "my.worktrees\n", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasWorktreesEntry(tt.content)
			if got != tt.want {
				t.Errorf("hasWorktreesEntry(%q) = %v, want %v", tt.content, got, tt.want)
			}
		})
	}
}
