# Kinora audit (phase 0)

Code review of v1.0.30 (`77a2f41`), done before the reliability work. Each finding names the code, the failure
scenario and a proposed fix. Severity: **High** = data loss or privilege escalation without unusual
preconditions; **Medium** = a real bug or abuse path with a plausible trigger; **Low** = hardening, edge cases.

No High finding was confirmed. Several areas were checked and look sound; see "Checked, no finding" at the end.

## Status after phase 1

| Finding | Status | Change |
|---|---|---|
| M1 CSRF | fixed | `csrfGuard` drops the session cookie from state-changing requests marked `Sec-Fetch-Site: same-site/cross-site`; Bearer requests unaffected (`internal/server/cors.go`) |
| M2 shared downloads | fixed | a job keeps every requesting user (`Job.Users`); the file stays shared |
| M3 transcode limits | fixed | max. 8 sessions per account (the least recently used stops), optional server-wide cap `KINORA_MAX_TRANSCODES` (HTTP 503) |
| M4 blocking queue | fixed | non-blocking enqueue (HTTP 429 when the queue is full), max. 10 pending downloads per user |
| M5 newer database | fixed | refuses to start; `KINORA_ALLOW_NEWER_DB=1` overrides |
| M6 panics | fixed | `internal/safe`: background loops restart with backoff; scanner files, indexer searches and metadata refreshes recover per item |
| M7 error details | fixed | clients get a reference id, the log has the details |
| L1 artwork without profile | fixed | the account's library restriction applies |
| L2 folder removal | fixed | known sidecars (NFO, artwork) go with the film, unknown files and their folder stay |
| L3 path mapping | fixed | a mapped path outside the local folder fails the import |
| L4 retention | fixed | counted from `DoneAt` |
| L5 kids subtitles | fixed | `notKids` on subtitle download/delete |
| L6 first-run setup | open | to be documented in the install guides (phase 6/8) |
| L7 DSN | fixed | `?`/`#` in the database path is refused with a clear message |

Fuzzing (phase 1) found three more bugs, fixed with regression inputs under `testdata/fuzz/`:
- `ParseFile`: "S01E07E01" was stored as the episode range 7–1;
- `ReadNFO`: a negative `<tmdbid>` was accepted (now ignored); NFO files over 4 MB are no longer read into memory;
- Cardigann `ParseSize`: a huge size ("9999999999999 PB") wrapped to a negative number, which passes every size limit.

CI gap found while adding the problem media: the `test` job did not generate `testmedia/`, so every API test that
needs the test library skipped itself in CI. The job now generates both libraries and fails when a test skips for
missing media.

## Medium

### M1. CSRF from same-site origins with the session cookie
`internal/server/cors.go` (`cors`): a cross-origin request that is not from an app origin is passed on with its
`Cookie` header intact ("no CORS headers: the browser will not expose the response"). The browser cannot read
the response, but the request still runs. `SameSite=Lax` stops cross-*site* POSTs, yet a page on the same
host with another port (`http://192.168.1.10:8080` next to Kinora on `:8096`) or a sibling subdomain is
same-*site*, so the cookie is sent. `decode()` ignores `Content-Type`, so a `text/plain` form POST carrying JSON
is a "simple request" with no preflight.
**Scenario:** an admin signed in to the web client opens a page served by another app on the same NAS; that page
posts to `/api/admin/users` and creates an admin account, or to `/api/admin/update-apply`.
**Fix:** for cookie-authenticated, state-changing methods (POST/PUT/PATCH/DELETE), reject the request when an
`Origin` (or, when missing, `Sec-Fetch-Site: cross-site|same-site`) header shows a foreign origin. Bearer-token
requests are unaffected.

### M2. Offline download jobs are shared across users, but only the first user can see them
`internal/media/download.go` (`JobKey`, `Enqueue`): the job key covers item, file, quality and audio track, not
the user. When user B requests the same download as user A, `Enqueue` returns A's job (with `UserID = A`).
`handleDownloads` filters by user and `handleDownloadFile` checks `j.UserID != c.User.ID`, so B never sees the job
and gets 404 for the file.
**Fix:** keep the transcoded file shared (it is the expensive part), but track the requesting users per job
(set of user ids), and check membership instead of equality.

### M3. Unlimited concurrent transcodes per user
`internal/media/transcode.go` (`Manager.Start`), `internal/server/play.go` (`handlePlayback`): only the
`prev_session` passed by the client is stopped. Any signed-in profile (kids too) can start any number of ffmpeg
processes by calling `/api/items/{id}/playback` repeatedly; they live until the reaper sees them idle.
**Scenario:** a buggy client retry loop, or a malicious user, saturates CPU/GPU for everyone.
**Fix:** a per-user cap (e.g. 3 sessions; the oldest is stopped) and an admin-configurable global cap with a
clear error message.

### M4. The download queue can block request handlers
`internal/media/download.go` (`Enqueue`): `d.queue <- j` on a channel with capacity 256, drained by one worker
that runs one long ffmpeg job at a time. With 256 queued jobs the next `POST /api/items/{id}/download` blocks
until a job finishes (possibly hours), holding the connection. There is no per-user limit.
**Fix:** non-blocking send with an error ("queue full"), plus a per-user queued-job limit.

### M5. A newer database is opened by an older binary without warning
`internal/store/db.go` (`migrate`): if `schema_version` is greater than `len(migrations)` (after rolling back a
self-update or running an older image on the same volume), the loop does nothing and the server runs against a
schema it does not know. Inserts can fail later, or worse, write rows that the newer version misreads.
**Fix:** refuse to start with a clear message ("the database was created by a newer Kinora version; restore a
backup or update"), with an explicit override flag for experts.

### M6. A panic in a background goroutine stops the whole server
`net/http` recovers panics inside handlers, but the scanner, acquisition poller, series monitor, intro
analyzer, push notifier and download worker run in plain goroutines started from `main`. A panic there (for
example from an indexer definition the admin uploaded, or an unexpected NFO) terminates the process.
**Fix:** a small `safeGo(name, fn)` helper that recovers, logs the stack and restarts the loop with backoff;
fuzz the parsers that feed these loops (phase 1).

### M7. Raw internal errors are returned to clients
`internal/server/server.go` (`internalErr`): `"belső hiba: " + err.Error()` goes to the client, which can include
file system paths, SQL fragments or upstream URLs. `handleStream` also returns the `os.Open` error text.
**Fix:** log the details, return a generic message plus a short request id that appears in the log line.

## Low

### L1. Artwork of hidden items is served to a session without a selected profile
`internal/server/images.go`: when the session has no profile (and no profile lock), `filter` stays `nil` and no
visibility check runs, so `/api/images/{id}/{kind}` returns artwork for any item id, including items outside the
account's allowed libraries. Only artwork leaks, not metadata, but the README states that the filter applies to
images on every endpoint.
**Fix:** without a profile, apply at least the account's library restriction (`LibrarySQL`), or require a profile
like the other item endpoints.

### L2. Removing an imported film deletes its whole folder once no video is left
`internal/acquire/remove.go` (`deleteImported`): after removing the video and its subtitles,
`os.RemoveAll(dir)` runs when the folder holds no other video. NFO files, artwork, extras without a video
extension and anything the user put there are deleted too. The folder is usually created by Kinora, but import
also reuses an existing folder with the same name.
**Fix:** remove only files Kinora created or that belong to the removed video (same base name), then `os.Remove`
the folder only if it is empty.

### L3. Download-client paths can step outside the mapped folder
`internal/acquire/manager.go` (`MapPath`): `filepath.Join(local, rest)` cleans `..` components, so a content path
like `<remote>/../../etc` reported by the download client maps outside `local`. The download client is
configured by the admin, so this needs a compromised or misbehaving client.
**Fix:** after joining, check with `filepath.Rel` that the result stays inside `local`; otherwise fail the import.

### L4. Offline downloads expire by creation time
`internal/media/download.go` (`prune`): retention is measured from `CreatedAt`, not `DoneAt`. A job that waited
in the queue for days is deleted soon after it finishes.
**Fix:** use `DoneAt` for finished jobs.

### L5. Kids profiles can download subtitles
`internal/server/routes.go`: `POST /api/items/{id}/subtitles/download` and the delete endpoint are `withProfile`
without `notKids`. The downloaded subtitle is attached to the item for every profile.
**Fix:** wrap both with `notKids`, as the other write endpoints are.

### L6. First-run setup is open to whoever reaches the server first
`POST /api/setup` creates the first admin while no user exists. On an instance exposed to the internet before
setup, anyone could claim it. This is the usual trade-off for self-hosted software.
**Fix (docs + optional):** say it clearly in the install docs; optionally accept setup only from private
addresses unless `KINORA_ALLOW_REMOTE_SETUP=1`.

### L7. Database path is concatenated into the DSN
`internal/store/db.go` (`Open`): `path + "?_journal_mode=…"`. A data directory containing `?` breaks opening the
database. Rare, but the error message would be confusing.
**Fix:** build the DSN with `file:` + `url.PathEscape` or reject such paths with a clear message.

## Technical debt

- **CI** already runs `go vet`, `go test -race`, a container smoke test and the image builds
  (`.github/workflows/ci.yml`). Missing: `staticcheck`, `govulncheck`, fuzz targets, and the `apps/shared` tests on
  every push (`apps.yml` runs only when `apps/**` changes). The Docker build already runs on PRs (smoke job).
- **Server error messages are Hungarian only.** Most `writeErr` strings are Hungarian literals; English/German
  clients show them untranslated. Error codes (stable keys) plus client-side translation would fix this.
- **Large single files:** `web/static/app.js`, `internal/acquire/cardigann/engine.go`, `internal/scanner/scanner.go`,
  `internal/meta/tmdb.go` (850+ lines each). Not a bug, but it makes review harder.
- **No fuzzing** of the parsers that read untrusted input: file names, NFO XML, Cardigann YAML/templates,
  release names. These are the most likely sources of panics (M6).
- **Test media** is generated by `scripts/mkmedia.sh` for the smoke test only; there are no integration tests
  for HDR metadata, many audio tracks, PGS subtitles, VFR or truncated files.
- **Documentation:** no `CONTRIBUTING.md` or `SECURITY.md`; the README's "better than Jellyfin" table contains
  claims about Jellyfin that are not accurate (Jellyfin has Quick Connect, parental controls, SyncPlay and a
  Next Up row). Handled in phase 8.

## Checked, no finding

- SQL: all queries use parameters; the dynamic parts (`rules.go` ORDER BY and field expressions, `LibrarySQL`
  integer lists) come from whitelists or integers.
- Passwords: PBKDF2-SHA256, 210 000 iterations, constant-time compare. Session tokens are stored hashed.
- Rate limits on login, join, PIN and Quick Connect include a per-peer limit, so a forged `X-Forwarded-For`
  from the LAN does not remove the cap.
- Item endpoints (details, children, playback, stream, subtitles, segments, scores, language choices, downloads,
  parties) load items through profile-aware store calls or `ItemVisible`.
- HLS file names are validated with a strict regexp; sessions are bound to the user.
- Import and removal check that targets stay inside the library root (`filepath.Rel`); symlinks are skipped while
  walking downloads.
- Outbound HTTP bodies (indexers, clients, TMDB, OpenSubtitles, relay) are read through `io.LimitReader`.
- WebSocket origin check for parties; admin secrets are masked in settings and client responses.

## Baseline test run

Run on 2026-11-23 against `77a2f41`, inside the dev container (`scripts/go.sh`):

- `go vet ./...`: clean.
- `go test -race -count=1 ./...`: all 11 packages with tests pass (`internal/server` is the slowest at ~121 s).
- `apps/` `npm test` (shared unit tests): 11/11 pass.
