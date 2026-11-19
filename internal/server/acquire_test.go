package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAcquireAdminAPI(t *testing.T) {
	c, st := newTestServer(t)
	var set struct {
		Enabled bool `json:"enabled"`
		Profile struct {
			Resolutions []int `json:"resolutions"`
		} `json:"profile"`
		ImportMode string           `json:"import_mode"`
		Indexers   []map[string]any `json:"indexers"`
		Clients    []map[string]any `json:"clients"`
		Libraries  []map[string]any `json:"libraries"`
	}
	if code := c.do("GET", "/api/admin/acquire/settings", nil, &set); code != 200 {
		t.Fatalf("settings %d", code)
	}
	if set.Enabled || set.ImportMode != "hardlink" || len(set.Profile.Resolutions) == 0 {
		t.Fatalf("defaults: %+v", set)
	}
	// off by default: searching and grabbing are refused
	if code := c.do("GET", "/api/admin/acquire/releases?tmdb_id=603", nil, nil); code != http.StatusConflict {
		t.Errorf("releases while off: %d", code)
	}
	if code := c.do("POST", "/api/admin/acquire/grab", map[string]any{"tmdb_id": 603, "library_id": 1}, nil); code != http.StatusConflict {
		t.Errorf("grab while off: %d", code)
	}

	// an indexer: the API key goes out masked and survives a save with the masked value
	idx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("apikey") != "topsecretkey" {
			fmt.Fprint(w, `<error code="100" description="Invalid API Key"/>`)
			return
		}
		fmt.Fprint(w, `<caps><server title="Jackett" version="0.22"/></caps>`)
	}))
	defer idx.Close()
	var x map[string]any
	if code := c.do("POST", "/api/admin/acquire/indexers", map[string]any{"name": "Jackett", "url": idx.URL + "/torznab", "api_key": "topsecretkey", "enabled": true}, &x); code != 200 {
		t.Fatalf("create indexer %d", code)
	}
	if k := x["api_key"].(string); strings.Contains(k, "topsecret") || !strings.HasSuffix(k, "tkey") {
		t.Errorf("api key not masked: %q", k)
	}
	id := int64(x["id"].(float64))
	x["name"] = "Jackett 2"
	if code := c.do("PATCH", fmt.Sprintf("/api/admin/acquire/indexers/%d", id), x, nil); code != 200 {
		t.Fatalf("update %d", code)
	}
	stored, _ := st.AcqIndexer(id)
	if stored.APIKey != "topsecretkey" || stored.Name != "Jackett 2" {
		t.Errorf("stored: %+v", stored)
	}
	var test map[string]string
	if code := c.do("POST", "/api/admin/acquire/indexers/test", x, &test); code != 200 || test["server"] != "Jackett 0.22" {
		t.Errorf("test with masked key: %d %v", code, test)
	}
	x["api_key"] = "wrong"
	if code := c.do("POST", "/api/admin/acquire/indexers/test", x, nil); code != http.StatusBadGateway {
		t.Errorf("test with a wrong key: %d", code)
	}
	if code := c.do("POST", "/api/admin/acquire/indexers", map[string]any{"name": "x", "url": "ftp://x"}, nil); code != 400 {
		t.Errorf("ftp indexer: %d", code)
	}

	// a download client: path mapping needs both halves
	if code := c.do("POST", "/api/admin/acquire/clients", map[string]any{"url": "http://qb:8080", "path_remote": "/downloads"}, nil); code != 400 {
		t.Errorf("half a mapping: %d", code)
	}
	var cl map[string]any
	if code := c.do("POST", "/api/admin/acquire/clients", map[string]any{"url": "http://qb:8080", "username": "admin", "password": "pw123456", "enabled": true}, &cl); code != 200 {
		t.Fatalf("client %d", code)
	}
	if p := cl["password"].(string); strings.Contains(p, "pw12") || cl["kind"] != "qbittorrent" || cl["name"] != "qBittorrent" {
		t.Errorf("client: %v", cl)
	}

	// switching on, profile validation
	if code := c.do("PUT", "/api/admin/acquire/settings", map[string]any{"enabled": true, "profile": map[string]any{"resolutions": []int{}}}, nil); code != 400 {
		t.Errorf("empty resolutions: %d", code)
	}
	if code := c.do("PUT", "/api/admin/acquire/settings", map[string]any{"enabled": true, "import_mode": "copy"}, &set); code != 200 || !set.Enabled || set.ImportMode != "copy" {
		t.Errorf("enable: %+v", set)
	}
	var q []any
	if code := c.do("GET", "/api/admin/acquire/queue", nil, &q); code != 200 || len(q) != 0 {
		t.Errorf("queue: %d %v", code, q)
	}
	if code := c.do("DELETE", fmt.Sprintf("/api/admin/acquire/indexers/%d", id), nil, nil); code != 200 {
		t.Errorf("delete %d", code)
	}
}
