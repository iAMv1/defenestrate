# PowerShell completion for DEFENESTRATE.
# Install:  . ./completion/DEFENESTRATE.ps1   (or dot-source from $PROFILE)

$subcommands = @('clean','uninstall','analyze','analyse','status','optimize',
                 'purge','installer','hud','update','history','menu',
                 'version','--help','-h')
$flags = @('--dry-run','--json','--watch','--all','--yes','-y','--top','--delete',
           '--paths','--whitelist')

Register-ArgumentCompleter -Native -CommandName DEFENESTRATE -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)
    $words = $commandAst.CommandElements | Select-Object -Skip 1 |
        ForEach-Object { $_.ToString() } | Where-Object { $_ }
    if ($words.Count -le 1) {
        $subcommands | Where-Object { $_ -like "$wordToComplete*" } |
            ForEach-Object { [System.Management.Automation.CompletionResult]::new($_) }
        return
    }
    $flags | Where-Object { $_ -like "$wordToComplete*" } |
        ForEach-Object { [System.Management.Automation.CompletionResult]::new($_) }
}
