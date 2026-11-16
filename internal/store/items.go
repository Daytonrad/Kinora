package store

import (
	"database/sql"
	"errors"
	"strings"
)

// Item is a movie, series, season or episode.
type Item struct {
	ID              int64    `json:"id"`
	LibraryID       int64    `json:"library_id"`
	Kind            string   `json:"kind"`
	ParentID        *int64   `json:"parent_id,omitempty"`
	SeriesID        *int64   `json:"series_id,omitempty"`
	Title           string   `json:"title"`
	SortTitle       string   `json:"-"`
	Year            *int     `json:"year,omitempty"`
	SeasonNum       *int     `json:"season_num,omitempty"`
	EpisodeNum      *int     `json:"episode_num,omitempty"`
	EpisodeEnd      *int     `json:"episode_end,omitempty"`
	Overview        string   `json:"overview,omitempty"`
	ContentRating   string   `json:"content_rating,omitempty"`
	RatingLevel     *int     `json:"rating_level,omitempty"`
	CommunityRating *float64 `json:"community_rating,omitempty"`
	RuntimeSec      int      `json:"runtime_sec,omitempty"`
	Path            string   `json:"-"`
	Size            int64    `json:"size,omitempty"`
	Container       string   `json:"container,omitempty"`
	VideoCodec      string   `json:"video_codec,omitempty"`
	AudioCodec      string   `json:"audio_codec,omitempty"`
	Width           int      `json:"width,omitempty"`
	Height          int      `json:"height,omitempty"`
	Bitrate         int      `json:"bitrate,omitempty"`
	HDR             string   `json:"hdr,omitempty"`
	CreditsStart    *float64 `json:"credits_start,omitempty"`
	TmdbID          *int64   `json:"tmdb_id,omitempty"`
	HasPoster       bool     `json:"has_poster"`
	HasBackdrop     bool     `json:"has_backdrop"`
	Missing         bool     `json:"missing,omitempty"`
	AddedAt         int64    `json:"added_at"`
	UpdatedAt       int64    `json:"updated_at"`
	SeriesTitle     string   `json:"series_title,omitempty"`

	UserData UserData `json:"user_data"`
	// Series/season aggregates (filled by list queries)
	EpisodeCount  int `json:"episode_count,omitempty"`
	UnplayedCount int `json:"unplayed_count,omitempty"`

	Genres   []string     `json:"genres,omitempty"`
	Tags     []string     `json:"tags,omitempty"`
	Streams  []StreamInfo `json:"streams,omitempty"`
	Chapters []ChapterRow `json:"chapters,omitempty"`
}

// StreamInfo describes a playable stream of a file item.
type StreamInfo struct {
	Index        int    `json:"index"`
	Type         string `json:"type"`
	TypeIndex    int    `json:"type_index"`
	Codec        string `json:"codec"`
	Lang         string `json:"lang"`
	Title        string `json:"title"`
	Channels     int    `json:"channels,omitempty"`
	Default      bool   `json:"default"`
	Forced       bool   `json:"forced"`
	External     bool   `json:"external"`
	ExternalPath string `json:"-"`
	TextBased    bool   `json:"text_based"`
}

// TextSubtitleCodecs can be converted to WebVTT.
var TextSubtitleCodecs = map[string]bool{"subrip": true, "srt": true, "ass": true, "ssa": true, "webvtt": true, "mov_text": true, "text": true}

const itemSelect = `SELECT i.id,i.library_id,i.kind,i.parent_id,i.series_id,i.title,i.sort_title,i.year,i.season_num,i.episode_num,i.episode_end,
 i.overview,i.content_rating,i.rating_level,i.community_rating,i.runtime_sec,i.path,i.size,i.container,i.video_codec,i.audio_codec,
 i.width,i.height,i.bitrate,i.hdr,i.credits_start,i.tmdb_id,i.has_poster,i.has_backdrop,i.missing,i.added_at,i.updated_at,
 COALESCE(sr.title,''),
 COALESCE(ud.position,0),COALESCE(ud.played,0),COALESCE(ud.play_count,0),ud.last_played_at,COALESCE(ud.favorite,0),ud.stars,COALESCE(ud.hidden,0),
 CASE WHEN i.kind IN ('series','season') THEN (SELECT COUNT(*) FROM items e WHERE e.kind='episode' AND e.missing=0 AND (e.series_id=i.id OR e.parent_id=i.id) AND COALESCE(e.season_num,1)<>0) ELSE 0 END,
 CASE WHEN i.kind IN ('series','season') THEN (SELECT COUNT(*) FROM items e LEFT JOIN user_data eu ON eu.item_id=e.id AND eu.profile_id=?
   WHERE e.kind='episode' AND e.missing=0 AND (e.series_id=i.id OR e.parent_id=i.id) AND COALESCE(e.season_num,1)<>0 AND COALESCE(eu.played,0)=0) ELSE 0 END
FROM items i
LEFT JOIN items sr ON sr.id=i.series_id AND i.kind IN ('season','episode')
LEFT JOIN user_data ud ON ud.item_id=i.id AND ud.profile_id=?`

func scanItem(row interface{ Scan(...any) error }) (*Item, error) {
	it := &Item{}
	var year, season, ep, epEnd, lvl, stars sql.NullInt64
	var parent, series, tmdb, last sql.NullInt64
	var cr, credits sql.NullFloat64
	err := row.Scan(&it.ID, &it.LibraryID, &it.Kind, &parent, &series, &it.Title, &it.SortTitle, &year, &season, &ep, &epEnd,
		&it.Overview, &it.ContentRating, &lvl, &cr, &it.RuntimeSec, &it.Path, &it.Size, &it.Container, &it.VideoCodec, &it.AudioCodec,
		&it.Width, &it.Height, &it.Bitrate, &it.HDR, &credits, &tmdb, &it.HasPoster, &it.HasBackdrop, &it.Missing, &it.AddedAt, &it.UpdatedAt,
		&it.SeriesTitle,
		&it.UserData.Position, &it.UserData.Played, &it.UserData.PlayCount, &last, &it.UserData.Favorite, &stars, &it.UserData.Hidden,
		&it.EpisodeCount, &it.UnplayedCount)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	ip := func(n sql.NullInt64) *int {
		if !n.Valid {
			return nil
		}
		v := int(n.Int64)
		return &v
	}
	i64 := func(n sql.NullInt64) *int64 {
		if !n.Valid {
			return nil
		}
		v := n.Int64
		return &v
	}
	it.Year, it.SeasonNum, it.EpisodeNum, it.EpisodeEnd, it.RatingLevel = ip(year), ip(season), ip(ep), ip(epEnd), ip(lvl)
	it.ParentID, it.SeriesID, it.TmdbID = i64(parent), i64(series), i64(tmdb)
	it.UserData.LastPlayedAt = i64(last)
	it.UserData.Stars = ip(stars)
	if cr.Valid {
		it.CommunityRating = &cr.Float64
	}
	if credits.Valid {
		it.CreditsStart = &credits.Float64
	}
	if it.Kind == "series" || it.Kind == "season" {
		it.UserData.Played = it.EpisodeCount > 0 && it.UnplayedCount == 0
	}
	return it, nil
}

func profileID(p *Profile) int64 {
	if p == nil {
		return 0
	}
	return p.ID
}

// QueryItems runs an item query. where must start with " AND" (or be empty) and
// is combined with the profile's rating filter. tail holds ORDER BY / LIMIT.
func (s *Store) QueryItems(p *Profile, where string, args []any, tail string, tailArgs ...any) ([]*Item, error) {
	rf, rargs := RatingSQL(p, "i")
	pid := profileID(p)
	q := itemSelect + ` WHERE 1=1` + where + rf + " " + tail
	all := append([]any{pid, pid}, args...)
	all = append(all, rargs...)
	all = append(all, tailArgs...)
	rows, err := s.DB.Query(q, all...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Item{}
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// Item loads one visible item (without streams).
func (s *Store) Item(id int64, p *Profile) (*Item, error) {
	items, err := s.QueryItems(p, ` AND i.id=?`, []any{id}, "")
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, ErrNotFound
	}
	return items[0], nil
}

// ItemFull loads an item with streams, chapters, genres and tags.
func (s *Store) ItemFull(id int64, p *Profile) (*Item, error) {
	it, err := s.Item(id, p)
	if err != nil {
		return nil, err
	}
	if it.Streams, err = s.Streams(id); err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(`SELECT start,title FROM chapters WHERE item_id=? ORDER BY start`, id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var c ChapterRow
		rows.Scan(&c.Start, &c.Title)
		it.Chapters = append(it.Chapters, c)
	}
	rows.Close()
	s.fillGenresTags([]*Item{it})
	return it, nil
}

// Streams returns the streams of a file item ordered by type and index.
func (s *Store) Streams(id int64) ([]StreamInfo, error) {
	rows, err := s.DB.Query(`SELECT idx,type,type_idx,codec,lang,title,channels,is_default,is_forced,external_path
		FROM media_streams WHERE item_id=? ORDER BY CASE type WHEN 'video' THEN 0 WHEN 'audio' THEN 1 ELSE 2 END, type_idx`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StreamInfo{}
	for rows.Next() {
		var st StreamInfo
		if err := rows.Scan(&st.Index, &st.Type, &st.TypeIndex, &st.Codec, &st.Lang, &st.Title, &st.Channels, &st.Default, &st.Forced, &st.ExternalPath); err != nil {
			return nil, err
		}
		st.External = st.ExternalPath != ""
		st.TextBased = st.Type == "subtitle" && TextSubtitleCodecs[st.Codec]
		out = append(out, st)
	}
	return out, rows.Err()
}

// fillGenresTags loads genres and tags for a batch of items.
func (s *Store) fillGenresTags(items []*Item) {
	if len(items) == 0 {
		return
	}
	idx := map[int64]*Item{}
	ph := make([]string, 0, len(items))
	args := make([]any, 0, len(items))
	for _, it := range items {
		idx[it.ID] = it
		ph = append(ph, "?")
		args = append(args, it.ID)
	}
	in := strings.Join(ph, ",")
	if rows, err := s.DB.Query(`SELECT item_id,genre FROM item_genres WHERE item_id IN (`+in+`) ORDER BY genre`, args...); err == nil {
		for rows.Next() {
			var id int64
			var g string
			rows.Scan(&id, &g)
			idx[id].Genres = append(idx[id].Genres, g)
		}
		rows.Close()
	}
	if rows, err := s.DB.Query(`SELECT item_id,tag FROM item_tags WHERE item_id IN (`+in+`) ORDER BY tag`, args...); err == nil {
		for rows.Next() {
			var id int64
			var t string
			rows.Scan(&id, &t)
			idx[id].Tags = append(idx[id].Tags, t)
		}
		rows.Close()
	}
}

// FillGenresTags is the exported batch loader.
func (s *Store) FillGenresTags(items []*Item) { s.fillGenresTags(items) }
