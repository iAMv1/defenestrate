# clean_size.ps1 - shell-first measurement engine.
# Input : temp JSON file path as $args[0] -> { "roots": ["C:\a", ...] }
# Output: ONE JSON object mapping EVERY directory under the roots (inclusive)
#         to its recursive byte total.
# Fast path: single .NET walk collects per-dir DIRECT file bytes; a reversed
# discovery-order fold adds subtrees into parents (no per-file ancestor loop).
# Reparse points are never descended, matching the Go walker's policy.
param([string]$SpecPath)
$spec = Get-Content -LiteralPath $SpecPath -Raw | ConvertFrom-Json
$sizes = @{}
$childDirs = @{}
$order = New-Object System.Collections.Generic.List[string]
foreach ($root in $spec.roots) {
    if ([string]::IsNullOrWhiteSpace($root)) { continue }
    if (-not [System.IO.Directory]::Exists($root)) { continue }
    $stack = New-Object System.Collections.Stack
    $stack.Push($root)
    while ($stack.Count -gt 0) {
        $dir = [string]$stack.Pop()
        if ($sizes.ContainsKey($dir)) { continue }
        try {
            $di = [System.IO.DirectoryInfo]::new($dir)
            if ($di.Attributes -band [System.IO.FileAttributes]::ReparsePoint) { continue }
            $sizes[$dir] = [long]0
            $order.Add($dir)
            $kids = @()
            foreach ($f in $di.EnumerateFiles()) {
                try { $sizes[$dir] += [long]$f.Length } catch { }
            }
            foreach ($d in $di.EnumerateDirectories()) {
                if (-not ($d.Attributes -band [System.IO.FileAttributes]::ReparsePoint)) {
                    $kids += $d.FullName
                    $stack.Push($d.FullName)
                }
            }
            if ($kids.Count -gt 0) { $childDirs[$dir] = $kids }
        } catch {
            if (-not $sizes.ContainsKey($dir)) { $sizes[$dir] = [long]0; $order.Add($dir) }
        }
    }
}
for ($i = $order.Count - 1; $i -ge 0; $i--) {
    $d = $order[$i]
    $kids = $childDirs[$d]
    if ($kids) {
        foreach ($k in $kids) {
            $kv = $sizes[$k]
            if ($kv) { $sizes[$d] += $kv }
        }
    }
}
$sizes | ConvertTo-Json -Compress -Depth 2
