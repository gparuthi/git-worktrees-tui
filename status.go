package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// PRStatus is the subset of `gh pr view` output we care about for deciding
// whether a worktree is safe to delete.
type PRStatus struct {
	Number int    `json:"number"`
	State  string `json:"state"` // OPEN, MERGED, CLOSED
	URL    string `json:"url"`
}

// prStatusesMsg carries the resolved PR for each worktree branch. A nil value
// for a present key means "looked up, no PR" (distinct from "not looked up yet",
// which is an absent key).
type prStatusesMsg map[string]*PRStatus

// worktreesEnrichedMsg carries worktrees with their local git metadata filled
// in. It is merged back into the model by path so the list can render
// immediately and have its badges appear a moment later.
type worktreesEnrichedMsg []Worktree

// enrichWorktreesCmd computes local git metadata off the UI thread. `git status`
// is slow on large monorepos, so this must not block the initial list render.
func enrichWorktreesCmd(worktrees []Worktree) tea.Cmd {
	snapshot := make([]Worktree, len(worktrees))
	copy(snapshot, worktrees)
	return func() tea.Msg {
		enrichWorktreesLocal(snapshot)
		return worktreesEnrichedMsg(snapshot)
	}
}

// gitOutAt runs a git command in dir and returns its trimmed stdout.
func gitOutAt(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// enrichWorktreesLocal fills in fast, local-only git metadata (last commit
// time, dirty state, unpushed commit count) for each worktree. Each worktree's
// probes fork a few git processes, so they run concurrently.
func enrichWorktreesLocal(worktrees []Worktree) {
	const maxConcurrent = 8
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup
	for i := range worktrees {
		wg.Add(1)
		sem <- struct{}{}
		go func(wt *Worktree) {
			defer wg.Done()
			defer func() { <-sem }()
			enrichWorktreeLocal(wt)
		}(&worktrees[i])
	}
	wg.Wait()
}

func enrichWorktreeLocal(wt *Worktree) {
	if out, err := gitOutAt(wt.Path, "log", "-1", "--format=%ct"); err == nil {
		if ts, perr := strconv.ParseInt(out, 10, 64); perr == nil {
			wt.LastCommit = time.Unix(ts, 0)
		}
	}
	if out, err := gitOutAt(wt.Path, "status", "--porcelain"); err == nil {
		wt.Dirty = out != ""
	}
	// Unpushed commits, relative to the branch's upstream when one is set.
	// New --no-track worktrees have no upstream; the command fails and Ahead
	// stays 0, which is what we want.
	if out, err := gitOutAt(wt.Path, "rev-list", "--count", "@{upstream}..HEAD"); err == nil {
		wt.Ahead, _ = strconv.Atoi(out)
	}
}

// getPRStatus resolves the PR (if any) associated with branch. Best-effort:
// returns nil when gh is missing/unauthenticated, there is no PR for the
// branch, or branch is empty (detached HEAD).
func getPRStatus(branch, dir string) *PRStatus {
	if branch == "" {
		return nil
	}
	cmd := exec.Command("gh", "pr", "view", branch, "--json", "number,state,url")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var s PRStatus
	if json.Unmarshal(out, &s) != nil || s.Number == 0 {
		return nil
	}
	return &s
}

// fetchPRStatuses resolves the PR for every distinct worktree branch
// concurrently and returns a branch->status map. Network-bound.
func fetchPRStatuses(worktrees []Worktree) map[string]*PRStatus {
	type job struct{ branch, dir string }
	jobs := make([]job, 0, len(worktrees))
	seen := make(map[string]struct{}, len(worktrees))
	for _, wt := range worktrees {
		if wt.Branch == "" {
			continue
		}
		if _, dup := seen[wt.Branch]; dup {
			continue
		}
		seen[wt.Branch] = struct{}{}
		jobs = append(jobs, job{branch: wt.Branch, dir: wt.Path})
	}

	const maxConcurrent = 6
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup
	var mu sync.Mutex
	results := make(map[string]*PRStatus, len(jobs))
	for _, j := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func(j job) {
			defer wg.Done()
			defer func() { <-sem }()
			st := getPRStatus(j.branch, j.dir)
			mu.Lock()
			results[j.branch] = st
			mu.Unlock()
		}(j)
	}
	wg.Wait()
	return results
}

// getPRStatusesCmd wraps fetchPRStatuses as a tea.Cmd so the lookup runs after
// the worktree list has already rendered, rather than blocking it.
func getPRStatusesCmd(worktrees []Worktree) tea.Cmd {
	return func() tea.Msg {
		return prStatusesMsg(fetchPRStatuses(worktrees))
	}
}

// plainWorktreeMeta is the colorless counterpart to model.worktreeMeta, used by
// the non-interactive `--list-worktrees` output.
func plainWorktreeMeta(wt Worktree, prs map[string]*PRStatus) string {
	var parts []string
	if age := relativeTime(wt.LastCommit); age != "" {
		parts = append(parts, age)
	}
	if wt.Dirty {
		parts = append(parts, "dirty")
	}
	if wt.Ahead > 0 {
		parts = append(parts, fmt.Sprintf("ahead %d", wt.Ahead))
	}
	if st := prs[wt.Branch]; st != nil {
		parts = append(parts, fmt.Sprintf("PR #%d %s", st.Number, strings.ToLower(st.State)))
	}
	return strings.Join(parts, ", ")
}

// relativeTime renders a compact age like "3d", "2h", "now". Empty for zero.
func relativeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dw", int(d.Hours()/(24*7)))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo", int(d.Hours()/(24*30)))
	default:
		return fmt.Sprintf("%dy", int(d.Hours()/(24*365)))
	}
}
