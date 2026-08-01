$ErrorActionPreference = "Stop"

python scripts/verify-migrations.py
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
