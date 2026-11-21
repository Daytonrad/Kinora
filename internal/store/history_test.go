package store

import "testing"

func TestWatchHistorySurvivesDeletion(t *testing.T) {
	f := fixture(t)
	s := f.s
	s.DB.Exec(`UPDATE items SET tmdb_id=603 WHERE id=?`, f.ids["matrix"])
	s.DB.Exec(`UPDATE items SET tmdb_id=1399 WHERE id=?`, f.ids["show"])

	matrix, _ := s.Item(f.ids["matrix"], f.p)
	if _, err := s.ReportProgress(f.p.ID, matrix, 5900, false); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"ep1", "ep2"} {
		ep, _ := s.Item(f.ids[k], f.p)
		if err := s.SetPlayed(f.p.ID, ep, true); err != nil {
			t.Fatal(err)
		}
	}

	// the library goes, the history stays
	if _, err := s.DB.Exec(`DELETE FROM libraries WHERE id=?`, f.libID); err != nil {
		t.Fatal(err)
	}
	mv := s.WatchedTitles(f.p.ID, "movie", []int64{603, 604}, false)
	if mv[603] == nil || mv[603].At == 0 || mv[604] != nil {
		t.Fatalf("movie history: %+v", mv)
	}
	sr := s.WatchedTitles(f.p.ID, "series", []int64{1399}, true)
	if sr[1399] == nil || sr[1399].Episodes != 2 || sr[1399].Seasons[1] != 2 {
		t.Fatalf("series history: %+v", sr[1399])
	}
	if other := s.WatchedTitles(f.kid.ID, "movie", []int64{603}, false); len(other) != 0 {
		t.Fatalf("another profile sees it: %+v", other)
	}
}

func TestWatchHistoryUnmark(t *testing.T) {
	f := fixture(t)
	s := f.s
	s.DB.Exec(`UPDATE items SET tmdb_id=603 WHERE id=?`, f.ids["matrix"])
	matrix, _ := s.Item(f.ids["matrix"], f.p)
	s.SetPlayed(f.p.ID, matrix, true)
	s.SetPlayed(f.p.ID, matrix, false)
	if w := s.WatchedTitles(f.p.ID, "movie", []int64{603}, false); len(w) != 0 {
		t.Fatalf("unmarked film still watched: %+v", w)
	}
}

func TestWatchHistoryLateTmdbID(t *testing.T) {
	f := fixture(t)
	s := f.s
	matrix, _ := s.Item(f.ids["matrix"], f.p)
	s.SetPlayed(f.p.ID, matrix, true) // no TMDB id yet: only user_data
	s.DB.Exec(`UPDATE items SET tmdb_id=603 WHERE id=?`, f.ids["matrix"])
	if w := s.WatchedTitles(f.p.ID, "movie", []int64{603}, false); w[603] == nil {
		t.Fatal("live user_data not counted")
	}
}
