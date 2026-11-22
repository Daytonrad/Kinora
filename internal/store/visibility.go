package store

import (
	"fmt"
	"strconv"
	"strings"
)

// RatingSQL returns a WHERE fragment (starting with " AND") that hides items
// the profile may not see: those above its rating limit and those outside the
// account's allowed libraries. alias is the items table alias.
// Kids profiles never see items with unknown rating. Every query that exposes
// items (lists, details, search, recommendations, collections, streams,
// subtitles, downloads, parties, images) must include it.
func RatingSQL(p *Profile, alias string) (string, []any) {
	frag := LibrarySQL(p, alias)
	limit, strict := p.RatingLimit()
	if limit < 0 {
		return frag, nil
	}
	if strict {
		return frag + fmt.Sprintf(" AND %[1]s.rating_level IS NOT NULL AND %[1]s.rating_level <= ?", alias), []any{limit}
	}
	return frag + fmt.Sprintf(" AND (%[1]s.rating_level IS NULL OR %[1]s.rating_level <= ?)", alias), []any{limit}
}

// LibrarySQL returns " AND alias.library_id IN (…)" for a profile limited to some
// libraries, "" otherwise. The ids are integers, so they are inlined.
func LibrarySQL(p *Profile, alias string) string {
	if p == nil || p.Libraries == nil {
		return ""
	}
	if len(p.Libraries) == 0 {
		return " AND 0"
	}
	ids := make([]string, len(p.Libraries))
	for i, id := range p.Libraries {
		ids[i] = strconv.FormatInt(id, 10)
	}
	return fmt.Sprintf(" AND %s.library_id IN (%s)", alias, strings.Join(ids, ","))
}

// LibraryAllowed reports whether a profile may see a library.
func (p *Profile) LibraryAllowed(id int64) bool {
	if p == nil || p.Libraries == nil {
		return true
	}
	for _, l := range p.Libraries {
		if l == id {
			return true
		}
	}
	return false
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
