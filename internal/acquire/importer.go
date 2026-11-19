package acquire

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/ScotyLucas/kinora/internal/scanner"
)

// ImportMovie is what lands in the library.
type ImportMovie struct {
	Source  string // the finished download: a file or a folder (as Kinora sees it)
	LibRoot string // the movies library's folder
	Title   string
	Year    int
	IMDb    string
	Quality string // "1080p BluRay"
	Mode    string // "hardlink" (falls back to a copy across file systems) or "copy"
}

// ImportResult says where the film went and how.
type ImportResult struct {
	Path   string   `json:"path"`
	Linked bool     `json:"linked"` // hardlinked (no extra space) rather than copied
	Extras []string `json:"extras,omitempty"`
}

var subExts = map[string]bool{".srt": true, ".ass": true, ".ssa": true, ".vtt": true, ".sub": true, ".idx": true}

// SafeName makes a title usable as a file or folder name on every system.
func SafeName(s string) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case r < 32 || r == 127:
			return -1
		case strings.ContainsRune(`<>:"/\|?*`, r):
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	s = strings.Trim(s, " .")
	if len(s) > 180 {
		s = strings.TrimRight(s[:180], " .")
	}
	if s == "" {
		s = "Film"
	}
	return s
}

// MainVideo picks the film in a download: the largest video file that is not a sample.
func MainVideo(src string) (string, []string, error) {
	info, err := os.Stat(src)
	if err != nil {
		return "", nil, fmt.Errorf("a letöltött fájl nem található: %s", src)
	}
	if !info.IsDir() {
		if !scanner.VideoExts[strings.ToLower(filepath.Ext(src))] {
			return "", nil, errors.New("a letöltés nem videófájl")
		}
		return src, siblingsSubs(filepath.Dir(src), src), nil
	}
	var best string
	var bestSize int64
	var subs []string
	filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if strings.EqualFold(d.Name(), "sample") || strings.EqualFold(d.Name(), "samples") {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil // never follow a link out of the download
		}
		ext := strings.ToLower(filepath.Ext(p))
		if subExts[ext] {
			subs = append(subs, p)
			return nil
		}
		if !scanner.VideoExts[ext] {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return nil
		}
		name := strings.ToLower(d.Name())
		if strings.Contains(name, "sample") && fi.Size() < 500<<20 {
			return nil
		}
		if fi.Size() > bestSize {
			best, bestSize = p, fi.Size()
		}
		return nil
	})
	if best == "" {
		return "", nil, errors.New("a letöltésben nincs videófájl")
	}
	sort.Strings(subs)
	return best, subs, nil
}

func siblingsSubs(dir, video string) []string {
	base := strings.TrimSuffix(filepath.Base(video), filepath.Ext(video))
	ents, _ := os.ReadDir(dir)
	var out []string
	for _, e := range ents {
		if e.IsDir() || !subExts[strings.ToLower(filepath.Ext(e.Name()))] {
			continue
		}
		if strings.HasPrefix(e.Name(), base) {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	return out
}

// Import links (or copies) the film into the library as
// "<Title> (<Year>) {imdb-tt…}/<Title> (<Year>) - <Quality>.<ext>", with its subtitles next to it.
func Import(m ImportMovie) (ImportResult, error) {
	video, subs, err := MainVideo(m.Source)
	if err != nil {
		return ImportResult{}, err
	}
	root, err := filepath.Abs(m.LibRoot)
	if err != nil {
		return ImportResult{}, err
	}
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		return ImportResult{}, fmt.Errorf("a könyvtár mappája nem érhető el: %s", m.LibRoot)
	}
	name := SafeName(m.Title)
	if m.Year > 0 {
		name += " (" + strconv.Itoa(m.Year) + ")"
	}
	folder := name
	if m.IMDb != "" {
		folder += " {imdb-" + m.IMDb + "}" // the scanner matches the film by this tag
	}
	dir := filepath.Join(root, folder)
	if rel, err := filepath.Rel(root, dir); err != nil || strings.HasPrefix(rel, "..") {
		return ImportResult{}, errors.New("érvénytelen célmappa")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ImportResult{}, fmt.Errorf("a célmappa nem hozható létre: %w", err)
	}
	fileBase := name
	if m.Quality != "" {
		fileBase += " - " + SafeName(m.Quality)
	}
	ext := strings.ToLower(filepath.Ext(video))
	target, err := freeName(dir, fileBase, ext, video)
	if err != nil {
		return ImportResult{}, err
	}
	res := ImportResult{Path: target}
	if target != "" {
		linked, err := place(video, target, m.Mode)
		if err != nil {
			return ImportResult{}, err
		}
		res.Linked = linked
	}
	if res.Path == "" { // the same file is already there
		res.Path = filepath.Join(dir, fileBase+ext)
	}
	// subtitles: "<file>.<lang>.srt" keeps the language part of the original name
	videoBase := strings.TrimSuffix(filepath.Base(video), filepath.Ext(video))
	for _, s := range subs {
		sb := filepath.Base(s)
		tail := strings.TrimPrefix(sb, videoBase)
		if tail == sb { // a subtitle with another name (e.g. Subs/2_English.srt) keeps it as its suffix
			tail = "." + SafeName(strings.TrimSuffix(sb, filepath.Ext(sb))) + strings.ToLower(filepath.Ext(sb))
		}
		dst := strings.TrimSuffix(res.Path, ext) + tail
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		if _, err := place(s, dst, m.Mode); err == nil {
			res.Extras = append(res.Extras, dst)
		}
	}
	return res, nil
}

// freeName returns a target path not yet taken; "" when the same file (same size) is already there.
func freeName(dir, base, ext, src string) (string, error) {
	si, err := os.Stat(src)
	if err != nil {
		return "", err
	}
	for n := 1; n < 100; n++ {
		p := filepath.Join(dir, base+ext)
		if n > 1 {
			p = filepath.Join(dir, base+" ("+strconv.Itoa(n)+")"+ext)
		}
		ti, err := os.Stat(p)
		if errors.Is(err, fs.ErrNotExist) {
			return p, nil
		}
		if err == nil && (os.SameFile(si, ti) || ti.Size() == si.Size()) {
			return "", nil
		}
	}
	return "", errors.New("túl sok azonos nevű fájl a célmappában")
}

// place hardlinks src to dst when asked and possible, otherwise copies it. Reports whether it linked.
func place(src, dst, mode string) (bool, error) {
	if mode != "copy" {
		if err := os.Link(src, dst); err == nil {
			return true, nil
		}
	}
	return false, copyFile(src, dst)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".kinora-part"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("a fájl nem másolható: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return fmt.Errorf("a fájl másolása megszakadt: %w", err)
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

// ImportShow is what lands in a series library: one or more episodes of one season.
type ImportShow struct {
	Source  string // a file or a folder (a season pack)
	LibRoot string
	Title   string
	Year    int
	Quality string
	Mode    string
	Season  int
	Want    []int // the episodes to take; files of other episodes are left alone
}

// ImportedEpisodes says where each episode went.
type ImportedEpisodes struct {
	Folder   string         `json:"folder"`
	Episodes map[int]string `json:"episodes"`
	Linked   bool           `json:"linked"`
}

// videoFiles lists the non-sample video files of a download, with the subtitles found in it.
func videoFiles(src string) ([]string, []string, error) {
	info, err := os.Stat(src)
	if err != nil {
		return nil, nil, fmt.Errorf("a letöltött fájl nem található: %s", src)
	}
	if !info.IsDir() {
		if !scanner.VideoExts[strings.ToLower(filepath.Ext(src))] {
			return nil, nil, errors.New("a letöltés nem videófájl")
		}
		return []string{src}, siblingsSubs(filepath.Dir(src), src), nil
	}
	var videos, subs []string
	filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if strings.EqualFold(d.Name(), "sample") || strings.EqualFold(d.Name(), "samples") {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(p))
		if subExts[ext] {
			subs = append(subs, p)
			return nil
		}
		if !scanner.VideoExts[ext] {
			return nil
		}
		if fi, err := d.Info(); err == nil && strings.Contains(strings.ToLower(d.Name()), "sample") && fi.Size() < 300<<20 {
			return nil
		}
		videos = append(videos, p)
		return nil
	})
	sort.Strings(videos)
	sort.Strings(subs)
	if len(videos) == 0 {
		return nil, nil, errors.New("a letöltésben nincs videófájl")
	}
	return videos, subs, nil
}

// ImportEpisodes links (or copies) episodes into "<Title> (<Year>)/Season 01/<Title> (<Year>) - S01E03 - <Quality>.ext",
// each with the subtitles named after it. A file is matched to its episode by the SxxEyy in its name.
func ImportEpisodes(m ImportShow) (ImportedEpisodes, error) {
	videos, subs, err := videoFiles(m.Source)
	if err != nil {
		return ImportedEpisodes{}, err
	}
	root, err := filepath.Abs(m.LibRoot)
	if err != nil {
		return ImportedEpisodes{}, err
	}
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		return ImportedEpisodes{}, fmt.Errorf("a könyvtár mappája nem érhető el: %s", m.LibRoot)
	}
	show := SafeName(m.Title)
	if m.Year > 0 {
		show += " (" + strconv.Itoa(m.Year) + ")"
	}
	dir := filepath.Join(root, show, fmt.Sprintf("Season %02d", m.Season))
	if rel, err := filepath.Rel(root, dir); err != nil || strings.HasPrefix(rel, "..") {
		return ImportedEpisodes{}, errors.New("érvénytelen célmappa")
	}
	want := map[int]bool{}
	for _, e := range m.Want {
		want[e] = true
	}
	out := ImportedEpisodes{Folder: filepath.Join(root, show), Episodes: map[int]string{}}
	for _, v := range videos {
		r := ParseRelease(filepath.Base(v))
		eps := r.Episodes
		if r.Season >= 0 && r.Season != m.Season {
			continue
		}
		if len(eps) == 0 && len(videos) == 1 && len(m.Want) == 1 { // a lone file without SxxEyy: the one episode asked for
			eps = m.Want
		}
		var take []int
		for _, e := range eps {
			if len(want) == 0 || want[e] {
				take = append(take, e)
			}
		}
		if len(take) == 0 {
			continue
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return out, fmt.Errorf("a célmappa nem hozható létre: %w", err)
		}
		code := fmt.Sprintf("S%02dE%02d", m.Season, take[0])
		if len(take) > 1 {
			code += fmt.Sprintf("-E%02d", take[len(take)-1])
		}
		base := show + " - " + code
		if m.Quality != "" {
			base += " - " + SafeName(m.Quality)
		}
		ext := strings.ToLower(filepath.Ext(v))
		target, err := freeName(dir, base, ext, v)
		if err != nil {
			return out, err
		}
		if target == "" {
			target = filepath.Join(dir, base+ext)
		} else {
			linked, err := place(v, target, m.Mode)
			if err != nil {
				return out, err
			}
			out.Linked = out.Linked || linked
		}
		for _, e := range take {
			out.Episodes[e] = target
		}
		vb := strings.TrimSuffix(filepath.Base(v), filepath.Ext(v))
		for _, s := range subs {
			sb := filepath.Base(s)
			if !strings.HasPrefix(sb, vb) {
				continue
			}
			dst := strings.TrimSuffix(target, ext) + strings.TrimPrefix(sb, vb)
			if _, err := os.Stat(dst); err != nil {
				place(s, dst, m.Mode)
			}
		}
	}
	if len(out.Episodes) == 0 {
		return out, errors.New("a letöltésben nem találtam a keresett részeket")
	}
	return out, nil
}
