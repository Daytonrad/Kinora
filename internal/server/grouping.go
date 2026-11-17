package server

import "github.com/ScotyLucas/kinora/internal/store"

// groupEpisodes replaces the episodes of a series that appears with two or more
// episodes in a list by one card of the series itself (at the position of its first
// episode); the series page then lists the episodes by season. Single episodes stay.
func (s *Server) groupEpisodes(p *store.Profile, items []*store.Item) []*store.Item {
	count := map[int64]int{}
	present := map[int64]bool{}
	for _, it := range items {
		if it.Kind == "episode" && it.SeriesID != nil {
			count[*it.SeriesID]++
		}
		if it.Kind == "series" {
			present[it.ID] = true
		}
	}
	out := make([]*store.Item, 0, len(items))
	for _, it := range items {
		if it.Kind != "episode" || it.SeriesID == nil || count[*it.SeriesID] < 2 {
			out = append(out, it)
			continue
		}
		sid := *it.SeriesID
		if present[sid] {
			continue // the series card is already in the list (or was added for an earlier episode)
		}
		series, err := s.st.Item(sid, p)
		if err != nil {
			out = append(out, it)
			continue
		}
		present[sid] = true
		out = append(out, series)
	}
	return out
}
