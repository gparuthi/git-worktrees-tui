package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
)

const version = "v0.2.0"

type model struct {
	worktrees            []Worktree
	branches             []Branch
	allBranches          []Branch
	filterInput          textinput.Model
	cursor               int
	selected             map[int]struct{}
	view                 string // "worktrees" or "branches" or "newbranch"
	filtering            bool
	viewportHeight       int
	scrollOffset         int
	newBranchInput       textinput.Model
	creatingBranch       bool
	windowWidth          int
	windowHeight         int
	deletingWorktree     bool
	deletingPath         string
	creatingWorktree     bool
	creatingForBranch    string
	creatingNewBranch    bool
	creatingNewBranchName string
	statusMessage        string
}

type Worktree struct {
	Path   string
	Branch string
	Head   string
}

type Branch struct {
	Name     string
	Type     string // "local" or "remote"
	LastCommit string
}

type clearStatusMsg struct{}

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7C3AED")).
			PaddingLeft(2)
	
	activeTabStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#7C3AED")).
			Padding(0, 2)
	
	inactiveTabStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666")).
			Padding(0, 2)
	
	selectedItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(lipgloss.Color("#7C3AED")).
				Bold(true).
				PaddingLeft(1).
				PaddingRight(1)
	
	normalItemStyle = lipgloss.NewStyle().
			PaddingLeft(2)
	
	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666")).
			PaddingTop(1).
			PaddingLeft(2)
	
	inputStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#10B981")).
			Bold(true).
			PaddingLeft(2)
	
	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EF4444")).
			Bold(true).
			PaddingLeft(2)
	
	branchTypeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#10B981"))
	
	remoteBranchTypeStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#F59E0B"))
)

func initialModel() model {
	filterInput := textinput.New()
	filterInput.Placeholder = "Type to fuzzy filter branches..."
	filterInput.CharLimit = 100
	filterInput.Width = 40
	
	newBranchInput := textinput.New()
	newBranchInput.Placeholder = "Enter branch name..."
	newBranchInput.CharLimit = 100
	newBranchInput.Width = 40
	
	return model{
		selected:              make(map[int]struct{}),
		view:                  "worktrees",
		filtering:             false,
		viewportHeight:        20,
		scrollOffset:          0,
		creatingBranch:        false,
		windowWidth:           80,
		windowHeight:          24,
		deletingWorktree:      false,
		deletingPath:          "",
		creatingWorktree:      false,
		creatingForBranch:     "",
		creatingNewBranch:     false,
		creatingNewBranchName: "",
		statusMessage:         "",
		filterInput:           filterInput,
		newBranchInput:        newBranchInput,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		tea.ClearScreen,
		getWorktreesCmd(),
		getBranchesCmd(),
	)
}

func clearStatusAfterDelay() tea.Cmd {
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg {
		return clearStatusMsg{}
	})
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd
	
	// Update text inputs if they're active
	if m.filtering {
		m.filterInput, cmd = m.filterInput.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		// Apply filter as user types
		m.filterBranches()
	}
	
	if m.creatingBranch {
		m.newBranchInput, cmd = m.newBranchInput.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	
	switch msg := msg.(type) {
	case tea.KeyMsg:
		keyStr := msg.String()
		
		// If we're filtering or creating a branch, let the text input handle most keys
		if m.filtering || m.creatingBranch {
			switch keyStr {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				if m.filtering {
					m.filtering = false
					m.filterInput.SetValue("")
					m.filterInput.Blur()
					m.branches = m.allBranches
					m.cursor = 0
				} else if m.creatingBranch {
					m.creatingBranch = false
					m.newBranchInput.SetValue("")
					m.newBranchInput.Blur()
					m.view = "branches"
				}
			case "enter":
				if m.creatingBranch && m.newBranchInput.Value() != "" {
					return m, createNewBranchWorktreeCmd(m.newBranchInput.Value())
				} else if m.filtering && len(m.branches) > 0 {
					// Exit filtering mode and create worktree
					m.filtering = false
					m.filterInput.SetValue("")
					m.filterInput.Blur()
					m.creatingWorktree = true
					m.creatingForBranch = m.branches[m.cursor].Name
					m.statusMessage = fmt.Sprintf("Creating worktree for branch '%s'...", m.branches[m.cursor].Name)
					return m, createWorktreeCmd(m.branches[m.cursor])
				}
			case "up", "k":
				if m.filtering && m.cursor > 0 {
					m.cursor--
					m.adjustScrollOffset()
				}
			case "down", "j":
				if m.filtering && m.cursor < len(m.branches)-1 {
					m.cursor++
					m.adjustScrollOffset()
				}
			}
			// Return early to let text input handle other keys
			return m, tea.Batch(cmds...)
		}
		
		// Handle key combinations and special keys when not in input mode
		switch {
		case keyStr == "ctrl+c" || keyStr == "q":
			return m, tea.Quit
			
		case keyStr == "enter":
			if m.view == "worktrees" && len(m.worktrees) > 0 {
				return m, openWorktreeCmd(m.worktrees[m.cursor])
			} else if m.view == "branches" && len(m.branches) > 0 {
				// Set creating status
				m.creatingWorktree = true
				m.creatingForBranch = m.branches[m.cursor].Name
				m.statusMessage = fmt.Sprintf("Creating worktree for branch '%s'...", m.branches[m.cursor].Name)
				return m, createWorktreeCmd(m.branches[m.cursor])
			}
			
		case keyStr == "up" || keyStr == "k":
			if m.cursor > 0 {
				m.cursor--
				m.adjustScrollOffset()
			}
			
		case keyStr == "down" || keyStr == "j":
			if m.view == "worktrees" && m.cursor < len(m.worktrees)-1 {
				m.cursor++
				m.adjustScrollOffset()
			} else if m.view == "branches" && m.cursor < len(m.branches)-1 {
				m.cursor++
				m.adjustScrollOffset()
			}
			
		case keyStr == "tab":
			if !m.filtering && !m.creatingBranch {
				if m.view == "worktrees" {
					m.view = "branches"
				} else {
					m.view = "worktrees"
				}
				m.cursor = 0
				m.scrollOffset = 0
			}
			
		case (keyStr == "/" || keyStr == "f") && m.view == "branches" && !m.filtering && !m.creatingBranch:
			m.filtering = true
			m.filterInput.SetValue("")
			m.filterInput.Focus()
			cmd = m.filterInput.Focus()
			cmds = append(cmds, cmd)
			
		case keyStr == "n" && m.view == "branches" && !m.filtering && !m.creatingBranch:
			m.creatingBranch = true
			m.newBranchInput.SetValue("")
			m.newBranchInput.Focus()
			cmd = m.newBranchInput.Focus()
			cmds = append(cmds, cmd)
			
		case keyStr == "d" && !m.filtering && !m.creatingBranch && !m.deletingWorktree && m.view == "worktrees" && len(m.worktrees) > 0:
			return m, deleteWorktreeCmd(m.worktrees[m.cursor])
			
		}

	case worktreesMsg:
		m.worktrees = []Worktree(msg)
	case branchesMsg:
		m.allBranches = []Branch(msg)
		m.branches = m.allBranches
		m.filterBranches()
	case newBranchCreatingMsg:
		// Show immediate feedback while creating
		m.creatingNewBranch = true
		m.creatingNewBranchName = msg.branchName
		m.statusMessage = fmt.Sprintf("Creating new branch '%s' and worktree...", msg.branchName)
		return m, performCreateNewBranchWorktreeCmd(msg.branchName)
	case newBranchCreatedMsg:
		m.creatingBranch = false
		m.creatingNewBranch = false
		m.creatingNewBranchName = ""
		m.newBranchInput.SetValue("")
		m.newBranchInput.Blur()
		m.view = "worktrees"
		m.cursor = 0
		m.scrollOffset = 0
		m.statusMessage = "✅ New branch and worktree created successfully"
		return m, tea.Batch(
			getWorktreesCmd(),
			clearStatusAfterDelay(),
		)
	case deletingWorktreeMsg:
		m.deletingWorktree = true
		m.deletingPath = msg.path
		// Find the worktree to delete
		for _, worktree := range m.worktrees {
			if worktree.Path == msg.path {
				return m, performDeleteWorktreeCmd(worktree)
			}
		}
		return m, nil
	case worktreeDeletedMsg:
		m.deletingWorktree = false
		m.deletingPath = ""
		return m, getWorktreesCmd()
	case worktreeCreatedMsg:
		// Switch to worktrees view and refresh the list
		m.view = "worktrees"
		m.cursor = 0
		m.scrollOffset = 0
		m.creatingWorktree = false
		m.creatingForBranch = ""
		m.statusMessage = fmt.Sprintf("✅ Successfully created worktree for branch '%s'", msg.branch)
		return m, tea.Batch(
			getWorktreesCmd(),
			clearStatusAfterDelay(),
		)
	case tea.WindowSizeMsg:
		m.windowWidth = msg.Width
		m.windowHeight = msg.Height
		m.viewportHeight = msg.Height - 8
	case clearStatusMsg:
		m.statusMessage = ""
	default:
		// Handle errors from git operations
		if err, ok := msg.(error); ok {
			if m.creatingNewBranch {
				m.creatingNewBranch = false
				m.creatingNewBranchName = ""
			}
			if m.creatingWorktree {
				m.creatingWorktree = false
				m.creatingForBranch = ""
			}
			m.statusMessage = fmt.Sprintf("❌ Error: %v", err)
			return m, clearStatusAfterDelay()
		}
	}

	if len(cmds) > 0 {
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

func (m model) View() string {
	var content strings.Builder
	
	// Header with tabs
	header := m.renderHeader()
	content.WriteString(header)
	content.WriteString("\n\n")

	// Show status message if any
	if m.statusMessage != "" {
		statusStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#10B981")).
			Bold(true).
			PaddingLeft(2)
		content.WriteString(statusStyle.Render(m.statusMessage))
		content.WriteString("\n\n")
	} else if m.creatingWorktree {
		// Show creating status
		creatingStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F59E0B")).
			Bold(true).
			PaddingLeft(2)
		content.WriteString(creatingStyle.Render("⏳ Creating worktree..."))
		content.WriteString("\n\n")
	} else if m.creatingNewBranch {
		// Show creating new branch status
		creatingStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F59E0B")).
			Bold(true).
			PaddingLeft(2)
		content.WriteString(creatingStyle.Render(fmt.Sprintf("⏳ Creating new branch '%s'...", m.creatingNewBranchName)))
		content.WriteString("\n\n")
	}

	if m.view == "worktrees" {
		if len(m.worktrees) == 0 {
			content.WriteString(errorStyle.Render("No worktrees found."))
			content.WriteString("\n")
		} else {
			start, end := m.getViewportRange(len(m.worktrees))
			for i := start; i < end; i++ {
				if i >= len(m.worktrees) {
					break
				}
				worktree := m.worktrees[i]
				itemContent := m.renderWorktreeItem(worktree, i == m.cursor)
				content.WriteString(itemContent)
				content.WriteString("\n")
			}
			// Add scroll indicator
			if len(m.worktrees) > m.viewportHeight {
				content.WriteString(m.renderScrollIndicator(end-start, len(m.worktrees)))
				content.WriteString("\n")
			}
		}
		
		content.WriteString(helpStyle.Render("Press 'enter' to open, 'd' to delete, 'tab' to switch to branches"))
	} else {
		if m.creatingBranch {
			content.WriteString(inputStyle.Render("New branch name: "))
			content.WriteString(m.newBranchInput.View())
			content.WriteString("\n")
		} else if m.filtering {
			content.WriteString(inputStyle.Render("Filter: "))
			content.WriteString(m.filterInput.View())
			content.WriteString("\n")
		} else {
			content.WriteString("\n")
		}
		
		if m.creatingBranch {
			// Don't show branch list when creating new branch
		} else if len(m.branches) == 0 {
			if m.filtering {
				content.WriteString(errorStyle.Render("No branches match filter."))
			} else {
				content.WriteString(errorStyle.Render("No branches found."))
			}
			content.WriteString("\n")
		} else {
			start, end := m.getViewportRange(len(m.branches))
			for i := start; i < end; i++ {
				if i >= len(m.branches) {
					break
				}
				branch := m.branches[i]
				itemContent := m.renderBranchItem(branch, i == m.cursor)
				content.WriteString(itemContent)
				content.WriteString("\n")
			}
			// Add scroll indicator
			if len(m.branches) > m.viewportHeight {
				content.WriteString(m.renderScrollIndicator(end-start, len(m.branches)))
				content.WriteString("\n")
			}
		}
		
		if m.creatingBranch {
			content.WriteString(helpStyle.Render("Press 'enter' to create, 'esc' to cancel (standard text editing keys work)"))
		} else if m.filtering {
			content.WriteString(helpStyle.Render("Type to fuzzy filter, 'enter' to select, 'esc' to cancel (all text editing keys work)"))
		} else {
			content.WriteString(helpStyle.Render("Press 'enter' to create worktree, 'n' for new branch, 'f' or '/' to filter, 'tab' to switch to worktrees"))
		}
	}

	content.WriteString("\n")
	content.WriteString(helpStyle.Render("Press 'q' to quit."))
	return content.String()
}

func (m model) renderHeader() string {
	var tabs []string
	
	if m.view == "worktrees" {
		tabs = append(tabs, activeTabStyle.Render("Worktrees"))
		tabs = append(tabs, inactiveTabStyle.Render("Branches"))
	} else {
		tabs = append(tabs, inactiveTabStyle.Render("Worktrees"))
		tabs = append(tabs, activeTabStyle.Render("Branches"))
	}
	
	// Add version to the right
	versionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#666666")).
		PaddingLeft(2)
	
	tabsWidth := lipgloss.Width(lipgloss.JoinHorizontal(lipgloss.Top, tabs...))
	versionText := versionStyle.Render(version)
	versionWidth := lipgloss.Width(versionText)
	
	// Calculate spacing to right-align version
	spacing := m.windowWidth - tabsWidth - versionWidth
	if spacing < 1 {
		spacing = 1
	}
	
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		lipgloss.JoinHorizontal(lipgloss.Top, tabs...),
		strings.Repeat(" ", spacing),
		versionText,
	)
}

func (m model) renderWorktreeItem(worktree Worktree, selected bool) string {
	// Shorten path for display
	displayPath := worktree.Path
	if len(displayPath) > 50 {
		displayPath = "..." + displayPath[len(displayPath)-47:]
	}
	
	content := fmt.Sprintf("%s (%s)", filepath.Base(displayPath), worktree.Branch)
	
	// Check if this worktree is being deleted
	if m.deletingWorktree && worktree.Path == m.deletingPath {
		deletingStyle := errorStyle.Copy().Strikethrough(true)
		return deletingStyle.Render("🗑️  Deleting " + content + "...")
	}
	
	if selected {
		return selectedItemStyle.Render("▶ " + content)
	}
	return normalItemStyle.Render("  " + content)
}

func (m model) renderBranchItem(branch Branch, selected bool) string {
	var typeStyle lipgloss.Style
	var typeLabel string
	
	if branch.Type == "local" {
		typeStyle = branchTypeStyle
		typeLabel = "local"
	} else {
		typeStyle = remoteBranchTypeStyle
		typeLabel = "remote"
	}
	
	content := fmt.Sprintf("%s %s", typeStyle.Render("["+typeLabel+"]"), branch.Name)
	
	if selected {
		return selectedItemStyle.Render("▶ " + content)
	}
	return normalItemStyle.Render("  " + content)
}

func (m model) renderScrollIndicator(currentItems, totalItems int) string {
	if totalItems <= m.viewportHeight {
		return ""
	}
	
	scrollInfo := fmt.Sprintf(" (%d/%d)", currentItems, totalItems)
	return helpStyle.Render(scrollInfo)
}

func (m *model) filterBranches() {
	filterText := m.filterInput.Value()
	if filterText == "" {
		m.branches = m.allBranches
		return
	}

	// Create a slice of branch names for fuzzy search
	branchNames := make([]string, len(m.allBranches))
	for i, branch := range m.allBranches {
		branchNames[i] = branch.Name
	}
	
	// Perform fuzzy search
	matches := fuzzy.Find(filterText, branchNames)
	
	// Build filtered branches based on matches
	filtered := make([]Branch, 0, len(matches))
	for _, match := range matches {
		filtered = append(filtered, m.allBranches[match.Index])
	}
	
	m.branches = filtered
	
	// Reset cursor if it's out of bounds
	if m.cursor >= len(m.branches) {
		m.cursor = 0
	}
}

func (m *model) adjustScrollOffset() {
	if m.cursor < m.scrollOffset {
		m.scrollOffset = m.cursor
	} else if m.cursor >= m.scrollOffset+m.viewportHeight {
		m.scrollOffset = m.cursor - m.viewportHeight + 1
	}
}

func (m *model) getViewportRange(totalItems int) (int, int) {
	if totalItems <= m.viewportHeight {
		return 0, totalItems
	}
	
	start := m.scrollOffset
	end := start + m.viewportHeight
	
	if end > totalItems {
		end = totalItems
		start = end - m.viewportHeight
		if start < 0 {
			start = 0
		}
	}
	
	return start, end
}

func isValidBranchChar(s string) bool {
	if len(s) != 1 {
		return false
	}
	c := s[0]
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '/' || c == '.'
}

func runNonInteractive(listWorktrees, listBranches *bool, createWorktreeFlag, deleteWorktreeFlag, createNewBranch *string) {
	if *listWorktrees {
		worktrees, err := getWorktrees()
		if err != nil {
			fmt.Printf("Error getting worktrees: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Worktrees:")
		for _, wt := range worktrees {
			fmt.Printf("  %s (%s)\n", wt.Path, wt.Branch)
		}
	}

	if *listBranches {
		branches, err := getBranches()
		if err != nil {
			fmt.Printf("Error getting branches: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Branches:")
		for _, branch := range branches {
			fmt.Printf("  [%s] %s\n", branch.Type, branch.Name)
		}
	}

	if *createWorktreeFlag != "" {
		// Find the branch
		branches, err := getBranches()
		if err != nil {
			fmt.Printf("Error getting branches: %v\n", err)
			os.Exit(1)
		}
		
		var targetBranch *Branch
		for _, branch := range branches {
			if branch.Name == *createWorktreeFlag {
				targetBranch = &branch
				break
			}
		}
		
		if targetBranch == nil {
			fmt.Printf("Error: branch '%s' not found\n", *createWorktreeFlag)
			os.Exit(1)
		}
		
		err = createWorktree(*targetBranch)
		if err != nil {
			fmt.Printf("Error creating worktree: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Successfully created worktree for branch '%s'\n", targetBranch.Name)
	}

	if *deleteWorktreeFlag != "" {
		// Find the worktree
		worktrees, err := getWorktrees()
		if err != nil {
			fmt.Printf("Error getting worktrees: %v\n", err)
			os.Exit(1)
		}
		
		var targetWorktree *Worktree
		for _, wt := range worktrees {
			if wt.Path == *deleteWorktreeFlag || filepath.Base(wt.Path) == *deleteWorktreeFlag {
				targetWorktree = &wt
				break
			}
		}
		
		if targetWorktree == nil {
			fmt.Printf("Error: worktree '%s' not found\n", *deleteWorktreeFlag)
			os.Exit(1)
		}
		
		err = deleteWorktree(*targetWorktree)
		if err != nil {
			fmt.Printf("Error deleting worktree: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Successfully deleted worktree at '%s'\n", targetWorktree.Path)
	}

	if *createNewBranch != "" {
		err := createNewBranchWorktree(*createNewBranch)
		if err != nil {
			fmt.Printf("Error creating new branch and worktree: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Successfully created new branch '%s' and worktree\n", *createNewBranch)
	}
}

func main() {
	// Define command-line flags
	listWorktrees := flag.Bool("list-worktrees", false, "List all worktrees")
	listBranches := flag.Bool("list-branches", false, "List all branches")
	createWorktreeFlag := flag.String("create-worktree", "", "Create a worktree for the specified branch")
	deleteWorktreeFlag := flag.String("delete-worktree", "", "Delete the worktree at the specified path")
	createNewBranch := flag.String("create-new-branch", "", "Create a new branch and worktree")
	nonInteractive := flag.Bool("non-interactive", false, "Run in non-interactive mode")
	help := flag.Bool("help", false, "Show help message")

	flag.Parse()
	
	if *help {
		fmt.Println("wtree - Git worktree manager")
		fmt.Println("\nUsage:")
		fmt.Println("  wtree                       Run in interactive mode (default)")
		fmt.Println("  wtree --list-worktrees      List all worktrees")
		fmt.Println("  wtree --list-branches       List all branches")
		fmt.Println("  wtree --create-worktree <branch>   Create a worktree for the specified branch")
		fmt.Println("  wtree --delete-worktree <path>     Delete the worktree at the specified path")
		fmt.Println("  wtree --create-new-branch <name>   Create a new branch and worktree")
		fmt.Println("  wtree --help                Show this help message")
		fmt.Println("\nExamples:")
		fmt.Println("  wtree --create-worktree feature/new-feature")
		fmt.Println("  wtree --delete-worktree ../playground-feature-new-feature")
		fmt.Println("  wtree --create-new-branch bugfix/fix-issue")
		return
	}

	// Log current working directory
	wd, err := os.Getwd()
	if err != nil {
		log.Printf("Warning: could not get working directory: %v", err)
	} else {
		log.Printf("wtree running from: %s", wd)
	}

	// Check if we're in a git repository
	if !isGitRepository() {
		fmt.Println("Error: wtree must be run from within a git repository")
		fmt.Println("Please navigate to a git repository and try again.")
		os.Exit(1)
	}

	// Handle non-interactive commands
	if *listWorktrees || *listBranches || *createWorktreeFlag != "" || *deleteWorktreeFlag != "" || *createNewBranch != "" || *nonInteractive {
		runNonInteractive(listWorktrees, listBranches, createWorktreeFlag, deleteWorktreeFlag, createNewBranch)
		return
	}

	// Run interactive mode with alternate screen
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}