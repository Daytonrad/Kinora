package acquire

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Usenet download clients: SABnzbd and NZBGet. Kinora downloads the NZB from the indexer itself and
// uploads it, so the indexer's API key never reaches the client.

func nzbName(r AddRequest) string {
	n := SafeName(r.Name)
	if n == "Film" && r.Name == "" {
		n = strings.TrimPrefix(r.InfoHash, "nzb:")
	}
	return n
}

// ------------------------------------------------------------------ SABnzbd

// SABnzbd speaks the SABnzbd API (?mode=…&apikey=…&output=json). The API key goes in the password field.
type SABnzbd struct {
	base, key, category string
	hc                  *http.Client
}

// NewSABnzbd makes a client (default port 8080, path /sabnzbd when it runs under it).
func NewSABnzbd(cfg ClientConfig, hc *http.Client) (*SABnzbd, error) {
	u, err := url.Parse(strings.TrimSpace(cfg.URL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("a SABnzbd címe http:// vagy https:// kezdetű legyen (pl. http://192.168.1.10:8080)")
	}
	if cfg.Password == "" {
		return nil, errors.New("add meg a SABnzbd API-kulcsát (Config → General → API Key) a jelszó mezőben")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/api"
	return &SABnzbd{base: u.String(), key: cfg.Password, category: cfg.Category, hc: hc}, nil
}

// Protocol: SABnzbd downloads from Usenet.
func (s *SABnzbd) Protocol() string { return "usenet" }

func (s *SABnzbd) do(req *http.Request, out any) error {
	resp, err := s.hc.Do(req)
	if err != nil {
		return fmt.Errorf("a SABnzbd nem érhető el: %w", stripURL(err))
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("SABnzbd: HTTP %d", resp.StatusCode)
	}
	var e struct {
		Status *bool  `json:"status"`
		Error  string `json:"error"`
	}
	if json.Unmarshal(b, &e) != nil {
		return errors.New("SABnzbd: érthetetlen válasz – jó a cím?")
	}
	if e.Error != "" {
		if strings.Contains(strings.ToLower(e.Error), "api key") {
			return errors.New("hibás SABnzbd API-kulcs")
		}
		return fmt.Errorf("SABnzbd: %s", e.Error)
	}
	if e.Status != nil && !*e.Status {
		return errors.New("SABnzbd: a kérés nem sikerült")
	}
	if out != nil {
		return json.Unmarshal(b, out)
	}
	return nil
}

func (s *SABnzbd) get(ctx context.Context, v url.Values, out any) error {
	v.Set("apikey", s.key)
	v.Set("output", "json")
	req, err := http.NewRequestWithContext(ctx, "GET", s.base+"?"+v.Encode(), nil)
	if err != nil {
		return err
	}
	return s.do(req, out)
}

// Test checks the API key and returns the version.
func (s *SABnzbd) Test(ctx context.Context) (string, error) {
	var q struct {
		Queue struct {
			Version string `json:"version"`
		} `json:"queue"`
	}
	if err := s.get(ctx, url.Values{"mode": {"queue"}, "limit": {"1"}}, &q); err != nil {
		return "", err
	}
	return strings.TrimSpace("SABnzbd " + q.Queue.Version), nil
}

// Add uploads the NZB; the job id (nzo_id) follows it.
func (s *SABnzbd) Add(ctx context.Context, r AddRequest) (string, error) {
	if r.NZB == nil {
		return "", errors.New("a SABnzbd csak Usenet-kiadást (NZB) tölt le")
	}
	cat := r.Category
	if cat == "" {
		cat = s.category
	}
	v := url.Values{"mode": {"addfile"}, "apikey": {s.key}, "output": {"json"}, "nzbname": {nzbName(r)}}
	if cat != "" {
		v.Set("cat", cat)
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("name", nzbName(r)+".nzb")
	fw.Write(r.NZB)
	mw.Close()
	req, err := http.NewRequestWithContext(ctx, "POST", s.base+"?"+v.Encode(), &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	var out struct {
		IDs []string `json:"nzo_ids"`
	}
	if err := s.do(req, &out); err != nil {
		return "", err
	}
	if len(out.IDs) == 0 {
		return "", errors.New("SABnzbd: nem kaptam feladat-azonosítót")
	}
	return out.IDs[0], nil
}

// Status looks for the jobs in the queue and in the history.
func (s *SABnzbd) Status(ctx context.Context, ids []string) (map[string]TorrentStatus, error) {
	out := map[string]TorrentStatus{}
	if len(ids) == 0 {
		return out, nil
	}
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	var q struct {
		Queue struct {
			Slots []struct {
				ID       string `json:"nzo_id"`
				Name     string `json:"filename"`
				Status   string `json:"status"`
				Percent  string `json:"percentage"`
				MB       string `json:"mb"`
				TimeLeft string `json:"timeleft"`
			} `json:"slots"`
			KBPerSec string `json:"kbpersec"`
		} `json:"queue"`
	}
	if err := s.get(ctx, url.Values{"mode": {"queue"}, "limit": {"500"}}, &q); err != nil {
		return nil, err
	}
	rate, _ := strconv.ParseFloat(q.Queue.KBPerSec, 64)
	for i, x := range q.Queue.Slots {
		if !want[x.ID] {
			continue
		}
		p, _ := strconv.ParseFloat(x.Percent, 64)
		mb, _ := strconv.ParseFloat(x.MB, 64)
		st := TorrentStatus{Hash: x.ID, Name: x.Name, State: x.Status, Progress: p / 100, Size: int64(mb * (1 << 20)), ETA: hmsSeconds(x.TimeLeft)}
		if i == 0 { // SABnzbd downloads one job at a time: the speed is the first one's
			st.Speed = int64(rate * 1024)
		}
		out[x.ID] = st
	}
	var h struct {
		History struct {
			Slots []struct {
				ID      string `json:"nzo_id"`
				Name    string `json:"name"`
				Status  string `json:"status"`
				Storage string `json:"storage"`
				Bytes   int64  `json:"bytes"`
				Fail    string `json:"fail_message"`
			} `json:"slots"`
		} `json:"history"`
	}
	if err := s.get(ctx, url.Values{"mode": {"history"}, "limit": {"500"}}, &h); err != nil {
		return nil, err
	}
	for _, x := range h.History.Slots {
		if !want[x.ID] {
			continue
		}
		out[x.ID] = TorrentStatus{Hash: x.ID, Name: x.Name, State: x.Status, Progress: 1, Size: x.Bytes, ETA: -1,
			ContentPath: x.Storage, Done: x.Status == "Completed", Failed: x.Status == "Failed"}
	}
	return out, nil
}

// hmsSeconds reads SABnzbd's "1:02:03" time left.
func hmsSeconds(s string) int64 {
	var total int64
	for _, p := range strings.Split(s, ":") {
		n, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			return -1
		}
		total = total*60 + n
	}
	if total == 0 {
		return -1
	}
	return total
}

// Remove deletes the job from the queue or the history.
func (s *SABnzbd) Remove(ctx context.Context, id string, deleteFiles bool) error {
	del := "0"
	if deleteFiles {
		del = "1"
	}
	err := s.get(ctx, url.Values{"mode": {"queue"}, "name": {"delete"}, "value": {id}, "del_files": {del}}, nil)
	if err2 := s.get(ctx, url.Values{"mode": {"history"}, "name": {"delete"}, "value": {id}, "del_files": {del}}, nil); err != nil && err2 != nil {
		return err
	}
	return nil
}

// ------------------------------------------------------------------ NZBGet

// NZBGet speaks NZBGet's JSON-RPC (/jsonrpc) with basic authentication.
type NZBGet struct {
	url, user, pass, category string
	hc                        *http.Client
}

// NewNZBGet makes a client (default port 6789).
func NewNZBGet(cfg ClientConfig, hc *http.Client) (*NZBGet, error) {
	u, err := url.Parse(strings.TrimSpace(cfg.URL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("az NZBGet címe http:// vagy https:// kezdetű legyen (pl. http://192.168.1.10:6789)")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/jsonrpc"
	return &NZBGet{url: u.String(), user: cfg.Username, pass: cfg.Password, category: cfg.Category, hc: hc}, nil
}

// Protocol: NZBGet downloads from Usenet.
func (n *NZBGet) Protocol() string { return "usenet" }

func (n *NZBGet) call(ctx context.Context, method string, params []any, out any) error {
	body, _ := json.Marshal(map[string]any{"method": method, "params": params, "id": 1})
	req, err := http.NewRequestWithContext(ctx, "POST", n.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if n.user != "" || n.pass != "" {
		req.SetBasicAuth(n.user, n.pass)
	}
	resp, err := n.hc.Do(req)
	if err != nil {
		return fmt.Errorf("az NZBGet nem érhető el: %w", stripURL(err))
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if resp.StatusCode == http.StatusUnauthorized {
		return errors.New("hibás NZBGet felhasználónév vagy jelszó")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("NZBGet: HTTP %d", resp.StatusCode)
	}
	var r struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(b, &r) != nil {
		return errors.New("NZBGet: érthetetlen válasz – jó a cím?")
	}
	if r.Error != nil {
		return fmt.Errorf("NZBGet: %s", r.Error.Message)
	}
	if out != nil {
		return json.Unmarshal(r.Result, out)
	}
	return nil
}

// Test returns the NZBGet version.
func (n *NZBGet) Test(ctx context.Context) (string, error) {
	var v string
	if err := n.call(ctx, "version", []any{}, &v); err != nil {
		return "", err
	}
	return "NZBGet " + v, nil
}

// Add uploads the NZB; NZBGet's numeric id follows it.
func (n *NZBGet) Add(ctx context.Context, r AddRequest) (string, error) {
	if r.NZB == nil {
		return "", errors.New("az NZBGet csak Usenet-kiadást (NZB) tölt le")
	}
	cat := r.Category
	if cat == "" {
		cat = n.category
	}
	var id int64
	params := []any{nzbName(r) + ".nzb", base64.StdEncoding.EncodeToString(r.NZB), cat, 0, false, r.Paused, "", 0, "SCORE", []any{}}
	if err := n.call(ctx, "append", params, &id); err != nil {
		return "", err
	}
	if id <= 0 {
		return "", errors.New("az NZBGet nem fogadta el az NZB-t")
	}
	return strconv.FormatInt(id, 10), nil
}

// Status looks for the jobs in the queue and in the history.
func (n *NZBGet) Status(ctx context.Context, ids []string) (map[string]TorrentStatus, error) {
	out := map[string]TorrentStatus{}
	if len(ids) == 0 {
		return out, nil
	}
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	var groups []struct {
		ID        int64   `json:"NZBID"`
		Name      string  `json:"NZBName"`
		Status    string  `json:"Status"`
		SizeMB    float64 `json:"FileSizeMB"`
		LeftMB    float64 `json:"RemainingSizeMB"`
		DownRate  float64 `json:"DownloadRate"`
		DestDir   string  `json:"DestDir"`
		Remaining int64   `json:"RemainingSec"`
	}
	if err := n.call(ctx, "listgroups", []any{0}, &groups); err != nil {
		return nil, err
	}
	for _, g := range groups {
		id := strconv.FormatInt(g.ID, 10)
		if !want[id] {
			continue
		}
		p := 0.0
		if g.SizeMB > 0 {
			p = (g.SizeMB - g.LeftMB) / g.SizeMB
		}
		out[id] = TorrentStatus{Hash: id, Name: g.Name, State: g.Status, Progress: p, Size: int64(g.SizeMB * (1 << 20)), ETA: -1}
	}
	var hist []struct {
		ID       int64   `json:"NZBID"`
		Name     string  `json:"Name"`
		Status   string  `json:"Status"`
		DestDir  string  `json:"DestDir"`
		FinalDir string  `json:"FinalDir"`
		SizeMB   float64 `json:"FileSizeMB"`
	}
	if err := n.call(ctx, "history", []any{false}, &hist); err != nil {
		return nil, err
	}
	for _, h := range hist {
		id := strconv.FormatInt(h.ID, 10)
		if !want[id] {
			continue
		}
		dir := h.FinalDir
		if dir == "" {
			dir = h.DestDir
		}
		st := strings.ToUpper(h.Status)
		out[id] = TorrentStatus{Hash: id, Name: h.Name, State: h.Status, Progress: 1, Size: int64(h.SizeMB * (1 << 20)), ETA: -1, ContentPath: dir,
			Done: strings.HasPrefix(st, "SUCCESS") || strings.HasPrefix(st, "WARNING"), Failed: strings.HasPrefix(st, "FAILURE") || strings.HasPrefix(st, "DELETED")}
	}
	return out, nil
}

// Remove deletes the job from the queue or the history.
func (n *NZBGet) Remove(ctx context.Context, id string, deleteFiles bool) error {
	num, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return errors.New("érvénytelen NZBGet-azonosító")
	}
	queueCmd, histCmd := "GroupDelete", "HistoryDelete"
	if deleteFiles {
		queueCmd, histCmd = "GroupFinalDelete", "HistoryFinalDelete"
	}
	var ok bool
	if err := n.call(ctx, "editqueue", []any{queueCmd, "", []int64{num}}, &ok); err == nil && ok {
		return nil
	}
	return n.call(ctx, "editqueue", []any{histCmd, "", []int64{num}}, &ok)
}
