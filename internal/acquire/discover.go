package acquire

import (
	"context"

	"github.com/ScotyLucas/kinora/internal/meta"
	"github.com/ScotyLucas/kinora/internal/store"
)

// DiscoverItem is a TMDB film with what Kinora knows about it, for the "Felfedezés" page.
type DiscoverItem struct {
	meta.SearchResult
	Kind     string `json:"kind"`              // movie | series
	ItemID   int64  `json:"item_id,omitempty"` // already in a library: open this item
	Request  string `json:"request,omitempty"` // state of the newest request for it
	Followed bool   `json:"followed,omitempty"`
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

// MovieDetail is one film for the request dialog.
type MovieDetail struct {
	meta.MovieInfo
	ItemID  int64             `json:"item_id,omitempty"`
	Request *store.AcqRequest `json:"request,omitempty"` // the open request, or the newest closed one
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
	d := &MovieDetail{MovieInfo: mv}
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
	ItemID   int64             `json:"item_id,omitempty"`
	Request  *store.AcqRequest `json:"request,omitempty"`
	Followed bool              `json:"followed"`
	Seasons  []SeasonState     `json:"seasons"`
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
	d := &SeriesDetail{TVInfo: info, Seasons: []SeasonState{}}
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
