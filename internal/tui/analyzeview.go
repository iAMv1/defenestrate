package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/iAMv1/defenestrate/internal/analyze"
	"github.com/iAMv1/defenestrate/internal/safety"
	"github.com/iAMv1/defenestrate/internal/ui"
)

// analyzeModel is the interactive disk explorer: drill-down tree with bars,
// a large-items pane, and Recycle-Bin deletion behind an explicit confirm.
type analyzeModel struct {
	stack    []string // path chain; last = current root
	entries  []analyze.Entry
	large    []analyze.File
	total    int64
	files    int
	cursor   int
	pane     int // 0 = tree, 1 = large items, 2 = treemap
	loading  bool
	errText  string
	confirm  string // path pending deletion confirmation ("": none)
	deleting bool
	width    int
}

type loadedMsg struct {
	path string
	res  *analyze.Result
	err  error
}

func newAnalyzeModel(root string) analyzeModel {
	return analyzeModel{stack: []string{root}, loading: true}
}

func loadCmd(path string) tea.Cmd {
	return func() tea.Msg {
		r, err := analyze.Walk(path, 40)
		if err != nil {
			return loadedMsg{path: path, err: err}
		}
		return loadedMsg{path: path, res: r}
	}
}

func (m analyzeModel) Init() tea.Cmd {
	return loadCmd(m.current())
}

func (m analyzeModel) current() string { return m.stack[len(m.stack)-1] }

func (m *analyzeModel) push(p string) {
	if p != m.current() {
		m.stack = append(m.stack, p)
		m.loading = true
	}
}

func (m *analyzeModel) pop() bool {
	if len(m.stack) > 1 {
		m.stack = m.stack[:len(m.stack)-1]
		m.loading = true
		return true
	}
	return false
}

func (m analyzeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case loadedMsg:
		m.loading = false
		if msg.err != nil {
			m.errText = msg.err.Error()
			return m, nil
		}
		m.errText = ""
		if msg.path == m.current() {
			m.entries = msg.res.Children
			m.large = msg.res.LargeFiles
			m.total = msg.res.TotalBytes
			m.files = msg.res.TotalFiles
			m.cursor = 0
		}
	case tea.KeyMsg:
		if m.confirm != "" {
			switch strings.ToLower(msg.String()) {
			case "y":
				target := m.confirm
				m.confirm = ""
				m.deleting = true
				return m, tea.Sequence(
					func() tea.Msg {
						err := safety.Recycle([]string{target})
						return deletedMsg{path: target, err: err}
					},
				)
			default:
				m.confirm = ""
			}
			return m, nil
		}
		if m.deleting {
			return m, nil
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			list := m.activeCount()
			if m.cursor < list-1 {
				m.cursor++
			}
		case "enter", "l":
			if m.pane == 0 && m.cursor < len(m.entries) && m.entries[m.cursor].IsDir {
				m.push(m.entries[m.cursor].Path)
			}
		case "h", "esc", "backspace":
			if m.pop() {
				return m, loadCmd(m.current())
			}
		case "tab":
			m.pane = 1 - (m.pane % 2) // toggle tree/large
			m.cursor = 0
		case "t":
			m.pane = 2 // treemap
			m.cursor = 0
		case "d":
			p := m.selectedPath()
			if p != "" {
				m.confirm = p
			}
		case "r":
			m.loading = true
			return m, loadCmd(m.current())
		}
	case deletedMsg:
		m.deleting = false
		if msg.err != nil {
			m.errText = msg.err.Error()
		} else {
			m.errText = ""
			safety.Logf("recycle (analyze)", msg.path, 0)
		}
		return m, loadCmd(m.current())
	}
	return m, nil
}

type deletedMsg struct {
	path string
	err  error
}

func (m analyzeModel) activeCount() int {
	if m.pane == 1 {
		return len(m.large)
	}
	return len(m.entries)
}

func (m analyzeModel) selectedPath() string {
	if m.pane == 1 && m.cursor < len(m.large) {
		return m.large[m.cursor].Path
	}
	if m.pane == 0 && m.cursor < len(m.entries) {
		return m.entries[m.cursor].Path
	}
	return ""
}

func (m analyzeModel) View() string {
	var b strings.Builder
	crumbs := strings.Join(m.stack, " › ")
	b.WriteString(ui.Header("DEFENESTRATE ANALYZE", crumbs, max(m.width, 60)))
	if m.loading {
		b.WriteString(ui.StyleDim.Render("scanning…") + "\n")
		return b.String()
	}
	if m.errText != "" {
		b.WriteString(ui.StyleBad.Render("error: "+m.errText) + "\n")
	}

	free := ui.FreeBytes(m.current())
	fmt.Fprintf(&b, "%s   %s free\n\n",
		ui.Bold("Total: "+ui.HumanBytes(m.total)), ui.HumanBytesU(free))

	maxSize := int64(1)
	for _, e := range m.entries {
		if e.Size > maxSize {
			maxSize = e.Size
		}
	}

	switch m.pane {
	case 0:
		for i, e := range m.entries {
			row := renderTreeRow(e, maxSize, i == m.cursor)
			b.WriteString(row + "\n")
		}
		if len(m.entries) == 0 {
			b.WriteString(ui.StyleDim.Render("  empty") + "\n")
		}
	case 1:
		if len(m.large) == 0 {
			b.WriteString(ui.StyleDim.Render("  no large items") + "\n")
		}
		for i, f := range m.large {
			sel := i == m.cursor
			p := f.Path
			if len(p) > intW(m.width) {
				p = "…" + p[len(p)-(intW(m.width)-2):]
			}
			line := fmt.Sprintf("  %10s  %s", ui.HumanBytes(f.Size), p)
			if sel {
				b.WriteString(ui.StyleSel.Render("▶ "+line) + "\n")
			} else {
				b.WriteString("  " + line + "\n")
			}
		}
	case 2:
		cols, rows := treemapSize(m.width)
		tiles := BuildTreeMap(m.entries, cols, rows)
		selected := ""
		if m.cursor < len(m.entries) {
			selected = m.entries[m.cursor].Path
		}
		b.WriteString(RenderTreeMap(tiles, cols, rows, selected))
	}

	if m.confirm != "" {
		b.WriteString("\n" + ui.StyleWarn.Render("Recycle to bin: "+m.confirm))
		b.WriteString("\n" + ui.StyleHelp.Render("y confirm · any key cancel"))
	} else if !m.deleting {
		b.WriteString("\n" + ui.StyleHelp.Render("↑↓ move · enter open · h back · tab large · t treemap · d recycle · r rescan · q quit"))
	}
	return b.String()
}

func renderTreeRow(e analyze.Entry, maxSize int64, selected bool) string {
	frac := float64(e.Size) / float64(maxSize)
	label := e.Name
	if len(label) > 34 {
		label = label[:33] + "…"
	}
	kind := "dir "
	if !e.IsDir {
		kind = "file"
	}
	line := fmt.Sprintf("%-4s %-34s %s %10s",
		ui.StyleDim.Render(kind), label, ui.BarColored(frac, 22), ui.HumanBytes(e.Size))
	if selected {
		return ui.StyleSel.Render("▶ " + line)
	}
	return "  " + line
}

func intW(width int) int {
	w := width - 16
	if w < 40 {
		w = 40
	}
	return w
}

func treemapSize(width int) (int, int) {
	cols := width - 2
	if cols < 24 {
		cols = 24
	}
	return cols, 16
}

// RunAnalyzeTUI launches the interactive explorer at root.
func RunAnalyzeTUI(root string) error {
	root, _ = filepath.Abs(root)
	if _, err := tea.NewProgram(newAnalyzeModel(root), tea.WithAltScreen()).Run(); err != nil {
		return err
	}
	return nil
}

var _ = lipgloss.NewStyle // keep import if styles are added later
