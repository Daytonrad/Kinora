package store

import (
	"database/sql"
	"errors"
	"strings"
)

// AcqSeries is a series the acquisition module follows.
type AcqSeries struct {
	ID            int64  `json:"id"`
	TmdbID        int64  `json:"tmdb_id"`
	ImdbID        string `json:"imdb_id"`
	TVDB          int64  `json:"tvdb_id"`
	Title         string `json:"title"`
	OriginalTitle string `json:"original_title"`
	EnglishTitle  string `json:"english_title"`
	Year          int    `json:"year"`
	Poster        string `json:"poster"`
	LibraryID     *int64 `json:"library_id"`
	Monitor       string `json:"monitor"` // all | future | seasons | none
	Ended         bool   `json:"ended"`
	RefreshedAt   int64  `json:"refreshed_at"`
	AddedBy       *int64 `json:"added_by"`
	CreatedAt     int64  `json:"created_at"`
	// Have: every episode in the library; Wanted: followed, aired and missing
	Have        int    `json:"have"`
	Wanted      int    `json:"wanted"`
	Downloading int    `json:"downloading"`
	Total       int    `json:"total"`    // every episode Kinora knows of (all seasons, specials excluded)
	NextAir     string `json:"next_air"` // the next monitored episode's air date
}

// AcqEpisode is one episode of a followed series.
type AcqEpisode struct {
	Season     int    `json:"season"`
	Episode    int    `json:"episode"`
	Title      string `json:"title"`
	AirDate    string `json:"air_date"`
	Monitored  bool   `json:"monitored"`
	Have       bool   `json:"have"`
	GrabID     *int64 `json:"grab_id"`
	SearchedAt int64  `json:"searched_at"`
	State      string `json:"state"` // have | downloading | wanted | future | skipped
}

// EpisodeRow is an episode as TMDB lists it.
type EpisodeRow struct {
	Season, Episode int
	Title, AirDate  string
}

const activeGrab = `EXISTS(SELECT 1 FROM acq_grabs g WHERE g.id=e.grab_id AND g.status IN ('downloading','importing'))`

const seriesCols = `s.id,s.tmdb_id,s.imdb_id,s.tvdb_id,s.title,s.original_title,s.english_title,s.year,s.poster,s.library_id,s.monitor,s.ended,s.refreshed_at,s.added_by,s.created_at,
 (SELECT COUNT(*) FROM acq_episodes e WHERE e.series_id=s.id AND e.season>0 AND e.have=1),
 (SELECT COUNT(*) FROM acq_episodes e WHERE e.series_id=s.id AND e.season>0 AND e.monitored=1 AND e.have=0 AND e.air_date<>'' AND e.air_date<=date('now') AND NOT ` + activeGrab + `),
 (SELECT COUNT(*) FROM acq_episodes e WHERE e.series_id=s.id AND e.season>0 AND e.have=0 AND ` + activeGrab + `),
 (SELECT COUNT(*) FROM acq_episodes e WHERE e.series_id=s.id AND e.season>0),
 COALESCE((SELECT MIN(e.air_date) FROM acq_episodes e WHERE e.series_id=s.id AND e.monitored=1 AND e.air_date>date('now')),'')`

func scanSeries(row interface{ Scan(...any) error }) (*AcqSeries, error) {
	x := &AcqSeries{}
	var lib, by sql.NullInt64
	var ended int
	if err := row.Scan(&x.ID, &x.TmdbID, &x.ImdbID, &x.TVDB, &x.Title, &x.OriginalTitle, &x.EnglishTitle, &x.Year, &x.Poster, &lib, &x.Monitor, &ended,
		&x.RefreshedAt, &by, &x.CreatedAt, &x.Have, &x.Wanted, &x.Downloading, &x.Total, &x.NextAir); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if lib.Valid {
		v := lib.Int64
		x.LibraryID = &v
	}
	if by.Valid {
		v := by.Int64
		x.AddedBy = &v
	}
	x.Ended = ended == 1
	return x, nil
}

// AcqSeriesList lists the followed series by title.
func (s *Store) AcqSeriesList() ([]*AcqSeries, error) {
	rows, err := s.DB.Query(`SELECT ` + seriesCols + ` FROM acq_series s ORDER BY s.title COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*AcqSeries{}
	for rows.Next() {
		x, err := scanSeries(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// AcqSeriesByID loads one followed series.
func (s *Store) AcqSeriesByID(id int64) (*AcqSeries, error) {
	return scanSeries(s.DB.QueryRow(`SELECT `+seriesCols+` FROM acq_series s WHERE s.id=?`, id))
}

// AcqSeriesByTmdb loads a followed series by its TMDB id.
func (s *Store) AcqSeriesByTmdb(tmdbID int64) (*AcqSeries, error) {
	return scanSeries(s.DB.QueryRow(`SELECT `+seriesCols+` FROM acq_series s WHERE s.tmdb_id=?`, tmdbID))
}

// SaveAcqSeries inserts a series (ID 0) or updates its metadata, library and monitor mode.
func (s *Store) SaveAcqSeries(x *AcqSeries) error {
	if x.ID == 0 {
		x.CreatedAt = now()
		res, err := s.DB.Exec(`INSERT INTO acq_series(tmdb_id,imdb_id,tvdb_id,title,original_title,english_title,year,poster,library_id,monitor,ended,refreshed_at,added_by,created_at)
 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, x.TmdbID, x.ImdbID, x.TVDB, x.Title, x.OriginalTitle, x.EnglishTitle, x.Year, x.Poster, x.LibraryID, x.Monitor,
			b2i(x.Ended), x.RefreshedAt, x.AddedBy, x.CreatedAt)
		if err != nil {
			return err
		}
		x.ID, _ = res.LastInsertId()
		return nil
	}
	_, err := s.DB.Exec(`UPDATE acq_series SET imdb_id=?,tvdb_id=?,title=?,original_title=?,english_title=?,year=?,poster=?,library_id=?,monitor=?,ended=?,refreshed_at=? WHERE id=?`,
		x.ImdbID, x.TVDB, x.Title, x.OriginalTitle, x.EnglishTitle, x.Year, x.Poster, x.LibraryID, x.Monitor, b2i(x.Ended), x.RefreshedAt, x.ID)
	return err
}

// DeleteAcqSeries stops following a series (the files stay).
func (s *Store) DeleteAcqSeries(id int64) error {
	_, err := s.DB.Exec(`DELETE FROM acq_series WHERE id=?`, id)
	return err
}

// UpsertEpisodes stores the episode list; new episodes get monitored by monitorNew, known ones keep their flags.
func (s *Store) UpsertEpisodes(seriesID int64, eps []EpisodeRow, monitorNew func(season int, airDate string) bool) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, e := range eps {
		var n int
		tx.QueryRow(`SELECT COUNT(*) FROM acq_episodes WHERE series_id=? AND season=? AND episode=?`, seriesID, e.Season, e.Episode).Scan(&n)
		if n > 0 {
			if _, err := tx.Exec(`UPDATE acq_episodes SET title=?, air_date=? WHERE series_id=? AND season=? AND episode=?`, e.Title, e.AirDate, seriesID, e.Season, e.Episode); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.Exec(`INSERT INTO acq_episodes(series_id,season,episode,title,air_date,monitored) VALUES(?,?,?,?,?,?)`,
			seriesID, e.Season, e.Episode, e.Title, e.AirDate, b2i(monitorNew(e.Season, e.AirDate))); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Episodes lists a series' episodes with their state, by season and number.
func (s *Store) Episodes(seriesID int64) ([]*AcqEpisode, error) {
	rows, err := s.DB.Query(`SELECT e.season,e.episode,e.title,e.air_date,e.monitored,e.have,e.grab_id,e.searched_at,`+activeGrab+`, e.air_date<>'' AND e.air_date<=date('now')
 FROM acq_episodes e WHERE e.series_id=? ORDER BY e.season, e.episode`, seriesID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*AcqEpisode{}
	for rows.Next() {
		e := &AcqEpisode{}
		var mon, have, active, aired int
		var grab sql.NullInt64
		if err := rows.Scan(&e.Season, &e.Episode, &e.Title, &e.AirDate, &mon, &have, &grab, &e.SearchedAt, &active, &aired); err != nil {
			return nil, err
		}
		e.Monitored, e.Have = mon == 1, have == 1
		if grab.Valid {
			v := grab.Int64
			e.GrabID = &v
		}
		switch {
		case e.Have:
			e.State = "have"
		case active == 1:
			e.State = "downloading"
		case !e.Monitored:
			e.State = "skipped"
		case aired == 1:
			e.State = "wanted"
		default:
			e.State = "future"
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// SetSeasonMonitored turns monitoring of a season (or of every season with -1) on or off.
func (s *Store) SetSeasonMonitored(seriesID int64, season int, on bool) error {
	q := `UPDATE acq_episodes SET monitored=? WHERE series_id=? AND season>0`
	args := []any{b2i(on), seriesID}
	if season >= 0 {
		q += ` AND season=?`
		args = append(args, season)
	}
	_, err := s.DB.Exec(q, args...)
	return err
}

// WantedEpisodes are the monitored, aired episodes missing from the library with no running download,
// not searched since searchedBefore.
func (s *Store) WantedEpisodes(seriesID, searchedBefore int64) ([]*AcqEpisode, error) {
	all, err := s.Episodes(seriesID)
	if err != nil {
		return nil, err
	}
	out := []*AcqEpisode{}
	for _, e := range all {
		if e.State == "wanted" && e.Season > 0 && e.SearchedAt < searchedBefore {
			out = append(out, e)
		}
	}
	return out, nil
}

// MarkEpisodesSearched records a search for these episodes (season → episodes).
func (s *Store) MarkEpisodesSearched(seriesID int64, eps [][2]int, at int64) {
	for _, e := range eps {
		s.DB.Exec(`UPDATE acq_episodes SET searched_at=? WHERE series_id=? AND season=? AND episode=?`, at, seriesID, e[0], e[1])
	}
}

// ResetEpisodeSearch makes episodes due for searching again (after a failed download, or on demand).
func (s *Store) ResetEpisodeSearch(seriesID int64) {
	s.DB.Exec(`UPDATE acq_episodes SET searched_at=0 WHERE series_id=?`, seriesID)
}

// SetEpisodesGrab links episodes to the download fetching them.
func (s *Store) SetEpisodesGrab(seriesID int64, eps [][2]int, grabID int64) {
	for _, e := range eps {
		s.DB.Exec(`UPDATE acq_episodes SET grab_id=? WHERE series_id=? AND season=? AND episode=?`, grabID, seriesID, e[0], e[1])
	}
}

// MarkEpisodesHave records episodes now in the library.
func (s *Store) MarkEpisodesHave(seriesID int64, eps [][2]int) {
	for _, e := range eps {
		s.DB.Exec(`UPDATE acq_episodes SET have=1 WHERE series_id=? AND season=? AND episode=?`, seriesID, e[0], e[1])
	}
}

// LibraryEpisodes lists (season, episode) of a series found in the libraries: by its TMDB id, or before the
// metadata match by one of its titles (and year, when both have one).
func (s *Store) LibraryEpisodes(tmdbID int64, titles []string, year int) ([][2]int, error) {
	where := []string{"tmdb_id=?"}
	args := []any{tmdbID}
	for _, t := range titles {
		if t = strings.TrimSpace(t); t != "" {
			where = append(where, "(lower(title)=lower(?) AND (?=0 OR year IS NULL OR year=0 OR abs(year-?)<=1))")
			args = append(args, t, year, year)
		}
	}
	rows, err := s.DB.Query(`SELECT DISTINCT e.season_num, e.episode_num FROM items e
 WHERE e.kind='episode' AND e.missing=0 AND e.season_num IS NOT NULL AND e.episode_num IS NOT NULL
 AND e.series_id IN (SELECT id FROM items WHERE kind='series' AND missing=0 AND (`+strings.Join(where, " OR ")+`))`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][2]int
	for rows.Next() {
		var a, b int
		if rows.Scan(&a, &b) == nil {
			out = append(out, [2]int{a, b})
		}
	}
	return out, rows.Err()
}

// SeriesTmdbIDs returns which of these TMDB ids are series in a library, with an item id for each.
func (s *Store) SeriesTmdbIDs(ids []int64) (map[int64]int64, error) {
	out := map[int64]int64{}
	if len(ids) == 0 {
		return out, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.DB.Query(`SELECT tmdb_id, MIN(id) FROM items WHERE kind='series' AND missing=0 AND tmdb_id IN (?`+strings.Repeat(",?", len(ids)-1)+`) GROUP BY tmdb_id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, item int64
		if rows.Scan(&id, &item) == nil {
			out[id] = item
		}
	}
	return out, rows.Err()
}

// FollowedSeries returns which of these TMDB ids are followed series.
func (s *Store) FollowedSeries(ids []int64) map[int64]bool {
	out := map[int64]bool{}
	if len(ids) == 0 {
		return out
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.DB.Query(`SELECT tmdb_id FROM acq_series WHERE tmdb_id IN (?`+strings.Repeat(",?", len(ids)-1)+`)`, args...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			out[id] = true
		}
	}
	return out
}

// ResetSearchForGrab makes the episodes of a failed download due for a search again.
func (s *Store) ResetSearchForGrab(grabID int64) {
	s.DB.Exec(`UPDATE acq_episodes SET searched_at=0 WHERE grab_id=?`, grabID)
}
