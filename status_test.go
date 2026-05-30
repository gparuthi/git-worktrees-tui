package main

import (
	"strings"
	"testing"
	"time"
)

func TestRelativeTime(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		t    time.Time
		want string
	}{
		{"zero", time.Time{}, ""},
		{"now", now.Add(-10 * time.Second), "now"},
		{"minutes", now.Add(-5 * time.Minute), "5m"},
		{"hours", now.Add(-3 * time.Hour), "3h"},
		{"days", now.Add(-2 * 24 * time.Hour), "2d"},
		{"weeks", now.Add(-2 * 7 * 24 * time.Hour), "2w"},
		{"months", now.Add(-2 * 30 * 24 * time.Hour), "2mo"},
		{"years", now.Add(-2 * 365 * 24 * time.Hour), "2y"},
	}
	for _, c := range cases {
		if got := relativeTime(c.t); got != c.want {
			t.Errorf("%s: relativeTime = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestRenderPRBadge(t *testing.T) {
	cases := []struct {
		state string
		want  string
	}{
		{"MERGED", "PR #1 merged"},
		{"OPEN", "PR #1 open"},
		{"CLOSED", "PR #1 closed"},
		{"", "PR #1 open"}, // unexpected state falls back to open styling
	}
	for _, c := range cases {
		got := renderPRBadge(&PRStatus{Number: 1, State: c.state})
		if !strings.Contains(got, c.want) {
			t.Errorf("state %q: badge %q does not contain %q", c.state, got, c.want)
		}
	}
}

func TestWorktreeFlagsPlain(t *testing.T) {
	cases := []struct {
		wt   Worktree
		want string
	}{
		{Worktree{}, ""},
		{Worktree{Dirty: true}, "*"},
		{Worktree{Ahead: 3}, "↑3"},
		{Worktree{Dirty: true, Ahead: 2}, "* ↑2"},
	}
	for _, c := range cases {
		if got := worktreeFlagsPlain(c.wt); got != c.want {
			t.Errorf("worktreeFlagsPlain(%+v) = %q, want %q", c.wt, got, c.want)
		}
	}
}

func TestWorktreeRow(t *testing.T) {
	m := initialModel()
	m.worktrees = []Worktree{
		{Path: "/repo/wt-merged", Branch: "gp-merged", LastCommit: time.Now().Add(-3 * 24 * time.Hour)},
		{Path: "/repo/wt-open", Branch: "gp-open", Dirty: true, Ahead: 2, LastCommit: time.Now().Add(-2 * time.Hour)},
		{Path: "/repo/wt-none", Branch: "gp-none", LastCommit: time.Now().Add(-time.Hour)},
	}
	m.prStatuses = map[string]*PRStatus{
		"gp-merged": {Number: 100, State: "MERGED"},
		"gp-open":   {Number: 200, State: "OPEN"},
		"gp-none":   nil, // looked up, no PR
	}
	c := m.worktreeColWidths()

	// Colored row for a clean, merged worktree advertises the merged PR + age.
	row := m.worktreeRow(m.worktrees[0], c, true)
	for _, want := range []string{"wt-merged", "gp-merged", "3d", "PR #100 merged"} {
		if !strings.Contains(row, want) {
			t.Errorf("merged row missing %q: %q", want, row)
		}
	}

	// A dirty worktree with an open PR shows the '*' marker, ahead count, badge.
	row = m.worktreeRow(m.worktrees[1], c, true)
	for _, want := range []string{"*", "↑2", "2h", "PR #200 open"} {
		if !strings.Contains(row, want) {
			t.Errorf("dirty row missing %q: %q", want, row)
		}
	}

	// A branch looked up with no PR renders no PR text.
	if row := m.worktreeRow(m.worktrees[2], c, true); strings.Contains(row, "PR #") {
		t.Errorf("no-PR row should not render a PR badge: %q", row)
	}

	// While loading, an unknown branch shows the pending indicator.
	m.prLoading = true
	loading := Worktree{Path: "/repo/wt-unknown", Branch: "gp-unknown"}
	if got := m.prCell(loading, false); !strings.Contains(got, "PR") {
		t.Errorf("loading worktree should show pending PR indicator: %q", got)
	}
}

func TestWorktreeColWidthsCaps(t *testing.T) {
	m := initialModel()
	long := strings.Repeat("x", 100)
	m.worktrees = []Worktree{{Path: "/repo/" + long, Branch: long}}
	c := m.worktreeColWidths()
	if c.name > maxNameCol {
		t.Errorf("name col %d exceeds cap %d", c.name, maxNameCol)
	}
	if c.branch > maxBranchCol {
		t.Errorf("branch col %d exceeds cap %d", c.branch, maxBranchCol)
	}
}
