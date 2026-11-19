package store

import "os"

// Subtitles fetched from a subtitle service live in the data folder (the media folders
// are read-only) and are listed after the file's own subtitle streams.

// DownloadedSub is a subtitle fetched for an item.
type DownloadedSub struct {
	ID              int64
	ItemID          int64
	Lang            string
	Title           string
	Path            string
	Provider        string
	FileID          string
	HearingImpaired bool
	Forced          bool
}

// appendDownloadedSubs adds the item's downloaded subtitles to its streams, numbered
// after the last subtitle stream.
func (s *Store) appendDownloadedSubs(itemID int64, streams []StreamInfo) ([]StreamInfo, error) {
	rows, err := s.DB.Query(`SELECT id,lang,title,path,provider,forced FROM item_subtitles WHERE item_id=? ORDER BY id`, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	next, idx := 0, -1
	for _, st := range streams {
		if st.Type == "subtitle" {
			if st.TypeIndex >= next {
				next = st.TypeIndex + 1
			}
			if st.Index <= idx {
				idx = st.Index - 1
			}
		}
	}
	for rows.Next() {
		st := StreamInfo{Type: "subtitle", Codec: "subrip", External: true, TextBased: true}
		if err := rows.Scan(&st.DownloadID, &st.Lang, &st.Title, &st.ExternalPath, &st.Provider, &st.Forced); err != nil {
			return nil, err
		}
		st.TypeIndex, st.Index = next, idx
		next++
		idx--
		streams = append(streams, st)
	}
	return streams, rows.Err()
}

// AddDownloadedSub stores a fetched subtitle (the same provider file again replaces the old row).
func (s *Store) AddDownloadedSub(d *DownloadedSub) (int64, error) {
	_, err := s.DB.Exec(`INSERT INTO item_subtitles(item_id,lang,title,path,provider,file_id,hearing_impaired,forced,created_at)
		VALUES(?,?,?,?,?,?,?,?,?)
		ON CONFLICT(item_id,provider,file_id) DO UPDATE SET lang=excluded.lang, title=excluded.title, path=excluded.path,
		  hearing_impaired=excluded.hearing_impaired, forced=excluded.forced`,
		d.ItemID, d.Lang, d.Title, d.Path, d.Provider, d.FileID, d.HearingImpaired, d.Forced, now())
	if err != nil {
		return 0, err
	}
	var id int64
	err = s.DB.QueryRow(`SELECT id FROM item_subtitles WHERE item_id=? AND provider=? AND file_id=?`, d.ItemID, d.Provider, d.FileID).Scan(&id)
	return id, err
}

// DeleteDownloadedSub removes a fetched subtitle and its file.
func (s *Store) DeleteDownloadedSub(itemID, id int64) error {
	var path string
	if err := s.DB.QueryRow(`SELECT path FROM item_subtitles WHERE id=? AND item_id=?`, id, itemID).Scan(&path); err != nil {
		return ErrNotFound
	}
	if _, err := s.DB.Exec(`DELETE FROM item_subtitles WHERE id=?`, id); err != nil {
		return err
	}
	os.Remove(path)
	return nil
}
