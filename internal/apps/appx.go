package apps

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// Appx listing via PowerShell. Store apps have no Uninstall registry key, so
// they are invisible to ListInstalled â€” this fills that gap.

type appxRow struct {
	Name             string `json:"Name"`
	Version          string `json:"Version"`
	Publisher        string `json:"Publisher"`
	PackageFullName  string `json:"PackageFullName"`
	InstallLocation  string `json:"InstallLocation"`
	NonRemovable     bool   `json:"NonRemovable"`
}

// listAppx returns user-installed Store apps (frameworks and staged
// packages excluded by -NonRemovable filtering below).
func listAppx() ([]App, error) {
	script := "Get-AppxPackage | Where-Object { -not $_.IsFramework -and -not $_.NonRemovable } | " +
		"Select-Object DisplayName, Version, Publisher, PackageFullName, InstallLocation, NonRemovable | " +
		"ConvertTo-Json -Compress"
	raw, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		return nil, fmt.Errorf("Get-AppxPackage: %w", err)
	}
	trimmed := strings.TrimSpace(strings.TrimPrefix(string(raw), "\ufeff"))
	if trimmed == "" {
		return nil, nil
	}
	var rows []appxRow
	if strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal([]byte(trimmed), &rows); err != nil {
			return nil, err
		}
	} else {
		var r appxRow
		if err := json.Unmarshal([]byte(trimmed), &r); err != nil {
			return nil, err
		}
		rows = []appxRow{r}
	}
	var apps []App
	for _, r := range rows {
		if r.PackageFullName == "" {
			continue
		}
		name := r.Name
		if strings.TrimSpace(name) == "" {
			name = appxNameFromFull(r.PackageFullName)
		}
		if strings.TrimSpace(name) == "" {
			continue
		}
		apps = append(apps, App{
			Name:            name,
			Version:         r.Version,
			Publisher:       r.Publisher,
			Store:           true,
			PackageFullName: r.PackageFullName,
		})
	}
	return apps, nil
}

// appxNameFromFull derives a displayable name from PackageFullName
// (Name_Version_Arch__PublisherHash) when DisplayName is null.
func appxNameFromFull(full string) string {
	if i := strings.Index(full, "_"); i > 0 {
		return full[:i]
	}
	return full
}
// removeAppx removes one Store package for the current user.
func removeAppx(packageFullName string) error {
	cmd := fmt.Sprintf("Remove-AppxPackage -Package '%s'", strings.ReplaceAll(packageFullName, "'", "''"))
	_, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", cmd).CombinedOutput()
	return err
}
