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
	// the built-in engine (kind "cardigann"): the definition and the admin's values for its settings (JSON)
	DefinitionID string `json:"definition_id"`
	Config       string `json:"config"`
	LastOKAt     *int64 `json:"last_ok_at"`
	LastError    string `json:"last_error"`
	CreatedAt    int64  `json:"created_at"`
}

// AcqClient is a stored download client.
type AcqClient struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	URL        string `json:"url"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	Category   string `json:"category"`  // films (and series, when CategoryTV is empty)
	SavePath   string `json:"save_path"` // films (and series, when SavePathTV is empty)
	CategoryTV string `json:"category_tv"`
	SavePathTV string `json:"save_path_tv"`
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
	InfoHash     string  `json:"info_hash"`   // the release's identity (torrent info hash, "nzb:…")
	DownloadID   string  `json:"download_id"` // the client's id for the download
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
	RequestID    *int64  `json:"request_id"`
	SeriesID     *int64  `json:"series_id"`
	Season       int     `json:"season"`   // -1 for films
	Episodes     string  `json:"episodes"` // "3", or "1,2,3" for a pack; "" = the whole season
	ProfileID    *int64  `json:"profile_id"`
	Replaces     string  `json:"replaces"` // an upgrade: the file it replaces once imported
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

const idxCols = `id,name,kind,url,api_key,categories,enabled,priority,last_ok_at,last_error,created_at,definition_id,config`

func scanIndexer(row interface{ Scan(...any) error }) (*AcqIndexer, error) {
	x := &AcqIndexer{}
	var en int
	var ok sql.NullInt64
	if err := row.Scan(&x.ID, &x.Name, &x.Kind, &x.URL, &x.APIKey, &x.Categories, &en, &x.Priority, &ok, &x.LastError, &x.CreatedAt, &x.DefinitionID, &x.Config); err != nil {
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
		if x.Config == "" {
			x.Config = "{}"
		}
		res, err := s.DB.Exec(`INSERT INTO acq_indexers(name,kind,url,api_key,categories,enabled,priority,last_error,created_at,definition_id,config) VALUES(?,?,?,?,?,?,?,'',?,?,?)`,
			x.Name, x.Kind, x.URL, x.APIKey, x.Categories, b2i(x.Enabled), x.Priority, x.CreatedAt, x.DefinitionID, x.Config)
		if err != nil {
			return err
		}
		x.ID, _ = res.LastInsertId()
		return nil
	}
	if x.Config == "" {
		x.Config = "{}"
	}
	_, err := s.DB.Exec(`UPDATE acq_indexers SET name=?,kind=?,url=?,api_key=?,categories=?,enabled=?,priority=?,definition_id=?,config=? WHERE id=?`,
		x.Name, x.Kind, x.URL, x.APIKey, x.Categories, b2i(x.Enabled), x.Priority, x.DefinitionID, x.Config, x.ID)
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

const cliCols = `id,name,kind,url,username,password,category,save_path,category_tv,save_path_tv,path_remote,path_local,enabled,created_at`

func scanClient(row interface{ Scan(...any) error }) (*AcqClient, error) {
	c := &AcqClient{}
	var en int
	if err := row.Scan(&c.ID, &c.Name, &c.Kind, &c.URL, &c.Username, &c.Password, &c.Category, &c.SavePath, &c.CategoryTV, &c.SavePathTV, &c.PathRemote, &c.PathLocal, &en, &c.CreatedAt); err != nil {
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
		res, err := s.DB.Exec(`INSERT INTO acq_clients(name,kind,url,username,password,category,save_path,category_tv,save_path_tv,path_remote,path_local,enabled,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			c.Name, c.Kind, c.URL, c.Username, c.Password, c.Category, c.SavePath, c.CategoryTV, c.SavePathTV, c.PathRemote, c.PathLocal, b2i(c.Enabled), c.CreatedAt)
		if err != nil {
			return err
		}
		c.ID, _ = res.LastInsertId()
		return nil
	}
	_, err := s.DB.Exec(`UPDATE acq_clients SET name=?,kind=?,url=?,username=?,password=?,category=?,save_path=?,category_tv=?,save_path_tv=?,path_remote=?,path_local=?,enabled=? WHERE id=?`,
		c.Name, c.Kind, c.URL, c.Username, c.Password, c.Category, c.SavePath, c.CategoryTV, c.SavePathTV, c.PathRemote, c.PathLocal, b2i(c.Enabled), c.ID)
	return err
}

// DeleteAcqClient removes a download client.
func (s *Store) DeleteAcqClient(id int64) error {
	_, err := s.DB.Exec(`DELETE FROM acq_clients WHERE id=?`, id)
	return err
}

// ---------------------------------------------------------------- grabs

const grabCols = `id,kind,tmdb_id,imdb_id,title,year,library_id,release_title,indexer_id,indexer_name,client_id,info_hash,size,quality,score,
 status,progress,eta,speed,error,target_path,requested_by,created_at,updated_at,request_id,series_id,season,episodes,download_id,profile_id,replaces`

func scanGrab(row interface{ Scan(...any) error }) (*AcqGrab, error) {
	g := &AcqGrab{}
	var lib, idx, cli, req, rq, ser, prof sql.NullInt64
	if err := row.Scan(&g.ID, &g.Kind, &g.TmdbID, &g.ImdbID, &g.Title, &g.Year, &lib, &g.ReleaseTitle, &idx, &g.IndexerName, &cli, &g.InfoHash,
		&g.Size, &g.Quality, &g.Score, &g.Status, &g.Progress, &g.ETA, &g.Speed, &g.Error, &g.TargetPath, &req, &g.CreatedAt, &g.UpdatedAt, &rq, &ser, &g.Season, &g.Episodes, &g.DownloadID, &prof, &g.Replaces); err != nil {
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
	g.LibraryID, g.IndexerID, g.ClientID, g.RequestedBy, g.RequestID, g.SeriesID, g.ProfileID = ptr(lib), ptr(idx), ptr(cli), ptr(req), ptr(rq), ptr(ser), ptr(prof)
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
	if g.Kind == "movie" || g.Kind == "" {
		g.Season = -1
	}
	res, err := s.DB.Exec(`INSERT INTO acq_grabs(kind,tmdb_id,imdb_id,title,year,library_id,release_title,indexer_id,indexer_name,client_id,info_hash,size,quality,score,
 status,progress,eta,speed,error,target_path,requested_by,created_at,updated_at,request_id,series_id,season,episodes,download_id,profile_id,replaces) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,0,-1,0,'','',?,?,?,?,?,?,?,?,?,?)`,
		g.Kind, g.TmdbID, g.ImdbID, g.Title, g.Year, g.LibraryID, g.ReleaseTitle, g.IndexerID, g.IndexerName, g.ClientID, g.InfoHash, g.Size, g.Quality, g.Score,
		g.Status, g.RequestedBy, g.CreatedAt, g.UpdatedAt, g.RequestID, g.SeriesID, g.Season, g.Episodes, g.DownloadID, g.ProfileID, g.Replaces)
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

// MovieTmdbIDs returns which of these TMDB ids are films already in a library, with an item id for each.
func (s *Store) MovieTmdbIDs(ids []int64) (map[int64]int64, error) {
	out := map[int64]int64{}
	if len(ids) == 0 {
		return out, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.DB.Query(`SELECT tmdb_id, MIN(id) FROM items WHERE kind='movie' AND missing=0 AND tmdb_id IN (?`+strings.Repeat(",?", len(ids)-1)+`) GROUP BY tmdb_id`, args...)
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

// ---------------------------------------------------------------- requests

// AcqRequest is a film someone asked for.
type AcqRequest struct {
	ID        int64  `json:"id"`
	Kind      string `json:"kind"`
	TmdbID    int64  `json:"tmdb_id"`
	ImdbID    string `json:"imdb_id"`
	Title     string `json:"title"`
	Year      int    `json:"year"`
	Poster    string `json:"poster"`
	UserID    *int64 `json:"user_id"`
	UserName  string `json:"user_name"`
	ProfileID *int64 `json:"profile_id"`
	Status    string `json:"status"`
	Note      string `json:"note"`
	Seasons   string `json:"seasons"` // series: "all", "future" or "1,2"
	// the library the requester chose (a films library for a film, a series library for a series)
	LibraryID   *int64 `json:"library_id"`
	LibraryName string `json:"library_name"`
	Attempts    int    `json:"attempts"`
	NextTryAt   int64  `json:"next_try_at"`
	DecidedBy   *int64 `json:"decided_by"`
	Seen        bool   `json:"seen"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
	// the running download of this request, if any (0..1)
	Progress float64 `json:"progress"`
}

// Request states.
const (
	ReqPending     = "pending"     // waits for an admin
	ReqApproved    = "approved"    // to be searched
	ReqWaiting     = "waiting"     // no acceptable release yet: searched again at next_try_at
	ReqDownloading = "downloading" // a grab runs
	ReqAvailable   = "available"   // in the library
	ReqDenied      = "denied"
	ReqFailed      = "failed"
	ReqCanceled    = "canceled"
)

// OpenRequestStates are the states of a request still being worked on.
var OpenRequestStates = []string{ReqPending, ReqApproved, ReqWaiting, ReqDownloading}

const reqCols = `r.id,r.kind,r.tmdb_id,r.imdb_id,r.title,r.year,r.poster,r.user_id,COALESCE(u.username,''),r.profile_id,r.status,r.note,r.attempts,
 r.next_try_at,r.decided_by,r.seen,r.created_at,r.updated_at,r.seasons,r.library_id,COALESCE((SELECT l.name FROM libraries l WHERE l.id=r.library_id),''),
 COALESCE((SELECT g.progress FROM acq_grabs g WHERE g.request_id=r.id AND g.status IN ('downloading','importing') ORDER BY g.id DESC LIMIT 1),0)`

const reqFrom = ` FROM acq_requests r LEFT JOIN users u ON u.id=r.user_id`

func scanRequest(row interface{ Scan(...any) error }) (*AcqRequest, error) {
	q := &AcqRequest{}
	var user, prof, dec, lib sql.NullInt64
	var seen int
	if err := row.Scan(&q.ID, &q.Kind, &q.TmdbID, &q.ImdbID, &q.Title, &q.Year, &q.Poster, &user, &q.UserName, &prof, &q.Status, &q.Note, &q.Attempts,
		&q.NextTryAt, &dec, &seen, &q.CreatedAt, &q.UpdatedAt, &q.Seasons, &lib, &q.LibraryName, &q.Progress); err != nil {
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
	q.UserID, q.ProfileID, q.DecidedBy, q.LibraryID, q.Seen = ptr(user), ptr(prof), ptr(dec), ptr(lib), seen == 1
	return q, nil
}

func (s *Store) queryRequests(where string, args ...any) ([]*AcqRequest, error) {
	rows, err := s.DB.Query(`SELECT `+reqCols+reqFrom+` `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*AcqRequest{}
	for rows.Next() {
		q, err := scanRequest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

func inStates(col string, states []string) (string, []any) {
	args := make([]any, len(states))
	for i, st := range states {
		args[i] = st
	}
	return col + ` IN (?` + strings.Repeat(",?", len(states)-1) + `)`, args
}

// AcqRequests lists requests: of one user (userID > 0) and/or in some states, newest first.
func (s *Store) AcqRequests(userID int64, limit int, states ...string) ([]*AcqRequest, error) {
	where := []string{"1=1"}
	var args []any
	if userID > 0 {
		where = append(where, "r.user_id=?")
		args = append(args, userID)
	}
	if len(states) > 0 {
		w, a := inStates("r.status", states)
		where = append(where, w)
		args = append(args, a...)
	}
	args = append(args, limit)
	return s.queryRequests(`WHERE `+strings.Join(where, " AND ")+` ORDER BY r.id DESC LIMIT ?`, args...)
}

// AcqRequest loads one request.
func (s *Store) AcqRequest(id int64) (*AcqRequest, error) {
	return scanRequest(s.DB.QueryRow(`SELECT `+reqCols+reqFrom+` WHERE r.id=?`, id))
}

// OpenRequestFor returns a request for this film that is still being worked on.
func (s *Store) OpenRequestFor(kind string, tmdbID int64) (*AcqRequest, error) {
	w, a := inStates("r.status", OpenRequestStates)
	return scanRequest(s.DB.QueryRow(`SELECT `+reqCols+reqFrom+` WHERE r.kind=? AND r.tmdb_id=? AND `+w+` ORDER BY r.id LIMIT 1`, append([]any{kind, tmdbID}, a...)...))
}

// DueRequests are the approved ones and the waiting ones whose retry time has come.
func (s *Store) DueRequests(at int64, limit int) ([]*AcqRequest, error) {
	return s.queryRequests(`WHERE r.status='approved' OR (r.status='waiting' AND r.next_try_at<=?) ORDER BY r.id LIMIT ?`, at, limit)
}

// CountUserRequests counts a user's requests since a time (denied and canceled ones do not count).
func (s *Store) CountUserRequests(userID, since int64) int {
	var n int
	s.DB.QueryRow(`SELECT COUNT(*) FROM acq_requests WHERE user_id=? AND created_at>=? AND status NOT IN ('denied','canceled')`, userID, since).Scan(&n)
	return n
}

// AddAcqRequest stores a new request.
func (s *Store) AddAcqRequest(q *AcqRequest) error {
	q.CreatedAt, q.UpdatedAt = now(), now()
	res, err := s.DB.Exec(`INSERT INTO acq_requests(kind,tmdb_id,imdb_id,title,year,poster,user_id,profile_id,status,note,attempts,next_try_at,decided_by,seen,created_at,updated_at,seasons,library_id)
 VALUES(?,?,?,?,?,?,?,?,?,'',0,0,?,0,?,?,?,?)`, q.Kind, q.TmdbID, q.ImdbID, q.Title, q.Year, q.Poster, q.UserID, q.ProfileID, q.Status, q.DecidedBy, q.CreatedAt, q.UpdatedAt, q.Seasons, q.LibraryID)
	if err != nil {
		return err
	}
	q.ID, _ = res.LastInsertId()
	return nil
}

// SetRequestStatus moves a request on; nextTry matters for the waiting state.
func (s *Store) SetRequestStatus(id int64, status, note string, nextTry int64) error {
	if len(note) > 300 {
		note = note[:300]
	}
	_, err := s.DB.Exec(`UPDATE acq_requests SET status=?, note=?, next_try_at=?, updated_at=? WHERE id=?`, status, note, nextTry, now(), id)
	return err
}

// DeleteAcqRequest removes a request; its grabs keep their history (request_id becomes NULL).
func (s *Store) DeleteAcqRequest(id int64) error {
	_, err := s.DB.Exec(`DELETE FROM acq_requests WHERE id=?`, id)
	return err
}

// DecideRequest records an admin's approval or refusal.
func (s *Store) DecideRequest(id int64, status, note string, adminID int64) error {
	_, err := s.DB.Exec(`UPDATE acq_requests SET status=?, note=?, decided_by=?, updated_at=? WHERE id=?`, status, note, adminID, now(), id)
	return err
}

// BumpRequestAttempts counts one failed download of a request and returns the new count.
func (s *Store) BumpRequestAttempts(id int64) int {
	s.DB.Exec(`UPDATE acq_requests SET attempts=attempts+1, updated_at=? WHERE id=?`, now(), id)
	var n int
	s.DB.QueryRow(`SELECT attempts FROM acq_requests WHERE id=?`, id).Scan(&n)
	return n
}

// MarkFilmAvailable closes every open request for a film that is now in the library.
func (s *Store) MarkFilmAvailable(kind string, tmdbID int64) {
	w, a := inStates("status", OpenRequestStates)
	s.DB.Exec(`UPDATE acq_requests SET status='available', note='', seen=0, updated_at=? WHERE kind=? AND tmdb_id=? AND `+w, append([]any{now(), kind, tmdbID}, a...)...)
}

// UnseenAvailable lists a user's fulfilled requests they were not told about yet.
func (s *Store) UnseenAvailable(userID int64) ([]*AcqRequest, error) {
	return s.queryRequests(`WHERE r.user_id=? AND r.status='available' AND r.seen=0 ORDER BY r.id`, userID)
}

// MarkRequestsSeen records that the user saw their fulfilled requests.
func (s *Store) MarkRequestsSeen(userID int64) {
	s.DB.Exec(`UPDATE acq_requests SET seen=1 WHERE user_id=? AND status='available'`, userID)
}

// RequestStates returns the newest request state per TMDB id (kind "movie" or "series"), for badges.
func (s *Store) RequestStates(kind string, ids []int64) (map[int64]string, error) {
	out := map[int64]string{}
	if len(ids) == 0 {
		return out, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.DB.Query(`SELECT tmdb_id, status FROM acq_requests WHERE kind=? AND status<>'canceled' AND tmdb_id IN (?`+strings.Repeat(",?", len(ids)-1)+`) ORDER BY id`, append([]any{kind}, args...)...)
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

// ---------------------------------------------------------------- blocklist

// Blocklist marks a release as bad, so automatic grabs skip it.
func (s *Store) Blocklist(hash, title, reason string) {
	if hash == "" {
		return
	}
	s.DB.Exec(`INSERT INTO acq_blocklist(info_hash,title,reason,created_at) VALUES(?,?,?,?) ON CONFLICT(info_hash) DO UPDATE SET reason=excluded.reason`, hash, title, reason, now())
}

// Blocklisted returns the blocklisted hashes among these.
func (s *Store) Blocklisted(hashes []string) map[string]bool {
	out := map[string]bool{}
	if len(hashes) == 0 {
		return out
	}
	args := make([]any, len(hashes))
	for i, h := range hashes {
		args[i] = h
	}
	rows, err := s.DB.Query(`SELECT info_hash FROM acq_blocklist WHERE info_hash IN (?`+strings.Repeat(",?", len(hashes)-1)+`)`, args...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var h string
		if rows.Scan(&h) == nil {
			out[h] = true
		}
	}
	return out
}
