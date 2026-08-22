// Package shell emits the Defenestrate PowerShell module: the shell-first
// surface of the CLI. Every destructive verb stays behind the same binary and
// the same safety funnel - the module only wraps it so Windows-native users
// get real functions, pipeline-friendly objects (JSON -> PSObject) and tab
// completion instead of memorizing flags.
package shell

import (
	"fmt"
	"os"
	"path/filepath"
)

const moduleName = "Defenestrate"

// ModuleSource returns the full .ps1m source. Generated, deterministic.
func ModuleSource(version string) string {
	return `#Requires -Version 5.1
<#
.SYNOPSIS
    Defenestrate - throw bloat out the Window. PowerShell surface over the
    defenestrate CLI. Same binary, same safety funnel, native ergonomics.
.NOTES
    Version ` + version + `. Regenerate: defenestrate shell --install
#>

Set-StrictMode -Version Latest

$script:DefExe = $null
function _defExe {
    if ($script:DefExe) { return $script:DefExe }
    $cmd = Get-Command defenestrate -ErrorAction SilentlyContinue
    if ($cmd) { $script:DefExe = $cmd.Source; return $script:DefExe }
    foreach ($c in @(
        (Join-Path $env:LOCALAPPDATA 'Defenestrate\defenestrate.exe'),
        (Join-Path $env:USERPROFILE '.local\bin\defenestrate.exe'))) {
        if (Test-Path $c) { $script:DefExe = $c; return $script:DefExe }
    }
    throw "defenestrate executable not found - add it to PATH or run: defenestrate update"
}

function _def {
    param([string[]]$CliArgs)
    $out = & (_defExe) @CliArgs 2>&1
    $code = 0
    if (Get-Variable LASTEXITCODE -Scope Global -ErrorAction SilentlyContinue) { $code = $LASTEXITCODE }
    if ($code -ne 0) { $out | ForEach-Object { Write-Error $_ } }
    , $out
}

function _defJson {
    param([string[]]$CliArgs)
    $raw = (& (_defExe) @CliArgs | Out-String).Trim()
    try { $raw | ConvertFrom-Json } catch { Write-Error "defenestrate returned non-JSON output"; $null }
}

function Invoke-DefClean {
    <#
    .SYNOPSIS
        Reclaim cache/junk space.
    .EXAMPLE
        Invoke-DefClean -DryRun
    #>
    param([switch]$DryRun, [string[]]$WhitelistAdd, [string]$WhitelistRemove)
    $a = @('clean')
    if ($DryRun) { $a += '--dry-run' }
    foreach ($w in $WhitelistAdd) { $a += @('--whitelist','add',$w) }
    if ($WhitelistRemove) { $a += @('--whitelist','remove',$WhitelistRemove) }
    _def -CliArgs $a
}

function Get-DefStatus {
    <#
    .SYNOPSIS
        Machine snapshot (health, cpu, memory, disks, network).
    .EXAMPLE
        Get-DefStatus | Select-Object health_score
    #>
    param([switch]$Watch)
    _defJson -CliArgs (@('status','--json') + $(if ($Watch) { @('--watch') }))
}

function Invoke-DefAnalyze {
    <#
    .SYNOPSIS
        Where did the disk go?
    .EXAMPLE
        Invoke-DefAnalyze C:\Users\me -Top 25
    #>
    param([Parameter(Mandatory)][string]$Path, [int]$Top = 20, [switch]$Delete)
    $a = @('analyze', $Path, '--top', "$Top")
    if ($Delete) { $a += '--delete' } else { $a += '--json' }
    _defJson -CliArgs $a
}

function Invoke-DefUninstall {
    <#
    .SYNOPSIS
        Remove a program plus registry-evidence leftovers.
    .DESCRIPTION
        Always previews first; pass -Yes to execute.
    .EXAMPLE
        Invoke-DefUninstall "7-Zip" -Yes
    #>
    param([Parameter(Mandatory)][string]$Name, [switch]$DryRun, [switch]$Yes)
    $a = @('uninstall', $Name)
    if ($DryRun) { $a += '--dry-run' }
    if ($Yes) { $a += '--yes' }
    _def -CliArgs $a
}

function Invoke-DefPurge {
    <#
    .SYNOPSIS
        Build artifacts (.git-aware marker scoping).
    .EXAMPLE
        Invoke-DefPurge -DryRun
    #>
    param([switch]$DryRun, [switch]$All, [string[]]$Paths)
    $a = @('purge')
    if ($DryRun) { $a += '--dry-run' }
    if ($All) { $a += '--all' }
    foreach ($p in $Paths) { $a += $p }
    _def -CliArgs $a
}

function Invoke-DefInstallerSweep {
    <#
    .SYNOPSIS
        Orphaned installers in Downloads/Desktop/Temp.
    #>
    param([switch]$DryRun, [switch]$All)
    $a = @('installer')
    if ($DryRun) { $a += '--dry-run' }
    if ($All) { $a += '--all' }
    _def -CliArgs $a
}

function Get-DefHistory {
    <#
    .SYNOPSIS
        Audit trail of everything recycled.
    .EXAMPLE
        Get-DefHistory -Last 50
    #>
    param([int]$Last = 100)
    _defJson -CliArgs @('history','--json','--last',"$Last")
}

function Optimize-DefWindows {
    <#
    .SYNOPSIS
        Bounded maintenance tasks (admin ones skipped unelevated).
    #>
    param([switch]$DryRun, [switch]$All)
    $a = @('optimize')
    if ($DryRun) { $a += '--dry-run' }
    if ($All) { $a += '--all' }
    _def -CliArgs $a
}

Set-Alias def-clean Invoke-DefClean
Set-Alias def-status Get-DefStatus
Export-ModuleMember -Function *-Def* -Alias def-*`
}

// Print writes the module to stdout.
func Print(version string) { fmt.Print(ModuleSource(version)) }

// Install writes the module into the user's PowerShell module path and says where.
func Install(version string) (string, error) {
	base := os.Getenv("Documents")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, "Documents")
	}
	dir := filepath.Join(base, "WindowsPowerShell", "Modules", moduleName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	p := filepath.Join(dir, moduleName+".psm1")
	// UTF-8 BOM: Windows PowerShell 5.1 reads BOM-less files as ANSI and any
	// future non-ASCII literal would corrupt the parse.
	if err := os.WriteFile(p, append([]byte{0xEF, 0xBB, 0xBF}, []byte(ModuleSource(version))...), 0o644); err != nil {
		return "", err
	}
	return p, nil
}
