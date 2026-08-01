$ErrorActionPreference = "Stop"

node scripts/verify-migrations.mjs
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
