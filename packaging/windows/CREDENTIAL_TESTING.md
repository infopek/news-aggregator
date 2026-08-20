# Windows Credential Manager verification

Run from an unprivileged Windows 11 account:

```powershell
go test ./tests/integration -run 'TestWindowsCredential(Lifecycle|AccessDeniedIsSafe)' -count=1 -v
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

The hosted `windows-2025` job is Windows Server evidence. It does not replace
the final Windows 11 acceptance run. From a normal, non-elevated PowerShell 7
session on Windows 11 at the repository root, run:

```powershell
.\packaging\windows\windows11-acceptance.ps1
```

The wrapper refuses any OS other than Windows 11 and any elevated token. The
smoke builds into a path containing spaces, records the executable SHA-256,
launches with isolated AppData, verifies the embedded SPA and API, checks the
owning process's listening sockets, records one browser-launch request,
exercises occupied-port and unavailable-browser behavior, sends a real console
control event while a refresh is active, verifies durable cancellation after
restart, runs fresh/current/interrupted/newer migration and Credential Manager
checks, and scans generated files for a randomized credential sentinel.

GitHub Actions publishes the Windows Server diagnostic artifact as
`windows-native-<sha>`. Preserve the Windows 11 output directory separately as
the final VERIFY-003 acceptance artifact; its `native-smoke.log` records the
exact revision, checksum, token decision, and APPROVE/REJECT boundary.
