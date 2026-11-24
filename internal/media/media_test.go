package media

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ScotyLucas/kinora/internal/store"
)

func testFile(t *testing.T, rel string) string {
	t.Helper()
	p, _ := filepath.Abs(filepath.Join("../../testmedia", rel))
	if _, err := os.Stat(p); err != nil {
		t.Skip("testmedia missing – run scripts/mkmedia.sh testmedia")
	}
	return p
}

func probeItem(t *testing.T, path string) *store.Item {
	t.Helper()
	pr, err := Probe(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	it := &store.Item{ID: 1, Title: filepath.Base(path), Path: path, Container: pr.Container, VideoCodec: pr.VideoCodec,
		AudioCodec: pr.AudioCodec, Width: pr.Width, Height: pr.Height, Bitrate: pr.Bitrate, HDR: pr.HDR, RuntimeSec: int(pr.Duration)}
	for _, s := range pr.Streams {
		it.Streams = append(it.Streams, store.StreamInfo{Index: s.Index, Type: s.Type, TypeIndex: s.TypeIndex, Codec: s.Codec, Lang: s.Lang,
			Default: s.Default, TextBased: s.Type == "subtitle" && store.TextSubtitleCodecs[s.Codec]})
	}
	return it
}

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func TestProbe(t *testing.T) {
	pr, err := Probe(context.Background(), testFile(t, "Movies/Big Test Movie (2019)/Big Test Movie (2019).mp4"))
	if err != nil {
		t.Fatal(err)
	}
	if pr.Container != "mp4" || pr.VideoCodec != "h264" || pr.AudioCodec != "aac" || len(pr.Chapters) != 2 || pr.CreditsStart == nil || *pr.CreditsStart != 50 {
		t.Fatalf("%+v", pr)
	}
}

func TestProbeSubtitleDisposition(t *testing.T) {
	path := filepath.Join(t.TempDir(), "subtitles.mkv")
	cmd := exec.Command(FFmpeg, "-v", "error", "-f", "srt", "-i", "pipe:0",
		"-map", "0:s:0", "-map", "0:s:0", "-c", "copy",
		"-disposition:s:0", "hearing_impaired", "-disposition:s:1", "0", path)
	cmd.Stdin = strings.NewReader("1\n00:00:00,000 --> 00:00:01,000\nHello\n")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generate test subtitles: %v: %s", err, out)
	}
	pr, err := Probe(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if len(pr.Streams) != 2 || !pr.Streams[0].HearingImpaired || pr.Streams[1].HearingImpaired {
		t.Fatalf("unexpected subtitle dispositions: %+v", pr.Streams)
	}
}

func segmentProbe(t *testing.T, seg string) string {
	t.Helper()
	out, err := exec.Command(FFprobe, "-v", "error", "-show_entries", "stream=codec_name", "-of", "csv=p=0", seg).Output()
	if err != nil {
		t.Fatalf("ffprobe segment: %v", err)
	}
	// mpegts lists streams once per program as well: de-duplicate
	seen := map[string]bool{}
	var uniq []string
	for _, f := range strings.Fields(string(out)) {
		if !seen[f] {
			seen[f] = true
			uniq = append(uniq, f)
		}
	}
	return strings.Join(uniq, ",")
}

func runSession(t *testing.T, it *store.Item, caps Caps, wantMode string) {
	t.Helper()
	enc := NewEncoders()
	enc.SelfTest(context.Background())
	m := NewManager(t.TempDir(), enc, quiet)
	defer m.StopAll()
	d := Decide(it, PlaybackRequest{Caps: caps}, nil, 0, enc.ToneMap())
	if d.Mode != wantMode {
		t.Fatalf("mode %s, want %s (%v)", d.Mode, wantMode, d.Reasons)
	}
	s, err := m.Start(context.Background(), HLSSpec{Item: it, UserID: 7, Decision: d, Start: 5, Encoder: enc.Pick("auto")})
	if err != nil {
		t.Fatal(err)
	}
	if m.Get(s.ID, 8) != nil {
		t.Fatal("session must be bound to its user")
	}
	pl, err := s.Playlist()
	if err != nil || !strings.Contains(string(pl), "seg00000.ts") {
		t.Fatalf("playlist: %s %v", pl, err)
	}
	// wait for the first segment to be complete, then check it decodes as h264+aac
	deadline := time.Now().Add(20 * time.Second)
	for !strings.Contains(string(pl), "seg00001.ts") && !s.Finished() && time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
		pl, _ = s.Playlist()
	}
	if got := segmentProbe(t, filepath.Join(s.Dir, "seg00000.ts")); got != "h264,aac" {
		t.Fatalf("segment streams = %q", got)
	}
	for !s.Finished() && time.Now().Before(deadline.Add(30*time.Second)) {
		time.Sleep(200 * time.Millisecond)
	}
	pl, _ = s.Playlist()
	if !strings.Contains(string(pl), "#EXT-X-ENDLIST") {
		t.Fatal("finished playlist must end with ENDLIST")
	}
	dir := s.Dir
	m.Stop(s.ID)
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("session dir not removed")
	}
}

func TestRemuxSession(t *testing.T) {
	it := probeItem(t, testFile(t, "Shows/Test Show (2020)/Season 01/Test.Show.S01E01.Pilot.Part.1.mkv"))
	runSession(t, it, chrome, "remux")
}

func TestTranscodeSession(t *testing.T) {
	it := probeItem(t, testFile(t, "Movies/Space Drama (2015)/Space.Drama.2015.1080p.BluRay.x265.mkv"))
	runSession(t, it, chrome, "transcode")
}

func TestCP1250Subtitle(t *testing.T) {
	srt := testFile(t, "Movies/Big Test Movie (2019)/Big Test Movie (2019).hu.srt")
	b, _ := os.ReadFile(srt)
	if IsUTF8(b) {
		t.Fatal("fixture should be CP1250")
	}
	subs := NewSubtitles(t.TempDir())
	it := &store.Item{Path: "unused"}
	p, err := subs.VTT(context.Background(), it, store.StreamInfo{Type: "subtitle", Codec: "subrip", External: true, ExternalPath: srt, TextBased: true})
	if err != nil {
		t.Fatal(err)
	}
	out, _ := os.ReadFile(p)
	if !strings.HasPrefix(string(out), "WEBVTT") || !strings.Contains(string(out), "Árvíztűrő tükörfúrógép") || !strings.Contains(string(out), "Őszi éjszaka") {
		t.Fatalf("bad VTT:\n%s", out)
	}
}

func TestEmbeddedSubtitle(t *testing.T) {
	it := probeItem(t, testFile(t, "Movies/Space Drama (2015)/Space.Drama.2015.1080p.BluRay.x265.mkv"))
	var sub store.StreamInfo
	for _, s := range it.Streams {
		if s.Type == "subtitle" {
			sub = s
		}
	}
	if sub.Lang != "eng" || !sub.TextBased {
		t.Fatalf("embedded sub: %+v", sub)
	}
	p, err := NewSubtitles(t.TempDir()).VTT(context.Background(), it, sub)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := os.ReadFile(p)
	if !strings.Contains(string(out), "Houston, we have a subtitle.") {
		t.Fatalf("VTT: %s", out)
	}
}

func TestEncoderSelfTest(t *testing.T) {
	enc := NewEncoders()
	res := enc.SelfTest(context.Background())
	sw := false
	for _, r := range res {
		if r.Name == "libx264" && r.OK {
			sw = true
		}
		if !r.OK && r.Error == "" {
			t.Errorf("%s failed without a reason", r.Name)
		}
	}
	if !sw {
		t.Fatalf("libx264 must work: %+v", res)
	}
	if enc.Pick("h264_nonexistent").Name == "" {
		t.Fatal("pick must fall back")
	}
}

func TestDownload(t *testing.T) {
	it := probeItem(t, testFile(t, "Movies/Kids.Cartoon.2018.mp4"))
	enc := NewEncoders()
	enc.SelfTest(context.Background())
	d := NewDownloader(t.TempDir(), enc, quiet)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)
	q, _ := QualityByName("360p")
	j, err := d.Enqueue(it, 1, q, 0, "Kids Cartoon [360p].mp4")
	if err != nil {
		t.Fatal(err)
	}
	if j2, _ := d.Enqueue(it, 1, q, 0, "x"); j2.Key != j.Key {
		t.Fatal("same request must reuse the job")
	}
	// another user asking for the same file shares the job and sees it too
	if j3, _ := d.Enqueue(it, 2, q, 0, "y"); j3.Key != j.Key || !j3.HasUser(2) || !j3.HasUser(1) {
		t.Fatalf("second user must join the job: %+v", j3)
	}
	if len(d.Jobs(2)) != 1 || len(d.Jobs(3)) != 0 {
		t.Fatalf("jobs per user: %d / %d", len(d.Jobs(2)), len(d.Jobs(3)))
	}
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		got, path, _ := d.Get(j.Key)
		if got.Status == "done" {
			if got.Progress != 1 || got.Size == 0 {
				t.Fatalf("%+v", got)
			}
			out, _ := exec.Command(FFprobe, "-v", "error", "-show_entries", "stream=codec_name,width", "-of", "csv=p=0", path).Output()
			if !strings.Contains(string(out), "h264,640") {
				t.Fatalf("download streams: %s", out)
			}
			return
		}
		if got.Status == "error" {
			t.Fatal(got.Error)
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("download timed out")
}

// Enqueue never blocks a request handler: a full queue and a user with too many pending jobs are errors.
func TestDownloadQueueLimits(t *testing.T) {
	d := NewDownloader(t.TempDir(), NewEncoders(), quiet) // Run is not started: nothing leaves the queue
	q, _ := QualityByName("360p")
	item := func(id int64) *store.Item { return &store.Item{ID: id, Title: "x", Path: "/none"} }
	for i := 0; i < MaxJobsPerUser; i++ {
		if _, err := d.Enqueue(item(int64(i+1)), 1, q, 0, "x.mp4"); err != nil {
			t.Fatalf("job %d: %v", i, err)
		}
	}
	if _, err := d.Enqueue(item(999), 1, q, 0, "x.mp4"); err != ErrTooManyJobs {
		t.Fatalf("over the per-user limit: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		var err error
		for i := 0; i < maxQueue+5 && err == nil; i++ {
			_, err = d.Enqueue(item(int64(1000+i)), int64(100+i), q, 0, "x.mp4")
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err != ErrQueueFull {
			t.Fatalf("full queue: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Enqueue blocked on a full queue")
	}
}

func fakeSession(m *Manager, id string, user int64, seen time.Time) {
	s := &Session{ID: id, UserID: user, Dir: filepath.Join(m.dir, id), done: make(chan struct{})}
	close(s.done) // an exited ffmpeg: Stop only removes the session
	s.lastSeen.Store(seen.UnixNano())
	m.sess[id] = s
}

func TestSessionLimits(t *testing.T) {
	m := NewManager(t.TempDir(), NewEncoders(), quiet)
	now := time.Now()
	for i := 0; i < MaxSessionsPerUser; i++ {
		fakeSession(m, fmt.Sprintf("u1-%d", i), 1, now.Add(time.Duration(i)*time.Second))
	}
	fakeSession(m, "u2", 2, now.Add(-time.Hour))
	if err := m.makeRoom(1); err != nil {
		t.Fatal(err)
	}
	if m.sess["u1-0"] != nil {
		t.Fatal("the least recently used session of the user must stop")
	}
	if m.sess["u2"] == nil || m.sess["u1-1"] == nil {
		t.Fatal("other sessions must stay")
	}

	m.MaxSessions = len(m.sess)
	if err := m.makeRoom(3); err != ErrBusy {
		t.Fatalf("server-wide limit: %v", err)
	}
	m.MaxSessions = 0
	if err := m.makeRoom(3); err != nil {
		t.Fatal(err)
	}
}
