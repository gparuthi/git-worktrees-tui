package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestDeleteLogAndUndo runs a real delete (which logs) then undo (which
// recreates the worktree), against a throwaway repo. HOME is redirected so the
// deletions log lands in a temp dir, not the user's real ~/.wtree.
func TestDeleteLogAndUndo(t *testing.T) {
	tmp := t.TempDir()
	if r, err := filepath.EvalSymlinks(tmp); err == nil {
		tmp = r // macOS /tmp -> /private/tmp; git resolves it, so match it here
	}
	t.Setenv("HOME", tmp)

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
	if err := os.WriteFile(filepath.Join(work, "a.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(work, "add", ".")
	run(work, "commit", "-m", "init")

	// A worktree on a feature branch.
	wtPath := filepath.Join(tmp, "work-feature")
	run(work, "worktree", "add", "-b", "feature", wtPath)
	headOut, _ := exec.Command("git", "-C", wtPath, "rev-parse", "HEAD").Output()
	head := strings.TrimSpace(string(headOut))

	// deleteWorktree resolves the main repo from cwd, so run from there.
	t.Chdir(work)
	if err := deleteWorktree(Worktree{Path: wtPath, Branch: "feature", Head: head}); err != nil {
		t.Fatalf("deleteWorktree: %v", err)
	}
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Fatalf("worktree dir should be gone after delete")
	}

	// The deletion should have been logged.
	recs, err := readDeletions()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 deletion record, got %d", len(recs))
	}
	if recs[0].Path != wtPath || recs[0].Branch != "feature" || recs[0].RepoRoot != work {
		t.Fatalf("unexpected record: %+v", recs[0])
	}

	// Undo should recreate the worktree and trim the log.
	msg := undoLastDeletionCmd()()
	if _, ok := msg.(undoDoneMsg); !ok {
		t.Fatalf("expected undoDoneMsg, got %T (%v)", msg, msg)
	}
	if _, err := os.Stat(wtPath); err != nil {
		t.Fatalf("worktree should be restored: %v", err)
	}
	out, _ := exec.Command("git", "-C", wtPath, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if got := strings.TrimSpace(string(out)); got != "feature" {
		t.Fatalf("restored branch = %q, want feature", got)
	}
	if recs, _ := readDeletions(); len(recs) != 0 {
		t.Fatalf("log should be empty after undo, got %d", len(recs))
	}

	// Nothing left to undo.
	if _, ok := undoLastDeletionCmd()().(undoNothingMsg); !ok {
		t.Errorf("expected undoNothingMsg when log is empty")
	}
}

// TestDeleteQueue exercises the queue state machine without touching git: the
// processDeleteCmd commands it returns are not executed here.
func TestDeleteQueue(t *testing.T) {
	m := initialModel()
	m.view = "worktrees"
	m.worktrees = []Worktree{{Path: "/a", Branch: "a"}, {Path: "/b", Branch: "b"}, {Path: "/c", Branch: "c"}}

	press := func(m model, r rune) model {
		nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		return nm.(model)
	}

	// 'd' on /a: marked + active + cursor advances.
	m = press(m, 'd')
	if !m.deleting["/a"] || !m.deleteActive {
		t.Fatalf("after first 'd': deleting[/a]=%v active=%v", m.deleting["/a"], m.deleteActive)
	}
	if m.cursor != 1 {
		t.Fatalf("cursor should advance to 1, got %d", m.cursor)
	}

	// 'd' on /b: queued (a still active), not started.
	m = press(m, 'd')
	if len(m.deleteQueue) != 1 || m.deleteQueue[0].Path != "/b" {
		t.Fatalf("/b should be queued: %+v", m.deleteQueue)
	}

	// /a finishes: cleared, removed from list, /b becomes active.
	nm, _ := m.Update(deleteDoneMsg{path: "/a"})
	m = nm.(model)
	if m.deleting["/a"] {
		t.Error("/a should be cleared")
	}
	for _, wt := range m.worktrees {
		if wt.Path == "/a" {
			t.Error("/a should be removed from list")
		}
	}
	if !m.deleteActive || len(m.deleteQueue) != 0 {
		t.Fatalf("/b should be active and dequeued: active=%v queue=%+v", m.deleteActive, m.deleteQueue)
	}

	// /b finishes: no more work.
	nm, _ = m.Update(deleteDoneMsg{path: "/b"})
	m = nm.(model)
	if m.deleteActive {
		t.Error("no deletions left; active should be false")
	}
}
