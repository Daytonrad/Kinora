package store

import (
	"regexp"
	"strings"
)

// Versions of a film: every movie file with the same IMDb id is one film. Lists show
// one card per film and library (the file with the lowest id), the detail page offers
// the versions, and the watch state (position, played, favorite, stars, hidden) is
// shared between them.

// NotAltVersion hides the other versions of a film in item lists (append to a where).
const NotAltVersion = ` AND NOT (i.kind='movie' AND i.imdb_id<>'' AND EXISTS(SELECT 1 FROM items v
 WHERE v.imdb_id=i.imdb_id AND v.kind='movie' AND v.library_id=i.library_id AND v.missing=0 AND v.id<i.id))`

var reImdb = regexp.MustCompile(`(?i)\btt(\d{7,10})\b`)

// NormalizeImdbID returns "tt0133093" from "tt0133093", "0133093", "133093" or a URL; "" when it is not one.
func NormalizeImdbID(s string) string {
	s = strings.TrimSpace(s)
	if m := reImdb.FindStringSubmatch(s); m != nil {
		return "tt" + m[1]
	}
	if s != "" && len(s) <= 10 && strings.Trim(s, "0123456789") == "" {
		for len(s) < 7 {
			s = "0" + s
		}
		return "tt" + s
	}
	return ""
}

// SetImdbID stores the IMDb id of a movie and merges the watch state of its versions.
func (s *Store) SetImdbID(id int64, imdb string) error {
	imdb = NormalizeImdbID(imdb)
	if imdb == "" {
		return nil
	}
	var old string
	s.DB.QueryRow(`SELECT imdb_id FROM items WHERE id=?`, id).Scan(&old)
	if old == imdb {
		return nil
	}
	if _, err := s.DB.Exec(`UPDATE items SET imdb_id=? WHERE id=? AND kind='movie'`, imdb, id); err != nil {
		return err
	}
	return s.mergeVersionData(imdb)
}

// mergeVersionData gives every version of a film the same watch state per profile: the
// most recently played version's position and played flag, any favorite, the best stars.
func (s *Store) mergeVersionData(imdb string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT DISTINCT ud.profile_id FROM user_data ud JOIN items i ON i.id=ud.item_id WHERE i.imdb_id=? AND i.kind='movie'`, imdb)
	if err != nil {
		return err
	}
	var profiles []int64
	for rows.Next() {
		var p int64
		rows.Scan(&p)
		profiles = append(profiles, p)
	}
	rows.Close()
	for _, pid := range profiles {
		var lead int64
		err := tx.QueryRow(`SELECT ud.item_id FROM user_data ud JOIN items i ON i.id=ud.item_id
			WHERE ud.profile_id=? AND i.imdb_id=? AND i.kind='movie'
			ORDER BY COALESCE(ud.last_played_at,0) DESC, ud.played DESC, ud.position DESC LIMIT 1`, pid, imdb).Scan(&lead)
		if err != nil {
			continue
		}
		if _, err := tx.Exec(`UPDATE user_data SET
			favorite=(SELECT MAX(u.favorite) FROM user_data u JOIN items i ON i.id=u.item_id WHERE u.profile_id=? AND i.imdb_id=? AND i.kind='movie'),
			stars=(SELECT MAX(u.stars) FROM user_data u JOIN items i ON i.id=u.item_id WHERE u.profile_id=? AND i.imdb_id=? AND i.kind='movie'),
			play_count=(SELECT MAX(u.play_count) FROM user_data u JOIN items i ON i.id=u.item_id WHERE u.profile_id=? AND i.imdb_id=? AND i.kind='movie')
			WHERE profile_id=? AND item_id=?`, pid, imdb, pid, imdb, pid, imdb, pid, lead); err != nil {
			return err
		}
		if err := mirrorUserData(tx, pid, lead); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// mirrorUserData copies a movie's watch state to its other versions.
func mirrorUserData(q execer, profileID, itemID int64) error {
	_, err := q.Exec(`INSERT INTO user_data(profile_id,item_id,position,played,play_count,last_played_at,favorite,stars,hidden)
		SELECT ud.profile_id, v.id, ud.position, ud.played, ud.play_count, ud.last_played_at, ud.favorite, ud.stars, ud.hidden
		FROM user_data ud JOIN items i ON i.id=ud.item_id
		JOIN items v ON v.imdb_id=i.imdb_id AND v.kind='movie' AND v.id<>i.id
		WHERE ud.profile_id=? AND ud.item_id=? AND i.kind='movie' AND i.imdb_id<>''
		ON CONFLICT(profile_id,item_id) DO UPDATE SET position=excluded.position, played=excluded.played,
		  play_count=excluded.play_count, last_played_at=excluded.last_played_at, favorite=excluded.favorite,
		  stars=excluded.stars, hidden=excluded.hidden`, profileID, itemID)
	return err
}

// Versions lists the present versions of a movie visible to the profile, best picture first.
// It returns nil when the movie has no other version.
func (s *Store) Versions(p *Profile, it *Item) ([]*Item, error) {
	if it.Kind != "movie" || it.ImdbID == "" || it.VersionCount < 2 {
		return nil, nil
	}
	vs, err := s.QueryItems(p, ` AND i.kind='movie' AND i.imdb_id=? AND i.missing=0`, []any{it.ImdbID},
		`ORDER BY i.height DESC, CASE WHEN i.hdr<>'' THEN 0 ELSE 1 END, i.size DESC, i.id`)
	if err != nil || len(vs) < 2 {
		return nil, err
	}
	return vs, nil
}

// VersionPref returns the version the profile played last (0 = none).
func (s *Store) VersionPref(profileID int64, imdb string) int64 {
	var id int64
	s.DB.QueryRow(`SELECT vp.item_id FROM version_prefs vp JOIN items i ON i.id=vp.item_id AND i.missing=0
		WHERE vp.profile_id=? AND vp.imdb_id=?`, profileID, imdb).Scan(&id)
	return id
}

// SetVersionPref remembers the version a profile chose for a film.
func (s *Store) SetVersionPref(profileID int64, it *Item) error {
	if it.Kind != "movie" || it.ImdbID == "" {
		return nil
	}
	_, err := s.DB.Exec(`INSERT INTO version_prefs(profile_id,imdb_id,item_id,updated_at) VALUES(?,?,?,?)
		ON CONFLICT(profile_id,imdb_id) DO UPDATE SET item_id=excluded.item_id, updated_at=excluded.updated_at`,
		profileID, it.ImdbID, it.ID, now())
	return err
}
