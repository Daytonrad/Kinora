package store

import (
	"database/sql"
	"errors"
	"strings"
)

// AcqIndexer is a stored Torznab indexer.
type AcqIndexer struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	URL        string `json:"url"`
	APIKey     string `json:"api_key"`
	Categories string `json:"categories"`
	Enabled    bool   `json:"enabled"`
	Priority   int    `json:"priority"`
	LastOKAt   *int64 `json:"last_ok_at"`
	LastError  string `json:"last_error"`
	CreatedAt  int64  `json:"created_at"`
}

// AcqClient is a stored download client.
type AcqClient struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	URL        string `json:"url"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	Category   string `json:"category"`
	SavePath   string `json:"save_path"`
	PathRemote string `json:"path_remote"`
	PathLocal  string `json:"path_local"`
	Enabled    bool   `json:"enabled"`
	CreatedAt  int64  `json:"created_at"`
}

// AcqGrab is one download Kinora started.
type AcqGrab struct {
	ID           int64   `json:"id"`
	Kind         string  `json:"kind"`
	TmdbID       int64   `json:"tmdb_id"`
	ImdbID       string  `json:"imdb_id"`
	Title        string  `json:"title"`
	Year         int     `json:"year"`
	LibraryID    *int64  `json:"library_id"`
	ReleaseTitle string  `json:"release_title"`
	IndexerID    *int64  `json:"indexer_id"`
	IndexerName  string  `json:"indexer_name"`
	ClientID     *int64  `json:"client_id"`
	InfoHash     string  `json:"info_hash"`
	Size         int64   `json:"size"`
	Quality      string  `json:"quality"`
	Score        int     `json:"score"`
	Status       string  `json:"status"` // downloading | importing | done | failed | removed
	Progress     float64 `json:"progress"`
	ETA          int64   `json:"eta"`
	Speed        int64   `json:"speed"`
	Error        string  `json:"error"`
	TargetPath   string  `json:"target_path"`
	RequestedBy  *int64  `json:"requested_by"`
	CreatedAt    int64   `json:"created_at"`
	UpdatedAt    int64   `json:"updated_at"`
}

// Grab states.
const (
	GrabDownloading = "downloading"
	GrabImporting   = "importing"
	GrabDone        = "done"
	GrabFailed      = "failed"
	GrabRemoved     = "removed"
)

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ---------------------------------------------------------------- indexers

const idxCols = `id,name,kind,url,api_key,categories,enabled,priority,last_ok_at,last_error,created_at`

func scanIndexer(row interface{ Scan(...any) error }) (*AcqIndexer, error) {
	x := &AcqIndexer{}
	var en int
	var ok sql.NullInt64
	if err := row.Scan(&x.ID, &x.Name, &x.Kind, &x.URL, &x.APIKey, &x.Categories, &en, &x.Priority, &ok, &x.LastError, &x.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	x.Enabled = en == 1
	if ok.Valid {
		x.LastOKAt = &ok.Int64
	}
	return x, nil
}

// AcqIndexers lists the indexers, highest priority (lowest number) first.
func (s *Store) AcqIndexers() ([]*AcqIndexer, error) {
	rows, err := s.DB.Query(`SELECT ` + idxCols + ` FROM acq_indexers ORDER BY priority, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*AcqIndexer{}
	for rows.Next() {
		x, err := scanIndexer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// AcqIndexer loads one indexer.
func (s *Store) AcqIndexer(id int64) (*AcqIndexer, error) {
	return scanIndexer(s.DB.QueryRow(`SELECT `+idxCols+` FROM acq_indexers WHERE id=?`, id))
}

// SaveAcqIndexer inserts (ID 0) or updates an indexer.
func (s *Store) SaveAcqIndexer(x *AcqIndexer) error {
	if x.Kind == "" {
		x.Kind = "torznab"
	}
	if x.ID == 0 {
		x.CreatedAt = now()
		res, err := s.DB.Exec(`INSERT INTO acq_indexers(name,kind,url,api_key,categories,enabled,priority,last_error,created_at) VALUES(?,?,?,?,?,?,?,'',?)`,
			x.Name, x.Kind, x.URL, x.APIKey, x.Categories, b2i(x.Enabled), x.Priority, x.CreatedAt)
		if err != nil {
			return err
		}
		x.ID, _ = res.LastInsertId()
		return nil
	}
	_, err := s.DB.Exec(`UPDATE acq_indexers SET name=?,kind=?,url=?,api_key=?,categories=?,enabled=?,priority=? WHERE id=?`,
		x.Name, x.Kind, x.URL, x.APIKey, x.Categories, b2i(x.Enabled), x.Priority, x.ID)
	return err
}

// DeleteAcqIndexer removes an indexer.
func (s *Store) DeleteAcqIndexer(id int64) error {
	_, err := s.DB.Exec(`DELETE FROM acq_indexers WHERE id=?`, id)
	return err
}

// MarkAcqIndexer records the outcome of the last call to an indexer.
func (s *Store) MarkAcqIndexer(id int64, errText string) {
	if errText == "" {
		s.DB.Exec(`UPDATE acq_indexers SET last_ok_at=?, last_error='' WHERE id=?`, now(), id)
		return
	}
	if len(errText) > 300 {
		errText = errText[:300]
	}
	s.DB.Exec(`UPDATE acq_indexers SET last_error=? WHERE id=?`, errText, id)
}

// ---------------------------------------------------------------- clients

const cliCols = `id,name,kind,url,username,password,category,save_path,path_remote,path_local,enabled,created_at`

func scanClient(row interface{ Scan(...any) error }) (*AcqClient, error) {
	c := &AcqClient{}
	var en int
	if err := row.Scan(&c.ID, &c.Name, &c.Kind, &c.URL, &c.Username, &c.Password, &c.Category, &c.SavePath, &c.PathRemote, &c.PathLocal, &en, &c.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	c.Enabled = en == 1
	return c, nil
}

// AcqClients lists the download clients.
func (s *Store) AcqClients() ([]*AcqClient, error) {
	rows, err := s.DB.Query(`SELECT ` + cliCols + ` FROM acq_clients ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*AcqClient{}
	for rows.Next() {
		c, err := scanClient(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// AcqClient loads one download client.
func (s *Store) AcqClient(id int64) (*AcqClient, error) {
	return scanClient(s.DB.QueryRow(`SELECT `+cliCols+` FROM acq_clients WHERE id=?`, id))
}

// SaveAcqClient inserts (ID 0) or updates a download client.
func (s *Store) SaveAcqClient(c *AcqClient) error {
	if c.Kind == "" {
		c.Kind = "qbittorrent"
	}
	if c.ID == 0 {
		c.CreatedAt = now()
		res, err := s.DB.Exec(`INSERT INTO acq_clients(name,kind,url,username,password,category,save_path,path_remote,path_local,enabled,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
			c.Name, c.Kind, c.URL, c.Username, c.Password, c.Category, c.SavePath, c.PathRemote, c.PathLocal, b2i(c.Enabled), c.CreatedAt)
		if err != nil {
			return err
		}
		c.ID, _ = res.LastInsertId()
		return nil
	}
	_, err := s.DB.Exec(`UPDATE acq_clients SET name=?,kind=?,url=?,username=?,password=?,category=?,save_path=?,path_remote=?,path_local=?,enabled=? WHERE id=?`,
		c.Name, c.Kind, c.URL, c.Username, c.Password, c.Category, c.SavePath, c.PathRemote, c.PathLocal, b2i(c.Enabled), c.ID)
	return err
}

// DeleteAcqClient removes a download client.
func (s *Store) DeleteAcqClient(id int64) error {
	_, err := s.DB.Exec(`DELETE FROM acq_clients WHERE id=?`, id)
	return err
}

// ---------------------------------------------------------------- grabs

const grabCols = `id,kind,tmdb_id,imdb_id,title,year,library_id,release_title,indexer_id,indexer_name,client_id,info_hash,size,quality,score,
 status,progress,eta,speed,error,target_path,requested_by,created_at,updated_at`

func scanGrab(row interface{ Scan(...any) error }) (*AcqGrab, error) {
	g := &AcqGrab{}
	var lib, idx, cli, req sql.NullInt64
	if err := row.Scan(&g.ID, &g.Kind, &g.TmdbID, &g.ImdbID, &g.Title, &g.Year, &lib, &g.ReleaseTitle, &idx, &g.IndexerName, &cli, &g.InfoHash,
		&g.Size, &g.Quality, &g.Score, &g.Status, &g.Progress, &g.ETA, &g.Speed, &g.Error, &g.TargetPath, &req, &g.CreatedAt, &g.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	ptr := func(n sql.NullInt64) *int64 {
		if !n.Valid {
			return nil
		}
		v := n.Int64
		return &v
	}
	g.LibraryID, g.IndexerID, g.ClientID, g.RequestedBy = ptr(lib), ptr(idx), ptr(cli), ptr(req)
	return g, nil
}

// AcqGrabs lists downloads with the given states (all when none), newest first.
func (s *Store) AcqGrabs(limit int, states ...string) ([]*AcqGrab, error) {
	q := `SELECT ` + grabCols + ` FROM acq_grabs`
	args := []any{}
	if len(states) > 0 {
		q += ` WHERE status IN (?` + strings.Repeat(",?", len(states)-1) + `)`
		for _, st := range states {
			args = append(args, st)
		}
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.DB.Query(q, args...)
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

// AcqGrab loads one download.
func (s *Store) AcqGrab(id int64) (*AcqGrab, error) {
	return scanGrab(s.DB.QueryRow(`SELECT `+grabCols+` FROM acq_grabs WHERE id=?`, id))
}

// ActiveGrabForHash finds a running download of the same torrent.
func (s *Store) ActiveGrabForHash(hash string) (*AcqGrab, error) {
	return scanGrab(s.DB.QueryRow(`SELECT `+grabCols+` FROM acq_grabs WHERE info_hash=? AND status IN ('downloading','importing') LIMIT 1`, hash))
}

// AddAcqGrab stores a new download.
func (s *Store) AddAcqGrab(g *AcqGrab) error {
	g.CreatedAt, g.UpdatedAt = now(), now()
	if g.Status == "" {
		g.Status = GrabDownloading
	}
	res, err := s.DB.Exec(`INSERT INTO acq_grabs(kind,tmdb_id,imdb_id,title,year,library_id,release_title,indexer_id,indexer_name,client_id,info_hash,size,quality,score,
 status,progress,eta,speed,error,target_path,requested_by,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,0,-1,0,'','',?,?,?)`,
		g.Kind, g.TmdbID, g.ImdbID, g.Title, g.Year, g.LibraryID, g.ReleaseTitle, g.IndexerID, g.IndexerName, g.ClientID, g.InfoHash, g.Size, g.Quality, g.Score,
		g.Status, g.RequestedBy, g.CreatedAt, g.UpdatedAt)
	if err != nil {
		return err
	}
	g.ID, _ = res.LastInsertId()
	return nil
}

// UpdateAcqGrabProgress stores what the client reports.
func (s *Store) UpdateAcqGrabProgress(id int64, progress float64, eta, speed int64) {
	s.DB.Exec(`UPDATE acq_grabs SET progress=?, eta=?, speed=?, updated_at=? WHERE id=?`, progress, eta, speed, now(), id)
}

// SetAcqGrabStatus moves a download to another state (with an error text or the imported path).
func (s *Store) SetAcqGrabStatus(id int64, status, errText, target string) error {
	if len(errText) > 500 {
		errText = errText[:500]
	}
	_, err := s.DB.Exec(`UPDATE acq_grabs SET status=?, error=?, target_path=CASE WHEN ?<>'' THEN ? ELSE target_path END,
 progress=CASE WHEN ? IN ('importing','done') THEN 1 ELSE progress END, speed=0, updated_at=? WHERE id=?`,
		status, errText, target, target, status, now(), id)
	return err
}

// MovieTmdbIDs returns which of these TMDB ids are films already in a library.
func (s *Store) MovieTmdbIDs(ids []int64) (map[int64]bool, error) {
	out := map[int64]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.DB.Query(`SELECT DISTINCT tmdb_id FROM items WHERE kind='movie' AND missing=0 AND tmdb_id IN (?`+strings.Repeat(",?", len(ids)-1)+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			out[id] = true
		}
	}
	return out, rows.Err()
}

// GrabbedTmdbIDs returns the latest download state per TMDB id (films) for the "already asked" badge.
func (s *Store) GrabbedTmdbIDs(ids []int64) (map[int64]string, error) {
	out := map[int64]string{}
	if len(ids) == 0 {
		return out, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.DB.Query(`SELECT tmdb_id, status FROM acq_grabs WHERE kind='movie' AND status<>'removed' AND tmdb_id IN (?`+strings.Repeat(",?", len(ids)-1)+`) ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var st string
		if rows.Scan(&id, &st) == nil {
			out[id] = st
		}
	}
	return out, rows.Err()
}
