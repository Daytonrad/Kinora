#!/usr/bin/env bash
# Runs every Go fuzz target for FUZZTIME (default 60s). A failure writes the input to the package's
# testdata/fuzz/<Target>/ directory: fix the bug and commit that file, it becomes a regression test.
#   bash scripts/fuzz.sh                       (with a local Go toolchain)
#   scripts/go.sh bash scripts/fuzz.sh          (inside the dev container)
#   FUZZTIME=10m bash scripts/fuzz.sh FuzzParseRelease
set -euo pipefail
cd "$(dirname "$0")/.."
time="${FUZZTIME:-60s}"
targets=(
  "FuzzParseFile ./internal/scanner"
  "FuzzNameHelpers ./internal/scanner"
  "FuzzParseNFO ./internal/scanner"
  "FuzzParseRelease ./internal/acquire"
  "FuzzParseProfile ./internal/acquire"
  "FuzzParseDefinition ./internal/acquire/cardigann"
  "FuzzRender ./internal/acquire/cardigann"
  "FuzzFilters ./internal/acquire/cardigann"
  "FuzzParseSize ./internal/acquire/cardigann"
)
only="${1:-}"
for t in "${targets[@]}"; do
  read -r name pkg <<<"$t"
  [[ -n "$only" && "$only" != "$name" ]] && continue
  echo "=== $name ($pkg, $time)"
  go test -run '^$' -fuzz "^${name}\$" -fuzztime "$time" "$pkg"
done
