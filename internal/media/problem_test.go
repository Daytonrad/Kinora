package media

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ScotyLucas/kinora/internal/store"
)

// Integration tests on the files of scripts/gen-testmedia.sh (skipped when they are missing).

func problemFile(t *testing.T, rel string) string {
	t.Helper()
	root, _ := filepath.Abs("../../testmedia-problem")
	if _, err := os.Stat(filepath.Join(root, ".complete")); err != nil {
		t.Skip("problem media missing – run scripts/gen-testmedia.sh")
	}
	return filepath.Join(root, "Movies", rel)
}

func TestProblemHDR10(t *testing.T) {
	it := probeItem(t, problemFile(t, "HDR10 (2020)/HDR10.2020.2160p.UHD.BluRay.HDR10.x265.mkv"))
	if it.HDR != "HDR10" || it.VideoCodec != "hevc" {
		t.Fatalf("probe: hdr %q codec %q", it.HDR, it.VideoCodec)
	}
	// a browser without HEVC/HDR: transcode with tone mapping, and the reason says so
	d := Decide(it, PlaybackRequest{Caps: chrome}, nil, 0, true)
	if d.Mode != "transcode" || !d.ToneMap || !strings.Contains(strings.Join(d.Reasons, "|"), "HDR10") {
		t.Fatalf("SDR browser: %+v", d)
	}
	// without zscale the reason warns about washed-out colours instead of claiming a tone map
	if d := Decide(it, PlaybackRequest{Caps: chrome}, nil, 0, false); d.ToneMap || !strings.Contains(strings.Join(d.Reasons, "|"), "zscale") {
		t.Fatalf("no tone mapping available: %+v", d)
	}
	// an HDR TV that plays HEVC in MKV gets the file as it is
	tv := Caps{Video: []string{"h264", "hevc"}, Audio: []string{"aac", "ac3"}, Containers: []string{"mkv", "mp4"}, HDR: true}
	if d := Decide(it, PlaybackRequest{Caps: tv}, nil, 0, true); d.Mode != "direct" {
		t.Fatalf("HDR TV: %+v", d)
	}
	// the tone-mapped transcode really produces SDR H.264 segments
	enc := NewEncoders()
	enc.SelfTest(context.Background())
	if !enc.ToneMap() {
		t.Log("ffmpeg without zscale: the session runs without tone mapping")
	}
	m := NewManager(t.TempDir(), enc, quiet)
	defer m.StopAll()
	d = Decide(it, PlaybackRequest{Caps: chrome}, nil, 0, enc.ToneMap())
	s, err := m.Start(context.Background(), HLSSpec{Item: it, UserID: 1, Decision: d, Encoder: enc.Pick("auto")})
	if err != nil {
		t.Fatal(err)
	}
	waitFinished(t, s)
	if got := segmentProbe(t, filepath.Join(s.Dir, "seg00000.ts")); got != "h264,aac" {
		t.Fatalf("segment streams %q", got)
	}
}

func TestProblemManyAudioTracks(t *testing.T) {
	it := probeItem(t, problemFile(t, "Many Audio Tracks (2021)/Many.Audio.Tracks.2021.1080p.mkv"))
	var langs, codecs []string
	for _, s := range it.Streams {
		if s.Type == "audio" {
			langs = append(langs, s.Lang)
			codecs = append(codecs, s.Codec)
		}
	}
	if len(langs) != 8 {
		t.Fatalf("audio tracks: %v", langs)
	}
	if langs[0] != "hun" || langs[2] != "deu" || langs[7] != "" {
		t.Fatalf("languages (und must become empty): %v", langs)
	}
	if strings.Join(codecs, ",") != "aac,ac3,eac3,opus,flac,mp3,aac,ac3" {
		t.Fatalf("codecs: %v", codecs)
	}
	// a German profile gets the German track, which is not the first one: no direct play
	if got := ChooseAudio(it.Streams, nil, []string{"deu"}); got != 2 {
		t.Fatalf("German track: %d", got)
	}
	d := Decide(it, PlaybackRequest{Caps: chrome}, []string{"deu"}, 0, true)
	if d.Mode == "direct" || d.AudioIdx != 2 {
		t.Fatalf("decision: %+v", d)
	}
	// an explicit pick wins over the language list
	idx := 5
	if d := Decide(it, PlaybackRequest{Caps: chrome, AudioIdx: &idx}, []string{"deu"}, 0, true); d.AudioIdx != 5 {
		t.Fatalf("explicit track: %+v", d)
	}
}

func TestProblemImageSubtitleBurnIn(t *testing.T) {
	it := probeItem(t, problemFile(t, "Image Subtitles (2019)/Image.Subtitles.2019.mkv"))
	var sub *store.StreamInfo
	for i, s := range it.Streams {
		if s.Type == "subtitle" {
			sub = &it.Streams[i]
		}
	}
	if sub == nil || sub.Codec != "hdmv_pgs_subtitle" || sub.TextBased || sub.Lang != "hun" {
		t.Fatalf("PGS stream: %+v", sub)
	}
	// image subtitles cannot become WebVTT
	if _, err := NewSubtitles(t.TempDir()).VTT(context.Background(), it, *sub); err == nil {
		t.Fatal("a PGS track must not convert to WebVTT")
	}
	burn := 0
	d := Decide(it, PlaybackRequest{Caps: chrome, BurnSubIdx: &burn}, nil, 0, true)
	if d.Mode != "transcode" || d.BurnSubIdx == nil || !strings.Contains(strings.Join(d.Reasons, "|"), "hdmv_pgs_subtitle") {
		t.Fatalf("burn-in decision: %+v", d)
	}
	enc := NewEncoders()
	enc.SelfTest(context.Background())
	m := NewManager(t.TempDir(), enc, quiet)
	defer m.StopAll()
	s, err := m.Start(context.Background(), HLSSpec{Item: it, UserID: 1, Decision: d, Encoder: enc.Pick("auto")})
	if err != nil {
		t.Fatalf("burn-in session: %v", err)
	}
	waitFinished(t, s)
	if got := segmentProbe(t, filepath.Join(s.Dir, "seg00000.ts")); got != "h264,aac" {
		t.Fatalf("segment streams %q", got)
	}
}

func TestProblemTextSubtitles(t *testing.T) {
	subs := NewSubtitles(t.TempDir())
	for _, f := range []string{"Text Subtitles (2018)/Text.Subtitles.2018.mkv", "Text Subtitles (2018)/Text.Subtitles.2018.movtext.mp4"} {
		it := probeItem(t, problemFile(t, f))
		n := 0
		for _, s := range it.Streams {
			if s.Type != "subtitle" {
				continue
			}
			n++
			if !s.TextBased {
				t.Fatalf("%s: %s should be text based", f, s.Codec)
			}
			p, err := subs.VTT(context.Background(), it, s)
			if err != nil {
				t.Fatalf("%s %s: %v", f, s.Codec, err)
			}
			b, _ := os.ReadFile(p)
			if !strings.HasPrefix(string(b), "WEBVTT") || !strings.Contains(string(b), "Árvíztűrő") {
				t.Fatalf("%s %s VTT:\n%s", f, s.Codec, b)
			}
		}
		if want := map[bool]int{true: 3, false: 1}[strings.HasSuffix(f, ".mkv")]; n != want {
			t.Fatalf("%s: %d subtitle tracks, want %d", f, n, want)
		}
	}
}

func TestProblemVariableFrameRate(t *testing.T) {
	it := probeItem(t, problemFile(t, "Variable Frame Rate (2017)/Variable.Frame.Rate.2017.mkv"))
	if it.RuntimeSec < 5 || it.RuntimeSec > 7 {
		t.Fatalf("duration %d", it.RuntimeSec)
	}
	enc := NewEncoders()
	enc.SelfTest(context.Background())
	m := NewManager(t.TempDir(), enc, quiet)
	defer m.StopAll()
	d := Decide(it, PlaybackRequest{Caps: chrome, Force: "transcode"}, nil, 0, false)
	s, err := m.Start(context.Background(), HLSSpec{Item: it, UserID: 1, Decision: d, Encoder: enc.Pick("auto")})
	if err != nil {
		t.Fatal(err)
	}
	waitFinished(t, s)
}

// Broken files must fail cleanly (an error, or a usable partial result), never hang or panic.
func TestProblemBrokenFiles(t *testing.T) {
	for _, f := range []string{"Truncated (2016)/Truncated.2016.mkv", "Corrupt (2015)/Corrupt.2015.mkv", "Empty (2014)/Empty.2014.mkv"} {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		pr, err := Probe(ctx, problemFile(t, f))
		cancel()
		if strings.HasPrefix(f, "Empty") {
			if err == nil {
				t.Fatalf("%s: an empty file must not probe", f)
			}
			continue
		}
		if err != nil {
			t.Logf("%s: %v", f, err)
			continue
		}
		it := &store.Item{Path: f, Container: pr.Container, VideoCodec: pr.VideoCodec, AudioCodec: pr.AudioCodec, HDR: pr.HDR}
		for _, s := range pr.Streams {
			it.Streams = append(it.Streams, store.StreamInfo{Type: s.Type, TypeIndex: s.TypeIndex, Codec: s.Codec})
		}
		Decide(it, PlaybackRequest{Caps: chrome}, nil, 0, true)
	}
}

func TestProblemNames(t *testing.T) {
	long := "A Film With An Extraordinarily Long Title That Keeps Going Well Past What Any Sensible Person Would Type Into A File Name (2013)"
	for _, f := range []string{long + "/" + long + ".mkv", "Ünnepi Műsor – 日本語 (2020)/Ünnepi Műsor – 日本語 (2020).mkv"} {
		if it := probeItem(t, problemFile(t, f)); it.VideoCodec != "h264" {
			t.Fatalf("%s: %+v", f, it)
		}
	}
}

func waitFinished(t *testing.T, s *Session) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for !s.Finished() && time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
	}
	if !s.Finished() {
		t.Fatal("ffmpeg did not finish")
	}
	if pl, _ := s.Playlist(); !strings.Contains(string(pl), "#EXT-X-ENDLIST") {
		t.Fatalf("playlist not finished:\n%s", pl)
	}
}
