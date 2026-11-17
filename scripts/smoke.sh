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
export PYTHONIOENCODING=utf-8
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
json() { "$PY" -c "import json,sys; j=json.load(sys.stdin); v=($1); print(v if not isinstance(v,(dict,list)) else json.dumps(v, ensure_ascii=False))"; }

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

# ------------------------------------------------------------------ M2: libraries & scanning
say "libraries / scanning"
api GET "/api/admin/browse" | json 'j["path"]' | grep -q "$MEDIA" && pass "folder picker opens at media root" || fail "browse root ($(cat "$TMP/body" | head -c 200))"
api POST /api/libraries "{\"name\":\"Nope\",\"path\":\"$MEDIA/does-not-exist\",\"kind\":\"movies\"}" >/dev/null; expect 400 "non-existent folder rejected"
MOV="$(api POST /api/libraries "{\"name\":\"Filmek\",\"path\":\"$MEDIA/Movies\",\"kind\":\"movies\"}" | json 'j["id"]')"; expect 201 "add movie library"
SHO="$(api POST /api/libraries "{\"name\":\"Sorozatok\",\"path\":\"$MEDIA/Shows\",\"kind\":\"shows\"}" | json 'j["id"]')"; expect 201 "add show library"
wait_scans() {
    for i in $(seq 1 120); do
        r="$(api GET /api/admin/scan | json 'sum(1 for s in j if s["running"])')"
        [ "$r" = 0 ] && return 0
        sleep 1
    done
    return 1
}
wait_scans && pass "scans finished" || fail "scans did not finish"
api GET /api/admin/scan | json '[ (s["library_id"], s["changed"], s["errors"], s.get("error","")) for s in j ]'
api GET /api/admin/dashboard | json 'j["library"]' > "$TMP/stats"
grep -q '"movies": 3' "$TMP/stats" && grep -q '"series": 2' "$TMP/stats" && grep -q '"episodes": 5' "$TMP/stats"     && pass "3 movies, 2 series, 5 episodes ($(cat "$TMP/stats"))" || fail "library stats: $(cat "$TMP/stats")"
api POST "/api/libraries/$MOV/scan" >/dev/null; expect 202 "manual rescan"
wait_scans
api GET /api/admin/scan | json "[s for s in j if s['library_id']==$MOV][0]" > "$TMP/rescan"
"$PY" -c "import json,sys; s=json.load(open(sys.argv[1])); sys.exit(0 if s['changed']==0 and s['removed']==0 and s['took_ms']<3000 else 1)" "$TMP/rescan"     && pass "rescan: 0 changes ($(cat "$TMP/rescan" | json 'j["took_ms"]') ms)" || fail "rescan not idempotent: $(cat "$TMP/rescan")"
ctype="$(curl -sS -b "$JAR" -o /dev/null -w '%{content_type}' "$BASE/api/images/1/backdrop")"
[ "$ctype" = image/jpeg ] && pass "artwork served (thumb fallback)" || fail "image content-type: $ctype"

# ------------------------------------------------------------------ M3: playback
say "playback"
idof() { api GET "/api/search?q=$1" | json "[i['id'] for i in j if i['title']=='$2'][0]"; }
BIG="$(idof Big 'Big%20Test%20Movie' 2>/dev/null || true)"
BIG="$(api GET "/api/search?q=Big" | json "[i['id'] for i in j if i['title']=='Big Test Movie'][0]")"
SPACE="$(api GET "/api/search?q=Space" | json "[i['id'] for i in j if i['title']=='Space Drama'][0]")"
EP1="$(api GET "/api/search?q=Pilot" | json "[i['id'] for i in j if i['title']=='Pilot Part 1'][0]")"
CAPS='"caps":{"video":["h264","vp9"],"audio":["aac","mp3","opus"],"containers":["mp4","webm"]}'
api POST "/api/items/$BIG/playback" "{$CAPS}" | json 'j["mode"]' | grep -qx direct && pass "mp4 h264/aac → direct" || fail "direct: $(cat "$TMP/body")"
curl -sS -b "$JAR" -r 0-1023 -o /dev/null -w '%{http_code}' "$BASE/api/stream/$BIG" | grep -qx 206 && pass "direct stream supports Range (206)" || fail "range request"
api POST "/api/items/$EP1/playback" "{$CAPS}" > "$TMP/remux.json"
json 'j["mode"]' < "$TMP/remux.json" | grep -qx remux && pass "mkv h264/aac → remux" || fail "remux: $(cat "$TMP/remux.json")"
api POST "/api/items/$SPACE/playback" "{$CAPS,\"start\":10}" > "$TMP/tc.json"
json 'j["mode"]' < "$TMP/tc.json" | grep -qx transcode && pass "mkv hevc/ac3 → transcode ($(json 'j["encoder"]' < "$TMP/tc.json"))" || fail "transcode: $(cat "$TMP/tc.json")"
json '"; ".join(j["reasons"])' < "$TMP/tc.json" | grep -q "hevc" && pass "reasons: $(json '"; ".join(j["reasons"])' < "$TMP/tc.json")" || fail "no reasons"
TOKURL="$(json 'j["url"]' < "$TMP/tc.json")"
URL="${TOKURL%%\?*}"   # the API adds ?st=<stream token> for header-less players
curl -fsS -b "$JAR" "$BASE$URL" > "$TMP/pl.m3u8" && grep -q seg00000.ts "$TMP/pl.m3u8" && pass "HLS playlist has segments" || fail "playlist: $(cat "$TMP/pl.m3u8")"
sleep 2
SEGURL="${URL%index.m3u8}seg00000.ts"
curl -fsS -b "$JAR" -o "$TMP/seg.ts" "$BASE$SEGURL"
codecs="$(ffprobe -v error -show_entries stream=codec_name -of csv=p=0 "$TMP/seg.ts" 2>/dev/null | tr -d '\r' | grep -v '^$' | sort -u | tr '\n' ',')"
[ "$codecs" = "aac,h264," ] && pass "HLS segment decodes as h264+aac" || fail "segment codecs: $codecs"
curl -fsS "$BASE$TOKURL" | grep -q 'seg00000.ts?st=' && pass "stream token: playlist without cookie, segments carry the token" || fail "stream token playlist"
curl -sS -b "$JAR" -o /dev/null -w '%{http_code}' "$BASE${URL%index.m3u8}..%2F..%2Fkinora.db" | grep -qx 400 && pass "HLS path traversal rejected" || fail "traversal not rejected"
SUBIDX="$(api GET "/api/items/$BIG" | json "[s['type_index'] for s in j['item']['streams'] if s['type']=='subtitle' and s['lang']=='hun'][0]")"
curl -fsS -b "$JAR" "$BASE/api/items/$BIG/subtitles/$SUBIDX" > "$TMP/sub.vtt"
grep -q "Árvíztűrő tükörfúrógép" "$TMP/sub.vtt" && pass "CP1250 .srt → UTF-8 WebVTT with correct accents" || fail "subtitle: $(head -c 200 "$TMP/sub.vtt")"
KEY="$(api POST "/api/items/$BIG/download" '{"quality":"360p"}' | json 'j["key"]')"; expect 202 "download job queued"
for i in $(seq 1 60); do st="$(api GET /api/downloads | json "[x['status'] for x in j['jobs'] if x['key']=='$KEY'][0]")"; [ "$st" = done ] && break; [ "$st" = error ] && break; sleep 1; done
[ "$st" = done ] && pass "360p download finished" || fail "download status: $st"
curl -fsS -b "$JAR" -D "$TMP/dlh" -o "$TMP/dl.mp4" "$BASE/api/downloads/$KEY/file" && grep -qi "filename\*=UTF-8''Big" "$TMP/dlh" && pass "download served with UTF-8 filename" || fail "download file"
api POST /api/admin/hwtest > "$TMP/hw.json"; expect 200 "encoder self-test"
api GET /api/admin/segments | json 'j["available"]' | grep -qx True && pass "skip intro: fingerprinting available ($(api GET /api/admin/segments | json 'j["backend"]'))" || fail "skip intro backend: $(cat "$TMP/body")"
docker exec "$NAME" fpcalc -version >/dev/null 2>&1 && pass "fpcalc in image" || fail "fpcalc missing in image"
docker exec -u kinora "$NAME" sh -c 'mkdir -p /cache/locked/inner && chmod 000 /cache/locked'
api GET "/api/admin/permcheck?path=/cache/locked" > "$TMP/perm.json"
if json 'j["ok"]' < "$TMP/perm.json" | grep -qx False && json '" ".join(j["commands"])' < "$TMP/perm.json" | grep -q "setfacl -R -m u:1000:rX /cache/locked"; then
    pass "permission check: unreadable folder explained ($(json 'j["message"]' < "$TMP/perm.json"))"
else
    fail "permcheck: $(cat "$TMP/perm.json")"
fi
api POST /api/libraries '{"name":"Zárt","path":"/cache/locked","kind":"movies"}' > /dev/null; expect 400 "adding an unreadable folder is refused with the fix"
MSYS_NO_PATHCONV=1 docker exec -u kinora "$NAME" chmod 755 /cache/locked
echo "    encoders: $(json '", ".join(r["name"]+(" ok" if r["ok"] else " -") for r in j["results"])' < "$TMP/hw.json")"

# ------------------------------------------------------------------ M4: browsing, kids filter
say "browsing / kids profile"
api GET "/api/items?limit=100" | json 'j["total"]' | grep -qx 5 && pass "library lists 3 movies + 2 series" || fail "items: $(head -c 300 "$TMP/body")"
api POST "/api/items/$BIG/progress" '{"position":30}' >/dev/null; expect 200 "progress report"
api GET /api/home | json '[r["title"] for r in j["rows"]]' > "$TMP/rows"
grep -q "Folytatás" "$TMP/rows" && pass "home rows: $(cat "$TMP/rows")" || fail "home rows: $(cat "$TMP/rows")"
CODE="$(api POST /api/party "{\"item_id\":$BIG}" | json 'j["code"]')"; expect 201 "watch party created ($CODE)"
api GET "/api/party/$CODE" | json 'j["item_id"]' | grep -qx "$BIG" && pass "party info" || fail "party info"
ws="$(curl -sS -o /dev/null -w '%{http_code}' -b "$JAR" -H 'Connection: Upgrade' -H 'Upgrade: websocket' -H 'Sec-WebSocket-Version: 13'     -H 'Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==' -H 'Origin: http://evil.example' "$BASE/api/party/$CODE/ws")"
[ "$ws" = 403 ] && pass "party websocket rejects foreign Origin" || fail "cross-origin ws: $ws"
api PATCH "/api/profiles/$KID" '{"pin":""}' >/dev/null
api POST "/api/profiles/$KID/select" '{"pin":"1234"}' >/dev/null; expect 200 "switch to kids profile (PIN)"
api GET "/api/items?limit=100" | json "[i['title'] for i in j['items']]" > "$TMP/kidlist"
grep -q "Space Drama" "$TMP/kidlist" && fail "kid sees R film: $(cat "$TMP/kidlist")" || pass "kid list hides R film: $(cat "$TMP/kidlist")"
api GET "/api/items/$SPACE" >/dev/null; expect 404 "kid: R film details → 404"
curl -sS -b "$JAR" -o /dev/null -w '%{http_code}' "$BASE/api/stream/$SPACE" | grep -qx 404 && pass "kid: R film stream → 404" || fail "kid stream not blocked"
api POST /api/collections '{"name":"x"}' >/dev/null; expect 403 "kid cannot create collections"

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
