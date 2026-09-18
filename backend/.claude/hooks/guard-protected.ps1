$ErrorActionPreference = "SilentlyContinue"
$raw = [Console]::In.ReadToEnd()
function Allow { Write-Output '{"continue":true,"permissionDecision":"allow"}'; exit 0 }
function Deny([string]$reason) {
  $r = $reason -replace '"','\"'
  Write-Output "{`"continue`":false,`"permissionDecision`":`"deny`",`"permissionDecisionReason`":`"$r`"}"
  exit 2
}
if (-not $raw) { Allow }
try { $data = $raw | ConvertFrom-Json } catch { Allow }
$fp = $data.tool_input.file_path
if (-not $fp) { $fp = $data.tool_input.path }
if (-not $fp) { Allow }
$norm = $fp -replace "\\", "/"
if ($norm -match "internal/infrastructure/database/migrations/.*\.sql$") { Deny "Applied goose migrations are immutable. Add a NEW numbered migration." }
if ($norm -match "(^|/)\.env($|\..*)") { Deny "Do not edit .env files." }
if ($norm -match "(^|/)go\.sum$") { Deny "Do not hand-edit go.sum." }
Allow