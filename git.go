package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
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
	
	repoRoot, err := getRepoRoot()
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
	return cmd.Run()
}

func getRepoName() (string, error) {
	repoRoot, err := getRepoRoot()
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

func createNewBranchWorktree(branchName string) error {
	repoName, err := getRepoName()
	if err != nil {
		return err
	}
	
	repoRoot, err := getRepoRoot()
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

func isGitRepository() bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	return cmd.Run() == nil
}