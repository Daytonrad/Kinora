package store

import (
	"database/sql"
	"errors"
)

// Library is a scanned media folder.
type Library struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Path       string `json:"path"`
	Kind       string `json:"kind"` // movies | shows | mixed
	LastScanAt *int64 `json:"last_scan_at"`
	CreatedAt  int64  `json:"created_at"`
	Count      int    `json:"count"` // top level titles (movies + series), not missing
}

const libCols = `l.id,l.name,l.path,l.kind,l.last_scan_at,l.created_at,
 (SELECT COUNT(*) FROM items i WHERE i.library_id=l.id AND i.kind IN ('movie','series') AND i.missing=0)`

func scanLibrary(row interface{ Scan(...any) error }) (*Library, error) {
	l := &Library{}
	var last sql.NullInt64
	if err := row.Scan(&l.ID, &l.Name, &l.Path, &l.Kind, &last, &l.CreatedAt, &l.Count); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if last.Valid {
		l.LastScanAt = &last.Int64
	}
	return l, nil
}

// Libraries lists all libraries.
func (s *Store) Libraries() ([]*Library, error) {
	rows, err := s.DB.Query(`SELECT ` + libCols + ` FROM libraries l ORDER BY l.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Library{}
	for rows.Next() {
		l, err := scanLibrary(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// Library loads one library.
func (s *Store) Library(id int64) (*Library, error) {
	return scanLibrary(s.DB.QueryRow(`SELECT `+libCols+` FROM libraries l WHERE l.id=?`, id))
}

// CreateLibrary adds a library.
func (s *Store) CreateLibrary(name, path, kind string) (*Library, error) {
	res, err := s.DB.Exec(`INSERT INTO libraries(name,path,kind,created_at) VALUES(?,?,?,?)`, name, path, kind, now())
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.Library(id)
}

// UpdateLibrary renames a library or changes its folder; items are kept.
func (s *Store) UpdateLibrary(id int64, name, path string) error {
	res, err := s.DB.Exec(`UPDATE libraries SET name=?, path=? WHERE id=?`, name, path, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteLibrary removes a library and (by cascade) its items; returns removed item ids for image cleanup.
func (s *Store) DeleteLibrary(id int64) ([]int64, error) {
	rows, err := s.DB.Query(`SELECT id FROM items WHERE library_id=?`, id)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for rows.Next() {
		var i int64
		rows.Scan(&i)
		ids = append(ids, i)
	}
	rows.Close()
	_, err = s.DB.Exec(`DELETE FROM libraries WHERE id=?`, id)
	return ids, err
}

// TouchLibraryScan records the completion time of a scan.
func (s *Store) TouchLibraryScan(id int64) {
	s.DB.Exec(`UPDATE libraries SET last_scan_at=? WHERE id=?`, now(), id)
}

// LibraryStats summarizes the whole collection.
type LibraryStats struct {
	Movies   int   `json:"movies"`
	Series   int   `json:"series"`
	Episodes int   `json:"episodes"`
	Files    int   `json:"files"`
	Missing  int   `json:"missing"`
	Bytes    int64 `json:"bytes"`
}

// Stats counts titles, files and total size.
func (s *Store) Stats() LibraryStats {
	var st LibraryStats
	s.DB.QueryRow(`SELECT
		COALESCE(SUM(kind='movie' AND missing=0),0),
		COALESCE(SUM(kind='series' AND missing=0),0),
		COALESCE(SUM(kind='episode' AND missing=0),0),
		COALESCE(SUM(kind IN ('movie','episode') AND missing=0),0),
		COALESCE(SUM(kind IN ('movie','episode') AND missing=1),0),
		COALESCE(SUM(CASE WHEN kind IN ('movie','episode') AND missing=0 THEN size ELSE 0 END),0)
		FROM items`).Scan(&st.Movies, &st.Series, &st.Episodes, &st.Files, &st.Missing, &st.Bytes)
	return st
}
