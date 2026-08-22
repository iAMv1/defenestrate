// Package hud runs a system-tray widget: live CPU/RAM next to the clock,
// tooltip detail, and a small menu. It is the Windows answer to Mole's
// macOS menu-bar HUD.
package hud

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/getlantern/systray"

	"github.com/iAMv1/defenestrate/internal/monitor"
)

// Run blocks until the user quits from the tray menu.
func Run() error {
	systray.Run(onReady, onExit)
	return nil
}

func onReady() {
	systray.SetIcon(iconBytes())
	systray.SetTitle("starting…")
	systray.SetTooltip("DEFENESTRATE — system monitor")

	mStatus := systray.AddMenuItem("Open dashboard", "Launch the full status dashboard")
	mClean := systray.AddMenuItem("Deep clean (dry-run)", "Preview reclaimable space")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Stop the tray widget")

	go func() {
		for range mStatus.ClickedCh {
			exe, _ := os.Executable()
			exec.Command(exe, "status").Start()
		}
	}()
	go func() {
		for range mClean.ClickedCh {
			exe, _ := os.Executable()
			c := exec.Command(exe, "clean", "--dry-run")
			c.Start()
		}
	}()
	go func() {
		for range mQuit.ClickedCh {
			systray.Quit()
		}
	}()

	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for range tick.C {
		s, err := monitor.Take()
		if err != nil {
			continue
		}
		title := fmt.Sprintf("CPU %2.0f%%  RAM %2.0f%%", s.CPU, s.Mem.UsedPercent)
		systray.SetTitle(title)
		var tt strings.Builder
		fmt.Fprintf(&tt, "DEFENESTRATE\nCPU %.1f%%\nMemory %.1f%% (%s / %s)",
			s.CPU, s.Mem.UsedPercent,
			human(s.Mem.Total-s.Mem.Used), human(s.Mem.Total))
		for _, d := range s.Disks {
			fmt.Fprintf(&tt, "\n%s %.0f%% used (free %s)", d.Drive, d.UsedPct*100, human(d.Free))
		}
		if s.Health < 50 {
			tt.WriteString("\n⚠ health low")
		}
		systray.SetTooltip(tt.String())
	}
	_ = mStatus
	_ = mClean
	_ = mQuit
}

func onExit() {}

func human(b uint64) string {
	const u = 1024
	if b < u {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(u), 0
	for n := b / u; n >= u; n /= u {
		div *= u
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
