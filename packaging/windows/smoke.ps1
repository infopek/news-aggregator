param(
  [string]$OutputRoot = (Join-Path $env:RUNNER_TEMP "News Aggregator Native Smoke"),
  [string]$Revision,
  [string]$CredentialSentinel,
  [switch]$Restricted
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"
$PSNativeCommandUseErrorActionPreference = $true
$CredentialSentinel = if ($CredentialSentinel) { $CredentialSentinel } else { "VERIFY003-$([guid]::NewGuid().ToString('N'))" }

if (-not $Restricted) {
  New-Item -ItemType Directory -Force -Path $OutputRoot | Out-Null
  $account = "Verify003$([guid]::NewGuid().ToString('N').Substring(0,8))"
  $passwordText = "N3ws!$([guid]::NewGuid().ToString('N'))"
  $password = ConvertTo-SecureString $passwordText -AsPlainText -Force
  $credential = [PSCredential]::new(".\$account", $password)
  $repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
  $Revision = (& git -C $repoRoot rev-parse HEAD).Trim()
  $childOut = Join-Path $OutputRoot "restricted-stdout.log"
  $childError = Join-Path $OutputRoot "restricted-stderr.log"
  $toolRoot = Join-Path $env:RUNNER_TEMP "News Aggregator Native Tool Cache-$([guid]::NewGuid().ToString('N'))"
  try {
    New-LocalUser -Name $account -Password $password -PasswordNeverExpires -UserMayNotChangePassword | Out-Null
    & icacls.exe $repoRoot /grant "${account}:(OI)(CI)M" /T /Q | Out-Null
    & icacls.exe $OutputRoot /grant "${account}:(OI)(CI)M" /T /Q | Out-Null
    $env:GOCACHE = Join-Path $toolRoot "Go Cache"
    $env:GOPATH = Join-Path $toolRoot "Go Path"
    $env:npm_config_cache = Join-Path $toolRoot "Npm Cache"
    New-Item -ItemType Directory -Force -Path $env:GOCACHE,$env:GOPATH,$env:npm_config_cache | Out-Null
    & icacls.exe $toolRoot /grant "${account}:(OI)(CI)M" /T /Q | Out-Null
    Write-Host "Launching native smoke as temporary standard user $account"
    $arguments = "-NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File `"$PSCommandPath`" -OutputRoot `"$OutputRoot`" -Revision `"$Revision`" -CredentialSentinel `"$CredentialSentinel`" -Restricted"
    $launcher = Start-Process -FilePath (Get-Process -Id $PID).Path -Credential $credential -LoadUserProfile -ArgumentList $arguments -RedirectStandardOutput $childOut -RedirectStandardError $childError -PassThru -Wait
    if ($launcher.ExitCode -ne 0) {
      if (Test-Path $childOut) { Get-Content $childOut | Write-Host }
      if (Test-Path $childError) { Get-Content $childError | Write-Error }
      throw "standard-user native smoke exited with code $($launcher.ExitCode)"
    }
    foreach ($launcherLog in @($childOut, $childError)) {
      if ((Get-Content $launcherLog -Raw).Contains($CredentialSentinel)) {
        throw "credential sentinel leaked into $launcherLog"
      }
    }
  } finally {
    if (Get-LocalUser -Name $account -ErrorAction SilentlyContinue) {
      Remove-LocalUser -Name $account
    }
    if (Test-Path $toolRoot) {
      Remove-Item -Recurse -Force $toolRoot
    }
  }
  exit 0
}

$repo = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$build = Join-Path $OutputRoot "Build With Spaces"
$data = Join-Path $OutputRoot "User Data"
$logs = Join-Path $OutputRoot "Evidence"
$probe = Join-Path $OutputRoot "Browser Probe"
$emptyPath = Join-Path $OutputRoot "Empty Path"
$temp = Join-Path $OutputRoot "Temp"
$localData = Join-Path $OutputRoot "Local Data"
$exe = Join-Path $build "news-aggregator.exe"
$probeExe = Join-Path $probe "rundll32.exe"
$controlExe = Join-Path $build "process-control.exe"
$fixtureExe = Join-Path $build "shutdown-fixture.exe"
$browserLog = Join-Path $logs "browser-launch.log"
$runtimeLog = Join-Path $logs "runtime.log"
$runtimeError = Join-Path $logs "runtime-error.log"
$smokeLog = Join-Path $logs "native-smoke.log"
$app,$restart,$fixtureProcess = $null,$null,$null
$originalAppData,$originalPort,$originalPath = $env:APPDATA,$env:NEWS_AGGREGATOR_PORT,$env:PATH

New-Item -ItemType Directory -Force -Path $build,$data,$logs,$probe,$emptyPath,$temp,$localData | Out-Null
$env:APPDATA = $data
$env:LOCALAPPDATA = $localData
$env:TEMP = $temp
$env:TMP = $temp
$env:GOTMPDIR = $temp
Set-Content -Path $smokeLog -Value "VERIFY-003 native Windows smoke`nstarted=$([DateTime]::UtcNow.ToString('o'))`nrepo=$repo`noutput=$OutputRoot"

function Record([string]$Message) {
  Add-Content -Path $smokeLog -Value $Message
  Write-Host $Message
}

function Free-Port {
  $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
  $listener.Start()
  $port = ([System.Net.IPEndPoint]$listener.LocalEndpoint).Port
  $listener.Stop()
  return $port
}

function Wait-Ready([int]$Port, [System.Diagnostics.Process]$Process) {
  for ($attempt = 0; $attempt -lt 100; $attempt++) {
    if ($Process.HasExited) { throw "application exited before readiness with code $($Process.ExitCode)" }
    try {
      $response = Invoke-WebRequest -UseBasicParsing -TimeoutSec 1 "http://127.0.0.1:$Port/api/v1/health"
      if ($response.StatusCode -eq 200) { return }
    } catch {}
    Start-Sleep -Milliseconds 100
  }
  throw "application readiness timed out"
}

function Stop-App([System.Diagnostics.Process]$Process, [string]$StopFile) {
  if ($Process.HasExited) { return }
  # Cache the native handle before exit; Windows PowerShell 5.1 can otherwise
  # leave ExitCode unset after the timed WaitForExit overload.
  $null = $Process.Handle
  New-Item -ItemType File -Force -Path $StopFile | Out-Null
  if (-not $Process.WaitForExit(10000)) { throw "application did not stop within ten seconds" }
  $Process.WaitForExit()
  $Process.Refresh()
  $exitCode = $Process.ExitCode
  if ($null -eq $exitCode) { throw "application exit code was unavailable" }
  if ($exitCode -ne 0) { throw "application did not exit cleanly: $exitCode" }
}

Push-Location $repo
try {
  $tokenGroups = Join-Path $logs "process-token-groups.txt"
  whoami /groups | Out-File $tokenGroups
  $principal = [Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())
  $isAdministrator = $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
  $groupsText = Get-Content $tokenGroups -Raw
  if ($isAdministrator -or $groupsText -match 'High Mandatory Level|System Mandatory Level') {
    throw "native smoke must run with a non-admin, non-elevated token"
  }
  Record "token=non-admin non-elevated"
  Record "revision=$Revision"

  $nodeVersion = (node --version).Trim()
  if ($LASTEXITCODE -ne 0 -or $nodeVersion -notmatch '^v22\.') {
    throw "native smoke requires Node.js 22; detected $nodeVersion"
  }
  $npmVersion = (npm --version).Trim()
  if ($LASTEXITCODE -ne 0 -or $npmVersion -ne '11.6.2') {
    throw "native smoke requires npm 11.6.2; detected $npmVersion"
  }
  Record "toolchain node=$nodeVersion npm=$npmVersion"

  Record "building frontend"
  npm --prefix web ci | Out-File -Append -FilePath $smokeLog
  if ($LASTEXITCODE -ne 0) { throw "npm ci failed with exit code $LASTEXITCODE" }
  npm --prefix web run build | Out-File -Append -FilePath $smokeLog
  if ($LASTEXITCODE -ne 0) { throw "frontend build failed with exit code $LASTEXITCODE" }
  Record "building executable and browser probe from path containing spaces"
  go build -trimpath -o $exe ./cmd/news-aggregator
  if ($LASTEXITCODE -ne 0) { throw "application build failed with exit code $LASTEXITCODE" }
  go build -trimpath -o $probeExe ./packaging/windows/browser-probe
  if ($LASTEXITCODE -ne 0) { throw "browser probe build failed with exit code $LASTEXITCODE" }
  go build -trimpath -o $controlExe ./packaging/windows/process-control
  if ($LASTEXITCODE -ne 0) { throw "process-control build failed with exit code $LASTEXITCODE" }
  go build -trimpath -o $fixtureExe ./packaging/windows/shutdown-fixture
  if ($LASTEXITCODE -ne 0) { throw "shutdown fixture build failed with exit code $LASTEXITCODE" }
  $hash = (Get-FileHash -Algorithm SHA256 $exe).Hash.ToLowerInvariant()
  "${hash}  news-aggregator.exe" | Set-Content (Join-Path $logs "news-aggregator.exe.sha256")
  Record "sha256=$hash"

  $port = Free-Port
  $env:APPDATA = $data
  $env:NEWS_AGGREGATOR_PORT = [string]$port
  $env:NEWS_AGGREGATOR_BROWSER_PROBE_LOG = $browserLog
  $env:PATH = "$probe;$originalPath"
  $pidFile = Join-Path $logs "application.pid"
  $stopFile = Join-Path $logs "application.stop"
  $app = Start-Process -FilePath $controlExe -ArgumentList "`"$pidFile`"", "`"$stopFile`"", "`"$exe`"" -PassThru -RedirectStandardOutput $runtimeLog -RedirectStandardError $runtimeError
  $env:PATH = $originalPath
  Wait-Ready $port $app
  for ($attempt=0; $attempt -lt 50 -and -not (Test-Path $pidFile); $attempt++) { Start-Sleep -Milliseconds 100 }
  $applicationPID = [int](Get-Content $pidFile -Raw)
  Record "ready pid=$applicationPID port=$port"

  $health = Invoke-RestMethod "http://127.0.0.1:$Port/api/v1/health"
  if ($health.status -ne "ready") { throw "health contract was not ready" }
  $spa = Invoke-WebRequest -UseBasicParsing "http://127.0.0.1:$Port/settings"
  if ($spa.StatusCode -ne 200 -or $spa.Content -notmatch '<div id="app">') { throw "embedded SPA route failed" }
  $listeners = Get-NetTCPConnection -State Listen -OwningProcess $applicationPID
  $listeners | Format-Table -AutoSize | Out-String | Set-Content (Join-Path $logs "listening-sockets.txt")
  if (-not $listeners -or ($listeners | Where-Object { $_.LocalAddress -notin @("127.0.0.1","::1") })) { throw "application exposed a non-loopback listener" }
  for ($attempt=0; $attempt -lt 50 -and -not (Test-Path $browserLog); $attempt++) { Start-Sleep -Milliseconds 100 }
  $launches = @(Get-Content $browserLog)
  if ($launches.Count -ne 1 -or $launches[0] -notmatch "http://127.0.0.1:$port") { throw "default-browser boundary was invoked $($launches.Count) times" }
  Record "browser launch invoked exactly once"

  $occupied = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, (Free-Port))
  $occupied.Start()
  $occupiedPort = ([System.Net.IPEndPoint]$occupied.LocalEndpoint).Port
  $env:NEWS_AGGREGATOR_PORT = [string]$occupiedPort
  $occupiedLog = Join-Path $logs "occupied-port.log"
  $second = Start-Process -FilePath $exe -PassThru -Wait -RedirectStandardError $occupiedLog
  $occupied.Stop()
  if ($second.ExitCode -eq 0) { throw "occupied-port launch unexpectedly succeeded" }
  Record "occupied port refused safely exit=$($second.ExitCode)"

  $fixtureURLFile = Join-Path $logs "shutdown-fixture-url.txt"
  $fixtureProcess = Start-Process -FilePath $fixtureExe -ArgumentList "`"$fixtureURLFile`"" -PassThru
  for ($attempt=0; $attempt -lt 50 -and -not (Test-Path $fixtureURLFile); $attempt++) { Start-Sleep -Milliseconds 100 }
  if (-not (Test-Path $fixtureURLFile)) { throw "shutdown fixture did not become ready" }
  $fixtureURL = (Get-Content $fixtureURLFile -Raw).Trim()
  $fixturePort = ([uri]$fixtureURL).Port
  $source = @{name="Shutdown fixture";url=$fixtureURL;kind="feed";enabled=$true;contentPermission="metadata_only";adapterConfig=@{format="rss"};scraperPolicy=@{status="not_applicable";termsUrl=$null;robotsUrl=$null;reviewedAt=$null;reviewNotes=$null}} | ConvertTo-Json -Depth 5
  Record "preparing bounded active-refresh shutdown fixture"
  Invoke-RestMethod -TimeoutSec 5 -Method Post -ContentType "application/json" -Body $source "http://127.0.0.1:$port/api/v1/sources" | Out-Null
  # The loopback destinations are intentionally rejected very quickly by the
  # SSRF boundary. Keep enough queued work that even fast desktop hardware
  # still has active coordinator work when the control event arrives.
  $sourceCount = 1024
  for ($index = 1; $index -lt $sourceCount; $index++) {
    $queuedSource = @{name="Queued shutdown fixture $index";url="http://127.0.0.1:$fixturePort/feed-$index";kind="feed";enabled=$true;contentPermission="metadata_only";adapterConfig=@{format="rss"};scraperPolicy=@{status="not_applicable";termsUrl=$null;robotsUrl=$null;reviewedAt=$null;reviewNotes=$null}} | ConvertTo-Json -Depth 5
    Invoke-RestMethod -TimeoutSec 5 -Method Post -ContentType "application/json" -Body $queuedSource "http://127.0.0.1:$port/api/v1/sources" | Out-Null
    if ($index % 128 -eq 0) { Record "prepared $index of $sourceCount shutdown sources" }
  }
  $refreshRun = Invoke-RestMethod -TimeoutSec 5 -Method Post "http://127.0.0.1:$port/api/v1/refresh"
  if ($refreshRun.status -ne "running") { throw "refresh did not enter running state before shutdown" }
  Stop-App $app $stopFile
  Record "graceful console shutdown completed with active refresh"

  $env:NEWS_AGGREGATOR_PORT = [string]$port
  $env:PATH = $emptyPath
  $unavailableLog = Join-Path $logs "browser-unavailable.log"
  $restartPIDFile = Join-Path $logs "restart.pid"
  $restartStopFile = Join-Path $logs "restart.stop"
  $restart = Start-Process -FilePath $controlExe -ArgumentList "`"$restartPIDFile`"", "`"$restartStopFile`"", "`"$exe`"" -PassThru -RedirectStandardError $unavailableLog
  $env:PATH = $originalPath
  Wait-Ready $port $restart
  if ((Get-Content $unavailableLog -Raw) -notmatch "default browser unavailable") { throw "browser-unavailable boundary was not handled" }
  $recoveredRun = Invoke-RestMethod "http://127.0.0.1:$port/api/v1/refresh/$($refreshRun.id)"
  $recoveredRun | ConvertTo-Json -Depth 5 | Set-Content (Join-Path $logs "shutdown-refresh.json")
  $cancelledOutcomes = @($recoveredRun.outcomes | Where-Object { $_.errorCode -eq "cancelled" })
  if ($recoveredRun.status -eq "running" -or -not $recoveredRun.finishedAt -or $recoveredRun.outcomes.Count -ne $sourceCount -or $cancelledOutcomes.Count -eq 0) {
    throw "active refresh was not durably finalized during graceful shutdown"
  }
  Stop-App $restart $restartStopFile
  Record "current database restart, finalized refresh, and browser-unavailable path passed"

  Record "running native migration and Credential Manager checks"
  go test ./tests/integration -run 'TestMigrationCompatibilityMatrix/(empty_and_current|interrupted_migration_is_atomic_and_retryable|newer_schema_rejected)' -count=1 -v | Out-File -Append -FilePath $smokeLog
  if ($LASTEXITCODE -ne 0) { throw "native migration checks failed with exit code $LASTEXITCODE" }
  $env:NEWS_AGGREGATOR_CREDENTIAL_SENTINEL = $CredentialSentinel
  go test ./tests/integration -run 'TestWindowsCredential(Lifecycle|AccessDeniedIsSafe)' -count=1 -v | Out-File -Append -FilePath $smokeLog
  if ($LASTEXITCODE -ne 0) { throw "native credential checks failed with exit code $LASTEXITCODE" }
  $env:NEWS_AGGREGATOR_CREDENTIAL_SENTINEL = $null

  $openLauncherLogs = @((Join-Path $OutputRoot "restricted-stdout.log"), (Join-Path $OutputRoot "restricted-stderr.log"))
  $scanFiles = Get-ChildItem -Path $OutputRoot -Recurse -File | Where-Object { $_.FullName -ne $smokeLog -and $_.FullName -notin $openLauncherLogs }
  foreach ($file in $scanFiles) {
    $bytes = [System.IO.File]::ReadAllBytes($file.FullName)
    $text = [System.Text.Encoding]::UTF8.GetString($bytes)
    if ($text.Contains($CredentialSentinel)) { throw "credential sentinel leaked into $($file.FullName)" }
  }
  Record "credential sentinel absent from database, logs, executable, and artifacts"
  Record "decision=APPROVE"
} finally {
  if ($app -and -not $app.HasExited) { Stop-Process -Id $app.Id -Force }
  if ($restart -and -not $restart.HasExited) { Stop-Process -Id $restart.Id -Force }
  if ($fixtureProcess -and -not $fixtureProcess.HasExited) { Stop-Process -Id $fixtureProcess.Id -Force }
  $env:APPDATA,$env:NEWS_AGGREGATOR_PORT,$env:PATH = $originalAppData,$originalPort,$originalPath
  Pop-Location
}
