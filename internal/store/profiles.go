package store

import (
	"database/sql"
	"errors"
)

// AcqProfile is a named quality profile (its rules are JSON, read by internal/acquire).
type AcqProfile struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	JSON      string `json:"json"`
	IsDefault bool   `json:"is_default"`
	CreatedAt int64  `json:"created_at"`
}

func scanAcqProfile(row interface{ Scan(...any) error }) (*AcqProfile, error) {
	p := &AcqProfile{}
	var def int
	if err := row.Scan(&p.ID, &p.Name, &p.JSON, &def, &p.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	p.IsDefault = def == 1
	return p, nil
}

// AcqProfiles lists the named quality profiles, the default first.
func (s *Store) AcqProfiles() ([]*AcqProfile, error) {
	rows, err := s.DB.Query(`SELECT id,name,json,is_default,created_at FROM acq_profiles ORDER BY is_default DESC, name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*AcqProfile{}
	for rows.Next() {
		p, err := scanAcqProfile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// AcqProfileByID loads one profile.
func (s *Store) AcqProfileByID(id int64) (*AcqProfile, error) {
	return scanAcqProfile(s.DB.QueryRow(`SELECT id,name,json,is_default,created_at FROM acq_profiles WHERE id=?`, id))
}

// DefaultAcqProfile is the profile marked default, if any.
func (s *Store) DefaultAcqProfile() (*AcqProfile, error) {
	return scanAcqProfile(s.DB.QueryRow(`SELECT id,name,json,is_default,created_at FROM acq_profiles WHERE is_default=1 LIMIT 1`))
}

// SaveAcqProfile inserts (ID 0) or updates a profile; making it default clears the flag on the others.
func (s *Store) SaveAcqProfile(p *AcqProfile) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if p.IsDefault {
		if _, err := tx.Exec(`UPDATE acq_profiles SET is_default=0`); err != nil {
			return err
		}
	}
	if p.ID == 0 {
		p.CreatedAt = now()
		res, err := tx.Exec(`INSERT INTO acq_profiles(name,json,is_default,created_at) VALUES(?,?,?,?)`, p.Name, p.JSON, b2i(p.IsDefault), p.CreatedAt)
		if err != nil {
			return err
		}
		p.ID, _ = res.LastInsertId()
	} else if _, err := tx.Exec(`UPDATE acq_profiles SET name=?, json=?, is_default=? WHERE id=?`, p.Name, p.JSON, b2i(p.IsDefault), p.ID); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteAcqProfile removes a profile; series and downloads using it fall back to the default.
func (s *Store) DeleteAcqProfile(id int64) error {
	_, err := s.DB.Exec(`DELETE FROM acq_profiles WHERE id=?`, id)
	return err
}

// UpgradeCandidates are imported films whose upgrade check is due: the newest finished download per film,
// with nothing newer running or done for it.
func (s *Store) UpgradeCandidates(checkedBefore int64, limit int) ([]*AcqGrab, error) {
	rows, err := s.DB.Query(`SELECT `+grabCols+` FROM acq_grabs g0 WHERE g0.kind='movie' AND g0.status='done' AND g0.target_path<>'' AND g0.upgrade_checked_at<?
 AND NOT EXISTS(SELECT 1 FROM acq_grabs g1 WHERE g1.kind='movie' AND g1.tmdb_id=g0.tmdb_id AND g1.id>g0.id AND g1.status IN ('downloading','importing','done'))
 ORDER BY g0.upgrade_checked_at, g0.id LIMIT ?`, checkedBefore, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*AcqGrab{}
	for rows.Next() {
		g, err := scanGrab(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// MarkUpgradeChecked records an upgrade search for a finished download.
func (s *Store) MarkUpgradeChecked(grabID int64) {
	s.DB.Exec(`UPDATE acq_grabs SET upgrade_checked_at=? WHERE id=?`, now(), grabID)
}

// SetSeriesProfile sets (or clears, with 0) a followed series' quality profile.
func (s *Store) SetSeriesProfile(seriesID, profileID int64) error {
	var v any
	if profileID > 0 {
		v = profileID
	}
	_, err := s.DB.Exec(`UPDATE acq_series SET profile_id=? WHERE id=?`, v, seriesID)
	return err
}

// SeriesProfileID returns a series' quality profile (0 = the default).
func (s *Store) SeriesProfileID(seriesID int64) int64 {
	var v sql.NullInt64
	s.DB.QueryRow(`SELECT profile_id FROM acq_series WHERE id=?`, seriesID).Scan(&v)
	return v.Int64
}

// AcqCount is a name with a number (top requesters, indexers).
type AcqCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
	Done  int    `json:"done,omitempty"`
}

// AcqStats is the numbers of the acquisition dashboard.
type AcqStats struct {
	Downloads struct {
		Total  int   `json:"total"`
		Done   int   `json:"done"`
		Failed int   `json:"failed"`
		Active int   `json:"active"`
		Bytes  int64 `json:"bytes"`
	} `json:"downloads"`
	Kinds    map[string]int `json:"kinds"` // movie, episode, season
	Requests struct {
		Total     int `json:"total"`
		Open      int `json:"open"`
		Available int `json:"available"`
		Denied    int `json:"denied"`
	} `json:"requests"`
	Requesters []AcqCount `json:"requesters"`
	Indexers   []AcqCount `json:"indexers"`
	Series     int        `json:"series"`
}

func (s *Store) counts(q string) []AcqCount {
	out := []AcqCount{}
	rows, err := s.DB.Query(q)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var x AcqCount
		if rows.Scan(&x.Name, &x.Count, &x.Done) == nil {
			out = append(out, x)
		}
	}
	return out
}

// AcqStatistics counts downloads, requests and their sources.
func (s *Store) AcqStatistics() (*AcqStats, error) {
	st := &AcqStats{Kinds: map[string]int{}}
	d := &st.Downloads
	if err := s.DB.QueryRow(`SELECT COUNT(*), COALESCE(SUM(status='done'),0), COALESCE(SUM(status='failed'),0), COALESCE(SUM(status IN ('downloading','importing')),0),
 COALESCE(SUM(CASE WHEN status='done' THEN size ELSE 0 END),0) FROM acq_grabs`).Scan(&d.Total, &d.Done, &d.Failed, &d.Active, &d.Bytes); err != nil {
		return nil, err
	}
	for _, k := range s.counts(`SELECT kind, COUNT(*), 0 FROM acq_grabs WHERE status='done' GROUP BY kind`) {
		st.Kinds[k.Name] = k.Count
	}
	r := &st.Requests
	s.DB.QueryRow(`SELECT COUNT(*), COALESCE(SUM(status IN ('pending','approved','waiting','downloading')),0), COALESCE(SUM(status='available'),0),
 COALESCE(SUM(status='denied'),0) FROM acq_requests WHERE status<>'canceled'`).Scan(&r.Total, &r.Open, &r.Available, &r.Denied)
	st.Requesters = s.counts(`SELECT COALESCE(u.username,'?'), COUNT(*) n, COALESCE(SUM(q.status='available'),0) FROM acq_requests q LEFT JOIN users u ON u.id=q.user_id
 WHERE q.status<>'canceled' GROUP BY q.user_id ORDER BY n DESC LIMIT 5`)
	st.Indexers = s.counts(`SELECT indexer_name, COUNT(*) n, COALESCE(SUM(status='done'),0) FROM acq_grabs WHERE indexer_name<>'' GROUP BY indexer_name ORDER BY n DESC LIMIT 10`)
	s.DB.QueryRow(`SELECT COUNT(*) FROM acq_series`).Scan(&st.Series)
	return st, nil
}

// AcqDefinition is an uploaded indexer definition (Cardigann YAML).
type AcqDefinition struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	YAML      string `json:"-"`
	CreatedAt int64  `json:"created_at"`
	InUse     int    `json:"in_use"`
}

// AcqDefinitions lists the uploaded definitions with how many indexers use each.
func (s *Store) AcqDefinitions() ([]*AcqDefinition, error) {
	rows, err := s.DB.Query(`SELECT d.id,d.name,d.type,d.yaml,d.created_at,(SELECT COUNT(*) FROM acq_indexers i WHERE i.definition_id=d.id)
 FROM acq_definitions d ORDER BY d.name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*AcqDefinition{}
	for rows.Next() {
		d := &AcqDefinition{}
		if err := rows.Scan(&d.ID, &d.Name, &d.Type, &d.YAML, &d.CreatedAt, &d.InUse); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// AcqDefinition loads one definition.
func (s *Store) AcqDefinition(id string) (*AcqDefinition, error) {
	d := &AcqDefinition{}
	err := s.DB.QueryRow(`SELECT id,name,type,yaml,created_at FROM acq_definitions WHERE id=?`, id).Scan(&d.ID, &d.Name, &d.Type, &d.YAML, &d.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return d, err
}

// SaveAcqDefinition stores (or replaces) a definition by its id.
func (s *Store) SaveAcqDefinition(d *AcqDefinition) error {
	d.CreatedAt = now()
	_, err := s.DB.Exec(`INSERT INTO acq_definitions(id,name,type,yaml,created_at) VALUES(?,?,?,?,?)
 ON CONFLICT(id) DO UPDATE SET name=excluded.name, type=excluded.type, yaml=excluded.yaml, created_at=excluded.created_at`, d.ID, d.Name, d.Type, d.YAML, d.CreatedAt)
	return err
}

// DeleteAcqDefinition removes a definition (refused by the caller while indexers use it).
func (s *Store) DeleteAcqDefinition(id string) error {
	_, err := s.DB.Exec(`DELETE FROM acq_definitions WHERE id=?`, id)
	return err
}
