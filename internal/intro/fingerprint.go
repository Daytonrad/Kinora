// Package intro finds the shared intro (opening theme) and credits of a season's
// episodes from Chromaprint audio fingerprints, so players can offer "Skip intro".
//
// Jellyfin needs a third-party plugin for this; Kinora does it built in. The method:
// fingerprint the first N minutes of every episode, compare episode pairs at every
// time offset, take the longest stretch where the fingerprint items differ in at
// most a few bits, and let several pairs vote. Credits use the last minutes of each
// episode the same way, snapped to a black frame when one is nearby.
package intro

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// ItemSec is the time step of one Chromaprint item (algorithm 1 = TEST2:
// 11025 Hz, 4096-sample frames with 2/3 overlap → 1365.33 samples).
const ItemSec = 4096.0 / 3.0 / 11025.0

// Fingerprinter extracts raw Chromaprint fingerprints with ffmpeg's chromaprint
// muxer when available, otherwise with fpcalc (libchromaprint-tools).
type Fingerprinter struct {
	FFmpeg string
	Fpcalc string
	Mode   string // "ffmpeg" | "fpcalc" | "" (unavailable)
	Nice   bool   // run at low CPU priority (nice -n 19) on Unix
}

// Detect checks which fingerprint backend works on this machine.
func Detect(ffmpeg string) *Fingerprinter {
	f := &Fingerprinter{FFmpeg: ffmpeg}
	if _, err := exec.LookPath("nice"); err == nil && runtime.GOOS != "windows" {
		f.Nice = true
	}
	if out, err := exec.Command(ffmpeg, "-hide_banner", "-muxers").Output(); err == nil && bytes.Contains(out, []byte("chromaprint")) {
		f.Mode = "ffmpeg"
		return f
	}
	if p, err := exec.LookPath("fpcalc"); err == nil {
		f.Fpcalc, f.Mode = p, "fpcalc"
	}
	return f
}

// Available reports whether fingerprints can be made.
func (f *Fingerprinter) Available() bool { return f != nil && f.Mode != "" }

// Backend is a human readable description for the admin page.
func (f *Fingerprinter) Backend() string {
	switch {
	case f == nil || f.Mode == "":
		return "nem elérhető (sem az ffmpeg chromaprint muxere, sem az fpcalc nem található)"
	case f.Mode == "ffmpeg":
		return "ffmpeg chromaprint muxer"
	}
	return "fpcalc (" + f.Fpcalc + ")"
}

func (f *Fingerprinter) command(ctx context.Context, name string, args ...string) *exec.Cmd {
	if f.Nice {
		return exec.CommandContext(ctx, "nice", append([]string{"-n", "19", name}, args...)...)
	}
	return exec.CommandContext(ctx, name, args...)
}

var errNoBackend = errors.New("nincs ujjlenyomat-készítő (ffmpeg chromaprint / fpcalc)")

// Range fingerprints dur seconds of audio starting at start.
func (f *Fingerprinter) Range(ctx context.Context, path string, start, dur float64) ([]uint32, error) {
	if !f.Available() {
		return nil, errNoBackend
	}
	ss, t := strconv.FormatFloat(start, 'f', 3, 64), strconv.FormatFloat(dur, 'f', 3, 64)
	if f.Mode == "ffmpeg" {
		var stderr bytes.Buffer
		cmd := f.command(ctx, f.FFmpeg, "-hide_banner", "-loglevel", "error", "-nostdin", "-ss", ss, "-t", t, "-i", path,
			"-vn", "-sn", "-dn", "-map", "0:a:0", "-ac", "1", "-f", "chromaprint", "-fp_format", "raw", "-")
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("ffmpeg chromaprint: %v: %s", err, lastLine(stderr.String()))
		}
		fp := make([]uint32, len(out)/4)
		for i := range fp {
			fp[i] = binary.LittleEndian.Uint32(out[i*4:])
		}
		return fp, nil
	}
	// fpcalc only reads from the start of a file: cut the range to a temporary WAV first.
	tmp, err := os.CreateTemp("", "kinora-fp-*.wav")
	if err != nil {
		return nil, err
	}
	tmp.Close()
	defer os.Remove(tmp.Name())
	if out, err := f.command(ctx, f.FFmpeg, "-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-ss", ss, "-t", t, "-i", path,
		"-vn", "-map", "0:a:0", "-ac", "1", "-ar", "11025", tmp.Name()).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %v: %s", err, lastLine(string(out)))
	}
	out, err := f.command(ctx, f.Fpcalc, "-raw", "-json", "-length", strconv.Itoa(int(dur)+1), tmp.Name()).Output()
	if err != nil {
		return nil, fmt.Errorf("fpcalc: %v", err)
	}
	var j struct {
		Fingerprint []uint32 `json:"fingerprint"`
	}
	if err := json.Unmarshal(out, &j); err != nil {
		return nil, fmt.Errorf("fpcalc kimenet: %v", err)
	}
	return j.Fingerprint, nil
}

// BlackRanges runs ffmpeg blackdetect on a part of the video and returns the black
// intervals in file time.
func (f *Fingerprinter) BlackRanges(ctx context.Context, path string, start, dur float64) ([][2]float64, error) {
	var stderr bytes.Buffer
	cmd := f.command(ctx, f.FFmpeg, "-hide_banner", "-nostdin", "-ss", strconv.FormatFloat(start, 'f', 3, 64), "-t", strconv.FormatFloat(dur, 'f', 3, 64),
		"-i", path, "-an", "-sn", "-map", "0:v:0", "-vf", "fps=4,scale=160:-2,blackdetect=d=0.4:pix_th=0.12", "-f", "null", "-")
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("blackdetect: %v: %s", err, lastLine(stderr.String()))
	}
	var out [][2]float64
	for _, l := range strings.Split(stderr.String(), "\n") {
		i := strings.Index(l, "black_start:")
		if i < 0 {
			continue
		}
		var a, b float64
		fields := strings.Fields(l[i:])
		for _, fld := range fields {
			if v, ok := strings.CutPrefix(fld, "black_start:"); ok {
				a, _ = strconv.ParseFloat(v, 64)
			}
			if v, ok := strings.CutPrefix(fld, "black_end:"); ok {
				b, _ = strconv.ParseFloat(v, 64)
			}
		}
		out = append(out, [2]float64{start + a, start + b})
	}
	return out, nil
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return s[i+1:]
	}
	return s
}

// cachePath is where a fingerprint is cached (keyed by item, file signature and range).
func cachePath(dir string, itemID int64, sig string, start, dur float64) string {
	return filepath.Join(dir, fmt.Sprintf("%d-%s-%d-%d.fp", itemID, sig, int(start*10), int(dur*10)))
}

// Cached returns a fingerprint from the cache dir, computing and storing it on a miss.
func (f *Fingerprinter) Cached(ctx context.Context, dir string, itemID int64, sig, path string, start, dur float64) ([]uint32, error) {
	p := cachePath(dir, itemID, sig, start, dur)
	if b, err := os.ReadFile(p); err == nil && len(b)%4 == 0 && len(b) > 0 {
		fp := make([]uint32, len(b)/4)
		for i := range fp {
			fp[i] = binary.LittleEndian.Uint32(b[i*4:])
		}
		return fp, nil
	}
	fp, err := f.Range(ctx, path, start, dur)
	if err != nil {
		return nil, err
	}
	b := make([]byte, len(fp)*4)
	for i, v := range fp {
		binary.LittleEndian.PutUint32(b[i*4:], v)
	}
	if os.MkdirAll(dir, 0o755) == nil {
		os.WriteFile(p, b, 0o644)
	}
	return fp, nil
}
