package acquire

import (
	"context"

	"github.com/ScotyLucas/kinora/internal/meta"
	"github.com/ScotyLucas/kinora/internal/store"
)

// DiscoverItem is a TMDB film with what Kinora knows about it, for the "Felfedezés" page.
type DiscoverItem struct {
	meta.SearchResult
	Kind     string         `json:"kind"`              // movie | series
	ItemID   int64          `json:"item_id,omitempty"` // already in a library: open this item
	Request  string         `json:"request,omitempty"` // state of the newest request for it
	Followed bool           `json:"followed,omitempty"`
	Watched  *store.Watched `json:"watched,omitempty"` // the profile has seen it (even if no longer in the library)
}

// MarkWatched adds the profile's watch history to discover results.
func (m *Manager) MarkWatched(profileID int64, items []DiscoverItem) {
	ids := map[string][]int64{}
	for _, it := range items {
		ids[it.Kind] = append(ids[it.Kind], it.ID)
	}
	seen := map[string]map[int64]*store.Watched{}
	for kind, list := range ids {
		seen[kind] = m.st.WatchedTitles(profileID, kind, list, false)
	}
	for i := range items {
		items[i].Watched = seen[items[i].Kind][items[i].ID]
	}
}

// WatchedTitle is the profile's history of one TMDB title (series with per-season counts).
func (m *Manager) WatchedTitle(profileID int64, kind string, id int64) *store.Watched {
	return m.st.WatchedTitles(profileID, kind, []int64{id}, kind == "series")[id]
}

func (m *Manager) mark(res []meta.SearchResult) []DiscoverItem {
	ids := make([]int64, len(res))
	for i, r := range res {
		ids[i] = r.ID
	}
	have, _ := m.st.MovieTmdbIDs(ids)
	reqs, _ := m.st.RequestStates("movie", ids)
	out := make([]DiscoverItem, len(res))
	for i, r := range res {
		out[i] = DiscoverItem{SearchResult: r, Kind: "movie", ItemID: have[r.ID], Request: reqs[r.ID]}
	}
	return out
}

func (m *Manager) markSeries(res []meta.SearchResult) []DiscoverItem {
	ids := make([]int64, len(res))
	for i, r := range res {
		ids[i] = r.ID
	}
	have, _ := m.st.SeriesTmdbIDs(ids)
	reqs, _ := m.st.RequestStates("series", ids)
	fol := m.st.FollowedSeries(ids)
	out := make([]DiscoverItem, len(res))
	for i, r := range res {
		out[i] = DiscoverItem{SearchResult: r, Kind: "series", ItemID: have[r.ID], Request: reqs[r.ID], Followed: fol[r.ID]}
	}
	return out
}

// Discover returns a TMDB film list (trending, popular, upcoming, top).
func (m *Manager) Discover(ctx context.Context, list string) ([]DiscoverItem, error) {
	if !m.Enabled() {
		return nil, ErrDisabled
	}
	if _, tv := meta.TVLists[list]; tv {
		res, err := m.movies.TVList(ctx, list, 1)
		if err != nil {
			return nil, err
		}
		return m.markSeries(res), nil
	}
	res, err := m.movies.MovieList(ctx, list, 1)
	if err != nil {
		return nil, err
	}
	return m.mark(res), nil
}

// DiscoverSearch searches TMDB for films or series (kind "series").
func (m *Manager) DiscoverSearch(ctx context.Context, q, kind string) ([]DiscoverItem, error) {
	if !m.Enabled() {
		return nil, ErrDisabled
	}
	if kind == "series" {
		res, err := m.movies.Search(ctx, "series", q, 0)
		if err != nil {
			return nil, err
		}
		return m.markSeries(res), nil
	}
	res, err := m.movies.Search(ctx, "movie", q, 0)
	if err != nil {
		return nil, err
	}
	return m.mark(res), nil
}

// DiscoverBrowse is one page of a filtered listing (streaming service, genre, year, …).
type DiscoverBrowse struct {
	Results    []DiscoverItem `json:"results"`
	Page       int            `json:"page"`
	TotalPages int            `json:"total_pages"`
	Total      int            `json:"total"`
}

// Browse lists films or series by the filter, marked like the other lists.
func (m *Manager) Browse(ctx context.Context, b meta.BrowseFilter) (DiscoverBrowse, error) {
	if !m.Enabled() {
		return DiscoverBrowse{}, ErrDisabled
	}
	p, err := m.movies.Browse(ctx, b)
	if err != nil {
		return DiscoverBrowse{}, err
	}
	out := DiscoverBrowse{Page: p.Page, TotalPages: p.TotalPages, Total: p.Total}
	if b.Kind == "series" {
		out.Results = m.markSeries(p.Results)
	} else {
		out.Results = m.mark(p.Results)
	}
	return out, nil
}

// DiscoverOptions is what the filter bar offers: the streaming services of the region
// and the genres, both for films and series.
type DiscoverOptions struct {
	Region       string               `json:"region"`
	Providers    []meta.WatchProvider `json:"providers"`
	MovieGenres  []meta.Genre         `json:"movie_genres"`
	SeriesGenres []meta.Genre         `json:"series_genres"`
}

// Options gathers the filter choices. The service list is the films one followed by the
// series-only services (Apple TV+ and the like are on both).
func (m *Manager) Options(ctx context.Context) (DiscoverOptions, error) {
	if !m.Enabled() {
		return DiscoverOptions{}, ErrDisabled
	}
	o := DiscoverOptions{Region: m.movies.WatchRegion(), Providers: []meta.WatchProvider{}, MovieGenres: []meta.Genre{}, SeriesGenres: []meta.Genre{}}
	pm, err := m.movies.WatchProviders(ctx, "movie")
	if err != nil {
		return o, err
	}
	pt, _ := m.movies.WatchProviders(ctx, "series")
	seen := map[int]bool{}
	for _, p := range append(pm, pt...) {
		if !seen[p.ID] {
			seen[p.ID] = true
			o.Providers = append(o.Providers, p)
		}
	}
	if g, err := m.movies.Genres(ctx, "movie"); err == nil {
		o.MovieGenres = g
	}
	if g, err := m.movies.Genres(ctx, "series"); err == nil {
		o.SeriesGenres = g
	}
	return o, nil
}

// whereToWatch is best effort: a detail page still opens when TMDB has no answer.
func (m *Manager) whereToWatch(ctx context.Context, kind string, id int64) *meta.WhereToWatch {
	w, err := m.movies.TitleProviders(ctx, kind, id)
	if err != nil {
		return nil
	}
	return &w
}

// MovieDetail is one film for the request dialog.
type MovieDetail struct {
	meta.MovieInfo
	Watch   *meta.WhereToWatch `json:"watch,omitempty"`
	ItemID  int64              `json:"item_id,omitempty"`
	Request *store.AcqRequest  `json:"request,omitempty"` // the open request, or the newest closed one
	Watched *store.Watched     `json:"watched,omitempty"`
}

// DiscoverMovie returns a film with its library and request state.
func (m *Manager) DiscoverMovie(ctx context.Context, id int64) (*MovieDetail, error) {
	if !m.Enabled() {
		return nil, ErrDisabled
	}
	mv, err := m.movies.Movie(ctx, id)
	if err != nil {
		return nil, err
	}
	d := &MovieDetail{MovieInfo: mv, Watch: m.whereToWatch(ctx, "movie", id)}
	have, _ := m.st.MovieTmdbIDs([]int64{id})
	d.ItemID = have[id]
	if q, err := m.st.OpenRequestFor("movie", id); err == nil {
		d.Request = q
	}
	return d, nil
}

// SeasonState is how much of a season is in the library.
type SeasonState struct {
	meta.TVSeason
	Have int `json:"have"`
}

// SeriesDetail is one series for the request dialog.
type SeriesDetail struct {
	meta.TVInfo
	Watch    *meta.WhereToWatch `json:"watch,omitempty"`
	ItemID   int64              `json:"item_id,omitempty"`
	Request  *store.AcqRequest  `json:"request,omitempty"`
	Followed bool               `json:"followed"`
	Seasons  []SeasonState      `json:"seasons"`
	Watched  *store.Watched     `json:"watched,omitempty"`
}

// DiscoverSeries returns a series with its seasons and how many episodes of each are in the library.
func (m *Manager) DiscoverSeries(ctx context.Context, id int64) (*SeriesDetail, error) {
	if !m.Enabled() {
		return nil, ErrDisabled
	}
	info, err := m.movies.TV(ctx, id)
	if err != nil {
		return nil, err
	}
	d := &SeriesDetail{TVInfo: info, Watch: m.whereToWatch(ctx, "series", id), Seasons: []SeasonState{}}
	have, _ := m.st.SeriesTmdbIDs([]int64{id})
	d.ItemID = have[id]
	d.Followed = m.st.FollowedSeries([]int64{id})[id]
	if q, err := m.st.OpenRequestFor("series", id); err == nil {
		d.Request = q
	}
	eps, _ := m.st.LibraryEpisodes(id, []string{info.Title, info.OriginalTitle, info.EnglishTitle}, info.Year)
	per := map[int]int{}
	for _, e := range eps {
		per[e[0]]++
	}
	for _, s := range info.Seasons {
		if s.Number > 0 {
			d.Seasons = append(d.Seasons, SeasonState{TVSeason: s, Have: per[s.Number]})
		}
	}
	return d, nil
}

// LookupSeries searches TMDB for series (admin): in the library, followed.
func (m *Manager) LookupSeries(ctx context.Context, q string) ([]DiscoverItem, error) {
	res, err := m.movies.Search(ctx, "series", q, 0)
	if err != nil {
		return nil, err
	}
	return m.markSeries(res), nil
}
