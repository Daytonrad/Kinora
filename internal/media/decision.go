package media

import (
	"fmt"
	"strings"

	"github.com/ScotyLucas/kinora/internal/store"
)

// Caps is what the browser reported it can decode (MediaSource.isTypeSupported / canPlayType).
type Caps struct {
	Video      []string `json:"video"`      // h264, hevc, vp9, av1
	Audio      []string `json:"audio"`      // aac, mp3, opus, ac3, eac3, flac
	Containers []string `json:"containers"` // mp4, webm, mkv, mov
	HDR        bool     `json:"hdr"`
}

func has(list []string, v string) bool {
	for _, x := range list {
		if strings.EqualFold(x, v) {
			return true
		}
	}
	return false
}

// PlaybackRequest is the client's playback request.
type PlaybackRequest struct {
	Caps        Caps    `json:"caps"`
	AudioIdx    *int    `json:"audio_idx"`    // audio type index
	BurnSubIdx  *int    `json:"burn_sub_idx"` // subtitle type index to burn in (image based)
	Start       float64 `json:"start"`
	MaxBitrate  int     `json:"max_bitrate"` // kbps, 0 = unlimited
	Force       string  `json:"force"`       // "" | "transcode" | "remux"
	PrevSession string  `json:"prev_session"`
}

// Decision is the server's answer, with human readable (Hungarian) reasons.
type Decision struct {
	Mode       string   `json:"mode"` // direct | remux | transcode
	Reasons    []string `json:"reasons"`
	AudioIdx   int      `json:"audio_idx"` // chosen audio type index (-1 = none)
	VideoCopy  bool     `json:"-"`
	AudioCopy  bool     `json:"-"`
	Bitrate    int      `json:"bitrate"` // target video bitrate (bps) when transcoding
	Width      int      `json:"width"`
	ToneMap    bool     `json:"tonemap"`
	BurnSubIdx *int     `json:"burn_sub_idx,omitempty"`
}

// codecName maps ffprobe codec names to the names clients report.
func codecName(c string) string {
	switch c {
	case "h265":
		return "hevc"
	case "vp09":
		return "vp9"
	case "av01":
		return "av1"
	}
	return c
}

// ChooseAudio picks the audio track: explicit request > profile language
// priority > default flag > first track.
func ChooseAudio(streams []store.StreamInfo, requested *int, langs []string) int {
	var audio []store.StreamInfo
	for _, s := range streams {
		if s.Type == "audio" {
			audio = append(audio, s)
		}
	}
	if len(audio) == 0 {
		return -1
	}
	if requested != nil {
		for _, a := range audio {
			if a.TypeIndex == *requested {
				return a.TypeIndex
			}
		}
	}
	for _, l := range langs {
		for _, a := range audio {
			if l != "" && strings.EqualFold(a.Lang, l) {
				return a.TypeIndex
			}
		}
	}
	for _, a := range audio {
		if a.Default {
			return a.TypeIndex
		}
	}
	return audio[0].TypeIndex
}

// TargetWidth derives the output width from the video bitrate (bps).
func TargetWidth(bitrate int) int {
	switch {
	case bitrate >= 12_000_000:
		return 3840
	case bitrate >= 5_000_000:
		return 1920
	case bitrate >= 2_500_000:
		return 1280
	case bitrate >= 1_000_000:
		return 854
	}
	return 640
}

// Decide chooses direct play, remux or transcode for an item.
// maxKbps is the effective limit (smaller of profile and client), 0 = none.
func Decide(it *store.Item, req PlaybackRequest, audioLangs []string, maxKbps int, toneMapAvailable bool) Decision {
	d := Decision{AudioIdx: ChooseAudio(it.Streams, req.AudioIdx, audioLangs)}
	vc := codecName(it.VideoCodec)
	audioCodec := ""
	firstAudio := -1
	for _, s := range it.Streams {
		if s.Type == "audio" {
			if firstAudio < 0 {
				firstAudio = s.TypeIndex
			}
			if s.TypeIndex == d.AudioIdx {
				audioCodec = s.Codec
			}
		}
	}

	var why []string
	containerOK := has(req.Caps.Containers, it.Container)
	videoOK := vc != "" && has(req.Caps.Video, vc)
	audioOK := audioCodec == "" || has(req.Caps.Audio, audioCodec)
	overBitrate := maxKbps > 0 && it.Bitrate > maxKbps*1000
	hdrBlocked := it.HDR != "" && !req.Caps.HDR

	if !containerOK {
		why = append(why, fmt.Sprintf("a böngésző nem tudja lejátszani a(z) %s konténert", it.Container))
	}
	if !videoOK {
		why = append(why, fmt.Sprintf("a böngésző nem tudja lejátszani a(z) %s videót", vc))
	}
	if !audioOK {
		why = append(why, fmt.Sprintf("a(z) %s hang nem támogatott", audioCodec))
	}
	if d.AudioIdx >= 0 && d.AudioIdx != firstAudio {
		why = append(why, fmt.Sprintf("nem az első hangsáv lett kiválasztva (%d. hangsáv)", d.AudioIdx+1))
	}
	if overBitrate {
		why = append(why, fmt.Sprintf("a forrás bitrátája (%.1f Mbps) meghaladja a korlátot (%.1f Mbps)", float64(it.Bitrate)/1e6, float64(maxKbps)/1000))
	}
	if req.BurnSubIdx != nil {
		codec := ""
		for _, s := range it.Streams {
			if s.Type == "subtitle" && s.TypeIndex == *req.BurnSubIdx {
				codec = s.Codec
			}
		}
		why = append(why, fmt.Sprintf("képalapú felirat beégetése (%s)", firstNonEmpty(codec, "felirat")))
		d.BurnSubIdx = req.BurnSubIdx
	}
	if hdrBlocked {
		why = append(why, fmt.Sprintf("a kliens nem jelzett %s-támogatást", it.HDR))
	}
	if req.Force != "" {
		why = append(why, "kézi minőség- vagy módválasztás")
	}

	direct := containerOK && videoOK && audioOK && (d.AudioIdx < 0 || d.AudioIdx == firstAudio) &&
		!overBitrate && req.BurnSubIdx == nil && !hdrBlocked && req.Force == ""
	if direct {
		d.Mode = "direct"
		d.Reasons = []string{"minden kompatibilis – az eredeti fájl megy, átkódolás nélkül"}
		return d
	}

	remux := vc == "h264" && videoOK && !overBitrate && req.BurnSubIdx == nil && !hdrBlocked && req.Force != "transcode"
	d.AudioCopy = audioCodec == "aac" && has(req.Caps.Audio, "aac")
	if remux {
		d.Mode = "remux"
		d.VideoCopy = true
		if !d.AudioCopy && audioCodec != "" {
			why = append(why, fmt.Sprintf("hang átkódolása: %s → AAC", audioCodec))
		}
		d.Reasons = append(why, "a videó változatlanul megy (remux HLS-be), csak a csomagolás változik")
		return d
	}

	d.Mode = "transcode"
	d.AudioCopy = false // segments are re-timed; AAC re-encode keeps A/V sync robust
	switch {
	case maxKbps > 0:
		d.Bitrate = maxKbps * 1000
	case it.Bitrate > 0:
		d.Bitrate = it.Bitrate + 500_000
	default:
		d.Bitrate = 8_000_000
	}
	if d.Bitrate > 10_000_000 {
		d.Bitrate = 10_000_000
	}
	d.Width = TargetWidth(d.Bitrate)
	if it.Width > 0 && d.Width > it.Width {
		d.Width = it.Width
	}
	if d.Width%2 == 1 {
		d.Width--
	}
	if it.HDR != "" {
		if toneMapAvailable {
			d.ToneMap = true
			why = append(why, it.HDR+" → SDR tónusleképezés")
		} else {
			why = append(why, it.HDR+" forrás: az ffmpeg nem tud tónusleképezést (zscale hiányzik), a színek fakók lehetnek")
		}
	}
	d.Reasons = why
	return d
}
