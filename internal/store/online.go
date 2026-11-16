package store

import "strings"

// OnlineMeta is metadata fetched from an online source (TMDB).
type OnlineMeta struct {
	TmdbID        int64
	Title         string
	Year          int
	Overview      string
	ContentRating string
	Rating        *float64
	Genres        []string
	Tags          []string
}

// SetTmdbID stores a TMDB id (0 = searched, no match).
func (s *Store) SetTmdbID(id, tmdb int64) error {
	_, err := s.DB.Exec(`UPDATE items SET tmdb_id=? WHERE id=?`, tmdb, id)
	return err
}

// ApplyOnline writes online metadata to a movie or series. Local (NFO / manual)
// items are protected because the fetcher only selects tmdb_id NULL/0 – unless
// the admin explicitly applies a match.
func (s *Store) ApplyOnline(id int64, m OnlineMeta) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	set := []string{"tmdb_id=?", "updated_at=?"}
	args := []any{m.TmdbID, now()}
	if m.Title != "" {
		set = append(set, "title=?", "sort_title=?")
		args = append(args, m.Title, sortTitleSimple(m.Title))
	}
	if m.Year > 0 {
		set = append(set, "year=?")
		args = append(args, m.Year)
	}
	if m.Overview != "" {
		set = append(set, "overview=?")
		args = append(args, m.Overview)
	}
	if m.Rating != nil {
		set = append(set, "community_rating=?")
		args = append(args, *m.Rating)
	}
	if m.ContentRating != "" {
		lvl, ok := RatingLevel(m.ContentRating)
		var lv any
		if ok {
			lv = lvl
		}
		set = append(set, "content_rating=?", "rating_level=?")
		args = append(args, m.ContentRating, lv)
	}
	if _, err := tx.Exec(`UPDATE items SET `+strings.Join(set, ",")+` WHERE id=?`, append(args, id)...); err != nil {
		return err
	}
	if len(m.Genres) > 0 {
		if _, err := tx.Exec(`DELETE FROM item_genres WHERE item_id=?`, id); err != nil {
			return err
		}
		for _, g := range m.Genres {
			if _, err := tx.Exec(`INSERT OR IGNORE INTO item_genres(item_id,genre) VALUES(?,?)`, id, g); err != nil {
				return err
			}
		}
	}
	for _, t := range m.Tags {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO item_tags(item_id,tag) VALUES(?,?)`, id, t); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// sortTitleSimple mirrors scanner.SortTitle (store cannot import scanner).
func sortTitleSimple(t string) string {
	s := strings.ToLower(strings.TrimSpace(t))
	for _, a := range []string{"the ", "a ", "an ", "az ", "egy "} {
		if strings.HasPrefix(s, a) && len(s) > len(a) {
			return s[len(a):]
		}
	}
	return s
}

// SeasonRef is a season id and number.
type SeasonRef struct {
	ID  int64
	Num int
}

// SeasonsOf lists the seasons of a series.
func (s *Store) SeasonsOf(seriesID int64) ([]SeasonRef, error) {
	rows, err := s.DB.Query(`SELECT id, season_num FROM items WHERE parent_id=? AND kind='season' AND season_num IS NOT NULL`, seriesID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SeasonRef
	for rows.Next() {
		var r SeasonRef
		rows.Scan(&r.ID, &r.Num)
		out = append(out, r)
	}
	return out, rows.Err()
}

// EpisodeID finds an episode by series/season/episode number.
func (s *Store) EpisodeID(seriesID int64, season, episode int) int64 {
	var id int64
	s.DB.QueryRow(`SELECT id FROM items WHERE series_id=? AND kind='episode' AND season_num=? AND episode_num=?`, seriesID, season, episode).Scan(&id)
	return id
}

// ApplyEpisode sets an episode's online title/overview unless it has local metadata (tmdb_id -1).
func (s *Store) ApplyEpisode(id, tmdb int64, title, overview string) {
	set := []string{"tmdb_id=?"}
	args := []any{tmdb}
	if title != "" {
		set = append(set, "title=?", "sort_title=?")
		args = append(args, title, sortTitleSimple(title))
	}
	if overview != "" {
		set = append(set, "overview=?")
		args = append(args, overview)
	}
	s.DB.Exec(`UPDATE items SET `+strings.Join(set, ",")+` WHERE id=? AND (tmdb_id IS NULL OR tmdb_id>=0)`, append(args, id)...)
}
