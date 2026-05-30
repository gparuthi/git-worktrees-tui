package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestReuseWorktreeBranch exercises the alt+n flow end-to-end against a real
// throwaway repo: it sets up an origin with a master branch, adds a worktree on
// a feature branch, then reuses that worktree onto a fresh branch off
// origin/master and checks the result.
func TestReuseWorktreeBranch(t *testing.T) {
	tmp := t.TempDir()
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run(tmp, "init", "--bare", "-b", "master", "remote.git")
	remote := filepath.Join(tmp, "remote.git")
	run(tmp, "clone", remote, "work")
	work := filepath.Join(tmp, "work")
	run(work, "config", "user.email", "t@example.com")
	run(work, "config", "user.name", "Tester")

	if err := os.WriteFile(filepath.Join(work, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(work, "add", ".")
	run(work, "commit", "-m", "init")
	run(work, "push", "origin", "master")

	// Worktree on a feature branch, simulating "PR merged, want to reuse".
	wtPath := filepath.Join(tmp, "wt-feature")
	run(work, "worktree", "add", "-b", "feature", wtPath)

	if err := reuseWorktreeBranch(wtPath, "gp-newwork"); err != nil {
		t.Fatalf("reuseWorktreeBranch: %v", err)
	}

	// The worktree should now be on the new branch.
	out, err := exec.Command("git", "-C", wtPath, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(out)); got != "gp-newwork" {
		t.Fatalf("worktree branch = %q, want gp-newwork", got)
	}

	// The new branch should sit at origin/master's commit and have no upstream
	// (created with --no-track).
	head, _ := exec.Command("git", "-C", wtPath, "rev-parse", "HEAD").Output()
	master, _ := exec.Command("git", "-C", wtPath, "rev-parse", "origin/master").Output()
	if strings.TrimSpace(string(head)) != strings.TrimSpace(string(master)) {
		t.Fatalf("new branch HEAD %s != origin/master %s", head, master)
	}
	if _, err := exec.Command("git", "-C", wtPath, "rev-parse", "--abbrev-ref", "gp-newwork@{upstream}").Output(); err == nil {
		t.Errorf("expected no upstream for --no-track branch, but one was set")
	}
}
