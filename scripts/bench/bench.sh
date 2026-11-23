#!/usr/bin/env bash
# Kinora vs Jellyfin benchmark. Needs Docker with Compose v2 and bash (Linux, macOS, or Git Bash on Windows).
#
#   scripts/bench/bench.sh                 # both servers
#   scripts/bench/bench.sh kinora          # one of them
#
# Settings (environment):
#   REPS=3          first/second scan repetitions (each on a fresh config)
#   CLIENTS=16      concurrent clients in the load phase      LOAD_S=60   its length in seconds
#   API_N=100       sequential requests per API scenario      PLAY_N=5    playback starts per mode
#   IDLE_S=60       idle sampling window, after the server has settled (CPU < 3% for 10 samples, max SETTLE_MAX s)
#   MOVIES=5000 SHOWS=500                    library size (each series: 2 seasons × 5 episodes)
#   BENCH_CPUSET=0-3 BENCH_MEM=4g            the same CPUs and memory limit for both servers
#   RUNNER_CPUSET=4-11                       the CPUs of the load generator (must not overlap BENCH_CPUSET)
#   JELLYFIN_TAG=12.4.20261026-211555        the Jellyfin image
#
# Results: scripts/bench/results/<UTC date-time>/ (raw JSON, docker stats samples, report.md).
set -euo pipefail
cd "$(dirname "$0")"
export MSYS_NO_PATHCONV=1
REPS="${REPS:-3}" CLIENTS="${CLIENTS:-16}" LOAD_S="${LOAD_S:-60}" API_N="${API_N:-100}" PLAY_N="${PLAY_N:-5}"
IDLE_S="${IDLE_S:-60}" SETTLE_MAX="${SETTLE_MAX:-1800}"
export JELLYFIN_TAG="${JELLYFIN_TAG:-12.4.20261026-211555}" BENCH_CPUSET="${BENCH_CPUSET:-0-3}" BENCH_MEM="${BENCH_MEM:-4g}"
export RUNNER_CPUSET="${RUNNER_CPUSET:-4-11}"
export MOVIES="${MOVIES:-5000}" SHOWS="${SHOWS:-500}"
targets=("${@:-kinora jellyfin}")
read -r -a targets <<<"${targets[*]}"

DC=(docker compose -f compose.yml)
RUN="${RUN:-$(date -u +%Y%m%d-%H%M)}"
OUT="results/$RUN"
mkdir -p "$OUT"
say() { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
now() { date +%s.%N; }

say "results: scripts/bench/$OUT"
{
    echo "date (UTC):   $(date -u '+%Y-%m-%d %H:%M')"
    echo "kinora:       $(git rev-parse --short HEAD 2>/dev/null || echo '?')$(git diff --quiet 2>/dev/null || echo ' (+ uncommitted changes)')"
    echo "jellyfin:     jellyfin/jellyfin:$JELLYFIN_TAG"
    echo "docker:       $(docker info --format '{{.ServerVersion}}, {{.OperatingSystem}}, kernel {{.KernelVersion}}, {{.NCPU}} CPUs, {{.MemTotal}} bytes visible to Docker')"
    echo "limits:       CPUs $BENCH_CPUSET (cpuset) and $BENCH_MEM RAM per server; load generator on CPUs $RUNNER_CPUSET"
    echo "library:      $MOVIES films + $SHOWS series × 2 seasons × 5 episodes (2 s clips) + 2 one-minute 1080p files"
    echo "runs:         scans ×$REPS, API ${API_N}× per scenario, load $CLIENTS clients × ${LOAD_S} s, playback ${PLAY_N}× per mode"
    [ -n "${HOST_CPU:-}" ] && echo "host CPU:     $HOST_CPU"
    [ -n "${HOST_NOTE:-}" ] && echo "note:         $HOST_NOTE"
} >"$OUT/env.txt"
cat "$OUT/env.txt"

say "building images"
"${DC[@]}" build kinora runner >/dev/null
"${DC[@]}" pull -q jellyfin
say "generating the library (skipped when already there)"
"${DC[@]}" run --rm -T gen
say "building the runner"
"${DC[@]}" run --rm -T --entrypoint go runner build -o scripts/bench/.bin/runner ./scripts/bench/runner
runner() { "${DC[@]}" run --rm -T --entrypoint /src/scripts/bench/.bin/runner runner "$@"; }

sampler() { # sampler <container> <file>: docker stats every ~2 s
    while :; do
        if l="$(docker stats --no-stream --format '{{.CPUPerc}};{{.MemUsage}}' "$1" 2>/dev/null)"; then
            echo "$(now);$l" >>"$2"
        else
            sleep 1
        fi
    done
}

settle() { # settle <stats file>: until the last 10 samples are all under 3% CPU
    local start=$SECONDS
    while ((SECONDS - start < SETTLE_MAX)); do
        sleep 10
        if [ "$(tail -n 10 "$1" 2>/dev/null | awk -F';' '{gsub("%","",$2); if ($2+0 < 3) n++} END {print n+0}')" = 10 ]; then
            echo "  · settled after $((SECONDS - start)) s"
            return
        fi
    done
    echo "  · did not settle within $SETTLE_MAX s (measuring anyway)"
}

for t in "${targets[@]}"; do
    for rep in $(seq 1 "$REPS"); do
        say "$t: scan $rep/$REPS (fresh configuration)"
        "${DC[@]}" rm -sfv "$t" >/dev/null 2>&1 || true
        docker volume rm -f "kinora-bench_$t-config" "kinora-bench_$t-cache" >/dev/null
        "${DC[@]}" up -d "$t" >/dev/null
        runner scan -target "$t" >"$OUT/$t-scan-$rep.json"
    done
    cid="$("${DC[@]}" ps -q "$t")"
    sampler "$cid" "$OUT/$t-stats.csv" &
    sp=$!
    trap 'kill $sp 2>/dev/null || true' EXIT
    say "$t: waiting until idle"
    settle "$OUT/$t-stats.csv"
    s="$(now)"
    sleep "$IDLE_S"
    echo "$s $(now)" >"$OUT/$t-idle.txt"
    say "$t: API latency"
    runner api -target "$t" -n "$API_N" >"$OUT/$t-api.json"
    say "$t: load"
    runner load -target "$t" -c "$CLIENTS" -d "${LOAD_S}s" >"$OUT/$t-load.json"
    sleep 5
    say "$t: playback start"
    runner play -target "$t" -n "$PLAY_N" >"$OUT/$t-play.json"
    kill "$sp" 2>/dev/null || true
    "${DC[@]}" stop "$t" >/dev/null
done

say "report"
runner report -dir "scripts/bench/$OUT" | tee "$OUT/report.md"
