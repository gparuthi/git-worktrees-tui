# Worktree TUI

A Terminal User Interface (TUI) application for managing Git worktrees written in Go.

## Features

- **List existing worktrees** - View all current worktrees with their paths and branches
- **Create new worktrees** - Create worktrees for existing branches or new branches (`n` from anywhere in the TUI)
- **Create from a GitHub PR** - `wtree --from-pr 165334` (or paste a `pull/...` URL); also `p` from inside the TUI
- **Repo map config** - `~/.config/wtree/repos.yaml` lets `--from-pr <url>` work from any directory by routing PRs from known repos to a configured local checkout
- **Delete worktrees** - Remove unwanted worktrees
- **Branch filtering** - View branches sorted by local/remote and recency
- **IDE integration** - Open worktrees in Cursor IDE with a single keypress
- **Drop into the new worktree** - When a worktree is created (TUI or `--from-pr`), the shell wrapper `cd`s into it on exit

## Installation

```bash
go install
```

### Shell Wrapper (recommended)

Add this to your `.zshrc` to enable the `t`, `a`, and `c` shortcuts (cd into worktree, launch claude):

```bash
function wtree() {
  local tmp=$(mktemp -t "wtree-cmd.XXXXXX")
  command wtree --cmd-file="$tmp" "$@"
  if [ -f "$tmp" ]; then
    local cmd=$(cat "$tmp")
    rm -f "$tmp"
    [ -n "$cmd" ] && eval "$cmd"
  fi
}
```

Without the wrapper, `t`/`a`/`c` shortcuts won't work (the program can't change your shell's directory).

## Usage

```bash
wtree
```

### Key Bindings

- **Tab** - Switch between worktrees and branches view
- **↑/↓ or k/j** - Navigate up/down
- **Enter** -
  - In worktrees view: Open worktree in Cursor IDE
  - In branches view: Create new worktree for selected branch
- **t** - cd into selected worktree (requires shell wrapper)
- **a** - cd into worktree and start `claude` (requires shell wrapper)
- **c** - cd into worktree and start `claude -r` (requires shell wrapper)
- **d** - Delete selected worktree (in worktrees view)
- **/** - Start fuzzy filtering branches (in branches view)
- **r** - Refresh worktrees/branches
- **n** - Create new branch and worktree (any view) — TUI exits and shell `cd`s into it
- **p** - Create worktree from a GitHub PR (paste number or URL) — TUI exits and shell `cd`s into it
- **Esc** - Clear filter/cancel new branch creation
- **Backspace** - Remove last character from filter/branch name
- **q or Ctrl+C** - Quit application

### Views

#### Worktrees View
- Shows all existing worktrees with their paths and associated branches
- Press Enter to open a worktree in Cursor IDE
- Press 'd' to delete a worktree

#### Branches View  
- Shows all branches (local and remote) sorted by type and recency
- Local branches are shown first, followed by remote branches
- Press Enter to create a new worktree for the selected branch
- Press 'n' to create a new branch and worktree - type the branch name and press Enter
- Press '/' to start fuzzy filtering - type to filter branches by name
- Filter is case-insensitive and matches any part of the branch name

## Non-interactive usage

```bash
wtree --from-pr 123
wtree --from-pr https://github.com/owner/repo/pull/123
wtree --create-worktree feature/foo
wtree --create-new-branch bugfix/bar
```

`--from-pr` resolves the head ref via `gh pr view`, fetches `refs/pull/<n>/head` (works for fork PRs too), and creates a worktree at `<repo>-pr-<n>`. The shell wrapper `cd`s into it on exit.

## Repo map (`~/.config/wtree/repos.yaml`)

A starter file is written on first run. Map github owner/repo to local checkouts so `wtree --from-pr <url>` works from any directory:

```yaml
repos:
  - owner: myorg
    repo: my-monorepo
    path: ~/work/my-monorepo
  - owner: myuser
    repo: my-project
    path: ~/work/my-project
```

When the PR URL's owner/repo matches an entry, wtree operates on that checkout regardless of cwd. Without a match, it falls back to cwd's repo (and verifies the URL matches its origin).

## Requirements

- Git repository (or a configured repo map for `--from-pr <url>` from anywhere)
- `gh` CLI (for `--from-pr`)
- Cursor IDE (for opening worktrees)
- Go 1.19+ (for building from source)

## How it works

The application uses Git commands to:
- List worktrees: `git worktree list --porcelain`
- List branches: `git for-each-ref`
- Create worktrees: `git worktree add`
- Delete worktrees: `git worktree remove`
- Open in IDE: `cursor <path>`

Worktrees are created in the parent directory using the format `<repo-name>-<branch-name>` where forward slashes in branch names are replaced with hyphens.