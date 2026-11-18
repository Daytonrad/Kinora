package media

import (
	"path/filepath"
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

var apple = Caps{Video: []string{"h264", "hevc"}, Audio: []string{"aac", "ac3", "eac3"}, Containers: []string{"mp4", "mov"}}

func TestDecideHEVCRemux(t *testing.T) {
	d := Decide(item("mkv", "hevc", "eac3", 12_000_000), PlaybackRequest{Caps: apple}, nil, 0, true)
	if d.Mode != "remux" || !d.VideoCopy || !d.FMP4 || !d.AudioCopy {
		t.Fatalf("mkv hevc+eac3 on an hevc/eac3 client → fMP4 remux with audio copy, got %+v", d)
	}
	args := strings.Join(BuildHLSArgs(HLSSpec{Item: &store.Item{Path: "/x.mkv", VideoCodec: "hevc"}, Decision: d}, "/tmp/s"), " ")
	for _, want := range []string{"-c:v copy", "-tag:v hvc1", "-c:a copy", "-hls_segment_type fmp4", "seg%05d.m4s"} {
		if !strings.Contains(args, want) {
			t.Fatalf("missing %q in %s", want, args)
		}
	}
	// Output paths go to ffmpeg with '/' even on Windows: the HLS muxer puts init.mp4 next to
	// the playlist only when it finds a '/' in the playlist path.
	dir := filepath.Join(t.TempDir(), "sess")
	out := BuildHLSArgs(HLSSpec{Item: &store.Item{Path: "/x.mkv"}, Decision: d}, dir)
	if pl := out[len(out)-1]; pl != filepath.ToSlash(dir)+"/index.m3u8" || strings.Contains(pl, `\`) {
		t.Fatalf("playlist path %q", pl)
	}
	// h264 remux stays on MPEG-TS
	d = Decide(item("mkv", "h264", "aac", 4_000_000), PlaybackRequest{Caps: apple}, nil, 0, true)
	if d.Mode != "remux" || d.FMP4 {
		t.Fatalf("h264 remux must not use fMP4: %+v", d)
	}
	if a := strings.Join(BuildHLSArgs(HLSSpec{Item: &store.Item{Path: "/x.mkv"}, Decision: d}, "/tmp/s"), " "); !strings.Contains(a, "seg%05d.ts") || strings.Contains(a, "fmp4") {
		t.Fatalf("ts args: %s", a)
	}
	// HDR HEVC without HDR support, or a client without HEVC, still transcodes
	hdr := item("mkv", "hevc", "aac", 12_000_000)
	hdr.HDR = "HDR10"
	if d := Decide(hdr, PlaybackRequest{Caps: apple}, nil, 0, true); d.Mode != "transcode" {
		t.Fatalf("hdr hevc → transcode, got %+v", d)
	}
	if d := Decide(item("mkv", "hevc", "aac", 12_000_000), PlaybackRequest{Caps: chrome}, nil, 0, true); d.Mode != "transcode" {
		t.Fatalf("no hevc caps → transcode, got %+v", d)
	}
}

func TestSurroundAudioCopy(t *testing.T) {
	// h264 + eac3 on a client with eac3: audio copied, fMP4 (hls.js can't take AC-3 from TS), no hvc1 tag
	d := Decide(item("mkv", "h264", "eac3", 4_000_000), PlaybackRequest{Caps: apple}, nil, 0, true)
	if d.Mode != "remux" || !d.AudioCopy || !d.FMP4 {
		t.Fatalf("eac3 copy: %+v", d)
	}
	a := strings.Join(BuildHLSArgs(HLSSpec{Item: &store.Item{Path: "/x.mkv", VideoCodec: "h264"}, Decision: d}, "/tmp/s"), " ")
	if !strings.Contains(a, "-c:a copy") || !strings.Contains(a, "fmp4") || strings.Contains(a, "hvc1") {
		t.Fatalf("eac3 copy args: %s", a)
	}
	// a client without eac3 gets AAC on MPEG-TS
	d = Decide(item("mkv", "h264", "eac3", 4_000_000), PlaybackRequest{Caps: chrome}, nil, 0, true)
	if d.AudioCopy || d.FMP4 {
		t.Fatalf("eac3 without caps: %+v", d)
	}
}

func testEncoders(ok ...string) *Encoders {
	e := NewEncoders()
	for _, enc := range AllEncoders {
		r := EncoderResult{Encoder: enc}
		for _, n := range ok {
			r.OK = r.OK || n == enc.Name
		}
		e.results = append(e.results, r)
	}
	return e
}

func TestPickFor(t *testing.T) {
	e := testEncoders("h264_nvenc", "hevc_nvenc", "av1_nvenc", "h264_qsv", "libx264")
	cases := []struct {
		configured, mode string
		caps             Caps
		want             string
	}{
		{"auto", "auto", apple, "hevc_nvenc"},
		{"auto", "auto", Caps{Video: []string{"h264", "av1"}}, "av1_nvenc"},
		{"auto", "auto", chrome, "av1_nvenc"}, // chrome reports av1, no hevc
		{"auto", "auto", Caps{Video: []string{"h264"}}, "h264_nvenc"},
		{"auto", "h264", apple, "h264_nvenc"},
		{"h264_qsv", "auto", apple, "h264_qsv"}, // hevc_qsv failed the self-test
		{"libx264", "auto", apple, "libx264"},   // no software HEVC/AV1
		{"hevc_nvenc", "auto", Caps{Video: []string{"h264"}}, "h264_nvenc"},
	}
	for _, c := range cases {
		if got := e.PickFor(c.configured, c.mode, c.caps).Name; got != c.want {
			t.Errorf("PickFor(%q,%q,%v)=%s want %s", c.configured, c.mode, c.caps.Video, got, c.want)
		}
	}
	if got := e.Pick("hevc_nvenc").Name; got != "h264_nvenc" {
		t.Errorf("Pick(hevc_nvenc)=%s want the family's H.264", got)
	}
}

func TestHEVCTranscodeArgs(t *testing.T) {
	it := item("mkv", "mpeg4", "aac", 6_000_000)
	d := Decide(it, PlaybackRequest{Caps: apple}, nil, 0, true)
	enc, _ := encoderByName("hevc_nvenc")
	a := strings.Join(BuildHLSArgs(HLSSpec{Item: it, Decision: d, Encoder: enc}, "/tmp/s"), " ")
	for _, want := range []string{"-c:v hevc_nvenc", "-tag:v hvc1", "-cq 25", "-maxrate", "-hls_segment_type fmp4"} {
		if !strings.Contains(a, want) {
			t.Fatalf("missing %q in %s", want, a)
		}
	}
	x, _ := encoderByName("libx264")
	a = strings.Join(BuildHLSArgs(HLSSpec{Item: it, Decision: d, Encoder: x}, "/tmp/s"), " ")
	if !strings.Contains(a, "-crf 21") || strings.Contains(a, "fmp4") || strings.Contains(a, "-b:v") {
		t.Fatalf("libx264 args: %s", a)
	}
}
