// Package media wraps ffmpeg/ffprobe: probing, hardware encoder self-tests,
// HLS transcoding sessions, WebVTT subtitles and offline downloads.
//
// Jellyfin problems it addresses: "mysterious transcoder errors" (every
// encoder is actually exercised with a 1 s test encode, and the playback
// decision explains *why* it transcodes; ffmpeg's last stderr line is shown
// on failure), no transcoded downloads for offline viewing, and broken
// Hungarian accents in CP1250 .srt files.
package media

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Binaries can be overridden (KINORA_FFMPEG / KINORA_FFPROBE).
var (
	FFmpeg  = "ffmpeg"
	FFprobe = "ffprobe"
)

// Stream is one elementary stream.
type Stream struct {
	Index           int    `json:"index"`      // absolute ffprobe index
	Type            string `json:"type"`       // video | audio | subtitle
	TypeIndex       int    `json:"type_index"` // index among streams of the same type
	Codec           string `json:"codec"`
	Lang            string `json:"lang"`
	Title           string `json:"title"`
	Channels        int    `json:"channels,omitempty"`
	Default         bool   `json:"default"`
	Forced          bool   `json:"forced"`
	HearingImpaired bool   `json:"hearing_impaired"`
	External        string `json:"-"` // path of an external subtitle file
}

// Chapter marker.
type Chapter struct {
	Start float64 `json:"start"`
	Title string  `json:"title"`
}

// ProbeResult is the normalized technical description of a file.
type ProbeResult struct {
	Container    string
	Duration     float64
	Bitrate      int // bits/s
	VideoCodec   string
	AudioCodec   string
	Width        int
	Height       int
	HDR          string // "" | HDR10 | HLG | Dolby Vision
	Streams      []Stream
	Chapters     []Chapter
	CreditsStart *float64
}

type ffStream struct {
	Index         int               `json:"index"`
	CodecType     string            `json:"codec_type"`
	CodecName     string            `json:"codec_name"`
	Width         int               `json:"width"`
	Height        int               `json:"height"`
	Channels      int               `json:"channels"`
	ColorTransfer string            `json:"color_transfer"`
	BitRate       string            `json:"bit_rate"`
	Tags          map[string]string `json:"tags"`
	Disposition   map[string]int    `json:"disposition"`
	SideData      []map[string]any  `json:"side_data_list"`
}

type ffOutput struct {
	Format struct {
		FormatName string            `json:"format_name"`
		Duration   string            `json:"duration"`
		BitRate    string            `json:"bit_rate"`
		Tags       map[string]string `json:"tags"`
	} `json:"format"`
	Streams  []ffStream `json:"streams"`
	Chapters []struct {
		StartTime string            `json:"start_time"`
		Tags      map[string]string `json:"tags"`
	} `json:"chapters"`
}

// Probe runs ffprobe (60 s timeout) and normalizes the result.
func Probe(ctx context.Context, path string) (*ProbeResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, FFprobe, "-v", "error", "-print_format", "json",
		"-show_format", "-show_streams", "-show_chapters", path)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := lastLine(stderr.String())
		if ctx.Err() == context.DeadlineExceeded {
			msg = "időtúllépés (60 mp)"
		}
		return nil, fmt.Errorf("ffprobe: %s", firstNonEmpty(msg, err.Error()))
	}
	var ff ffOutput
	if err := json.Unmarshal(out, &ff); err != nil {
		return nil, fmt.Errorf("ffprobe kimenet: %w", err)
	}
	return normalize(&ff, path), nil
}

func normalize(ff *ffOutput, path string) *ProbeResult {
	r := &ProbeResult{}
	r.Duration, _ = strconv.ParseFloat(ff.Format.Duration, 64)
	r.Bitrate, _ = strconv.Atoi(ff.Format.BitRate)
	r.Container = normalizeContainer(ff.Format.FormatName, path)
	counts := map[string]int{}
	for _, s := range ff.Streams {
		if s.CodecType != "video" && s.CodecType != "audio" && s.CodecType != "subtitle" {
			continue
		}
		if s.CodecType == "video" && s.Disposition["attached_pic"] == 1 {
			continue // cover art
		}
		st := Stream{
			Index: s.Index, Type: s.CodecType, TypeIndex: counts[s.CodecType], Codec: s.CodecName,
			Lang: s.Tags["language"], Title: s.Tags["title"], Channels: s.Channels,
			Default: s.Disposition["default"] == 1, Forced: s.Disposition["forced"] == 1,
			HearingImpaired: s.Disposition["hearing_impaired"] == 1,
		}
		if st.Lang == "und" {
			st.Lang = ""
		}
		counts[s.CodecType]++
		r.Streams = append(r.Streams, st)
		switch s.CodecType {
		case "video":
			if r.VideoCodec == "" {
				r.VideoCodec, r.Width, r.Height = s.CodecName, s.Width, s.Height
				r.HDR = detectHDR(s)
			}
		case "audio":
			if r.AudioCodec == "" {
				r.AudioCodec = s.CodecName
			}
		}
	}
	if r.Bitrate == 0 && r.Duration > 0 {
		if st, err := statSize(path); err == nil {
			r.Bitrate = int(float64(st*8) / r.Duration)
		}
	}
	for _, c := range ff.Chapters {
		start, _ := strconv.ParseFloat(c.StartTime, 64)
		r.Chapters = append(r.Chapters, Chapter{Start: start, Title: c.Tags["title"]})
	}
	r.CreditsStart = CreditsStart(r.Chapters, r.Duration)
	return r
}

func detectHDR(s ffStream) string {
	for _, sd := range s.SideData {
		if t, _ := sd["side_data_type"].(string); strings.Contains(strings.ToLower(t), "dovi") || strings.Contains(t, "Dolby Vision") {
			return "Dolby Vision"
		}
	}
	switch s.ColorTransfer {
	case "smpte2084":
		return "HDR10"
	case "arib-std-b67":
		return "HLG"
	}
	return ""
}

func normalizeContainer(formatName, path string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	f := strings.ToLower(formatName)
	switch {
	case strings.Contains(f, "matroska") || strings.Contains(f, "webm"):
		if ext == "webm" {
			return "webm"
		}
		return "mkv"
	case strings.Contains(f, "mp4") || strings.Contains(f, "mov"):
		if ext == "mov" {
			return "mov"
		}
		return "mp4"
	case f == "mpegts":
		return "ts"
	case f == "avi":
		return "avi"
	}
	if i := strings.Index(f, ","); i > 0 {
		return f[:i]
	}
	return f
}

var reCredits = regexp.MustCompile(`(?i)credit|stáblista|stablista|end ?titles|outro|ending|abspann|générique`)

// CreditsStart finds a credits chapter in the second half of the runtime.
func CreditsStart(ch []Chapter, duration float64) *float64 {
	for _, c := range ch {
		if c.Start >= duration/2 && reCredits.MatchString(c.Title) {
			v := c.Start
			return &v
		}
	}
	return nil
}

// LastLine returns the last non-empty line (ffmpeg's real error is usually there).
func LastLine(s string) string { return lastLine(s) }

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			return l
		}
	}
	return ""
}

func firstNonEmpty(a ...string) string {
	for _, s := range a {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
