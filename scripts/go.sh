#!/usr/bin/env bash
# Runs the Go toolchain inside the Kinora dev container, so no local Go/gcc is needed.
#   scripts/go.sh go test ./...
#   scripts/go.sh go vet ./...
set -euo pipefail
cd "$(dirname "$0")/.."
img=kinora-dev
if ! docker image inspect "$img" >/dev/null 2>&1; then
    docker build -q --target dev -t "$img" . >/dev/null
fi
src="$(pwd -W 2>/dev/null || pwd)"
tty=""
[ -t 1 ] && tty="-t"
MSYS_NO_PATHCONV=1 exec docker run --rm $tty \
    -v "$src":/src -w /src \
    -v kinora-gomod:/go/pkg/mod -v kinora-gocache:/root/.cache/go-build \
    -e CGO_ENABLED=1 -e GOFLAGS=-buildvcs=false \
    "$img" "$@"
