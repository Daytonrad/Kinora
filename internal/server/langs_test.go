package server

import (
	"testing"

	"github.com/ScotyLucas/kinora/internal/store"
)

func TestTitleLangsAPI(t *testing.T) {
	c, st := newTestServer(t)
	var me struct {
		Profiles []store.Profile `json:"profiles"`
	}
	c.do("GET", "/api/me", nil, &me)
	c.do("POST", "/api/profiles/"+itoa(me.Profiles[0].ID)+"/select", map[string]string{}, nil)
	id := func(title string) string {
		var i int64
		st.DB.QueryRow(`SELECT id FROM items WHERE title=?`, title).Scan(&i)
		return itoa(i)
	}
	type langs struct {
		TitleID   int64    `json:"title_id"`
		AudioLang string   `json:"audio_lang"`
		SubLang   string   `json:"sub_lang"`
		Audio     []string `json:"audio"`
		Subs      []string `json:"subs"`
	}
	type play struct {
		SubIdx  int `json:"sub_idx"`
		Streams []store.StreamInfo
	}
	caps := map[string]any{"video": []string{"h264"}, "audio": []string{"aac", "ac3", "mp3"}, "containers": []string{"mp4", "matroska"}}
	movie := id("Big Test Movie")

	var l langs
	if code := c.do("GET", "/api/items/"+movie+"/langs", nil, &l); code != 200 || len(l.Subs) == 0 || l.AudioLang != "" || l.SubLang != "" {
		t.Fatalf("langs: %d %+v", code, l)
	}
	sub := l.Subs[0]
	// subtitles off for this film
	if code := c.do("PUT", "/api/items/"+movie+"/langs", map[string]string{"sub_lang": "off"}, &l); code != 200 || l.SubLang != "off" {
		t.Fatalf("put off: %d %+v", code, l)
	}
	var p play
	c.do("POST", "/api/items/"+movie+"/playback", map[string]any{"caps": caps}, &p)
	if p.SubIdx != -1 {
		t.Errorf("off: sub_idx %d", p.SubIdx)
	}
	// a language: the playback starts with that track
	c.do("PUT", "/api/items/"+movie+"/langs", map[string]string{"sub_lang": sub}, nil)
	c.do("POST", "/api/items/"+movie+"/playback", map[string]any{"caps": caps}, &p)
	ok := false
	for _, s := range p.Streams {
		if s.Type == "subtitle" && s.TypeIndex == p.SubIdx && s.Lang == sub {
			ok = true
		}
	}
	if !ok {
		t.Errorf("sub %q: sub_idx %d in %+v", sub, p.SubIdx, p.Streams)
	}
	if code := c.do("PUT", "/api/items/"+movie+"/langs", map[string]string{"audio_lang": "not a lang"}, nil); code != 400 {
		t.Errorf("bad code: %d", code)
	}
	// back to the profile's settings: the row goes
	c.do("PUT", "/api/items/"+movie+"/langs", map[string]string{"sub_lang": ""}, &l)
	var n int
	st.DB.QueryRow(`SELECT COUNT(*) FROM title_langs`).Scan(&n)
	if l.SubLang != "" || n != 0 {
		t.Errorf("reset: %+v rows=%d", l, n)
	}

	// an episode stands for its series: the next episode sees the choice
	c.do("PUT", "/api/items/"+id("Pilot Part 1")+"/langs", map[string]string{"audio_lang": "hun", "sub_lang": "off"}, &l)
	var series int64
	st.DB.QueryRow(`SELECT series_id FROM items WHERE title='Pilot Part 1'`).Scan(&series)
	if l.TitleID != series || series == 0 {
		t.Fatalf("episode → series: %+v (series %d)", l, series)
	}
	var other langs
	st.DB.QueryRow(`SELECT id FROM items WHERE series_id=? AND kind='episode' AND title<>'Pilot Part 1' LIMIT 1`, series).Scan(&n)
	if c.do("GET", "/api/items/"+itoa(int64(n))+"/langs", nil, &other); other.AudioLang != "hun" || other.SubLang != "off" {
		t.Errorf("other episode: %+v", other)
	}
	if c.do("GET", "/api/items/"+itoa(series)+"/langs", nil, &other); other.TitleID != series || other.SubLang != "off" || len(other.Audio) == 0 {
		t.Errorf("series: %+v", other)
	}
}
