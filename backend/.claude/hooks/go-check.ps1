# PostToolUse hook: format the edited Go file and vet the module.
# Receives Claude Code hook JSON on stdin. No-ops for non-.go files.
$ErrorActionPreference = 'SilentlyContinue'

$raw = [Console]::In.ReadToEnd()
if (-not $raw) { exit 0 }

try { $data = $raw | ConvertFrom-Json } catch { exit 0 }

$fp = $data.tool_input.file_path
if (-not $fp) { $fp = $data.tool_input.path }
if (-not $fp -or ($fp -notmatch '\.go$')) { exit 0 }

$proj = $env:CLAUDE_PROJECT_DIR
if (-not $proj) { $proj = (Get-Location).Path }

# Format just the edited file (idempotent).
if (Test-Path $fp) { & gofmt -w -- "$fp" 2>$null }

# Compile + vet the whole module; surface only the first errors to Claude.
Push-Location $proj
$vet = & go vet ./... 2>&1
$code = $LASTEXITCODE
Pop-Location

if ($code -ne 0) {
    $msg = ($vet | Select-Object -First 40) -join "`n"
    # Exit 2 => feed stderr back to Claude as actionable context (non-blocking here).
    [Console]::Error.WriteLine("go vet reported issues after editing $fp`:`n$msg")
    exit 2
}
exit 0
