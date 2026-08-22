package safety

import "os/exec"

// execPowershell isolates the single place we shell out to PowerShell.
func execPowershell(args []string) (string, error) {
	cmd := exec.Command("powershell", args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
