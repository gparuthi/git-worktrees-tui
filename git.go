package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type worktreesMsg []Worktree
type branchesMsg []Branch
type newBranchCreatedMsg struct{}
type newBranchCreatingMsg struct{ branchName string }
type worktreeDeletedMsg struct{}
type deletingWorktreeMsg struct{ path string }
type worktreeCreatedMsg struct {
	branch string
}

func getWorktreesCmd() tea.Cmd {
	return func() tea.Msg {
		worktrees, err := getWorktrees()
		if err != nil {
			return worktreesMsg{}
		}
		return worktreesMsg(worktrees)
	}
}

func getBranchesCmd() tea.Cmd {
	return func() tea.Msg {
		branches, err := getBranches()
		if err != nil {
			return branchesMsg{}
		}
		return branchesMsg(branches)
	}
}

func createWorktreeCmd(branch Branch) tea.Cmd {
	return func() tea.Msg {
		err := createWorktree(branch)
		if err != nil {
			return err
		}
		return worktreeCreatedMsg{branch: branch.Name}
	}
}

func deleteWorktreeCmd(worktree Worktree) tea.Cmd {
	return func() tea.Msg {
		// First send a message that we're starting to delete
		return deletingWorktreeMsg{path: worktree.Path}
	}
}

func performDeleteWorktreeCmd(worktree Worktree) tea.Cmd {
	return func() tea.Msg {
		err := deleteWorktree(worktree)
		if err != nil {
			return err
		}
		return worktreeDeletedMsg{}
	}
}

func openWorktreeCmd(worktree Worktree) tea.Cmd {
	return func() tea.Msg {
		err := openWorktree(worktree)
		if err != nil {
			return err
		}
		return nil
	}
}

func openTerminalCmd(worktree Worktree) tea.Cmd {
	return func() tea.Msg {
		err := openTerminal(worktree)
		if err != nil {
			return err
		}
		return tea.Quit()
	}
}

func openTerminalWithClaudeCmd(worktree Worktree) tea.Cmd {
	return func() tea.Msg {
		err := openTerminalWithClaude(worktree)
		if err != nil {
			return err
		}
		return tea.Quit()
	}
}

func openTerminalWithClaudeRCmd(worktree Worktree) tea.Cmd {
	return func() tea.Msg {
		err := openTerminalWithClaudeR(worktree)
		if err != nil {
			return err
		}
		return tea.Quit()
	}
}

func createNewBranchWorktreeCmd(branchName string) tea.Cmd {
	return func() tea.Msg {
		return newBranchCreatingMsg{branchName: branchName}
	}
}

func performCreateNewBranchWorktreeCmd(branchName string) tea.Cmd {
	return func() tea.Msg {
		err := createNewBranchWorktree(branchName)
		if err != nil {
			return err
		}
		return newBranchCreatedMsg{}
	}
}

func getWorktrees() ([]Worktree, error) {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var worktrees []Worktree
	lines := strings.Split(string(output), "\n")
	var currentWorktree Worktree

	for _, line := range lines {
		if strings.HasPrefix(line, "worktree ") {
			if currentWorktree.Path != "" {
				worktrees = append(worktrees, currentWorktree)
			}
			currentWorktree = Worktree{
				Path: strings.TrimPrefix(line, "worktree "),
			}
		} else if strings.HasPrefix(line, "branch ") {
			currentWorktree.Branch = strings.TrimPrefix(line, "branch refs/heads/")
		} else if strings.HasPrefix(line, "HEAD ") {
			currentWorktree.Head = strings.TrimPrefix(line, "HEAD ")
		}
	}

	if currentWorktree.Path != "" {
		worktrees = append(worktrees, currentWorktree)
	}

	return worktrees, nil
}

func getBranches() ([]Branch, error) {
	localBranches, err := getLocalBranches()
	if err != nil {
		return nil, err
	}

	remoteBranches, err := getRemoteBranches()
	if err != nil {
		return nil, err
	}

	var allBranches []Branch
	allBranches = append(allBranches, localBranches...)
	allBranches = append(allBranches, remoteBranches...)

	sort.Slice(allBranches, func(i, j int) bool {
		if allBranches[i].Type != allBranches[j].Type {
			return allBranches[i].Type == "local"
		}
		return allBranches[i].LastCommit > allBranches[j].LastCommit
	})

	return allBranches, nil
}

func getLocalBranches() ([]Branch, error) {
	cmd := exec.Command("git", "for-each-ref", "--format=%(refname:short)|%(committerdate:iso8601)", "refs/heads/")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var branches []Branch
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) == 2 {
			lastCommit, _ := time.Parse("2006-01-02 15:04:05 -0700", parts[1])
			branches = append(branches, Branch{
				Name:     parts[0],
				Type:     "local",
				LastCommit: lastCommit.Format("2006-01-02 15:04:05"),
			})
		}
	}

	return branches, nil
}

func getRemoteBranches() ([]Branch, error) {
	cmd := exec.Command("git", "for-each-ref", "--format=%(refname:short)|%(committerdate:iso8601)", "refs/remotes/")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var branches []Branch
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) == 2 {
			lastCommit, _ := time.Parse("2006-01-02 15:04:05 -0700", parts[1])
			branches = append(branches, Branch{
				Name:     parts[0],
				Type:     "remote",
				LastCommit: lastCommit.Format("2006-01-02 15:04:05"),
			})
		}
	}

	return branches, nil
}

func createWorktree(branch Branch) error {
	repoName, err := getRepoName()
	if err != nil {
		return err
	}
	
	repoRoot, err := getMainRepoRoot()
	if err != nil {
		return err
	}
	
	parentDir := filepath.Dir(repoRoot)
	branchName := strings.ReplaceAll(branch.Name, "/", "-")
	worktreePath := filepath.Join(parentDir, repoName + "-" + branchName)
	
	var cmd *exec.Cmd
	if branch.Type == "local" {
		cmd = exec.Command("git", "worktree", "add", worktreePath, branch.Name)
	} else {
		localBranchName := strings.TrimPrefix(branch.Name, "origin/")
		cmd = exec.Command("git", "worktree", "add", "-b", localBranchName, worktreePath, branch.Name)
	}
	
	return cmd.Run()
}

func deleteWorktree(worktree Worktree) error {
	cmd := exec.Command("git", "worktree", "remove", worktree.Path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Try again with --force if the initial remove failed
		cmdForce := exec.Command("git", "worktree", "remove", "--force", worktree.Path)
		forceOutput, forceErr := cmdForce.CombinedOutput()
		if forceErr != nil {
			msg := strings.TrimSpace(string(forceOutput))
			if msg == "" {
				msg = strings.TrimSpace(string(output))
			}
			if msg != "" {
				return fmt.Errorf("%s", msg)
			}
			return forceErr
		}
	}
	return nil
}

func getRepoName() (string, error) {
	repoRoot, err := getMainRepoRoot()
	if err != nil {
		return "", err
	}
	return filepath.Base(repoRoot), nil
}

func getRepoRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	
	return strings.TrimSpace(string(output)), nil
}

func getMainRepoRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--git-common-dir")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	commonDir := strings.TrimSpace(string(output))
	if commonDir == "" {
		return "", fmt.Errorf("could not determine git common dir")
	}

	if !filepath.IsAbs(commonDir) {
		repoRoot, err := getRepoRoot()
		if err != nil {
			return "", err
		}
		commonDir = filepath.Join(repoRoot, commonDir)
	}

	return filepath.Dir(commonDir), nil
}

func createNewBranchWorktree(branchName string) error {
	repoName, err := getRepoName()
	if err != nil {
		return err
	}
	
	repoRoot, err := getMainRepoRoot()
	if err != nil {
		return err
	}
	
	// Find the main branch from origin (origin/main or origin/master)
	mainBranch, err := getOriginMainBranch()
	if err != nil {
		return err
	}
	
	parentDir := filepath.Dir(repoRoot)
	sanitizedBranchName := strings.ReplaceAll(branchName, "/", "-")
	worktreePath := filepath.Join(parentDir, repoName + "-" + sanitizedBranchName)
	
	cmd := exec.Command("git", "worktree", "add", "--no-track", "-b", branchName, worktreePath, mainBranch)
	return cmd.Run()
}

func getOriginMainBranch() (string, error) {
	// Try origin/main first
	cmd := exec.Command("git", "rev-parse", "--verify", "origin/main")
	if err := cmd.Run(); err == nil {
		return "origin/main", nil
	}
	
	// Fall back to origin/master
	cmd = exec.Command("git", "rev-parse", "--verify", "origin/master")
	if err := cmd.Run(); err == nil {
		return "origin/master", nil
	}
	
	// If neither exists, try to find the default remote branch
	cmd = exec.Command("git", "symbolic-ref", "refs/remotes/origin/HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("could not find origin main branch (tried origin/main, origin/master)")
	}
	
	// Parse refs/remotes/origin/HEAD -> refs/remotes/origin/main
	ref := strings.TrimSpace(string(output))
	if strings.HasPrefix(ref, "refs/remotes/") {
		return strings.TrimPrefix(ref, "refs/remotes/"), nil
	}
	
	return "", fmt.Errorf("could not parse origin main branch reference")
}

func openWorktree(worktree Worktree) error {
	cmd := exec.Command("cursor", worktree.Path)
	return cmd.Run()
}

func openTerminal(worktree Worktree) error {
	// Change to the worktree directory
	if err := os.Chdir(worktree.Path); err != nil {
		return fmt.Errorf("failed to change directory: %w", err)
	}

	// Find zsh binary
	zshPath, err := exec.LookPath("zsh")
	if err != nil {
		return fmt.Errorf("zsh not found: %w", err)
	}

	// Replace current process with zsh
	// This will close wtree and start zsh in the worktree directory
	return syscall.Exec(zshPath, []string{"zsh"}, os.Environ())
}

func openTerminalWithClaude(worktree Worktree) error {
	// Change to the worktree directory
	if err := os.Chdir(worktree.Path); err != nil {
		return fmt.Errorf("failed to change directory: %w", err)
	}

	// Find claude binary
	claudePath, err := exec.LookPath("claude")
	if err != nil {
		return fmt.Errorf("claude not found: %w", err)
	}

	// Replace current process with claude
	return syscall.Exec(claudePath, []string{"claude"}, os.Environ())
}

func openTerminalWithClaudeR(worktree Worktree) error {
	// Change to the worktree directory
	if err := os.Chdir(worktree.Path); err != nil {
		return fmt.Errorf("failed to change directory: %w", err)
	}

	// Find claude binary
	claudePath, err := exec.LookPath("claude")
	if err != nil {
		return fmt.Errorf("claude not found: %w", err)
	}

	// Replace current process with claude -r
	return syscall.Exec(claudePath, []string{"claude", "-r"}, os.Environ())
}

func isGitRepository() bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	return cmd.Run() == nil
}