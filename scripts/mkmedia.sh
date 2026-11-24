#!/usr/bin/env bash
# Generates Kinora's synthetic test library with ffmpeg.
#   scripts/mkmedia.sh [outdir=testmedia]
set -euo pipefail
OUT="${1:-testmedia}"
FF="${FFMPEG:-ffmpeg}"
mkdir -p "$OUT"
cd "$OUT"
ff() { "$FF" -hide_banner -loglevel error -y "$@"; }

V="testsrc2=size=640x360:rate=24"
A="sine=frequency=440:sample_rate=48000"

# ---------------------------------------------------------------- Movies
d="Movies/Big Test Movie (2019)"
mkdir -p "$d"
cat > "$d/chapters.txt" <<'EOF'
;FFMETADATA1
title=Big Test Movie
[CHAPTER]
TIMEBASE=1/1000
START=0
END=50000
title=Intro
[CHAPTER]
TIMEBASE=1/1000
START=50000
END=60000
title=End Credits
EOF
ff -f lavfi -i "$V" -f lavfi -i "$A" -i "$d/chapters.txt" -map 0:v -map 1:a -map_metadata 2 -map_chapters 2 \
   -t 60 -c:v libx264 -preset veryfast -pix_fmt yuv420p -g 48 -c:a aac -b:a 96k -movflags +faststart \
   "$d/Big Test Movie (2019).mp4"
rm "$d/chapters.txt"
cat > "$d/movie.nfo" <<'EOF'
<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<movie>
  <title>Big Test Movie</title>
  <year>2019</year>
  <plot>Egy nagyon fontos tesztfilm, amely két fejezetből áll. Az utolsó tíz másodperc a stáblista.</plot>
  <genre>Sci-Fi / Drama</genre>
  <tag>Teszt</tag>
  <mpaa>PG-13</mpaa>
  <rating>7.9</rating>
</movie>
EOF
# CP1250 encoded Hungarian subtitle (Á í ű ő ü ö ú ó é as single CP1250 bytes)
printf '1\r\n00:00:01,000 --> 00:00:05,000\r\n\xC1rv\xEDzt\xFBr\xF5 t\xFCk\xF6rf\xFAr\xF3g\xE9p\r\n\r\n2\r\n00:00:06,000 --> 00:00:09,000\r\nŐszi \xE9jszaka\r\n\r\n' \
  | sed 's/Ő/\xD5/' > "$d/Big Test Movie (2019).hu.srt"

d="Movies/Space Drama (2015)"
mkdir -p "$d"
cat > "$d/eng.srt" <<'EOF'
1
00:00:01,000 --> 00:00:04,000
Houston, we have a subtitle.

2
00:00:05,000 --> 00:00:08,000
Second line in English.
EOF
ff -f lavfi -i "testsrc2=size=1920x1080:rate=24" -f lavfi -i "sine=frequency=330:sample_rate=48000" -i "$d/eng.srt" \
   -map 0:v -map 1:a -map 2:s -t 40 -c:v libx265 -preset ultrafast -x265-params log-level=error -pix_fmt yuv420p \
   -c:a ac3 -b:a 192k -ac 2 -c:s srt -disposition:s:0 hearing_impaired -metadata:s:s:0 language=eng -metadata:s:a:0 language=eng \
   "$d/Space.Drama.2015.1080p.BluRay.x265.mkv"
rm "$d/eng.srt"
cat > "$d/movie.nfo" <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<movie>
  <title>Space Drama</title>
  <year>2015</year>
  <plot>Felnőtteknek szóló űrdráma HEVC videóval és AC3 hanggal.</plot>
  <genre>Sci-Fi</genre>
  <genre>Thriller</genre>
  <mpaa>R</mpaa>
  <rating>8.1</rating>
</movie>
EOF

ff -f lavfi -i "$V" -f lavfi -i "$A" -t 30 -c:v libx264 -preset veryfast -pix_fmt yuv420p -c:a aac -b:a 96k \
   -movflags +faststart "Movies/Kids.Cartoon.2018.mp4"

# ---------------------------------------------------------------- Shows
d="Shows/Test Show (2020)"
mkdir -p "$d/Season 01" "$d/Season 02"
for n in 1 2 3; do
  ff -f lavfi -i "$V" -f lavfi -i "sine=frequency=$((200+n*100)):sample_rate=48000" -t 20 \
     -c:v libx264 -preset veryfast -pix_fmt yuv420p -g 48 -c:a aac -b:a 96k -metadata:s:a:0 language=hun \
     "$d/Season 01/Test.Show.S01E0$n.Pilot.Part.$n.mkv"
done
ff -f lavfi -i "$V" -f lavfi -i "$A" -t 20 -c:v libx264 -preset veryfast -pix_fmt yuv420p -c:a aac -b:a 96k \
   "$d/Season 02/Test.Show.S02E01.mkv"
cat > "$d/tvshow.nfo" <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<tvshow>
  <title>Test Show</title>
  <year>2020</year>
  <plot>Családbarát tesztsorozat két évaddal.</plot>
  <genre>Comedy</genre>
  <genre>Family</genre>
  <mpaa>TV-PG</mpaa>
  <rating>7.2</rating>
</tvshow>
EOF

mkdir -p "Shows/Other.Series"
ff -f lavfi -i "$V" -f lavfi -i "$A" -t 15 -c:v libx264 -preset veryfast -pix_fmt yuv420p -c:a aac -b:a 96k \
   -movflags +faststart "Shows/Other.Series/Other.Series.S01E01.mp4"

touch .complete
echo "test media ready in $(pwd)"
