package jfcompat

import "testing"

// TestLatestTypes: /Items/Latest follows IncludeItemTypes (JellyCon's global "TV Shows - Latest" list showed
// films before), and the kids profile's limits still apply.
func TestLatestTypes(t *testing.T) {
	e := newEnv(t)
	tok := e.login("anna", "secret123")
	type dto struct{ Name, Type string }
	var all, eps, films, music []dto
	e.call("GET", "/Users/x/Items/Latest?Limit=20", tok, nil, &all)
	e.call("GET", "/Users/x/Items/Latest?Limit=20&IncludeItemTypes=Episode", tok, nil, &eps)
	e.call("GET", "/Items/Latest?IncludeItemTypes=Movie", tok, nil, &films)
	e.call("GET", "/Items/Latest?IncludeItemTypes=Audio", tok, nil, &music)
	if len(all) != 5 || len(eps) != 2 || len(films) != 3 || len(music) != 0 {
		t.Fatalf("all %v / episodes %v / films %v / audio %v", all, eps, films, music)
	}
	for _, d := range eps {
		if d.Type != "Series" {
			t.Fatalf("episode list: %v", eps)
		}
	}
	for _, d := range films {
		if d.Type != "Movie" {
			t.Fatalf("film list: %v", films)
		}
	}
	kidTok := e.login("anna/Kicsi", "secret123#1234")
	e.call("GET", "/Items/Latest?IncludeItemTypes=Movie", kidTok, nil, &films)
	for _, d := range films {
		if d.Name == "Space Drama" || d.Name == "Kids Cartoon" {
			t.Fatalf("kids latest: %v", films)
		}
	}
}
