package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/iAMv1/defenestrate/internal/monitor"
	"github.com/iAMv1/defenestrate/internal/ui"
)

// statusModel is the live dashboard: bento cards, 1 Hz tick, sparkline
// history for CPU/memory/IO. Keys: q quit · p pause · c cycle core rows.
type statusModel struct {
	snap     *monitor.Snapshot
	hw       *monitor.HardwareExtras
	paused   bool
	width    int
	coreRows int // 2 / 4 / 8 / 0 = all
	tickN    int
	cpuHist  []float64
	memHist  []float64
	ioHist   []float64
	errText  string
}

type snapMsg struct {
	snap *monitor.Snapshot
	err  error
}

type hwMsg struct{ hw monitor.HardwareExtras }

type tickMsg struct{}

const histCap = 60 // one minute of samples

func newStatusModel() statusModel {
	p := loadPrefs()
	if p.CoreRows != 2 && p.CoreRows != 4 && p.CoreRows != 8 && p.CoreRows != 0 {
		p.CoreRows = 4
	}
	return statusModel{coreRows: p.CoreRows}
}

func (m statusModel) Init() tea.Cmd {
	return tea.Batch(takeSnapCmd(), hwProbeCmd(), tickCmd())
}

func takeSnapCmd() tea.Cmd {
	return func() tea.Msg {
		s, err := monitor.Take()
		return snapMsg{snap: s, err: err}
	}
}

// hwProbeCmd samples slow-moving hardware (battery/temp/GPU name) — one
// PowerShell round-trip, refreshed every 10 ticks, not every second.
func hwProbeCmd() tea.Cmd {
	return func() tea.Msg { return hwMsg{hw: monitor.ProbeHardware()} }
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m statusModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case hwMsg:
		m.hw = &msg.hw
	case snapMsg:
		if msg.err != nil {
			m.errText = msg.err.Error()
		} else {
			m.errText = ""
			m.snap = msg.snap
			m.cpuHist = appendHist(m.cpuHist, m.snap.CPU)
			m.memHist = appendHist(m.memHist, m.snap.Mem.UsedPercent)
			io := 0.0
			for _, d := range m.snap.Disks {
				io += d.ReadMBs + d.WriteMBs
			}
			m.ioHist = appendHist(m.ioHist, io)
		}
		return m, tickCmd()
	case tickMsg:
		if m.paused {
			return m, tickCmd()
		}
		m.tickN++
		cmds := []tea.Cmd{takeSnapCmd()}
		if m.tickN%10 == 0 {
			cmds = append(cmds, hwProbeCmd())
		}
		return m, tea.Batch(append(cmds, tickCmd())...)
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "p":
			m.paused = !m.paused
		case "c":
			switch m.coreRows {
			case 2:
				m.coreRows = 4
			case 4:
				m.coreRows = 8
			case 8:
				m.coreRows = 0 // all
			default:
				m.coreRows = 2
			}
			savePrefs(prefs{CoreRows: m.coreRows})
		}
	}
	return m, nil
}

func appendHist(hist []float64, v float64) []float64 {
	hist = append(hist, v)
	if len(hist) > histCap {
		hist = hist[1:]
	}
	return hist
}

func (m statusModel) View() string {
	var b strings.Builder
	b.WriteString(ui.Header("DEFENESTRATE STATUS", headerRight(m), max(m.width, 40)))
	if m.errText != "" {
		b.WriteString(ui.StyleBad.Render("sample error: "+m.errText) + "\n")
	}
	if m.snap == nil {
		b.WriteString(ui.StyleDim.Render("sampling…") + "\n")
		return b.String()
	}
	s := m.snap
	state := "live"
	if m.paused {
		state = ui.StyleWarn.Render("PAUSED")
	}
	b.WriteString(fmt.Sprintf("Health %s   %s   %d cores · up %s\n\n",
		ui.HealthText(s.Health), state, s.Cores, s.Uptime))

	w := cardWidth(m.width)

	// Card 1+2 side by side: CPU | MEMORY
	cpuCard := renderCPUCard(s, m.coreRows, w)
	memCard := renderMemCard(s, w)
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, cpuCard, memCard))
	b.WriteString("\n")

	// Hardware extras card (hidden entirely when machine provides nothing).
	if m.hw != nil && (m.hw.HasBattery() || m.hw.HasTemp() || m.hw.HasGPU()) {
		hw := m.hw
		var hb strings.Builder
		hb.WriteString(ui.Section("Hardware") + "\n")
		if hw.HasBattery() {
			state := "discharging"
			if hw.Charging {
				state = ui.StyleGood.Render("charging")
			}
			fmt.Fprintf(&hb, "  Battery %s %d%% (%s)\n",
				ui.BarColored(float64(hw.BatteryPercent)/100, 14), hw.BatteryPercent, state)
		}
		if hw.HasTemp() {
			ts := fmt.Sprint(hw.TempC)
			tempStyle := ui.StyleGood
			if hw.TempC >= 80 {
				tempStyle = ui.StyleBad
			} else if hw.TempC >= 65 {
				tempStyle = ui.StyleWarn
			}
			fmt.Fprintf(&hb, "  Temp    %s°C\n", tempStyle.Render(ts))
		}
		if hw.HasGPU() {
			line := "  GPU     " + gpuShort(hw.GPUName)
			if hw.GPUPercent >= 0 {
				line += fmt.Sprintf(" · %d%%", hw.GPUPercent)
			}
			if hw.GPUMemTotalMB > 0 {
				line += fmt.Sprintf(" · %d/%d MB", hw.GPUMemUsedMB, hw.GPUMemTotalMB)
			}
			hb.WriteString(line + "\n")
		}
		b.WriteString(hb.String())
	}

	// Sparklines row.
	b.WriteString(ui.Section("History (60s)"))
	fmt.Fprintf(&b, "  cpu  %s\n", ui.Sparkline(m.cpuHist, sparkWidth(m.width)))
	fmt.Fprintf(&b, "  mem  %s\n", ui.Sparkline(m.memHist, sparkWidth(m.width)))
	fmt.Fprintf(&b, "  io   %s MB/s\n", ui.Sparkline(m.ioHist, sparkWidth(m.width)))

	// Disk table with IO rates.
	fmt.Fprintf(&b, "\n%s\n", ui.Section("Disks"))
	for _, d := range s.Disks {
		bar := ui.BarColored(d.UsedPct, 20)
		fmt.Fprintf(&b, "  %-3s %s %5.1f%%  free %-9s read %6.1f  write %6.1f MB/s\n",
			d.Drive, bar, d.UsedPct*100, ui.HumanBytesU(d.Free), d.ReadMBs, d.WriteMBs)
	}

	// Network card (interfaces with any traffic in the window).
	if len(s.Net) > 0 {
		fmt.Fprintf(&b, "\n%s\n", ui.Section("Network"))
		for _, n := range s.Net {
			fmt.Fprintf(&b, "  %-18s ▼ %6.2f  ▲ %6.2f MB/s\n",
				truncateProc(n.Interface, 18), n.DownMBs, n.UpMBs)
		}
	}

	// Processes card — honest CPU-share attribution (Windows has no
	// per-process wattage API; share is the proxy and is labeled as such).
	if len(s.Processes) > 0 {
		fmt.Fprintf(&b, "\n%s\n", ui.Section("Processes (CPU share)"))
		for i, p := range s.Processes {
			bar := ui.Bar(p.CPUShare/100, 14)
			fmt.Fprintf(&b, "  %-24s %s %5.1f%%  mem %s\n",
				truncateProc(p.Name, 24), bar, p.CPUShare, ui.HumanBytesU(p.MemBytes))
			_ = i
		}
	}

	b.WriteString("\n" + ui.StyleHelp.Render("q quit · p pause · c cores ("+coreLabel(m.coreRows)+")"))
	return b.String()
}

func truncateProc(name string, n int) string {
	if len(name) <= n {
		return name
	}
	return name[:n-1] + "…"
}

func renderCPUCard(s *monitor.Snapshot, coreRows, width int) string {
	var b strings.Builder
	b.WriteString(ui.Section("CPU") + "\n")
	fmt.Fprintf(&b, "  Total %s %5.1f%%\n", ui.BarColored(s.CPU/100, barWidth(width)), s.CPU)
	if len(s.Detail.PerCore) == 0 {
		return b.String()
	}
	shown := s.Detail.PerCore
	if coreRows > 0 && len(shown) > coreRows {
		shown = shown[:coreRows]
	}
	for i, pc := range shown {
		fmt.Fprintf(&b, "  C%-3d %s %5.1f%%\n", i+1, ui.BarColored(pc/100, barWidth(width)), pc)
	}
	if hidden := len(s.Detail.PerCore) - len(shown); hidden > 0 {
		fmt.Fprintf(&b, "  %s\n", ui.StyleDim.Render(fmt.Sprintf("… %d more (c cycles)", hidden)))
	}
	return b.String()
}

func renderMemCard(s *monitor.Snapshot, width int) string {
	var b strings.Builder
	b.WriteString(ui.Section("Memory") + "\n")
	fmt.Fprintf(&b, "  Used  %s %5.1f%%\n", ui.BarColored(s.Mem.UsedPercent/100, barWidth(width)), s.Mem.UsedPercent)
	fmt.Fprintf(&b, "  Total %-9s\n", ui.HumanBytesU(s.Mem.Total))
	fmt.Fprintf(&b, "  Avail %s\n", ui.HumanBytesU(s.Mem.Total-s.Mem.Used))
	return b.String()
}

func cardWidth(termW int) int {
	if termW >= 100 {
		return termW / 2
	}
	return maxInt2(termW, 36)
}

func barWidth(termW int) int {
	if termW >= 100 {
		return 18
	}
	return 24
}

func sparkWidth(termW int) int {
	switch {
	case termW >= 120:
		return 60
	case termW >= 80:
		return 40
	default:
		return 30
	}
}

func coreLabel(n int) string {
	if n == 0 {
		return "all"
	}
	return fmt.Sprint(n)
}

func headerRight(m statusModel) string {
	if m.paused {
		return "paused"
	}
	return time.Now().Format("15:04:05")
}

// gpuShort trims vendor noise from adapter names.
func gpuShort(name string) string {
	name = strings.TrimSpace(name)
	for _, p := range []string{"NVIDIA ", "AMD ", "Intel(R) "} {
		if strings.HasPrefix(name, p) {
			name = strings.TrimPrefix(name, p)
		}
	}
	if len(name) > 28 {
		name = name[:27] + "…"
	}
	return name
}

func maxInt2(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// RunStatusTUI launches the interactive dashboard. Returns an error only if
// the program itself fails; normal quits return nil.
func RunStatusTUI() error {
	if _, err := tea.NewProgram(newStatusModel(), tea.WithAltScreen()).Run(); err != nil {
		return err
	}
	return nil
}
