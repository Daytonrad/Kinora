package store

import "database/sql"

// UserData is per-profile state of an item.
type UserData struct {
	Position     float64 `json:"position"`
	Played       bool    `json:"played"`
	PlayCount    int     `json:"play_count"`
	LastPlayedAt *int64  `json:"last_played_at"`
	Favorite     bool    `json:"favorite"`
	Stars        *int    `json:"stars"`
	Hidden       bool    `json:"hidden"`
}

// GetUserData returns the profile's state for an item (zero value if none).
func (s *Store) GetUserData(profileID, itemID int64) UserData {
	var d UserData
	var last sql.NullInt64
	var stars sql.NullInt64
	s.DB.QueryRow(`SELECT position,played,play_count,last_played_at,favorite,stars,hidden FROM user_data WHERE profile_id=? AND item_id=?`,
		profileID, itemID).Scan(&d.Position, &d.Played, &d.PlayCount, &last, &d.Favorite, &stars, &d.Hidden)
	if last.Valid {
		d.LastPlayedAt = &last.Int64
	}
	if stars.Valid {
		v := int(stars.Int64)
		d.Stars = &v
	}
	return d
}

func ensureUD(q interface {
	Exec(string, ...any) (sql.Result, error)
}, profileID, itemID int64) error {
	_, err := q.Exec(`INSERT OR IGNORE INTO user_data(profile_id,item_id) VALUES(?,?)`, profileID, itemID)
	return err
}
