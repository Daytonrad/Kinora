package store

import (
	"math"
	"sort"
	"strings"
	"time"
)

// The recommender is fully local and explainable: a taste profile is built
// from the profile's own activity (watched, favorites, stars; episodes roll up
// to their series), with recency decay, and candidates are scored against it.

// Taste is a weighted preference profile.
type Taste struct {
	Genres  map[string]float64
	Tags    map[string]float64
	Decades map[int]float64
}

type candidate struct {
	ID      int64
	Kind    string
	Year    int
	Rating  float64
	Genres  []string
	Tags    []string
	Played  bool
	Hidden  bool
	Touched bool // any profile of the household started it
}

// Recommendation is a scored item with a human readable reason.
type Recommendation struct {
	ID     int64
	Score  float64
	Reason string
}

func lower(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = strings.ToLower(s)
	}
	return out
}

// candidates loads visible, present movies & series with genres/tags and per-profile state.
func (s *Store) candidates(p *Profile) (map[int64]*candidate, error) {
	rf, rargs := RatingSQL(p, "i")
	rows, err := s.DB.Query(`SELECT i.id,i.kind,COALESCE(i.year,0),COALESCE(i.community_rating,0),
		COALESCE(ud.hidden,0),
		CASE WHEN i.kind='series' THEN
		  EXISTS(SELECT 1 FROM items e WHERE e.series_id=i.id AND e.kind='episode' AND e.missing=0)
		  AND NOT EXISTS(SELECT 1 FROM items e LEFT JOIN user_data eu ON eu.item_id=e.id AND eu.profile_id=?
		    WHERE e.series_id=i.id AND e.kind='episode' AND e.missing=0 AND COALESCE(e.season_num,1)<>0 AND COALESCE(eu.played,0)=0)
		ELSE COALESCE(ud.played,0) END,
		EXISTS(SELECT 1 FROM user_data hu JOIN profiles hp ON hp.id=hu.profile_id
		  LEFT JOIN items he ON he.id=hu.item_id
		  WHERE hp.user_id=? AND (hu.position>0 OR hu.played=1) AND (hu.item_id=i.id OR he.series_id=i.id))
		FROM items i LEFT JOIN user_data ud ON ud.item_id=i.id AND ud.profile_id=?
		WHERE i.kind IN ('movie','series') AND i.missing=0`+NotAltVersion+rf, append([]any{p.ID, p.UserID, p.ID}, rargs...)...)
	if err != nil {
		return nil, err
	}
	out := map[int64]*candidate{}
	for rows.Next() {
		c := &candidate{}
		if err := rows.Scan(&c.ID, &c.Kind, &c.Year, &c.Rating, &c.Hidden, &c.Played, &c.Touched); err != nil {
			rows.Close()
			return nil, err
		}
		out[c.ID] = c
	}
	rows.Close()
	for _, q := range []struct {
		sql string
		tag bool
	}{{`SELECT g.item_id,g.genre FROM item_genres g JOIN items i ON i.id=g.item_id WHERE i.kind IN ('movie','series')`, false},
		{`SELECT t.item_id,t.tag FROM item_tags t JOIN items i ON i.id=t.item_id WHERE i.kind IN ('movie','series')`, true}} {
		rows, err := s.DB.Query(q.sql)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id int64
			var v string
			rows.Scan(&id, &v)
			if c := out[id]; c != nil {
				if q.tag {
					c.Tags = append(c.Tags, v)
				} else {
					c.Genres = append(c.Genres, v)
				}
			}
		}
		rows.Close()
	}
	return out, nil
}

// activity returns movie/series ids with their taste weight, rolled up from episodes.
func (s *Store) activity(p *Profile) (map[int64]float64, []int64, error) {
	rows, err := s.DB.Query(`SELECT CASE WHEN i.kind IN ('episode','season') THEN i.series_id ELSE i.id END AS tid,
		u.played, u.position>0, u.favorite, u.stars, COALESCE(u.last_played_at,0)
		FROM user_data u JOIN items i ON i.id=u.item_id
		WHERE u.profile_id=? AND (u.played=1 OR u.position>0 OR u.favorite=1 OR u.stars IS NOT NULL)
		ORDER BY COALESCE(u.last_played_at,0) DESC LIMIT 300`, p.ID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	w := map[int64]float64{}
	lastSeen := map[int64]int64{}
	var order []int64
	nowT := time.Now().Unix()
	for rows.Next() {
		var tid *int64
		var played, prog, fav bool
		var stars *int
		var last int64
		if err := rows.Scan(&tid, &played, &prog, &fav, &stars, &last); err != nil {
			return nil, nil, err
		}
		if tid == nil {
			continue
		}
		decay := 1.0
		if last > 0 {
			days := float64(nowT-last) / 86400
			decay = math.Exp(-days / 180)
		}
		v := 0.0
		if played || prog {
			v += 1.0 * decay
		}
		if fav {
			v += 1.5
		}
		if stars != nil {
			v += float64(*stars-3) * 0.8
		}
		// episodes of the same series add up, but cap the series' influence
		w[*tid] = math.Max(-3, math.Min(4, w[*tid]+v))
		if (played || prog) && last > lastSeen[*tid] {
			if lastSeen[*tid] == 0 {
				order = append(order, *tid)
			}
			lastSeen[*tid] = last
		}
	}
	sort.SliceStable(order, func(a, b int) bool { return lastSeen[order[a]] > lastSeen[order[b]] })
	return w, order, rows.Err()
}

// BuildTaste computes the taste profile.
func (s *Store) BuildTaste(p *Profile, cands map[int64]*candidate) (*Taste, []int64, error) {
	act, order, err := s.activity(p)
	if err != nil {
		return nil, nil, err
	}
	t := &Taste{Genres: map[string]float64{}, Tags: map[string]float64{}, Decades: map[int]float64{}}
	for id, w := range act {
		c := cands[id]
		if c == nil {
			continue
		}
		for _, g := range lower(c.Genres) {
			t.Genres[g] += w
		}
		for _, tg := range lower(c.Tags) {
			t.Tags[tg] += w
		}
		if c.Year > 0 {
			t.Decades[c.Year/10*10] += w
		}
	}
	return t, order, nil
}

func (t *Taste) score(c *candidate) (float64, string) {
	gs := 0.0
	var top []string
	for _, g := range c.Genres {
		v := t.Genres[strings.ToLower(g)]
		gs += v
		if v > 0 {
			top = append(top, g)
		}
	}
	if n := len(c.Genres); n > 0 {
		gs /= math.Sqrt(float64(n))
	}
	ts := 0.0
	for _, tg := range c.Tags {
		ts += t.Tags[strings.ToLower(tg)]
	}
	ds := 0.0
	if c.Year > 0 {
		ds = t.Decades[c.Year/10*10]
	}
	rs := 0.0
	if c.Rating > 0 {
		rs = (c.Rating - 6.5) * 0.12
	}
	score := gs + ts*0.8 + ds*0.1 + rs
	reason := ""
	if len(top) > 0 {
		if len(top) > 2 {
			top = top[:2]
		}
		reason = "mert szereted: " + strings.Join(top, ", ")
	} else if c.Rating >= 7.5 {
		reason = "magasra értékelt"
	}
	return score, reason
}

// Recommendations holds every recommender row of the home screen.
type Recommendations struct {
	ForYou       []Recommendation
	BecauseTitle []int64 // the seed titles ("Mert megnézted: X")
	Because      [][]Recommendation
	HiddenGems   []Recommendation
}

// Recommend computes "Neked ajánljuk", "Mert megnézted: X" (3 latest titles) and "Rejtett kincsek".
func (s *Store) Recommend(p *Profile) (*Recommendations, error) {
	cands, err := s.candidates(p)
	if err != nil {
		return nil, err
	}
	taste, order, err := s.BuildTaste(p, cands)
	if err != nil {
		return nil, err
	}
	out := &Recommendations{}
	if len(order) > 0 {
		var scored []Recommendation
		for _, c := range cands {
			if c.Played || c.Hidden {
				continue
			}
			sc, why := taste.score(c)
			if sc > 0 {
				scored = append(scored, Recommendation{c.ID, sc, why})
			}
		}
		sort.Slice(scored, func(i, j int) bool { return scored[i].Score > scored[j].Score })
		if len(scored) > 20 {
			scored = scored[:20]
		}
		out.ForYou = scored
	}
	for _, id := range order {
		if len(out.BecauseTitle) >= 3 {
			break
		}
		if cands[id] == nil {
			continue
		}
		sim := s.similar(cands, id, 12, true)
		if len(sim) > 0 {
			out.BecauseTitle = append(out.BecauseTitle, id)
			out.Because = append(out.Because, sim)
		}
	}
	var gems []Recommendation
	for _, c := range cands {
		if c.Rating >= 7.5 && !c.Touched && !c.Hidden {
			gems = append(gems, Recommendation{c.ID, c.Rating, "magasra értékelt, a háztartásban még senki nem kezdte el"})
		}
	}
	sort.Slice(gems, func(i, j int) bool { return gems[i].Score > gems[j].Score })
	if len(gems) > 20 {
		gems = gems[:20]
	}
	out.HiddenGems = gems
	return out, nil
}

// similar ranks candidates by Jaccard similarity on genres+tags plus a year bonus.
func (s *Store) similar(cands map[int64]*candidate, id int64, limit int, unwatchedOnly bool) []Recommendation {
	src := cands[id]
	if src == nil {
		return nil
	}
	set := func(c *candidate) map[string]bool {
		m := map[string]bool{}
		for _, g := range c.Genres {
			m["g:"+strings.ToLower(g)] = true
		}
		for _, t := range c.Tags {
			m["t:"+strings.ToLower(t)] = true
		}
		return m
	}
	a := set(src)
	var out []Recommendation
	for _, c := range cands {
		if c.ID == id || (unwatchedOnly && (c.Played || c.Hidden)) {
			continue
		}
		b := set(c)
		inter, union := 0, len(a)
		for k := range b {
			if a[k] {
				inter++
			} else {
				union++
			}
		}
		if inter == 0 || union == 0 {
			continue
		}
		j := float64(inter) / float64(union)
		if src.Year > 0 && c.Year > 0 {
			dy := math.Abs(float64(src.Year - c.Year))
			j += math.Max(0, 1-dy/20) * 0.2
		}
		out = append(out, Recommendation{c.ID, j, ""})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// Similar returns items similar to id (for the detail page).
func (s *Store) Similar(p *Profile, id int64, limit int) ([]int64, error) {
	cands, err := s.candidates(p)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for _, r := range s.similar(cands, id, limit, false) {
		ids = append(ids, r.ID)
	}
	return ids, nil
}

// ItemsByIDs loads visible items keeping the given order.
func (s *Store) ItemsByIDs(p *Profile, ids []int64) ([]*Item, error) {
	if len(ids) == 0 {
		return []*Item{}, nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	items, err := s.QueryItems(p, ` AND i.id IN (`+ph+`)`, args, "")
	if err != nil {
		return nil, err
	}
	pos := map[int64]int{}
	for i, id := range ids {
		pos[id] = i
	}
	sort.Slice(items, func(a, b int) bool { return pos[items[a].ID] < pos[items[b].ID] })
	s.fillGenresTags(items, p)
	return items, nil
}
