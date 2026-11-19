package acquire

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const testMagnet = "magnet:?xt=urn:btih:dd8255ecdc7ca55fb0bbf81323d87062db1f6d1c"
const testHash = "dd8255ecdc7ca55fb0bbf81323d87062db1f6d1c"

func TestTransmission(t *testing.T) {
	var mu sync.Mutex
	torrents := map[string]map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if u, p, _ := r.BasicAuth(); u != "admin" || p != "pw" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Header.Get("X-Transmission-Session-Id") != "sess" {
			w.Header().Set("X-Transmission-Session-Id", "sess")
			w.WriteHeader(http.StatusConflict)
			return
		}
		var in struct {
			Method string         `json:"method"`
			Args   map[string]any `json:"arguments"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		out := map[string]any{}
		switch in.Method {
		case "session-get":
			out["version"] = "4.0.6"
		case "torrent-add":
			h := testHash
			if _, ok := in.Args["metainfo"]; ok {
				h = "ABCDEF0123456789ABCDEF0123456789ABCDEF01"
			}
			if _, dup := torrents[strings.ToLower(h)]; dup {
				out["torrent-duplicate"] = map[string]any{"hashString": h}
			} else {
				torrents[strings.ToLower(h)] = map[string]any{"hashString": h, "name": "Movie", "percentDone": 0.5, "status": 4, "rateDownload": 100,
					"eta": 60, "downloadDir": in.Args["download-dir"], "totalSize": 1000, "leftUntilDone": 500, "error": 0}
				out["torrent-added"] = map[string]any{"hashString": h}
			}
		case "torrent-get":
			var list []any
			for _, id := range in.Args["ids"].([]any) {
				if x, ok := torrents[id.(string)]; ok {
					list = append(list, x)
				}
			}
			out["torrents"] = list
		case "torrent-remove":
			delete(torrents, in.Args["ids"].([]any)[0].(string))
		}
		json.NewEncoder(w).Encode(map[string]any{"result": "success", "arguments": out})
	}))
	defer srv.Close()
	ctx := context.Background()
	bad, _ := NewTransmission(ClientConfig{URL: srv.URL, Username: "admin", Password: "x"}, srv.Client())
	if _, err := bad.Test(ctx); err == nil || !strings.Contains(err.Error(), "jelszó") {
		t.Errorf("bad password: %v", err)
	}
	c, err := NewTransmission(ClientConfig{URL: srv.URL, Username: "admin", Password: "pw", SavePath: "/dl"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	if v, err := c.Test(ctx); err != nil || v != "Transmission 4.0.6" {
		t.Fatalf("test: %q %v", v, err)
	}
	id, err := c.Add(ctx, AddRequest{Fetched: Fetched{Magnet: testMagnet, InfoHash: testHash}})
	if err != nil || id != testHash {
		t.Fatalf("add: %q %v", id, err)
	}
	if id2, _ := c.Add(ctx, AddRequest{Fetched: Fetched{Magnet: testMagnet, InfoHash: testHash}}); id2 != testHash {
		t.Errorf("duplicate add: %q", id2)
	}
	id3, _ := c.Add(ctx, AddRequest{Fetched: Fetched{Torrent: []byte("d4:infod4:name1:xee"), InfoHash: "abc"}})
	if id3 != "abcdef0123456789abcdef0123456789abcdef01" {
		t.Errorf("file add id: %q", id3)
	}
	st, err := c.Status(ctx, []string{testHash, "missing"})
	if err != nil || st[testHash].Progress != 0.5 || st[testHash].Done || st[testHash].ContentPath != "/dl/Movie" {
		t.Fatalf("status: %+v %v", st, err)
	}
	torrents[testHash]["percentDone"], torrents[testHash]["leftUntilDone"], torrents[testHash]["status"] = 1, 0, 6
	if st, _ = c.Status(ctx, []string{testHash}); !st[testHash].Done {
		t.Errorf("finished: %+v", st[testHash])
	}
	torrents[testHash]["error"] = 3
	if st, _ = c.Status(ctx, []string{testHash}); !st[testHash].Failed {
		t.Errorf("local error: %+v", st[testHash])
	}
	if err := c.Remove(ctx, testHash, true); err != nil || torrents[testHash] != nil {
		t.Errorf("remove: %v", err)
	}
	if _, err := c.Add(ctx, AddRequest{Fetched: Fetched{NZB: []byte("<nzb/>")}}); err == nil {
		t.Error("an NZB went to Transmission")
	}
}

func TestDeluge(t *testing.T) {
	var mu sync.Mutex
	logged, connected := false, false
	torrents := map[string]map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		var in struct {
			Method string `json:"method"`
			Params []any  `json:"params"`
			ID     int    `json:"id"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		reply := func(res any, e any) {
			json.NewEncoder(w).Encode(map[string]any{"result": res, "error": e, "id": in.ID})
		}
		if in.Method == "auth.login" {
			logged = in.Params[0] == "deluge"
			if logged {
				http.SetCookie(w, &http.Cookie{Name: "_session_id", Value: "s", Path: "/"})
			}
			reply(logged, nil)
			return
		}
		if c, err := r.Cookie("_session_id"); err != nil || c.Value != "s" || !logged {
			reply(nil, map[string]any{"message": "Not authenticated", "code": 1})
			return
		}
		switch in.Method {
		case "web.connected":
			reply(connected, nil)
		case "web.get_hosts":
			reply([]any{[]any{"host1", "127.0.0.1", 58846, "Online"}}, nil)
		case "web.connect":
			connected = in.Params[0] == "host1"
			reply(nil, nil)
		case "daemon.get_version":
			reply("2.1.1", nil)
		case "core.add_torrent_magnet":
			torrents[testHash] = map[string]any{"name": "Show.S01", "progress": 40.0, "state": "Downloading", "download_payload_rate": 2048.0, "eta": 30.0,
				"save_path": in.Params[1].(map[string]any)["download_location"], "total_size": 5000, "is_finished": false}
			reply(testHash, nil)
		case "core.get_torrents_status":
			out := map[string]any{}
			for _, id := range in.Params[0].(map[string]any)["id"].([]any) {
				if x, ok := torrents[id.(string)]; ok {
					out[id.(string)] = x
				}
			}
			reply(out, nil)
		case "core.remove_torrent":
			delete(torrents, in.Params[0].(string))
			reply(true, nil)
		default:
			reply(nil, map[string]any{"message": "unknown " + in.Method, "code": 2})
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	bad, _ := NewDeluge(ClientConfig{URL: srv.URL, Password: "nope"}, srv.Client())
	if _, err := bad.Test(ctx); err == nil || !strings.Contains(err.Error(), "jelszó") {
		t.Errorf("bad password: %v", err)
	}
	c, _ := NewDeluge(ClientConfig{URL: srv.URL, Password: "deluge", SavePath: "/dl"}, srv.Client())
	if v, err := c.Test(ctx); err != nil || v != "Deluge 2.1.1" || !connected {
		t.Fatalf("test: %q %v connected=%v", v, err, connected)
	}
	id, err := c.Add(ctx, AddRequest{Fetched: Fetched{Magnet: testMagnet, InfoHash: testHash}})
	if err != nil || id != testHash {
		t.Fatalf("add: %q %v", id, err)
	}
	st, _ := c.Status(ctx, []string{testHash})
	if s := st[testHash]; s.Progress != 0.4 || s.Speed != 2048 || s.ContentPath != "/dl/Show.S01" || s.Done {
		t.Errorf("status: %+v", s)
	}
	torrents[testHash]["is_finished"], torrents[testHash]["state"], torrents[testHash]["progress"] = true, "Seeding", 100.0
	if st, _ = c.Status(ctx, []string{testHash}); !st[testHash].Done {
		t.Errorf("finished: %+v", st[testHash])
	}
	logged = false // the web session ran out: the client signs in again
	if err := c.Remove(ctx, testHash, false); err != nil || torrents[testHash] != nil {
		t.Errorf("remove after relogin: %v", err)
	}
}

func TestSABnzbd(t *testing.T) {
	var mu sync.Mutex
	var uploaded []byte
	history := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		q := r.URL.Query()
		if q.Get("apikey") != "key123" {
			fmt.Fprint(w, `{"status":false,"error":"API Key Incorrect"}`)
			return
		}
		switch q.Get("mode") {
		case "addfile":
			f, _, err := r.FormFile("name")
			if err == nil {
				uploaded, _ = io.ReadAll(f)
			}
			fmt.Fprint(w, `{"status":true,"nzo_ids":["SABnzbd_nzo_abc"]}`)
		case "queue":
			if q.Get("name") == "delete" {
				fmt.Fprint(w, `{"status":true}`)
				return
			}
			if history {
				fmt.Fprint(w, `{"queue":{"version":"4.3.2","kbpersec":"0","slots":[]}}`)
				return
			}
			fmt.Fprint(w, `{"queue":{"version":"4.3.2","kbpersec":"1024","slots":[{"nzo_id":"SABnzbd_nzo_abc","filename":"Movie.2020.1080p","status":"Downloading","percentage":"25","mb":"4096","timeleft":"0:10:00"}]}}`)
		case "history":
			if q.Get("name") == "delete" {
				fmt.Fprint(w, `{"status":true}`)
				return
			}
			if !history {
				fmt.Fprint(w, `{"history":{"slots":[]}}`)
				return
			}
			fmt.Fprint(w, `{"history":{"slots":[{"nzo_id":"SABnzbd_nzo_abc","name":"Movie.2020.1080p","status":"Completed","storage":"/downloads/complete/Movie.2020.1080p","bytes":4294967296}]}}`)
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	if _, err := NewSABnzbd(ClientConfig{URL: srv.URL}, srv.Client()); err == nil {
		t.Error("no API key accepted")
	}
	bad, _ := NewSABnzbd(ClientConfig{URL: srv.URL, Password: "nope"}, srv.Client())
	if _, err := bad.Test(ctx); err == nil || !strings.Contains(err.Error(), "API-kulcs") {
		t.Errorf("bad key: %v", err)
	}
	c, _ := NewSABnzbd(ClientConfig{URL: srv.URL, Password: "key123", Category: "movies"}, srv.Client())
	if v, err := c.Test(ctx); err != nil || v != "SABnzbd 4.3.2" {
		t.Fatalf("test: %q %v", v, err)
	}
	if _, err := c.Add(ctx, AddRequest{Fetched: Fetched{Magnet: testMagnet}}); err == nil {
		t.Error("a magnet went to SABnzbd")
	}
	id, err := c.Add(ctx, AddRequest{Fetched: Fetched{NZB: []byte("<nzb></nzb>"), InfoHash: "nzb:1", Name: "Movie.2020.1080p"}})
	if err != nil || id != "SABnzbd_nzo_abc" || string(uploaded) != "<nzb></nzb>" {
		t.Fatalf("add: %q %v %q", id, err, uploaded)
	}
	st, _ := c.Status(ctx, []string{id})
	if s := st[id]; s.Progress != 0.25 || s.Speed != 1024*1024 || s.ETA != 600 || s.Done {
		t.Errorf("queue: %+v", s)
	}
	history = true
	st, _ = c.Status(ctx, []string{id})
	if s := st[id]; !s.Done || s.ContentPath != "/downloads/complete/Movie.2020.1080p" {
		t.Errorf("history: %+v", s)
	}
	if err := c.Remove(ctx, id, false); err != nil {
		t.Error(err)
	}
}

func TestNZBGet(t *testing.T) {
	var mu sync.Mutex
	done := false
	var nzb string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if u, p, _ := r.BasicAuth(); u != "nzbget" || p != "tegbzn6789" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var in struct {
			Method string `json:"method"`
			Params []any  `json:"params"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		var res any
		switch in.Method {
		case "version":
			res = "24.3"
		case "append":
			b, _ := base64.StdEncoding.DecodeString(in.Params[1].(string))
			nzb = string(b)
			res = 42
		case "listgroups":
			if done {
				res = []any{}
			} else {
				res = []any{map[string]any{"NZBID": 42, "NZBName": "Show.S01E01", "Status": "DOWNLOADING", "FileSizeMB": 1000, "RemainingSizeMB": 250}}
			}
		case "history":
			if done {
				res = []any{map[string]any{"NZBID": 42, "Name": "Show.S01E01", "Status": "SUCCESS/ALL", "FinalDir": "/downloads/Show.S01E01", "FileSizeMB": 1000}}
			} else {
				res = []any{}
			}
		case "editqueue":
			res = true
		}
		json.NewEncoder(w).Encode(map[string]any{"result": res})
	}))
	defer srv.Close()
	ctx := context.Background()
	bad, _ := NewNZBGet(ClientConfig{URL: srv.URL, Username: "nzbget", Password: "x"}, srv.Client())
	if _, err := bad.Test(ctx); err == nil || !strings.Contains(err.Error(), "jelszó") {
		t.Errorf("bad password: %v", err)
	}
	c, _ := NewNZBGet(ClientConfig{URL: srv.URL, Username: "nzbget", Password: "tegbzn6789"}, srv.Client())
	if v, err := c.Test(ctx); err != nil || v != "NZBGet 24.3" {
		t.Fatalf("test: %q %v", v, err)
	}
	id, err := c.Add(ctx, AddRequest{Fetched: Fetched{NZB: []byte("<nzb/>"), Name: "Show.S01E01"}})
	if err != nil || id != "42" || nzb != "<nzb/>" {
		t.Fatalf("add: %q %v", id, err)
	}
	st, _ := c.Status(ctx, []string{"42"})
	if s := st["42"]; s.Progress != 0.75 || s.Done {
		t.Errorf("queue: %+v", s)
	}
	done = true
	st, _ = c.Status(ctx, []string{"42"})
	if s := st["42"]; !s.Done || s.ContentPath != "/downloads/Show.S01E01" {
		t.Errorf("history: %+v", s)
	}
	if err := c.Remove(ctx, "42", false); err != nil {
		t.Error(err)
	}
}

// A Newznab indexer gives Usenet releases: no seeders, an "nzb:" identity, and the NZB is downloaded by Kinora.
func TestNewznab(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/getnzb" {
			fmt.Fprint(w, `<?xml version="1.0"?><!DOCTYPE nzb><nzb xmlns="http://www.newzbin.com/DTD/2003/nzb"></nzb>`)
			return
		}
		if r.URL.Path == "/notnzb" {
			fmt.Fprint(w, `<html>login</html>`)
			return
		}
		fmt.Fprintf(w, `<?xml version="1.0"?><rss xmlns:newznab="http://www.newznab.com/DTD/2010/feeds/attributes/"><channel>
<item><title>The.Matrix.1999.1080p.BluRay.x264-GRP</title><guid>abc-123</guid><link>%s/getnzb?id=abc</link><size>9000000000</size>
<newznab:attr name="imdb" value="0133093"/></item></channel></rss>`, srv.URL)
	}))
	defer srv.Close()
	tz := &Torznab{HC: srv.Client()}
	res, err := tz.Search(context.Background(), Indexer{Kind: "newznab", Name: "NZB", URL: srv.URL}, Query{IMDb: "tt0133093"})
	if err != nil || len(res) != 1 {
		t.Fatalf("search: %v %v", res, err)
	}
	r := res[0]
	if r.Protocol != "usenet" || r.Seeders != -1 || !strings.HasPrefix(r.InfoHash, "nzb:") || r.IMDb != "tt0133093" {
		t.Errorf("result: %+v", r)
	}
	if _, rej := DefaultProfile().Score(r.Release, r.Size, r.Seeders); rej != "" {
		t.Errorf("unknown seeders rejected a Usenet release: %s", rej)
	}
	f, err := FetchRelease(context.Background(), srv.Client(), r)
	if err != nil || len(f.NZB) == 0 || f.InfoHash != r.InfoHash || f.Name != r.Title {
		t.Fatalf("fetch nzb: %+v %v", f, err)
	}
	r.Link = srv.URL + "/notnzb"
	if _, err := FetchRelease(context.Background(), srv.Client(), r); err == nil {
		t.Error("an HTML page accepted as NZB")
	}
}
