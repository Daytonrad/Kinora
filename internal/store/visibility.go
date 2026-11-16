package store

import "fmt"

// RatingSQL returns a WHERE fragment (starting with " AND") that hides items
// above the profile's rating limit. alias is the items table alias.
// Kids profiles never see items with unknown rating. Every query that exposes
// items (lists, details, search, recommendations, collections, streams,
// subtitles, downloads, parties, images) must include it.
func RatingSQL(p *Profile, alias string) (string, []any) {
	limit, strict := p.RatingLimit()
	if limit < 0 {
		return "", nil
	}
	if strict {
		return fmt.Sprintf(" AND %[1]s.rating_level IS NOT NULL AND %[1]s.rating_level <= ?", alias), []any{limit}
	}
	return fmt.Sprintf(" AND (%[1]s.rating_level IS NULL OR %[1]s.rating_level <= ?)", alias), []any{limit}
}

// ItemVisible reports whether a profile may see an item.
func (s *Store) ItemVisible(id int64, p *Profile) bool {
	frag, args := RatingSQL(p, "i")
	var n int
	s.DB.QueryRow(`SELECT COUNT(*) FROM items i WHERE i.id=?`+frag, append([]any{id}, args...)...).Scan(&n)
	return n > 0
}

// ItemKindSeries returns an item's kind and series id (0 if none).
func (s *Store) ItemKindSeries(id int64) (string, int64) {
	var k string
	var sid *int64
	s.DB.QueryRow(`SELECT kind, series_id FROM items WHERE id=?`, id).Scan(&k, &sid)
	if sid == nil {
		return k, 0
	}
	return k, *sid
}

// FirstEpisode returns the first present episode of a series or season (0 if none).
func (s *Store) FirstEpisode(containerID int64) int64 {
	var id int64
	s.DB.QueryRow(`SELECT id FROM items WHERE kind='episode' AND missing=0 AND (series_id=? OR parent_id=?)
		ORDER BY CASE WHEN season_num=0 THEN 1 ELSE 0 END, season_num, episode_num LIMIT 1`, containerID, containerID).Scan(&id)
	return id
}
