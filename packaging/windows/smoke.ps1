param(
  [string]$OutputRoot = (Join-Path $env:RUNNER_TEMP "News Aggregator Native Smoke")
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"
$repo = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$build = Join-Path $OutputRoot "Build With Spaces"
$data = Join-Path $OutputRoot "User Data"
$logs = Join-Path $OutputRoot "Evidence"
$probe = Join-Path $OutputRoot "Browser Probe"
$emptyPath = Join-Path $OutputRoot "Empty Path"
$exe = Join-Path $build "news-aggregator.exe"
$probeExe = Join-Path $probe "rundll32.exe"
$browserLog = Join-Path $logs "browser-launch.log"
$runtimeLog = Join-Path $logs "runtime.log"
$runtimeError = Join-Path $logs "runtime-error.log"
$smokeLog = Join-Path $logs "native-smoke.log"
$sentinel = "VERIFY003-$([guid]::NewGuid().ToString('N'))"
$app,$restart = $null,$null
$originalAppData,$originalPort,$originalPath = $env:APPDATA,$env:NEWS_AGGREGATOR_PORT,$env:PATH

New-Item -ItemType Directory -Force -Path $build,$data,$logs,$probe,$emptyPath | Out-Null
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

function Stop-App([System.Diagnostics.Process]$Process) {
  if ($Process.HasExited) { return }
  Stop-Process -Id $Process.Id
  if (-not $Process.WaitForExit(10000)) { throw "application did not stop within ten seconds" }
}

Push-Location $repo
try {
  Record "building frontend"
  npm --prefix web ci | Out-File -Append -FilePath $smokeLog
  npm --prefix web run build | Out-File -Append -FilePath $smokeLog
  Record "building executable and browser probe from path containing spaces"
  go build -trimpath -o $exe ./cmd/news-aggregator
  go build -trimpath -o $probeExe ./packaging/windows/browser-probe
  $hash = (Get-FileHash -Algorithm SHA256 $exe).Hash.ToLowerInvariant()
  "${hash}  news-aggregator.exe" | Set-Content (Join-Path $logs "news-aggregator.exe.sha256")
  Record "sha256=$hash"

  whoami /groups | Out-File (Join-Path $logs "process-token-groups.txt")
  $port = Free-Port
  $env:APPDATA = $data
  $env:NEWS_AGGREGATOR_PORT = [string]$port
  $env:NEWS_AGGREGATOR_BROWSER_PROBE_LOG = $browserLog
  $env:PATH = "$probe;$originalPath"
  $app = Start-Process -FilePath $exe -PassThru -RedirectStandardOutput $runtimeLog -RedirectStandardError $runtimeError
  $env:PATH = $originalPath
  Wait-Ready $port $app
  Record "ready pid=$($app.Id) port=$port"

  $health = Invoke-RestMethod "http://127.0.0.1:$Port/api/v1/health"
  if ($health.status -ne "ready") { throw "health contract was not ready" }
  $spa = Invoke-WebRequest -UseBasicParsing "http://127.0.0.1:$Port/settings"
  if ($spa.StatusCode -ne 200 -or $spa.Content -notmatch '<div id="app">') { throw "embedded SPA route failed" }
  $listeners = Get-NetTCPConnection -State Listen -OwningProcess $app.Id
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

  Stop-App $app
  $env:NEWS_AGGREGATOR_PORT = [string]$port
  $env:PATH = $emptyPath
  $unavailableLog = Join-Path $logs "browser-unavailable.log"
  $restart = Start-Process -FilePath $exe -PassThru -RedirectStandardError $unavailableLog
  $env:PATH = $originalPath
  Wait-Ready $port $restart
  if ((Get-Content $unavailableLog -Raw) -notmatch "default browser unavailable") { throw "browser-unavailable boundary was not handled" }
  Stop-App $restart
  Record "current database restart and browser-unavailable path passed"

  Record "running native migration and Credential Manager checks"
  go test ./tests/integration -run 'TestMigrationCompatibilityMatrix/newer_schema_rejected' -count=1 -v | Out-File -Append -FilePath $smokeLog
  $env:NEWS_AGGREGATOR_CREDENTIAL_SENTINEL = $sentinel
  go test ./tests/integration -run TestWindowsCredentialLifecycle -count=1 -v | Out-File -Append -FilePath $smokeLog
  $env:NEWS_AGGREGATOR_CREDENTIAL_SENTINEL = $null

  $scanFiles = Get-ChildItem -Path $OutputRoot -Recurse -File | Where-Object { $_.FullName -ne $smokeLog }
  foreach ($file in $scanFiles) {
    $bytes = [System.IO.File]::ReadAllBytes($file.FullName)
    $text = [System.Text.Encoding]::UTF8.GetString($bytes)
    if ($text.Contains($sentinel)) { throw "credential sentinel leaked into $($file.FullName)" }
  }
  Record "credential sentinel absent from database, logs, executable, and artifacts"
  Record "decision=APPROVE"
} finally {
  if ($app) { Stop-App $app }
  if ($restart) { Stop-App $restart }
  $env:APPDATA,$env:NEWS_AGGREGATOR_PORT,$env:PATH = $originalAppData,$originalPort,$originalPath
  Pop-Location
}
