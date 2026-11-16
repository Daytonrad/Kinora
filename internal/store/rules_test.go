package store

import (
	"strings"
	"testing"
)

type fx struct {
	s     *Store
	p     *Profile // adult
	kid   *Profile
	ids   map[string]int64
	libID int64
}

func ident(s string) string { return strings.TrimPrefix(strings.ToLower(s), "the ") }

// fixture builds a small library directly in SQL.
func fixture(t *testing.T) *fx {
	t.Helper()
	s := testStore(t)
	u, _ := s.CreateUser("csalad", "secret1", false)
	ps, _ := s.Profiles(u.ID)
	name, kids := "Gyerek", true
	kid, _ := s.CreateProfile(u.ID, ProfileInput{Name: &name, IsKids: &kids})
	lib, _ := s.CreateLibrary("Mix", "/m", "mixed")
	f := &fx{s: s, p: ps[0], kid: kid, ids: map[string]int64{}, libID: lib.ID}
	add := func(key, kind, title string, year int, rating string, cr float64, genres ...string) int64 {
		lvl, ok := RatingLevel(rating)
		var lv any
		if ok {
			lv = lvl
		}
		res, err := s.DB.Exec(`INSERT INTO items(library_id,kind,key,title,sort_title,year,content_rating,rating_level,community_rating,runtime_sec,height,width,added_at,updated_at)
			VALUES(?,?,?,?,?,?,?,?,?,6000,1080,1920,?,?)`, lib.ID, kind, key, title, ident(title), year, rating, lv, cr, now()-int64(year), now())
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		for _, g := range genres {
			s.DB.Exec(`INSERT INTO item_genres(item_id,genre) VALUES(?,?)`, id, g)
		}
		f.ids[key] = id
		return id
	}
	add("matrix", "movie", "The Matrix", 1999, "R", 8.7, "Sci-Fi", "Action")
	add("arrival", "movie", "Arrival", 2016, "PG-13", 7.9, "Sci-Fi", "Drama")
	add("nemo", "movie", "Finding Nemo", 2003, "G", 8.1, "Animation", "Family")
	add("unknown", "movie", "Mystery Tape", 2010, "", 5.0, "Drama")
	add("dune", "movie", "Dune", 2021, "PG-13", 8.0, "Sci-Fi")
	sid := add("show", "series", "Test Show", 2020, "TV-PG", 7.6, "Comedy")
	s.DB.Exec(`UPDATE items SET series_id=? WHERE id=?`, sid, sid)
	season := add("show/s1", "season", "1. évad", 2020, "TV-PG", 0)
	s.DB.Exec(`UPDATE items SET parent_id=?, series_id=?, season_num=1 WHERE id=?`, sid, sid, season)
	for i := 1; i <= 3; i++ {
		e := add("ep"+string(rune('0'+i)), "episode", "Ep "+string(rune('0'+i)), 2020, "TV-PG", 0)
		s.DB.Exec(`UPDATE items SET parent_id=?, series_id=?, season_num=1, episode_num=?, runtime_sec=1200 WHERE id=?`, season, sid, i, e)
	}
	sp := add("special", "episode", "Special", 2020, "TV-PG", 0)
	s.DB.Exec(`UPDATE items SET parent_id=?, series_id=?, season_num=0, episode_num=1 WHERE id=?`, season, sid, sp)
	s.DB.Exec(`INSERT INTO item_tags(item_id,tag) VALUES(?,?)`, f.ids["arrival"], "Villeneuve")
	s.DB.Exec(`INSERT INTO item_tags(item_id,tag) VALUES(?,?)`, f.ids["dune"], "Villeneuve")
	return f
}

func titles(items []*Item) string {
	var t []string
	for _, i := range items {
		t = append(t, i.Title)
	}
	return strings.Join(t, ",")
}

func TestRuleEngine(t *testing.T) {
	f := fixture(t)
	browse := func(rs RuleSet) string {
		items, total, err := f.s.Browse(f.p, rs, 0, 100)
		if err != nil {
			t.Fatal(err)
		}
		if total != len(items) {
			t.Fatalf("total %d != %d", total, len(items))
		}
		return titles(items)
	}
	cases := []struct {
		rs   RuleSet
		want string
	}{
		{RuleSet{}, "Arrival,Dune,Finding Nemo,The Matrix,Mystery Tape,Test Show"},
		{RuleSet{Rules: []Rule{{"genre", "is", "sci-fi"}}, Sort: "community_rating", Order: "desc"}, "The Matrix,Dune,Arrival"},
		{RuleSet{Rules: []Rule{{"genre", "is_not", "Sci-Fi"}, {"kind", "is", "movie"}}}, "Finding Nemo,Mystery Tape"},
		{RuleSet{Match: "any", Rules: []Rule{{"year", "lte", 2000}, {"tag", "is", "villeneuve"}}, Sort: "year"}, "The Matrix,Arrival,Dune"},
		{RuleSet{Rules: []Rule{{"title", "contains", "%"}}}, ""},
		{RuleSet{Rules: []Rule{{"title", "starts_with", "the"}}}, "The Matrix"},
		{RuleSet{Rules: []Rule{{"community_rating", "gte", "8"}}, Sort: "title", Limit: 2}, "Dune,Finding Nemo"},
		{RuleSet{Rules: []Rule{{"resolution", "gte", "1080"}, {"kind", "is", "movie"}, {"year", "gte", 2016}}}, "Arrival,Dune"},
	}
	for i, c := range cases {
		if got := browse(c.rs); got != c.want {
			t.Errorf("case %d: got %q want %q", i, got, c.want)
		}
	}
	if _, err := ParseRuleSet(`{"rules":[{"field":"title","op":"gte","value":"x"}]}`); err == nil {
		t.Error("invalid op accepted")
	}
	if _, err := ParseRuleSet(`{"rules":[{"field":"x'; DROP TABLE items;--","op":"is","value":1}]}`); err == nil {
		t.Error("unknown field accepted")
	}
	// played on series = no unplayed (non-special) episode
	show, _ := f.s.Item(f.ids["show"], f.p)
	f.s.SetPlayed(f.p.ID, show, true)
	if got := browse(RuleSet{Rules: []Rule{{"played", "is", true}}}); got != "Test Show" {
		t.Errorf("played series: %q", got)
	}
}

func TestKidsFiltering(t *testing.T) {
	f := fixture(t)
	items, _, _ := f.s.Browse(f.kid, RuleSet{}, 0, 100)
	got := titles(items)
	if strings.Contains(got, "Matrix") || strings.Contains(got, "Mystery") || strings.Contains(got, "Arrival") {
		t.Fatalf("kid sees too much: %s", got)
	}
	if got != "Finding Nemo,Test Show" {
		t.Fatalf("kid list: %s", got)
	}
	if _, err := f.s.Item(f.ids["matrix"], f.kid); err != ErrNotFound {
		t.Fatal("kid can load R item")
	}
	if f.s.ItemVisible(f.ids["unknown"], f.kid) {
		t.Fatal("kid must not see unrated items")
	}
	if res, _ := f.s.Search(f.kid, "matrix", 10); len(res) != 0 {
		t.Fatal("kid search leak")
	}
	// adult with explicit limit still sees unrated content
	lvl := 2
	f.s.UpdateProfile(f.p.ID, ProfileInput{MaxRatingLevel: &lvl})
	p, _ := f.s.Profile(f.p.ID)
	if !f.s.ItemVisible(f.ids["unknown"], p) || f.s.ItemVisible(f.ids["matrix"], p) {
		t.Fatal("adult limit")
	}
}

func TestProgressNextUpHide(t *testing.T) {
	f := fixture(t)
	ep := func(k string) *Item {
		it, err := f.s.Item(f.ids[k], f.p)
		if err != nil {
			t.Fatal(err)
		}
		return it
	}
	// half of ep1 → continue watching
	f.s.ReportProgress(f.p.ID, ep("ep1"), 600, false)
	cw, _ := f.s.ContinueWatching(f.p, 10)
	if titles(cw) != "Ep 1" {
		t.Fatalf("continue: %s", titles(cw))
	}
	if nu, _ := f.s.NextUp(f.p, 10); len(nu) != 0 {
		t.Fatalf("in-progress episode must not be next up: %s", titles(nu))
	}
	// >92% → played; next up = ep2
	played, _ := f.s.ReportProgress(f.p.ID, ep("ep1"), 1150, false)
	if !played {
		t.Fatal("92% rule")
	}
	nu, _ := f.s.NextUp(f.p, 10)
	if titles(nu) != "Ep 2" {
		t.Fatalf("next up: %s", titles(nu))
	}
	// hide the series: gone from next up without marking anything played
	f.s.SetFlag(f.p.ID, f.ids["show"], "hidden", true)
	if nu, _ := f.s.NextUp(f.p, 10); len(nu) != 0 {
		t.Fatal("hidden series still in next up")
	}
	if ud := f.s.GetUserData(f.p.ID, f.ids["ep2"]); ud.Played {
		t.Fatal("hide must not mark played")
	}
	// rewatching brings it back
	f.s.ReportProgress(f.p.ID, ep("ep2"), 30, false)
	if f.s.GetUserData(f.p.ID, f.ids["show"]).Hidden {
		t.Fatal("progress must unhide the series")
	}
	// finishing the last regular episode: specials are skipped
	f.s.ReportProgress(f.p.ID, ep("ep2"), 0, true)
	f.s.ReportProgress(f.p.ID, ep("ep3"), 0, true)
	if nu, _ := f.s.NextUp(f.p, 10); len(nu) != 0 {
		t.Fatalf("specials must be skipped: %s", titles(nu))
	}
	next, _ := f.s.NextEpisode(f.p, ep("ep1"))
	if next == nil || next.Title != "Ep 2" {
		t.Fatal("next episode")
	}
	pn, _ := f.s.PlayNext(f.p, f.ids["show"])
	if pn == nil {
		t.Fatal("play next")
	}
}

func TestRecommenderAndCollections(t *testing.T) {
	f := fixture(t)
	arrival, _ := f.s.Item(f.ids["arrival"], f.p)
	f.s.ReportProgress(f.p.ID, arrival, 0, true)
	five := 5
	f.s.SetStars(f.p.ID, arrival.ID, &five)
	rec, err := f.s.Recommend(f.p)
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.ForYou) == 0 || rec.ForYou[0].ID != f.ids["dune"] {
		t.Fatalf("for you should start with Dune (Sci-Fi + Villeneuve): %+v", rec.ForYou)
	}
	if !strings.Contains(rec.ForYou[0].Reason, "Sci-Fi") {
		t.Fatalf("reason: %q", rec.ForYou[0].Reason)
	}
	for _, r := range rec.ForYou {
		if r.ID == arrival.ID {
			t.Fatal("watched item recommended")
		}
	}
	if len(rec.BecauseTitle) != 1 || rec.BecauseTitle[0] != arrival.ID || rec.Because[0][0].ID != f.ids["dune"] {
		t.Fatalf("because you watched: %+v %+v", rec.BecauseTitle, rec.Because)
	}
	for _, g := range rec.HiddenGems {
		if g.ID == arrival.ID {
			t.Fatal("gem already started")
		}
	}
	// kids never get R/unrated recommendations
	krec, _ := f.s.Recommend(f.kid)
	for _, g := range krec.HiddenGems {
		if g.ID == f.ids["matrix"] {
			t.Fatal("kid gem leak")
		}
	}

	name := "Sci-Fi esték"
	col, err := f.s.CreateCollection(f.p, CollectionInput{Name: &name, Kind: "smart",
		Rules: &RuleSet{Rules: []Rule{{"genre", "is", "Sci-Fi"}}, Sort: "year"}})
	if err != nil {
		t.Fatal(err)
	}
	items, total, _ := f.s.CollectionItems(f.p, col, 0, 50)
	if total != 3 || titles(items) != "The Matrix,Arrival,Dune" {
		t.Fatalf("smart: %d %s", total, titles(items))
	}
	mname, shared := "Kedvencek", true
	man, _ := f.s.CreateCollection(f.p, CollectionInput{Name: &mname, Shared: &shared})
	f.s.AddToCollection(man, f.ids["nemo"])
	f.s.AddToCollection(man, f.ids["matrix"])
	f.s.ReorderCollection(man, []int64{f.ids["matrix"], f.ids["nemo"]})
	items, _, _ = f.s.CollectionItems(f.p, man, 0, 50)
	if titles(items) != "The Matrix,Finding Nemo" {
		t.Fatalf("manual order: %s", titles(items))
	}
	kitems, _, _ := f.s.CollectionItems(f.kid, man, 0, 50)
	if titles(kitems) != "Finding Nemo" {
		t.Fatalf("kid sees shared collection filtered: %s", titles(kitems))
	}
	refs := f.s.CollectionsOf(f.p, f.ids["matrix"])
	if len(refs) != 2 {
		t.Fatalf("collections of matrix: %+v", refs)
	}
	// private collection not visible to another profile
	kc, _ := f.s.Collections(f.kid)
	if len(kc) != 1 || kc[0].ID != man.ID {
		t.Fatalf("kid should only see the shared collection: %+v", kc)
	}
}
