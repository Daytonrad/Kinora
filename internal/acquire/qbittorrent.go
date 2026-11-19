package acquire

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
)

// AddRequest is one download for a client.
type AddRequest struct {
	Fetched
	Category string
	SavePath string
	Tag      string
	Paused   bool
}

// TorrentStatus is a client's view of one download.
type TorrentStatus struct {
	Hash        string  `json:"hash"`
	Name        string  `json:"name"`
	State       string  `json:"state"`
	Progress    float64 `json:"progress"` // 0..1
	Size        int64   `json:"size"`
	Speed       int64   `json:"speed"` // bytes/s
	ETA         int64   `json:"eta"`   // seconds, -1 unknown
	ContentPath string  `json:"content_path"`
	Done        bool    `json:"done"`
	Failed      bool    `json:"failed"`
}

// DownloadClient is a torrent client Kinora drives (qBittorrent first; Transmission, Deluge later).
type DownloadClient interface {
	Test(ctx context.Context) (string, error)
	Add(ctx context.Context, r AddRequest) error
	Status(ctx context.Context, hashes []string) (map[string]TorrentStatus, error)
	Remove(ctx context.Context, hash string, deleteFiles bool) error
}

// ClientConfig is a stored download client.
type ClientConfig struct {
	ID         int64
	Kind       string // "qbittorrent"
	URL        string
	Username   string
	Password   string
	Category   string
	SavePath   string
	PathRemote string // where the client says files are…
	PathLocal  string // …and where Kinora sees them
}

// NewClient builds the client for a stored configuration.
func NewClient(cfg ClientConfig, hc *http.Client) (DownloadClient, error) {
	switch cfg.Kind {
	case "qbittorrent", "":
		return NewQBittorrent(cfg, hc)
	}
	return nil, fmt.Errorf("ismeretlen letöltőkliens: %s", cfg.Kind)
}

// QBittorrent speaks the qBittorrent WebUI API v2 (qBittorrent 4.1+ and 5.x).
type QBittorrent struct {
	base     string
	user     string
	pass     string
	category string
	savePath string
	hc       *http.Client
	mu       sync.Mutex
	loggedIn bool
}

// NewQBittorrent makes a client with its own cookie jar (the SID session cookie).
func NewQBittorrent(cfg ClientConfig, hc *http.Client) (*QBittorrent, error) {
	u, err := url.Parse(strings.TrimSpace(cfg.URL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("a qBittorrent címe http:// vagy https:// kezdetű legyen (pl. http://192.168.1.10:8080)")
	}
	jar, _ := cookiejar.New(nil)
	c := *hc
	c.Jar = jar
	return &QBittorrent{base: strings.TrimRight(u.String(), "/"), user: cfg.Username, pass: cfg.Password,
		category: cfg.Category, savePath: cfg.SavePath, hc: &c}, nil
}

func (q *QBittorrent) login(ctx context.Context) error {
	form := url.Values{"username": {q.user}, "password": {q.pass}}
	req, _ := http.NewRequestWithContext(ctx, "POST", q.base+"/api/v2/auth/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", q.base) // qBittorrent's CSRF check compares Referer/Origin with its own address
	resp, err := q.hc.Do(req)
	if err != nil {
		return fmt.Errorf("a qBittorrent nem érhető el: %w", stripURL(err))
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	// qBittorrent 5.x: 204 on success, 401 on a wrong password; 4.x: 200 with "Ok." or "Fails."
	text := strings.TrimSpace(string(body))
	switch {
	case resp.StatusCode == http.StatusForbidden:
		return errors.New("a qBittorrent letiltotta ezt a címet a túl sok hibás belépés miatt – várj, vagy indítsd újra")
	case resp.StatusCode == http.StatusUnauthorized || text == "Fails.":
		return errors.New("hibás qBittorrent felhasználónév vagy jelszó")
	case resp.StatusCode == http.StatusNoContent || (resp.StatusCode == http.StatusOK && text == "Ok."):
	default:
		return fmt.Errorf("qBittorrent belépés: HTTP %d", resp.StatusCode)
	}
	q.loggedIn = true
	return nil
}

// do sends one API request, logging in first and once more when the session expired.
func (q *QBittorrent) do(ctx context.Context, method, path string, body func() (io.Reader, string)) ([]byte, int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for attempt := 0; attempt < 2; attempt++ {
		if !q.loggedIn {
			if err := q.login(ctx); err != nil {
				return nil, 0, err
			}
		}
		var rd io.Reader
		ct := ""
		if body != nil {
			rd, ct = body()
		}
		req, err := http.NewRequestWithContext(ctx, method, q.base+path, rd)
		if err != nil {
			return nil, 0, err
		}
		if ct != "" {
			req.Header.Set("Content-Type", ct)
		}
		req.Header.Set("Referer", q.base)
		resp, err := q.hc.Do(req)
		if err != nil {
			return nil, 0, fmt.Errorf("a qBittorrent nem érhető el: %w", stripURL(err))
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if (resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized) && attempt == 0 {
			q.loggedIn = false // session expired (4.x: 403, 5.x: 401)
			continue
		}
		return b, resp.StatusCode, nil
	}
	return nil, 0, errors.New("qBittorrent: a belépés nem tartott meg")
}

func form(v url.Values) func() (io.Reader, string) {
	return func() (io.Reader, string) {
		return strings.NewReader(v.Encode()), "application/x-www-form-urlencoded"
	}
}

// Test logs in and returns the qBittorrent version.
func (q *QBittorrent) Test(ctx context.Context) (string, error) {
	q.mu.Lock()
	q.loggedIn = false
	q.mu.Unlock()
	b, st, err := q.do(ctx, "GET", "/api/v2/app/version", nil)
	if err != nil {
		return "", err
	}
	if st != http.StatusOK {
		return "", fmt.Errorf("qBittorrent: HTTP %d", st)
	}
	return "qBittorrent " + strings.TrimSpace(string(b)), nil
}

func (q *QBittorrent) ensureCategory(ctx context.Context, cat, path string) {
	if cat == "" {
		return
	}
	v := url.Values{"category": {cat}}
	if path != "" {
		v.Set("savePath", path)
	}
	q.do(ctx, "POST", "/api/v2/torrents/createCategory", form(v)) // 409: exists – fine
}

// Add hands a magnet or .torrent file to qBittorrent.
func (q *QBittorrent) Add(ctx context.Context, r AddRequest) error {
	cat := r.Category
	if cat == "" {
		cat = q.category
	}
	save := r.SavePath
	if save == "" {
		save = q.savePath
	}
	q.ensureCategory(ctx, cat, save)
	build := func() (io.Reader, string) {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		if r.Magnet != "" {
			mw.WriteField("urls", r.Magnet)
		} else {
			fw, _ := mw.CreateFormFile("torrents", r.InfoHash+".torrent")
			fw.Write(r.Torrent)
		}
		if cat != "" {
			mw.WriteField("category", cat)
		}
		if save != "" {
			mw.WriteField("savepath", save)
		}
		if r.Tag != "" {
			mw.WriteField("tags", r.Tag)
		}
		if r.Paused {
			mw.WriteField("paused", "true")  // qBittorrent 4.x
			mw.WriteField("stopped", "true") // qBittorrent 5.x
		}
		mw.Close()
		return &buf, mw.FormDataContentType()
	}
	b, st, err := q.do(ctx, "POST", "/api/v2/torrents/add", build)
	if err != nil {
		return err
	}
	text := strings.TrimSpace(string(b))
	if st == http.StatusUnsupportedMediaType || text == "Fails." {
		return errors.New("a qBittorrent nem fogadta el a torrentet (érvénytelen, vagy már szerepel a listájában)")
	}
	if st < 200 || st > 299 {
		return fmt.Errorf("qBittorrent: HTTP %d %s", st, text)
	}
	return nil
}

type qbTorrent struct {
	Hash        string  `json:"hash"`
	Name        string  `json:"name"`
	State       string  `json:"state"`
	Progress    float64 `json:"progress"`
	Size        int64   `json:"size"`
	DLSpeed     int64   `json:"dlspeed"`
	ETA         int64   `json:"eta"`
	ContentPath string  `json:"content_path"`
	SavePath    string  `json:"save_path"`
}

var qbDone = map[string]bool{"uploading": true, "stalledUP": true, "pausedUP": true, "stoppedUP": true, "queuedUP": true,
	"forcedUP": true, "checkingUP": true}
var qbFailed = map[string]bool{"error": true, "missingFiles": true}

// Status returns the downloads with these hashes; a hash missing from the map is gone from the client.
func (q *QBittorrent) Status(ctx context.Context, hashes []string) (map[string]TorrentStatus, error) {
	out := map[string]TorrentStatus{}
	if len(hashes) == 0 {
		return out, nil
	}
	b, st, err := q.do(ctx, "GET", "/api/v2/torrents/info?hashes="+url.QueryEscape(strings.Join(hashes, "|")), nil)
	if err != nil {
		return nil, err
	}
	if st != http.StatusOK {
		return nil, fmt.Errorf("qBittorrent: HTTP %d", st)
	}
	var list []qbTorrent
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, errors.New("qBittorrent: érthetetlen válasz")
	}
	for _, t := range list {
		cp := t.ContentPath
		if cp == "" { // qBittorrent before 4.4 has no content_path
			cp = strings.TrimRight(t.SavePath, "/\\") + "/" + t.Name
		}
		eta := t.ETA
		if eta >= 8640000 { // qBittorrent's "infinity"
			eta = -1
		}
		h := strings.ToLower(t.Hash)
		out[h] = TorrentStatus{Hash: h, Name: t.Name, State: t.State, Progress: t.Progress, Size: t.Size, Speed: t.DLSpeed, ETA: eta,
			ContentPath: cp, Done: t.Progress >= 1 && qbDone[t.State], Failed: qbFailed[t.State]}
	}
	return out, nil
}

// Remove deletes a download from qBittorrent, with or without its files.
func (q *QBittorrent) Remove(ctx context.Context, hash string, deleteFiles bool) error {
	v := url.Values{"hashes": {hash}, "deleteFiles": {fmt.Sprint(deleteFiles)}}
	_, st, err := q.do(ctx, "POST", "/api/v2/torrents/delete", form(v))
	if err != nil {
		return err
	}
	if st < 200 || st > 299 {
		return fmt.Errorf("qBittorrent: HTTP %d", st)
	}
	return nil
}
