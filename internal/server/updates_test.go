package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSemver(t *testing.T) {
	for _, c := range []struct {
		a, b string
		less bool
	}{{"v1.0.9", "v1.0.10", true}, {"1.0.10", "v1.0.10", false}, {"v1.2.0", "v1.1.9", false}, {"kinora v0.9.9", "v1.0.0", true}} {
		a, _ := parseSemver(c.a)
		b, _ := parseSemver(c.b)
		if semverLess(a, b) != c.less {
			t.Errorf("%s < %s: want %v", c.a, c.b, c.less)
		}
	}
	if _, ok := parseSemver("dev"); ok {
		t.Error("dev parsed as a version")
	}
}

func TestReleaseChecker(t *testing.T) {
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"tag_name":"v1.0.10","html_url":"https://github.com/ScotyLucas/Kinora/releases/tag/v1.0.10","published_at":"2026-11-01T10:00:00Z"}`))
	}))
	defer gh.Close()
	oldURL, oldVer := ReleaseURL, Version
	ReleaseURL, Version = gh.URL, "v1.0.9"
	defer func() { ReleaseURL, Version = oldURL, oldVer }()

	rc := newReleaseChecker()
	if st := rc.Status(context.Background()); st.Available || st.Disabled {
		t.Fatalf("first call must not block or claim an update: %+v", st)
	}
	var st releaseInfo
	for i := 0; i < 100; i++ {
		if st = rc.Status(context.Background()); st.CheckedAt != 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !st.Available || st.Latest != "v1.0.10" || st.URL == "" || st.Published == 0 {
		t.Fatalf("want v1.0.10 available: %+v", st)
	}
	Version = "v1.0.10"
	if st = rc.Status(context.Background()); st.Available {
		t.Fatalf("same version reported as update: %+v", st)
	}
	Version = "dev"
	if st = rc.Status(context.Background()); !st.Disabled {
		t.Fatalf("dev build must not check: %+v", st)
	}
}
