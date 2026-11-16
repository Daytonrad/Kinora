#!/usr/bin/env bash
# Kinora container smoke test.
#   Builds the image, starts it with the generated test library under /media,
#   then walks the main API flows with curl.
#
#   scripts/smoke.sh            # build + run in Docker (the milestone gate)
#   SMOKE_NATIVE=1 scripts/smoke.sh   # run against a native binary instead (no Docker)
set -euo pipefail
cd "$(dirname "$0")/.."
ROOT="$(pwd)"
PY=""
for c in python3 python; do
    if "$c" -c 'import json' >/dev/null 2>&1; then PY="$c"; break; fi
done
[ -n "$PY" ] || { echo "python is required for JSON parsing"; exit 1; }
PORT="${SMOKE_PORT:-18096}"
BASE="http://127.0.0.1:$PORT"
NAME=kinora-smoke
IMAGE=kinora:smoke
TMP="$(mktemp -d)"
JAR="$TMP/cookies"
FAILS=0

cleanup() {
    if [ -z "${SMOKE_NATIVE:-}" ]; then
        if [ "$FAILS" != 0 ] || [ -n "${SMOKE_LOGS:-}" ]; then docker logs --tail 80 "$NAME" 2>&1 | sed 's/^/  [kinora] /' || true; fi
        docker rm -f "$NAME" >/dev/null 2>&1 || true
    else
        [ -n "${NPID:-}" ] && kill "$NPID" 2>/dev/null || true
    fi
    rm -rf "$TMP"
}
trap cleanup EXIT

say()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
pass() { printf '  \033[32m✓\033[0m %s\n' "$*"; }
fail() { printf '  \033[31m✗ %s\033[0m\n' "$*"; FAILS=$((FAILS+1)); }
die()  { fail "$*"; exit 1; }

# json <expr> : evaluates a python expression on stdin JSON bound to `j`
json() { "$PY" -c "import json,sys; j=json.load(sys.stdin); v=($1); print(v if not isinstance(v,(dict,list)) else json.dumps(v))"; }

api() { # api METHOD PATH [BODY]  -> body on stdout, status in $STATUS
    local m="$1" p="$2" b="${3:-}"
    local args=(-sS -o "$TMP/body" -w '%{http_code}' -X "$m" -b "$JAR" -c "$JAR" -H 'Content-Type: application/json')
    [ -n "$b" ] && args+=(--data "$b")
    curl "${args[@]}" "$BASE$p" >"$TMP/status" || echo 000 >"$TMP/status"
    STATUS="$(cat "$TMP/status")"
    cat "$TMP/body"
}
expect() { # expect STATUS description (status file survives pipelines/subshells)
    STATUS="$(cat "$TMP/status")"
    if [ "$STATUS" = "$1" ]; then pass "$2"; else fail "$2 (HTTP $STATUS: $(head -c 300 "$TMP/body"))"; fi
}

# ------------------------------------------------------------------ media
if [ ! -f testmedia/.complete ]; then
    say "generating test media"
    scripts/mkmedia.sh testmedia
fi

# ------------------------------------------------------------------ start
if [ -z "${SMOKE_NATIVE:-}" ]; then
    say "building image"
    docker build -q --target runtime --build-arg VERSION=smoke -t "$IMAGE" . >/dev/null
    docker rm -f "$NAME" >/dev/null 2>&1 || true
    media="$ROOT/testmedia"
    if command -v cygpath >/dev/null; then media="$(cygpath -w "$media")"; fi
    say "starting container"
    MSYS_NO_PATHCONV=1 docker run -d --name "$NAME" -p "$PORT:8096" \
        -e TZ=Europe/Budapest -e PUID="${SMOKE_PUID:-1000}" -e PGID="${SMOKE_PGID:-1000}" -e KINORA_LOG_LEVEL=debug \
        -v "$media:/media:ro" "$IMAGE" >/dev/null
    MEDIA=/media
else
    say "starting native binary"
    BIN="${SMOKE_BIN:-$TMP/kinora}"
    [ -x "$BIN" ] || go build -o "$BIN" ./cmd/kinora
    "$BIN" --addr "127.0.0.1:$PORT" --data "$TMP/data" >"$TMP/log" 2>&1 &
    NPID=$!
    MEDIA="$ROOT/testmedia"
fi

for i in $(seq 1 60); do
    curl -fsS "$BASE/api/status" >/dev/null 2>&1 && break
    sleep 1
    [ "$i" = 60 ] && die "server did not come up"
done
pass "server is up"

if [ -z "${SMOKE_NATIVE:-}" ]; then
    for i in $(seq 1 40); do
        h="$(docker inspect -f '{{.State.Health.Status}}' "$NAME")"
        [ "$h" = healthy ] && break
        sleep 1
    done
    [ "$h" = healthy ] && pass "container healthy" || fail "container health: $h"
    u="$(docker exec "$NAME" stat -c %U /proc/1)"
    [ "$u" = kinora ] && pass "runs as non-root user kinora" || fail "pid1 user: $u"
fi

# ------------------------------------------------------------------ M1: account
say "setup / login / profiles"
api GET /api/status | json 'j["setup_needed"]' | grep -q True && pass "setup needed on fresh install" || fail "setup_needed"
api POST /api/setup '{"server_name":"Smoke","username":"admin","password":"secret123"}' >/dev/null; expect 200 "setup creates admin"
api POST /api/setup '{"username":"x","password":"secret123"}' >/dev/null; expect 409 "second setup rejected"
api POST /api/logout >/dev/null
api GET /api/me >/dev/null; expect 401 "logged out → 401"
api POST /api/login '{"username":"admin","password":"wrong"}' >/dev/null; expect 401 "bad password rejected"
api POST /api/login '{"username":"admin","password":"secret123"}' >/dev/null; expect 200 "login"
PID="$(api GET /api/me | json 'j["profiles"][0]["id"]')"
api GET /api/libraries >/dev/null; expect 428 "no profile selected → 428"
api POST "/api/profiles/$PID/select" '{}' >/dev/null; expect 200 "select profile"
KID="$(api POST /api/profiles '{"name":"Gyerek","is_kids":true,"pin":"1234","color":"#22aa55"}' | json 'j["id"]')"; expect 201 "create kids profile"
api POST "/api/profiles/$KID/select" '{"pin":"0000"}' >/dev/null; expect 403 "wrong PIN rejected"
INV="$(api POST /api/admin/invites '{"uses":1,"hours":24}' | json 'j["code"]')"; expect 201 "create invite"
api GET /api/admin/settings | json 'j["values"]["server_name"]' | grep -q Smoke && pass "settings readable" || fail "settings"
curl -fsS -b "$JAR" -o "$TMP/backup.db" "$BASE/api/admin/backup" && head -c 15 "$TMP/backup.db" | grep -q "SQLite format" && pass "backup download" || fail "backup"

if [ -f "$ROOT/scripts/smoke_media.sh" ]; then
    # shellcheck source=/dev/null
    . "$ROOT/scripts/smoke_media.sh"
fi

# ------------------------------------------------------------------ shutdown
if [ -z "${SMOKE_NATIVE:-}" ]; then
    say "graceful shutdown"
    t0=$(date +%s)
    docker stop -t 15 "$NAME" >/dev/null
    dt=$(( $(date +%s) - t0 ))
    code="$(docker inspect -f '{{.State.ExitCode}}' "$NAME")"
    [ "$code" = 0 ] && [ "$dt" -le 11 ] && pass "SIGTERM → exit 0 in ${dt}s" || fail "shutdown exit=$code took ${dt}s"
    docker logs "$NAME" 2>&1 | grep -q '"bye"\|msg=bye' && pass "shutdown logged" || fail "no shutdown log"
fi

echo
if [ "$FAILS" = 0 ]; then printf '\033[1;32mSMOKE OK\033[0m\n'; else printf '\033[1;31mSMOKE FAILED (%d)\033[0m\n' "$FAILS"; exit 1; fi
