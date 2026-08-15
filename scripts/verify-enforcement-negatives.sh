#!/usr/bin/env bash
set -euo pipefail

temporary_directory="$(mktemp -d)"
openapi_file="api/openapi.yaml"
batch_file="assembly/generated/task_batches/frontend-workflows.json"
collaboration_file="assembly/generated/collaboration_state.json"
if ! git diff --quiet -- "$openapi_file" "$batch_file" "$collaboration_file"; then
  echo 'RESULT FAIL negative enforcement requires clean contract and lifecycle inputs'
  exit 1
fi
cleanup() {
  cp "$temporary_directory/openapi.yaml" "$openapi_file"
  cp "$temporary_directory/frontend-workflows.json" "$batch_file"
  cp "$temporary_directory/collaboration_state.json" "$collaboration_file"
  rm -rf "$temporary_directory"
}
trap cleanup EXIT
cp "$openapi_file" "$temporary_directory/openapi.yaml"
cp "$batch_file" "$temporary_directory/frontend-workflows.json"
cp "$collaboration_file" "$temporary_directory/collaboration_state.json"

node -e "const fs=require('fs');const p='$openapi_file';const v=JSON.parse(fs.readFileSync(p));v.info.title='News Aggregator Contract Drift';fs.writeFileSync(p,JSON.stringify(v,null,2)+'\n')"
if node scripts/generate-api-types.mjs --check >/dev/null 2>&1; then
  echo 'RESULT FAIL generated bindings accepted semantic OpenAPI drift'
  exit 1
fi
cp "$temporary_directory/openapi.yaml" "$openapi_file"
echo 'NEGATIVE OK generated bindings reject semantic OpenAPI drift'

node -e "const fs=require('fs');const p='$batch_file';const v=JSON.parse(fs.readFileSync(p));v.tasks[0].depends_on.push('UNKNOWN-NEGATIVE-TASK');fs.writeFileSync(p,JSON.stringify(v,null,2)+'\n')"
if python3 assembly/tools/validate_task_batches.py project_workspace.json >/dev/null 2>&1; then
  echo 'RESULT FAIL task-batch validator accepted an unknown dependency'
  exit 1
fi
cp "$temporary_directory/frontend-workflows.json" "$batch_file"
echo 'NEGATIVE OK task-batch validator rejects unknown dependencies'

node -e "const fs=require('fs');const p='$collaboration_file';const v=JSON.parse(fs.readFileSync(p));v.task_assignments[0].task_id='UNKNOWN-NEGATIVE-TASK';fs.writeFileSync(p,JSON.stringify(v,null,2)+'\n')"
if python3 assembly/tools/validate_collaboration_state.py >/dev/null 2>&1; then
  echo 'RESULT FAIL lifecycle validator accepted an unknown task reference'
  exit 1
fi
cp "$temporary_directory/collaboration_state.json" "$collaboration_file"
echo 'NEGATIVE OK lifecycle validator rejects unknown task references'

zero_tests="$temporary_directory/zero-tests.json"
printf '%s\n' '{"Time":"2026-08-15T00:00:00Z","Action":"start","Package":"example/empty"}' '{"Time":"2026-08-15T00:00:00Z","Action":"pass","Package":"example/empty","Elapsed":0}' > "$zero_tests"
if node scripts/assert-go-tests-ran.mjs "$zero_tests" >/dev/null 2>&1; then
  echo 'RESULT FAIL integration discovery accepted zero tests'
  exit 1
fi
echo 'NEGATIVE OK integration discovery rejects zero tests'
echo 'RESULT OK verification_negative_enforcement=true'
