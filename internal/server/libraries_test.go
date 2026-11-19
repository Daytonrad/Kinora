package server

import (
	"path/filepath"
	"testing"

	"github.com/ScotyLucas/kinora/internal/store"
)

func TestLibraryUpdate(t *testing.T) {
	c, st := newTestServer(t)
	libs, _ := st.Libraries()
	lib := libs[0]
	var before int
	st.DB.QueryRow(`SELECT COUNT(*) FROM items WHERE library_id=?`, lib.ID).Scan(&before)

	var got store.Library
	if code := c.do("PATCH", "/api/libraries/"+itoa(lib.ID), map[string]any{"name": "  Mozifilmek "}, &got); code != 200 || got.Name != "Mozifilmek" || got.Path != lib.Path {
		t.Fatalf("rename: %d %+v", code, got)
	}
	if code := c.do("PATCH", "/api/libraries/"+itoa(lib.ID), map[string]any{"name": " "}, nil); code != 400 {
		t.Fatalf("empty name: %d", code)
	}
	if code := c.do("PATCH", "/api/libraries/"+itoa(lib.ID), map[string]any{"path": "relative/dir"}, nil); code != 400 {
		t.Fatalf("relative path: %d", code)
	}
	if code := c.do("PATCH", "/api/libraries/"+itoa(lib.ID), map[string]any{"path": filepath.Join(lib.Path, "nope")}, nil); code != 400 {
		t.Fatalf("missing folder: %d", code)
	}
	if code := c.do("PATCH", "/api/libraries/99999", map[string]any{"name": "x"}, nil); code != 404 {
		t.Fatalf("unknown library: %d", code)
	}
	var after int
	st.DB.QueryRow(`SELECT COUNT(*) FROM items WHERE library_id=?`, lib.ID).Scan(&after)
	if after != before {
		t.Fatalf("items changed: %d → %d", before, after)
	}
}
