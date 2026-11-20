package store

import (
	"database/sql"
	"errors"
	"sort"
	"strings"
)

// A profile can choose the audio and subtitle language of one film or one series: the episodes of a series
// share the choice, so the next episode starts the way the last one was set. Empty means "as the profile says".

// SubOff is the subtitle language that means "no subtitles" for the title.
const SubOff = "off"

// TitleLangs is a profile's language choice for a title.
type TitleLangs struct {
	TitleID   int64  `json:"title_id"`
	AudioLang string `json:"audio_lang"` // "" = the profile's audio languages
	SubLang   string `json:"sub_lang"`   // "" = the profile's subtitle setting, "off", or a language
}

// TitleOf is the item a language choice belongs to: the film itself, or the series of a season or episode.
func TitleOf(it *Item) int64 {
	if (it.Kind == "episode" || it.Kind == "season") && it.SeriesID != nil {
		return *it.SeriesID
	}
	return it.ID
}

// TitleLangsFor returns the choice for a title; with none saved, the empty one.
func (s *Store) TitleLangsFor(profileID, titleID int64) TitleLangs {
	t := TitleLangs{TitleID: titleID}
	err := s.DB.QueryRow(`SELECT audio_lang, sub_lang FROM title_langs WHERE profile_id=? AND title_id=?`, profileID, titleID).Scan(&t.AudioLang, &t.SubLang)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return TitleLangs{TitleID: titleID}
	}
	return t
}

// SetTitleLangs saves the choice; both empty removes it (back to the profile's settings).
func (s *Store) SetTitleLangs(profileID int64, t TitleLangs) error {
	t.AudioLang, t.SubLang = strings.ToLower(strings.TrimSpace(t.AudioLang)), strings.ToLower(strings.TrimSpace(t.SubLang))
	if t.AudioLang == "" && t.SubLang == "" {
		_, err := s.DB.Exec(`DELETE FROM title_langs WHERE profile_id=? AND title_id=?`, profileID, t.TitleID)
		return err
	}
	_, err := s.DB.Exec(`INSERT INTO title_langs(profile_id,title_id,audio_lang,sub_lang,updated_at) VALUES(?,?,?,?,?)
		ON CONFLICT(profile_id,title_id) DO UPDATE SET audio_lang=excluded.audio_lang, sub_lang=excluded.sub_lang, updated_at=excluded.updated_at`,
		profileID, t.TitleID, t.AudioLang, t.SubLang, now())
	return err
}

// TitleLanguages lists the audio and subtitle languages a film's file, or any episode of a series, has.
func (s *Store) TitleLanguages(titleID int64) (audio, subs []string, err error) {
	rows, err := s.DB.Query(`SELECT DISTINCT m.type, m.lang FROM media_streams m JOIN items i ON i.id=m.item_id
		WHERE (i.id=? OR i.series_id=?) AND m.type IN ('audio','subtitle') AND m.lang<>''`, titleID, titleID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	audio, subs = []string{}, []string{}
	for rows.Next() {
		var typ, lang string
		if err := rows.Scan(&typ, &lang); err != nil {
			return nil, nil, err
		}
		if typ == "audio" {
			audio = append(audio, strings.ToLower(lang))
		} else {
			subs = append(subs, strings.ToLower(lang))
		}
	}
	sort.Strings(audio)
	sort.Strings(subs)
	return dedupe(audio), dedupe(subs), rows.Err()
}

func dedupe(l []string) []string {
	out := l[:0]
	for i, x := range l {
		if i == 0 || x != l[i-1] {
			out = append(out, x)
		}
	}
	return out
}
