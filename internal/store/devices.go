package store

import (
	"database/sql"
	"strings"
	"time"
)

// Platforms a device can report. Anything else is stored as "web".
var Platforms = map[string]bool{"web": true, "android": true, "androidtv": true, "ios": true, "tizen": true, "webos": true}

// DeviceInfo describes the client that owns a session.
type DeviceInfo struct {
	Name       string `json:"device_name"`
	Platform   string `json:"platform"`
	AppVersion string `json:"app_version"`
	IP         string `json:"-"`
}

// Clean trims and bounds the client supplied fields.
func (d DeviceInfo) Clean() DeviceInfo {
	cut := func(v string, n int) string {
		v = strings.TrimSpace(strings.Map(func(r rune) rune {
			if r < 32 {
				return -1
			}
			return r
		}, v))
		if r := []rune(v); len(r) > n {
			v = string(r[:n])
		}
		return v
	}
	d.Name = cut(d.Name, 60)
	d.AppVersion = cut(d.AppVersion, 24)
	d.Platform = strings.ToLower(cut(d.Platform, 16))
	if !Platforms[d.Platform] {
		d.Platform = "web"
	}
	return d
}

// Device is one logged-in session as shown in "My devices" and the admin list.
type Device struct {
	ID         string `json:"id"`
	UserID     int64  `json:"user_id"`
	Username   string `json:"username,omitempty"`
	Name       string `json:"name"`
	Platform   string `json:"platform"`
	AppVersion string `json:"app_version"`
	IP         string `json:"ip"`
	CreatedAt  int64  `json:"created_at"`
	LastSeen   int64  `json:"last_seen"`
	ProfileID  *int64 `json:"profile_id"`
	Current    bool   `json:"current"`
}

// SessionState is a resolved session.
type SessionState struct {
	User        *User
	Profile     *Profile
	DeviceID    string
	ProfileLock int64 // >0: the session may only use this profile (Quick Connect kids device)
}

// CreateDeviceSession creates a session for a device. profileID preselects a
// profile, lockProfile pins the session to it.
func (s *Store) CreateDeviceSession(userID int64, d DeviceInfo, profileID, lockProfile int64) (tok, deviceID string, err error) {
	d = d.Clean()
	tok = RandomToken(32)
	deviceID = RandomToken(8)
	t := now()
	var pid, lock any
	if profileID > 0 {
		pid = profileID
	}
	if lockProfile > 0 {
		lock = lockProfile
	}
	_, err = s.DB.Exec(`INSERT INTO sessions(token,user_id,profile_id,expires_at,device_id,device_name,platform,app_version,ip,created_at,last_seen,profile_lock)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		hashToken(tok), userID, pid, time.Now().Add(SessionTTL).Unix(), deviceID, d.Name, d.Platform, d.AppVersion, d.IP, t, t, lock)
	return tok, deviceID, err
}

// SessionFull resolves a token. Expiry slides forward (rewritten at most once a
// day); last activity and IP are refreshed at most once a minute.
func (s *Store) SessionFull(tok, ip string) (*SessionState, error) {
	if tok == "" {
		return nil, ErrNotFound
	}
	h := hashToken(tok)
	var uid int64
	var pid, lock sql.NullInt64
	var exp, last int64
	var dev, oldIP string
	err := s.DB.QueryRow(`SELECT user_id, profile_id, expires_at, device_id, last_seen, ip, profile_lock FROM sessions WHERE token=?`, h).
		Scan(&uid, &pid, &exp, &dev, &last, &oldIP, &lock)
	if err != nil {
		return nil, ErrNotFound
	}
	t := time.Now()
	if exp < t.Unix() {
		s.DB.Exec(`DELETE FROM sessions WHERE token=?`, h)
		return nil, ErrNotFound
	}
	if newExp := t.Add(SessionTTL).Unix(); newExp-exp > 24*3600 {
		s.DB.Exec(`UPDATE sessions SET expires_at=? WHERE token=?`, newExp, h)
	}
	if t.Unix()-last >= 60 || (ip != "" && ip != oldIP) {
		if ip == "" {
			ip = oldIP
		}
		s.DB.Exec(`UPDATE sessions SET last_seen=?, ip=? WHERE token=?`, t.Unix(), ip, h)
	}
	u, err := s.UserByID(uid)
	if err != nil {
		return nil, err
	}
	ss := &SessionState{User: u, DeviceID: dev, ProfileLock: lock.Int64}
	if pid.Valid {
		p, _ := s.Profile(pid.Int64)
		if p != nil && p.UserID == uid && (ss.ProfileLock == 0 || p.ID == ss.ProfileLock) {
			ss.Profile = p
		}
	}
	return ss, nil
}

// DeviceActive reports whether the device session still exists for the user
// (stream tokens die with the session that minted them).
func (s *Store) DeviceActive(deviceID string, userID int64) bool {
	var n int
	s.DB.QueryRow(`SELECT COUNT(*) FROM sessions WHERE device_id=? AND user_id=? AND expires_at>=?`, deviceID, userID, now()).Scan(&n)
	return n > 0
}

const deviceCols = `s.device_id, s.user_id, u.username, s.device_name, s.platform, s.app_version, s.ip, s.created_at, s.last_seen, s.profile_id`

func (s *Store) queryDevices(where string, args ...any) ([]Device, error) {
	rows, err := s.DB.Query(`SELECT `+deviceCols+` FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.expires_at>=? `+where+
		` ORDER BY s.last_seen DESC`, append([]any{now()}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Device{}
	for rows.Next() {
		var d Device
		var pid sql.NullInt64
		if err := rows.Scan(&d.ID, &d.UserID, &d.Username, &d.Name, &d.Platform, &d.AppVersion, &d.IP, &d.CreatedAt, &d.LastSeen, &pid); err != nil {
			return nil, err
		}
		if pid.Valid {
			d.ProfileID = &pid.Int64
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Devices lists the sessions of one account.
func (s *Store) Devices(userID int64) ([]Device, error) { return s.queryDevices(`AND s.user_id=?`, userID) }

// AllDevices lists every session (admin).
func (s *Store) AllDevices() ([]Device, error) { return s.queryDevices(``) }

// DeleteDevice logs out one device. userID 0 means any account (admin).
func (s *Store) DeleteDevice(userID int64, deviceID string) (bool, error) {
	q, args := `DELETE FROM sessions WHERE device_id=?`, []any{deviceID}
	if userID > 0 {
		q += ` AND user_id=?`
		args = append(args, userID)
	}
	res, err := s.DB.Exec(q, args...)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// DeleteDevices logs out every device of an account except keepDeviceID ("" = all).
func (s *Store) DeleteDevices(userID int64, keepDeviceID string) (int64, error) {
	res, err := s.DB.Exec(`DELETE FROM sessions WHERE user_id=? AND device_id<>?`, userID, keepDeviceID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
