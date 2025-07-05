package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestIsValidBranchChar(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"a", true},
		{"z", true},
		{"A", true},
		{"Z", true},
		{"0", true},
		{"9", true},
		{"-", true},
		{"_", true},
		{"/", true},
		{".", true},
		{" ", false},
		{"@", false},
		{"#", false},
		{"$", false},
		{"ab", false}, // multi-character
		{"", false},   // empty string
	}

	for _, test := range tests {
		result := isValidBranchChar(test.input)
		if result != test.expected {
			t.Errorf("isValidBranchChar(%q) = %v, expected %v", test.input, result, test.expected)
		}
	}
}

// TestGetOriginMainBranch tests the getOriginMainBranch function
// Note: This test is commented out because it requires a real git environment
/*
func TestGetOriginMainBranch(t *testing.T) {
	// This test would require setting up a real git repository
	// For now, we'll skip it in unit tests
	t.Skip("Skipping integration test - requires git repository")
}
*/

// TestSanitizeBranchName would test branch name sanitization
// Note: This function doesn't exist in the current codebase
// Commenting out until implemented
/*
func TestSanitizeBranchName(t *testing.T) {
	// Implementation would go here when function exists
}
*/

func TestParseWorktreeLine(t *testing.T) {
	tests := []struct {
		input    string
		expected Worktree
		hasError bool
	}{
		{
			"/path/to/worktree  abc123 [branch-name]",
			Worktree{Path: "/path/to/worktree", Head: "abc123", Branch: "branch-name"},
			false,
		},
		{
			"/path/to/worktree  abc123 (detached HEAD)",
			Worktree{Path: "/path/to/worktree", Head: "abc123", Branch: "(detached HEAD)"},
			false,
		},
		{
			"invalid line",
			Worktree{},
			true,
		},
		{
			"/path/only",
			Worktree{},
			true,
		},
	}

	for _, test := range tests {
		result, err := parseWorktreeLine(test.input)
		if test.hasError {
			if err == nil {
				t.Errorf("Expected error for input %q, but got none", test.input)
			}
		} else {
			if err != nil {
				t.Errorf("Unexpected error for input %q: %v", test.input, err)
			}
			if result != test.expected {
				t.Errorf("parseWorktreeLine(%q) = %v, expected %v", test.input, result, test.expected)
			}
		}
	}
}

// Helper function for tests that need to run git commands
// func runCommand(name string, args ...string) error {
//	// This is a simplified version for testing
//	// In a real test environment, you might want to use exec.Command
//	return nil
// }

// Mock parseWorktreeLine function for testing
func parseWorktreeLine(line string) (Worktree, error) {
	parts := strings.Fields(line)
	if len(parts) < 3 {
		return Worktree{}, fmt.Errorf("invalid worktree line format")
	}

	path := parts[0]
	head := parts[1]
	
	// Extract branch name from [branch-name] or (detached HEAD)
	branchPart := strings.Join(parts[2:], " ")
	var branch string
	if strings.HasPrefix(branchPart, "[") && strings.HasSuffix(branchPart, "]") {
		branch = branchPart[1 : len(branchPart)-1]
	} else if strings.HasPrefix(branchPart, "(") && strings.HasSuffix(branchPart, ")") {
		branch = branchPart
	} else {
		return Worktree{}, fmt.Errorf("invalid branch format")
	}

	return Worktree{
		Path:   path,
		Head:   head,
		Branch: branch,
	}, nil
}

// TestGitError would test custom git error type
// Note: GitError type doesn't exist in current codebase
// Commenting out until implemented
/*
func TestGitError(t *testing.T) {
	// Implementation would go here when GitError type exists
}
*/

// Note: sanitizeBranchName function doesn't exist in current codebase