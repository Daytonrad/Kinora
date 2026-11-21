package store

import "strings"

// Watch history: what a profile has watched, by TMDB id. user_data belongs to a library
// item and goes with it; this stays when the file, the item or the whole library is gone,
// so the Felfedezés page can still say "you have seen this".

// Watched is a profile's history of one TMDB title.
type Watched struct {
	At       int64       `json:"at"`                 // last watched, unix seconds
	Episodes int         `json:"episodes,omitempty"` // series: distinct episodes watched
	Seasons  map[int]int `json:"seasons,omitempty"`  // series detail: watched episodes per season
}

// recordWatched writes a played film or episode into the history. Items with no TMDB id
// (and episodes of such series) are skipped: the live user_data covers them while present.
func recordWatched(q execer, profileID, itemID, at int64) {
	q.Exec(`INSERT INTO watch_history(profile_id,kind,tmdb_id,season,episode,watched_at)
		SELECT ?, 'movie', tmdb_id, 0, 0, ? FROM items WHERE id=? AND kind='movie' AND tmdb_id>0
		ON CONFLICT DO UPDATE SET watched_at=excluded.watched_at`, profileID, at, itemID)
	q.Exec(`INSERT INTO watch_history(profile_id,kind,tmdb_id,season,episode,watched_at)
		SELECT ?, 'series', s.tmdb_id, e.season_num, e.episode_num, ? FROM items e JOIN items s ON s.id=e.series_id
		WHERE e.id=? AND e.kind='episode' AND s.tmdb_id>0 AND e.season_num IS NOT NULL AND e.episode_num IS NOT NULL
		ON CONFLICT DO UPDATE SET watched_at=excluded.watched_at`, profileID, at, itemID)
}

// forgetWatched removes a film or episode from the history (marked unwatched).
func forgetWatched(q execer, profileID, itemID int64) {
	q.Exec(`DELETE FROM watch_history WHERE profile_id=? AND kind='movie' AND season=0 AND episode=0
		AND tmdb_id=(SELECT tmdb_id FROM items WHERE id=? AND kind='movie' AND tmdb_id>0)`, profileID, itemID)
	q.Exec(`DELETE FROM watch_history WHERE profile_id=? AND kind='series' AND EXISTS(
		SELECT 1 FROM items e JOIN items s ON s.id=e.series_id WHERE e.id=? AND e.kind='episode'
		AND s.tmdb_id=watch_history.tmdb_id AND e.season_num=watch_history.season AND e.episode_num=watch_history.episode)`, profileID, itemID)
}

func idArgs(ids []int64) (string, []any) {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return "?" + strings.Repeat(",?", len(ids)-1), args
}

// watchedRows is the history united with the live user_data, so a title whose TMDB id
// came after it was played still counts. Columns: tmdb_id, season, episode, at.
func watchedRows(kind, in string) string {
	if kind == "movie" {
		return `SELECT tmdb_id, 0 AS season, 0 AS episode, watched_at AS at FROM watch_history
			WHERE profile_id=? AND kind='movie' AND tmdb_id IN (` + in + `)
		UNION ALL
		SELECT i.tmdb_id, 0, 0, COALESCE(u.last_played_at,0) FROM user_data u JOIN items i ON i.id=u.item_id
			WHERE u.profile_id=? AND u.played=1 AND i.kind='movie' AND i.tmdb_id IN (` + in + `)`
	}
	return `SELECT tmdb_id, season, episode, watched_at AS at FROM watch_history
			WHERE profile_id=? AND kind='series' AND tmdb_id IN (` + in + `)
		UNION ALL
		SELECT s.tmdb_id, e.season_num, e.episode_num, COALESCE(u.last_played_at,0) FROM user_data u
			JOIN items e ON e.id=u.item_id JOIN items s ON s.id=e.series_id
			WHERE u.profile_id=? AND u.played=1 AND e.kind='episode' AND e.season_num IS NOT NULL
			AND e.episode_num IS NOT NULL AND s.tmdb_id IN (` + in + `)`
}

// WatchedTitles returns the profile's history of these TMDB titles (kind movie | series).
// With seasons, a series also carries its per-season episode counts.
func (s *Store) WatchedTitles(profileID int64, kind string, ids []int64, seasons bool) map[int64]*Watched {
	out := map[int64]*Watched{}
	if profileID == 0 || len(ids) == 0 {
		return out
	}
	in, args := idArgs(ids)
	all := append(append(append([]any{profileID}, args...), profileID), args...)
	rows, err := s.DB.Query(`SELECT tmdb_id, season, MAX(at), COUNT(DISTINCT episode) FROM (`+watchedRows(kind, in)+`)
		GROUP BY tmdb_id, season`, all...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id, at int64
		var season, eps int
		if rows.Scan(&id, &season, &at, &eps) != nil {
			continue
		}
		w := out[id]
		if w == nil {
			w = &Watched{}
			out[id] = w
		}
		if at > w.At {
			w.At = at
		}
		if kind == "series" {
			w.Episodes += eps
			if seasons {
				if w.Seasons == nil {
					w.Seasons = map[int]int{}
				}
				w.Seasons[season] = eps
			}
		}
	}
	return out
}
