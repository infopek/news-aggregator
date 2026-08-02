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
