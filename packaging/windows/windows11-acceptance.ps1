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
$originalNpmCLI = $env:NEWS_AGGREGATOR_NPM_CLI
$portableNode = $null
$repo = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$revision = (& git -C $repo rev-parse HEAD).Trim()
try {
  $nodeVersion,$npmVersion = $null,$null
  if (Get-Command node -ErrorAction SilentlyContinue) { $nodeVersion = (& node --version 2>$null) }
  if (Get-Command npm -ErrorAction SilentlyContinue) { $npmVersion = (& npm --version 2>$null) }
  if ($nodeVersion -notmatch '^v22\.' -or $npmVersion -ne '11.6.2') {
    Write-Host "Provisioning checksum-verified portable Node.js 22 and npm 11.6.2"
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
    $userTemp = [Environment]::ExpandEnvironmentVariables([Environment]::GetEnvironmentVariable("TEMP", "User"))
    if (-not $userTemp) { $userTemp = $env:TEMP }
    $portableNode = Join-Path $userTemp ("news-aggregator-node22-" + [guid]::NewGuid().ToString("N"))
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
    $env:NEWS_AGGREGATOR_NPM_CLI = Join-Path $nodeHome "node_modules\npm\bin\npm-cli.js"
    if (-not (Test-Path $env:NEWS_AGGREGATOR_NPM_CLI)) { throw "portable npm CLI was not installed" }
  }

  & (Join-Path $PSScriptRoot "smoke.ps1") -OutputRoot $OutputRoot -Revision $revision -Restricted
  if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
  $evidence = Join-Path $OutputRoot "Evidence"

  # The portable smoke shadows rundll32 to count deterministic launch
  # requests. Windows 11 acceptance additionally starts the built executable
  # with the real system PATH and observes a browser-owned loopback connection.
  $browserData = Join-Path $OutputRoot "Actual Browser User Data"
  $browserLog = Join-Path $evidence "actual-browser.log"
  $browserPortListener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
  $browserPortListener.Start()
  $browserPort = ([System.Net.IPEndPoint]$browserPortListener.LocalEndpoint).Port
  $browserPortListener.Stop()
  $browserStop = Join-Path $evidence "actual-browser.stop"
  $browserPIDFile = Join-Path $evidence "actual-browser.pid"
  $savedAppData,$savedPort = $env:APPDATA,$env:NEWS_AGGREGATOR_PORT
  $browserProcess = $null
  try {
    New-Item -ItemType Directory -Force -Path $browserData | Out-Null
    $env:APPDATA = $browserData
    $env:NEWS_AGGREGATOR_PORT = [string]$browserPort
    $control = Join-Path $OutputRoot "Build With Spaces\process-control.exe"
    $application = Join-Path $OutputRoot "Build With Spaces\news-aggregator.exe"
    $browserProcess = Start-Process -FilePath $control -ArgumentList "`"$browserPIDFile`"", "`"$browserStop`"", "`"$application`"" -PassThru
    $browserConnection = $null
    for ($attempt = 0; $attempt -lt 200 -and -not $browserConnection; $attempt++) {
      # The browser side owns the connection whose remote port is the app's
      # listening port; the server side has that value as its local port.
      $browserConnection = Get-NetTCPConnection -State Established -RemotePort $browserPort -ErrorAction SilentlyContinue | Select-Object -First 1
      Start-Sleep -Milliseconds 50
    }
    if (-not $browserConnection) { throw "actual default browser did not connect to the local application" }
    $client = Get-Process -Id $browserConnection.OwningProcess -ErrorAction Stop
    "port=$browserPort`nclientPid=$($client.Id)`nclientProcess=$($client.ProcessName)`nurl=http://127.0.0.1:$browserPort" | Set-Content $browserLog
    New-Item -ItemType File -Force -Path $browserStop | Out-Null
    if (-not $browserProcess.WaitForExit(10000)) { throw "actual-browser application did not stop" }
  } finally {
    if ($browserProcess -and -not $browserProcess.HasExited) { Stop-Process -Id $browserProcess.Id -Force }
    $env:APPDATA,$env:NEWS_AGGREGATOR_PORT = $savedAppData,$savedPort
    if (Test-Path $browserData) { Remove-Item -Recurse -Force $browserData }
  }

  Get-CimInstance Win32_OperatingSystem | Select-Object Caption,Version,BuildNumber,OSArchitecture | Format-List | Out-File (Join-Path $evidence "windows-version.txt")
  Add-Content (Join-Path $evidence "native-smoke.log") "windows_acceptance=$caption"
  Add-Content (Join-Path $evidence "native-smoke.log") "actual_default_browser=connected process=$($client.ProcessName)"

  # smoke.ps1 deliberately redirects caches and scratch state beneath the
  # isolated output root. They are useful during execution but are not
  # acceptance evidence and can exceed GitHub's comment-attachment limit.
  foreach ($transientName in @("Actual Browser User Data", "Browser Probe", "Empty Path", "Local Data", "Temp")) {
    $transientPath = Join-Path $OutputRoot $transientName
    if (Test-Path $transientPath) { Remove-Item -Recurse -Force $transientPath }
  }

  $archive = "$OutputRoot.zip"
  if (Test-Path $archive) {
    throw "Refusing to overwrite existing evidence archive $archive"
  }
  Compress-Archive -Path (Join-Path $OutputRoot "*") -DestinationPath $archive
  $archiveInfo = Get-Item $archive
  if ($archiveInfo.Length -gt 25MB) {
    throw "evidence archive is larger than GitHub's 25 MB comment-upload limit: $($archiveInfo.Length) bytes"
  }
  $archiveHash = (Get-FileHash $archive -Algorithm SHA256).Hash.ToLowerInvariant()
  $comment = @"
Windows 11 native acceptance evidence for exact revision ``$revision``.

Executed from normal, non-admin PowerShell.

ZIP SHA-256: ``$archiveHash``
"@
  $commentPath = "$OutputRoot-pr-comment.txt"
  Set-Content -Path $commentPath -Value $comment
  Write-Host ""
  Write-Host "Windows 11 acceptance passed."
  Write-Host "Evidence ZIP: $archive"
  Write-Host "ZIP size: $($archiveInfo.Length) bytes"
  Write-Host "PR comment: $commentPath"
  Write-Host ""
  Write-Host $comment
} finally {
  $env:PATH = $originalPath
  $env:NEWS_AGGREGATOR_NPM_CLI = $originalNpmCLI
  if ($portableNode -and (Test-Path $portableNode)) {
    Remove-Item -Recurse -Force $portableNode
  }
}
