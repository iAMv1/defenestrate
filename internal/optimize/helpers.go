package optimize

import (
	"fmt"
	"os/exec"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// ps runs one PowerShell command.
func ps(cmd string) error {
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", cmd).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// pendingReboot probes the two canonical Windows markers.
func pendingReboot() bool {
	for _, path := range []string{
		`SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\RebootPending`,
		`SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate\Auto Update\RebootRequired`,
	} {
		if _, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE); err == nil {
			return true
		}
		if _, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE); err == nil {
			return true
		}
	}
	return false
}

func uptimeString() string {
	// Total seconds avoids all timezone ambiguity between PowerShell and Go.
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive",
		"-Command", "[int][double]::Parse(([DateTimeOffset]::Now - (Get-CimInstance Win32_OperatingSystem).LastBootUpTime).TotalSeconds.ToString('0'))").Output()
	if err != nil {
		return "unknown"
	}
	var secs float64
	if _, perr := fmt.Sscanf(strings.TrimSpace(string(out)), "%f", &secs); perr != nil || secs < 0 {
		return "unknown"
	}
	d := int(secs) / 86400
	h := (int(secs) % 86400) / 3600
	m := (int(secs) % 3600) / 60
	return fmt.Sprintf("%dd %dh %dm", d, h, m)
}
