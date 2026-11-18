package meta

// Metadata in the viewers' languages. The items table holds the metadata_lang texts (the admin setting);
// every other language a client asks for (X-Kinora-Lang) is registered in metadata_langs, and each refresh
// fills item_i18n for it from TMDB by the stored ids: titles and overviews of movies, series, seasons and
// episodes, and the genre names.

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

const langsKey = "metadata_langs"

// maxViewerLangs caps the languages fetched besides metadata_lang (each one is a full pass over the library).
const maxViewerLangs = 8

var reUILang = regexp.MustCompile(`^[a-z]{2,3}(-[a-z0-9]{2,8})?$`)

// TMDBLang maps a client language code (hu, en, pt-br) to a TMDB language (hu-HU, en-US, pt-BR); "" if invalid.
func TMDBLang(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	if !reUILang.MatchString(code) {
		return ""
	}
	base, region, _ := strings.Cut(code, "-")
	switch base { // TMDB's main translations; en-GB and the like are often empty
	case "hu":
		return "hu-HU"
	case "en":
		return "en-US"
	case "de":
		return "de-DE"
	}
	if region == "" {
		return base
	}
	return base + "-" + strings.ToUpper(region)
}

// ViewLang returns the TMDB language a client with UI language code wants metadata in, or "" when that is
// metadata_lang (the stored texts). A new language is registered and its translations fetched in the background.
func (f *Fetcher) ViewLang(ctx context.Context, code string) string {
	lang := TMDBLang(code)
	if lang == "" || lang == f.st.Setting("metadata_lang") {
		return ""
	}
	f.mu.Lock()
	f.loadLangsLocked()
	known, full := f.langs[lang], len(f.langs) >= maxViewerLangs
	if !known && !full {
		f.langs[lang] = true
		f.st.SetSetting(langsKey, strings.Join(sortedKeys(f.langs), ","))
	}
	f.mu.Unlock()
	if !known && !full {
		f.log.Info("tmdb: new viewer language", "lang", lang)
		f.Kick(ctx)
	}
	return lang
}

func (f *Fetcher) loadLangsLocked() {
	if f.langs != nil {
		return
	}
	f.langs = map[string]bool{}
	for _, l := range strings.Split(f.st.Setting(langsKey), ",") {
		if l = strings.TrimSpace(l); l != "" {
			f.langs[l] = true
		}
	}
}

// viewerLangs lists the registered languages except metadata_lang.
func (f *Fetcher) viewerLangs() []string {
	base := f.st.Setting("metadata_lang")
	f.mu.Lock()
	f.loadLangsLocked()
	all := sortedKeys(f.langs)
	f.mu.Unlock()
	out := all[:0:0]
	for _, l := range all {
		if l != base {
			out = append(out, l)
		}
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (f *Fetcher) translateAll(ctx context.Context) error {
	for _, lang := range f.viewerLangs() {
		if err := f.translate(ctx, lang); err != nil {
			return err
		}
	}
	return nil
}

// translate fetches lang texts for matched movies and series that lack them (or have new seasons / episodes).
func (f *Fetcher) translate(ctx context.Context, lang string) error {
	rows, err := f.st.DB.Query(`SELECT i.id,i.kind,i.title,i.tmdb_id FROM items i
		WHERE i.kind IN ('movie','series') AND i.missing=0 AND i.tmdb_id>0 AND (
		  NOT EXISTS(SELECT 1 FROM item_i18n t WHERE t.item_id=i.id AND t.lang=?)
		  OR (i.kind='series' AND EXISTS(SELECT 1 FROM items e WHERE (e.series_id=i.id OR e.parent_id=i.id) AND e.id<>i.id
		      AND e.kind IN ('season','episode') AND e.missing=0
		      AND NOT EXISTS(SELECT 1 FROM item_i18n t WHERE t.item_id=e.id AND t.lang=?))))`, lang, lang)
	if err != nil {
		return err
	}
	type job struct {
		id, tmdb    int64
		kind, title string
	}
	var jobs []job
	for rows.Next() {
		var j job
		if rows.Scan(&j.id, &j.kind, &j.title, &j.tmdb) == nil {
			jobs = append(jobs, j)
		}
	}
	rows.Close()
	if len(jobs) == 0 && f.st.HasGenreLabels(lang) {
		return nil
	}
	if err := f.genreLabels(ctx, lang); err != nil {
		if isFatal(err) {
			return err
		}
		f.log.Warn("tmdb genres", "lang", lang, "err", err)
	}
	if len(jobs) == 0 {
		return nil
	}
	f.mu.Lock()
	f.status = Status{Running: true, Total: len(jobs), Translate: lang}
	f.mu.Unlock()
	f.log.Info("tmdb: fetching titles in a viewer language", "lang", lang, "items", len(jobs))
	for _, j := range jobs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err := f.translateOne(ctx, j.id, j.kind, j.tmdb, lang)
		f.mu.Lock()
		f.status.Done++
		if err != nil {
			f.status.LastError = j.title + ": " + err.Error()
		} else {
			f.status.Matched++
		}
		f.mu.Unlock()
		if err != nil {
			f.log.Warn("tmdb translation", "title", j.title, "lang", lang, "err", err)
			if isFatal(err) {
				return err
			}
		}
	}
	return nil
}

func isFatal(err error) bool { return errors.Is(err, ErrNoKey) || strings.Contains(err.Error(), "401") }

// translateOne stores the lang texts of one movie / series (with its seasons and episodes).
func (f *Fetcher) translateOne(ctx context.Context, id int64, kind string, tmdb int64, lang string) error {
	q := func() url.Values { return url.Values{"language": {lang}} }
	if kind == "movie" {
		var d movieDetails
		if err := f.get(ctx, fmt.Sprintf("/movie/%d", tmdb), q(), &d); err != nil {
			if errors.Is(err, errNotFound) {
				return f.st.SetTranslation(id, lang, "", "")
			}
			return err
		}
		return f.st.SetTranslation(id, lang, d.Title, d.Overview)
	}
	var d tvDetails
	if err := f.get(ctx, fmt.Sprintf("/tv/%d", tmdb), q(), &d); err != nil {
		if errors.Is(err, errNotFound) {
			if err := f.st.SetTranslation(id, lang, "", ""); err != nil {
				return err
			}
			return f.st.MarkChildrenTranslated(id, lang)
		}
		return err
	}
	seasons, err := f.st.SeasonsOf(id)
	if err != nil {
		return err
	}
	for _, sn := range seasons {
		var sd seasonDetails
		if err := f.get(ctx, fmt.Sprintf("/tv/%d/season/%d", tmdb, sn.Num), q(), &sd); err != nil {
			if errors.Is(err, errNotFound) {
				continue
			}
			return err
		}
		if err := f.st.SetTranslation(sn.ID, lang, sd.Name, sd.Overview); err != nil {
			return err
		}
		for _, ep := range sd.Episodes {
			if eid := f.st.EpisodeID(id, sn.Num, ep.EpisodeNumber); eid != 0 {
				if err := f.st.SetTranslation(eid, lang, ep.Name, ep.Overview); err != nil {
					return err
				}
			}
		}
	}
	if err := f.st.MarkChildrenTranslated(id, lang); err != nil {
		return err
	}
	// the series row last: it marks the whole series as done
	return f.st.SetTranslation(id, lang, d.Name, d.Overview)
}

// genreLabels maps the metadata_lang genre names to their lang names by TMDB genre id.
func (f *Fetcher) genreLabels(ctx context.Context, lang string) error {
	base := f.st.Setting("metadata_lang")
	type list struct {
		Genres []genre `json:"genres"`
	}
	labels := map[string]string{}
	for _, kind := range []string{"movie", "tv"} {
		var from, to list
		if err := f.get(ctx, "/genre/"+kind+"/list", url.Values{"language": {base}}, &from); err != nil {
			return err
		}
		if err := f.get(ctx, "/genre/"+kind+"/list", url.Values{"language": {lang}}, &to); err != nil {
			return err
		}
		names := map[int64]string{}
		for _, g := range to.Genres {
			names[g.ID] = g.Name
		}
		for _, g := range from.Genres {
			l := names[g.ID]
			if l == "" || g.Name == "" {
				continue
			}
			labels[g.Name] = l
			// series genres are stored split ("Sci-Fi & Fantasy" → "Sci-Fi", "Fantasy")
			if a, b := strings.Split(g.Name, " & "), strings.Split(l, " & "); len(a) > 1 && len(a) == len(b) {
				for i := range a {
					labels[strings.TrimSpace(a[i])] = strings.TrimSpace(b[i])
				}
			}
		}
	}
	return f.st.SetGenreLabels(lang, labels)
}
