param(
  [string]$OutputRoot = (Join-Path $env:TEMP "News Aggregator Windows 11 Evidence")
)

$ErrorActionPreference = "Stop"
$caption = (Get-CimInstance Win32_OperatingSystem).Caption
if ($caption -notmatch "Windows 11") {
  throw "VERIFY-003 requires Windows 11; detected $caption"
}
$principal = [Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())
if ($principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
  throw "Run this command from a non-elevated Windows 11 terminal"
}

$originalPath = $env:PATH
$portableNode = $null
try {
  $nodeVersion,$npmVersion = $null,$null
  if (Get-Command node -ErrorAction SilentlyContinue) { $nodeVersion = (& node --version 2>$null) }
  if (Get-Command npm -ErrorAction SilentlyContinue) { $npmVersion = (& npm --version 2>$null) }
  if ($nodeVersion -notmatch '^v22\.' -or $npmVersion -ne '11.6.2') {
    Write-Host "Provisioning checksum-verified portable Node.js 22 and npm 11.6.2"
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
    $portableNode = Join-Path $env:TEMP ("news-aggregator-node22-" + [guid]::NewGuid().ToString("N"))
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
    & npm.cmd install --global npm@11.6.2
    if ($LASTEXITCODE -ne 0) { throw "portable npm installation failed with exit code $LASTEXITCODE" }
  }

  & (Join-Path $PSScriptRoot "smoke.ps1") -OutputRoot $OutputRoot -Revision ((git -C (Join-Path $PSScriptRoot "..\..") rev-parse HEAD).Trim()) -Restricted
  if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
  $evidence = Join-Path $OutputRoot "Evidence"
  Get-CimInstance Win32_OperatingSystem | Select-Object Caption,Version,BuildNumber,OSArchitecture | Format-List | Out-File (Join-Path $evidence "windows-version.txt")
  Add-Content (Join-Path $evidence "native-smoke.log") "windows_acceptance=$caption"
  $archive = "$OutputRoot.zip"
  if (Test-Path $archive) {
    throw "Refusing to overwrite existing evidence archive $archive"
  }
  Compress-Archive -Path (Join-Path $OutputRoot "*") -DestinationPath $archive
  Write-Host "Windows 11 acceptance evidence: $archive"
} finally {
  $env:PATH = $originalPath
  if ($portableNode -and (Test-Path $portableNode)) {
    Remove-Item -Recurse -Force $portableNode
  }
}
