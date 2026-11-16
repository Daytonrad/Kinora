---
name: verify
description: Verify Kinora before a commit or at the end of a milestone - runs go vet, go test (inside the Docker dev container, no local Go needed) and the container smoke test scripts/smoke.sh. Use whenever code changed and before every commit.
---

# Kinora verify

Run these from the repo root, in order, and stop at the first failure. Go is **not** installed on the
host; `scripts/go.sh` runs the toolchain in the `kinora-dev` image (golang:1.24-bookworm + ffmpeg).

1. `scripts/go.sh go vet ./...`
2. `scripts/go.sh go test -count=1 ./...`
   - integration tests need the generated library: run `scripts/mkmedia.sh testmedia` first if `testmedia/.complete` is missing.
3. `scripts/smoke.sh` – builds the runtime image, runs it with `testmedia` mounted read-only at `/media`
   and walks setup → library → scan → playback decision → HLS segment over curl. Must end with `SMOKE OK`.
   Set `SMOKE_LOGS=1` to always print container logs.

If Docker Desktop is not running on Windows, start it with
`"/c/Program Files/Docker/Docker/Docker Desktop.exe" &` and wait until `docker info` reports CPUs.

Report: the vet/test summary lines, the smoke ✓/✗ list, and any failure output verbatim.
A milestone is only done when all three are green.
