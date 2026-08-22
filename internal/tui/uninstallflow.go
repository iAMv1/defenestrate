package tui

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/iAMv1/defenestrate/internal/apps"
	"github.com/iAMv1/defenestrate/internal/safety"
	"github.com/iAMv1/defenestrate/internal/ui"
)

// uninstallModel: searchable program list → evidence plan → execute → done.
type uninstallModel struct {
	all      []apps.App
	filter   string
	filterOn bool
	cursor   int
	phase    int // -1 loading · 0 list · 1 plan · 2 running · 3 done
	target   *apps.App
	sweep    apps.SweepResult
	lb       int64
	logs     []string
	freed    int64
	width    int
}

type appsLoadedMsg struct{ list []apps.App }
type executedMsg struct {
	logs  []string
	freed int64
}

func newUninstallModel() uninstallModel { return uninstallModel{phase: -1} }

func loadAppsCmd() tea.Cmd {
	return func() tea.Msg {
		list, _ := apps.ListInstalled()
		return appsLoadedMsg{list: list}
	}
}

func uninstallAllCmd(app *apps.App, sweep *apps.SweepResult) tea.Cmd {
	return func() tea.Msg {
		var logs []string
		var freed int64
		if app.Store {
			if err := apps.RemoveAppx(app.PackageFullName); err != nil {
				logs = append(logs, ui.StyleBad.Render("✗ Remove-AppxPackage: "+err.Error()))
			} else {
				logs = append(logs, ui.StyleGood.Render("✓ Store package removed"))
			}
		} else if cmd := firstNonEmptyStr(app.Quiet, silentFlag(app.Uninstall)); cmd != "" {
			if err := apps.RunUninstallCommand(cmd); err != nil {
				logs = append(logs, ui.StyleBad.Render("✗ vendor uninstaller: "+err.Error()))
			} else {
				logs = append(logs, ui.StyleGood.Render("✓ vendor uninstaller completed"))
			}
		} else {
			logs = append(logs, ui.StyleWarn.Render("! no uninstaller registered"))
		}
		for _, d := range sweep.Evidence {
			size := dirSizeTUI(d)
			if err := safety.Recycle([]string{d}); err != nil {
				logs = append(logs, ui.StyleBad.Render(fmt.Sprintf("✗ skip %s (%v)", d, err)))
				continue
			}
			freed += size
			safety.Logf("uninstall-recycle", d, size)
			logs = append(logs, ui.StyleGood.Render(fmt.Sprintf("✓ recycled %s", d)))
		}
		for _, k := range sweep.RegKeys {
			logs = append(logs, ui.StyleDim.Render("regkey left for manual review: "+k))
		}
		for _, s := range sweep.StartupShortcuts {
			if err := safety.Recycle([]string{s}); err == nil {
				freed += 0
				logs = append(logs, ui.StyleGood.Render("✓ startup shortcut recycled: "+s))
			}
		}
		return executedMsg{logs: logs, freed: freed}
	}
}

func (m uninstallModel) Init() tea.Cmd { return loadAppsCmd() }

func (m uninstallModel) filtered() []apps.App {
	if m.filter == "" {
		return m.all
	}
	n := strings.ToLower(m.filter)
	var out []apps.App
	for _, a := range m.all {
		if strings.Contains(strings.ToLower(a.Name), n) {
			out = append(out, a)
		}
	}
	return out
}

func (m uninstallModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case appsLoadedMsg:
		m.phase = 0
		m.all = msg.list
	case executedMsg:
		m.logs = msg.logs
		m.freed = msg.freed
		m.phase = 3
	case tea.KeyMsg:
		switch m.phase {
		case -1, 2:
			return m, nil
		case 3:
			switch msg.String() {
			case "enter", "esc":
				m.phase = 0
				m.cursor = 0
				m.filter, m.filterOn = "", false
				m.target, m.sweep, m.logs, m.freed = nil, apps.SweepResult{}, nil, 0
			case "q":
				return m, tea.Quit
			}
			return m, nil
		case 1:
			lower := strings.ToLower(msg.String())
			if lower == "y" {
				if safety.DryRun() {
					m.phase = 3
					m.logs = []string{ui.StyleWarn.Render("[dry-run] nothing executed")}
					return m, nil
				}
				m.phase = 2
				return m, uninstallAllCmd(m.target, &m.sweep)
			}
			if lower == "n" || lower == "esc" {
				m.phase = 0
			}
			return m, nil
		case 0:
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit
			case "esc":
				m.filterOn, m.filter = false, ""
			case "/":
				m.filterOn = true
			case "up", "k":
				if m.cursor > 0 {
					m.cursor--
				}
			case "down", "j":
				if m.cursor < len(m.filtered())-1 {
					m.cursor++
				}
			case "enter":
				list := m.filtered()
				if m.cursor < len(list) {
					app := list[m.cursor]
					m.target = &app
					m.sweep = apps.SweepLeftoversFor(&app)
					m.lb = 0
					for _, d := range m.sweep.Evidence {
						m.lb += dirSizeTUI(d)
					}
					m.phase = 1
				}
			default:
				if m.filterOn && len(msg.String()) == 1 {
					m.filter += msg.String()
					m.cursor = 0
				} else if m.filterOn && msg.String() == "backspace" && len(m.filter) > 0 {
					m.filter = m.filter[:len(m.filter)-1]
					m.cursor = 0
				}
			}
		}
	}
	return m, nil
}

func (m uninstallModel) View() string {
	var b strings.Builder
	b.WriteString(ui.Header("DEFENESTRATE UNINSTALL", fmt.Sprintf("%d programs", len(m.filtered())), max(m.width, 50)))

	switch m.phase {
	case -1:
		return b.String() + ui.StyleDim.Render("Loading installed programs…")+"\n"
	case 2:
		b.WriteString("\n" + ui.StyleGood.Render(fmt.Sprintf("Uninstalling %q — vendor uninstaller may open windows…", m.target.Name)) + "\n")
		return b.String()
	case 1:
		b.WriteString(uninstallPlanView(m))
		b.WriteString("\n" + ui.StyleHelp.Render("y run · n cancel"))
		return b.String()
	case 3:
		b.WriteString(ui.Section("Done") + "\n")
		for _, l := range m.logs {
			b.WriteString(l + "\n")
		}
		fmt.Fprintf(&b, "\n%s\n", ui.StyleDim.Render(fmt.Sprintf("freed %s · enter back · q quit", ui.HumanBytes(m.freed))))
		return b.String()
	}

	if m.filterOn {
		fmt.Fprintf(&b, "filter: %s_\n\n", m.filter)
	} else {
		fmt.Fprintf(&b, "%s\n\n", ui.StyleHelp.Render("/ filter · ↑↓ move · enter select · q quit"))
	}
	list := m.filtered()
	start, end := window(len(list), m.cursor, 20)
	for i := start; i < end; i++ {
		a := list[i]
		line := fmt.Sprintf("%3d. %-46s %-11s %s",
			i+1, truncateU(a.Name, 46), truncateU(a.Version, 11), sizeOrDash(a.SizeKB))
		tag := ""
		if a.Store {
			tag = ui.StyleDim.Render(" [store]")
		}
		if i == m.cursor {
			b.WriteString(ui.StyleSel.Render("▶ "+line) + tag + "\n")
		} else {
			b.WriteString("  " + line + tag + "\n")
		}
	}
	return b.String()
}

func uninstallPlanView(m uninstallModel) string {
	var b strings.Builder
	title := ui.Section("Plan for " + m.target.Name)
	if safety.DryRun() {
		title += ui.StyleWarn.Render("  [DRY RUN]")
	}
	b.WriteString(title + "\n")
	if m.target.Store {
		fmt.Fprintf(&b, "  Remove-AppxPackage '%s'\n", m.target.PackageFullName)
		b.WriteString("  No filesystem sweep — Windows services the WindowsApps tree.\n")
		return b.String()
	}
	fmt.Fprintf(&b, "  Vendor uninstaller: %s\n",
		firstNonEmptyStr(m.target.Quiet, m.target.Uninstall, "(none registered)"))
	if len(m.sweep.Evidence) == 0 {
		b.WriteString("  No registry-evidence folders found.\n")
	} else {
		fmt.Fprintf(&b, "  Recycle %d evidence locations (%s):\n", len(m.sweep.Evidence), ui.HumanBytes(m.lb))
		for _, d := range m.sweep.Evidence {
			flag := ""
			if safety.Check(d) != nil {
				flag = ui.StyleWarn.Render("   ⚠ protected — skipped")
			}
			fmt.Fprintf(&b, "    %10s  %s%s\n", ui.HumanBytes(dirSizeTUI(d)), d, flag)
		}
	}
	if len(m.sweep.Review) > 0 {
		fmt.Fprintf(&b, "  Review-only (never touched): %d folders\n", len(m.sweep.Review))
	}
	return b.String()
}

func window(total, cursor, size int) (int, int) {
	if total <= size {
		return 0, total
	}
	start := cursor - size/2
	if start < 0 {
		start = 0
	}
	if start+size > total {
		start = total - size
	}
	return start, start + size
}

func truncateU(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func sizeOrDash(kb uint64) string {
	if kb == 0 {
		return ui.StyleDim.Render("—")
	}
	return ui.HumanBytesU(kb * 1024)
}

func dirSizeTUI(p string) int64 {
	var total int64
	filepath.WalkDir(p, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if info, e2 := d.Info(); e2 == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func silentFlag(u string) string {
	if u == "" {
		return ""
	}
	lower := strings.ToLower(u)
	switch {
	case strings.Contains(lower, "msiexec"):
		return strings.Replace(u, "/I", "/X", 1) + " /qn /norestart"
	case strings.Contains(lower, "unins"):
		return u + " /S"
	default:
		return u
	}
}

// RunUninstallTUI launches the interactive uninstaller.
func RunUninstallTUI() error {
	if _, err := tea.NewProgram(newUninstallModel(), tea.WithAltScreen()).Run(); err != nil {
		return err
	}
	return nil
}
