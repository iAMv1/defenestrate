// Package optimize runs bounded, explainable maintenance tasks. Modeled on
// Mole's optimize contract: every task states what it does before it does it,
// admin-required tasks are skipped (never silently elevated), dry-run prints
// the plan and touches nothing.
package optimize

import (
	"flag"
	"fmt"
	"strings"

	"golang.org/x/sys/windows"

	"github.com/iAMv1/defenestrate/internal/safety"
	"github.com/iAMv1/defenestrate/internal/ui"
)

// Task is one bounded maintenance action.
type Task struct {
	Name        string
	Desc        string
	NeedsAdmin  bool
	Destructive bool // asks confirmation even with --yes
	Run         func() error
}

func isAdmin() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

// tasks is the data-only registry. Adding maintenance means adding a row.
var tasks = []Task{
	{
		Name: "flush-dns",
		Desc: "Flush the DNS resolver cache",
		Run:  safety.FlushDNS,
	},
	{
		Name: "arp-flush",
		Desc: "Clear the ARP table",
		NeedsAdmin: true,
		Run: func() error {
			return ps("arp -d *")
		},
	},
	{
		Name: "empty-recycle-bin",
		Desc: "Empty the Recycle Bin (all drives)",
		NeedsAdmin: false,
		Destructive: true,
		Run: func() error {
			return ps("Clear-RecycleBin -Force -ErrorAction SilentlyContinue")
		},
	},
	{
		Name: "restart-search",
		Desc: "Restart Windows Search service (fixes stuck indexing)",
		NeedsAdmin: true,
		Run: func() error {
			if err := ps("Restart-Service wsearch -Force"); err != nil {
				return err
			}
			return nil
		},
	},
	{
		Name: "resync-perf-counters",
		Desc: "Rebuild performance counter settings",
		NeedsAdmin: true,
		Run: func() error {
			return ps("winmgmt /resyncperf")
		},
	},
}

// healthReport is read-only diagnostics printed alongside the plan.
func healthReport() {
	fmt.Println(ui.Section("System report (read-only)"))
	if pendingReboot() {
		fmt.Println("  " + ui.Warn("A reboot is pending — some fixes won't apply until restart."))
	} else {
		fmt.Println("  " + ui.Good("No pending reboot."))
	}
	fmt.Println("  Uptime: " + uptimeString())
}

func Run(args []string) error {
	fs := flag.NewFlagSet("optimize", flag.ContinueOnError)
	yes := fs.Bool("y", false, "do not ask for confirmation")
	all := fs.Bool("all", false, "include destructive tasks (Recycle Bin)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	admin := isAdmin()
	fmt.Println(ui.Title("DEFENESTRATE optimize") + ui.If(safety.DryRun(), ui.Warn("  [DRY RUN]")))
	fmt.Println(ui.Dim("elevated="+boolStr(admin)+"  (admin tasks are skipped without elevation)"))
	healthReport()

	fmt.Println(ui.Section("Planned tasks"))
	var runnable []Task
	for _, t := range tasks {
		line := "  [ ] " + pad(t.Name, 22) + t.Desc
		switch {
		case t.NeedsAdmin && !admin:
			fmt.Println(line + "  " + ui.Dim("(needs admin — will skip)"))
		case t.Destructive && !*all:
			fmt.Println(line + "  " + ui.Dim("(destructive — run with --all to include)"))
		default:
			fmt.Println(line)
			runnable = append(runnable, t)
		}
	}
	if safety.DryRun() {
		fmt.Println(ui.Warn("\nDry run — nothing executed."))
		return nil
	}
	if len(runnable) == 0 {
		fmt.Println(ui.Good("\nNothing to do."))
		return nil
	}

	destructive := false
	for _, t := range runnable {
		if t.Destructive {
			destructive = true
		}
	}
	if destructive && !*yes && !confirmOpt("Include destructive tasks (Recycle Bin)?") {
		var keep []Task
		for _, t := range runnable {
			if !t.Destructive {
				keep = append(keep, t)
			}
		}
		runnable = keep
	}

	fmt.Println()
	okCount, skipCount := 0, 0
	for _, t := range runnable {
		fmt.Printf("  %s %-22s ", ui.Dim("…"), t.Name)
		if t.NeedsAdmin && !admin {
			fmt.Println(ui.Dim("skipped (needs admin)"))
			skipCount++
			continue
		}
		if err := t.Run(); err != nil {
			fmt.Println(ui.Bad("failed:", err))
			skipCount++
			continue
		}
		fmt.Println(ui.Check)
		okCount++
		safety.Logf("optimize", t.Name, 0)
	}
	fmt.Println(ui.Rule())
	fmt.Printf("%s applied · %d skipped/failed\n", ui.Bold(fmt.Sprint(okCount)), skipCount)
	return nil
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func pad(s string, n int) string {
	for len(s) < n {
		s += " "
	}
	return s
}

func confirmOpt(q string) bool {
	fmt.Printf("%s [y/N] ", q)
	var in string
	fmt.Scanln(&in)
	in = strings.TrimSpace(strings.ToLower(in))
	return in == "y" || in == "yes"
}
