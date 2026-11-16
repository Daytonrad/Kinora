package media

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ScotyLucas/kinora/internal/store"
)

// IdleTimeout stops a session nobody fetched from for this long.
const IdleTimeout = 90 * time.Second

// Session is one running HLS ffmpeg process.
type Session struct {
	ID       string
	UserID   int64
	ItemID   int64
	Title    string
	Mode     string
	Encoder  string
	Start    float64
	Dir      string
	Started  time.Time
	cmd      *exec.Cmd
	done     chan struct{}
	stderr   *tailBuffer
	err      error
	lastSeen atomic.Int64
	stopping atomic.Bool // set by Stop: an exit after this is expected
}

// Finished reports whether ffmpeg exited.
func (s *Session) Finished() bool {
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

func (s *Session) touch() { s.lastSeen.Store(time.Now().UnixNano()) }

// SessionInfo is a dashboard view of a session.
type SessionInfo struct {
	ID      string `json:"id"`
	ItemID  int64  `json:"item_id"`
	Title   string `json:"title"`
	Mode    string `json:"mode"`
	Encoder string `json:"encoder"`
	UserID  int64  `json:"user_id"`
	Running bool   `json:"running"`
	Seconds int    `json:"seconds"`
}

// Manager owns transcode sessions.
type Manager struct {
	dir  string
	log  *slog.Logger
	Enc  *Encoders
	mu   sync.Mutex
	sess map[string]*Session
}

// NewManager creates the manager and wipes leftovers of previous runs.
func NewManager(cacheDir string, enc *Encoders, log *slog.Logger) *Manager {
	dir := filepath.Join(cacheDir, "transcode")
	os.RemoveAll(dir)
	os.MkdirAll(dir, 0o755)
	return &Manager{dir: dir, log: log, Enc: enc, sess: map[string]*Session{}}
}

// Sessions lists active sessions.
func (m *Manager) Sessions() []SessionInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []SessionInfo{}
	for _, s := range m.sess {
		out = append(out, SessionInfo{ID: s.ID, ItemID: s.ItemID, Title: s.Title, Mode: s.Mode, Encoder: s.Encoder, UserID: s.UserID,
			Running: !s.Finished(), Seconds: int(time.Since(s.Started).Seconds())})
	}
	return out
}

// HLSSpec describes what to produce.
type HLSSpec struct {
	Item     *store.Item
	UserID   int64
	Decision Decision
	Start    float64
	Encoder  Encoder
}

// BuildHLSArgs returns the ffmpeg command line for a session writing into dir.
func BuildHLSArgs(sp HLSSpec, dir string) []string {
	d := sp.Decision
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin"}
	if !d.VideoCopy {
		args = append(args, sp.Encoder.InitArgs()...)
	}
	if sp.Start > 0 {
		args = append(args, "-ss", fmt.Sprintf("%.3f", sp.Start))
	}
	args = append(args, "-i", sp.Item.Path)

	opts := EncodeOpts{Bitrate: d.Bitrate, Width: d.Width, ToneMap: d.ToneMap, GOP: true}
	if d.VideoCopy {
		args = append(args, "-map", "0:v:0", "-c:v", "copy")
	} else if d.BurnSubIdx != nil {
		fc := fmt.Sprintf("[0:v:0][0:s:%d]overlay=eof_action=pass,%s[v]", *d.BurnSubIdx, sp.Encoder.Filter(opts))
		args = append(args, "-filter_complex", fc, "-map", "[v]")
		args = append(args, sp.Encoder.Args(opts)...)
	} else {
		args = append(args, "-map", "0:v:0", "-vf", sp.Encoder.Filter(opts))
		args = append(args, sp.Encoder.Args(opts)...)
	}
	if d.AudioIdx >= 0 {
		args = append(args, "-map", fmt.Sprintf("0:a:%d", d.AudioIdx))
		if d.AudioCopy {
			args = append(args, "-c:a", "copy")
		} else {
			args = append(args, "-c:a", "aac", "-b:a", "192k", "-ac", "2")
		}
	}
	args = append(args, "-sn", "-dn", "-avoid_negative_ts", "make_zero", "-max_muxing_queue_size", "2048",
		"-f", "hls", "-hls_time", "4", "-hls_list_size", "0", "-hls_playlist_type", "event",
		"-hls_flags", "independent_segments+temp_file",
		"-hls_segment_filename", filepath.Join(dir, "seg%05d.ts"),
		filepath.Join(dir, "index.m3u8"))
	return args
}

// Start launches ffmpeg and waits (max 30 s) until the playlist has a segment.
func (m *Manager) Start(ctx context.Context, sp HLSSpec) (*Session, error) {
	id := store.RandomToken(12)
	dir := filepath.Join(m.dir, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	args := BuildHLSArgs(sp, dir)
	cmd := exec.Command(FFmpeg, args...)
	tb := &tailBuffer{max: 8192}
	cmd.Stderr = tb
	s := &Session{ID: id, UserID: sp.UserID, ItemID: sp.Item.ID, Title: sp.Item.Title, Mode: sp.Decision.Mode,
		Encoder: sp.Encoder.Name, Start: sp.Start, Dir: dir, Started: time.Now(), cmd: cmd, done: make(chan struct{}), stderr: tb}
	if sp.Decision.VideoCopy {
		s.Encoder = "copy"
	}
	s.touch()
	m.log.Debug("ffmpeg", "args", strings.Join(args, " "))
	if err := cmd.Start(); err != nil {
		os.RemoveAll(dir)
		return nil, fmt.Errorf("az ffmpeg nem indult el: %w", err)
	}
	go func() {
		s.err = cmd.Wait()
		close(s.done)
		if s.err != nil && !s.stopping.Load() {
			m.log.Warn("ffmpeg exited", "session", id, "err", s.err, "stderr", lastLine(tb.String()))
		}
	}()
	m.mu.Lock()
	m.sess[id] = s
	m.mu.Unlock()
	m.log.Info("transcode started", "session", id, "item", sp.Item.Title, "mode", s.Mode, "encoder", s.Encoder, "start", sp.Start)

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if hasSegment(filepath.Join(dir, "index.m3u8")) {
			return s, nil
		}
		if s.Finished() {
			if hasSegment(filepath.Join(dir, "index.m3u8")) {
				return s, nil
			}
			m.Stop(id)
			msg := lastLine(tb.String())
			if msg == "" && s.err != nil {
				msg = s.err.Error()
			}
			return nil, fmt.Errorf("az átkódolás nem sikerült: %s", msg)
		}
		select {
		case <-ctx.Done():
			m.Stop(id)
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	msg := lastLine(tb.String())
	m.Stop(id)
	return nil, fmt.Errorf("az átkódolás 30 mp alatt sem adott első szegmenst (lassú encoder?) %s", msg)
}

func hasSegment(playlist string) bool {
	b, err := os.ReadFile(playlist)
	return err == nil && bytes.Contains(b, []byte(".ts"))
}

// Get returns a session owned by userID.
func (m *Manager) Get(id string, userID int64) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.sess[id]
	if s == nil || s.UserID != userID {
		return nil
	}
	s.touch()
	return s
}

// Stop kills a session and removes its files.
func (m *Manager) Stop(id string) {
	m.mu.Lock()
	s := m.sess[id]
	delete(m.sess, id)
	m.mu.Unlock()
	if s == nil {
		return
	}
	s.stopping.Store(true)
	if !s.Finished() {
		gracefulStop(s.cmd)
		select {
		case <-s.done:
		case <-time.After(3 * time.Second):
			s.cmd.Process.Kill()
			<-s.done
		}
	}
	os.RemoveAll(s.Dir)
	m.log.Debug("transcode stopped", "session", id)
}

// StopAll stops every session (shutdown).
func (m *Manager) StopAll() {
	m.mu.Lock()
	ids := make([]string, 0, len(m.sess))
	for id := range m.sess {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(id string) { defer wg.Done(); m.Stop(id) }(id)
	}
	wg.Wait()
}

// Reap stops idle sessions periodically until ctx ends.
func (m *Manager) Reap(ctx context.Context) {
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			cut := time.Now().Add(-IdleTimeout).UnixNano()
			m.mu.Lock()
			var idle []string
			for id, s := range m.sess {
				if s.lastSeen.Load() < cut {
					idle = append(idle, id)
				}
			}
			m.mu.Unlock()
			for _, id := range idle {
				m.log.Info("transcode idle, stopping", "session", id)
				m.Stop(id)
			}
		}
	}
}

// Playlist returns the playlist, appending #EXT-X-ENDLIST once ffmpeg is done.
func (s *Session) Playlist() ([]byte, error) {
	b, err := os.ReadFile(filepath.Join(s.Dir, "index.m3u8"))
	if err != nil {
		return nil, err
	}
	if s.Finished() && !bytes.Contains(b, []byte("#EXT-X-ENDLIST")) {
		if !bytes.HasSuffix(b, []byte("\n")) {
			b = append(b, '\n')
		}
		b = append(b, []byte("#EXT-X-ENDLIST\n")...)
	}
	return b, nil
}

// TranscodedSeconds sums the EXTINF durations written so far.
func (s *Session) TranscodedSeconds() float64 {
	f, err := os.Open(filepath.Join(s.Dir, "index.m3u8"))
	if err != nil {
		return 0
	}
	defer f.Close()
	total := 0.0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var d float64
		if _, err := fmt.Sscanf(sc.Text(), "#EXTINF:%f,", &d); err == nil {
			total += d
		}
	}
	return total
}

// tailBuffer keeps the last max bytes written (ffmpeg stderr).
type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.max {
		t.buf = t.buf[len(t.buf)-t.max:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}

var _ io.Writer = (*tailBuffer)(nil)
