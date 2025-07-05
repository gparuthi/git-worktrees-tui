package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestInitialModel(t *testing.T) {
	model := initialModel()

	// Test initial state
	if model.view != "worktrees" {
		t.Errorf("Expected initial view to be 'worktrees', got %q", model.view)
	}

	if model.cursor != 0 {
		t.Errorf("Expected initial cursor to be 0, got %d", model.cursor)
	}

	if model.scrollOffset != 0 {
		t.Errorf("Expected initial scrollOffset to be 0, got %d", model.scrollOffset)
	}

	if model.filtering {
		t.Error("Expected filtering to be false initially")
	}

	if model.creatingBranch {
		t.Error("Expected creatingBranch to be false initially")
	}

	if model.deletingWorktree {
		t.Error("Expected deletingWorktree to be false initially")
	}

	if model.newBranchName != "" {
		t.Errorf("Expected newBranchName to be empty initially, got %q", model.newBranchName)
	}

	if model.filterText != "" {
		t.Errorf("Expected filterText to be empty initially, got %q", model.filterText)
	}
}

func TestModelUpdate_QuitCommands(t *testing.T) {
	m := initialModel()

	// Test Ctrl+C
	keyMsg := tea.KeyMsg{Type: tea.KeyCtrlC}
	newModel, cmd := m.Update(keyMsg)
	if cmd == nil {
		t.Error("Expected quit command for Ctrl+C")
	}

	// Test 'q' key
	keyMsg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	m = newModel.(model)
	newModel, cmd = m.Update(keyMsg)
	if cmd == nil {
		t.Error("Expected quit command for 'q' key")
	}
}

func TestModelUpdate_TabSwitching(t *testing.T) {
	m := initialModel()

	// Start in worktrees view
	if m.view != "worktrees" {
		t.Errorf("Expected to start in worktrees view, got %q", m.view)
	}

	// Press tab to switch to branches
	keyMsg := tea.KeyMsg{Type: tea.KeyTab}
	newModel, _ := m.Update(keyMsg)
	m = newModel.(model)

	if m.view != "branches" {
		t.Errorf("Expected to switch to branches view, got %q", m.view)
	}

	if m.cursor != 0 {
		t.Errorf("Expected cursor to reset to 0 after view switch, got %d", m.cursor)
	}

	if m.scrollOffset != 0 {
		t.Errorf("Expected scrollOffset to reset to 0 after view switch, got %d", m.scrollOffset)
	}

	// Press tab again to switch back to worktrees
	newModel, _ = m.Update(keyMsg)
	m = newModel.(model)

	if m.view != "worktrees" {
		t.Errorf("Expected to switch back to worktrees view, got %q", m.view)
	}
}

func TestModelUpdate_BranchCreation(t *testing.T) {
	m := initialModel()
	m.view = "branches" // Set to branches view

	// Press 'n' to start creating branch
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}}
	newModel, _ := m.Update(keyMsg)
	m = newModel.(model)

	if !m.creatingBranch {
		t.Error("Expected creatingBranch to be true after pressing 'n'")
	}

	if m.newBranchName != "" {
		t.Errorf("Expected newBranchName to be empty initially, got %q", m.newBranchName)
	}

	// Type valid characters
	testChars := []rune{'f', 'e', 'a', 't', 'u', 'r', 'e', '-', 'b', 'r', 'a', 'n', 'c', 'h'}
	for _, char := range testChars {
		keyMsg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{char}}
		newModel, _ = m.Update(keyMsg)
		m = newModel.(model)
	}

	expected := "feature-branch"
	if m.newBranchName != expected {
		t.Errorf("Expected newBranchName to be %q, got %q", expected, m.newBranchName)
	}

	// Test the 'd' key specifically (this was the bug we fixed)
	keyMsg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}}
	newModel, _ = m.Update(keyMsg)
	m = newModel.(model)

	expected = "feature-branchd"
	if m.newBranchName != expected {
		t.Errorf("Expected 'd' to be added to branch name, got %q", m.newBranchName)
	}

	// Test backspace
	keyMsg = tea.KeyMsg{Type: tea.KeyBackspace}
	newModel, _ = m.Update(keyMsg)
	m = newModel.(model)

	expected = "feature-branch"
	if m.newBranchName != expected {
		t.Errorf("Expected backspace to remove last character, got %q", m.newBranchName)
	}

	// Test escape to cancel
	keyMsg = tea.KeyMsg{Type: tea.KeyEsc}
	newModel, _ = m.Update(keyMsg)
	m = newModel.(model)

	if m.creatingBranch {
		t.Error("Expected creatingBranch to be false after pressing escape")
	}

	if m.newBranchName != "" {
		t.Errorf("Expected newBranchName to be empty after escape, got %q", m.newBranchName)
	}

	if m.view != "branches" {
		t.Errorf("Expected to return to branches view after escape, got %q", m.view)
	}
}

func TestModelUpdate_Filtering(t *testing.T) {
	m := initialModel()
	m.view = "branches"
	m.allBranches = []Branch{
		{Name: "main", Type: "local"},
		{Name: "feature-branch", Type: "local"},
		{Name: "bugfix-branch", Type: "local"},
	}
	m.branches = m.allBranches

	// Press '/' to start filtering
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}
	newModel, _ := m.Update(keyMsg)
	m = newModel.(model)

	if !m.filtering {
		t.Error("Expected filtering to be true after pressing '/'")
	}

	// Type filter text
	testChars := []rune{'f', 'e', 'a', 't', 'u', 'r', 'e'}
	for _, char := range testChars {
		keyMsg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{char}}
		newModel, _ = m.Update(keyMsg)
		m = newModel.(model)
	}

	expected := "feature"
	if m.filterText != expected {
		t.Errorf("Expected filterText to be %q, got %q", expected, m.filterText)
	}

	// Test backspace in filter mode
	keyMsg = tea.KeyMsg{Type: tea.KeyBackspace}
	newModel, _ = m.Update(keyMsg)
	m = newModel.(model)

	expected = "featur"
	if m.filterText != expected {
		t.Errorf("Expected filterText after backspace to be %q, got %q", expected, m.filterText)
	}

	// Test escape to cancel filtering
	keyMsg = tea.KeyMsg{Type: tea.KeyEsc}
	newModel, _ = m.Update(keyMsg)
	m = newModel.(model)

	if m.filtering {
		t.Error("Expected filtering to be false after pressing escape")
	}

	if m.filterText != "" {
		t.Errorf("Expected filterText to be empty after escape, got %q", m.filterText)
	}

	if len(m.branches) != len(m.allBranches) {
		t.Errorf("Expected branches to be restored to allBranches after escape")
	}
}

func TestModelUpdate_Navigation(t *testing.T) {
	m := initialModel()
	m.worktrees = []Worktree{
		{Path: "/path1", Branch: "main"},
		{Path: "/path2", Branch: "feature"},
		{Path: "/path3", Branch: "bugfix"},
	}

	// Test down navigation
	keyMsg := tea.KeyMsg{Type: tea.KeyDown}
	newModel, _ := m.Update(keyMsg)
	m = newModel.(model)

	if m.cursor != 1 {
		t.Errorf("Expected cursor to be 1 after down, got %d", m.cursor)
	}

	// Test up navigation
	keyMsg = tea.KeyMsg{Type: tea.KeyUp}
	newModel, _ = m.Update(keyMsg)
	m = newModel.(model)

	if m.cursor != 0 {
		t.Errorf("Expected cursor to be 0 after up, got %d", m.cursor)
	}

	// Test 'j' key (vim-style down)
	keyMsg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}
	newModel, _ = m.Update(keyMsg)
	m = newModel.(model)

	if m.cursor != 1 {
		t.Errorf("Expected cursor to be 1 after 'j', got %d", m.cursor)
	}

	// Test 'k' key (vim-style up)
	keyMsg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}
	newModel, _ = m.Update(keyMsg)
	m = newModel.(model)

	if m.cursor != 0 {
		t.Errorf("Expected cursor to be 0 after 'k', got %d", m.cursor)
	}
}

func TestModelUpdate_DeleteWorktree(t *testing.T) {
	m := initialModel()
	m.view = "worktrees"
	m.worktrees = []Worktree{
		{Path: "/path1", Branch: "main"},
		{Path: "/path2", Branch: "feature"},
	}

	// Press 'd' to delete worktree
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}}
	_, cmd := m.Update(keyMsg)

	// Should return a command to delete the worktree
	if cmd == nil {
		t.Error("Expected delete command when pressing 'd' in worktrees view")
	}

	// Test that 'd' doesn't work when not in worktrees view
	m.view = "branches"
	_, cmd = m.Update(keyMsg)
	if cmd != nil {
		t.Error("Expected no command when pressing 'd' in branches view")
	}

	// Test that 'd' doesn't work when creating branch
	m.view = "worktrees"
	m.creatingBranch = true
	_, cmd = m.Update(keyMsg)
	if cmd != nil {
		t.Error("Expected no command when pressing 'd' while creating branch")
	}
}

func TestFilterBranches(t *testing.T) {
	m := initialModel()
	m.allBranches = []Branch{
		{Name: "main", Type: "local"},
		{Name: "feature-branch", Type: "local"},
		{Name: "bugfix-branch", Type: "local"},
		{Name: "release-v1.0", Type: "local"},
	}
	m.filterText = "feature"

	m.filterBranches()

	if len(m.branches) != 1 {
		t.Errorf("Expected 1 filtered branch, got %d", len(m.branches))
	}

	if m.branches[0].Name != "feature-branch" {
		t.Errorf("Expected filtered branch to be 'feature-branch', got %q", m.branches[0].Name)
	}

	// Test case-insensitive filtering
	m.filterText = "FEATURE"
	m.filterBranches()

	if len(m.branches) != 1 {
		t.Errorf("Expected 1 filtered branch with case-insensitive search, got %d", len(m.branches))
	}

	// Test filtering with multiple matches
	m.filterText = "branch"
	m.filterBranches()

	if len(m.branches) != 2 {
		t.Errorf("Expected 2 filtered branches, got %d", len(m.branches))
	}

	// Test empty filter
	m.filterText = ""
	m.filterBranches()

	if len(m.branches) != len(m.allBranches) {
		t.Errorf("Expected all branches with empty filter, got %d", len(m.branches))
	}
}

func TestAdjustScrollOffset(t *testing.T) {
	m := initialModel()
	m.worktrees = make([]Worktree, 20) // Create 20 worktrees for testing

	// Test scroll down
	m.cursor = 15
	m.adjustScrollOffset()

	// The exact scroll behavior depends on the terminal height,
	// but we can test that scrollOffset is adjusted
	if m.scrollOffset < 0 {
		t.Error("scrollOffset should not be negative")
	}

	// Test scroll up
	m.cursor = 0
	m.adjustScrollOffset()

	if m.scrollOffset != 0 {
		t.Errorf("Expected scrollOffset to be 0 when cursor is 0, got %d", m.scrollOffset)
	}
}

func TestViewString(t *testing.T) {
	m := initialModel()

	// Test empty model
	view := m.View()
	if view == "" {
		t.Error("View should not be empty")
	}

	// Test with some data
	m.worktrees = []Worktree{
		{Path: "/path1", Branch: "main"},
		{Path: "/path2", Branch: "feature"},
	}
	m.branches = []Branch{
		{Name: "main", Type: "local"},
		{Name: "feature", Type: "local"},
	}

	view = m.View()
	if view == "" {
		t.Error("View should not be empty with data")
	}

	// Should contain worktree information
	if !strings.Contains(view, "main") {
		t.Error("View should contain branch names")
	}
}