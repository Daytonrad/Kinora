package server

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestDownloadByStreamToken: a finished offline download can be fetched with the stream token in its file_url
// (the Android app's DownloadManager has no session), and that token opens nothing else.
func TestDownloadByStreamToken(t *testing.T) {
	c, st, srv := newTestServerFull(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go srv.Downloads.Run(ctx)
	selectFirstProfile(t, c)
	var big, space int64
	st.DB.QueryRow(`SELECT id FROM items WHERE title='Big Test Movie'`).Scan(&big)
	st.DB.QueryRow(`SELECT id FROM items WHERE title='Space Drama'`).Scan(&space)

	var job struct{ Key string }
	if code := c.do("POST", "/api/items/"+itoa(big)+"/download", map[string]string{"quality": "360p"}, &job); code != http.StatusAccepted || job.Key == "" {
		t.Fatalf("download job: %d %+v", code, job)
	}
	type jobRow struct {
		Key     string `json:"key"`
		Status  string `json:"status"`
		Error   string `json:"error"`
		FileURL string `json:"file_url"`
	}
	var done jobRow
	for deadline := time.Now().Add(90 * time.Second); time.Now().Before(deadline); time.Sleep(500 * time.Millisecond) {
		var r struct{ Jobs []jobRow }
		c.do("GET", "/api/downloads", nil, &r)
		for _, j := range r.Jobs {
			if j.Key == job.Key {
				done = j
			}
		}
		if done.Status == "done" || done.Status == "error" {
			break
		}
	}
	if done.Status != "done" || !strings.Contains(done.FileURL, "st=") {
		t.Fatalf("finished job: %+v", done)
	}

	anon := &client{t: t, base: c.base, hc: http.DefaultClient}
	if resp, b := anon.raw("GET", done.FileURL, nil); resp.StatusCode != 200 || len(b) < 1000 || resp.Header.Get("Content-Type") != "video/mp4" {
		t.Fatalf("file by token: %d, %d bytes, %s", resp.StatusCode, len(b), resp.Header.Get("Content-Type"))
	}
	// without a token or session: refused
	if resp, _ := anon.raw("GET", "/api/downloads/"+job.Key+"/file", nil); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no token: %d", resp.StatusCode)
	}
	// the stream token of another title does not open this file
	var other struct{ URL string }
	if code := c.do("POST", "/api/items/"+itoa(space)+"/download", map[string]string{"quality": "original"}, &other); code != 200 || !strings.Contains(other.URL, "st=") {
		t.Fatalf("original: %d %+v", code, other)
	}
	tok := other.URL[strings.Index(other.URL, "st=")+3:]
	if resp, _ := anon.raw("GET", "/api/downloads/"+job.Key+"/file?st="+tok, nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("another title's token: %d", resp.StatusCode)
	}
	// the "original" URL works without the session too (a range is enough)
	req, _ := http.NewRequest("GET", c.base+other.URL, nil)
	req.Header.Set("Range", "bytes=0-1023")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("original by token: %d", resp.StatusCode)
	}
}
