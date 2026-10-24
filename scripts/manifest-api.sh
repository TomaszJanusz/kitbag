#!/usr/bin/env bash
# apidiff can report API changes with a zero exit status; require empty output.
set -euo pipefail

changes=$(apidiff docs/api/manifest.export github.com/TomaszJanusz/kitbag/pkg/manifest)
if [[ -n "$changes" ]]; then
  printf '%s\n' "$changes"
  printf '%s\n' 'Review the public API change, then run mise run manifest:api:update.' >&2
  exit 1
fi
