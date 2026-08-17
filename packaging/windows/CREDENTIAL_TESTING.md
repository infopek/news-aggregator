# Windows Credential Manager verification

Run from an unprivileged Windows 11 account:

```powershell
go test ./tests/integration -run TestWindowsCredentialLifecycle -count=1 -v
```

The test creates a uniquely named generic credential under the
`infopek.news-aggregator.v1` namespace, verifies create/resolve/replace/delete,
and registers best-effort cleanup before its first write. It never prints the
credential values. A Linux cross-build proves the Windows implementation
compiles, but does not substitute for this native runtime test.

## Naming and lifecycle

Production entries use the stable `infopek.news-aggregator.v1` namespace and
an opaque hash derived from the immutable source ID. Renaming a source cannot
change its reference. Reconfiguration uses Credential Manager's replacement
write, and source deletion derives and deletes only that source's entry.
Missing, denied, cancelled, and unavailable operations are mapped to safe
sentinel errors without target names or native error text.

## Complete native smoke

From a normal, non-elevated PowerShell session at the repository root:

```powershell
.\packaging\windows\smoke.ps1
```

The script builds into a path containing spaces, records the executable
SHA-256, launches with isolated AppData, verifies the embedded SPA and API,
checks the owning process's listening sockets, records one browser-launch
request, exercises occupied-port and unavailable-browser behavior, restarts
the current database, runs native migration and Credential Manager checks,
and scans generated files for a randomized credential sentinel. GitHub Actions
publishes the executable and evidence directory as `windows-native-<sha>`.
