param(
  [string]$OutputRoot
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"
$PSNativeCommandUseErrorActionPreference = $true

if ($env:OS -ne "Windows_NT") {
  throw "This build wrapper requires Windows"
}

$repo = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
if (-not $OutputRoot) { $OutputRoot = Join-Path $repo "build" }
$originalPath = $env:PATH
$portableNode = $null
$npmCLI = $null

function Invoke-Npm([string[]]$Arguments) {
  if ($npmCLI) {
    & node $npmCLI @Arguments
  } else {
    & npm @Arguments
  }
  if ($LASTEXITCODE -ne 0) {
    throw "npm failed with exit code $LASTEXITCODE"
  }
}

try {
  if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw "Go is required but was not found on PATH"
  }

  $nodeVersion,$npmVersion = $null,$null
  if (Get-Command node -ErrorAction SilentlyContinue) { $nodeVersion = (& node --version 2>$null) }
  if (Get-Command npm -ErrorAction SilentlyContinue) { $npmVersion = (& npm --version 2>$null) }
  if ($nodeVersion -notmatch '^v22\.' -or $npmVersion -ne '11.6.2') {
    Write-Host "Provisioning checksum-verified portable Node.js 22 and npm 11.6.2"
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
    $portableNode = Join-Path $env:TEMP ("news-aggregator-build-node22-" + [guid]::NewGuid().ToString("N"))
    New-Item -ItemType Directory -Path $portableNode | Out-Null
    $baseURL = "https://nodejs.org/dist/latest-v22.x"
    $entries = (Invoke-WebRequest "$baseURL/SHASUMS256.txt" -UseBasicParsing -TimeoutSec 30).Content -split "`n"
    $entry = $entries | Where-Object { $_ -match 'node-v22\..*-win-x64\.zip$' } | Select-Object -First 1
    if (-not $entry) { throw "Node.js 22 checksum manifest did not contain the Windows x64 archive" }
    $parts = $entry.Trim() -split '\s+'
    $expectedHash,$fileName = $parts[0],$parts[-1]
    $archivePath = Join-Path $portableNode $fileName
    Invoke-WebRequest "$baseURL/$fileName" -OutFile $archivePath -UseBasicParsing -TimeoutSec 120
    $actualHash = (Get-FileHash $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actualHash -ne $expectedHash) { throw "Node.js 22 archive checksum mismatch" }
    Expand-Archive $archivePath -DestinationPath $portableNode
    $nodeHome = Join-Path $portableNode ([IO.Path]::GetFileNameWithoutExtension($fileName))
    $env:PATH = "$nodeHome;$env:PATH"
    $portableNpm = Join-Path $nodeHome "npm.cmd"
    & $portableNpm install --global npm@11.6.2
    if ($LASTEXITCODE -ne 0) { throw "portable npm installation failed with exit code $LASTEXITCODE" }
    $npmCLI = Join-Path $nodeHome "node_modules\npm\bin\npm-cli.js"
    if (-not (Test-Path $npmCLI)) { throw "portable npm CLI was not installed" }
  }

  Push-Location $repo
  try {
    Write-Host "Installing frontend dependencies"
    Invoke-Npm @("--prefix", "web", "ci")
    Write-Host "Building embedded frontend"
    Invoke-Npm @("--prefix", "web", "run", "build")

    New-Item -ItemType Directory -Force -Path $OutputRoot | Out-Null
    $executable = Join-Path $OutputRoot "news-aggregator.exe"
    Write-Host "Building Windows executable"
    & go build -trimpath -o $executable ./cmd/news-aggregator
    if ($LASTEXITCODE -ne 0) { throw "Go build failed with exit code $LASTEXITCODE" }

    $keepFile = Join-Path $repo "internal\webassets\dist\.gitkeep"
    if (-not (Test-Path $keepFile)) { [IO.File]::WriteAllBytes($keepFile, [byte[]]@()) }
    $hash = (Get-FileHash $executable -Algorithm SHA256).Hash.ToLowerInvariant()
    Write-Host ""
    Write-Host "Build complete: $executable"
    Write-Host "SHA-256: $hash"
  } finally {
    Pop-Location
  }
} finally {
  $env:PATH = $originalPath
  if ($portableNode -and (Test-Path $portableNode)) {
    Remove-Item -Recurse -Force $portableNode
  }
}
