#!/usr/bin/env bash
# Generates short "problem" media files with ffmpeg for the integration tests (internal/media,
# internal/scanner). Nothing here is committed: the files are a few MB and are rebuilt on demand.
#   scripts/gen-testmedia.sh [outdir=testmedia-problem]
#
# What it makes (Movies/ is a normal movies library the scanner can walk):
#   HDR10 (2020)/                     HEVC Main10, PQ transfer, BT.2020, mastering display + MaxCLL
#   Many Audio Tracks (2021)/         8 audio tracks in different codecs and languages
#   Image Subtitles (2019)/           a PGS (Blu-ray, image based) subtitle track. ffmpeg cannot encode
#                                     PGS, so the script writes a minimal .sup by hand and muxes it
#   Text Subtitles (2018)/            SRT, ASS and WebVTT tracks in MKV, mov_text in MP4
#   Variable Frame Rate (2017)/       frames dropped unevenly, written with -fps_mode vfr
#   Truncated (2016)/                 an MKV cut off at 40 % (no cues, no end of clusters)
#   Corrupt (2015)/                   an MKV with garbage written over its middle
#   Empty (2014)/                     a zero-byte .mkv
#   <~200 byte name>/                 a very long folder and file name
#   Ünnepi Műsor – 日本語 (2020)/      a Unicode path
set -euo pipefail
OUT="${1:-testmedia-problem}"
FF="${FFMPEG:-ffmpeg}"
mkdir -p "$OUT/Movies"
cd "$OUT/Movies"
ff() { "$FF" -hide_banner -loglevel error -y "$@"; }

V="testsrc2=size=640x360:rate=24"
A="sine=frequency=440:sample_rate=48000"

# ---------------------------------------------------------------- HDR10
d="HDR10 (2020)"
mkdir -p "$d"
ff -f lavfi -i "testsrc2=size=1280x720:rate=24" -f lavfi -i "$A" -t 4 \
   -c:v libx265 -preset ultrafast -pix_fmt yuv420p10le \
   -x265-params "log-level=error:hdr10=1:repeat-headers=1:colorprim=bt2020:transfer=smpte2084:colormatrix=bt2020nc:master-display=G(13250,34500)B(7500,3000)R(34000,16000)WP(15635,16450)L(10000000,1):max-cll=1000,400" \
   -color_primaries bt2020 -color_trc smpte2084 -colorspace bt2020nc \
   -c:a aac -b:a 96k "$d/HDR10.2020.2160p.UHD.BluRay.HDR10.x265.mkv"

# ---------------------------------------------------------------- many audio tracks
d="Many Audio Tracks (2021)"
mkdir -p "$d"
args=(-f lavfi -i "$V")
for f in 220 247 262 294 330 349 392 440; do args+=(-f lavfi -i "sine=frequency=$f:sample_rate=48000"); done
maps=(-map 0:v)
for i in $(seq 1 8); do maps+=(-map "$i:a"); done
ff "${args[@]}" "${maps[@]}" -t 4 -c:v libx264 -preset veryfast -pix_fmt yuv420p \
   -c:a:0 aac -c:a:1 ac3 -c:a:2 eac3 -c:a:3 libopus -c:a:4 flac -c:a:5 mp3 -c:a:6 aac -c:a:7 ac3 \
   -metadata:s:a:0 language=hun -metadata:s:a:1 language=eng -metadata:s:a:2 language=deu \
   -metadata:s:a:3 language=fra -metadata:s:a:4 language=jpn -metadata:s:a:5 language=spa \
   -metadata:s:a:6 language=ita -metadata:s:a:7 language=und \
   -metadata:s:a:1 title="Commentary" -disposition:a:0 default \
   "$d/Many.Audio.Tracks.2021.1080p.mkv"

# ---------------------------------------------------------------- subtitles
cat > sub.srt <<'EOF'
1
00:00:00,500 --> 00:00:02,000
Első felirat – Árvíztűrő tükörfúrógép

2
00:00:02,000 --> 00:00:03,500
Second line
EOF
ff -i sub.srt sub.ass
ff -i sub.srt sub.vtt

# A minimal PGS stream: one 8x2 white bitmap shown from 0.5 s, cleared at 2 s.
# Segment = "PG", PTS (4 bytes, 90 kHz), DTS (4), type (1), size (2), payload.
#   0x16 PCS (composition), 0x17 WDS (window), 0x14 PDS (palette), 0x15 ODS (object, RLE), 0x80 END
pgs() {
  local pts='\x00\x00\xAF\xC8' clear='\x00\x02\xBF\x20' dts='\x00\x00\x00\x00'
  local line='\x01\x01\x01\x01\x01\x01\x01\x01\x00\x00' # 8 pixels of palette entry 1, end of line
  # display set 1: epoch start, one object at 100,100
  printf "PG$pts$dts"'\x16\x00\x13\x02\x80\x01\x68\x10\x00\x00\x80\x00\x00\x01\x00\x00\x00\x00\x00\x64\x00\x64'
  printf "PG$pts$dts"'\x17\x00\x0A\x01\x00\x00\x64\x00\x64\x00\x08\x00\x02'
  printf "PG$pts$dts"'\x14\x00\x07\x00\x00\x01\xEB\x80\x80\xFF'
  printf "PG$pts$dts"'\x15\x00\x1F\x00\x00\x00\xC0\x00\x00\x18\x00\x08\x00\x02'"$line$line"
  printf "PG$pts$dts"'\x80\x00\x00'
  # display set 2: no objects, clears the screen
  printf "PG$clear$dts"'\x16\x00\x0B\x02\x80\x01\x68\x10\x00\x01\x00\x00\x00\x00'
  printf "PG$clear$dts"'\x17\x00\x0A\x01\x00\x00\x64\x00\x64\x00\x08\x00\x02'
  printf "PG$clear$dts"'\x80\x00\x00'
}
d="Image Subtitles (2019)"
mkdir -p "$d"
pgs > sub.sup
ff -f lavfi -i "$V" -f lavfi -i "$A" -i sub.sup -map 0:v -map 1:a -map 2:s -t 4 \
   -c:v libx264 -preset veryfast -pix_fmt yuv420p -c:a aac -b:a 96k \
   -c:s copy -metadata:s:s:0 language=hun \
   "$d/Image.Subtitles.2019.mkv"
rm -f sub.sup

d="Text Subtitles (2018)"
mkdir -p "$d"
ff -f lavfi -i "$V" -f lavfi -i "$A" -i sub.srt -i sub.ass -i sub.vtt -map 0:v -map 1:a -map 2:s -map 3:s -map 4:s -t 4 \
   -c:v libx264 -preset veryfast -pix_fmt yuv420p -c:a aac -b:a 96k \
   -c:s:0 srt -c:s:1 ass -c:s:2 webvtt \
   -metadata:s:s:0 language=hun -metadata:s:s:1 language=eng -metadata:s:s:2 language=deu -disposition:s:2 forced \
   "$d/Text.Subtitles.2018.mkv"
ff -f lavfi -i "$V" -f lavfi -i "$A" -i sub.srt -map 0:v -map 1:a -map 2:s -t 4 \
   -c:v libx264 -preset veryfast -pix_fmt yuv420p -c:a aac -b:a 96k -c:s mov_text -metadata:s:s:0 language=hun \
   -movflags +faststart "$d/Text.Subtitles.2018.movtext.mp4"
rm -f sub.srt sub.ass sub.vtt

# ---------------------------------------------------------------- variable frame rate
d="Variable Frame Rate (2017)"
mkdir -p "$d"
# keep every frame for 2 s, then every third, then every second: uneven frame intervals
ff -f lavfi -i "testsrc2=size=640x360:rate=30" -f lavfi -i "$A" -t 6 \
   -vf "select='lt(t\,2)+between(t\,2\,4)*not(mod(n\,3))+gte(t\,4)*not(mod(n\,2))'" -fps_mode vfr \
   -c:v libx264 -preset veryfast -pix_fmt yuv420p -c:a aac -b:a 96k \
   "$d/Variable.Frame.Rate.2017.mkv"

# ---------------------------------------------------------------- broken files
d="Truncated (2016)"
mkdir -p "$d"
ff -f lavfi -i "$V" -f lavfi -i "$A" -t 6 -c:v libx264 -preset veryfast -pix_fmt yuv420p -c:a aac -b:a 96k full.mkv
size=$(wc -c < full.mkv)
head -c $((size * 40 / 100)) full.mkv > "$d/Truncated.2016.mkv"

d="Corrupt (2015)"
mkdir -p "$d"
cp full.mkv "$d/Corrupt.2015.mkv"
# 64 KiB of 0xFF from 30 % into the file (deterministic, so test runs are reproducible)
head -c 65536 /dev/zero | tr '\0' '\377' | dd of="$d/Corrupt.2015.mkv" bs=1 seek=$((size * 30 / 100)) conv=notrunc status=none
rm full.mkv

d="Empty (2014)"
mkdir -p "$d"
: > "$d/Empty.2014.mkv"

# ---------------------------------------------------------------- names
long="A Film With An Extraordinarily Long Title That Keeps Going Well Past What Any Sensible Person Would Type Into A File Name (2013)"
mkdir -p "$long"
ff -f lavfi -i "$V" -f lavfi -i "$A" -t 2 -c:v libx264 -preset veryfast -pix_fmt yuv420p -c:a aac -b:a 96k \
   "$long/$long.mkv"

d="Ünnepi Műsor – 日本語 (2020)"
mkdir -p "$d"
ff -f lavfi -i "$V" -f lavfi -i "$A" -t 2 -c:v libx264 -preset veryfast -pix_fmt yuv420p -c:a aac -b:a 96k \
   "$d/Ünnepi Műsor – 日本語 (2020).mkv"

touch ../.complete
echo "problem media ready in $OUT"
