#!/usr/bin/env bash
set -euo pipefail

python3 assembly/tools/validate_task_batches.py project_workspace.json
python3 assembly/tools/validate_collaboration_state.py
