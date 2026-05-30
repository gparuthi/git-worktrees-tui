package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type worktreesMsg []Worktree
type branchesMsg []Branch
type newBranchCreatedMsg struct{}
type newBranchCreatingMsg struct{ branchName string }

// deleteDoneMsg reports the result of one queued deletion (err is nil on success).
type deleteDoneMsg struct {
	path string
	err  error
}
type worktreeCreatedMsg struct {
	branch string
}
type dirtyWorktreeErr struct {
	worktree Worktree
}

func (e dirtyWorktreeErr) Error() string {
	return fmt.Sprintf("worktree %s has modified or untracked files", e.worktree.Path)
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

// processDeleteCmd performs one deletion in the background and reports the
// result. Deletions are run one at a time (serialized by the model) because
// `git worktree remove`/`prune` mutate shared repo state.
func processDeleteCmd(worktree Worktree) tea.Cmd {
	return func() tea.Msg {
		return deleteDoneMsg{path: worktree.Path, err: deleteWorktree(worktree)}
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
	return getWorktreesAt("")
}

func getWorktreesAt(repoRoot string) ([]Worktree, error) {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	if repoRoot != "" {
		cmd.Dir = repoRoot
	}
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

	mtimes := make(map[string]time.Time, len(worktrees))
	for _, wt := range worktrees {
		if info, err := os.Stat(wt.Path); err == nil {
			mtimes[wt.Path] = info.ModTime()
		}
	}
	sort.SliceStable(worktrees, func(i, j int) bool {
		return mtimes[worktrees[i].Path].After(mtimes[worktrees[j].Path])
	})

	// NOTE: local git enrichment (last commit / dirty / ahead) is intentionally
	// NOT done here — `git status` is slow on large monorepos and would block
	// the list from rendering. The TUI enriches asynchronously via
	// enrichWorktreesCmd; the non-interactive path calls enrichWorktreesLocal.
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
	parentDir, repoName, err := preferredWorktreeParent()
	if err != nil {
		return err
	}

	branchName := strings.ReplaceAll(branch.Name, "/", "-")
	worktreePath := filepath.Join(parentDir, repoName+"-"+branchName)
	
	log.Printf("[CREATE] creating worktree path=%s branch=%s type=%s", worktreePath, branch.Name, branch.Type)

	var cmd *exec.Cmd
	if branch.Type == "local" {
		cmd = exec.Command("git", "worktree", "add", worktreePath, branch.Name)
	} else {
		localBranchName := strings.TrimPrefix(branch.Name, "origin/")
		cmd = exec.Command("git", "worktree", "add", "-b", localBranchName, worktreePath, branch.Name)
	}

	if err := cmd.Run(); err != nil {
		log.Printf("[CREATE] failed: %v", err)
		return err
	}
	log.Printf("[CREATE] succeeded")
	return nil
}

func deleteWorktree(worktree Worktree) error {
	mainRoot, err := getMainRepoRoot()
	if err != nil {
		log.Printf("[DELETE] failed to get main repo root: %v", err)
		return fmt.Errorf("could not find main repo root: %w", err)
	}

	log.Printf("[DELETE] removing worktree path=%s branch=%s mainRoot=%s", worktree.Path, worktree.Branch, mainRoot)

	// Prune stale worktrees first to clean up any dangling references
	pruneCmd := exec.Command("git", "worktree", "prune")
	pruneCmd.Dir = mainRoot
	if pruneOut, pruneErr := pruneCmd.CombinedOutput(); pruneErr != nil {
		log.Printf("[DELETE] prune warning: %v output=%s", pruneErr, strings.TrimSpace(string(pruneOut)))
	}

	cmd := exec.Command("git", "worktree", "remove", worktree.Path)
	cmd.Dir = mainRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		outStr := strings.TrimSpace(string(output))
		log.Printf("[DELETE] initial remove failed: %v output=%s", err, outStr)
		// If dirty worktree, ask user to confirm force delete
		if strings.Contains(outStr, "modified or untracked files") {
			return dirtyWorktreeErr{worktree: worktree}
		}
		return fmt.Errorf("%s", outStr)
	}
	log.Printf("[DELETE] remove succeeded")

	// Record enough to undo (the branch + commits survive `git worktree remove`).
	if logErr := logDeletion(DeletionRecord{
		Path:     worktree.Path,
		Branch:   worktree.Branch,
		Head:     worktree.Head,
		RepoRoot: mainRoot,
	}); logErr != nil {
		log.Printf("[DELETE] failed to write deletion log: %v", logErr)
	}
	return nil
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
	parentDir, repoName, err := preferredWorktreeParent()
	if err != nil {
		return err
	}

	// Find the main branch from origin (origin/main or origin/master)
	mainBranch, err := getOriginMainBranch()
	if err != nil {
		return err
	}

	sanitizedBranchName := strings.ReplaceAll(branchName, "/", "-")
	worktreePath := filepath.Join(parentDir, repoName+"-"+sanitizedBranchName)
	
	log.Printf("[CREATE] new branch worktree path=%s branch=%s base=%s", worktreePath, branchName, mainBranch)
	cmd := exec.Command("git", "worktree", "add", "--no-track", "-b", branchName, worktreePath, mainBranch)
	if err := cmd.Run(); err != nil {
		log.Printf("[CREATE] new branch failed: %v", err)
		return err
	}
	log.Printf("[CREATE] new branch succeeded")
	return nil
}

type reuseBranchDoneMsg struct {
	path   string
	branch string
}

// reuseBranchCmd switches an existing worktree onto a brand-new branch off
// origin/master, in place — for reusing a worktree after its PR merged.
func reuseBranchCmd(path, branch string) tea.Cmd {
	return func() tea.Msg {
		if err := reuseWorktreeBranch(path, branch); err != nil {
			return err
		}
		return reuseBranchDoneMsg{path: path, branch: branch}
	}
}

func reuseWorktreeBranch(path, branch string) error {
	base, err := originMainBranchAt(path)
	if err != nil {
		return err
	}
	log.Printf("[REUSE] worktree=%s new branch=%s base=%s", path, branch, base)
	cmd := exec.Command("git", "switch", "--no-track", "-c", branch, base)
	cmd.Dir = path
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git switch: %s", strings.TrimSpace(string(out)))
	}
	log.Printf("[REUSE] succeeded")
	return nil
}

// originMainBranchAt is getOriginMainBranch but scoped to a specific worktree dir.
func originMainBranchAt(dir string) (string, error) {
	for _, ref := range []string{"origin/main", "origin/master"} {
		cmd := exec.Command("git", "rev-parse", "--verify", ref)
		cmd.Dir = dir
		if cmd.Run() == nil {
			return ref, nil
		}
	}
	cmd := exec.Command("git", "symbolic-ref", "refs/remotes/origin/HEAD")
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("could not find origin main branch (tried origin/main, origin/master)")
	}
	ref := strings.TrimSpace(string(output))
	if strings.HasPrefix(ref, "refs/remotes/") {
		return strings.TrimPrefix(ref, "refs/remotes/"), nil
	}
	return "", fmt.Errorf("could not parse origin main branch reference")
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

// worktreePathFor returns the worktree path that would be created for a branch
// of the given name. Used to "cd" into a freshly created worktree when exiting
// the TUI.
func worktreePathFor(branchName string) string {
	parentDir, repoName, err := preferredWorktreeParent()
	if err != nil {
		return ""
	}
	sanitized := strings.ReplaceAll(branchName, "/", "-")
	return filepath.Join(parentDir, repoName+"-"+sanitized)
}

// preferredWorktreeParent returns the parent directory where new worktrees
// should be created and the basename to use as the repo prefix.
//
// If origin's owner/repo matches a repos.yaml entry, the parent is derived
// from the configured path — so all worktrees for the repo live under one
// directory regardless of where the main worktree happens to be on disk.
// Otherwise this falls back to the historical behavior of placing worktrees
// next to the main worktree.
func preferredWorktreeParent() (parentDir, repoName string, err error) {
	repoRoot, err := getMainRepoRoot()
	if err != nil {
		return "", "", err
	}
	fallbackParent := filepath.Dir(repoRoot)
	fallbackName := filepath.Base(repoRoot)

	owner, repo, ok := originOwnerRepo(repoRoot)
	if !ok {
		return fallbackParent, fallbackName, nil
	}
	cfg, cfgErr := loadConfig()
	if cfgErr != nil {
		log.Printf("[CONFIG] load failed: %v (using cwd-based parent)", cfgErr)
		return fallbackParent, fallbackName, nil
	}
	configured := cfg.findRepoPath(owner, repo)
	if configured == "" {
		return fallbackParent, fallbackName, nil
	}
	if _, statErr := os.Stat(configured); statErr != nil {
		log.Printf("[CONFIG] %s/%s maps to %s but path missing: %v (using cwd-based parent)", owner, repo, configured, statErr)
		return fallbackParent, fallbackName, nil
	}
	log.Printf("[CONFIG] %s/%s -> configured parent %s", owner, repo, filepath.Dir(configured))
	return filepath.Dir(configured), filepath.Base(configured), nil
}

var originURLRe = regexp.MustCompile(`github\.com[/:]([^/]+)/([^/?#]+?)(?:\.git)?$`)

// originOwnerRepo extracts the owner and repo from the `origin` remote URL at
// repoRoot. Handles both SSH (git@github.com:owner/repo.git) and HTTPS
// (https://github.com/owner/repo[.git]) forms. Returns ok=false if origin is
// missing or doesn't look like a GitHub URL.
func originOwnerRepo(repoRoot string) (owner, repo string, ok bool) {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return "", "", false
	}
	url := strings.TrimSpace(string(out))
	if m := originURLRe.FindStringSubmatch(url); len(m) == 3 {
		return m[1], m[2], true
	}
	return "", "", false
}

func isGitRepository() bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	return cmd.Run() == nil
}