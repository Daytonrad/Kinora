package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Prowlarr through the API: preview, background apply, progress, and only admins.
func TestArrImportAPI(t *testing.T) {
	c, st := newTestServer(t)
	pw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "pk" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/v1/system/status":
			w.Write([]byte(`{"appName":"Prowlarr","version":"1.28.2"}`))
		case "/api/v1/indexer":
			w.Write([]byte(`[{"id":3,"name":"1337x","protocol":"torrent","enable":true,"priority":25}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer pw.Close()

	var bad map[string]string
	if code := c.do("POST", "/api/admin/import/arr/preview", map[string]any{"app": "sonarr", "url": pw.URL, "api_key": "pk"}, &bad); code != 502 || bad["code"] != "wrong_app" {
		t.Fatalf("wrong app: %d %v", code, bad)
	}
	var plan struct {
		ID       string `json:"id"`
		Indexers []struct {
			Action string `json:"action"`
			URL    string `json:"url"`
		} `json:"indexers"`
	}
	if code := c.do("POST", "/api/admin/import/arr/preview", map[string]any{"app": "prowlarr", "url": pw.URL, "api_key": "pk"}, &plan); code != 200 {
		t.Fatalf("preview %d", code)
	}
	if len(plan.Indexers) != 1 || plan.Indexers[0].Action != "create" || plan.Indexers[0].URL != pw.URL+"/3/api" {
		t.Fatalf("plan: %+v", plan)
	}
	if code := c.do("POST", "/api/admin/import/arr/apply", map[string]any{"plan_id": plan.ID}, nil); code != 202 {
		t.Fatalf("apply %d", code)
	}
	var status struct {
		Started  bool `json:"started"`
		Progress struct {
			Running bool `json:"running"`
			Done    int  `json:"done"`
		} `json:"progress"`
	}
	for i := 0; i < 50; i++ {
		c.do("GET", "/api/admin/import/arr/"+plan.ID, nil, &status)
		if status.Started && !status.Progress.Running {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if status.Progress.Running || status.Progress.Done != 1 {
		t.Fatalf("status: %+v", status)
	}
	ixs, _ := st.AcqIndexers()
	if len(ixs) != 1 || ixs[0].APIKey != "pk" {
		t.Fatalf("indexers: %+v", ixs)
	}
	if code := c.do("GET", "/api/admin/import/arr/nope", nil, nil); code != 404 {
		t.Fatalf("unknown plan: %d", code)
	}
}
