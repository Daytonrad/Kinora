package scanner

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ScotyLucas/kinora/internal/store"
)

// testMedia returns a private copy of the generated test library (scripts/mkmedia.sh).
func testMedia(t *testing.T) string {
	t.Helper()
	src, _ := filepath.Abs("../../testmedia")
	if _, err := os.Stat(filepath.Join(src, ".complete")); err != nil {
		t.Skip("testmedia missing – run scripts/mkmedia.sh testmedia")
	}
	dst := filepath.Join(t.TempDir(), "media")
	err := filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		if err == nil {
			os.Chtimes(target, info.ModTime(), info.ModTime())
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

func setup(t *testing.T) (*store.Store, *Scanner, string) {
	t.Helper()
	media := testMedia(t)
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "k.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return st, New(st, filepath.Join(dir, "images"), log), media
}

func count(t *testing.T, st *store.Store, q string, args ...any) int {
	t.Helper()
	var n int
	if err := st.DB.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestScanMoviesAndShows(t *testing.T) {
	st, sc, media := setup(t)
	ctx := context.Background()
	movies, _ := st.CreateLibrary("Filmek", filepath.Join(media, "Movies"), "movies")
	shows, _ := st.CreateLibrary("Sorozatok", filepath.Join(media, "Shows"), "shows")

	s1, err := sc.ScanNow(ctx, movies.ID)
	if err != nil {
		t.Fatal(err)
	}
	if s1.Changed != 3 || s1.Errors != 0 {
		t.Fatalf("first movie scan: %+v", s1)
	}
	if _, err := sc.ScanNow(ctx, shows.ID); err != nil {
		t.Fatal(err)
	}

	// movie metadata from NFO + probe
	var title, rating, vcodec, acodec, container string
	var year, level int
	var credits float64
	var tmdb int64
	err = st.DB.QueryRow(`SELECT title,year,content_rating,rating_level,video_codec,audio_codec,container,credits_start,tmdb_id FROM items WHERE kind='movie' AND title='Big Test Movie'`).
		Scan(&title, &year, &rating, &level, &vcodec, &acodec, &container, &credits, &tmdb)
	if err != nil {
		t.Fatal(err)
	}
	if year != 2019 || rating != "PG-13" || level != 2 || vcodec != "h264" || acodec != "aac" || container != "mp4" || credits < 49 || credits > 51 || tmdb != -1 {
		t.Fatalf("big test movie: %s %d %s %d %s %s %s %v %d", title, year, rating, level, vcodec, acodec, container, credits, tmdb)
	}
	if n := count(t, st, `SELECT COUNT(*) FROM item_genres g JOIN items i ON i.id=g.item_id WHERE i.title='Big Test Movie'`); n != 2 {
		t.Fatalf("genres split by '/': %d", n)
	}
	if n := count(t, st, `SELECT COUNT(*) FROM media_streams s JOIN items i ON i.id=s.item_id WHERE i.title='Big Test Movie' AND s.type='subtitle' AND s.lang='hun' AND s.external_path<>''`); n != 1 {
		t.Fatalf("external hun subtitle not found")
	}
	if n := count(t, st, `SELECT COUNT(*) FROM items WHERE title='Space Drama' AND rating_level=3 AND video_codec='hevc' AND audio_codec='ac3'`); n != 1 {
		t.Fatal("space drama")
	}
	if n := count(t, st, `SELECT COUNT(*) FROM items WHERE title='Kids Cartoon' AND year=2018 AND rating_level IS NULL`); n != 1 {
		t.Fatal("kids cartoon")
	}

	// shows: Test Show (2 seasons, 4 episodes, TV-PG inherited), Other Series
	if n := count(t, st, `SELECT COUNT(*) FROM items WHERE library_id=? AND kind='series'`, shows.ID); n != 2 {
		t.Fatalf("series = %d", n)
	}
	if n := count(t, st, `SELECT COUNT(*) FROM items WHERE library_id=? AND kind='season'`, shows.ID); n != 3 {
		t.Fatalf("seasons = %d", n)
	}
	if n := count(t, st, `SELECT COUNT(*) FROM items e JOIN items s ON s.id=e.series_id WHERE e.kind='episode' AND s.title='Test Show' AND e.rating_level=1`); n != 4 {
		t.Fatalf("Test Show episodes with inherited rating = %d", n)
	}
	if n := count(t, st, `SELECT COUNT(*) FROM items WHERE kind='episode' AND title='Pilot Part 2' AND season_num=1 AND episode_num=2`); n != 1 {
		t.Fatal("episode title parse")
	}
	// thumbnails generated
	var anyID int64
	st.DB.QueryRow(`SELECT id FROM items WHERE kind='episode' LIMIT 1`).Scan(&anyID)
	if _, err := os.Stat(filepath.Join(sc.imgDir, itoa(anyID), "thumb.jpg")); err != nil {
		t.Fatal("thumbnail missing", err)
	}

	// second scan: nothing changes, and it is fast
	t0 := time.Now()
	s2, err := sc.ScanNow(ctx, movies.ID)
	if err != nil {
		t.Fatal(err)
	}
	if s2.Changed != 0 || s2.Total != 0 || s2.Removed != 0 {
		t.Fatalf("rescan should be a no-op: %+v", s2)
	}
	if d := time.Since(t0); d > 2*time.Second {
		t.Fatalf("rescan too slow: %v", d)
	}

	// rename/move keeps id and watch state
	var id int64
	st.DB.QueryRow(`SELECT id FROM items WHERE title='Kids Cartoon'`).Scan(&id)
	prof, _ := st.CreateUser("u", "secret1", false)
	ps, _ := st.Profiles(prof.ID)
	st.DB.Exec(`INSERT INTO user_data(profile_id,item_id,played,play_count) VALUES(?,?,1,1)`, ps[0].ID, id)
	os.MkdirAll(filepath.Join(media, "Movies", "Kids Cartoon (2018)"), 0o755)
	if err := os.Rename(filepath.Join(media, "Movies", "Kids.Cartoon.2018.mp4"), filepath.Join(media, "Movies", "Kids Cartoon (2018)", "Kids Cartoon (2018).mp4")); err != nil {
		t.Fatal(err)
	}
	s3, err := sc.ScanNow(ctx, movies.ID)
	if err != nil {
		t.Fatal(err)
	}
	if s3.Moved != 1 || s3.Removed != 0 {
		t.Fatalf("move not detected: %+v", s3)
	}
	var id2 int64
	var missing int
	st.DB.QueryRow(`SELECT id,missing FROM items WHERE path LIKE '%Kids Cartoon (2018).mp4'`).Scan(&id2, &missing)
	if id2 != id || missing != 0 {
		t.Fatalf("moved item id %d→%d missing=%d", id, id2, missing)
	}
	if ud := st.GetUserData(ps[0].ID, id); !ud.Played {
		t.Fatal("watch state lost after move")
	}

	// deleting a file marks it missing (not deleted)
	os.Remove(filepath.Join(media, "Shows", "Test Show (2020)", "Season 02", "Test.Show.S02E01.mkv"))
	s4, _ := sc.ScanNow(ctx, shows.ID)
	if s4.Removed != 1 {
		t.Fatalf("removed=%d", s4.Removed)
	}
	if n := count(t, st, `SELECT COUNT(*) FROM items WHERE kind='season' AND season_num=2 AND missing=1`); n != 1 {
		t.Fatal("empty season should be missing")
	}
	if n := count(t, st, `SELECT COUNT(*) FROM items WHERE kind='episode' AND season_num=2`); n != 1 {
		t.Fatal("episode row must survive as missing")
	}
}

func TestUnavailableRootDeletesNothing(t *testing.T) {
	st, sc, media := setup(t)
	ctx := context.Background()
	lib, _ := st.CreateLibrary("Filmek", filepath.Join(media, "Movies"), "movies")
	if _, err := sc.ScanNow(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	before := count(t, st, `SELECT COUNT(*) FROM items WHERE missing=0`)

	// share unmounted: root gone
	os.Rename(filepath.Join(media, "Movies"), filepath.Join(media, "Movies.offline"))
	if _, err := sc.ScanNow(ctx, lib.ID); err == nil {
		t.Fatal("scan of unavailable root must fail")
	}
	// share mounted but empty mountpoint
	os.MkdirAll(filepath.Join(media, "Movies"), 0o755)
	if _, err := sc.ScanNow(ctx, lib.ID); err == nil {
		t.Fatal("scan of suddenly empty root must fail")
	}
	if after := count(t, st, `SELECT COUNT(*) FROM items WHERE missing=0`); after != before {
		t.Fatalf("items changed: %d → %d", before, after)
	}
}

func itoa(i int64) string { return filepath.Base(ImageDir("", i)) }
