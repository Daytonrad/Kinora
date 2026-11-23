package scanner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ScotyLucas/kinora/internal/media"
)

// Image widths (px).
const (
	PosterWidth   = 500
	BackdropWidth = 1280
	ThumbWidth    = 480
)

var imageExts = []string{".jpg", ".jpeg", ".png", ".webp"}

var (
	posterNames   = []string{"poster", "folder", "cover"}
	backdropNames = []string{"fanart", "backdrop", "background"}
)

// dirListing is a cached directory listing (lowercased name → real name).
type dirListing map[string]string

func listDir(dir string) dirListing {
	out := dirListing{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if !e.IsDir() {
			out[strings.ToLower(e.Name())] = e.Name()
		}
	}
	return out
}

// find returns the first existing file among base names × image extensions.
func (l dirListing) find(dir string, bases ...string) string {
	for _, b := range bases {
		for _, ext := range imageExts {
			if real, ok := l[strings.ToLower(b+ext)]; ok {
				return filepath.Join(dir, real)
			}
		}
	}
	return ""
}

// ImageDir is where artwork of item id lives.
func ImageDir(root string, id int64) string {
	return filepath.Join(root, fmt.Sprint(id))
}

// resizeImage converts any image ffmpeg can read into a width-limited JPEG.
func resizeImage(ctx context.Context, src, dst string, width int) error {
	return runFFmpeg(ctx, dst, "-i", src, "-vf", fmt.Sprintf("scale='min(%d,iw)':-2", width), "-frames:v", "1", "-q:v", "3")
}

// videoThumb grabs a representative frame around `at` seconds.
func videoThumb(ctx context.Context, video, dst string, at float64, width int) error {
	return runFFmpeg(ctx, dst, "-ss", fmt.Sprintf("%.2f", at), "-i", video,
		"-vf", fmt.Sprintf("thumbnail=25,scale=%d:-2", width), "-frames:v", "1", "-q:v", "4")
}

func runFFmpeg(ctx context.Context, dst string, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := dst + ".tmp.jpg"
	full := append([]string{"-hide_banner", "-loglevel", "error", "-y"}, args...)
	full = append(full, "-f", "image2", tmp)
	cmd := exec.CommandContext(ctx, media.FFmpeg, full...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		os.Remove(tmp)
		return fmt.Errorf("ffmpeg kép: %s", strings.TrimSpace(media.LastLine(string(out))+" "+err.Error()))
	}
	if st, err := os.Stat(tmp); err != nil || st.Size() == 0 {
		os.Remove(tmp)
		return fmt.Errorf("ffmpeg kép: üres kimenet")
	}
	return os.Rename(tmp, dst)
}
