package acquire

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ScotyLucas/kinora/internal/scanner"
	"github.com/ScotyLucas/kinora/internal/store"
)

// deleteImported removes what a finished grab put into the library: the film file with its
// subtitles (and its folder once no video is left in it), or the grab's episodes of a season.
// Only paths inside the library root are touched.
func deleteImported(g *store.AcqGrab, libRoot string) error {
	root, err := filepath.Abs(libRoot)
	if err != nil {
		return err
	}
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		return fmt.Errorf("a könyvtár mappája nem érhető el: %s", libRoot)
	}
	target, err := filepath.Abs(g.TargetPath)
	if err != nil || g.TargetPath == "" {
		return errors.New("a letöltés nem tudja, hová került a könyvtárban")
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return errors.New("a célfájl nincs a könyvtár mappájában")
	}
	if g.Kind == "episode" || g.Kind == "season" {
		return deleteEpisodes(root, target, g.Season, splitInts(g.Episodes))
	}
	fi, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil // already gone
	}
	if err != nil || !fi.Mode().IsRegular() {
		return errors.New("a célfájl nem törölhető")
	}
	if err := removeWithSubs(target); err != nil {
		return err
	}
	// the film's own folder ("Title (Year) {imdb-…}") goes too once no video is left in it: first the
	// film's metadata and artwork (movie.nfo, poster.jpg…), then the folder itself if that emptied it.
	// Files Kinora does not recognise stay, and so does the folder holding them.
	dir := filepath.Dir(target)
	if r, err := filepath.Rel(root, dir); err == nil && r != "." && !strings.ContainsRune(r, filepath.Separator) && !hasVideo(dir) {
		ents, _ := os.ReadDir(dir)
		for _, e := range ents {
			if e.Type().IsRegular() && filmSidecars[strings.ToLower(e.Name())] {
				os.Remove(filepath.Join(dir, e.Name()))
			}
		}
		os.Remove(dir) // fails harmlessly when something is left
	}
	return nil
}

// deleteEpisodes removes the episode files of one season from a show folder; no episodes = the whole season.
func deleteEpisodes(root, showDir string, season int, eps []int) error {
	if season < 0 {
		return errors.New("ismeretlen évad")
	}
	seasonDir := filepath.Join(showDir, fmt.Sprintf("Season %02d", season))
	ents, err := os.ReadDir(seasonDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	want := map[int]bool{}
	for _, e := range eps {
		want[e] = true
	}
	for _, e := range ents {
		if !e.Type().IsRegular() || !scanner.VideoExts[strings.ToLower(filepath.Ext(e.Name()))] {
			continue
		}
		r := ParseRelease(e.Name())
		if r.Season != season || len(r.Episodes) == 0 {
			continue
		}
		hit := len(want) == 0
		for _, n := range r.Episodes {
			hit = hit || want[n]
		}
		if hit {
			if err := removeWithSubs(filepath.Join(seasonDir, e.Name())); err != nil {
				return err
			}
		}
	}
	// empty folders left behind go; anything else (artwork, other files) keeps them
	os.Remove(seasonDir)
	if r, err := filepath.Rel(root, showDir); err == nil && r != "." && !strings.HasPrefix(r, "..") {
		os.Remove(showDir)
	}
	return nil
}

// filmSidecars are the metadata and artwork files of a film folder (Kodi/Jellyfin naming), removed with the film.
var filmSidecars = map[string]bool{
	"movie.nfo": true, "poster.jpg": true, "poster.png": true, "folder.jpg": true, "folder.png": true, "cover.jpg": true,
	"fanart.jpg": true, "fanart.png": true, "backdrop.jpg": true, "backdrop.png": true, "landscape.jpg": true,
	"banner.jpg": true, "clearlogo.png": true, "logo.png": true, "clearart.png": true, "disc.png": true, "thumb.jpg": true,
}

// hasVideo says whether a folder (or any folder in it) still holds a video file.
func hasVideo(dir string) bool {
	found := false
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && scanner.VideoExts[strings.ToLower(filepath.Ext(p))] {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// removeWithSubs deletes a video file and the "<name>.*" subtitles next to it.
func removeWithSubs(file string) error {
	if err := os.Remove(file); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("a fájl nem törölhető: %w", err)
	}
	base := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file)) + "."
	ents, _ := os.ReadDir(filepath.Dir(file))
	for _, e := range ents {
		if !e.IsDir() && strings.HasPrefix(e.Name(), base) && subExts[strings.ToLower(filepath.Ext(e.Name()))] {
			os.Remove(filepath.Join(filepath.Dir(file), e.Name()))
		}
	}
	return nil
}
