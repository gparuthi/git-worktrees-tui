package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type prCreatingMsg struct{ ref string }
type prCreatedMsg struct {
	number      int
	worktreePath string
	headRefName string
}

type PRInfo struct {
	Number      int    `json:"number"`
	HeadRefName string `json:"headRefName"`
	Title       string `json:"title"`
	URL         string `json:"url"`
}

// parsedPRRef holds what we extracted from the user's input. Owner+Repo are
// only set when the input was a github URL.
type parsedPRRef struct {
	Ref   string // argument for `gh pr view` (number or branch name)
	Owner string
	Repo  string
}

// resolvePRRef accepts a PR number, "#123", a github PR URL, or a head branch
// name and normalizes it to an argument suitable for `gh pr view`.
var prURLRe = regexp.MustCompile(`github\.com[/:]([^/]+)/([^/]+?)(?:\.git)?/pull/(\d+)`)

func resolvePRRef(input string) (parsedPRRef, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return parsedPRRef{}, fmt.Errorf("empty PR ref")
	}
	// Strip URL fragments/query-strings users sometimes paste.
	if i := strings.IndexAny(input, "#?"); i >= 0 && !strings.HasPrefix(input, "#") {
		input = input[:i]
	}
	if strings.HasPrefix(input, "#") {
		input = strings.TrimPrefix(input, "#")
	}
	if m := prURLRe.FindStringSubmatch(input); len(m) == 4 {
		return parsedPRRef{Ref: m[3], Owner: m[1], Repo: m[2]}, nil
	}
	// Either a bare number or a branch name — gh handles both.
	return parsedPRRef{Ref: input}, nil
}

// resolveRepoRoot picks the working directory to operate on for a PR. When the
// input was a URL, prefer the configured local checkout for that owner/repo;
// otherwise fall back to whatever git considers the main repo for cwd.
func resolveRepoRoot(parsed parsedPRRef) (string, error) {
	if parsed.Owner != "" && parsed.Repo != "" {
		cfg, err := loadConfig()
		if err != nil {
			log.Printf("[PR] config load failed: %v (falling back to cwd)", err)
		} else if path := cfg.findRepoPath(parsed.Owner, parsed.Repo); path != "" {
			if _, err := os.Stat(path); err != nil {
				return "", fmt.Errorf("config maps %s/%s to %s but path does not exist", parsed.Owner, parsed.Repo, path)
			}
			log.Printf("[PR] using configured path for %s/%s: %s", parsed.Owner, parsed.Repo, path)
			return path, nil
		}
	}
	root, err := getMainRepoRoot()
	if err != nil {
		return "", fmt.Errorf("not inside a git repo and no config match for this PR: %w", err)
	}
	if parsed.Owner != "" && parsed.Repo != "" {
		if err := verifyOriginMatchesAt(root, parsed.Owner, parsed.Repo); err != nil {
			return "", err
		}
	}
	return root, nil
}

func verifyOriginMatchesAt(repoRoot, owner, repo string) error {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return nil // best-effort check; let gh handle resolution
	}
	url := strings.TrimSpace(string(out))
	expected := strings.ToLower(owner + "/" + repo)
	if !strings.Contains(strings.ToLower(url), expected) {
		return fmt.Errorf("PR URL points to %s/%s but cwd's origin is %s — add a config entry or cd into the right repo", owner, repo, url)
	}
	return nil
}

func getPRInfo(ref, repoRoot string) (*PRInfo, error) {
	cmd := exec.Command("gh", "pr", "view", ref, "--json", "number,headRefName,title,url")
	if repoRoot != "" {
		cmd.Dir = repoRoot
	}
	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = strings.TrimSpace(string(exitErr.Stderr))
		}
		if stderr != "" {
			return nil, fmt.Errorf("gh pr view %s: %s", ref, stderr)
		}
		return nil, fmt.Errorf("gh pr view %s: %w", ref, err)
	}
	var info PRInfo
	if err := json.Unmarshal(out, &info); err != nil {
		return nil, fmt.Errorf("parsing gh output: %w", err)
	}
	if info.Number == 0 {
		return nil, fmt.Errorf("could not resolve PR from %q", ref)
	}
	return &info, nil
}

// createPRWorktree fetches refs/pull/<n>/head into a local pr-<n> branch and
// creates a worktree at <repoName>-pr-<n>. Works for fork PRs too because
// GitHub exposes the PR head under refs/pull/<n>/head on the base repo.
//
// When the input is a github URL whose owner/repo matches a configured local
// checkout (~/.wtree/config.json), all git operations run in that checkout,
// so wtree can be invoked from any directory.
func createPRWorktree(ref string) (*PRInfo, string, error) {
	parsed, err := resolvePRRef(ref)
	if err != nil {
		return nil, "", err
	}
	repoRoot, err := resolveRepoRoot(parsed)
	if err != nil {
		return nil, "", err
	}
	info, err := getPRInfo(parsed.Ref, repoRoot)
	if err != nil {
		return nil, "", err
	}

	repoName := filepath.Base(repoRoot)
	localBranch := fmt.Sprintf("pr-%d", info.Number)
	parentDir := filepath.Dir(repoRoot)
	worktreePath := filepath.Join(parentDir, repoName+"-"+localBranch)

	log.Printf("[PR] fetching pr=%d head=%s into local=%s (repoRoot=%s)", info.Number, info.HeadRefName, localBranch, repoRoot)

	// If the branch is already checked out somewhere, skip the fetch (git will
	// refuse to update a checked-out branch). Otherwise force-update so the
	// local branch tracks the latest PR head.
	branchCheckedOut, err := isBranchCheckedOutAt(localBranch, repoRoot)
	if err != nil {
		return nil, "", err
	}
	if !branchCheckedOut {
		fetchSpec := fmt.Sprintf("+refs/pull/%d/head:refs/heads/%s", info.Number, localBranch)
		fetchCmd := exec.Command("git", "fetch", "origin", fetchSpec)
		fetchCmd.Dir = repoRoot
		if out, err := fetchCmd.CombinedOutput(); err != nil {
			return nil, "", fmt.Errorf("git fetch: %s", strings.TrimSpace(string(out)))
		}
	} else {
		log.Printf("[PR] branch %s already checked out, skipping fetch", localBranch)
	}

	// If a worktree at this path already exists, return it as-is.
	if existing, _ := getWorktreesAt(repoRoot); existing != nil {
		for _, w := range existing {
			if w.Path == worktreePath {
				log.Printf("[PR] worktree already exists at %s", worktreePath)
				return info, worktreePath, nil
			}
		}
	}

	addCmd := exec.Command("git", "worktree", "add", worktreePath, localBranch)
	addCmd.Dir = repoRoot
	if out, err := addCmd.CombinedOutput(); err != nil {
		return nil, "", fmt.Errorf("git worktree add: %s", strings.TrimSpace(string(out)))
	}
	log.Printf("[PR] worktree created at %s", worktreePath)
	return info, worktreePath, nil
}

func isBranchCheckedOutAt(branch, repoRoot string) (bool, error) {
	worktrees, err := getWorktreesAt(repoRoot)
	if err != nil {
		return false, err
	}
	for _, w := range worktrees {
		if w.Branch == branch {
			return true, nil
		}
	}
	return false, nil
}

func createPRWorktreeCmd(ref string) tea.Cmd {
	return func() tea.Msg {
		info, path, err := createPRWorktree(ref)
		if err != nil {
			return err
		}
		return prCreatedMsg{number: info.Number, worktreePath: path, headRefName: info.HeadRefName}
	}
}

func startPRCreateCmd(ref string) tea.Cmd {
	return func() tea.Msg {
		return prCreatingMsg{ref: ref}
	}
}
