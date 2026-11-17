package store

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
)

// Segment is a skippable part of an episode (or movie).
type Segment struct {
	Type       string  `json:"type"` // intro | recap | credits | preview
	Start      float64 `json:"start"`
	End        float64 `json:"end"`
	Source     string  `json:"source"` // manual | chapter | auto
	Confidence float64 `json:"confidence"`
}

// SegmentTypes lists the valid segment types.
var SegmentTypes = map[string]bool{"intro": true, "recap": true, "credits": true, "preview": true}

// StoredSegments returns the rows stored for an item (manual and auto).
func (s *Store) StoredSegments(itemID int64) ([]Segment, error) {
	rows, err := s.DB.Query(`SELECT type,start,end,source,confidence FROM item_segments WHERE item_id=? ORDER BY start`, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Segment{}
	for rows.Next() {
		var g Segment
		if err := rows.Scan(&g.Type, &g.Start, &g.End, &g.Source, &g.Confidence); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// SetManualSegment stores a hand-marked segment; it always wins over chapters and detection.
func (s *Store) SetManualSegment(itemID int64, g Segment) error {
	if !SegmentTypes[g.Type] {
		return fmt.Errorf("ismeretlen szegmenstípus: %s", g.Type)
	}
	if g.Start < 0 || g.End <= g.Start {
		return fmt.Errorf("a szegmens vége a kezdete után legyen")
	}
	_, err := s.DB.Exec(`INSERT INTO item_segments(item_id,type,start,end,source,confidence,updated_at) VALUES(?,?,?,?, 'manual', ?, ?)
		ON CONFLICT(item_id,type,source) DO UPDATE SET start=excluded.start, end=excluded.end, confidence=excluded.confidence, updated_at=excluded.updated_at`,
		itemID, g.Type, g.Start, g.End, nonZero(g.Confidence, 1), now())
	return err
}

func nonZero(v, d float64) float64 {
	if v == 0 {
		return d
	}
	return v
}

// SetAutoSegment stores a detected segment. It is kept next to a manual mark (which
// wins when both exist), so removing the mark falls back to the detection.
func (s *Store) SetAutoSegment(itemID int64, g Segment) error {
	_, err := s.DB.Exec(`INSERT INTO item_segments(item_id,type,start,end,source,confidence,updated_at) VALUES(?,?,?,?, 'auto', ?, ?)
		ON CONFLICT(item_id,type,source) DO UPDATE SET start=excluded.start, end=excluded.end, confidence=excluded.confidence, updated_at=excluded.updated_at`,
		itemID, g.Type, g.Start, g.End, g.Confidence, now())
	return err
}

// DeleteSegment removes a stored segment of one source ("" = any).
func (s *Store) DeleteSegment(itemID int64, typ, source string) error {
	q, args := `DELETE FROM item_segments WHERE item_id=? AND type=?`, []any{itemID, typ}
	if source != "" {
		q += ` AND source=?`
		args = append(args, source)
	}
	_, err := s.DB.Exec(q, args...)
	return err
}

// ClearAutoSegments drops detected segments of an item (before re-analysis).
func (s *Store) ClearAutoSegments(itemID int64, typ string) error {
	_, err := s.DB.Exec(`DELETE FROM item_segments WHERE item_id=? AND type=? AND source='auto'`, itemID, typ)
	return err
}

var (
	reIntroChapter   = regexp.MustCompile(`(?i)^\s*(intro|opening|op|főcím|fõcím|focim|nyitány|opening credits|theme)\b`)
	reRecapChapter   = regexp.MustCompile(`(?i)^\s*(recap|previously|előzmények|elozmenyek|az előző részben|previously on)\b`)
	reCreditsChapter = regexp.MustCompile(`(?i)^\s*(credits|end credits|ending|ed|stáblista|stablista|outro|end titles)\b`)
	rePreviewChapter = regexp.MustCompile(`(?i)^\s*(preview|next episode|a következő részben|előzetes)\b`)
)

// ChapterSegments derives segments from chapter titles (intro, opening, OP, főcím, recap,
// előzmények, credits, stáblista, preview…): a chapter runs until the next one starts.
func ChapterSegments(chapters []ChapterRow, runtime float64) []Segment {
	var out []Segment
	seen := map[string]bool{}
	for i, c := range chapters {
		typ := ""
		switch {
		case reIntroChapter.MatchString(c.Title):
			typ = "intro"
		case reRecapChapter.MatchString(c.Title):
			typ = "recap"
		case reCreditsChapter.MatchString(c.Title):
			typ = "credits"
		case rePreviewChapter.MatchString(c.Title):
			typ = "preview"
		}
		if typ == "" || seen[typ] {
			continue
		}
		end := runtime
		if i+1 < len(chapters) {
			end = chapters[i+1].Start
		}
		if end <= c.Start {
			continue
		}
		seen[typ] = true
		out = append(out, Segment{Type: typ, Start: c.Start, End: end, Source: "chapter", Confidence: 1})
	}
	return out
}

// EffectiveSegments merges the sources by priority: manual > chapter > auto.
func EffectiveSegments(stored []Segment, chapters []ChapterRow, runtime float64) []Segment {
	byType := map[string]Segment{}
	rank := map[string]int{"manual": 3, "chapter": 2, "auto": 1}
	consider := func(g Segment) {
		if cur, ok := byType[g.Type]; !ok || rank[g.Source] > rank[cur.Source] {
			byType[g.Type] = g
		}
	}
	for _, g := range stored {
		consider(g)
	}
	for _, g := range ChapterSegments(chapters, runtime) {
		consider(g)
	}
	out := []Segment{}
	for _, t := range []string{"recap", "intro", "credits", "preview"} {
		if g, ok := byType[t]; ok {
			out = append(out, g)
		}
	}
	return out
}

// ---------------------------------------------------------------- analysis bookkeeping

// AnalysisEpisode is an episode as the intro detector needs it.
type AnalysisEpisode struct {
	ID         int64
	Path       string
	RuntimeSec float64
	Sig        string // size+mtime: re-analyse when the file changes
	Season     int64
	Series     int64
	Label      string
}

// SeasonEpisodes lists the available episodes of a season in episode order.
func (s *Store) SeasonEpisodes(seasonID int64) ([]AnalysisEpisode, error) {
	return s.analysisQuery(`i.parent_id=?`, seasonID)
}

func (s *Store) analysisQuery(where string, args ...any) ([]AnalysisEpisode, error) {
	rows, err := s.DB.Query(`SELECT i.id, i.path, i.runtime_sec, i.size, i.mtime, COALESCE(i.parent_id,0), COALESCE(i.series_id,0),
		COALESCE(sr.title,'') || ' S' || printf('%02d', COALESCE(i.season_num,0)) || 'E' || printf('%02d', COALESCE(i.episode_num,0))
		FROM items i LEFT JOIN items sr ON sr.id=i.series_id
		WHERE i.kind='episode' AND i.missing=0 AND `+where+` ORDER BY i.season_num, i.episode_num, i.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AnalysisEpisode
	for rows.Next() {
		var e AnalysisEpisode
		var size, mtime, rt int64
		if err := rows.Scan(&e.ID, &e.Path, &rt, &size, &mtime, &e.Season, &e.Series, &e.Label); err != nil {
			return nil, err
		}
		e.RuntimeSec = float64(rt)
		e.Sig = fmt.Sprintf("%d-%d", size, mtime)
		out = append(out, e)
	}
	return out, rows.Err()
}

// SeasonsNeedingAnalysis returns seasons that have an episode never analysed (or changed since).
func (s *Store) SeasonsNeedingAnalysis(limit int) ([]int64, error) {
	rows, err := s.DB.Query(`SELECT DISTINCT i.parent_id FROM items i LEFT JOIN segment_scans sc ON sc.item_id=i.id
		WHERE i.kind='episode' AND i.missing=0 AND i.parent_id IS NOT NULL
		  AND (sc.item_id IS NULL OR sc.file_sig <> (i.size || '-' || i.mtime))
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// AnalysisSeasonsOf resolves a series, season or episode id to the season ids to analyse.
func (s *Store) AnalysisSeasonsOf(itemID int64) ([]int64, error) {
	var kind string
	var parent sql.NullInt64
	if err := s.DB.QueryRow(`SELECT kind, parent_id FROM items WHERE id=?`, itemID).Scan(&kind, &parent); err != nil {
		return nil, ErrNotFound
	}
	switch kind {
	case "season":
		return []int64{itemID}, nil
	case "episode":
		if parent.Valid {
			return []int64{parent.Int64}, nil
		}
		return nil, nil
	case "series":
		rows, err := s.DB.Query(`SELECT id FROM items WHERE kind='season' AND parent_id=? ORDER BY season_num`, itemID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []int64
		for rows.Next() {
			var id int64
			rows.Scan(&id)
			out = append(out, id)
		}
		return out, rows.Err()
	}
	return nil, fmt.Errorf("csak sorozatra, évadra vagy epizódra futtatható")
}

// MarkAnalysed records that an episode was analysed with this file signature.
func (s *Store) MarkAnalysed(itemID int64, sig, errMsg string) error {
	_, err := s.DB.Exec(`INSERT INTO segment_scans(item_id,file_sig,scanned_at,error) VALUES(?,?,?,?)
		ON CONFLICT(item_id) DO UPDATE SET file_sig=excluded.file_sig, scanned_at=excluded.scanned_at, error=excluded.error`,
		itemID, sig, now(), strings.TrimSpace(errMsg))
	return err
}

// SegmentStats summarises detection coverage for the admin page.
type SegmentStats struct {
	Episodes      int `json:"episodes"`
	Analysed      int `json:"analysed"`
	WithIntro     int `json:"with_intro"`
	WithCredits   int `json:"with_credits"`
	Manual        int `json:"manual"`
	Errors        int `json:"errors"`
}

// SegmentStats counts episodes, analysed episodes and found segments.
func (s *Store) SegmentStats() SegmentStats {
	var st SegmentStats
	s.DB.QueryRow(`SELECT COUNT(*) FROM items WHERE kind='episode' AND missing=0`).Scan(&st.Episodes)
	s.DB.QueryRow(`SELECT COUNT(*), COALESCE(SUM(error<>''),0) FROM segment_scans`).Scan(&st.Analysed, &st.Errors)
	s.DB.QueryRow(`SELECT COUNT(DISTINCT CASE WHEN type='intro' THEN item_id END), COUNT(DISTINCT CASE WHEN type='credits' THEN item_id END),
		COALESCE(SUM(source='manual'),0) FROM item_segments`).
		Scan(&st.WithIntro, &st.WithCredits, &st.Manual)
	return st
}
