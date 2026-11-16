package media

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ScotyLucas/kinora/internal/store"
)

// Quality is an offline download preset.
type Quality struct {
	Name    string `json:"name"`
	Label   string `json:"label"`
	Width   int    `json:"width"`
	Bitrate int    `json:"bitrate"` // video bps
}

// Qualities for transcoded downloads ("original" is served directly).
var Qualities = []Quality{
	{"1080p", "1080p (~6 Mbps)", 1920, 6_000_000},
	{"720p", "720p (~3 Mbps)", 1280, 3_000_000},
	{"480p", "480p (~1,2 Mbps)", 854, 1_200_000},
	{"360p", "360p (~600 kbps)", 640, 600_000},
}

// QualityByName finds a preset.
func QualityByName(n string) (Quality, bool) {
	for _, q := range Qualities {
		if q.Name == n {
			return q, true
		}
	}
	return Quality{}, false
}

// DownloadRetention is how long finished downloads are kept.
const DownloadRetention = 7 * 24 * time.Hour

// Job is a background offline-download transcode.
type Job struct {
	Key       string  `json:"key"`
	ItemID    int64   `json:"item_id"`
	UserID    int64   `json:"user_id"`
	Title     string  `json:"title"`
	FileName  string  `json:"file_name"`
	Quality   string  `json:"quality"`
	AudioIdx  int     `json:"audio_idx"`
	Status    string  `json:"status"` // queued | running | done | error
	Progress  float64 `json:"progress"`
	Error     string  `json:"error,omitempty"`
	Size      int64   `json:"size,omitempty"`
	CreatedAt int64   `json:"created_at"`
	DoneAt    int64   `json:"done_at,omitempty"`

	src      string
	duration float64
	hdr      bool
}

// Downloader runs one download job at a time: a single worker goroutine
// drains the queue, which acts as the semaphore.
type Downloader struct {
	dir   string
	enc   *Encoders
	log   *slog.Logger
	queue chan *Job
	mu    sync.Mutex
	jobs  map[string]*Job

	// EncoderSetting returns the admin's encoder choice ("auto" or a name).
	EncoderSetting func() string
}

// NewDownloader loads finished jobs from <cache>/downloads.
func NewDownloader(cacheDir string, enc *Encoders, log *slog.Logger) *Downloader {
	d := &Downloader{dir: filepath.Join(cacheDir, "downloads"), enc: enc, log: log, jobs: map[string]*Job{},
		queue: make(chan *Job, 256), EncoderSetting: func() string { return "auto" }}
	os.MkdirAll(d.dir, 0o755)
	entries, _ := os.ReadDir(d.dir)
	for _, e := range entries {
		n := e.Name()
		if strings.HasSuffix(n, ".part") {
			os.Remove(filepath.Join(d.dir, n)) // interrupted by a restart
			continue
		}
		if !strings.HasSuffix(n, ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(d.dir, n))
		var j Job
		if err != nil || json.Unmarshal(b, &j) != nil || j.Status != "done" {
			os.Remove(filepath.Join(d.dir, n))
			continue
		}
		if _, err := os.Stat(d.file(j.Key)); err == nil {
			d.jobs[j.Key] = &j
		}
	}
	return d
}

func (d *Downloader) file(key string) string { return filepath.Join(d.dir, key+".mp4") }

// JobKey is stable for item/file/quality/audio, so repeated requests reuse the job.
func JobKey(it *store.Item, quality string, audioIdx int) string {
	h := sha1.Sum([]byte(fmt.Sprintf("%d|%s|%d|%d|%s|%d", it.ID, it.Path, it.Size, it.UpdatedAt, quality, audioIdx)))
	return hex.EncodeToString(h[:])[:20]
}

// Run processes the queue until ctx ends; it also prunes old downloads hourly.
func (d *Downloader) Run(ctx context.Context) {
	d.prune()
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			d.prune()
		case j := <-d.queue:
			d.run(ctx, j)
		}
	}
}

func (d *Downloader) prune() {
	cut := time.Now().Add(-DownloadRetention).Unix()
	d.mu.Lock()
	defer d.mu.Unlock()
	for k, j := range d.jobs {
		if (j.Status == "done" || j.Status == "error") && j.CreatedAt < cut {
			os.Remove(d.file(k))
			os.Remove(filepath.Join(d.dir, k+".json"))
			delete(d.jobs, k)
		}
	}
}

// Enqueue adds a job, or returns the existing one with the same key.
func (d *Downloader) Enqueue(it *store.Item, userID int64, q Quality, audioIdx int, fileName string) *Job {
	key := JobKey(it, q.Name, audioIdx)
	d.mu.Lock()
	if j := d.jobs[key]; j != nil && j.Status != "error" {
		d.mu.Unlock()
		return j
	}
	j := &Job{Key: key, ItemID: it.ID, UserID: userID, Title: it.Title, FileName: fileName, Quality: q.Name, AudioIdx: audioIdx,
		Status: "queued", CreatedAt: time.Now().Unix(), src: it.Path, duration: float64(it.RuntimeSec), hdr: it.HDR != ""}
	d.jobs[key] = j
	d.mu.Unlock()
	d.queue <- j
	return j
}

// Jobs lists jobs (snapshot), newest first. userID 0 = all.
func (d *Downloader) Jobs(userID int64) []Job {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := []Job{}
	for _, j := range d.jobs {
		if userID == 0 || j.UserID == userID {
			out = append(out, *j)
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].CreatedAt > out[b].CreatedAt })
	return out
}

// Get returns a job snapshot.
func (d *Downloader) Get(key string) (Job, string, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	j := d.jobs[key]
	if j == nil {
		return Job{}, "", false
	}
	return *j, d.file(key), true
}

// Active counts queued/running jobs.
func (d *Downloader) Active() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := 0
	for _, j := range d.jobs {
		if j.Status == "queued" || j.Status == "running" {
			n++
		}
	}
	return n
}

func (d *Downloader) set(j *Job, f func(*Job)) {
	d.mu.Lock()
	f(j)
	d.mu.Unlock()
}

func (d *Downloader) run(ctx context.Context, j *Job) {
	q, _ := QualityByName(j.Quality)
	enc := d.enc.Pick(d.EncoderSetting())
	opts := EncodeOpts{Bitrate: q.Bitrate, Width: q.Width, ToneMap: j.hdr && d.enc.ToneMap()}
	part := d.file(j.Key) + ".part"
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y"}
	args = append(args, enc.InitArgs()...)
	args = append(args, "-i", j.src, "-map", "0:v:0", "-vf", enc.Filter(opts))
	args = append(args, enc.Args(opts)...)
	if j.AudioIdx >= 0 {
		args = append(args, "-map", fmt.Sprintf("0:a:%d", j.AudioIdx), "-c:a", "aac", "-b:a", "160k", "-ac", "2")
	}
	args = append(args, "-sn", "-dn", "-movflags", "+faststart", "-progress", "pipe:1", "-nostats", "-f", "mp4", part)

	d.set(j, func(j *Job) { j.Status = "running" })
	d.log.Info("download started", "title", j.Title, "quality", j.Quality, "encoder", enc.Name)
	cmd := exec.CommandContext(ctx, FFmpeg, args...)
	tb := &tailBuffer{max: 4096}
	cmd.Stderr = tb
	stdout, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		d.set(j, func(j *Job) { j.Status, j.Error = "error", err.Error() })
		return
	}
	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		line := sc.Text()
		if v, ok := strings.CutPrefix(line, "out_time_us="); ok && j.duration > 0 {
			us, _ := strconv.ParseFloat(v, 64)
			p := us / 1e6 / j.duration
			if p > 0.999 {
				p = 0.999
			}
			if p >= 0 {
				d.set(j, func(j *Job) { j.Progress = p })
			}
		}
	}
	err := cmd.Wait()
	if err != nil {
		os.Remove(part)
		msg := firstNonEmpty(lastLine(tb.String()), err.Error())
		d.set(j, func(j *Job) { j.Status, j.Error = "error", "a letöltés átkódolása nem sikerült: "+msg })
		d.log.Warn("download failed", "title", j.Title, "err", msg)
		return
	}
	if err := os.Rename(part, d.file(j.Key)); err != nil {
		d.set(j, func(j *Job) { j.Status, j.Error = "error", err.Error() })
		return
	}
	st, _ := os.Stat(d.file(j.Key))
	d.set(j, func(j *Job) {
		j.Status, j.Progress, j.DoneAt = "done", 1, time.Now().Unix()
		if st != nil {
			j.Size = st.Size()
		}
	})
	d.mu.Lock()
	b, _ := json.Marshal(j)
	d.mu.Unlock()
	os.WriteFile(filepath.Join(d.dir, j.Key+".json"), b, 0o644)
	d.log.Info("download ready", "title", j.Title, "quality", j.Quality)
}
