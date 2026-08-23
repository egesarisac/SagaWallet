#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "usage: $0 <module-directory> <minimum-coverage-percent>" >&2
  exit 2
fi

module="$1"
minimum="$2"
repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
module_directory="${repository_root}/${module}"

if [[ ! -f "${module_directory}/go.mod" ]]; then
  echo "module does not contain go.mod: ${module}" >&2
  exit 2
fi

cd "${module_directory}"
go test -race -covermode=atomic -coverpkg=./... -coverprofile=coverage.out ./...

coverage="$(go tool cover -func=coverage.out | awk '/^total:/ { gsub(/%/, "", $3); print $3 }')"
if [[ -z "${coverage}" ]]; then
  echo "unable to determine coverage for ${module}" >&2
  exit 1
fi

awk -v actual="${coverage}" -v required="${minimum}" -v module="${module}" '
  BEGIN {
    printf "%s coverage: %.1f%% (minimum %.1f%%)\n", module, actual, required
    if (actual + 0.0001 < required) {
      exit 1
    }
  }
'
