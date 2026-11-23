package acquire

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ScotyLucas/kinora/internal/store"
)

func touch(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func TestDeleteImportedMovie(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "Film (2020) {imdb-tt1}")
	film := filepath.Join(dir, "Film (2020) - 1080p.mkv")
	touch(t, film)
	touch(t, filepath.Join(dir, "Film (2020) - 1080p.hu.srt"))
	touch(t, filepath.Join(dir, "poster.jpg"))
	other := filepath.Join(root, "Other (2021)", "Other (2021).mkv")
	touch(t, other)

	if err := deleteImported(&store.AcqGrab{Kind: "movie", TargetPath: film}, root); err != nil {
		t.Fatal(err)
	}
	if exists(dir) {
		t.Error("the film's folder should be gone")
	}
	if !exists(other) {
		t.Error("another film was touched")
	}
	// a second run (already deleted) is fine
	if err := deleteImported(&store.AcqGrab{Kind: "movie", TargetPath: film}, root); err != nil {
		t.Error(err)
	}
	// nothing outside the library root
	if err := deleteImported(&store.AcqGrab{Kind: "movie", TargetPath: filepath.Join(t.TempDir(), "x.mkv")}, root); err == nil {
		t.Error("a path outside the library was accepted")
	}
}

func TestDeleteImportedKeepsFolderWithOtherVideo(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "Film (2020)")
	film := filepath.Join(dir, "Film (2020) - 1080p.mkv")
	touch(t, film)
	keep := filepath.Join(dir, "Film (2020) - 2160p.mkv")
	touch(t, keep)
	if err := deleteImported(&store.AcqGrab{Kind: "movie", TargetPath: film}, root); err != nil {
		t.Fatal(err)
	}
	if exists(film) || !exists(keep) {
		t.Error("only the grab's own file should go")
	}
}

func TestDeleteImportedEpisodes(t *testing.T) {
	root := t.TempDir()
	show := filepath.Join(root, "Show (2019)")
	s1 := filepath.Join(show, "Season 01")
	e1 := filepath.Join(s1, "Show (2019) - S01E01 - 1080p.mkv")
	e2 := filepath.Join(s1, "Show (2019) - S01E02 - 1080p.mkv")
	touch(t, e1)
	touch(t, filepath.Join(s1, "Show (2019) - S01E01 - 1080p.en.srt"))
	touch(t, e2)
	s2e1 := filepath.Join(show, "Season 02", "Show (2019) - S02E01 - 1080p.mkv")
	touch(t, s2e1)

	if err := deleteImported(&store.AcqGrab{Kind: "episode", Season: 1, Episodes: "1", TargetPath: show}, root); err != nil {
		t.Fatal(err)
	}
	if exists(e1) || exists(filepath.Join(s1, "Show (2019) - S01E01 - 1080p.en.srt")) || !exists(e2) {
		t.Error("only S01E01 should go")
	}
	// a pack: the whole season
	if err := deleteImported(&store.AcqGrab{Kind: "season", Season: 1, TargetPath: show}, root); err != nil {
		t.Fatal(err)
	}
	if exists(s1) || !exists(s2e1) {
		t.Error("season 1 should go, season 2 stay")
	}
}

// Files Kinora does not know (the user's notes, a bonus archive) are not deleted with the film; the folder stays for them.
func TestDeleteImportedKeepsUnknownFiles(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "Film (2020)")
	film := filepath.Join(dir, "Film (2020) - 1080p.mkv")
	touch(t, film)
	touch(t, filepath.Join(dir, "movie.nfo"))
	notes := filepath.Join(dir, "my notes.txt")
	touch(t, notes)
	if err := deleteImported(&store.AcqGrab{Kind: "movie", TargetPath: film}, root); err != nil {
		t.Fatal(err)
	}
	if exists(film) || exists(filepath.Join(dir, "movie.nfo")) {
		t.Error("the film and its NFO should go")
	}
	if !exists(notes) {
		t.Error("an unknown file was deleted")
	}
}
