package intro

import (
	"context"
	"fmt"
	"math"
	"os/exec"
	"path/filepath"
	"testing"
)

// melody is episode-unique audio (stands in for dialogue and score): two voices whose
// pitch classes follow golden-ratio (Weyl) sequences seeded per episode, so the note
// order never repeats and two episodes do not share a stretch. Plain noise would not
// do: its flat chroma gives near-identical fingerprints for any seed.
func melody(key, dur int) string {
	k1 := 0.6180339887*float64(key) + 0.137
	k2 := 0.4142135623*float64(key) + 0.291
	return fmt.Sprintf("aevalsrc='0.35*sin(2*PI*220*pow(2,floor(12*mod(floor(t*4)*%.6f,1))/12)*t)+0.25*sin(2*PI*330*pow(2,floor(12*mod(floor(t*3)*%.6f+0.5,1))/12)*t)':s=22050:d=%d",
		k1+1.3, k2+2.1, dur)
}

const theme = "aevalsrc='0.4*sin(2*PI*(220+110*mod(floor(t*4),7))*t)+0.3*sin(2*PI*(330+55*mod(floor(t*3),5))*t)+0.2*sin(2*PI*(660+40*mod(floor(t*6),9))*t)':s=22050:d=30"

// episode writes an audio-only episode: [unique pre] + [theme 30 s] + [unique 60 s].
func episode(t *testing.T, dir string, n, pre int, withTheme bool) string {
	t.Helper()
	out := filepath.Join(dir, fmt.Sprintf("ep%d.mka", n))
	var args []string
	inputs := 0
	if pre > 0 {
		args = append(args, "-f", "lavfi", "-i", melody(n*3+1, pre))
		inputs++
	}
	if withTheme {
		args = append(args, "-f", "lavfi", "-i", theme)
	} else {
		args = append(args, "-f", "lavfi", "-i", melody(n*3+2, 30))
	}
	inputs++
	args = append(args, "-f", "lavfi", "-i", melody(n*3+3, 60))
	inputs++
	filter := ""
	for i := 0; i < inputs; i++ {
		filter += fmt.Sprintf("[%d]", i)
	}
	filter += fmt.Sprintf("concat=n=%d:v=0:a=1", inputs)
	args = append([]string{"-hide_banner", "-loglevel", "error", "-y"}, args...)
	args = append(args, "-filter_complex", filter, "-c:a", "flac", out)
	if b, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v\n%s", err, b)
	}
	return out
}

func fingerprinter(t *testing.T) *Fingerprinter {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg missing")
	}
	f := Detect("ffmpeg")
	if !f.Available() {
		t.Skip("no chromaprint backend")
	}
	return f
}

func TestDetectsIntroAtDifferentOffsets(t *testing.T) {
	f := fingerprinter(t)
	dir := t.TempDir()
	pres := []int{0, 45, 90}
	var eps []Episode
	for i, pre := range pres {
		p := episode(t, dir, i+1, pre, true)
		fp, err := f.Range(context.Background(), p, 0, 180)
		if err != nil {
			t.Fatal(err)
		}
		eps = append(eps, Episode{ID: int64(i + 1), FP: fp})
	}
	for i := range eps {
		for j := i + 1; j < len(eps); j++ {
			m, ok := Longest(eps[i].FP, eps[j].FP, MaxBitErrors, MaxGapItems, minItems(15))
			t.Logf("pair %d-%d: ok=%v a=%.2f–%.2f b=%.2f–%.2f density=%.2f", i+1, j+1, ok,
				float64(m.A0)*ItemSec, float64(m.A1)*ItemSec, float64(m.B0)*ItemSec, float64(m.B1)*ItemSec, float64(m.Hits)/float64(max(1, m.Len())))
		}
	}
	found := Common(eps, 15, 150, 3)
	for i, pre := range pres {
		g, ok := found[int64(i+1)]
		if !ok {
			t.Fatalf("episode %d: intro not found (%v)", i+1, found)
		}
		t.Logf("ep%d: %.2f–%.2f (want %d–%d), confidence %.2f", i+1, g.Start, g.End, pre, pre+30, g.Confidence)
		if math.Abs(g.Start-float64(pre)) > 1 || math.Abs(g.End-float64(pre+30)) > 1 {
			t.Errorf("episode %d: got %.2f–%.2f, want %d–%d ±1 s", i+1, g.Start, g.End, pre, pre+30)
		}
		if g.Confidence < 0.99 {
			t.Errorf("episode %d: confidence %.2f", i+1, g.Confidence)
		}
	}
}

func TestNoCommonIntroFindsNothing(t *testing.T) {
	f := fingerprinter(t)
	dir := t.TempDir()
	var eps []Episode
	for i, pre := range []int{20, 45, 10} {
		p := episode(t, dir, i+1, pre, false)
		fp, err := f.Range(context.Background(), p, 0, 180)
		if err != nil {
			t.Fatal(err)
		}
		eps = append(eps, Episode{ID: int64(i + 1), FP: fp})
	}
	if found := Common(eps, 15, 150, 3); len(found) != 0 {
		t.Fatalf("found a shared intro where there is none: %+v", found)
	}
}

func TestLocateSnippet(t *testing.T) {
	f := fingerprinter(t)
	dir := t.TempDir()
	a := episode(t, dir, 1, 10, true)
	b := episode(t, dir, 2, 70, true)
	full, err := f.Range(context.Background(), a, 0, 160)
	if err != nil {
		t.Fatal(err)
	}
	snippet := Snippet(full, 0, 10, 40) // the hand-marked intro of episode 1
	fp, err := f.Range(context.Background(), b, 0, 160)
	if err != nil {
		t.Fatal(err)
	}
	at, cov, ok := Locate(snippet, Episode{ID: 2, FP: fp})
	if !ok || math.Abs(at-70) > 1 || cov < 0.8 {
		t.Fatalf("locate: at %.2f coverage %.2f ok %v (want 70)", at, cov, ok)
	}
}

func TestLongestSynthetic(t *testing.T) {
	a := make([]uint32, 300)
	b := make([]uint32, 300)
	for i := range a {
		a[i] = uint32(i*2654435761) ^ 0x55aa55aa
		b[i] = uint32(i*40503) ^ 0x12345678
	}
	copy(b[150:250], a[40:140]) // shared stretch: a[40:140] == b[150:250]
	b[200] ^= 0xffff0000          // one corrupted item inside is tolerated
	m, ok := Longest(a, b, MaxBitErrors, MaxGapItems, 50)
	if !ok || m.A0 != 40 || m.A1 != 140 || m.B0 != 150 || m.B1 != 250 {
		t.Fatalf("match %+v ok %v", m, ok)
	}
}

func minItems(sec float64) int { return int(sec / ItemSec) }
