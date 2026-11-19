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
	"net/url"
	"strings"
	"sync"
)

// Transmission speaks the Transmission RPC (2.x–4.x): JSON over HTTP with the X-Transmission-Session-Id handshake.
type Transmission struct {
	url, user, pass, savePath string
	hc                        *http.Client
	mu                        sync.Mutex
	session                   string
}

// NewTransmission makes a client; an address without a path gets /transmission/rpc.
func NewTransmission(cfg ClientConfig, hc *http.Client) (*Transmission, error) {
	u, err := url.Parse(strings.TrimSpace(cfg.URL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("a Transmission címe http:// vagy https:// kezdetű legyen (pl. http://192.168.1.10:9091)")
	}
	if strings.Trim(u.Path, "/") == "" {
		u.Path = "/transmission/rpc"
	}
	return &Transmission{url: u.String(), user: cfg.Username, pass: cfg.Password, savePath: cfg.SavePath, hc: hc}, nil
}

// Protocol: Transmission downloads torrents.
func (t *Transmission) Protocol() string { return "torrent" }

func (t *Transmission) call(ctx context.Context, method string, args any, out any) error {
	body, _ := json.Marshal(map[string]any{"method": method, "arguments": args})
	t.mu.Lock()
	defer t.mu.Unlock()
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequestWithContext(ctx, "POST", t.url, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		if t.session != "" {
			req.Header.Set("X-Transmission-Session-Id", t.session)
		}
		if t.user != "" || t.pass != "" {
			req.SetBasicAuth(t.user, t.pass)
		}
		resp, err := t.hc.Do(req)
		if err != nil {
			return fmt.Errorf("a Transmission nem érhető el: %w", stripURL(err))
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
		resp.Body.Close()
		switch resp.StatusCode {
		case http.StatusConflict: // a new session id to send along
			t.session = resp.Header.Get("X-Transmission-Session-Id")
			continue
		case http.StatusUnauthorized:
			return errors.New("hibás Transmission felhasználónév vagy jelszó")
		case http.StatusOK:
		default:
			return fmt.Errorf("Transmission: HTTP %d", resp.StatusCode)
		}
		var r struct {
			Result    string          `json:"result"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(b, &r); err != nil {
			return errors.New("Transmission: érthetetlen válasz – jó a cím?")
		}
		if r.Result != "success" {
			return fmt.Errorf("Transmission: %s", r.Result)
		}
		if out != nil {
			return json.Unmarshal(r.Arguments, out)
		}
		return nil
	}
	return errors.New("Transmission: a munkamenet nem jött létre")
}

// Test returns the Transmission version.
func (t *Transmission) Test(ctx context.Context) (string, error) {
	var s struct {
		Version string `json:"version"`
	}
	if err := t.call(ctx, "session-get", map[string]any{"fields": []string{"version"}}, &s); err != nil {
		return "", err
	}
	return "Transmission " + s.Version, nil
}

// Add starts a torrent from a magnet link or a .torrent file.
func (t *Transmission) Add(ctx context.Context, r AddRequest) (string, error) {
	if r.NZB != nil {
		return "", errors.New("a Transmission nem tölt le Usenetről")
	}
	args := map[string]any{"paused": r.Paused}
	if r.Magnet != "" {
		args["filename"] = r.Magnet
	} else {
		args["metainfo"] = base64.StdEncoding.EncodeToString(r.Torrent)
	}
	save := r.SavePath
	if save == "" {
		save = t.savePath
	}
	if save != "" {
		args["download-dir"] = save
	}
	if r.Tag != "" {
		args["labels"] = []string{r.Tag}
	}
	var out struct {
		Added *struct {
			Hash string `json:"hashString"`
		} `json:"torrent-added"`
		Dup *struct {
			Hash string `json:"hashString"`
		} `json:"torrent-duplicate"`
	}
	if err := t.call(ctx, "torrent-add", args, &out); err != nil {
		return "", err
	}
	switch {
	case out.Added != nil:
		return strings.ToLower(out.Added.Hash), nil
	case out.Dup != nil:
		return strings.ToLower(out.Dup.Hash), nil
	}
	return r.InfoHash, nil
}

// Status reports the torrents with these hashes.
func (t *Transmission) Status(ctx context.Context, ids []string) (map[string]TorrentStatus, error) {
	out := map[string]TorrentStatus{}
	if len(ids) == 0 {
		return out, nil
	}
	var r struct {
		Torrents []struct {
			Hash        string  `json:"hashString"`
			Name        string  `json:"name"`
			Percent     float64 `json:"percentDone"`
			Status      int     `json:"status"`
			Rate        int64   `json:"rateDownload"`
			ETA         int64   `json:"eta"`
			Dir         string  `json:"downloadDir"`
			Size        int64   `json:"totalSize"`
			Left        int64   `json:"leftUntilDone"`
			Error       int     `json:"error"`
			ErrorString string  `json:"errorString"`
		} `json:"torrents"`
	}
	fields := []string{"hashString", "name", "percentDone", "status", "rateDownload", "eta", "downloadDir", "totalSize", "leftUntilDone", "error", "errorString"}
	if err := t.call(ctx, "torrent-get", map[string]any{"ids": ids, "fields": fields}, &r); err != nil {
		return nil, err
	}
	for _, x := range r.Torrents {
		h := strings.ToLower(x.Hash)
		eta := x.ETA
		if eta < 0 {
			eta = -1
		}
		state := map[int]string{0: "stopped", 1: "check-wait", 2: "checking", 3: "download-wait", 4: "downloading", 5: "seed-wait", 6: "seeding"}[x.Status]
		out[h] = TorrentStatus{Hash: h, Name: x.Name, State: state, Progress: x.Percent, Size: x.Size, Speed: x.Rate, ETA: eta,
			ContentPath: strings.TrimRight(x.Dir, "/\\") + "/" + x.Name,
			Done:        x.Percent >= 1 && x.Left == 0 && x.Status != 1 && x.Status != 2,
			Failed:      x.Error == 3} // 1–2: tracker warnings and errors, 3: a local error (disk, missing files)
	}
	return out, nil
}

// Remove deletes a torrent, with or without its files.
func (t *Transmission) Remove(ctx context.Context, id string, deleteFiles bool) error {
	return t.call(ctx, "torrent-remove", map[string]any{"ids": []string{id}, "delete-local-data": deleteFiles}, nil)
}
