package store

import "database/sql"

// PushIOS is one iOS device paired with the push relay.
type PushIOS struct {
	DeviceID string
	UserID   int64
	Handle   string
	Key      []byte // AES-256 key shared with the device; the relay never sees it
}

// SetPushIOS pairs (or re-pairs) the session's device.
func (s *Store) SetPushIOS(deviceID string, userID int64, handle string, key []byte) error {
	_, err := s.DB.Exec(`INSERT INTO push_ios(device_id,user_id,handle,key,created_at) VALUES(?,?,?,?,?)
ON CONFLICT(device_id) DO UPDATE SET user_id=excluded.user_id, handle=excluded.handle, key=excluded.key`, deviceID, userID, handle, key, now())
	return err
}

// PushIOSFor returns the pairing of one device (nil when unpaired).
func (s *Store) PushIOSFor(deviceID string) (*PushIOS, error) {
	p := &PushIOS{}
	err := s.DB.QueryRow(`SELECT device_id,user_id,handle,key FROM push_ios WHERE device_id=?`, deviceID).Scan(&p.DeviceID, &p.UserID, &p.Handle, &p.Key)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

// PushIOSForUser lists every paired device of an account.
func (s *Store) PushIOSForUser(userID int64) ([]PushIOS, error) {
	rows, err := s.DB.Query(`SELECT device_id,user_id,handle,key FROM push_ios WHERE user_id=?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PushIOS
	for rows.Next() {
		var p PushIOS
		if err := rows.Scan(&p.DeviceID, &p.UserID, &p.Handle, &p.Key); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeletePushIOSHandle forgets a handle the relay reported as gone.
func (s *Store) DeletePushIOSHandle(handle string) error {
	_, err := s.DB.Exec(`DELETE FROM push_ios WHERE handle=?`, handle)
	return err
}

// DeletePushIOS unpairs a device and returns its handle ("" when it was not paired).
func (s *Store) DeletePushIOS(deviceID string) (string, error) {
	var h string
	err := s.DB.QueryRow(`DELETE FROM push_ios WHERE device_id=? RETURNING handle`, deviceID).Scan(&h)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return h, err
}

// CountPushIOS is the number of paired iOS devices.
func (s *Store) CountPushIOS() int {
	var n int
	s.DB.QueryRow(`SELECT COUNT(*) FROM push_ios`).Scan(&n)
	return n
}

// PendingAvailable lists fulfilled requests not pushed yet. Older than since are
// marked as pushed without sending (push was off when they arrived).
func (s *Store) PendingAvailable(since int64) ([]*AcqRequest, error) {
	s.DB.Exec(`UPDATE acq_requests SET pushed=1 WHERE status='available' AND pushed=0 AND updated_at<?`, since)
	return s.queryRequests(`WHERE r.status='available' AND r.pushed=0 AND r.user_id IS NOT NULL ORDER BY r.id LIMIT 100`)
}

// MarkPushed records that a fulfilled request was pushed.
func (s *Store) MarkPushed(id int64) {
	s.DB.Exec(`UPDATE acq_requests SET pushed=1 WHERE id=?`, id)
}

// LibraryItemByTmdb returns the id of the movie or series with a TMDB id (0 = none).
func (s *Store) LibraryItemByTmdb(kind string, tmdbID int64) int64 {
	var id int64
	s.DB.QueryRow(`SELECT id FROM items WHERE kind=? AND tmdb_id=? AND missing=0 ORDER BY id LIMIT 1`, kind, tmdbID).Scan(&id)
	return id
}
