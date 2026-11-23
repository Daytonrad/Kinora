#!/usr/bin/env bash
# Generates the benchmark library into $1 (default /bench-media). Runs inside a container with ffmpeg
# (bench.sh uses the kinora-dev image), so the files live in a Docker volume, not on the host.
#
#   movies/<Title> (<Year>)/<Title> (<Year>).mp4            MOVIES films, a 2 s 320×180 H.264/AAC clip each
#   shows/<Name> (<Year>)/Season 0S/<Name> - S0SE0E.mp4      SHOWS series × SEASONS × EPISODES, the same clip
#   movies/Bench Direct (2020)/…mp4                           60 s 1080p H.264/AAC (direct play test)
#   movies/Bench Transcode (2020)/…mkv                        60 s 1080p HEVC/AC-3 (transcode test)
#
# Titles are "<Adjective> <Noun> <number>" from 25×25 words, so a one-word search ("river") matches 1/25.
set -euo pipefail
OUT="${1:-/bench-media}"
MOVIES="${MOVIES:-5000}" SHOWS="${SHOWS:-500}" SEASONS="${SEASONS:-2}" EPISODES="${EPISODES:-5}"
SPEC="movies=$MOVIES shows=$SHOWS seasons=$SEASONS episodes=$EPISODES v1"
if [ -f "$OUT/.complete" ] && [ "$(cat "$OUT/.complete")" = "$SPEC" ]; then
    echo "library already generated ($SPEC)"
    exit 0
fi
rm -rf "${OUT:?}/movies" "${OUT:?}/shows" "$OUT/.complete"
mkdir -p "$OUT/movies" "$OUT/shows"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

ff() { ffmpeg -hide_banner -loglevel error -y "$@"; }
ff -f lavfi -i testsrc=size=320x180:rate=10 -f lavfi -i sine=frequency=440 -t 2 \
   -c:v libx264 -preset veryfast -pix_fmt yuv420p -c:a aac -b:a 32k -shortest "$tmp/clip.mp4"

adj=(Silent Golden Broken Hidden Crimson Frozen Last Distant Wild Hollow Bright Quiet Iron Velvet Lost
     Burning Northern Secret Paper Silver Electric Midnight Lonely Restless Ancient)
noun=(River Harbor Garden Signal Mirror Forest Engine Station Voyage Letter Island Shadow Kingdom Echo
      Window Bridge Desert Comet Lantern Orchard Canyon Circuit Meadow Tower Archive)
title() { # title <n>
    local n=$1
    printf '%s %s %04d' "${adj[$(( n % 25 ))]}" "${noun[$(( (n / 25) % 25 ))]}" "$n"
}

echo "movies: $MOVIES"
for ((i = 1; i <= MOVIES; i++)); do
    t="$(title "$i")"
    y=$(( 1950 + i % 76 ))
    d="$OUT/movies/$t ($y)"
    mkdir -p "$d"
    cp "$tmp/clip.mp4" "$d/$t ($y).mp4"
done

echo "shows: $SHOWS × $SEASONS × $EPISODES"
for ((i = 1; i <= SHOWS; i++)); do
    t="$(title $(( i + 7000 )))"
    y=$(( 1980 + i % 46 ))
    for ((s = 1; s <= SEASONS; s++)); do
        d="$OUT/shows/$t ($y)/Season $(printf %02d "$s")"
        mkdir -p "$d"
        for ((e = 1; e <= EPISODES; e++)); do
            cp "$tmp/clip.mp4" "$d/$t - S$(printf %02d "$s")E$(printf %02d "$e").mp4"
        done
    done
done

echo "playback test files"
d="$OUT/movies/Bench Direct (2020)"
mkdir -p "$d"
ff -f lavfi -i testsrc2=size=1920x1080:rate=24 -f lavfi -i sine=frequency=330:sample_rate=48000 -t 60 \
   -c:v libx264 -preset veryfast -b:v 6M -pix_fmt yuv420p -c:a aac -b:a 160k -ac 2 -movflags +faststart \
   "$d/Bench Direct (2020).mp4"
d="$OUT/movies/Bench Transcode (2020)"
mkdir -p "$d"
ff -f lavfi -i testsrc2=size=1920x1080:rate=24 -f lavfi -i sine=frequency=550:sample_rate=48000 -t 60 \
   -c:v libx265 -preset ultrafast -b:v 6M -pix_fmt yuv420p -tag:v hvc1 -c:a ac3 -b:a 384k -ac 6 \
   "$d/Bench Transcode (2020).mkv"

echo "$SPEC" > "$OUT/.complete"
find "$OUT" -type f -name '*.mp4' -o -type f -name '*.mkv' | wc -l | xargs echo "video files:"
du -sh "$OUT" | cut -f1 | xargs echo "size:"
