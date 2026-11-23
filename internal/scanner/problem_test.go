package scanner

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/ScotyLucas/kinora/internal/store"
)

// Scans the library of scripts/gen-testmedia.sh: broken files must not stop the scan or hide the good ones,
// and odd names must come out as readable titles.
func TestScanProblemMedia(t *testing.T) {
	root, _ := filepath.Abs("../../testmedia-problem")
	if _, err := os.Stat(filepath.Join(root, ".complete")); err != nil {
		t.Skip("problem media missing – run scripts/gen-testmedia.sh")
	}
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "k.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	sc := New(st, filepath.Join(dir, "images"), slog.New(slog.NewTextHandler(io.Discard, nil)))
	lib, _ := st.CreateLibrary("Problémás", filepath.Join(root, "Movies"), "movies")

	s, err := sc.ScanNow(context.Background(), lib.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("first scan: %+v", s)
	for _, want := range []struct {
		title string
		year  int
	}{
		{"HDR10", 2020}, {"Many Audio Tracks", 2021}, {"Image Subtitles", 2019}, {"Text Subtitles", 2018},
		{"Variable Frame Rate", 2017}, {"Ünnepi Műsor – 日本語", 2020},
		{"A Film With An Extraordinarily Long Title That Keeps Going Well Past What Any Sensible Person Would Type Into A File Name", 2013},
	} {
		var year int
		if err := st.DB.QueryRow(`SELECT year FROM items WHERE kind='movie' AND title=? AND missing=0`, want.title).Scan(&year); err != nil || year != want.year {
			t.Errorf("%q: year %d, err %v", want.title, year, err)
		}
	}
	var pgs int
	st.DB.QueryRow(`SELECT COUNT(*) FROM media_streams s JOIN items i ON i.id=s.item_id WHERE i.title='Image Subtitles' AND s.codec='hdmv_pgs_subtitle'`).Scan(&pgs)
	if pgs != 1 {
		t.Errorf("PGS stream stored: %d", pgs)
	}
	var hdr string
	st.DB.QueryRow(`SELECT hdr FROM items WHERE title='HDR10'`).Scan(&hdr)
	if hdr != "HDR10" {
		t.Errorf("hdr %q", hdr)
	}
	// the empty file cannot be probed: it is an error of that file only
	if s.Errors == 0 {
		t.Error("the empty file should be reported as a scan error")
	}

	// nothing changed: the second scan touches nothing
	s2, err := sc.ScanNow(context.Background(), lib.ID)
	if err != nil {
		t.Fatal(err)
	}
	if s2.Changed != 0 {
		t.Errorf("second scan changed %d files: %+v", s2.Changed, s2)
	}
}
