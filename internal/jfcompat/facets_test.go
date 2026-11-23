package jfcompat

import (
	"net/url"
	"testing"
)

type facetList struct {
	Items []struct {
		Name, Id, Type string
		ChildCount     int
	}
}

func (l facetList) names() []string {
	var out []string
	for _, f := range l.Items {
		out = append(out, f.Name)
	}
	return out
}

func eqStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestFacets covers /Years, /Tags and /Genres (Kodi's JellyCon asks for the first two; they used to be 404) and
// the TagIds / Tags item filter, including the kids profile's limits.
func TestFacets(t *testing.T) {
	e := newEnv(t)
	tok := e.login("anna", "secret123")
	var views itemList
	e.call("GET", "/UserViews", tok, nil, &views)
	movies, shows := views.find("Filmek"), views.find("Sorozatok")

	var years facetList
	if code, b := e.call("GET", "/Years?IncludeItemTypes=Movie&Recursive=true&ParentId="+movies, tok, nil, &years); code != 200 {
		t.Fatalf("years: %d %s", code, b)
	}
	if got := years.names(); !eqStrings(got, []string{"2015", "2018", "2019"}) || years.Items[0].Type != "Year" {
		t.Fatalf("movie years: %v", got)
	}
	e.call("GET", "/Years?IncludeItemTypes=Series&ParentId="+shows, tok, nil, &years)
	if got := years.names(); !eqStrings(got, []string{"2020"}) {
		t.Fatalf("series years: %v", got)
	}
	// a year from the list filters the items
	var films itemList
	e.call("GET", "/Items?Recursive=true&IncludeItemTypes=Movie&Years=2015,2016", tok, nil, &films)
	if films.TotalRecordCount != 1 || films.find("Space Drama") == "" {
		t.Fatalf("Years filter: %+v", films)
	}

	var tags facetList
	e.call("GET", "/Tags?IncludeItemTypes=Movie&ParentId="+movies, tok, nil, &tags)
	if len(tags.Items) != 1 || tags.Items[0].Name != "Teszt" || len(tags.Items[0].Id) != 32 {
		t.Fatalf("tags: %+v", tags)
	}
	e.call("GET", "/Users/x/Items?Recursive=true&IncludeItemTypes=Movie&TagIds="+tags.Items[0].Id, tok, nil, &films)
	if films.TotalRecordCount != 1 || films.find("Big Test Movie") == "" {
		t.Fatalf("TagIds filter: %+v", films)
	}
	e.call("GET", "/Items?Recursive=true&IncludeItemTypes=Movie&Tags=teszt", tok, nil, &films)
	if films.TotalRecordCount != 1 {
		t.Fatalf("Tags filter: %+v", films)
	}
	e.call("GET", "/Items?Recursive=true&IncludeItemTypes=Movie&TagIds=0123456789abcdef0123456789abcdef", tok, nil, &films)
	if films.TotalRecordCount != 0 {
		t.Fatalf("unknown TagIds: %+v", films)
	}

	// genres follow IncludeItemTypes (JellyCon's global "Movies - Genres" list)
	var genres facetList
	e.call("GET", "/Genres?IncludeItemTypes=Series", tok, nil, &genres)
	for _, g := range genres.names() {
		if g == "Sci-Fi" || g == "Thriller" {
			t.Fatalf("film genre in the series list: %v", genres.names())
		}
	}
	if len(genres.Items) == 0 {
		t.Fatal("no series genres")
	}
	// a type Kinora has none of, and a library id that is not one
	e.call("GET", "/Years?IncludeItemTypes=MusicAlbum", tok, nil, &years)
	if len(years.Items) != 0 {
		t.Fatalf("music years: %v", years.names())
	}
	e.call("GET", "/Tags?ParentId=0123456789abcdef0123456789abcdef", tok, nil, &tags)
	if len(tags.Items) != 0 {
		t.Fatalf("tags of a bad parent: %+v", tags)
	}

	// the kids profile: Space Drama (R, 2015) is hidden, so neither its year nor a tag only it has shows up,
	// and that tag's id filters to nothing
	e.call("GET", "/Items?Recursive=true&IncludeItemTypes=Movie&SearchTerm=Space", tok, nil, &films)
	space, _ := e.srv.ids.decode(nsItem, films.find("Space Drama"))
	if _, err := e.st.SetTags(space, []string{"Felnott"}); err != nil {
		t.Fatal(err)
	}
	e.call("GET", "/Tags", tok, nil, &tags)
	adultID := ""
	for _, tg := range tags.Items {
		if tg.Name == "Felnott" {
			adultID = tg.Id
		}
	}
	if adultID == "" {
		t.Fatalf("adult tag missing for the main profile: %+v", tags)
	}
	kidTok := e.login("anna/Kicsi", "secret123#1234")
	e.call("GET", "/Years?IncludeItemTypes=Movie", kidTok, nil, &years)
	for _, y := range years.names() {
		if y == "2015" {
			t.Fatalf("kids years: %v", years.names())
		}
	}
	e.call("GET", "/Tags", kidTok, nil, &tags)
	for _, tg := range tags.Items {
		if tg.Name == "Felnott" {
			t.Fatalf("kids tags: %+v", tags)
		}
	}
	for _, q := range []string{"TagIds=" + adultID, "Tags=" + url.QueryEscape("Felnott"), "Years=2015"} {
		e.call("GET", "/Items?Recursive=true&"+q, kidTok, nil, &films)
		if films.TotalRecordCount != 0 {
			t.Fatalf("kids %s: %+v", q, films)
		}
	}
	// unauthenticated
	if code, _ := e.call("GET", "/Years", "", nil, nil); code != 401 {
		t.Fatalf("years without a token: %d", code)
	}
}
