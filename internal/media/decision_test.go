package media

import (
	"strings"
	"testing"

	"github.com/ScotyLucas/kinora/internal/store"
)

var chrome = Caps{Video: []string{"h264", "vp9", "av1"}, Audio: []string{"aac", "mp3", "opus", "flac"}, Containers: []string{"mp4", "webm"}}

func item(container, v, a string, bitrate int, audio ...store.StreamInfo) *store.Item {
	it := &store.Item{Container: container, VideoCodec: v, AudioCodec: a, Bitrate: bitrate, Width: 1920, Height: 1080}
	it.Streams = append(it.Streams, store.StreamInfo{Type: "video", Codec: v})
	if len(audio) == 0 {
		audio = []store.StreamInfo{{Type: "audio", TypeIndex: 0, Codec: a}}
	}
	it.Streams = append(it.Streams, audio...)
	return it
}

func TestDecideDirect(t *testing.T) {
	d := Decide(item("mp4", "h264", "aac", 4_000_000), PlaybackRequest{Caps: chrome}, nil, 0, true)
	if d.Mode != "direct" {
		t.Fatalf("want direct, got %+v", d)
	}
}

func TestDecideRemux(t *testing.T) {
	d := Decide(item("mkv", "h264", "aac", 4_000_000), PlaybackRequest{Caps: chrome}, nil, 0, true)
	if d.Mode != "remux" || !d.VideoCopy || !d.AudioCopy {
		t.Fatalf("mkv h264/aac → remux with copies, got %+v", d)
	}
	if !strings.Contains(strings.Join(d.Reasons, ";"), "mkv konténert") {
		t.Fatalf("reason missing: %v", d.Reasons)
	}
	d = Decide(item("mkv", "h264", "ac3", 4_000_000), PlaybackRequest{Caps: chrome}, nil, 0, true)
	if d.Mode != "remux" || d.AudioCopy {
		t.Fatalf("ac3 must be re-encoded: %+v", d)
	}
}

func TestDecideTranscode(t *testing.T) {
	d := Decide(item("mkv", "hevc", "ac3", 20_000_000), PlaybackRequest{Caps: chrome}, nil, 0, true)
	if d.Mode != "transcode" {
		t.Fatalf("hevc → transcode, got %+v", d)
	}
	r := strings.Join(d.Reasons, ";")
	if !strings.Contains(r, "hevc videót") || !strings.Contains(r, "ac3 hang") {
		t.Fatalf("reasons: %v", d.Reasons)
	}
	if d.Bitrate != 10_000_000 || d.Width != 1920 {
		t.Fatalf("bitrate cap / width: %d %d", d.Bitrate, d.Width)
	}
	// bitrate limit forces transcode even for compatible files
	d = Decide(item("mp4", "h264", "aac", 8_000_000), PlaybackRequest{Caps: chrome}, nil, 2000, true)
	if d.Mode != "transcode" || d.Bitrate != 2_000_000 || d.Width != 854 {
		t.Fatalf("limit: %+v", d)
	}
}

func TestDecideHDR(t *testing.T) {
	it := item("mp4", "hevc", "aac", 15_000_000)
	it.HDR = "HDR10"
	caps := chrome
	caps.Video = append(caps.Video, "hevc")
	if d := Decide(it, PlaybackRequest{Caps: caps}, nil, 0, true); d.Mode != "transcode" || !d.ToneMap {
		t.Fatalf("HDR without client support → tone-mapped transcode, got %+v", d)
	}
	caps.HDR = true
	if d := Decide(it, PlaybackRequest{Caps: caps}, nil, 0, true); d.Mode != "direct" {
		t.Fatalf("HDR capable client → direct, got %+v", d)
	}
}

func TestAudioSelection(t *testing.T) {
	tracks := []store.StreamInfo{
		{Type: "audio", TypeIndex: 0, Codec: "aac", Lang: "eng", Default: true},
		{Type: "audio", TypeIndex: 1, Codec: "aac", Lang: "hun"},
	}
	if a := ChooseAudio(tracks, nil, []string{"hun", "eng"}); a != 1 {
		t.Fatalf("profile priority: %d", a)
	}
	one := 0
	if a := ChooseAudio(tracks, &one, []string{"hun"}); a != 0 {
		t.Fatalf("explicit wins: %d", a)
	}
	if a := ChooseAudio(tracks, nil, nil); a != 0 {
		t.Fatalf("default flag: %d", a)
	}
	// non-first audio track → not direct
	d := Decide(item("mp4", "h264", "aac", 3_000_000, tracks...), PlaybackRequest{Caps: chrome}, []string{"hun"}, 0, true)
	if d.Mode != "remux" || d.AudioIdx != 1 {
		t.Fatalf("second track → remux, got %+v", d)
	}
}

func TestBurnIn(t *testing.T) {
	it := item("mp4", "h264", "aac", 3_000_000)
	it.Streams = append(it.Streams, store.StreamInfo{Type: "subtitle", TypeIndex: 0, Codec: "hdmv_pgs_subtitle"})
	i := 0
	d := Decide(it, PlaybackRequest{Caps: chrome, BurnSubIdx: &i}, nil, 0, true)
	if d.Mode != "transcode" || d.BurnSubIdx == nil {
		t.Fatalf("burn-in → transcode: %+v", d)
	}
	args := strings.Join(BuildHLSArgs(HLSSpec{Item: it, Decision: d, Encoder: AllEncoders[len(AllEncoders)-1]}, "/tmp/x"), " ")
	if !strings.Contains(args, "[0:v:0][0:s:0]overlay") {
		t.Fatalf("overlay missing: %s", args)
	}
}

func TestTargetWidth(t *testing.T) {
	for br, w := range map[int]int{15_000_000: 3840, 6_000_000: 1920, 3_000_000: 1280, 1_500_000: 854, 500_000: 640} {
		if got := TargetWidth(br); got != w {
			t.Errorf("TargetWidth(%d)=%d want %d", br, got, w)
		}
	}
}
