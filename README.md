# News Aggregator

## Build and run on Windows

From a normal PowerShell window at the repository root:

```powershell
.\packaging\windows\build.ps1
.\build\news-aggregator.exe
```

The build wrapper uses the repository's pinned Node/npm toolchain without
changing the system installation, builds the embedded web application, and
then compiles the standalone Windows executable. Go must be installed and
available on `PATH`.
