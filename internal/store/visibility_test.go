package store

import "testing"

func TestUserLibraries(t *testing.T) {
	s := testStore(t)
	u, _ := s.CreateUser("demo", "secret12", false)
	adm, _ := s.CreateUser("boss", "secret12", true)
	name := "Demo"
	p, err := s.CreateProfile(u.ID, ProfileInput{Name: &name})
	if err != nil {
		t.Fatal(err)
	}
	ap, _ := s.CreateProfile(adm.ID, ProfileInput{Name: &name})
	demo, _ := s.CreateLibrary("Demo", "/demo", "movies")
	other, _ := s.CreateLibrary("Movies", "/movies", "movies")
	add := func(lib int64, key string) int64 {
		res, err := s.DB.Exec(`INSERT INTO items(library_id,kind,key,title,sort_title,added_at,updated_at) VALUES(?,'movie',?,?,?,?,?)`,
			lib, key, key, key, now(), now())
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	inDemo, inOther := add(demo.ID, "sintel"), add(other.ID, "secret")

	count := func(p *Profile) int {
		items, err := s.QueryItems(p, ` AND i.kind='movie'`, nil, "")
		if err != nil {
			t.Fatal(err)
		}
		return len(items)
	}
	if count(p) != 2 {
		t.Fatal("unrestricted account should see both libraries")
	}

	if err := s.SetUserLibraries(u.ID, []int64{demo.ID}); err != nil {
		t.Fatal(err)
	}
	p, _ = s.Profile(p.ID)
	if count(p) != 1 || !s.ItemVisible(inDemo, p) || s.ItemVisible(inOther, p) {
		t.Fatal("restricted account must see only the Demo library")
	}
	if _, err := s.Item(inOther, p); err != ErrNotFound {
		t.Fatal("item of another library must not load")
	}
	if !p.LibraryAllowed(demo.ID) || p.LibraryAllowed(other.ID) {
		t.Fatal("LibraryAllowed wrong")
	}
	if uu, _ := s.UserByID(u.ID); len(uu.Libraries) != 1 || uu.Libraries[0] != demo.ID {
		t.Fatalf("user libraries not stored: %v", uu.Libraries)
	}

	s.SetUserLibraries(u.ID, []int64{})
	p, _ = s.Profile(p.ID)
	if count(p) != 0 {
		t.Fatal("empty list must hide everything")
	}

	s.SetUserLibraries(u.ID, nil)
	p, _ = s.Profile(p.ID)
	if count(p) != 2 || p.Libraries != nil {
		t.Fatal("nil must lift the restriction")
	}

	// admins are never limited
	s.SetUserLibraries(adm.ID, []int64{demo.ID})
	ap, _ = s.Profile(ap.ID)
	if count(ap) != 2 {
		t.Fatal("admin must see every library")
	}
}
