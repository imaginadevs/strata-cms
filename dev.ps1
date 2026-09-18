#Requires -Version 5.1
<#
.SYNOPSIS
  Windows PowerShell equivalent of the Makefile targets.

.EXAMPLE
  .\dev.ps1 setup
  .\dev.ps1 migrate-up
  .\dev.ps1 dev
  .\dev.ps1 docker-up
#>
param(
    [Parameter(Position = 0)]
    [ValidateSet(
        "setup", "dev", "dev-backend", "dev-frontend",
        "build", "build-frontend", "build-backend",
        "docker-build", "docker-up", "docker-down", "docker-logs",
        "migrate-up", "migrate-down", "migrate-create",
        "lint", "test", "clean", "help"
    )]
    [string]$Target = "help",

    [string]$Name
)

$ErrorActionPreference = "Stop"
$Root = $PSScriptRoot

function Write-Step([string]$Message) {
    Write-Host ("-> " + $Message) -ForegroundColor Cyan
}

function Write-Ok([string]$Message) {
    Write-Host ("OK " + $Message) -ForegroundColor Green
}

function Assert-Command([string]$CmdName) {
    if (-not (Get-Command $CmdName -ErrorAction SilentlyContinue)) {
        throw ("'{0}' is not installed or not on PATH." -f $CmdName)
    }
}

function Invoke-Setup {
    Assert-Command npm
    Assert-Command go

    $envExample = Join-Path $Root ".env.example"
    $envFile = Join-Path $Root ".env"
    if (-not (Test-Path $envFile)) {
        if (Test-Path $envExample) {
            Copy-Item $envExample $envFile
            Write-Ok "Created .env from .env.example"
        }
    } else {
        Write-Host ".env already exists - skipping copy"
    }

    Write-Step "Installing frontend dependencies..."
    Push-Location (Join-Path $Root "frontend")
    try { npm install } finally { Pop-Location }

    Write-Step "Downloading Go modules..."
    Push-Location (Join-Path $Root "backend")
    try { go mod download } finally { Pop-Location }

    Write-Ok "Setup complete. Edit .env then run: .\dev.ps1 dev"
}

function Invoke-DevBackend {
    Assert-Command go
    Write-Step "Starting backend..."
    Push-Location (Join-Path $Root "backend")
    try { go run ./cmd/server/main.go } finally { Pop-Location }
}

function Invoke-DevFrontend {
    Assert-Command npm
    Write-Step "Starting frontend..."
    Push-Location (Join-Path $Root "frontend")
    try { npm run dev } finally { Pop-Location }
}

function Invoke-Dev {
    Assert-Command go
    Assert-Command npm

    Write-Step "Starting backend + frontend..."
    $backend = Start-Process -FilePath "go" `
        -ArgumentList @("run", "./cmd/server/main.go") `
        -WorkingDirectory (Join-Path $Root "backend") `
        -NoNewWindow -PassThru

    $frontend = Start-Process -FilePath "npm.cmd" `
        -ArgumentList @("run", "dev") `
        -WorkingDirectory (Join-Path $Root "frontend") `
        -NoNewWindow -PassThru

    Write-Host ("Backend PID {0} / Frontend PID {1}" -f $backend.Id, $frontend.Id)
    Write-Host "Press Ctrl+C to stop both."

    try {
        Wait-Process -Id @($backend.Id, $frontend.Id)
    } finally {
        foreach ($p in @($backend, $frontend)) {
            if ($p -and -not $p.HasExited) {
                Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue
            }
        }
    }
}

function Invoke-BuildFrontend {
    Assert-Command npm
    Write-Step "Building frontend..."
    Push-Location (Join-Path $Root "frontend")
    try {
        npm ci
        npm run build
    } finally { Pop-Location }
}

function Invoke-BuildBackend {
    Assert-Command go
    Invoke-BuildFrontend

    Write-Step "Embedding frontend into backend..."
    $distSrc = Join-Path $Root "frontend\dist"
    $distDst = Join-Path $Root "backend\internal\infrastructure\static\dist"
    if (Test-Path $distDst) { Remove-Item $distDst -Recurse -Force }
    New-Item -ItemType Directory -Path $distDst -Force | Out-Null
    Copy-Item -Path (Join-Path $distSrc "*") -Destination $distDst -Recurse -Force

    Write-Step "Building backend..."
    $binDir = Join-Path $Root "bin"
    New-Item -ItemType Directory -Path $binDir -Force | Out-Null
    $env:CGO_ENABLED = "0"
    Push-Location (Join-Path $Root "backend")
    try {
        go build -ldflags="-s -w" -o (Join-Path $binDir "server.exe") ./cmd/server/main.go
    } finally { Pop-Location }
    Write-Ok "Built bin\server.exe"
}

function Invoke-DockerBuild {
    Assert-Command docker
    docker build -t cms:latest $Root
}

function Invoke-DockerUp {
    Assert-Command docker
    docker compose -f (Join-Path $Root "docker-compose.yml") up -d
}

function Invoke-DockerDown {
    Assert-Command docker
    docker compose -f (Join-Path $Root "docker-compose.yml") down
}

function Invoke-DockerLogs {
    Assert-Command docker
    docker compose -f (Join-Path $Root "docker-compose.yml") logs -f
}

function Invoke-MigrateUp {
    Assert-Command go
    Write-Step "Applying migrations..."
    Push-Location (Join-Path $Root "backend")
    try { go run ./cmd/migrate/main.go up } finally { Pop-Location }
}

function Invoke-MigrateDown {
    Assert-Command go
    Write-Step "Rolling back migration..."
    Push-Location (Join-Path $Root "backend")
    try { go run ./cmd/migrate/main.go down } finally { Pop-Location }
}

function Invoke-MigrateCreate {
    Assert-Command go
    if (-not $Name) {
        throw "Usage: .\dev.ps1 migrate-create -Name add_posts"
    }
    Write-Step ("Creating migration '{0}'..." -f $Name)
    Push-Location (Join-Path $Root "backend")
    try { go run ./cmd/migrate/main.go create $Name sql } finally { Pop-Location }
}

function Invoke-Lint {
    Assert-Command npm
    if (Get-Command golangci-lint -ErrorAction SilentlyContinue) {
        Push-Location (Join-Path $Root "backend")
        try { golangci-lint run ./... } finally { Pop-Location }
    } else {
        Write-Host "golangci-lint not found - skipping backend lint"
    }
    Push-Location (Join-Path $Root "frontend")
    try { npm run lint } finally { Pop-Location }
}

function Invoke-Test {
    Assert-Command go
    Assert-Command npm
    Push-Location (Join-Path $Root "backend")
    try { go test ./... -race -cover } finally { Pop-Location }
    Push-Location (Join-Path $Root "frontend")
    try { npm run test --if-present } finally { Pop-Location }
}

function Invoke-Clean {
    @(
        (Join-Path $Root "bin"),
        (Join-Path $Root "frontend\dist"),
        (Join-Path $Root "backend\tmp")
    ) | ForEach-Object {
        if (Test-Path $_) { Remove-Item $_ -Recurse -Force }
    }
    Write-Ok "Cleaned"
}

function Show-Help {
    Write-Host @"
Usage: .\dev.ps1 TARGET [-Name MIGRATION]

Targets:
  setup            Copy .env.example to .env (if missing), install deps
  dev              Start backend + frontend
  dev-backend      Start Go API only
  dev-frontend     Start Vite only
  build            Production build (frontend embedded in Go binary)
  docker-build     Build Docker image
  docker-up        Start docker compose stack
  docker-down      Stop docker compose stack
  docker-logs      Tail compose logs
  migrate-up       Apply pending migrations
  migrate-down     Roll back one migration
  migrate-create   Create migration (use -Name)
  lint             Lint backend + frontend
  test             Run tests
  clean            Remove build artifacts

Examples:
  .\dev.ps1 setup
  .\dev.ps1 docker-up
  .\dev.ps1 migrate-up
  .\dev.ps1 migrate-create -Name add_posts
  .\dev.ps1 dev
"@
}

switch ($Target) {
    "setup"          { Invoke-Setup }
    "dev"            { Invoke-Dev }
    "dev-backend"    { Invoke-DevBackend }
    "dev-frontend"   { Invoke-DevFrontend }
    "build"          { Invoke-BuildBackend }
    "build-frontend" { Invoke-BuildFrontend }
    "build-backend"  { Invoke-BuildBackend }
    "docker-build"   { Invoke-DockerBuild }
    "docker-up"      { Invoke-DockerUp }
    "docker-down"    { Invoke-DockerDown }
    "docker-logs"    { Invoke-DockerLogs }
    "migrate-up"     { Invoke-MigrateUp }
    "migrate-down"   { Invoke-MigrateDown }
    "migrate-create" { Invoke-MigrateCreate }
    "lint"           { Invoke-Lint }
    "test"           { Invoke-Test }
    "clean"          { Invoke-Clean }
    default          { Show-Help }
}
