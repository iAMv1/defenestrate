package monitor

import (
	"runtime"
	"sort"
	"sync"
	"time"

	gproc "github.com/shirou/gopsutil/v3/process"
)

// processSampler attributes CPU share by diffing cumulative CPU-time counters
// between ticks. First observation seeds the baseline: that tick reports no
// attribution (honest zero instead of a fake spike).
type processSampler struct {
	mu       sync.Mutex
	prev     map[int32]float64 // pid -> total CPU seconds
	lastTime time.Time
}

var procSampler = &processSampler{}

const topProcs = 6

// sampleTop returns the top consumers by CPU share over the elapsed window.
func sampleTop(limit int) []ProcStat {
	procSampler.mu.Lock()
	defer procSampler.mu.Unlock()

	now := time.Now()
	procs, err := gproc.Processes()
	if err != nil {
		return nil // cannot enumerate = report nothing, never fabricate
	}

	// First observation ever: seed the baseline, then take a second sample
	// after a short window so even one-shot invocations attribute honestly.
	if procSampler.lastTime.IsZero() {
		seed := map[int32]float64{}
		for _, p := range procs {
			if t, terr := p.Times(); terr == nil {
				seed[p.Pid] = t.User + t.System
			}
		}
		time.Sleep(300 * time.Millisecond)
		procs2, err2 := gproc.Processes()
		if err2 != nil {
			return nil
		}
		procSampler.prev = seed
		procSampler.lastTime = now
		now = time.Now()
		procs = procs2
	}

	cores := float64(runtime.NumCPU())
	curTimes := make(map[int32]float64, len(procs))
	type row struct {
		st ProcStat
		dt float64
	}
	var rows []row

	for _, p := range procs {
		times, err := p.Times()
		if err != nil {
			continue
		}
		total := times.User + times.System
		pid := p.Pid
		curTimes[pid] = total

		st := ProcStat{PID: pid}
		if name, nerr := p.Name(); nerr == nil {
			st.Name = name
		}
		if mp, merr := p.MemoryPercent(); merr == nil {
			st.MemPercent = float64(mp)
		}
		if mi, merr := p.MemoryInfo(); merr == nil {
			st.MemBytes = mi.RSS
		}
		if prev, ok := procSampler.prev[pid]; ok && !procSampler.lastTime.IsZero() {
			dt := total - prev
			if dt > 0 {
				st.CPUShare = dt / elapsedSeconds(procSampler.lastTime, now) / cores * 100
			}
		}
		rows = append(rows, row{st: st, dt: st.CPUShare})
		_ = rows[len(rows)-1].dt
	}

	sort.SliceStable(rows, func(a, b int) bool { return rows[a].st.CPUShare > rows[b].st.CPUShare })
	out := make([]ProcStat, 0, limit)
	for _, r := range rows {
		out = append(out, r.st)
		if len(out) >= limit {
			break
		}
	}

	procSampler.prev = curTimes
	procSampler.lastTime = now
	return out
}

func elapsedSeconds(from, to time.Time) float64 {
	d := to.Sub(from).Seconds()
	if d <= 0 {
		d = time.Since(from).Seconds()
	}
	if d <= 0 {
		d = 1
	}
	return d
}
