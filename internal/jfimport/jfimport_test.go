package jfimport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ScotyLucas/kinora/internal/store"
)

// fakeJellyfin serves the part of the Jellyfin API the importer reads, from fixtures shaped like the real
// answers. legacyGone makes /Users/{id}/Items answer 404, like a server that only has /Items?userId=.
type fakeJellyfin struct {
	key        string
	users      []User
	items      map[string][]Item // per user id
	legacyGone bool
	pages      int // item pages served
}

func (f *fakeJellyfin) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Authorization"), `Token="`+f.key+`"`) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		send := func(v any) { json.NewEncoder(w).Encode(v) }
		switch {
		case r.URL.Path == "/System/Info":
			send(map[string]any{"ServerName": "Otthoni Jellyfin", "Version": "10.10.7", "Id": "abc123"})
		case r.URL.Path == "/Users":
			send(f.users)
		case strings.HasPrefix(r.URL.Path, "/Users/") && strings.HasSuffix(r.URL.Path, "/Items"):
			if f.legacyGone {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			f.page(w, r, strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/Users/"), "/Items"))
		case r.URL.Path == "/Items":
			f.page(w, r, r.URL.Query().Get("userId"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func (f *fakeJellyfin) page(w http.ResponseWriter, r *http.Request, user string) {
	f.pages++
	all := f.items[user]
	start, _ := strconv.Atoi(r.URL.Query().Get("StartIndex"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("Limit"))
	end := min(start+limit, len(all))
	if start > len(all) {
		start = len(all)
	}
	json.NewEncoder(w).Encode(map[string]any{"Items": all[start:end], "TotalRecordCount": len(all)})
}

func ip(n int) *int { return &n }

// library builds a small Kinora library: two films, a series with three episodes (one double).
type library struct {
	st                                   *store.Store
	matrix, inception, dark              int64
	darkE1, darkE2, darkE34, otherSeries int64
	root                                 string
}

func newLibrary(t *testing.T) *library {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "k.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	lib, err := st.CreateLibrary("Filmek", dir, "mixed")
	if err != nil {
		t.Fatal(err)
	}
	l := &library{st: st, root: "/media"}
	ins := func(kind, key, title, path string, year, tmdb int64, imdb string, series *int64, season, ep, epEnd *int) int64 {
		res, err := st.DB.Exec(`INSERT INTO items(library_id,kind,key,title,sort_title,path,year,tmdb_id,imdb_id,series_id,season_num,episode_num,episode_end,added_at,updated_at)
			VALUES(?,?,?,?,?,?,NULLIF(?,0),?,?,?,?,?,?,1,1)`, lib.ID, kind, key, title, title, path, year, tmdb, imdb, series, season, ep, epEnd)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	l.matrix = ins("movie", "m1", "The Matrix", "/media/movies/The Matrix (1999)/The Matrix (1999).mkv", 1999, 603, "tt0133093", nil, nil, nil, nil)
	l.inception = ins("movie", "m2", "Inception", "/media/movies/Inception (2010)/Inception.mkv", 2010, -1, "", nil, nil, nil, nil)
	l.dark = ins("series", "s1", "Dark", "", 2017, 70523, "tt5753856", nil, nil, nil, nil)
	l.darkE1 = ins("episode", "e1", "Secrets", "/media/shows/Dark/Season 01/Dark.S01E01.mkv", 0, 0, "", &l.dark, ip(1), ip(1), nil)
	l.darkE2 = ins("episode", "e2", "Lies", "/media/shows/Dark/Season 01/Dark.S01E02.mkv", 0, 0, "", &l.dark, ip(1), ip(2), nil)
	l.darkE34 = ins("episode", "e3", "Past and Present", "/media/shows/Dark/Season 01/Dark.S01E03E04.mkv", 0, 0, "", &l.dark, ip(1), ip(3), ip(4))
	l.otherSeries = ins("series", "s2", "Local Show", "", 2020, -1, "", nil, nil, nil, nil)
	ins("episode", "e9", "Pilot", "/media/shows/Local Show/Season 01/Local.Show.S01E01.mkv", 0, 0, "", &l.otherSeries, ip(1), ip(1), nil)
	return l
}

// fixture: Anna (exists in Kinora as "anna"), Béla (new), a disabled account.
func newFake(l *library) *fakeJellyfin {
	ud := func(played bool, pos int64, fav bool, count int, last string) UserData {
		return UserData{Played: played, PlaybackPositionTicks: pos, IsFavorite: fav, PlayCount: count, LastPlayedDate: last}
	}
	f := &fakeJellyfin{key: "secret-key", items: map[string][]Item{}}
	f.users = []User{{ID: "u-anna", Name: "Anna"}, {ID: "u-bela", Name: "Béla/2"}, {ID: "u-off", Name: "Régi"}}
	f.users[0].Policy.IsAdministrator = true
	f.users[2].Policy.IsDisabled = true
	darkJF := Item{ID: "jf-dark", Name: "Dark", Type: "Series", ProductionYear: 2017, ProviderIDs: map[string]string{"Tmdb": "70523"}, UserData: ud(false, 0, true, 0, "")}
	f.items["u-anna"] = []Item{
		// by path, through the path mapping (Jellyfin sees /data/…)
		{ID: "jf-matrix", Name: "The Matrix", Type: "Movie", Path: "/data/movies/The Matrix (1999)/The Matrix (1999).mkv", ProductionYear: 1999,
			UserData: ud(true, 0, false, 2, "2024-03-01T20:15:00.0000000Z")},
		// by title + year (no ids, path elsewhere)
		{ID: "jf-inception", Name: "Inception", Type: "Movie", Path: "/old/Inception.mkv", ProductionYear: 2010, UserData: ud(false, 45*60*1e7, false, 0, "")},
		// not in Kinora at all
		{ID: "jf-missing", Name: "Nosferatu", Type: "Movie", ProductionYear: 1922, ProviderIDs: map[string]string{"Tmdb": "653"}, UserData: ud(true, 0, false, 1, "")},
		// nothing to import: skipped silently
		{ID: "jf-plain", Name: "Plain", Type: "Movie", UserData: ud(false, 0, false, 0, "")},
		darkJF,
		// episodes through the series' TMDB id, a double episode, and one Kinora lacks
		{ID: "jf-d1", Name: "Secrets", Type: "Episode", SeriesID: "jf-dark", SeriesName: "Dark", ParentIndexNumber: ip(1), IndexNumber: ip(1), Path: "/elsewhere/1.mkv", UserData: ud(true, 0, false, 1, "")},
		{ID: "jf-d4", Name: "Double Life", Type: "Episode", SeriesID: "jf-dark", SeriesName: "Dark", ParentIndexNumber: ip(1), IndexNumber: ip(4), Path: "/elsewhere/4.mkv", UserData: ud(false, 600*1e7, false, 0, "")},
		{ID: "jf-d9", Name: "Beginnings", Type: "Episode", SeriesID: "jf-dark", SeriesName: "Dark", ParentIndexNumber: ip(3), IndexNumber: ip(9), UserData: ud(true, 0, false, 1, "")},
		{ID: "jf-nonum", Name: "Special", Type: "Episode", SeriesID: "jf-dark", SeriesName: "Dark", UserData: ud(true, 0, false, 1, "")},
	}
	f.items["u-bela"] = []Item{
		// a series Kinora has without ids: found through the episode's folder
		{ID: "jf-local1", Name: "Pilot", Type: "Episode", SeriesID: "jf-local", SeriesName: "Lokális Sorozat", ParentIndexNumber: ip(1), IndexNumber: ip(1),
			Path: `D:\Shows\Local Show\Season 01\Local Show - 1x01.mkv`, UserData: ud(true, 0, true, 3, "")},
		{ID: "jf-local", Name: "Lokális Sorozat", Type: "Series", UserData: ud(false, 0, false, 0, "")},
	}
	f.items["u-off"] = []Item{{ID: "jf-matrix", Name: "The Matrix", Type: "Movie", UserData: ud(true, 0, false, 1, "")}}
	return f
}

var maps = []PathMap{{From: "/data/", To: "/media"}, {From: `D:\Shows`, To: "/media/shows"}}

func TestPreviewAndApply(t *testing.T) {
	l := newLibrary(t)
	if _, err := l.st.CreateUser("anna", "secret123", false); err != nil {
		t.Fatal(err)
	}
	f := newFake(l)
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()
	c, err := NewClient(srv.URL+"/", f.key)
	if err != nil {
		t.Fatal(err)
	}
	c.PageSize = 2 // exercise paging

	// the preview writes nothing
	before := count(t, l.st, `SELECT COUNT(*) FROM user_data`) + count(t, l.st, `SELECT COUNT(*) FROM users`)
	p, err := Preview(context.Background(), c, l.st, maps)
	if err != nil {
		t.Fatal(err)
	}
	if after := count(t, l.st, `SELECT COUNT(*) FROM user_data`) + count(t, l.st, `SELECT COUNT(*) FROM users`); after != before {
		t.Fatal("preview wrote to the database")
	}
	if p.ServerName != "Otthoni Jellyfin" || len(p.Users) != 3 {
		t.Fatalf("plan: %+v", p)
	}
	byName := map[string]*UserPlan{}
	for _, u := range p.Users {
		byName[u.Name] = u
	}
	anna, bela, off := byName["Anna"], byName["Béla/2"], byName["Régi"]
	if anna.Action != ActionExisting || bela.Action != ActionCreate || off.Action != ActionSkip {
		t.Fatalf("actions: %s %s %s", anna.Action, bela.Action, off.Action)
	}
	if bela.KinoraName != "Béla_2" {
		t.Fatalf("a name with / must be made valid: %q", bela.KinoraName)
	}
	got := map[string]Entry{}
	for _, e := range anna.Entries {
		got[e.JellyfinID] = e
	}
	want := map[string]string{
		"jf-matrix": ByPath, "jf-inception": ByTitle, "jf-dark": ByTmdb, "jf-d1": ByTmdb, "jf-d4": ByTmdb,
		"jf-missing": NoMatch, "jf-d9": NoEpisode, "jf-nonum": NoEpisodeNumbers,
	}
	for id, w := range want {
		e, ok := got[id]
		if !ok {
			t.Fatalf("%s missing from the plan", id)
		}
		if (e.Match != nil && e.Method != w) || (e.Match == nil && e.Reason != w) {
			t.Errorf("%s: method %q reason %q, want %q", id, e.Method, e.Reason, w)
		}
	}
	if _, ok := got["jf-plain"]; ok {
		t.Error("an item without state must not be listed")
	}
	if got["jf-d4"].Match.ID != l.darkE34 {
		t.Error("episode 4 is the second half of the double episode S01E03E04")
	}
	if anna.Matched != 5 || anna.Unmatched != 3 || p.ByReason[NoMatch] != 1 {
		t.Fatalf("counts: %d matched, %d unmatched, %v", anna.Matched, anna.Unmatched, p.ByReason)
	}
	if e := bela.Entries[0]; e.Match == nil || e.Match.ID != l.otherSeries+1 || e.Method != ByPath {
		t.Fatalf("episode by series folder: %+v", e)
	}

	// apply
	rep, err := Apply(l.st, p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Users) != 2 {
		t.Fatalf("users applied: %d (the disabled one is skipped)", len(rep.Users))
	}
	var annaRes, belaRes *UserResult
	for _, u := range rep.Users {
		if u.Name == "Anna" {
			annaRes = u
		} else {
			belaRes = u
		}
	}
	if annaRes.Created || annaRes.PasswordLink != "" || annaRes.Changed != 5 {
		t.Fatalf("anna: %+v", annaRes)
	}
	if !belaRes.Created || belaRes.PasswordLink == "" || belaRes.KinoraName != "Béla_2" {
		t.Fatalf("béla: %+v", belaRes)
	}
	annaUser, _ := l.st.UserByName("anna")
	prof, _ := l.st.FirstProfile(annaUser.ID)
	if d := l.st.GetUserData(prof.ID, l.matrix); !d.Played || d.PlayCount != 2 || d.LastPlayedAt == nil || *d.LastPlayedAt != 1709324100 {
		t.Fatalf("matrix state: %+v", d)
	}
	if d := l.st.GetUserData(prof.ID, l.inception); d.Played || d.Position != 2700 {
		t.Fatalf("inception resume position: %+v", d)
	}
	if d := l.st.GetUserData(prof.ID, l.dark); !d.Favorite {
		t.Fatal("series favourite")
	}
	if d := l.st.GetUserData(prof.ID, l.darkE34); d.Position != 600 {
		t.Fatalf("double episode position: %+v", d)
	}
	if n := count(t, l.st, `SELECT COUNT(*) FROM watch_history WHERE profile_id=?`, prof.ID); n < 2 {
		t.Fatalf("watch history entries: %d", n)
	}

	// the new account cannot sign in until the link was used
	belaUser, err := l.st.ImportedUser(Source, "u-bela")
	if err != nil || belaUser.HasPassword() {
		t.Fatalf("béla: %+v %v", belaUser, err)
	}
	if _, err := l.st.Authenticate("Béla_2", ""); err == nil {
		t.Fatal("an account without a password must not sign in")
	}

	// a second run is idempotent: same accounts, nothing changes, no new users
	users := count(t, l.st, `SELECT COUNT(*) FROM users`)
	p2, err := Preview(context.Background(), c, l.st, maps)
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range p2.Users {
		if u.Name == "Béla/2" && (u.Action != ActionLinked || u.KinoraID != belaUser.ID) {
			t.Fatalf("second preview of béla: %+v", u)
		}
	}
	rep2, err := Apply(l.st, p2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep2.Changed != 0 || count(t, l.st, `SELECT COUNT(*) FROM users`) != users {
		t.Fatalf("second apply changed %d items, users %d → %d", rep2.Changed, users, count(t, l.st, `SELECT COUNT(*) FROM users`))
	}
	// béla still has no password: the second run gives a fresh link (the old one stops working)
	var belaRes2 *UserResult
	for _, u := range rep2.Users {
		if u.JellyfinID == "u-bela" {
			belaRes2 = u
		}
	}
	if belaRes2.PasswordLink == "" || belaRes2.PasswordLink == belaRes.PasswordLink {
		t.Fatal("a repeated import must issue a new link for an account still without a password")
	}
	if _, err := l.st.PasswordLinkUser(belaRes.PasswordLink); err == nil {
		t.Fatal("the old link must stop working")
	}
	u, err := l.st.UsePasswordLink(belaRes2.PasswordLink, "jelszo123")
	if err != nil || u.ID != belaUser.ID {
		t.Fatalf("use link: %v", err)
	}
	if _, err := l.st.UsePasswordLink(belaRes2.PasswordLink, "masik123"); !errors.Is(err, store.ErrLinkInvalid) {
		t.Fatal("a link works once")
	}
	if _, err := l.st.Authenticate("Béla_2", "jelszo123"); err != nil {
		t.Fatalf("sign in with the new password: %v", err)
	}
}

// Imported state never overwrites what Kinora already knows.
func TestMergeKeepsKinoraState(t *testing.T) {
	l := newLibrary(t)
	u, _ := l.st.CreateUser("x", "secret123", false)
	prof, _ := l.st.FirstProfile(u.ID)
	l.st.ReportProgress(prof.ID, &store.Item{ID: l.inception, Kind: "movie", RuntimeSec: 9000}, 1200, false)
	l.st.SetFlag(prof.ID, l.inception, "favorite", true)
	changed, err := l.st.MergeUserData(prof.ID, l.inception, store.ImportedUserData{Position: 4000, Favorite: false, PlayCount: 0})
	if err != nil {
		t.Fatal(err)
	}
	d := l.st.GetUserData(prof.ID, l.inception)
	if changed || d.Position != 1200 || !d.Favorite {
		t.Fatalf("changed %v, state %+v", changed, d)
	}
}

func TestOnlySelectedUsers(t *testing.T) {
	l := newLibrary(t)
	f := newFake(l)
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()
	c, _ := NewClient(srv.URL, f.key)
	p, err := Preview(context.Background(), c, l.st, maps)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := Apply(l.st, p, []string{"u-bela"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Users) != 1 || rep.Users[0].JellyfinID != "u-bela" {
		t.Fatalf("users: %+v", rep.Users)
	}
	if _, err := l.st.UserByName("Anna"); err == nil {
		t.Fatal("Anna was not selected")
	}
}

func TestNewerServerWithoutLegacyItems(t *testing.T) {
	l := newLibrary(t)
	f := newFake(l)
	f.legacyGone = true
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()
	c, _ := NewClient(srv.URL, f.key)
	p, err := Preview(context.Background(), c, l.st, maps)
	if err != nil {
		t.Fatal(err)
	}
	if p.Matched == 0 {
		t.Fatal("the /Items?userId= fallback returned nothing")
	}
}

func TestClientErrors(t *testing.T) {
	if _, err := NewClient("ftp://x", "k"); !errors.Is(err, ErrBadURL) {
		t.Fatalf("scheme: %v", err)
	}
	if _, err := NewClient("http://x", " "); !errors.Is(err, ErrNoKey) {
		t.Fatalf("key: %v", err)
	}
	l := newLibrary(t)
	f := newFake(l)
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()
	c, _ := NewClient(srv.URL, "wrong")
	if _, err := Preview(context.Background(), c, l.st, nil); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("wrong key: %v", err)
	}
	other := httptest.NewServer(http.NotFoundHandler())
	defer other.Close()
	c, _ = NewClient(other.URL, "k")
	if _, err := Preview(context.Background(), c, l.st, nil); !errors.Is(err, ErrNotJellyfin) {
		t.Fatalf("not jellyfin: %v", err)
	}
}

func TestHelpers(t *testing.T) {
	for in, want := range map[string]string{
		"/data/movies/A.mkv":        "/media/movies/A.mkv",
		"/data":                     "/media",
		"/database/x.mkv":           "/database/x.mkv",
		`D:\Shows\Show\S01\E01.mkv`: "/media/shows/Show/S01/E01.mkv",
		"/elsewhere/a.mkv":          "/elsewhere/a.mkv",
	} {
		if got := apply(in, []PathMap{{From: "/data", To: "/media"}, {From: `D:\Shows\`, To: "/media/shows"}}); got != want {
			t.Errorf("apply(%q) = %q, want %q", in, got, want)
		}
	}
	if got := apply("/data/a.mkv", []PathMap{{From: "/data", To: `E:\Media`}}); got != `E:\Media\a.mkv` {
		t.Errorf("windows target: %q", got)
	}
	for in, want := range map[string]string{"A": "A_", `a/b\c<d>"e'f`: "a_b_c_d__e_f", strings.Repeat("é", 40): strings.Repeat("é", 32)} {
		if got := kinoraName(in); got != want {
			t.Errorf("kinoraName(%q) = %q", in, got)
		}
	}
	if (UserData{LastPlayedDate: "2024-03-01T20:15:00.0000000Z"}).LastPlayed() != 1709324100 ||
		(UserData{LastPlayedDate: "0001-01-01T00:00:00.0000000Z"}).LastPlayed() != 0 {
		t.Error("last played parsing")
	}
}

func count(t *testing.T, st *store.Store, q string, args ...any) int {
	t.Helper()
	var n int
	if err := st.DB.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
