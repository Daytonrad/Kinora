// Command runner is the measuring side of the Kinora vs Jellyfin benchmark (scripts/bench). It runs inside the
// compose network and talks to one server at a time through that server's own API, the way its web client does.
// Every subcommand prints one JSON object; bench.sh stores them and `runner report` turns them into Markdown.
//
//	runner scan   -target kinora|jellyfin          first-time setup, add the libraries, time the first scan and a rescan
//	runner api    -target … [-n 100]                sequential latency of the home page, a library page and a search
//	runner load   -target … [-c 16] [-d 60s]        the same mix from c concurrent clients (CPU/RAM sampled by bench.sh)
//	runner play   -target … [-n 5]                  playback start: direct play and transcode, time to the first media bytes
//	runner report -dir results/<run>                Markdown tables from the stored results
//
// It uses the standard library only.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	adminUser = "bench"
	adminPass = "bench-secret-123"
	searchFor = "river"
	pageSize  = 60
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: runner scan|api|load|play|report [flags]")
		os.Exit(2)
	}
	cmd := os.Args[1]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	targetName := fs.String("target", "", "kinora or jellyfin")
	n := fs.Int("n", 100, "repetitions")
	conc := fs.Int("c", 16, "concurrent clients (load)")
	dur := fs.Duration("d", 60*time.Second, "duration (load)")
	dir := fs.String("dir", "", "results directory (report)")
	only := fs.String("only", "", "api/load: only this scenario (home, library or search)")
	fs.Parse(os.Args[2:])

	if cmd == "report" {
		if err := report(*dir, os.Stdout); err != nil {
			fail(err)
		}
		return
	}
	t, err := newTarget(*targetName)
	if err != nil {
		fail(err)
	}
	ctx := context.Background()
	if err := t.waitUp(ctx, 3*time.Minute); err != nil {
		fail(err)
	}
	var out any
	switch cmd {
	case "scan":
		out, err = runScan(ctx, t)
	case "api":
		err = t.login(ctx)
		if err == nil {
			out, err = runAPI(ctx, t, *n, *only)
		}
	case "load":
		err = t.login(ctx)
		if err == nil {
			out, err = runLoad(ctx, t, *conc, *dur, *only)
		}
	case "play":
		err = t.login(ctx)
		if err == nil {
			out, err = runPlay(ctx, t, *n)
		}
	default:
		err = fmt.Errorf("unknown command %q", cmd)
	}
	if err != nil {
		fail(err)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(out)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "runner:", err)
	os.Exit(1)
}

func logf(format string, a ...any) { fmt.Fprintf(os.Stderr, "  · "+format+"\n", a...) }

// ------------------------------------------------------------------ targets

// counts is what a server indexed.
type counts struct{ Movies, Series, Episodes int }

// target is one server, driven through its own public API.
type target interface {
	name() string
	waitUp(ctx context.Context, max time.Duration) error
	setup(ctx context.Context) error        // first-run wizard + admin login
	login(ctx context.Context) error        // log in on an already set-up server
	addLibraries(ctx context.Context) error // movies + shows, scan started by scanAll
	scanAll(ctx context.Context) error      // start a scan of every library
	waitScan(ctx context.Context, since time.Time) error
	count(ctx context.Context) (counts, error)
	scenarios(ctx context.Context) ([]scenario, error)
	playDirect(ctx context.Context) error    // until the first MiB of the file
	playTranscode(ctx context.Context) error // until the first HLS segment has arrived
}

type scenario struct {
	Name string
	Do   func(ctx context.Context) error
}

func newTarget(name string) (target, error) {
	switch name {
	case "kinora":
		return newKinora("http://kinora:8096"), nil
	case "jellyfin":
		return newJellyfin("http://jellyfin:8096"), nil
	}
	return nil, fmt.Errorf("-target must be kinora or jellyfin, not %q", name)
}

// client is a small JSON HTTP helper.
type client struct {
	base   string
	http   *http.Client
	header http.Header
}

func newClient(base string) *client {
	jar, _ := cookiejar.New(nil)
	return &client{base: base, header: http.Header{},
		http: &http.Client{Jar: jar, Timeout: 2 * time.Minute, Transport: &http.Transport{MaxIdleConnsPerHost: 64}}}
}

// do sends a request; a non-2xx status is an error. out may be nil (the body is drained).
func (c *client) do(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	u := path
	if !strings.HasPrefix(u, "http") {
		u = c.base + path
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	for k, v := range c.header {
		req.Header[k] = v
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("%s %s: HTTP %d %s", method, path, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	if out == nil {
		_, err = io.Copy(io.Discard, resp.Body)
		return err
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// fetch reads a URL; maxBytes > 0 stops after that many bytes (a Range request).
func (c *client) fetch(ctx context.Context, path string, maxBytes int64) ([]byte, int, error) {
	u := path
	if !strings.HasPrefix(u, "http") {
		u = c.base + path
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, err
	}
	for k, v := range c.header {
		req.Header[k] = v
	}
	if maxBytes > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", maxBytes-1))
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	var r io.Reader = resp.Body
	if maxBytes > 0 {
		r = io.LimitReader(resp.Body, maxBytes)
	}
	b, err := io.ReadAll(r)
	return b, resp.StatusCode, err
}

func poll(ctx context.Context, max, every time.Duration, done func() (bool, error)) error {
	ctx, cancel := context.WithTimeout(ctx, max)
	defer cancel()
	for {
		ok, err := done()
		if ok {
			return nil
		}
		select {
		case <-ctx.Done():
			if err != nil {
				return fmt.Errorf("timed out: %w", err)
			}
			return errors.New("timed out")
		case <-time.After(every):
		}
	}
}

// ------------------------------------------------------------------ Kinora

type kinora struct {
	c                  *client
	movieLib, showLib  int64
	directID, transcID int64
}

func newKinora(base string) *kinora { return &kinora{c: newClient(base)} }

func (k *kinora) name() string { return "kinora" }

func (k *kinora) waitUp(ctx context.Context, max time.Duration) error {
	return poll(ctx, max, time.Second, func() (bool, error) {
		err := k.c.do(ctx, "GET", "/api/status", nil, nil)
		return err == nil, err
	})
}

func (k *kinora) setup(ctx context.Context) error {
	if err := k.c.do(ctx, "POST", "/api/setup", map[string]string{"server_name": "bench", "username": adminUser, "password": adminPass}, nil); err != nil {
		return err
	}
	return k.selectProfile(ctx)
}

func (k *kinora) login(ctx context.Context) error {
	if err := k.c.do(ctx, "POST", "/api/login", map[string]string{"username": adminUser, "password": adminPass}, nil); err != nil {
		return err
	}
	if err := k.selectProfile(ctx); err != nil {
		return err
	}
	return k.findIDs(ctx)
}

func (k *kinora) selectProfile(ctx context.Context) error {
	var me struct {
		Profiles []struct{ ID int64 } `json:"profiles"`
	}
	if err := k.c.do(ctx, "GET", "/api/me", nil, &me); err != nil {
		return err
	}
	if len(me.Profiles) == 0 {
		return errors.New("kinora: no profile")
	}
	return k.c.do(ctx, "POST", fmt.Sprintf("/api/profiles/%d/select", me.Profiles[0].ID), map[string]any{}, nil)
}

func (k *kinora) addLibraries(ctx context.Context) error {
	return nil // Kinora scans a library as soon as it is added: scanAll adds them
}

func (k *kinora) scanAll(ctx context.Context) error {
	if k.movieLib == 0 {
		var lib struct{ ID int64 }
		if err := k.c.do(ctx, "POST", "/api/libraries", map[string]string{"name": "Movies", "path": "/media/movies", "kind": "movies"}, &lib); err != nil {
			return err
		}
		k.movieLib = lib.ID
		if err := k.c.do(ctx, "POST", "/api/libraries", map[string]string{"name": "Shows", "path": "/media/shows", "kind": "shows"}, &lib); err != nil {
			return err
		}
		k.showLib = lib.ID
		return nil
	}
	for _, id := range []int64{k.movieLib, k.showLib} {
		if err := k.c.do(ctx, "POST", fmt.Sprintf("/api/libraries/%d/scan", id), map[string]any{}, nil); err != nil {
			return err
		}
	}
	return nil
}

func (k *kinora) waitScan(ctx context.Context, since time.Time) error {
	return poll(ctx, 2*time.Hour, 250*time.Millisecond, func() (bool, error) {
		var st []struct {
			Running   bool  `json:"running"`
			StartedAt int64 `json:"started_at"`
		}
		if err := k.c.do(ctx, "GET", "/api/admin/scan", nil, &st); err != nil {
			return false, err
		}
		seen := 0
		for _, s := range st {
			if s.Running {
				return false, nil
			}
			if s.StartedAt >= since.Unix()-1 {
				seen++
			}
		}
		return seen >= 2, nil
	})
}

func (k *kinora) count(ctx context.Context) (counts, error) {
	var d struct {
		Library struct{ Movies, Series, Episodes int } `json:"library"`
	}
	err := k.c.do(ctx, "GET", "/api/admin/dashboard", nil, &d)
	return counts(d.Library), err
}

func (k *kinora) findIDs(ctx context.Context) error {
	var libs []struct {
		ID   int64
		Kind string
	}
	if err := k.c.do(ctx, "GET", "/api/libraries", nil, &libs); err != nil {
		return err
	}
	for _, l := range libs {
		switch l.Kind {
		case "movies":
			k.movieLib = l.ID
		case "shows":
			k.showLib = l.ID
		}
	}
	var hits []struct {
		ID    int64
		Title string
	}
	if err := k.c.do(ctx, "GET", "/api/search?q=Bench", nil, &hits); err != nil {
		return err
	}
	for _, h := range hits {
		switch h.Title {
		case "Bench Direct":
			k.directID = h.ID
		case "Bench Transcode":
			k.transcID = h.ID
		}
	}
	if k.movieLib == 0 || k.directID == 0 || k.transcID == 0 {
		return fmt.Errorf("kinora: library or test files not found (libs %v, hits %v)", libs, hits)
	}
	return nil
}

// what the Kinora web client asks: one /api/home; a library page of 60 sorted by title; the grouped search
func (k *kinora) scenarios(ctx context.Context) ([]scenario, error) {
	return []scenario{
		{"home", func(ctx context.Context) error { return k.c.do(ctx, "GET", "/api/home", nil, nil) }},
		{"library", func(ctx context.Context) error {
			return k.c.do(ctx, "GET", fmt.Sprintf("/api/items?library=%d&sort=title&order=asc&offset=0&limit=%d", k.movieLib, pageSize), nil, nil)
		}},
		{"search", func(ctx context.Context) error {
			return k.c.do(ctx, "GET", "/api/search?group=1&q="+searchFor, nil, nil)
		}},
	}, nil
}

// the browser profile of the Kinora web client (Chrome-like): H.264/VP9, AAC/MP3/Opus, MP4/WebM
var kinoraCaps = map[string]any{"video": []string{"h264", "vp9"}, "audio": []string{"aac", "mp3", "opus"}, "containers": []string{"mp4", "webm"}}

func (k *kinora) playDirect(ctx context.Context) error {
	var p struct{ Mode, URL string }
	if err := k.c.do(ctx, "POST", fmt.Sprintf("/api/items/%d/playback", k.directID), map[string]any{"caps": kinoraCaps}, &p); err != nil {
		return err
	}
	if p.Mode != "direct" {
		return fmt.Errorf("kinora: expected direct play, got %q", p.Mode)
	}
	b, code, err := k.c.fetch(ctx, p.URL, 1<<20)
	if err == nil && (code != 206 || len(b) != 1<<20) {
		err = fmt.Errorf("kinora direct: HTTP %d, %d bytes", code, len(b))
	}
	return err
}

func (k *kinora) playTranscode(ctx context.Context) error {
	var p struct{ Mode, URL, Session string }
	if err := k.c.do(ctx, "POST", fmt.Sprintf("/api/items/%d/playback", k.transcID), map[string]any{"caps": kinoraCaps}, &p); err != nil {
		return err
	}
	defer k.c.do(context.Background(), "DELETE", "/api/hls/"+p.Session, nil, nil)
	if p.Mode != "transcode" {
		return fmt.Errorf("kinora: expected transcode, got %q", p.Mode)
	}
	return firstSegment(ctx, k.c, p.URL)
}

// firstSegment loads an HLS playlist (following a master playlist's first variant) until it names a segment,
// then downloads that segment completely: what a player needs before the first frame.
func firstSegment(ctx context.Context, c *client, playlist string) error {
	base, err := url.Parse(c.base + "/")
	if err != nil {
		return err
	}
	cur, _ := base.Parse(playlist)
	for depth := 0; depth < 3; depth++ {
		var seg string
		err := poll(ctx, 60*time.Second, 50*time.Millisecond, func() (bool, error) {
			b, code, err := c.fetch(ctx, cur.String(), 0)
			if err != nil || code != 200 {
				return false, fmt.Errorf("playlist HTTP %d: %v", code, err)
			}
			for _, line := range strings.Split(string(b), "\n") {
				line = strings.TrimSpace(line)
				if line != "" && !strings.HasPrefix(line, "#") {
					seg = line
					return true, nil
				}
			}
			return false, nil
		})
		if err != nil {
			return err
		}
		next, _ := cur.Parse(seg)
		if strings.Contains(strings.ToLower(next.Path), ".m3u8") {
			cur = next
			continue
		}
		return poll(ctx, 60*time.Second, 50*time.Millisecond, func() (bool, error) {
			b, code, err := c.fetch(ctx, next.String(), 0)
			if err != nil || code != 200 || len(b) == 0 {
				return false, fmt.Errorf("segment HTTP %d, %d bytes: %v", code, len(b), err)
			}
			return true, nil
		})
	}
	return errors.New("no segment in the playlists")
}

// ------------------------------------------------------------------ Jellyfin

type jellyfin struct {
	c                  *client
	userID, token      string
	movieLib, showLib  string
	directID, transcID string
}

func newJellyfin(base string) *jellyfin { return &jellyfin{c: newClient(base)} }

func (j *jellyfin) name() string { return "jellyfin" }

const jfAuth = `MediaBrowser Client="kinora-bench", Device="bench", DeviceId="kinora-bench-1", Version="1.0"`

func (j *jellyfin) waitUp(ctx context.Context, max time.Duration) error {
	j.c.header.Set("Authorization", jfAuth)
	return poll(ctx, max, time.Second, func() (bool, error) {
		err := j.c.do(ctx, "GET", "/System/Info/Public", nil, nil)
		return err == nil, err
	})
}

func (j *jellyfin) setup(ctx context.Context) error {
	steps := []struct {
		method, path string
		body         any
	}{
		{"POST", "/Startup/Configuration", map[string]string{"ServerName": "bench", "UICulture": "en-US", "MetadataCountryCode": "US", "PreferredMetadataLanguage": "en"}},
		{"GET", "/Startup/User", nil},
		{"POST", "/Startup/User", map[string]string{"Name": adminUser, "Password": adminPass}},
		{"POST", "/Startup/RemoteAccess", map[string]bool{"EnableRemoteAccess": true}},
		{"POST", "/Startup/Complete", nil},
	}
	for _, s := range steps {
		// While it starts, Jellyfin runs a temporary server that answers /System/Info/Public but 503s the wizard,
		// then switches to the real one (connections are refused for a moment): both are waited out.
		var stepErr error
		err := poll(ctx, 3*time.Minute, time.Second, func() (bool, error) {
			stepErr = j.c.do(ctx, s.method, s.path, s.body, nil)
			if stepErr != nil && (strings.Contains(stepErr.Error(), "HTTP 503") || !strings.Contains(stepErr.Error(), "HTTP ")) {
				return false, stepErr
			}
			return true, nil
		})
		if err == nil {
			err = stepErr
		}
		if err != nil {
			return err
		}
	}
	return j.auth(ctx)
}

func (j *jellyfin) auth(ctx context.Context) error {
	var res struct {
		AccessToken string
		User        struct{ Id string }
	}
	if err := j.c.do(ctx, "POST", "/Users/AuthenticateByName", map[string]string{"Username": adminUser, "Pw": adminPass}, &res); err != nil {
		return err
	}
	j.token, j.userID = res.AccessToken, res.User.Id
	j.c.header.Set("Authorization", jfAuth+`, Token="`+j.token+`"`)
	return nil
}

func (j *jellyfin) login(ctx context.Context) error {
	if err := j.auth(ctx); err != nil {
		return err
	}
	return j.findIDs(ctx)
}

// Libraries without internet metadata or image providers (Kinora fetches nothing without a TMDB key), but with
// the local image sources: embedded images and the Screen Grabber (Kinora also grabs a frame of every video during
// its scan). Chapter and trickplay images are not made during the scan (Jellyfin's default; Kinora makes neither).
// Everything else is Jellyfin's default.
func jfLibraryOptions(path string, types ...string) map[string]any {
	var to []map[string]any
	for _, t := range types {
		img := []string{"Embedded Image Extractor", "Screen Grabber"}
		if t == "Series" || t == "Season" {
			img = []string{} // folders: Jellyfin has no local frame grabber for them either
		}
		to = append(to, map[string]any{"Type": t, "MetadataFetchers": []string{}, "ImageFetchers": img})
	}
	return map[string]any{"LibraryOptions": map[string]any{
		"Enabled": true, "EnableRealtimeMonitor": true, "EnableInternetProviders": false,
		"EnableChapterImageExtraction": false, "ExtractChapterImagesDuringLibraryScan": false,
		"EnableTrickplayImageExtraction": false, "ExtractTrickplayImagesDuringLibraryScan": false,
		"SaveLocalMetadata": false, "PathInfos": []map[string]string{{"Path": path}}, "TypeOptions": to,
	}}
}

func (j *jellyfin) addLibraries(ctx context.Context) error {
	for _, l := range []struct {
		name, typ, path string
		types           []string
	}{
		{"Movies", "movies", "/media/movies", []string{"Movie"}},
		{"Shows", "tvshows", "/media/shows", []string{"Series", "Season", "Episode"}},
	} {
		q := url.Values{"name": {l.name}, "collectionType": {l.typ}, "refreshLibrary": {"false"}}
		if err := j.c.do(ctx, "POST", "/Library/VirtualFolders?"+q.Encode(), jfLibraryOptions(l.path, l.types...), nil); err != nil {
			return err
		}
	}
	return nil
}

func (j *jellyfin) scanAll(ctx context.Context) error {
	return j.c.do(ctx, "POST", "/Library/Refresh", nil, nil)
}

func (j *jellyfin) waitScan(ctx context.Context, since time.Time) error {
	return poll(ctx, 2*time.Hour, 250*time.Millisecond, func() (bool, error) {
		var tasks []struct {
			Key                 string
			State               string
			LastExecutionResult *struct {
				StartTimeUtc, EndTimeUtc string
				Status                   string
			}
		}
		if err := j.c.do(ctx, "GET", "/ScheduledTasks?isHidden=false", nil, &tasks); err != nil {
			return false, err
		}
		for _, t := range tasks {
			if t.Key != "RefreshLibrary" {
				continue
			}
			r := t.LastExecutionResult
			if t.State != "Idle" || r == nil {
				return false, nil
			}
			started, err := parseJFTime(r.StartTimeUtc)
			if err != nil {
				return false, err
			}
			return !started.Before(since.Add(-2 * time.Second)), nil
		}
		return false, errors.New("jellyfin: no RefreshLibrary task")
	})
}

// parseJFTime reads Jellyfin's UTC timestamps (seven fractional digits, with or without the Z).
func parseJFTime(v string) (time.Time, error) {
	if !strings.HasSuffix(v, "Z") && !strings.ContainsAny(v[min(len(v), 19):], "+-") {
		v += "Z"
	}
	return time.Parse(time.RFC3339Nano, v)
}

func (j *jellyfin) total(ctx context.Context, q string) (int, error) {
	var r struct{ TotalRecordCount int }
	err := j.c.do(ctx, "GET", "/Items?Recursive=true&Limit=0&"+q, nil, &r)
	return r.TotalRecordCount, err
}

func (j *jellyfin) count(ctx context.Context) (counts, error) {
	var c counts
	var err error
	if c.Movies, err = j.total(ctx, "IncludeItemTypes=Movie"); err != nil {
		return c, err
	}
	if c.Series, err = j.total(ctx, "IncludeItemTypes=Series"); err != nil {
		return c, err
	}
	c.Episodes, err = j.total(ctx, "IncludeItemTypes=Episode")
	return c, err
}

func (j *jellyfin) findIDs(ctx context.Context) error {
	var views struct {
		Items []struct{ Id, Name, CollectionType string }
	}
	if err := j.c.do(ctx, "GET", "/UserViews?userId="+j.userID, nil, &views); err != nil {
		return err
	}
	for _, v := range views.Items {
		switch v.CollectionType {
		case "movies":
			j.movieLib = v.Id
		case "tvshows":
			j.showLib = v.Id
		}
	}
	var hits struct{ Items []struct{ Id, Name string } }
	if err := j.c.do(ctx, "GET", "/Items?Recursive=true&IncludeItemTypes=Movie&SearchTerm=Bench&userId="+j.userID, nil, &hits); err != nil {
		return err
	}
	for _, h := range hits.Items { // without metadata providers Jellyfin keeps the folder name: "Bench Direct (2020)"
		switch {
		case strings.HasPrefix(h.Name, "Bench Direct"):
			j.directID = h.Id
		case strings.HasPrefix(h.Name, "Bench Transcode"):
			j.transcID = h.Id
		}
	}
	if j.movieLib == "" || j.directID == "" || j.transcID == "" {
		return fmt.Errorf("jellyfin: library or test files not found (%+v, %+v)", views, hits)
	}
	return nil
}

// what the Jellyfin web client asks for its home page (in parallel), a library page and a search
func (j *jellyfin) scenarios(ctx context.Context) ([]scenario, error) {
	u := "userId=" + j.userID
	fields := "Fields=PrimaryImageAspectRatio,MediaSourceCount&ImageTypeLimit=1&EnableImageTypes=Primary,Backdrop,Thumb"
	home := []string{
		"/UserViews?" + u,
		"/UserItems/Resume?" + u + "&Limit=12&MediaTypes=Video&" + fields,
		"/Items/Latest?" + u + "&Limit=16&ParentId=" + j.movieLib + "&" + fields,
		"/Items/Latest?" + u + "&Limit=16&ParentId=" + j.showLib + "&" + fields,
		"/Shows/NextUp?" + u + "&Limit=24&" + fields,
	}
	return []scenario{
		{"home", func(ctx context.Context) error {
			var wg sync.WaitGroup
			errs := make([]error, len(home))
			for i, p := range home {
				wg.Add(1)
				go func() { defer wg.Done(); errs[i] = j.c.do(ctx, "GET", p, nil, nil) }()
			}
			wg.Wait()
			return errors.Join(errs...)
		}},
		{"library", func(ctx context.Context) error {
			return j.c.do(ctx, "GET", fmt.Sprintf("/Items?%s&ParentId=%s&IncludeItemTypes=Movie&Recursive=true&SortBy=SortName&SortOrder=Ascending&StartIndex=0&Limit=%d&%s",
				u, j.movieLib, pageSize, fields), nil, nil)
		}},
		{"search", func(ctx context.Context) error {
			return j.c.do(ctx, "GET", fmt.Sprintf("/Items?%s&SearchTerm=%s&IncludeItemTypes=Movie,Series,Episode&Recursive=true&Limit=%d&%s",
				u, searchFor, pageSize, fields), nil, nil)
		}},
	}, nil
}

// a browser-like device profile: MP4/WebM direct play, H.264/AAC HLS transcoding
func jfDeviceProfile() map[string]any {
	return map[string]any{
		"MaxStreamingBitrate": 120000000,
		"DirectPlayProfiles": []map[string]string{
			{"Container": "mp4,m4v", "Type": "Video", "VideoCodec": "h264,vp9", "AudioCodec": "aac,mp3,opus"},
			{"Container": "webm", "Type": "Video", "VideoCodec": "vp9", "AudioCodec": "opus"},
		},
		"TranscodingProfiles": []map[string]any{
			{"Container": "ts", "Type": "Video", "VideoCodec": "h264", "AudioCodec": "aac", "Protocol": "hls", "Context": "Streaming", "MaxAudioChannels": "2"},
		},
		"CodecProfiles": []any{}, "SubtitleProfiles": []any{},
	}
}

type jfPlaybackInfo struct {
	PlaySessionId string
	MediaSources  []struct {
		Id                 string
		SupportsDirectPlay bool
		TranscodingUrl     string
	}
}

func (j *jellyfin) playbackInfo(ctx context.Context, id string) (*jfPlaybackInfo, error) {
	var pi jfPlaybackInfo
	err := j.c.do(ctx, "POST", "/Items/"+id+"/PlaybackInfo?userId="+j.userID,
		map[string]any{"UserId": j.userID, "DeviceProfile": jfDeviceProfile(), "AutoOpenLiveStream": true}, &pi)
	if err == nil && len(pi.MediaSources) == 0 {
		err = errors.New("jellyfin: no media source")
	}
	return &pi, err
}

func (j *jellyfin) playDirect(ctx context.Context) error {
	pi, err := j.playbackInfo(ctx, j.directID)
	if err != nil {
		return err
	}
	ms := pi.MediaSources[0]
	if !ms.SupportsDirectPlay {
		return errors.New("jellyfin: expected direct play")
	}
	q := url.Values{"static": {"true"}, "mediaSourceId": {ms.Id}, "playSessionId": {pi.PlaySessionId}, "api_key": {j.token}}
	b, code, err := j.c.fetch(ctx, "/Videos/"+j.directID+"/stream?"+q.Encode(), 1<<20)
	if err == nil && (code != 206 || len(b) != 1<<20) {
		err = fmt.Errorf("jellyfin direct: HTTP %d, %d bytes", code, len(b))
	}
	return err
}

func (j *jellyfin) playTranscode(ctx context.Context) error {
	pi, err := j.playbackInfo(ctx, j.transcID)
	if err != nil {
		return err
	}
	defer j.c.do(context.Background(), "DELETE", "/Videos/ActiveEncodings?deviceId=kinora-bench-1&playSessionId="+pi.PlaySessionId, nil, nil)
	tu := pi.MediaSources[0].TranscodingUrl
	if tu == "" {
		return errors.New("jellyfin: expected a transcoding URL")
	}
	return firstSegment(ctx, j.c, tu)
}

// ------------------------------------------------------------------ measurements

type scanResult struct {
	Target      string  `json:"target"`
	FirstScanS  float64 `json:"first_scan_s"`
	SecondScanS float64 `json:"second_scan_s"`
	Counts      counts  `json:"counts"`
}

func runScan(ctx context.Context, t target) (*scanResult, error) {
	if err := t.setup(ctx); err != nil {
		return nil, fmt.Errorf("setup: %w", err)
	}
	if err := t.addLibraries(ctx); err != nil {
		return nil, fmt.Errorf("libraries: %w", err)
	}
	res := &scanResult{Target: t.name()}
	for i, dst := range []*float64{&res.FirstScanS, &res.SecondScanS} {
		start := time.Now()
		if err := t.scanAll(ctx); err != nil {
			return nil, fmt.Errorf("scan %d: %w", i+1, err)
		}
		if err := t.waitScan(ctx, start); err != nil {
			return nil, fmt.Errorf("scan %d: %w", i+1, err)
		}
		*dst = time.Since(start).Seconds()
		logf("%s scan %d: %.1f s", t.name(), i+1, *dst)
		time.Sleep(3 * time.Second)
	}
	c, err := t.count(ctx)
	res.Counts = c
	return res, err
}

// pick keeps one scenario when only is set.
func pick(scs []scenario, err error) func(only string) ([]scenario, error) {
	return func(only string) ([]scenario, error) {
		if err != nil || only == "" {
			return scs, err
		}
		for _, sc := range scs {
			if sc.Name == only {
				return []scenario{sc}, nil
			}
		}
		return nil, fmt.Errorf("no scenario %q", only)
	}
}

type latency struct {
	N     int     `json:"n"`
	Errs  int     `json:"errors"`
	P50ms float64 `json:"p50_ms"`
	P95ms float64 `json:"p95_ms"`
	P99ms float64 `json:"p99_ms"`
	Mean  float64 `json:"mean_ms"`
}

func summarize(ds []time.Duration, errs int) latency {
	l := latency{N: len(ds), Errs: errs}
	if len(ds) == 0 {
		return l
	}
	sort.Slice(ds, func(a, b int) bool { return ds[a] < ds[b] })
	ms := func(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
	q := func(p float64) float64 { return ms(ds[min(len(ds)-1, int(p*float64(len(ds))))]) }
	var sum time.Duration
	for _, d := range ds {
		sum += d
	}
	l.P50ms, l.P95ms, l.P99ms, l.Mean = q(0.50), q(0.95), q(0.99), ms(sum/time.Duration(len(ds)))
	return l
}

type apiResult struct {
	Target    string             `json:"target"`
	Scenarios map[string]latency `json:"scenarios"`
}

func runAPI(ctx context.Context, t target, n int, only string) (*apiResult, error) {
	scs, err := pick(t.scenarios(ctx))(only)
	if err != nil {
		return nil, err
	}
	res := &apiResult{Target: t.name(), Scenarios: map[string]latency{}}
	for _, sc := range scs {
		for i := 0; i < 5; i++ { // warm-up
			if err := sc.Do(ctx); err != nil {
				return nil, fmt.Errorf("%s: %w", sc.Name, err)
			}
		}
		var ds []time.Duration
		errs := 0
		for i := 0; i < n; i++ {
			start := time.Now()
			if err := sc.Do(ctx); err != nil {
				errs++
				continue
			}
			ds = append(ds, time.Since(start))
		}
		res.Scenarios[sc.Name] = summarize(ds, errs)
		logf("%s %s: p50 %.1f ms", t.name(), sc.Name, res.Scenarios[sc.Name].P50ms)
	}
	return res, nil
}

type loadResult struct {
	Target      string             `json:"target"`
	Clients     int                `json:"clients"`
	DurationS   float64            `json:"duration_s"`
	Requests    int                `json:"requests"`
	ReqPerS     float64            `json:"req_per_s"`
	Scenarios   map[string]latency `json:"scenarios"`
	StartedUnix float64            `json:"started_unix"`
	EndedUnix   float64            `json:"ended_unix"`
}

// runLoad: c clients each loop over the scenarios (home, library, search, home, …) for d.
func runLoad(ctx context.Context, t target, c int, d time.Duration, only string) (*loadResult, error) {
	scs, err := pick(t.scenarios(ctx))(only)
	if err != nil {
		return nil, err
	}
	var mu sync.Mutex
	per := map[string][]time.Duration{}
	errs := map[string]int{}
	start := time.Now()
	lctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	var wg sync.WaitGroup
	for w := 0; w < c; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := w; lctx.Err() == nil; i++ {
				sc := scs[i%len(scs)]
				t0 := time.Now()
				err := sc.Do(lctx)
				el := time.Since(t0)
				if lctx.Err() != nil {
					return // cut off by the end of the run: not counted
				}
				mu.Lock()
				if err != nil {
					errs[sc.Name]++
				} else {
					per[sc.Name] = append(per[sc.Name], el)
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	end := time.Now()
	res := &loadResult{Target: t.name(), Clients: c, DurationS: end.Sub(start).Seconds(), Scenarios: map[string]latency{},
		StartedUnix: float64(start.UnixMilli()) / 1000, EndedUnix: float64(end.UnixMilli()) / 1000}
	for _, sc := range scs {
		l := summarize(per[sc.Name], errs[sc.Name])
		res.Scenarios[sc.Name] = l
		res.Requests += l.N
	}
	res.ReqPerS = float64(res.Requests) / res.DurationS
	logf("%s load: %d requests, %.1f req/s", t.name(), res.Requests, res.ReqPerS)
	return res, nil
}

type playResult struct {
	Target    string  `json:"target"`
	Direct    latency `json:"direct"`
	Transcode latency `json:"transcode"`
}

func runPlay(ctx context.Context, t target, n int) (*playResult, error) {
	res := &playResult{Target: t.name()}
	for _, m := range []struct {
		dst *latency
		do  func(context.Context) error
	}{{&res.Direct, t.playDirect}, {&res.Transcode, t.playTranscode}} {
		if err := m.do(ctx); err != nil { // warm-up (and a check that it works at all)
			return nil, err
		}
		time.Sleep(3 * time.Second)
		var ds []time.Duration
		errs := 0
		for i := 0; i < n; i++ {
			start := time.Now()
			if err := m.do(ctx); err != nil {
				logf("%s play: %v", t.name(), err)
				errs++
			} else {
				ds = append(ds, time.Since(start))
			}
			time.Sleep(3 * time.Second) // let the stopped transcode exit
		}
		*m.dst = summarize(ds, errs)
	}
	logf("%s play: direct p50 %.0f ms, transcode p50 %.0f ms", t.name(), res.Direct.P50ms, res.Transcode.P50ms)
	return res, nil
}
