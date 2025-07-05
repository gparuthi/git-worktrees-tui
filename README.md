# Worktree TUI

A Terminal User Interface (TUI) application for managing Git worktrees written in Go.

## Features

- **List existing worktrees** - View all current worktrees with their paths and branches
- **Create new worktrees** - Create worktrees for existing branches or new branches
- **Delete worktrees** - Remove unwanted worktrees
- **Branch filtering** - View branches sorted by local/remote and recency
- **IDE integration** - Open worktrees in Cursor IDE with a single keypress

## Installation

```bash
go build -o worktree-tui
```

## Usage

```bash
./worktree-tui
```

### Key Bindings

- **Tab** - Switch between worktrees and branches view
- **↑/↓ or k/j** - Navigate up/down
- **Enter** - 
  - In worktrees view: Open worktree in Cursor IDE
  - In branches view: Create new worktree for selected branch
- **d** - Delete selected worktree (in worktrees view)
- **/** - Start fuzzy filtering branches (in branches view)
- **n** - Create new branch and worktree (in branches view)
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

## Requirements

- Git repository
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