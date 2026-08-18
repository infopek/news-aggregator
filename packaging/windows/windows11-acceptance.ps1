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
