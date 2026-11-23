package jfcompat

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ScotyLucas/kinora/internal/scanner"
	"github.com/ScotyLucas/kinora/internal/store"
)

// ------------------------------------------------------------------ item → BaseItemDto

var kindType = map[string]string{"movie": "Movie", "series": "Series", "season": "Season", "episode": "Episode"}

// image file names Kinora keeps per item
func (s *Server) imageFile(id int64, name string) (string, time.Time, bool) {
	p := filepath.Join(scanner.ImageDir(s.ImgDir, id), name+".jpg")
	st, err := os.Stat(p)
	if err != nil {
		return "", time.Time{}, false
	}
	return p, st.ModTime(), true
}

func imageTag(id int64, mod time.Time) string {
	h := sha1.Sum([]byte(fmt.Sprintf("%d-%d", id, mod.UnixNano())))
	return hex.EncodeToString(h[:8])
}

// imageChain is where an image type of an item is looked for: its own files first, then its series'.
func (s *Server) imageChain(it *store.Item, typ string) []struct {
	id   int64
	name string
} {
	type src = struct {
		id   int64
		name string
	}
	own := func(n string) src { return src{it.ID, n} }
	var series int64
	if it.SeriesID != nil {
		series = *it.SeriesID
	}
	ser := func(n string) src { return src{series, n} }
	var chain []src
	switch strings.ToLower(typ) {
	case "primary":
		if it.Kind == "episode" {
			chain = []src{own("thumb"), own("poster"), own("backdrop")}
		} else {
			chain = []src{own("poster"), ser("poster")}
		}
	case "backdrop", "art":
		chain = []src{own("backdrop"), own("thumb"), ser("backdrop")}
	case "thumb":
		chain = []src{own("thumb"), own("backdrop"), ser("backdrop")}
	case "logo", "banner", "disc", "box", "screenshot", "menu", "chapter", "boxrear", "profile":
		return nil
	}
	out := chain[:0]
	for _, c := range chain {
		if c.id > 0 {
			out = append(out, c)
		}
	}
	return out
}

// findImage returns the file serving an image type of an item.
func (s *Server) findImage(it *store.Item, typ string) (string, time.Time, bool) {
	for _, c := range s.imageChain(it, typ) {
		if p, mod, ok := s.imageFile(c.id, c.name); ok {
			return p, mod, true
		}
	}
	return "", time.Time{}, false
}

type dtoOpts struct {
	media bool // MediaSources + MediaStreams (the item must be loaded with ItemFull)
}

func (s *Server) itemID(id int64) string { return s.ids.encode(nsItem, id) }

func (s *Server) dto(it *store.Item, c *reqCtx, o dtoOpts) map[string]any {
	d := map[string]any{
		"Id": s.itemID(it.ID), "ServerId": s.serverID(), "Name": it.Title, "SortName": it.Title, "Type": kindType[it.Kind],
		"IsFolder": it.Kind == "series" || it.Kind == "season", "LocationType": "FileSystem", "DateCreated": isoTime(it.AddedAt),
		"Etag": strconv.FormatInt(it.UpdatedAt, 10), "CanDelete": false, "CanDownload": it.Kind == "movie" || it.Kind == "episode",
	}
	if it.Kind == "movie" || it.Kind == "episode" {
		d["MediaType"] = "Video"
		d["VideoType"] = "VideoFile"
		d["Container"] = it.Container
		if it.Width > 0 {
			d["Width"], d["Height"] = it.Width, it.Height
		}
	}
	if it.Overview != "" {
		d["Overview"] = it.Overview
	}
	if it.Year != nil {
		d["ProductionYear"] = *it.Year
		d["PremiereDate"] = fmt.Sprintf("%04d-01-01T00:00:00.0000000Z", *it.Year)
	}
	if it.ContentRating != "" {
		d["OfficialRating"] = it.ContentRating
	}
	if it.CommunityRating != nil {
		d["CommunityRating"] = *it.CommunityRating
	}
	if it.RuntimeSec > 0 {
		d["RunTimeTicks"] = ticks(float64(it.RuntimeSec))
	}
	d["Genres"] = nonNil(it.Genres)
	d["Tags"] = nonNil(it.Tags)
	prov := map[string]string{}
	if it.TmdbID != nil && *it.TmdbID > 0 {
		prov["Tmdb"] = strconv.FormatInt(*it.TmdbID, 10)
	}
	if it.ImdbID != "" {
		prov["Imdb"] = it.ImdbID
	}
	d["ProviderIds"] = prov

	switch it.Kind {
	case "movie", "series":
		d["ParentId"] = s.ids.encode(nsLibrary, it.LibraryID)
	case "season":
		if it.SeriesID != nil {
			d["SeriesId"], d["ParentId"] = s.itemID(*it.SeriesID), s.itemID(*it.SeriesID)
		}
		d["SeriesName"] = it.SeriesTitle
		if it.SeasonNum != nil {
			d["IndexNumber"] = *it.SeasonNum
		}
	case "episode":
		if it.SeriesID != nil {
			d["SeriesId"] = s.itemID(*it.SeriesID)
		}
		if it.ParentID != nil {
			d["SeasonId"], d["ParentId"] = s.itemID(*it.ParentID), s.itemID(*it.ParentID)
		}
		d["SeriesName"] = it.SeriesTitle
		if it.SeasonNum != nil {
			d["ParentIndexNumber"] = *it.SeasonNum
			d["SeasonName"] = fmt.Sprintf("Season %d", *it.SeasonNum)
		}
		if it.EpisodeNum != nil {
			d["IndexNumber"] = *it.EpisodeNum
		}
		if it.EpisodeEnd != nil {
			d["IndexNumberEnd"] = *it.EpisodeEnd
		}
	}
	if it.Kind == "series" || it.Kind == "season" {
		d["ChildCount"] = it.EpisodeCount
		d["RecursiveItemCount"] = it.EpisodeCount
	}

	// artwork: tags are the cache keys clients use; types without a file are left out
	tags := map[string]string{}
	for _, t := range []string{"Primary", "Thumb"} {
		if _, mod, ok := s.findImage(it, t); ok {
			tags[t] = imageTag(it.ID, mod)
		}
	}
	d["ImageTags"] = tags
	d["BackdropImageTags"] = []string{}
	if _, mod, ok := s.findImage(it, "Backdrop"); ok {
		d["BackdropImageTags"] = []string{imageTag(it.ID, mod)}
	}
	if it.SeriesID != nil && (it.Kind == "episode" || it.Kind == "season") {
		if _, mod, ok := s.imageFile(*it.SeriesID, "poster"); ok {
			d["SeriesPrimaryImageTag"] = imageTag(*it.SeriesID, mod)
		}
		if _, mod, ok := s.imageFile(*it.SeriesID, "backdrop"); ok {
			d["ParentBackdropItemId"] = s.itemID(*it.SeriesID)
			d["ParentBackdropImageTags"] = []string{imageTag(*it.SeriesID, mod)}
		}
	}

	ud := it.UserData
	u := map[string]any{
		"PlaybackPositionTicks": ticks(ud.Position), "PlayCount": ud.PlayCount, "IsFavorite": ud.Favorite, "Played": ud.Played,
		"Key": s.itemID(it.ID), "ItemId": s.itemID(it.ID),
	}
	if ud.LastPlayedAt != nil {
		u["LastPlayedDate"] = isoTime(*ud.LastPlayedAt)
	}
	if it.RuntimeSec > 0 && ud.Position > 0 && !ud.Played {
		u["PlayedPercentage"] = 100 * ud.Position / float64(it.RuntimeSec)
	}
	if it.Kind == "series" || it.Kind == "season" {
		u["UnplayedItemCount"] = it.UnplayedCount
		u["Played"] = it.EpisodeCount > 0 && it.UnplayedCount == 0
	}
	d["UserData"] = u

	if o.media && (it.Kind == "movie" || it.Kind == "episode") {
		src := s.mediaSource(it, c)
		d["MediaSources"] = []any{src}
		d["MediaStreams"] = src["MediaStreams"]
		d["HasSubtitles"] = hasSubs(it)
		d["Chapters"] = s.chapters(it)
	}
	return d
}

func nonNil(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

func hasSubs(it *store.Item) bool {
	for _, st := range it.Streams {
		if st.Type == "subtitle" {
			return true
		}
	}
	return false
}

func (s *Server) chapters(it *store.Item) []map[string]any {
	out := []map[string]any{}
	for _, ch := range it.Chapters {
		out = append(out, map[string]any{"StartPositionTicks": ticks(ch.Start), "Name": ch.Title, "ImageDateModified": "0001-01-01T00:00:00.0000000Z"})
	}
	return out
}

func (s *Server) dtos(items []*store.Item, c *reqCtx, o dtoOpts) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		out = append(out, s.dto(it, c, o))
	}
	return out
}

func (s *Server) wantMedia(r *http.Request) bool {
	for _, f := range qList(r, "Fields") {
		if strings.EqualFold(f, "MediaSources") || strings.EqualFold(f, "MediaStreams") {
			return true
		}
	}
	return false
}

// withMedia loads streams and chapters of the playable items of a list (only when the client asks for them).
func (s *Server) withMedia(items []*store.Item, c *reqCtx) []*store.Item {
	for i, it := range items {
		if it.Kind == "movie" || it.Kind == "episode" {
			if full, err := s.Store.ItemFull(it.ID, c.Profile); err == nil {
				items[i] = full
			}
		}
	}
	return items
}

// ------------------------------------------------------------------ views (libraries)

func (s *Server) libraries(c *reqCtx) []*store.Library {
	libs, _ := s.Store.Libraries()
	var out []*store.Library
	for _, l := range libs {
		if c.Profile.LibraryAllowed(l.ID) {
			out = append(out, l)
		}
	}
	return out
}

func (s *Server) viewDTO(l *store.Library) map[string]any {
	ct := map[string]string{"movies": "movies", "shows": "tvshows"}[l.Kind]
	d := map[string]any{
		"Id": s.ids.encode(nsLibrary, l.ID), "ServerId": s.serverID(), "Name": l.Name, "SortName": l.Name, "Type": "CollectionFolder",
		"IsFolder": true, "LocationType": "FileSystem", "ImageTags": map[string]string{}, "BackdropImageTags": []string{},
		"UserData": map[string]any{"PlaybackPositionTicks": 0, "PlayCount": 0, "IsFavorite": false, "Played": false, "Key": s.ids.encode(nsLibrary, l.ID)},
	}
	if ct != "" {
		d["CollectionType"] = ct
	}
	return d
}

func (s *Server) handleViews(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var items []map[string]any
	for _, l := range s.libraries(c) {
		items = append(items, s.viewDTO(l))
	}
	if items == nil {
		items = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"Items": items, "TotalRecordCount": len(items), "StartIndex": 0})
}

// ------------------------------------------------------------------ /Items

// itemsQuery turns Jellyfin's /Items parameters into a Kinora query.
func (s *Server) itemsQuery(r *http.Request, c *reqCtx) (where string, args []any, order string, ok bool) {
	var conds []string
	add := func(cond string, a ...any) { conds = append(conds, cond); args = append(args, a...) }
	add(" AND i.missing=0")

	types := map[string]bool{}
	for _, t := range qList(r, "IncludeItemTypes") {
		types[strings.ToLower(t)] = true
	}
	excl := map[string]bool{}
	for _, t := range qList(r, "ExcludeItemTypes") {
		excl[strings.ToLower(t)] = true
	}
	recursive, _ := qBool(r, "Recursive")

	if ids := qList(r, "Ids"); len(ids) > 0 {
		var ph []string
		for _, x := range ids {
			if n, good := s.ids.decode(nsItem, x); good {
				ph = append(ph, "?")
				args = append(args, n)
			}
		}
		if len(ph) == 0 {
			return "", nil, "", false
		}
		conds = append(conds, " AND i.id IN ("+strings.Join(ph, ",")+")")
	}

	parentKind := ""
	if pid := qv(r, "ParentId"); pid != "" {
		if lib, good := s.ids.decode(nsLibrary, pid); good {
			if !c.Profile.LibraryAllowed(lib) {
				return "", nil, "", false
			}
			add(" AND i.library_id=?", lib)
			parentKind = "library"
		} else if id, good := s.ids.decode(nsItem, pid); good {
			parent, err := s.Store.Item(id, c.Profile)
			if err != nil {
				return "", nil, "", false
			}
			parentKind = parent.Kind
			switch {
			case parent.Kind == "series" && (recursive || types["episode"]):
				add(" AND i.series_id=? AND i.kind='episode'", id)
			case parent.Kind == "series":
				add(" AND i.parent_id=? AND i.kind='season'", id)
			case parent.Kind == "season":
				add(" AND i.parent_id=? AND i.kind='episode'", id)
			default:
				return "", nil, "", false
			}
		} else {
			return "", nil, "", false
		}
	}

	// which kinds: the asked types, else what the parent holds (a library's top level: films and series)
	var kinds []string
	for t, k := range map[string]string{"movie": "movie", "series": "series", "season": "season", "episode": "episode"} {
		if types[t] && !excl[t] {
			kinds = append(kinds, k)
		}
	}
	if len(types) > 0 && len(kinds) == 0 {
		return "", nil, "", false // only types Kinora has none of (BoxSet, Audio, …)
	}
	if len(kinds) == 0 && len(qList(r, "Ids")) == 0 && (parentKind == "library" || parentKind == "") {
		kinds = []string{"movie", "series"} // a library's top level (asked ids may be any kind)
		for k := range excl {
			for i, x := range kinds {
				if x == k {
					kinds = append(kinds[:i], kinds[i+1:]...)
					break
				}
			}
		}
	}
	if len(kinds) > 0 {
		add(" AND i.kind IN ('" + strings.Join(kinds, "','") + "')")
	}
	add(store.NotAltVersion)

	filters := map[string]bool{}
	for _, f := range qList(r, "Filters") {
		filters[strings.ToLower(f)] = true
	}
	if b, set := qBool(r, "IsFavorite"); set {
		filters["isfavorite"], filters["notfavorite"] = b, !b
	}
	if b, set := qBool(r, "IsPlayed"); set {
		filters["isplayed"], filters["isunplayed"] = b, !b
	}
	if filters["isfavorite"] {
		add(" AND COALESCE(ud.favorite,0)=1")
	}
	if filters["isresumable"] {
		add(" AND COALESCE(ud.position,0)>0 AND COALESCE(ud.played,0)=0")
	}
	if filters["isplayed"] {
		add(" AND COALESCE(ud.played,0)=1")
	}
	if filters["isunplayed"] {
		add(" AND COALESCE(ud.played,0)=0")
	}
	if term := strings.TrimSpace(qv(r, "SearchTerm")); term != "" {
		add(` AND (i.title LIKE ? ESCAPE '\' OR i.sort_title LIKE ? ESCAPE '\')`, "%"+likeEscape(term)+"%", "%"+likeEscape(term)+"%")
	}
	if pre := qv(r, "NameStartsWith"); pre != "" {
		add(` AND i.sort_title LIKE ? ESCAPE '\'`, likeEscape(pre)+"%")
	}
	if g := qList(r, "Genres"); len(g) > 0 {
		ph := strings.TrimSuffix(strings.Repeat("?,", len(g)), ",")
		a := make([]any, len(g))
		for i, x := range g {
			a[i] = x
		}
		add(" AND EXISTS(SELECT 1 FROM item_genres g WHERE g.item_id=i.id AND g.genre IN ("+ph+"))", a...)
	}
	if ys := qList(r, "Years"); len(ys) > 0 {
		var ph []string
		var a []any
		for _, y := range ys {
			if n, err := strconv.Atoi(y); err == nil {
				ph = append(ph, "?")
				a = append(a, n)
			}
		}
		if len(ph) > 0 {
			add(" AND i.year IN ("+strings.Join(ph, ",")+")", a...)
		}
	}
	return strings.Join(conds, ""), args, s.orderBy(r, kinds), true
}

func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// orderBy maps SortBy / SortOrder; only known columns are used.
func (s *Server) orderBy(r *http.Request, kinds []string) string {
	dir := "ASC"
	if strings.HasPrefix(strings.ToLower(qv(r, "SortOrder")), "desc") {
		dir = "DESC"
	}
	cols := map[string]string{
		"sortname": "i.sort_title COLLATE NOCASE", "name": "i.sort_title COLLATE NOCASE", "seriessortname": "sr.sort_title COLLATE NOCASE",
		"datecreated": "i.added_at", "premieredate": "i.year", "productionyear": "i.year", "communityrating": "i.community_rating",
		"criticrating": "i.community_rating", "dateplayed": "ud.last_played_at", "dateLastContentAdded": "i.added_at",
		"runtime": "i.runtime_sec", "parentindexnumber": "i.season_num", "indexnumber": "i.episode_num", "random": "RANDOM()",
		"playcount": "ud.play_count", "officialrating": "i.rating_level",
	}
	var parts []string
	for _, f := range qList(r, "SortBy") {
		if c, ok := cols[strings.ToLower(f)]; ok {
			if c == "RANDOM()" {
				parts = append(parts, c)
			} else {
				parts = append(parts, c+" "+dir)
			}
		}
	}
	if len(parts) == 0 {
		if len(kinds) == 1 && kinds[0] == "episode" {
			parts = []string{"CASE WHEN i.season_num=0 THEN 1 ELSE 0 END", "i.season_num", "i.episode_num"}
		} else if len(kinds) == 1 && kinds[0] == "season" {
			parts = []string{"CASE WHEN i.season_num=0 THEN 1 ELSE 0 END", "i.season_num"}
		} else {
			parts = []string{"i.sort_title COLLATE NOCASE"}
		}
	}
	return "ORDER BY " + strings.Join(parts, ", ") + ", i.id"
}

func (s *Server) handleItems(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	where, args, order, ok := s.itemsQuery(r, c)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"Items": []any{}, "TotalRecordCount": 0, "StartIndex": 0})
		return
	}
	start := max(qInt(r, "StartIndex", 0), 0)
	limit := qInt(r, "Limit", 0)
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	items, err := s.Store.QueryItems(c.Profile, where, args, order+" LIMIT ? OFFSET ?", limit, start)
	if err != nil {
		s.Log.Error("jellyfin-compat: items", "err", err)
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	total, _ := s.Store.CountItems(c.Profile, where, args)
	s.Store.FillGenresTags(items, c.Profile)
	media := s.wantMedia(r)
	if media {
		items = s.withMedia(items, c)
	}
	writeJSON(w, http.StatusOK, map[string]any{"Items": s.dtos(items, c, dtoOpts{media: media}), "TotalRecordCount": total, "StartIndex": start})
}

// itemFromPath loads the visible item of {item}; a library id answers as its view.
func (s *Server) handleItem(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	raw := r.PathValue("item")
	if lib, ok := s.ids.decode(nsLibrary, raw); ok && c.Profile.LibraryAllowed(lib) {
		if l, err := s.Store.Library(lib); err == nil {
			writeJSON(w, http.StatusOK, s.viewDTO(l))
			return
		}
	}
	id, ok := s.ids.decode(nsItem, raw)
	if !ok {
		writeErr(w, http.StatusNotFound, "item not found")
		return
	}
	it, err := s.Store.ItemFull(id, c.Profile)
	if err != nil {
		writeErr(w, http.StatusNotFound, "item not found")
		return
	}
	writeJSON(w, http.StatusOK, s.dto(it, c, dtoOpts{media: true}))
}

func (s *Server) handleLatest(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	limit := qInt(r, "Limit", 16)
	if limit <= 0 || limit > 100 {
		limit = 16
	}
	var items []*store.Item
	libs := s.libraries(c)
	if pid := qv(r, "ParentId"); pid != "" {
		lib, ok := s.ids.decode(nsLibrary, pid)
		if !ok || !c.Profile.LibraryAllowed(lib) {
			writeJSON(w, http.StatusOK, []any{})
			return
		}
		libs = nil
		if l, err := s.Store.Library(lib); err == nil {
			libs = []*store.Library{l}
		}
	}
	for _, l := range libs {
		got, _ := s.Store.RecentlyAdded(c.Profile, l.ID, limit)
		items = append(items, got...)
	}
	if len(items) > limit {
		items = items[:limit]
	}
	writeJSON(w, http.StatusOK, s.dtos(items, c, dtoOpts{}))
}

func (s *Server) handleResume(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	limit := qInt(r, "Limit", 12)
	if limit <= 0 || limit > 100 {
		limit = 12
	}
	items, _ := s.Store.ContinueWatching(c.Profile, limit)
	if pid := qv(r, "ParentId"); pid != "" {
		if lib, ok := s.ids.decode(nsLibrary, pid); ok {
			var keep []*store.Item
			for _, it := range items {
				if it.LibraryID == lib {
					keep = append(keep, it)
				}
			}
			items = keep
		}
	}
	if s.wantMedia(r) {
		items = s.withMedia(items, c)
	}
	list := s.dtos(items, c, dtoOpts{media: s.wantMedia(r)})
	writeJSON(w, http.StatusOK, map[string]any{"Items": list, "TotalRecordCount": len(list), "StartIndex": 0})
}

func (s *Server) handleNextUp(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	limit := qInt(r, "Limit", 12)
	if limit <= 0 || limit > 100 {
		limit = 12
	}
	var items []*store.Item
	if sid := qv(r, "SeriesId"); sid != "" {
		if id, ok := s.ids.decode(nsItem, sid); ok {
			if it, err := s.Store.PlayNext(c.Profile, id); err == nil && it != nil {
				items = []*store.Item{it}
			}
		}
	} else {
		items, _ = s.Store.NextUp(c.Profile, limit)
	}
	list := s.dtos(items, c, dtoOpts{})
	writeJSON(w, http.StatusOK, map[string]any{"Items": list, "TotalRecordCount": len(list), "StartIndex": 0})
}

func (s *Server) handleSeasons(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	id, ok := s.ids.decode(nsItem, r.PathValue("item"))
	series, err := s.Store.Item(id, c.Profile)
	if !ok || err != nil || series.Kind != "series" {
		writeErr(w, http.StatusNotFound, "series not found")
		return
	}
	items, _ := s.Store.Children(c.Profile, series)
	list := s.dtos(items, c, dtoOpts{})
	writeJSON(w, http.StatusOK, map[string]any{"Items": list, "TotalRecordCount": len(list), "StartIndex": 0})
}

func (s *Server) handleEpisodes(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	id, ok := s.ids.decode(nsItem, r.PathValue("item"))
	series, err := s.Store.Item(id, c.Profile)
	if !ok || err != nil || series.Kind != "series" {
		writeErr(w, http.StatusNotFound, "series not found")
		return
	}
	where, args := " AND i.series_id=? AND i.kind='episode' AND i.missing=0"+store.NotAltEpisode, []any{id}
	if sid := qv(r, "SeasonId"); sid != "" {
		if season, ok := s.ids.decode(nsItem, sid); ok {
			where += " AND i.parent_id=?"
			args = append(args, season)
		}
	} else if n := qInt(r, "Season", -1); n >= 0 {
		where += " AND i.season_num=?"
		args = append(args, n)
	}
	items, _ := s.Store.QueryItems(c.Profile, where, args, "ORDER BY CASE WHEN i.season_num=0 THEN 1 ELSE 0 END, i.season_num, i.episode_num IS NULL, i.episode_num, i.sort_title")
	media := s.wantMedia(r)
	if media {
		items = s.withMedia(items, c)
	}
	list := s.dtos(items, c, dtoOpts{media: media})
	writeJSON(w, http.StatusOK, map[string]any{"Items": list, "TotalRecordCount": len(list), "StartIndex": 0})
}

func (s *Server) handleSearchHints(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	limit := qInt(r, "Limit", 20)
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	items, _ := s.Store.Search(c.Profile, qv(r, "SearchTerm"), limit)
	hints := []map[string]any{}
	for _, it := range items {
		h := map[string]any{"ItemId": s.itemID(it.ID), "Id": s.itemID(it.ID), "Name": it.Title, "Type": kindType[it.Kind],
			"IsFolder": it.Kind == "series" || it.Kind == "season", "MediaType": "Video"}
		if it.Year != nil {
			h["ProductionYear"] = *it.Year
		}
		if it.RuntimeSec > 0 {
			h["RunTimeTicks"] = ticks(float64(it.RuntimeSec))
		}
		if _, mod, ok := s.findImage(it, "Primary"); ok {
			h["PrimaryImageTag"] = imageTag(it.ID, mod)
		}
		if it.Kind == "episode" {
			h["Series"] = it.SeriesTitle
			if it.EpisodeNum != nil {
				h["IndexNumber"] = *it.EpisodeNum
			}
			if it.SeasonNum != nil {
				h["ParentIndexNumber"] = *it.SeasonNum
			}
		}
		hints = append(hints, h)
	}
	writeJSON(w, http.StatusOK, map[string]any{"SearchHints": hints, "TotalRecordCount": len(hints)})
}

func (s *Server) handleSimilar(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var items []*store.Item
	if id, ok := s.ids.decode(nsItem, r.PathValue("item")); ok {
		if ids, err := s.Store.Similar(c.Profile, id, max(qInt(r, "Limit", 12), 1)); err == nil {
			items, _ = s.Store.ItemsByIDs(c.Profile, ids)
		}
	}
	list := s.dtos(items, c, dtoOpts{})
	writeJSON(w, http.StatusOK, map[string]any{"Items": list, "TotalRecordCount": len(list), "StartIndex": 0})
}

func (s *Server) handleGenres(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var lib int64
	if pid := qv(r, "ParentId"); pid != "" {
		lib, _ = s.ids.decode(nsLibrary, pid)
	}
	genres, _, _ := s.Store.Facets(c.Profile, lib)
	out := []map[string]any{}
	for _, g := range genres {
		out = append(out, map[string]any{"Name": g.Value, "Type": "Genre", "ServerId": s.serverID(), "ChildCount": g.Count})
	}
	writeJSON(w, http.StatusOK, map[string]any{"Items": out, "TotalRecordCount": len(out), "StartIndex": 0})
}

// empty answers for features Kinora has no counterpart of, so clients do not treat them as errors
func emptyList(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	writeJSON(w, http.StatusOK, []any{})
}
func emptyResult(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	writeJSON(w, http.StatusOK, map[string]any{"Items": []any{}, "TotalRecordCount": 0, "StartIndex": 0})
}

// ------------------------------------------------------------------ images (no token: clients do not send one)

func (s *Server) handleImage(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	id, ok := s.ids.decode(nsItem, r.PathValue("item"))
	if !ok {
		writeErr(w, http.StatusNotFound, "no image")
		return
	}
	// the item may be hidden from some profiles; the id itself is the capability (it cannot be guessed)
	it, err := s.Store.Item(id, nil)
	if err != nil {
		writeErr(w, http.StatusNotFound, "no image")
		return
	}
	typ := r.PathValue("type")
	if idx := r.PathValue("index"); idx != "" && idx != "0" {
		writeErr(w, http.StatusNotFound, "no image") // Kinora keeps one image of each type
		return
	}
	p, _, ok := s.findImage(it, typ)
	if !ok {
		writeErr(w, http.StatusNotFound, "no image")
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, p)
}
