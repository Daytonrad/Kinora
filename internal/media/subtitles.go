package media

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/ScotyLucas/kinora/internal/store"
)

// Subtitles converts text subtitles to cached WebVTT.
type Subtitles struct {
	dir string
	mu  sync.Mutex // serializes conversions of the same file
	inF map[string]*sync.Mutex
}

// NewSubtitles stores converted files under <cache>/subs.
func NewSubtitles(cacheDir string) *Subtitles {
	d := filepath.Join(cacheDir, "subs")
	os.MkdirAll(d, 0o755)
	return &Subtitles{dir: d, inF: map[string]*sync.Mutex{}}
}

// IsUTF8 reports whether a subtitle file is valid UTF-8 (BOM allowed).
func IsUTF8(b []byte) bool {
	b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})
	return utf8.Valid(b)
}

// VTT returns the path of a WebVTT version of a text subtitle stream.
func (s *Subtitles) VTT(ctx context.Context, it *store.Item, st store.StreamInfo) (string, error) {
	if !st.TextBased {
		return "", fmt.Errorf("a(z) %s felirat képalapú, csak beégetve jeleníthető meg", st.Codec)
	}
	src := it.Path
	if st.External {
		src = st.ExternalPath
	}
	fi, err := os.Stat(src)
	if err != nil {
		return "", fmt.Errorf("a feliratfájl nem érhető el: %w", err)
	}
	h := sha1.Sum([]byte(fmt.Sprintf("%s|%d|%d|%d|%v", src, fi.Size(), fi.ModTime().UnixNano(), st.TypeIndex, st.External)))
	key := hex.EncodeToString(h[:])
	out := filepath.Join(s.dir, key+".vtt")
	if _, err := os.Stat(out); err == nil {
		return out, nil
	}

	s.mu.Lock()
	m := s.inF[key]
	if m == nil {
		m = &sync.Mutex{}
		s.inF[key] = m
	}
	s.mu.Unlock()
	m.Lock()
	defer m.Unlock()
	if _, err := os.Stat(out); err == nil {
		return out, nil
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y"}
	if st.External {
		if b, err := os.ReadFile(src); err == nil && !IsUTF8(b) {
			// Hungarian (and most Central European) .srt files are Windows-1250.
			args = append(args, "-sub_charenc", "CP1250")
		}
		args = append(args, "-i", src, "-map", "0:s:0")
	} else {
		args = append(args, "-i", src, "-map", fmt.Sprintf("0:s:%d", st.TypeIndex))
	}
	tmp := out + ".tmp"
	args = append(args, "-c:s", "webvtt", "-f", "webvtt", tmp)
	if b, err := exec.CommandContext(ctx, FFmpeg, args...).CombinedOutput(); err != nil {
		os.Remove(tmp)
		return "", fmt.Errorf("felirat konvertálása sikertelen: %s", firstNonEmpty(lastLine(string(b)), err.Error()))
	}
	return out, os.Rename(tmp, out)
}
