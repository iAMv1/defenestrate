package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/iAMv1/defenestrate/internal/safety"
	"github.com/iAMv1/defenestrate/internal/ui"
)

// menuModel is the hub. Items either run an in-process TUI flow (clean,
// uninstall) or hand off to a subprocess command via tea.ExecProcess
// (status keeps its own alt-screen program; analyze likewise).
type menuModel struct {
	cursor   int
	version  string
	quitting bool
	width    int
	dryRun   bool
}

var menuItems = []struct {
	label string
	desc  string
	cmd   string // "" = quit
}{
	{"Deep clean", "caches · browser data · old logs — with review holds", "clean"},
	{"Uninstall apps", "programs + every registry-evidence leftover", "uninstall"},
	{"Analyze disk", "interactive tree of what is eating your space", "analyze"},
	{"Live status", "CPU / memory / disk dashboard", "status"},
	{"Optimize", "bounded maintenance tasks", "optimize"},
	{"History", "audit log of every mutation", "history"},
	{"Quit", "", ""},
}

func newMenuModel(version string) menuModel {
	return menuModel{version: version, dryRun: safety.DryRun()}
}

func (m menuModel) Init() tea.Cmd { return nil }

func (m menuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "q":
			if !m.filtering() {
				m.quitting = true
				return m, tea.Quit
			}
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(menuItems)-1 {
				m.cursor++
			}
		case "enter", "l":
			it := menuItems[m.cursor]
			if it.cmd == "" {
				m.quitting = true
				return m, tea.Quit
			}
			return m, m.menuExec(it.cmd)
		}
	}
	return m, nil
}

func (m menuModel) filtering() bool { return false }

// menuExec runs a DEFENESTRATE subcommand in a real subprocess while bubbletea
// suspends this program — each flow keeps its own alt-screen and prompts.
// Dry-run propagates: the hub never launches a child that could mutate.
func (m menuModel) menuExec(subcmd string) tea.Cmd {
	exe, err := os.Executable()
	if err != nil {
		exe = os.Args[0]
	}
	args := []string{subcmd}
	if m.dryRun {
		args = append(args, "--dry-run")
	}
	c := exec.Command(exe, args...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return tea.ExecProcess(c, func(error) tea.Msg { return nil })
}

func (m menuModel) View() string {
	var b strings.Builder
	b.WriteString(ui.StyleTitle.Render("DEFENESTRATE "+m.version+" — deep clean · uninstall · analyze · monitor"))
	b.WriteString("\n")
	if m.dryRun {
		b.WriteString(ui.StyleWarn.Render("DRY RUN ACTIVE — every action previews only") + "\n")
	}
	b.WriteString("\n")
	for i, it := range menuItems {
		prompt := "  "
		if i == m.cursor {
			prompt = "▶ "
		}
		line := prompt + it.label
		if it.desc != "" {
			line += ui.StyleDim.Render("  — " + it.desc)
		}
		if i == m.cursor {
			b.WriteString(ui.StyleSel.Render(line) + "\n")
		} else {
			b.WriteString(line + "\n")
		}
	}
	b.WriteString("\n" + ui.StyleHelp.Render("↑↓/jk move · enter run · q quit · tip: `DEFENESTRATE clean --dry-run` previews from the CLI"))
	return b.String()
}

// Menu runs the interactive hub.
func Menu(version string) error {
	fmt.Println(ui.StyleWarn.Render("tip: preview any action first with `DEFENESTRATE clean --dry-run`") + "\n")
	if _, err := tea.NewProgram(newMenuModel(version), tea.WithAltScreen()).Run(); err != nil {
		return err
	}
	return nil
}
