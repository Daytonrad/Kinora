package store

// Online metadata in other languages than metadata_lang. The items table holds the titles and overviews
// in metadata_lang; item_i18n holds them in the languages viewers use, keyed by TMDB language (en-US).
// A row with empty texts means "fetched, TMDB has nothing": queries fall back to the items values.

// SetTranslation stores an item's title and overview in lang.
func (s *Store) SetTranslation(id int64, lang, title, overview string) error {
	_, err := s.DB.Exec(`INSERT INTO item_i18n(item_id,lang,title,sort_title,overview) VALUES(?,?,?,?,?)
		ON CONFLICT(item_id,lang) DO UPDATE SET title=excluded.title,sort_title=excluded.sort_title,overview=excluded.overview`,
		id, lang, title, sortTitleSimple(title), overview)
	return err
}

// MarkChildrenTranslated records the seasons and episodes of a series that have no lang row yet as fetched,
// so ones TMDB does not know are not asked for again.
func (s *Store) MarkChildrenTranslated(seriesID int64, lang string) error {
	_, err := s.DB.Exec(`INSERT OR IGNORE INTO item_i18n(item_id,lang)
		SELECT id,? FROM items WHERE (series_id=? OR parent_id=?) AND kind IN ('season','episode') AND id<>?`, lang, seriesID, seriesID, seriesID)
	return err
}

// ClearTranslations drops the translations of an item and its seasons / episodes (it was matched to another title).
func (s *Store) ClearTranslations(id int64) error {
	_, err := s.DB.Exec(`DELETE FROM item_i18n WHERE item_id=? OR item_id IN (SELECT id FROM items WHERE series_id=? OR parent_id=?)`, id, id, id)
	return err
}

// SetGenreLabels replaces the genre names shown in lang (metadata_lang genre → lang genre).
func (s *Store) SetGenreLabels(lang string, labels map[string]string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM genre_i18n WHERE lang=?`, lang); err != nil {
		return err
	}
	for g, l := range labels {
		if _, err := tx.Exec(`INSERT OR REPLACE INTO genre_i18n(lang,genre,label) VALUES(?,?,?)`, lang, g, l); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ClearGenreLabels drops every genre translation (the metadata_lang genre names changed).
func (s *Store) ClearGenreLabels() error {
	_, err := s.DB.Exec(`DELETE FROM genre_i18n`)
	return err
}

// HasGenreLabels reports whether genre names were fetched for lang.
func (s *Store) HasGenreLabels(lang string) bool {
	var n int
	s.DB.QueryRow(`SELECT COUNT(*) FROM genre_i18n WHERE lang=?`, lang).Scan(&n)
	return n > 0
}

// genreLabels maps stored genre names to their lang names (nil for "").
func (s *Store) genreLabels(lang string) map[string]string {
	if lang == "" {
		return nil
	}
	rows, err := s.DB.Query(`SELECT genre,label FROM genre_i18n WHERE lang=?`, lang)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var g, l string
		if rows.Scan(&g, &l) == nil {
			out[g] = l
		}
	}
	return out
}
