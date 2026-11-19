package acquire

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ScotyLucas/kinora/internal/store"
)

// A requester chooses the library: films only among films libraries, series only among series ones.
func TestRequestLibraryChoice(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "k.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	st.SetSetting("acq_enabled", "1")
	m := New(st, fakeMovies{}, func(int64) {}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()
	u, _ := st.CreateUser("anna", "secret123", false)
	who := Requester{UserID: u.ID}

	if _, err := m.Request(ctx, who, 603, 0); err == nil || !strings.Contains(err.Error(), "nincs filmkönyvtár") {
		t.Errorf("no films library: %v", err)
	}
	films, _ := st.CreateLibrary("Filmek", filepath.Join(dir, "Filmek"), "movies")
	shows, _ := st.CreateLibrary("Sorozatok", filepath.Join(dir, "Sorozatok"), "shows")
	// one films library: chosen without asking
	if l, err := m.LibraryFor("movie", 0); err != nil || l.ID != films.ID {
		t.Errorf("the only films library: %v %v", l, err)
	}
	kids, _ := st.CreateLibrary("Gyerekfilmek", filepath.Join(dir, "Gyerek"), "movies")
	if _, err := m.Request(ctx, who, 603, 0); err == nil || !strings.Contains(err.Error(), "válaszd ki") {
		t.Errorf("two films libraries, none chosen: %v", err)
	}
	if _, err := m.Request(ctx, who, 603, shows.ID); err == nil || !strings.Contains(err.Error(), "filmkönyvtárba") {
		t.Errorf("a film into a series library: %v", err)
	}
	if _, err := m.RequestSeries(ctx, who, 1399, "all", films.ID); err == nil || !strings.Contains(err.Error(), "sorozatkönyvtárba") {
		t.Errorf("a series into a films library: %v", err)
	}
	q, err := m.Request(ctx, who, 603, kids.ID)
	if err != nil || q.LibraryID == nil || *q.LibraryID != kids.ID {
		t.Fatalf("film request: %+v %v", q, err)
	}
	if back, _ := st.AcqRequest(q.ID); back.LibraryName != "Gyerekfilmek" {
		t.Errorf("stored library: %+v", back)
	}
	// the only series library is used when none is given
	s, err := m.RequestSeries(ctx, who, 1399, "all", 0)
	if err != nil || s.LibraryID == nil || *s.LibraryID != shows.ID {
		t.Fatalf("series request: %+v %v", s, err)
	}
	// the request's library is the one the film is downloaded to
	if l, err := m.LibraryFor("movie", idOf(q.LibraryID)); err != nil || l.ID != kids.ID {
		t.Errorf("fulfil target: %v %v", l, err)
	}
}
