package scanner

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(dst), 0o755)
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	io.Copy(out, in)
}

func TestParseEpisodeInMoviesLibrary(t *testing.T) {
	p, ok := EpisodeInMoviesLibrary("Breaking.Bad.S01E02.720p.mkv")
	if !ok || p.Kind != "episode" || p.Series != "Breaking Bad" || p.Season != 1 || p.Episode != 2 {
		t.Fatalf("S01E02: %+v", p)
	}
	if p, ok := EpisodeInMoviesLibrary("Test Show (2020)/Season 01/Test.Show.S01E03.Pilot.Part.3.mkv"); !ok || p.Series != "Test Show" {
		t.Fatalf("folder layout: %+v", p)
	}
	if _, ok := EpisodeInMoviesLibrary("Apollo 13 (1995)/Apollo.13.1995.mkv"); ok {
		t.Fatal("a film is not an episode")
	}
}

// A series that was added as a movies library (every episode a separate "film") is
// regrouped into series → seasons → episodes on the next scan, keeping watch state.
func TestSeriesInMoviesLibraryIsRegrouped(t *testing.T) {
	st, sc, media := setup(t)
	ctx := context.Background()
	dir := t.TempDir()
	for _, ep := range []string{"Test.Show.S01E01.Pilot.Part.1.mkv", "Test.Show.S01E02.Pilot.Part.2.mkv"} {
		copyFile(t, filepath.Join(media, "Shows", "Test Show (2020)", "Season 01", ep), filepath.Join(dir, "Sorozat", ep))
	}
	// a lone S01E01-named file of another title stays a film
	copyFile(t, filepath.Join(media, "Shows", "Other.Series", "Other.Series.S01E01.mp4"), filepath.Join(dir, "Weird.S01E01.Named.Movie.2004.mp4"))
	lib, _ := st.CreateLibrary("Filmek", dir, "movies")
	if _, err := sc.ScanNow(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	var series, episodes, movies int
	st.DB.QueryRow(`SELECT COUNT(*) FILTER (WHERE kind='series'), COUNT(*) FILTER (WHERE kind='episode'), COUNT(*) FILTER (WHERE kind='movie') FROM items WHERE library_id=?`, lib.ID).
		Scan(&series, &episodes, &movies)
	if series != 1 || episodes != 2 || movies != 1 {
		t.Fatalf("series=%d episodes=%d movies=%d", series, episodes, movies)
	}

	// an older version had stored them as films: the next scan converts them in place
	var id int64
	st.DB.QueryRow(`SELECT id FROM items WHERE library_id=? AND kind='episode' ORDER BY episode_num LIMIT 1`, lib.ID).Scan(&id)
	st.DB.Exec(`UPDATE items SET kind='movie', parent_id=NULL, series_id=NULL, season_num=NULL, episode_num=NULL WHERE library_id=? AND kind='episode'`, lib.ID)
	st.DB.Exec(`INSERT INTO users(id,username,pass_hash,created_at) VALUES(99,'u','x',0)`)
	st.DB.Exec(`INSERT INTO profiles(id,user_id,name,created_at) VALUES(99,99,'p',0)`)
	st.DB.Exec(`INSERT INTO user_data(profile_id,item_id,played,play_count) VALUES(99,?,1,1)`, id)
	if _, err := sc.ScanNow(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	st.DB.QueryRow(`SELECT COUNT(*) FILTER (WHERE kind='episode' AND series_id IS NOT NULL), COUNT(*) FILTER (WHERE kind='movie') FROM items WHERE library_id=?`, lib.ID).
		Scan(&episodes, &movies)
	if episodes != 2 || movies != 1 {
		t.Fatalf("after rescan: episodes=%d movies=%d", episodes, movies)
	}
	var played int
	st.DB.QueryRow(`SELECT played FROM user_data WHERE profile_id=99 AND item_id=?`, id).Scan(&played)
	if played != 1 {
		t.Fatal("watch state lost while regrouping")
	}
}
