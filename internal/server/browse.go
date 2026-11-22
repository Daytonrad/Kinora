package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/ScotyLucas/kinora/internal/store"
)

// ------------------------------------------------------------------ home

type homeRow struct {
	ID     string        `json:"id"`
	Title  string        `json:"title"`
	Style  string        `json:"style"` // landscape | poster
	Items  []*store.Item `json:"items"`
	More   string        `json:"more,omitempty"`
	Reason map[int64]string `json:"reasons,omitempty"`
	HideAs string        `json:"hide_as,omitempty"` // "series": hiding hides the whole series
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	p := c.Profile
	rows := []homeRow{}
	if cw, err := s.st.ContinueWatching(p, 20); err == nil && len(cw) > 0 {
		rows = append(rows, homeRow{ID: "continue", Title: "Folytatás", Style: "landscape", Items: cw})
	}
	if nu, err := s.st.NextUp(p, 20); err == nil && len(nu) > 0 {
		rows = append(rows, homeRow{ID: "nextup", Title: "Következő rész", Style: "landscape", Items: nu, HideAs: "series"})
	}
	if cols, err := s.st.Collections(p); err == nil {
		for _, col := range cols {
			if !col.Pinned {
				continue
			}
			items, _, err := s.st.CollectionItems(p, col, 0, 20)
			if err == nil && len(items) > 0 {
				rows = append(rows, homeRow{ID: fmt.Sprintf("col-%d", col.ID), Title: col.Name, Style: "poster", Items: items,
					More: fmt.Sprintf("#/collections/%d", col.ID)})
			}
		}
	}
	var hero *store.Item
	if rec, err := s.st.Recommend(p); err == nil {
		addRec := func(id, title string, recs []store.Recommendation, more string) {
			ids := make([]int64, 0, len(recs))
			reasons := map[int64]string{}
			for _, r := range recs {
				ids = append(ids, r.ID)
				if r.Reason != "" {
					reasons[r.ID] = r.Reason
				}
			}
			items, err := s.st.ItemsByIDs(p, ids)
			if err == nil && len(items) > 0 {
				rows = append(rows, homeRow{ID: id, Title: title, Style: "poster", Items: items, Reason: reasons, More: more})
				if hero == nil {
					hero = items[0]
				}
			}
		}
		addRec("foryou", "Neked ajánljuk", rec.ForYou, "")
		for i, seed := range rec.BecauseTitle {
			if it, err := s.st.Item(seed, p); err == nil {
				addRec(fmt.Sprintf("because-%d", seed), "Mert megnézted: "+it.Title, rec.Because[i], fmt.Sprintf("#/item/%d", seed))
			}
		}
		addRec("gems", "Rejtett kincsek", rec.HiddenGems, "")
	} else {
		s.log.Warn("recommend", "err", err)
	}
	if libs, err := s.visibleLibraries(c.Profile); err == nil {
		for _, l := range libs {
			items, err := s.st.RecentlyAdded(p, l.ID, 20)
			if err == nil && len(items) > 0 {
				rows = append(rows, homeRow{ID: fmt.Sprintf("new-%d", l.ID), Title: "Új: " + l.Name, Style: "poster", Items: items,
					More: fmt.Sprintf("#/library/%d?sort=added&order=desc", l.ID)})
				if hero == nil {
					hero = items[0]
				}
			}
		}
	}
	if hero != nil {
		if full, err := s.st.Item(hero.ID, p); err == nil {
			s.st.FillGenresTags([]*store.Item{full}, p)
			hero = full
		}
	}
	// rows of titles (not "Folytatás"/"Következő rész", where the episode itself matters):
	// several episodes of one series become one series card
	for i := range rows {
		if rows[i].ID != "continue" && rows[i].ID != "nextup" {
			rows[i].Items = s.groupEpisodes(p, rows[i].Items)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"hero": hero, "rows": rows})
}

// ------------------------------------------------------------------ items

// rulesFromQuery builds a RuleSet from ?rules=JSON or simple parameters.
func rulesFromQuery(r *http.Request) (store.RuleSet, error) {
	q := r.URL.Query()
	if raw := q.Get("rules"); raw != "" {
		return store.ParseRuleSet(raw)
	}
	rs := store.RuleSet{Match: "all", Sort: q.Get("sort"), Order: q.Get("order")}
	add := func(field, op string, v any) { rs.Rules = append(rs.Rules, store.Rule{Field: field, Op: op, Value: v}) }
	if v := q.Get("library"); v != "" {
		add("library", "is", v)
	}
	if v := q.Get("kind"); v != "" {
		add("kind", "is", v)
	}
	for _, g := range q["genre"] {
		add("genre", "is", g)
	}
	for _, t := range q["tag"] {
		add("tag", "is", t)
	}
	if v := strings.TrimSpace(q.Get("q")); v != "" {
		add("title", "contains", v)
	}
	if v := q.Get("played"); v != "" {
		add("played", "is", v == "true" || v == "1")
	}
	if v := q.Get("favorite"); v == "true" || v == "1" {
		add("favorite", "is", true)
	}
	if v := q.Get("year_from"); v != "" {
		add("year", "gte", v)
	}
	if v := q.Get("year_to"); v != "" {
		add("year", "lte", v)
	}
	if v := q.Get("min_rating"); v != "" {
		add("community_rating", "gte", v)
	}
	// validate
	b := rs
	if _, _, _, err := b.Compile(0); err != nil {
		return rs, err
	}
	return rs, nil
}

func (s *Server) handleItems(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	rs, err := rulesFromQuery(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	items, total, err := s.st.Browse(c.Profile, rs, queryInt(r, "offset", 0), queryInt(r, "limit", 60))
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (s *Server) handleItem(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	p := c.Profile
	it, err := s.st.ItemFull(pathID(r, "id"), p)
	if err != nil {
		writeErr(w, http.StatusNotFound, "az elem nem található")
		return
	}
	resp := map[string]any{"item": it}
	switch it.Kind {
	case "series":
		seasons, _ := s.st.Children(p, it)
		resp["seasons"] = seasons
		if pn, _ := s.st.PlayNext(p, it.ID); pn != nil {
			resp["play_next"] = pn
		}
	case "season":
		eps, _ := s.st.Children(p, it)
		resp["episodes"] = eps
		if it.SeriesID != nil {
			if pn, _ := s.st.PlayNext(p, *it.SeriesID); pn != nil {
				resp["play_next"] = pn
			}
		}
	case "episode":
		if n, _ := s.st.NextEpisode(p, it); n != nil {
			resp["next"] = n
		}
	case "movie":
		if vs := s.versionsOf(p, it); vs != nil {
			resp["versions"] = vs
			pref := s.st.VersionPref(p.ID, it.ImdbID)
			for _, v := range vs {
				if v.ID == pref {
					resp["preferred_version"] = pref
				}
			}
		}
	}
	simID := it.ID
	if it.Kind != "movie" && it.SeriesID != nil {
		simID = *it.SeriesID
	}
	resp["collections"] = s.st.CollectionsOf(p, simID)
	if ids, err := s.st.Similar(p, simID, 16); err == nil {
		sim, _ := s.st.ItemsByIDs(p, ids)
		resp["similar"] = sim
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleChildren(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	it, err := s.st.Item(pathID(r, "id"), c.Profile)
	if err != nil {
		writeErr(w, http.StatusNotFound, "az elem nem található")
		return
	}
	items, err := s.st.Children(c.Profile, it)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleNext(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	it, err := s.st.Item(pathID(r, "id"), c.Profile)
	if err != nil {
		writeErr(w, http.StatusNotFound, "az elem nem található")
		return
	}
	n, err := s.st.NextEpisode(c.Profile, it)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"next": n})
}

// handleItemAction handles played / favorite / stars / hide / progress.
func (s *Server) handleItemAction(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	action := r.PathValue("action")
	var in struct {
		Played   *bool    `json:"played"`
		Favorite *bool    `json:"favorite"`
		Hidden   *bool    `json:"hidden"`
		Stars    *int     `json:"stars"`
		Position *float64 `json:"position"`
		Ended    bool     `json:"ended"`
	}
	if !decode(w, r, &in) {
		return
	}
	it, err := s.st.Item(pathID(r, "id"), c.Profile)
	if err != nil {
		writeErr(w, http.StatusNotFound, "az elem nem található")
		return
	}
	pid := c.Profile.ID
	switch action {
	case "played":
		v := in.Played == nil || *in.Played
		err = s.st.SetPlayed(pid, it, v)
	case "favorite":
		err = s.st.SetFlag(pid, it.ID, "favorite", in.Favorite == nil || *in.Favorite)
	case "hide":
		err = s.st.SetFlag(pid, it.ID, "hidden", in.Hidden == nil || *in.Hidden)
	case "stars":
		if in.Stars != nil && (*in.Stars < 1 || *in.Stars > 5) {
			writeErr(w, http.StatusBadRequest, "a csillag 1–5 lehet")
			return
		}
		err = s.st.SetStars(pid, it.ID, in.Stars)
	case "progress":
		if it.Kind != "movie" && it.Kind != "episode" {
			writeErr(w, http.StatusBadRequest, "csak filmre és epizódra jelenthető haladás")
			return
		}
		pos := 0.0
		if in.Position != nil {
			pos = *in.Position
		}
		var played bool
		played, err = s.st.ReportProgress(pid, it, pos, in.Ended)
		if err == nil {
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "played": played})
			return
		}
	default:
		writeErr(w, http.StatusNotFound, "ismeretlen művelet")
		return
	}
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.st.GetUserData(pid, it.ID))
}

func (s *Server) handleTags(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var in struct {
		Tags []string `json:"tags"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.Tags) > 50 {
		writeErr(w, http.StatusBadRequest, "legfeljebb 50 címke adható meg")
		return
	}
	it, err := s.st.Item(pathID(r, "id"), c.Profile)
	if err != nil {
		writeErr(w, http.StatusNotFound, "az elem nem található")
		return
	}
	tags, err := s.st.SetTags(it.ID, in.Tags)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tags": tags})
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	items, err := s.st.Search(c.Profile, r.URL.Query().Get("q"), 60)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if r.URL.Query().Get("group") == "1" { // clients showing cards: one card per series
		items = s.groupEpisodes(c.Profile, items)
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleFacets(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	lib, _ := strconv.ParseInt(r.URL.Query().Get("library"), 10, 64)
	genres, tags, err := s.st.Facets(c.Profile, lib)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"genres": genres, "tags": tags, "fields": store.Fields, "sorts": store.Sorts})
}

// ------------------------------------------------------------------ collections

func (s *Server) handleCollections(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	cs, err := s.st.Collections(c.Profile)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, cs)
}

func (s *Server) handleCollectionCreate(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var in store.CollectionInput
	if !decode(w, r, &in) {
		return
	}
	col, err := s.st.CreateCollection(c.Profile, in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, col)
}

func (s *Server) loadCollection(w http.ResponseWriter, r *http.Request, c *reqCtx) *store.Collection {
	col, err := s.st.Collection(c.Profile, pathID(r, "id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "a gyűjtemény nem található")
		return nil
	}
	return col
}

func (s *Server) handleCollection(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	col := s.loadCollection(w, r, c)
	if col == nil {
		return
	}
	items, total, err := s.st.CollectionItems(c.Profile, col, queryInt(r, "offset", 0), queryInt(r, "limit", 100))
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	col.Count = total
	writeJSON(w, http.StatusOK, map[string]any{"collection": col, "items": items, "total": total})
}

func (s *Server) handleCollectionUpdate(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	col := s.loadCollection(w, r, c)
	if col == nil {
		return
	}
	var in store.CollectionInput
	if !decode(w, r, &in) {
		return
	}
	if err := s.st.UpdateCollection(c.Profile, col, in); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	col, _ = s.st.Collection(c.Profile, col.ID)
	writeJSON(w, http.StatusOK, col)
}

func (s *Server) handleCollectionDelete(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	col := s.loadCollection(w, r, c)
	if col == nil {
		return
	}
	if err := s.st.DeleteCollection(col); err != nil {
		s.internalErr(w, r, err)
		return
	}
	ok(w)
}

func (s *Server) handleCollectionPreview(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var rs store.RuleSet
	if !decode(w, r, &rs) {
		return
	}
	if rs.Match != "any" {
		rs.Match = "all"
	}
	items, total, err := s.st.Browse(c.Profile, rs, 0, 24)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (s *Server) handleCollectionItem(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	col := s.loadCollection(w, r, c)
	if col == nil {
		return
	}
	itemID := pathID(r, "item")
	if _, err := s.st.Item(itemID, c.Profile); err != nil {
		writeErr(w, http.StatusNotFound, "az elem nem található")
		return
	}
	var err error
	if r.Method == http.MethodDelete {
		err = s.st.RemoveFromCollection(col, itemID)
	} else {
		err = s.st.AddToCollection(col, itemID)
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	ok(w)
}

func (s *Server) handleCollectionOrder(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	col := s.loadCollection(w, r, c)
	if col == nil {
		return
	}
	var in struct {
		Items []int64 `json:"items"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := s.st.ReorderCollection(col, in.Items); err != nil {
		s.internalErr(w, r, err)
		return
	}
	ok(w)
}
