package media

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Encoder is an H.264 encoder with the filter chain it needs.
type Encoder struct {
	Name     string `json:"name"`
	Label    string `json:"label"`
	Hardware bool   `json:"hardware"`
}

// EncoderResult is the outcome of a real 1 s test encode.
type EncoderResult struct {
	Encoder
	OK    bool   `json:"ok"`
	Ms    int64  `json:"ms"`
	Error string `json:"error,omitempty"`
}

// VAAPIDevice is the render node used by h264_vaapi (KINORA_VAAPI_DEVICE).
var VAAPIDevice = "/dev/dri/renderD128"

// AllEncoders in preference order (hardware first).
var AllEncoders = []Encoder{
	{"h264_nvenc", "NVIDIA NVENC", true},
	{"h264_qsv", "Intel Quick Sync (QSV)", true},
	{"h264_vaapi", "VA-API (Intel/AMD)", true},
	{"h264_videotoolbox", "Apple VideoToolbox", true},
	{"h264_v4l2m2m", "V4L2 M2M (Raspberry Pi / ARM)", true},
	{"libx264", "Szoftveres (libx264, veryfast)", false},
}

// EncodeOpts parameterizes a video encode.
type EncodeOpts struct {
	Bitrate int    // bps
	Width   int    // 0 = keep
	ToneMap bool   // HDR → SDR
	Overlay string // "[0:v][0:s:N]overlay" input label chain for image subtitle burn-in; "" = none
	GOP     bool   // constant 4 s GOP for HLS
}

// InitArgs are global args placed before -i (hardware device setup).
func (e Encoder) InitArgs() []string {
	switch e.Name {
	case "h264_qsv":
		return []string{"-init_hw_device", "qsv=hw", "-filter_hw_device", "hw"}
	case "h264_vaapi":
		return []string{"-init_hw_device", "vaapi=va:" + VAAPIDevice, "-filter_hw_device", "va"}
	}
	return nil
}

// Filter builds the -vf / -filter_complex chain (CPU filters first, then upload).
func (e Encoder) Filter(o EncodeOpts) string {
	var f []string
	if o.ToneMap {
		f = append(f, "zscale=t=linear:npl=100", "format=gbrpf32le", "zscale=p=bt709",
			"tonemap=tonemap=hable:desat=0", "zscale=t=bt709:m=bt709:r=tv")
	}
	if o.Width > 0 {
		f = append(f, fmt.Sprintf("scale=w='min(%d,iw)':h=-2", o.Width))
	}
	switch e.Name {
	case "h264_qsv":
		f = append(f, "format=nv12", "hwupload=extra_hw_frames=64")
	case "h264_vaapi":
		f = append(f, "format=nv12", "hwupload")
	default:
		f = append(f, "format=yuv420p")
	}
	return strings.Join(f, ",")
}

// Args are the codec arguments (after the filter).
func (e Encoder) Args(o EncodeOpts) []string {
	br := o.Bitrate
	if br <= 0 {
		br = 4_000_000
	}
	rate := []string{"-b:v", fmt.Sprint(br), "-maxrate", fmt.Sprint(br), "-bufsize", fmt.Sprint(br * 2)}
	var a []string
	switch e.Name {
	case "libx264":
		a = []string{"-c:v", "libx264", "-preset", "veryfast", "-profile:v", "high", "-level", "4.1"}
		if o.GOP {
			a = append(a, "-sc_threshold", "0")
		}
	case "h264_nvenc":
		a = []string{"-c:v", "h264_nvenc", "-preset", "p4", "-profile:v", "high"}
	case "h264_qsv":
		a = []string{"-c:v", "h264_qsv", "-preset", "veryfast", "-profile:v", "high"}
	case "h264_vaapi":
		a = []string{"-c:v", "h264_vaapi", "-profile:v", "high"}
	case "h264_videotoolbox":
		a = []string{"-c:v", "h264_videotoolbox", "-profile:v", "high"}
	case "h264_v4l2m2m":
		a = []string{"-c:v", "h264_v4l2m2m"}
	}
	a = append(a, rate...)
	if o.GOP {
		a = append(a, "-g", "48", "-keyint_min", "48", "-force_key_frames", "expr:gte(t,n_forced*4)")
	}
	return a
}

// Encoders holds self-test results and picks the encoder to use.
type Encoders struct {
	mu       sync.RWMutex
	results  []EncoderResult
	testedAt time.Time
	toneMap  bool
}

// NewEncoders creates an empty registry (call SelfTest).
func NewEncoders() *Encoders { return &Encoders{} }

// Results returns the last self-test results.
func (e *Encoders) Results() ([]EncoderResult, time.Time) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return append([]EncoderResult(nil), e.results...), e.testedAt
}

// ToneMap reports whether ffmpeg has zscale+tonemap.
func (e *Encoders) ToneMap() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.toneMap
}

// SelfTest encodes 1 s of testsrc2 with every encoder in parallel. Encoders that
// ffmpeg doesn't list are reported as unavailable without running.
func (e *Encoders) SelfTest(ctx context.Context) []EncoderResult {
	listed := ffmpegList(ctx, "-encoders")
	filters := ffmpegList(ctx, "-filters")
	results := make([]EncoderResult, len(AllEncoders))
	var wg sync.WaitGroup
	for i, enc := range AllEncoders {
		results[i] = EncoderResult{Encoder: enc}
		if !strings.Contains(listed, " "+enc.Name+" ") {
			results[i].Error = "az ffmpeg nem tartalmazza ezt az encodert"
			continue
		}
		if enc.Name == "h264_vaapi" {
			if _, err := os.Stat(VAAPIDevice); err != nil {
				results[i].Error = VAAPIDevice + " nem elérhető (Dockerben: devices: /dev/dri)"
				continue
			}
		}
		wg.Add(1)
		go func(i int, enc Encoder) {
			defer wg.Done()
			results[i] = testEncoder(ctx, enc)
		}(i, enc)
	}
	wg.Wait()
	e.mu.Lock()
	e.results = results
	e.testedAt = time.Now()
	e.toneMap = strings.Contains(filters, " zscale ") && strings.Contains(filters, " tonemap ")
	e.mu.Unlock()
	return results
}

func ffmpegList(ctx context.Context, what string) string {
	out, _ := exec.CommandContext(ctx, FFmpeg, "-hide_banner", what).Output()
	return string(out)
}

func testEncoder(ctx context.Context, enc Encoder) EncoderResult {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	opts := EncodeOpts{Bitrate: 2_000_000, Width: 1280, GOP: true}
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin"}
	args = append(args, enc.InitArgs()...)
	args = append(args, "-f", "lavfi", "-i", "testsrc2=size=1280x720:rate=24", "-t", "1", "-vf", enc.Filter(opts))
	args = append(args, enc.Args(opts)...)
	args = append(args, "-f", "null", "-")
	t := time.Now()
	out, err := exec.CommandContext(ctx, FFmpeg, args...).CombinedOutput()
	r := EncoderResult{Encoder: enc, Ms: time.Since(t).Milliseconds()}
	if err != nil {
		msg := lastLine(string(out))
		if ctx.Err() != nil {
			msg = "időtúllépés (20 mp)"
		}
		r.Error = firstNonEmpty(msg, err.Error())
		return r
	}
	r.OK = true
	return r
}

// Pick returns the encoder to use: configured one if it works, otherwise the
// first working hardware encoder, otherwise libx264.
func (e *Encoders) Pick(configured string) Encoder {
	e.mu.RLock()
	defer e.mu.RUnlock()
	okNames := map[string]bool{}
	for _, r := range e.results {
		if r.OK {
			okNames[r.Name] = true
		}
	}
	byName := func(n string) (Encoder, bool) {
		for _, enc := range AllEncoders {
			if enc.Name == n {
				return enc, true
			}
		}
		return Encoder{}, false
	}
	if configured != "" && configured != "auto" && okNames[configured] {
		enc, _ := byName(configured)
		return enc
	}
	hw := []Encoder{}
	for _, enc := range AllEncoders {
		if enc.Hardware && okNames[enc.Name] {
			hw = append(hw, enc)
		}
	}
	if len(hw) > 0 {
		return hw[0]
	}
	enc, _ := byName("libx264")
	return enc
}
