package acquire

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
)

// Deluge speaks the Deluge web UI's JSON-RPC (/json): auth.login, then the core.* calls of the daemon it is connected to.
type Deluge struct {
	url, pass, savePath string
	hc                  *http.Client
	mu                  sync.Mutex
	seq                 int
	ready               bool
}

// NewDeluge makes a client for the Deluge web UI (default port 8112).
func NewDeluge(cfg ClientConfig, hc *http.Client) (*Deluge, error) {
	u, err := url.Parse(strings.TrimSpace(cfg.URL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("a Deluge címe http:// vagy https:// kezdetű legyen (pl. http://192.168.1.10:8112)")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/json"
	jar, _ := cookiejar.New(nil)
	c := *hc
	c.Jar = jar
	return &Deluge{url: u.String(), pass: cfg.Password, savePath: cfg.SavePath, hc: &c}, nil
}

// Protocol: Deluge downloads torrents.
func (d *Deluge) Protocol() string { return "torrent" }

type delugeErr struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

func (d *Deluge) raw(ctx context.Context, method string, params []any, out any) (*delugeErr, error) {
	d.seq++
	body, _ := json.Marshal(map[string]any{"method": method, "params": params, "id": d.seq})
	req, err := http.NewRequestWithContext(ctx, "POST", d.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("a Deluge nem érhető el: %w", stripURL(err))
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Deluge: HTTP %d", resp.StatusCode)
	}
	var r struct {
		Result json.RawMessage `json:"result"`
		Error  *delugeErr      `json:"error"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, errors.New("Deluge: érthetetlen válasz – a webes felület címe kell (8112-es port)")
	}
	if r.Error != nil {
		return r.Error, nil
	}
	if out != nil && len(r.Result) > 0 {
		if err := json.Unmarshal(r.Result, out); err != nil {
			return nil, fmt.Errorf("Deluge: %w", err)
		}
	}
	return nil, nil
}

// login signs in to the web UI and connects it to a daemon when it is not connected yet.
func (d *Deluge) login(ctx context.Context) error {
	var ok bool
	if e, err := d.raw(ctx, "auth.login", []any{d.pass}, &ok); err != nil {
		return err
	} else if e != nil || !ok {
		return errors.New("hibás Deluge jelszó")
	}
	var connected bool
	if _, err := d.raw(ctx, "web.connected", []any{}, &connected); err != nil {
		return err
	}
	if !connected {
		var hosts [][]any
		if _, err := d.raw(ctx, "web.get_hosts", []any{}, &hosts); err != nil {
			return err
		}
		if len(hosts) == 0 || len(hosts[0]) == 0 {
			return errors.New("a Deluge webes felülete nincs démonhoz kapcsolva, és nem ismer egyet sem")
		}
		if e, err := d.raw(ctx, "web.connect", []any{hosts[0][0]}, nil); err != nil {
			return err
		} else if e != nil {
			return fmt.Errorf("Deluge: %s", e.Message)
		}
	}
	d.ready = true
	return nil
}

func (d *Deluge) call(ctx context.Context, method string, params []any, out any) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for attempt := 0; attempt < 2; attempt++ {
		if !d.ready {
			if err := d.login(ctx); err != nil {
				return err
			}
		}
		e, err := d.raw(ctx, method, params, out)
		if err != nil {
			return err
		}
		if e == nil {
			return nil
		}
		if e.Code == 1 && attempt == 0 { // not authenticated: the session ran out
			d.ready = false
			continue
		}
		return fmt.Errorf("Deluge: %s", e.Message)
	}
	return errors.New("Deluge: a belépés nem tartott meg")
}

// Test signs in and returns the daemon's version.
func (d *Deluge) Test(ctx context.Context) (string, error) {
	d.mu.Lock()
	d.ready = false
	d.mu.Unlock()
	// Deluge 2 answers daemon.get_version, Deluge 1 daemon.info; signing in and connecting is the real test
	var v string
	var err error
	for _, method := range []string{"daemon.get_version", "daemon.info"} {
		if err = d.call(ctx, method, []any{}, &v); err == nil {
			return "Deluge " + v, nil
		}
	}
	if strings.Contains(err.Error(), "Unknown method") {
		return "Deluge", nil
	}
	return "", err
}

// Add starts a torrent from a magnet link or a .torrent file.
func (d *Deluge) Add(ctx context.Context, r AddRequest) (string, error) {
	if r.NZB != nil {
		return "", errors.New("a Deluge nem tölt le Usenetről")
	}
	opts := map[string]any{"add_paused": r.Paused}
	save := r.SavePath
	if save == "" {
		save = d.savePath
	}
	if save != "" {
		opts["download_location"] = save
	}
	var hash *string
	var err error
	if r.Magnet != "" {
		err = d.call(ctx, "core.add_torrent_magnet", []any{r.Magnet, opts}, &hash)
	} else {
		err = d.call(ctx, "core.add_torrent_file", []any{r.InfoHash + ".torrent", base64.StdEncoding.EncodeToString(r.Torrent), opts}, &hash)
	}
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "already") { // the torrent is in the session already
			return r.InfoHash, nil
		}
		return "", err
	}
	if hash != nil && *hash != "" {
		return strings.ToLower(*hash), nil
	}
	return r.InfoHash, nil
}

// Status reports the torrents with these hashes.
func (d *Deluge) Status(ctx context.Context, ids []string) (map[string]TorrentStatus, error) {
	out := map[string]TorrentStatus{}
	if len(ids) == 0 {
		return out, nil
	}
	var r map[string]struct {
		Name     string  `json:"name"`
		Progress float64 `json:"progress"` // 0–100
		State    string  `json:"state"`
		Rate     float64 `json:"download_payload_rate"`
		ETA      float64 `json:"eta"`
		Path     string  `json:"save_path"`
		Size     int64   `json:"total_size"`
		Finished bool    `json:"is_finished"`
	}
	fields := []string{"name", "progress", "state", "download_payload_rate", "eta", "save_path", "total_size", "is_finished"}
	if err := d.call(ctx, "core.get_torrents_status", []any{map[string]any{"id": ids}, fields}, &r); err != nil {
		return nil, err
	}
	for h, x := range r {
		h = strings.ToLower(h)
		eta := int64(x.ETA)
		if eta <= 0 {
			eta = -1
		}
		out[h] = TorrentStatus{Hash: h, Name: x.Name, State: x.State, Progress: x.Progress / 100, Size: x.Size, Speed: int64(x.Rate), ETA: eta,
			ContentPath: strings.TrimRight(x.Path, "/\\") + "/" + x.Name,
			Done:        x.Finished && x.State != "Checking" && x.State != "Moving",
			Failed:      x.State == "Error"}
	}
	return out, nil
}

// Remove deletes a torrent, with or without its files.
func (d *Deluge) Remove(ctx context.Context, id string, deleteFiles bool) error {
	return d.call(ctx, "core.remove_torrent", []any{id, deleteFiles}, nil)
}
