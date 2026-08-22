package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/iAMv1/defenestrate/internal/clean"
	"github.com/iAMv1/defenestrate/internal/rules"
	"github.com/iAMv1/defenestrate/internal/safety"
	"github.com/iAMv1/defenestrate/internal/ui"
)

// cleanModel is the interactive deep-clean flow: scan async → category
// checklist (Low risk pre-selected) → confirm → recycle with per-category
// progress → summary. Held-for-review items are always visible and never
// touched by this flow.
type cleanModel struct {
	findings  []clean.Finding
	skipped   map[string][]string
	checked   map[int]bool // index into findings
	cursor    int
	scanning  bool
	running   bool
	done      bool
	freed     int64
	dryRun    bool
	width     int
	errText   string
	progressI int // next category to recycle
}

type scannedMsg struct {
	findings []clean.Finding
	skipped  map[string][]string
}

type recycledMsg struct {
	index int
	bytes int64
	err   error
}

func newCleanModel(dryRun bool) cleanModel {
	return cleanModel{scanning: true, skipped: map[string][]string{}, dryRun: dryRun}
}

func scanCmd() tea.Cmd {
	return func() tea.Msg {
		f, sk, err := clean.Scan(rules.Targets)
		if err != nil {
			return scannedMsg{}
		}
		_ = err
		return scannedMsg{findings: f, skipped: sk}
	}
}

func recycleCmd(f clean.Finding, index int) tea.Cmd {
	return func() tea.Msg {
		err := safety.Recycle(f.Paths)
		return recycledMsg{index: index, bytes: f.Bytes, err: err}
	}
}

func (m cleanModel) Init() tea.Cmd { return scanCmd() }

func (m cleanModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case scannedMsg:
		m.scanning = false
		m.findings = msg.findings
		m.skipped = msg.skipped
		for i, f := range m.findings {
			// Default selection: Low risk on; Medium/High opt-in.
			m.checked[i] = f.Target.Risk == rules.Low && len(f.Paths) > 0
		}
	case recycledMsg:
		if msg.err != nil {
			m.errText = msg.err.Error()
		} else {
			m.freed += msg.bytes
			m.findings[msg.index].Paths = nil // done
		}
		m.progressI++
		return m, m.nextRecycleCmd()
	case cleanedDoneMsg:
		m.running = false
		m.done = true
	case tea.KeyMsg:
		if m.scanning || m.running || m.done {
			if m.done && msg.String() == "enter" {
				return newCleanModel(m.dryRun), scanCmd() // loop back for another pass
			}
			return m, nil
		}
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.findings)-1 {
				m.cursor++
			}
		case " ", "space":
			m.checked[m.cursor] = !m.checked[m.cursor]
		case "a":
			all := true
			for i := range m.findings {
				if !m.checked[i] {
					all = false
				}
			}
			for i := range m.findings {
				m.checked[i] = !all
			}
		case "enter":
			var total int64
			any := false
			for i, f := range m.findings {
				if m.checked[i] {
					total += f.Bytes
					any = true
				}
			}
			if !any {
				return m, nil
			}
			if safety.DryRun() {
				m.done = true
				return m, nil // preview-only screen already shows plan
			}
			if !confirmClean(fmt.Sprintf("Recycle %s to the Recycle Bin?", ui.HumanBytes(total))) {
				return m, nil
			}
			m.running = true
			m.progressI = 0
			return m, m.nextRecycleCmd()
		}
	}
	return m, nil
}

// nextRecycleCmd recycles the next checked category (sequential so the
// progress line stays truthful), or signals completion.
func (m cleanModel) nextRecycleCmd() tea.Cmd {
	for i := m.progressI; i < len(m.findings); i++ {
		if m.checked[i] && len(m.findings[i].Paths) > 0 {
			f := m.findings[i]
			idx := i
			return func() tea.Msg {
				err := safety.Recycle(f.Paths)
				return recycledMsg{index: idx, bytes: f.Bytes, err: err}
			}
		}
	}
	return func() tea.Msg { return cleanedDoneMsg{} }
}

// confirmClean asks a y/N question on stdin (TUI suspends to line input).
func confirmClean(q string) bool {
	fmt.Printf("%s [y/N] ", q)
	var in string
	fmt.Scanln(&in)
	in = strings.TrimSpace(strings.ToLower(in))
	return in == "y" || in == "yes"
}

type cleanedDoneMsg struct{}

func (m cleanModel) View() string {
	var b strings.Builder
	mode := ""
	if safety.DryRun() {
		mode = ui.StyleWarn.Render("  [DRY RUN]")
	}
	b.WriteString(ui.Header("DEFENESTRATE CLEAN", modeText(mode), max(m.width, 50)))

	if m.scanning {
		b.WriteString(ui.StyleDim.Render("Scanning cleanable locations…") + "\n")
		return b.String()
	}

	var totalChecked, heldBytes int64
	for i, f := range m.findings {
		if m.checked[i] {
			totalChecked += f.Bytes
		}
		heldBytes += f.HeldBytes
	}

	for i, f := range m.findings {
		box := "[ ]"
		style := lipgloss.NewStyle()
		if m.checked[i] {
			box = "[x]"
		}
		if i == m.cursor && !m.done {
			box = ui.StyleSel.Render(box)
			style = ui.StyleSel.Inline(true)
		}
		line := fmt.Sprintf("%s %-42s %s", box, f.Target.Label, ui.HumanBytes(f.Bytes))
		b.WriteString(style.Render(line) + "\n")
		if len(f.HeldPaths) > 0 {
			b.WriteString(ui.StyleDim.Render(fmt.Sprintf(
				"     ⚠ held for review: %d items (%s) — never auto-deleted",
				len(f.HeldPaths), ui.HumanBytes(f.HeldBytes))) + "\n")
		}
	}

	b.WriteString(ui.Rule() + "\n")
	switch {
	case m.running:
		fmt.Fprintf(&b, "%s Recycling category %d/%d…\n",
			ui.StyleGood.Render("…"), m.progressI+1, len(m.findings))
	case m.done:
		if safety.DryRun() {
			fmt.Fprintf(&b, "%s\n", ui.StyleWarn.Render(fmt.Sprintf("Dry-run plan: %s would be recycled.", ui.HumanBytes(totalChecked))))
		} else {
			fmt.Fprintf(&b, "%s\n", ui.StyleGood.Render(fmt.Sprintf("Space freed: %s (Recycle Bin)", ui.HumanBytes(m.freed))))
		}
		fmt.Fprintf(&b, "%s\n", ui.StyleDim.Render("Held items were not touched. Enter rescans · q quits."))
	default:
		fmt.Fprintf(&b, "Selected: %s | Held for review: %s\n",
			ui.Bold(ui.HumanBytes(totalChecked)), ui.HumanBytes(heldBytes))
	}
	b.WriteString(ui.StyleHelp.Render("↑↓ move · space toggle · a all · enter run · q quit"))
	return b.String()
}

func modeText(s string) string { return s }

// RunCleanTUI launches the interactive clean flow.
func RunCleanTUI() error {
	if _, err := tea.NewProgram(newCleanModel(safety.DryRun()), tea.WithAltScreen()).Run(); err != nil {
		return err
	}
	return nil
}
