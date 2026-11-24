package media

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A playlist whose last rename failed (Windows) must not end the video early.
func TestSettleTempAfterFailedRename(t *testing.T) {
	dir := t.TempDir()
	stale := "#EXTM3U\n#EXTINF:4,\nseg00000.m4s\n"
	full := stale + "#EXTINF:4,\nseg00001.m4s\n#EXT-X-ENDLIST\n"
	os.WriteFile(filepath.Join(dir, "index.m3u8"), []byte(stale), 0o644)
	os.WriteFile(filepath.Join(dir, "index.m3u8.tmp"), []byte(full), 0o644)
	os.WriteFile(filepath.Join(dir, "seg00001.m4s.tmp"), []byte("seg"), 0o644)
	s := &Session{Dir: dir, done: make(chan struct{})}
	close(s.done)

	// even before settleTemp, a finished session serves the complete files
	if b, err := s.Playlist(); err != nil || !strings.Contains(string(b), "seg00001.m4s") {
		t.Fatalf("playlist before settle: %q %v", b, err)
	}
	if p := s.SegmentPath("seg00001.m4s"); !strings.HasSuffix(p, ".tmp") {
		t.Fatalf("segment path before settle: %s", p)
	}

	if n := settleTemp(dir); n != 2 {
		t.Fatalf("settled %d files, want 2", n)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "index.m3u8")); string(b) != full {
		t.Fatalf("playlist not replaced: %q", b)
	}
	if _, err := os.Stat(filepath.Join(dir, "seg00001.m4s")); err != nil {
		t.Fatal("segment not put in place")
	}
	if p := s.SegmentPath("seg00001.m4s"); p != filepath.Join(dir, "seg00001.m4s") {
		t.Fatalf("segment path after settle: %s", p)
	}
}
