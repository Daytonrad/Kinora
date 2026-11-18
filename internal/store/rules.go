package store

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Rule is one condition of a RuleSet.
type Rule struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value any    `json:"value"`
}

// RuleSet drives both library filtering and smart collections.
type RuleSet struct {
	Match string `json:"match"` // all | any
	Rules []Rule `json:"rules"`
	Sort  string `json:"sort,omitempty"`
	Order string `json:"order,omitempty"` // asc | desc
	Limit int    `json:"limit,omitempty"`
}

// FieldDesc describes a rule field for the UI's rule editor.
type FieldDesc struct {
	Field string   `json:"field"`
	Label string   `json:"label"`
	Type  string   `json:"type"` // enum | text | number | bool | days | library | genre | tag | collection
	Ops   []string `json:"ops"`
	Enum  []Option `json:"enum,omitempty"`
}

// Option is a value/label pair.
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

var (
	opsIs   = []string{"is", "is_not"}
	opsNum  = []string{"is", "gte", "lte"}
	opsText = []string{"contains", "not_contains", "starts_with"}
	opsBool = []string{"is"}
	opsDays = []string{"within_days", "older_than_days"}
)

// Fields lists every supported rule field.
var Fields = []FieldDesc{
	{"kind", "Típus", "enum", opsIs, []Option{{"movie", "Film"}, {"series", "Sorozat"}, {"episode", "Epizód"}}},
	{"library", "Könyvtár", "library", opsIs, nil},
	{"genre", "Műfaj", "genre", opsIs, nil},
	{"tag", "Címke", "tag", opsIs, nil},
	{"collection", "Gyűjtemény", "collection", opsIs, nil},
	{"title", "Cím", "text", opsText, nil},
	{"year", "Év", "number", opsNum, nil},
	{"community_rating", "Értékelés", "number", opsNum, nil},
	{"stars", "Saját csillag", "number", opsNum, nil},
	{"runtime", "Hossz (perc)", "number", opsNum, nil},
	{"resolution", "Felbontás", "enum", opsNum, []Option{{"480", "SD"}, {"720", "720p"}, {"1080", "1080p"}, {"2160", "4K"}}},
	{"added", "Hozzáadva", "days", opsDays, nil},
	{"played", "Megnézve", "bool", opsBool, nil},
	{"in_progress", "Folyamatban", "bool", opsBool, nil},
	{"favorite", "Kedvenc", "bool", opsBool, nil},
	{"hdr", "HDR", "bool", opsBool, nil},
	{"video_codec", "Videókodek", "enum", opsIs, []Option{{"h264", "H.264"}, {"hevc", "HEVC"}, {"av1", "AV1"}, {"vp9", "VP9"}, {"mpeg4", "MPEG-4"}}},
	{"content_rating", "Korhatár-besorolás", "text", opsIs, nil},
}

// Sorts lists the supported sort keys.
var Sorts = []Option{{"title", "Cím"}, {"added", "Hozzáadás"}, {"year", "Év"}, {"community_rating", "Értékelés"},
	{"runtime", "Hossz"}, {"last_played", "Utoljára nézve"}, {"random", "Véletlen"}}

func fieldDesc(f string) *FieldDesc {
	for i := range Fields {
		if Fields[i].Field == f {
			return &Fields[i]
		}
	}
	return nil
}

func valueString(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	case json.Number:
		return x.String()
	case nil:
		return ""
	}
	return fmt.Sprint(v)
}

func valueBool(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return x == "true" || x == "1" || x == "igen"
	case float64:
		return x != 0
	}
	return false
}

func likeEscape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// sqlPlayed / sqlInProgress work for movies, episodes and series (rolled up from episodes).
const sqlPlayed = `(CASE WHEN i.kind IN ('series','season') THEN
  EXISTS(SELECT 1 FROM items e WHERE (e.series_id=i.id OR e.parent_id=i.id) AND e.kind='episode' AND e.missing=0)
  AND NOT EXISTS(SELECT 1 FROM items e LEFT JOIN user_data eu ON eu.item_id=e.id AND eu.profile_id=?
    WHERE (e.series_id=i.id OR e.parent_id=i.id) AND e.kind='episode' AND e.missing=0 AND COALESCE(e.season_num,1)<>0 AND COALESCE(eu.played,0)=0)
 ELSE COALESCE(ud.played,0)=1 END)`

const sqlInProgress = `(CASE WHEN i.kind IN ('series','season') THEN
  EXISTS(SELECT 1 FROM items e JOIN user_data eu ON eu.item_id=e.id AND eu.profile_id=?
    WHERE (e.series_id=i.id OR e.parent_id=i.id) AND e.kind='episode' AND (eu.position>0 OR eu.played=1))
  AND EXISTS(SELECT 1 FROM items e LEFT JOIN user_data eu ON eu.item_id=e.id AND eu.profile_id=?
    WHERE (e.series_id=i.id OR e.parent_id=i.id) AND e.kind='episode' AND e.missing=0 AND COALESCE(e.season_num,1)<>0 AND COALESCE(eu.played,0)=0)
 ELSE COALESCE(ud.position,0)>0 AND COALESCE(ud.played,0)=0 END)`

const sqlResolution = `(CASE WHEN i.height>=1600 OR i.width>=3200 THEN 2160 WHEN i.height>=900 OR i.width>=1800 THEN 1080
  WHEN i.height>=650 OR i.width>=1200 THEN 720 WHEN i.height>0 THEN 480 ELSE NULL END)`

// compileRule turns one rule into a parametrized SQL condition.
func compileRule(r Rule, pid int64) (string, []any, error) {
	fd := fieldDesc(r.Field)
	if fd == nil {
		return "", nil, fmt.Errorf("ismeretlen mező: %s", r.Field)
	}
	okOp := false
	for _, o := range fd.Ops {
		if o == r.Op {
			okOp = true
		}
	}
	if !okOp {
		return "", nil, fmt.Errorf("a(z) %s mezőhöz nem használható a(z) %s művelet", fd.Label, r.Op)
	}
	v := valueString(r.Value)
	neg := r.Op == "is_not"
	not := func(cond string) string {
		if neg {
			return "NOT " + cond
		}
		return cond
	}
	num := func(expr string) (string, []any, error) {
		f, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", "."), 64)
		if err != nil {
			return "", nil, fmt.Errorf("a(z) %s mező értéke szám legyen", fd.Label)
		}
		op := map[string]string{"is": "=", "gte": ">=", "lte": "<="}[r.Op]
		return fmt.Sprintf("(%s IS NOT NULL AND %s %s ?)", expr, expr, op), []any{f}, nil
	}
	switch r.Field {
	case "kind":
		return not("i.kind=?"), []any{v}, nil
	case "library":
		return not("i.library_id=?"), []any{v}, nil
	case "genre":
		return not("EXISTS(SELECT 1 FROM item_genres g WHERE g.item_id=i.id AND g.genre=?)"), []any{v}, nil
	case "tag":
		return not("EXISTS(SELECT 1 FROM item_tags t WHERE t.item_id=i.id AND t.tag=?)"), []any{v}, nil
	case "collection":
		return not("EXISTS(SELECT 1 FROM collection_items ci WHERE ci.item_id=i.id AND ci.collection_id=?)"), []any{v}, nil
	case "title":
		if v == "" {
			return "1=1", nil, nil
		}
		switch r.Op {
		case "contains":
			return `i.title LIKE ? ESCAPE '\'`, []any{"%" + likeEscape(v) + "%"}, nil
		case "not_contains":
			return `i.title NOT LIKE ? ESCAPE '\'`, []any{"%" + likeEscape(v) + "%"}, nil
		default:
			return `i.title LIKE ? ESCAPE '\'`, []any{likeEscape(v) + "%"}, nil
		}
	case "year":
		return num("i.year")
	case "community_rating":
		return num("i.community_rating")
	case "stars":
		return num("ud.stars")
	case "runtime":
		return num("(i.runtime_sec/60.0)")
	case "resolution":
		return num(sqlResolution)
	case "added":
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return "", nil, fmt.Errorf("a napok száma pozitív egész legyen")
		}
		cut := time.Now().Add(-time.Duration(n) * 24 * time.Hour).Unix()
		if r.Op == "within_days" {
			return "i.added_at>=?", []any{cut}, nil
		}
		return "i.added_at<?", []any{cut}, nil
	case "played":
		c := sqlPlayed
		if !valueBool(r.Value) {
			c = "NOT " + c
		}
		return c, []any{pid}, nil
	case "in_progress":
		c := sqlInProgress
		if !valueBool(r.Value) {
			c = "NOT " + c
		}
		return c, []any{pid, pid}, nil
	case "favorite":
		if valueBool(r.Value) {
			return "COALESCE(ud.favorite,0)=1", nil, nil
		}
		return "COALESCE(ud.favorite,0)=0", nil, nil
	case "hdr":
		if valueBool(r.Value) {
			return "i.hdr<>''", nil, nil
		}
		return "i.hdr=''", nil, nil
	case "video_codec":
		return not("i.video_codec=?"), []any{v}, nil
	case "content_rating":
		return not("i.content_rating=? COLLATE NOCASE"), []any{v}, nil
	}
	return "", nil, fmt.Errorf("ismeretlen mező: %s", r.Field)
}

// Compile turns the rule set into a WHERE fragment (starting with " AND") and an
// ORDER BY clause. Only movies and series are returned unless a kind rule says otherwise.
func (rs *RuleSet) Compile(pid int64) (string, []any, string, error) {
	var conds []string
	var args []any
	hasKind := false
	for _, r := range rs.Rules {
		if r.Field == "kind" {
			hasKind = true
		}
		c, a, err := compileRule(r, pid)
		if err != nil {
			return "", nil, "", err
		}
		conds = append(conds, c)
		args = append(args, a...)
	}
	where := " AND i.missing=0"
	if !hasKind {
		where += " AND i.kind IN ('movie','series')"
	} else {
		where += " AND i.kind IN ('movie','series','episode')"
	}
	if len(conds) > 0 {
		j := " AND "
		if rs.Match == "any" {
			j = " OR "
		}
		where += " AND (" + strings.Join(conds, j) + ")"
	}
	return where, args, rs.orderBy(), nil
}

func (rs *RuleSet) orderBy() string {
	desc := rs.Order == "desc"
	dir := "ASC"
	if desc {
		dir = "DESC"
	}
	nullsLast := func(expr string) string {
		return fmt.Sprintf("ORDER BY (%s) IS NULL, %s %s, "+locSort, expr, expr, dir)
	}
	switch rs.Sort {
	case "added":
		return fmt.Sprintf("ORDER BY i.added_at %s, i.id %s", dir, dir)
	case "year":
		return nullsLast("i.year")
	case "community_rating":
		return nullsLast("i.community_rating")
	case "runtime":
		return nullsLast("NULLIF(i.runtime_sec,0)")
	case "last_played":
		return nullsLast(`CASE WHEN i.kind='series' THEN (SELECT MAX(eu.last_played_at) FROM items e JOIN user_data eu ON eu.item_id=e.id AND eu.profile_id=ud.profile_id WHERE e.series_id=i.id) ELSE ud.last_played_at END`)
	case "random":
		return "ORDER BY RANDOM()"
	}
	return "ORDER BY " + locSort + " " + dir + ", i.year"
}

// Browse runs a rule set with paging; it returns the page and the total count.
func (s *Store) Browse(p *Profile, rs RuleSet, offset, limit int) ([]*Item, int, error) {
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}
	if rs.Limit > 0 {
		if offset >= rs.Limit {
			return []*Item{}, rs.Limit, nil
		}
		if offset+limit > rs.Limit {
			limit = rs.Limit - offset
		}
	}
	pid := profileID(p)
	where, args, order, err := rs.Compile(pid)
	if err != nil {
		return nil, 0, err
	}
	rf, rargs := RatingSQL(p, "i")
	var total int
	cq := `SELECT COUNT(*) FROM items i LEFT JOIN user_data ud ON ud.item_id=i.id AND ud.profile_id=? WHERE 1=1` + where + rf
	if err := s.DB.QueryRow(cq, append(append([]any{pid}, args...), rargs...)...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if rs.Limit > 0 && total > rs.Limit {
		total = rs.Limit
	}
	items, err := s.QueryItems(p, where, args, order+" LIMIT ? OFFSET ?", limit, offset)
	if err != nil {
		return nil, 0, err
	}
	s.fillGenresTags(items, p)
	return items, total, nil
}

// ParseRuleSet decodes and validates JSON rules.
func ParseRuleSet(raw string) (RuleSet, error) {
	var rs RuleSet
	if strings.TrimSpace(raw) == "" {
		return RuleSet{Match: "all"}, nil
	}
	if err := json.Unmarshal([]byte(raw), &rs); err != nil {
		return rs, fmt.Errorf("hibás szabály JSON: %w", err)
	}
	if rs.Match != "any" {
		rs.Match = "all"
	}
	if len(rs.Rules) > 50 {
		return rs, fmt.Errorf("legfeljebb 50 szabály adható meg")
	}
	if rs.Limit < 0 || rs.Limit > 5000 {
		rs.Limit = 0
	}
	if _, _, _, err := rs.Compile(0); err != nil {
		return rs, err
	}
	return rs, nil
}
