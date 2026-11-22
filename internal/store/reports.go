package store

import "time"

// PartyReport is a watch party chat line a member reported to the admin.
type PartyReport struct {
	ID           int64     `json:"id"`
	Room         string    `json:"room"`
	ReporterUser int64     `json:"reporter_user"`
	ReporterName string    `json:"reporter_name"`
	SenderUser   int64     `json:"sender_user"`
	SenderName   string    `json:"sender_name"`
	SenderLogin  string    `json:"sender_login"` // the sender's account name, empty once the account is gone
	Text         string    `json:"text"`
	Reason       string    `json:"reason"`
	CreatedAt    time.Time `json:"created_at"`
}

// AddPartyReport stores a reported chat line.
func (s *Store) AddPartyReport(p PartyReport) error {
	null := func(v int64) any {
		if v <= 0 {
			return nil
		}
		return v
	}
	_, err := s.DB.Exec(`INSERT INTO party_reports(room,reporter_user,reporter_name,sender_user,sender_name,text,reason,created_at)
VALUES(?,?,?,?,?,?,?,?)`, p.Room, null(p.ReporterUser), p.ReporterName, null(p.SenderUser), p.SenderName, p.Text, p.Reason, p.CreatedAt.Unix())
	return err
}

// PartyReports lists the open reports, newest first.
func (s *Store) PartyReports() ([]PartyReport, error) {
	rows, err := s.DB.Query(`SELECT r.id, r.room, COALESCE(r.reporter_user,0), r.reporter_name, COALESCE(r.sender_user,0), r.sender_name,
  COALESCE(u.username,''), r.text, r.reason, r.created_at
FROM party_reports r LEFT JOIN users u ON u.id=r.sender_user ORDER BY r.created_at DESC, r.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PartyReport{}
	for rows.Next() {
		var p PartyReport
		var at int64
		if err := rows.Scan(&p.ID, &p.Room, &p.ReporterUser, &p.ReporterName, &p.SenderUser, &p.SenderName, &p.SenderLogin, &p.Text, &p.Reason, &at); err != nil {
			return nil, err
		}
		p.CreatedAt = time.Unix(at, 0)
		out = append(out, p)
	}
	return out, rows.Err()
}

// CountPartyReports is the number of open reports (the admin dashboard shows it).
func (s *Store) CountPartyReports() int {
	var n int
	s.DB.QueryRow(`SELECT COUNT(*) FROM party_reports`).Scan(&n)
	return n
}

// DeletePartyReport closes a report once the admin dealt with it.
func (s *Store) DeletePartyReport(id int64) error {
	_, err := s.DB.Exec(`DELETE FROM party_reports WHERE id=?`, id)
	return err
}
