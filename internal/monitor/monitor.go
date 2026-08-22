// Package monitor renders the live CPU / memory / disk dashboard with a
// mole-style health score. `--watch` refreshes every second; `--json` emits
// one machine-readable snapshot.
package monitor

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	gnet "github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	gmem "github.com/shirou/gopsutil/v3/mem"
	ghost "github.com/shirou/gopsutil/v3/host"

	"github.com/iAMv1/defenestrate/internal/ui"
)

// Snapshot is one point-in-time reading of the machine.
// ProcStat attributes resource share to one running process.
// Windows exposes no per-process wattage API — CPU share IS the honest
// power proxy here, labeled as share rather than watts.
type ProcStat struct {
	PID        int32   `json:"pid"`
	Name       string  `json:"name"`
	CPUShare   float64 `json:"cpu_share_percent"`
	MemPercent float64 `json:"mem_percent"`
	MemBytes   uint64  `json:"mem_bytes"`
}

type Snapshot struct {
	Health    int         `json:"health_score"`
	CPU       float64     `json:"cpu_percent"`
	Cores     int         `json:"cores"`
	Mem       MemStats    `json:"memory"`
	Disks     []DiskStats `json:"disks"`
	Net       []NetStat   `json:"network,omitempty"`
	Uptime    string      `json:"uptime"`
	Detail    CPUDetail   `json:"-"`
	Processes []ProcStat  `json:"processes,omitempty"` // top consumers by CPU
}

// NetStat is one interface's live throughput over the sample window.
type NetStat struct {
	Interface string  `json:"interface"`
	DownMBs   float64 `json:"down_mbs"`
	UpMBs     float64 `json:"up_mbs"`
}

// CPUDetail carries per-core readings (excluded from JSON; the flat
// cpu_percent is the automation surface).
type CPUDetail struct {
	PerCore []float64
}

type MemStats struct {
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	UsedPercent float64 `json:"used_percent"`
}

type DiskStats struct {
	Drive     string  `json:"drive"`
	UsedPct   float64 `json:"used_percent"`
	Total     uint64  `json:"total"`
	Free      uint64  `json:"free"`
	ReadMBs   float64 `json:"read_mbs"`
	WriteMBs  float64 `json:"write_mbs"`
}

type cpuDetail struct {
	PerCore []float64
}

func fixedDrives() []string {
	var out []string
	for c := 'C'; c <= 'Z'; c++ {
		root := string(c) + `:\`
		if _, err := os.Stat(root); err == nil {
			out = append(out, root)
		}
	}
	return out
}

// Take samples CPU over a short window and assembles the snapshot.
func Take() (*Snapshot, error) {
	total, err := cpu.Percent(400*time.Millisecond, false)
	if err != nil {
		return nil, err
	}
	perCore, _ := cpu.Percent(0, true)
	vm, err := gmem.VirtualMemory()
	if err != nil {
		return nil, err
	}
	snap := &Snapshot{
		CPU:    firstOr(total),
		Cores:  runtime.NumCPU(),
		Detail: CPUDetail{PerCore: perCore},
		Mem:    MemStats{Total: vm.Total, Used: vm.Used, UsedPercent: vm.UsedPercent},
	}
	up, _ := ghost.Uptime()
	snap.Uptime = humanUptime(up)

	before, _ := disk.IOCounters()
	beforeTime := time.Now()
	time.Sleep(300 * time.Millisecond)
	after, _ := disk.IOCounters()
	elapsed := time.Since(beforeTime).Seconds()
	for _, d := range fixedDrives() {
		pct, tot, free := ui.VolumeStats(d)
		ds := DiskStats{Drive: strings.TrimSuffix(d, `\`), UsedPct: pct, Total: tot, Free: free}
		// gopsutil names Windows counters like "C:".
		want := strings.ToLower(ds.Drive) // "c"
		for name, a := range after {
			b, ok := before[name]
			if !ok {
				continue
			}
			if strings.ToLower(strings.TrimSuffix(name, ":")) == want && elapsed > 0 {
				ds.ReadMBs = float64(a.ReadBytes-b.ReadBytes) / 1e6 / elapsed
				ds.WriteMBs = float64(a.WriteBytes-b.WriteBytes) / 1e6 / elapsed
				break
			}
		}
		snap.Disks = append(snap.Disks, ds)
	}
	snap.Health = score(snap)
	snap.Processes = sampleTop(topProcs)
	snap.Net = netRates()
	return snap, nil
}

// netRates diffs interface counters over the standard sample window.
func netRates() []NetStat {
	before, err0 := gnet.IOCounters(true)
	t0 := time.Now()
	time.Sleep(300 * time.Millisecond)
	after, err1 := gnet.IOCounters(true)
	if err0 != nil || err1 != nil {
		return nil
	}
	secs := time.Since(t0).Seconds()
	if secs <= 0 {
		secs = 0.3
	}
	var out []NetStat
	idx := make(map[string]int, len(before))
	for i, b := range before {
		idx[strings.ToLower(b.Name)] = i
	}
	for _, a := range after {
		i, ok := idx[strings.ToLower(a.Name)]
		if !ok {
			continue
		}
		b := before[i]
		lifetime := a.BytesRecv + a.BytesSent
		idle := a.BytesRecv == b.BytesRecv && a.BytesSent == b.BytesSent
		if lifetime < 1<<20 && idle {
			continue // loopback-ish / never-used interfaces stay hidden
		}
		out = append(out, NetStat{
			Interface: a.Name,
			DownMBs:   float64(a.BytesRecv-b.BytesRecv) / 1e6 / secs,
			UpMBs:     float64(a.BytesSent-b.BytesSent) / 1e6 / secs,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DownMBs+out[i].UpMBs > out[j].DownMBs+out[j].UpMBs })
	if len(out) > 4 {
		out = out[:4]
	}
	return out
}

var _ = ghost.Uptime // host package retained for future hardware info

func score(s *Snapshot) int {
	cpuPen := int(s.CPU / 5)
	memPen := int(s.Mem.UsedPercent / 5)
	diskPen := 0
	for _, d := range s.Disks {
		if p := int(d.UsedPct / 2); p > diskPen {
			diskPen = p
		}
	}
	h := 100 - cpuPen - memPen - diskPen
	if h < 0 {
		h = 0
	}
	return h
}

func healthColor(h int) string {
	switch {
	case h >= 80:
		return ui.Good(fmt.Sprintf("%d", h))
	case h >= 50:
		return ui.Warn(fmt.Sprintf("%d", h))
	default:
		return ui.Bad(fmt.Sprint(h))
	}
}

// Render draws the dashboard card layout.
func Render(s *Snapshot) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s  Health %s   %d cores · up %s\n\n",
		ui.Title("DEFENESTRATE status"), healthColor(s.Health), s.Cores, s.Uptime)

	fmt.Fprintf(&b, "%s\n", ui.Section("CPU"))
	fmt.Fprintf(&b, "  Total %s %5.1f%%\n", ui.Bar(s.CPU/100, 24), s.CPU)
	for i, pc := range s.Detail.PerCore {
		if i >= 4 {
			fmt.Fprintf(&b, "      … %d more cores\n", len(s.Detail.PerCore)-4)
			break
		}
		fmt.Fprintf(&b, "  Core %-2d %s %5.1f%%\n", i+1, ui.Bar(pc/100, 24), pc)
	}

	fmt.Fprintf(&b, "\n%s\n", ui.Section("Memory"))
	fmt.Fprintf(&b, "  Used  %s %5.1f%%\n", ui.Bar(s.Mem.UsedPercent/100, 24), s.Mem.UsedPercent)
	fmt.Fprintf(&b, "  Total %s / Avail %s\n", ui.HumanBytesU(s.Mem.Total), ui.HumanBytesU(s.Mem.Total-s.Mem.Used))

	fmt.Fprintf(&b, "\n%s\n", ui.Section("Disks"))
	for _, d := range s.Disks {
		fmt.Fprintf(&b, "  %-3s %s %5.1f%%  free %s   read %.1f MB/s  write %.1f MB/s\n",
			d.Drive, ui.Bar(d.UsedPct/100, 20), d.UsedPct*100,
			ui.HumanBytesU(d.Free), d.ReadMBs, d.WriteMBs)
	}
	return b.String()
}

// Run implements the NON-interactive output paths for
// `DEFENESTRATE status [--watch] [--json]`. The interactive terminal dashboard
// lives in tui.RunStatusTUI; main.go routes based on TTY detection.
func Run(args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	watch := fs.Bool("watch", false, "refresh every second")
	jsonOut := fs.Bool("json", false, "one JSON snapshot")
	if err := fs.Parse(args); err != nil {
		return err
	}

	emit := func() error {
		s, err := Take()
		if err != nil {
			return err
		}
		if *jsonOut {
			return json.NewEncoder(os.Stdout).Encode(s)
		}
		if *watch {
			fmt.Printf("\033[H\033[2J%s", Render(s))
			return nil
		}
		fmt.Print(Render(s))
		return nil
	}
	if err := emit(); err != nil {
		return err
	}
	if *watch && *jsonOut {
		// NDJSON time series: one object per line, forever. Pipe into jq.
		for range time.Tick(time.Second) {
			if err := emit(); err != nil {
				return err
			}
		}
		return nil
	}
	if *watch {
		for range time.Tick(time.Second) {
			if err := emit(); err != nil {
				return err
			}
		}
	}
	return nil
}

func firstOr(v []float64) float64 {
	if len(v) > 0 {
		return v[0]
	}
	return 0
}

func humanUptime(secs uint64) string {
	d := secs / 86400
	h := (secs % 86400) / 3600
	m := (secs % 3600) / 60
	if d > 0 {
		return fmt.Sprintf("%dd %dh %dm", d, h, m)
	}
	return fmt.Sprintf("%dh %dm", h, m)
}
