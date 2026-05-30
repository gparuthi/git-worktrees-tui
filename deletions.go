package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// DeletionRecord is one line of the deletions log — enough to recreate the
// worktree. `git worktree remove` leaves the branch and its commits intact, so
// the path + branch (or head) is all we need to undo.
type DeletionRecord struct {
	Time     time.Time `json:"time"`
	Path     string    `json:"path"`
	Branch   string    `json:"branch"`
	Head     string    `json:"head"`
	RepoRoot string    `json:"repoRoot"`
}

type undoDoneMsg struct{ path, branch string }
type undoNothingMsg struct{}

func deletionsLogPath() string {
	return filepath.Join(os.Getenv("HOME"), ".wtree", "deletions.jsonl")
}

// logDeletion appends a record to the deletions log (JSONL).
func logDeletion(rec DeletionRecord) error {
	if rec.Time.IsZero() {
		rec.Time = time.Now()
	}
	path := deletionsLogPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	return err
}

func readDeletions() ([]DeletionRecord, error) {
	f, err := os.Open(deletionsLogPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var recs []DeletionRecord
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var r DeletionRecord
		if json.Unmarshal([]byte(line), &r) == nil {
			recs = append(recs, r)
		}
	}
	return recs, sc.Err()
}

func writeDeletions(recs []DeletionRecord) error {
	path := deletionsLogPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var sb strings.Builder
	for _, r := range recs {
		b, err := json.Marshal(r)
		if err != nil {
			return err
		}
		sb.Write(b)
		sb.WriteByte('\n')
	}
	return os.WriteFile(path, []byte(sb.String()), 0o644)
}

// ensureDeletionsLog guarantees the file exists so `open` doesn't error on a
// fresh setup, and returns its path.
func ensureDeletionsLog() string {
	path := deletionsLogPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		_ = os.WriteFile(path, []byte{}, 0o644)
	}
	return path
}

// undoLastDeletionCmd recreates the most recently deleted worktree and, on
// success, drops it from the log.
func undoLastDeletionCmd() tea.Cmd {
	return func() tea.Msg {
		recs, err := readDeletions()
		if err != nil {
			return err
		}
		if len(recs) == 0 {
			return undoNothingMsg{}
		}
		rec := recs[len(recs)-1]
		if err := undoDeletion(rec); err != nil {
			return err
		}
		if err := writeDeletions(recs[:len(recs)-1]); err != nil {
			log.Printf("[UNDO] restored %s but failed to trim log: %v", rec.Path, err)
		}
		return undoDoneMsg{path: rec.Path, branch: rec.Branch}
	}
}

func undoDeletion(rec DeletionRecord) error {
	root := rec.RepoRoot
	if root == "" {
		r, err := getMainRepoRoot()
		if err != nil {
			return fmt.Errorf("no repo root recorded and none found: %w", err)
		}
		root = r
	}
	if _, err := os.Stat(rec.Path); err == nil {
		return fmt.Errorf("path already exists: %s", rec.Path)
	}

	var args []string
	switch {
	case rec.Branch == "":
		if rec.Head == "" {
			return fmt.Errorf("nothing to restore: no branch or head recorded")
		}
		args = []string{"worktree", "add", "--detach", rec.Path, rec.Head}
	case branchExistsAt(root, rec.Branch):
		args = []string{"worktree", "add", rec.Path, rec.Branch}
	case rec.Head != "":
		args = []string{"worktree", "add", "-b", rec.Branch, rec.Path, rec.Head}
	default:
		return fmt.Errorf("branch %q no longer exists and no head recorded", rec.Branch)
	}

	log.Printf("[UNDO] restoring %s via git %s (root=%s)", rec.Path, strings.Join(args, " "), root)
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return nil
}

func branchExistsAt(root, branch string) bool {
	cmd := exec.Command("git", "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	cmd.Dir = root
	return cmd.Run() == nil
}
