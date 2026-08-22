package monitor

import (
	"os/exec"
	"strconv"
	"strings"
)

// HardwareExtras are best-effort readings: every field degrades to "hidden"
// when the machine or tooling doesn't provide it. Nothing here is required
// for the dashboard to render.
type HardwareExtras struct {
	BatteryPercent int    // -1 = absent (desktop)
	Charging       bool
	TempC          int    // -1 = no thermal zone readable
	GPUName        string // "" = none detected
	GPUPercent     int    // -1 = no nvidia-smi
	GPUMemUsedMB   int
	GPUMemTotalMB  int
}

// ProbeHardware gathers battery + thermal + GPU in one PowerShell pass,
// then nvidia-smi if present. Errors leave zero values; callers hide.
func ProbeHardware() HardwareExtras {
	h := HardwareExtras{BatteryPercent: -1, TempC: -1, GPUPercent: -1}

	// One PS round-trip for battery + thermal + GPU name.
	script := `$b = Get-CimInstance Win32_Battery | Select-Object -First 1;
$t = Get-CimInstance -Namespace root/wmi -ClassName MSAcpi_ThermalZoneTemperature -ErrorAction SilentlyContinue | Select-Object -First 1;
$g = Get-CimInstance Win32_VideoController | Select-Object -First 1 -ExpandProperty Name;
$bPct = if ($b) { [int]$b.EstimatedChargeRemaining } else { -1 };
$bChg = if ($b -and $b.BatteryStatus -eq 2) { $true } else { $false };
$tC = if ($t) { [int](($t.CurrentTemperature / 10) - 273.15) } else { -1 };
"BP=$bPct|BC=$bChg|TC=$tC|GPU=$g"`
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err == nil {
		for _, part := range strings.Split(strings.TrimSpace(string(out)), "|") {
			kv := strings.SplitN(part, "=", 2)
			if len(kv) != 2 {
				continue
			}
			switch kv[0] {
			case "BP":
				if v, e := strconv.Atoi(kv[1]); e == nil && v >= 0 {
					h.BatteryPercent = v
				}
			case "BC":
				h.Charging = kv[1] == "True"
			case "TC":
				if v, e := strconv.Atoi(kv[1]); e == nil && v > 0 && v < 120 {
					h.TempC = v
				}
			case "GPU":
				if len(kv[1]) > 3 {
					h.GPUName = kv[1]
				}
			}
		}
	}

	// nvidia-smi utilization/memory when the driver ships it.
	if smiOut, serr := exec.Command("nvidia-smi",
		"--query-gpu=utilization.gpu,memory.used,memory.total",
		"--format=csv,noheader,nounits").Output(); serr == nil {
		fields := strings.Split(strings.TrimSpace(string(smiOut)), ", ")
		if len(fields) >= 3 {
			if v, e := strconv.Atoi(fields[0]); e == nil {
				h.GPUPercent = v
			}
			if v, e := strconv.Atoi(fields[1]); e == nil {
				h.GPUMemUsedMB = v
			}
			if v, e := strconv.Atoi(fields[2]); e == nil {
				h.GPUMemTotalMB = v
			}
		}
	}
	return h
}

// HasBattery reports whether a battery was detected.
func (h HardwareExtras) HasBattery() bool { return h.BatteryPercent >= 0 }

// HasTemp reports whether a thermal reading exists.
func (h HardwareExtras) HasTemp() bool { return h.TempC >= 0 }

// HasGPU reports whether a dedicated GPU query path exists.
func (h HardwareExtras) HasGPU() bool { return h.GPUName != "" }
