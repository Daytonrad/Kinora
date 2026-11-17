package server

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ScotyLucas/kinora/internal/intro"
	"github.com/ScotyLucas/kinora/internal/store"
)

// introShow writes a 3-episode season where the same 30 s theme starts at 0, 45 and 90 s.
func introShow(t *testing.T, root string) {
	t.Helper()
	dir := filepath.Join(root, "Intro Show (2021)", "Season 01")
	os.MkdirAll(dir, 0o755)
	theme := "aevalsrc='0.4*sin(2*PI*(220+110*mod(floor(t*4),7))*t)+0.3*sin(2*PI*(330+55*mod(floor(t*3),5))*t)+0.2*sin(2*PI*(660+40*mod(floor(t*6),9))*t)':s=22050:d=30"
	uniq := func(key, dur int) string {
		k1, k2 := 0.6180339887*float64(key)+1.437, 0.4142135623*float64(key)+2.391
		return fmt.Sprintf("aevalsrc='0.35*sin(2*PI*220*pow(2,floor(12*mod(floor(t*4)*%.6f,1))/12)*t)+0.25*sin(2*PI*330*pow(2,floor(12*mod(floor(t*3)*%.6f+0.5,1))/12)*t)':s=22050:d=%d", k1, k2, dur)
	}
	for n, pre := range []int{0, 45, 90} {
		ep := n + 1
		var args []string
		k := 0
		if pre > 0 {
			args = append(args, "-f", "lavfi", "-i", uniq(ep*3+1, pre))
			k++
		}
		// long enough tail: the intro window is capped at 45 % of the runtime
		args = append(args, "-f", "lavfi", "-i", theme, "-f", "lavfi", "-i", uniq(ep*3+3, 200))
		k += 2
		dur := pre + 230
		filter := ""
		for i := 0; i < k; i++ {
			filter += fmt.Sprintf("[%d]", i)
		}
		filter += fmt.Sprintf("concat=n=%d:v=0:a=1[a]", k)
		out := filepath.Join(dir, fmt.Sprintf("Intro.Show.S01E%02d.mkv", ep))
		args = append([]string{"-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", fmt.Sprintf("color=c=0x203040:s=160x90:r=5:d=%d", dur)}, args...)
		// the video input is 0, audio inputs shift by one
		f := ""
		for i := 1; i <= k; i++ {
			f += fmt.Sprintf("[%d]", i)
		}
		f += fmt.Sprintf("concat=n=%d:v=0:a=1[a]", k)
		args = append(args, "-filter_complex", f, "-map", "0:v", "-map", "[a]", "-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac", "-shortest", out)
		_ = filter
		if b, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
			t.Fatalf("ffmpeg: %v\n%s", err, b)
		}
	}
}

func TestSkipIntroEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg missing")
	}
	c, st, srv := newTestServerFull(t)
	fp := intro.Detect("ffmpeg")
	if !fp.Available() {
		t.Skip("no chromaprint backend")
	}
	root := t.TempDir()
	introShow(t, root)
	lib, _ := st.CreateLibrary("Intro", root, "shows")
	if _, err := srv.Scanner.ScanNow(context.Background(), lib.ID); err != nil {
		t.Fatal(err)
	}
	an := intro.NewAnalyzer(st, fp, t.TempDir(), srv.log)
	srv.Intro = an
	seasons, err := st.SeasonsNeedingAnalysis(10)
	if err != nil || len(seasons) == 0 {
		t.Fatalf("seasons needing analysis: %v %v", seasons, err)
	}
	for _, s := range seasons {
		if err := an.AnalyzeSeason(context.Background(), s, false); err != nil {
			t.Fatalf("analyze: %v", err)
		}
	}
	if left, _ := st.SeasonsNeedingAnalysis(10); len(left) != 0 {
		t.Fatalf("still unanalysed: %v", left)
	}
	selectFirstProfile(t, c)
	caps := map[string]any{"video": []string{"h264"}, "audio": []string{"aac"}, "containers": []string{"mkv", "mp4"}}
	want := map[string]float64{"Episode 1": 0, "Episode 2": 45, "Episode 3": 90}
	var ep2 int64
	for n, start := range want {
		var id int64
		st.DB.QueryRow(`SELECT i.id FROM items i JOIN items s ON s.id=i.series_id WHERE s.title='Intro Show' AND i.episode_num=?`, int(start/45)+1).Scan(&id)
		if start == 45 {
			ep2 = id
		}
		var pr struct {
			Segments []store.Segment `json:"segments"`
		}
		c.do("POST", "/api/items/"+itoa(id)+"/playback", map[string]any{"caps": caps}, &pr)
		var intro *store.Segment
		for i := range pr.Segments {
			if pr.Segments[i].Type == "intro" {
				intro = &pr.Segments[i]
			}
		}
		if intro == nil {
			t.Fatalf("%s: no intro segment in playback response: %+v", n, pr.Segments)
		}
		t.Logf("%s: intro %.1f–%.1f (%s, %.2f)", n, intro.Start, intro.End, intro.Source, intro.Confidence)
		if math.Abs(intro.Start-start) > 1 || math.Abs(intro.End-(start+30)) > 1 || intro.Source != "auto" {
			t.Errorf("%s: intro %.1f–%.1f, want %.0f–%.0f ±1 s (auto)", n, intro.Start, intro.End, start, start+30)
		}
	}

	// a manual mark wins over detection, and can be applied to the rest of the season
	if code := c.do("PUT", "/api/items/"+itoa(ep2)+"/segments/intro", map[string]float64{"start": 45, "end": 75}, nil); code != 200 {
		t.Fatalf("manual mark %d", code)
	}
	var segs struct {
		Segments []store.Segment `json:"segments"`
	}
	c.do("GET", "/api/items/"+itoa(ep2)+"/segments", nil, &segs)
	if segs.Segments[0].Source != "manual" {
		t.Fatalf("manual must win: %+v", segs.Segments)
	}
	var prop struct {
		Updated int `json:"updated"`
	}
	if code := c.do("POST", "/api/items/"+itoa(ep2)+"/segments/intro/propagate", nil, &prop); code != 200 || prop.Updated != 2 {
		t.Fatalf("propagate %d %+v", code, prop)
	}
	var ep3 int64
	st.DB.QueryRow(`SELECT i.id FROM items i JOIN items s ON s.id=i.series_id WHERE s.title='Intro Show' AND i.episode_num=3`).Scan(&ep3)
	manualOf := func(id int64) *store.Segment {
		stored, _ := st.StoredSegments(id)
		for i := range stored {
			if stored[i].Source == "manual" {
				return &stored[i]
			}
		}
		return nil
	}
	if m := manualOf(ep3); m == nil || math.Abs(m.Start-90) > 1 {
		t.Fatalf("propagated mark on E03: %+v", m)
	}
	// re-detection never touches a manual mark
	an.AnalyzeSeason(context.Background(), seasons[0], true)
	if manualOf(ep3) == nil {
		t.Fatal("detection removed a manual mark")
	}
	// only admins may mark; kids profiles see segments of visible items only
	if code := c.do("DELETE", "/api/items/"+itoa(ep2)+"/segments/intro", nil, nil); code != 200 {
		t.Fatalf("delete manual %d", code)
	}
	c.do("GET", "/api/items/"+itoa(ep2)+"/segments", nil, &segs)
	if len(segs.Segments) == 0 || segs.Segments[0].Source != "auto" {
		t.Fatalf("removing the mark must fall back to detection: %+v", segs.Segments)
	}
	c.do("POST", "/api/admin/users", map[string]any{"username": "bob", "password": "secret456"}, nil)
	bob := appClient(c, "", "null")
	var bl loginResp
	_, b := bob.raw("POST", "/api/login", map[string]string{"username": "bob", "password": "secret456"})
	jsonUnmarshal(b, &bl)
	bob = appClient(c, bl.Token, "null")
	selectFirstProfile(t, bob)
	if code := bob.do("PUT", "/api/items/"+itoa(ep2)+"/segments/intro", map[string]float64{"start": 1, "end": 20}, nil); code != 403 {
		t.Fatalf("non-admin mark %d", code)
	}
	var st2 struct {
		Available bool `json:"available"`
		Stats     struct {
			WithIntro int `json:"with_intro"`
		} `json:"stats"`
	}
	c.do("GET", "/api/admin/segments", nil, &st2)
	if !st2.Available || st2.Stats.WithIntro < 3 {
		t.Fatalf("admin status %+v", st2)
	}
}

func TestChapterSegmentsPriority(t *testing.T) {
	chs := []store.ChapterRow{{Start: 0, Title: "Előzmények"}, {Start: 40, Title: "Főcím"}, {Start: 95, Title: "Chapter 3"}, {Start: 1300, Title: "Stáblista"}}
	segs := store.EffectiveSegments([]store.Segment{{Type: "intro", Start: 41, End: 94, Source: "auto"}}, chs, 1400)
	got := map[string]store.Segment{}
	for _, g := range segs {
		got[g.Type] = g
	}
	if got["recap"].End != 40 || got["intro"].Source != "chapter" || got["intro"].Start != 40 || got["credits"].Start != 1300 || got["credits"].End != 1400 {
		t.Fatalf("chapter segments: %+v", segs)
	}
	segs = store.EffectiveSegments([]store.Segment{{Type: "intro", Start: 42, End: 90, Source: "manual"}}, chs, 1400)
	for _, g := range segs {
		if g.Type == "intro" && g.Source != "manual" {
			t.Fatalf("manual must beat chapters: %+v", g)
		}
	}
}
