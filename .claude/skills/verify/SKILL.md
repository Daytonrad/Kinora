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

4. Optional, for UI changes: start a fresh container with the test library
   (`docker run -d --name kinora-e2e -p 18097:8096 -v "$(pwd -W)/testmedia:/media:ro" kinora:smoke`) and run
   `BASE=http://127.0.0.1:18097 node scripts/e2e.mjs` from a folder where `npm i playwright` was done
   (it drives the system Edge, because Playwright's Chromium has no H.264/AAC). Look at the screenshots it writes.

5. Client apps (when `apps/` or the web tokens/fonts changed), from `apps/` after `npm install`:
   - `npm test` – unit tests of the shared app (address parsing, WebVTT, capability table) – must end with `# fail 0`.
   - `npm run build:shared` – TypeScript check + ES2015 classic-script bundle (fails on type errors).
   - App E2E against a server with the test library and an admin `admin`/`secret123`:
     `docker run -d --name kinora-apps -p 18098:8096 -v "$(pwd -W)/testmedia:/media:ro" kinora:smoke`, run setup + add the
     `/media/Movies` and `/media/Shows` libraries, then `SERVER=http://localhost:18098 npm run e2e -w shared`. It drives the TV UI from
     `file://` at 1920×1080 with arrow keys (Quick Connect login, facade follows focus, direct play, ±10 s, back/exit) and the phone UI at
     390×844; must end with `APP E2E OK`. Look at `apps/shared/test/out/*.png`.
   - Packaging: `npm run build:webos` (→ `.ipk`), `npm run build:tizen` (→ `.wgt`, unsigned without the Tizen CLI),
     `npm run build:android` (→ `apps/android/dist/kinora.apk`; Gradle runs in the `kinora-android-build` Docker image when no local SDK).
   - iOS only compiles on macOS: CI job `ios` in `.github/workflows/apps.yml`.

If Docker Desktop is not running on Windows, start it with
`"/c/Program Files/Docker/Docker/Docker Desktop.exe" &` and wait until `docker info` reports CPUs.

Report: the vet/test summary lines, the smoke ✓/✗ list, and any failure output verbatim.
A milestone is only done when steps 1–3 are green (and step 5 when the apps changed).
