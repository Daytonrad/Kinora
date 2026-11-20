package cardigann

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Many public sites sit behind a Cloudflare (or DDoS-Guard) browser check that no HTTP client passes.
// FlareSolverr (github.com/FlareSolverr/FlareSolverr) is a small service that opens the page in a real
// browser and hands back the page, the clearance cookies and the browser's User-Agent; the next requests
// carry those and usually get through directly. Prowlarr and Jackett use it the same way.

// FlareSolverr is a FlareSolverr service, e.g. http://flaresolverr:8191.
type FlareSolverr struct {
	URL string
	HC  *http.Client // its own client: solving a check takes up to a minute
}

// NewFlareSolverr returns a solver for the address, or nil when it is empty.
func NewFlareSolverr(addr string) *FlareSolverr {
	addr = strings.TrimRight(strings.TrimSpace(addr), "/")
	if addr == "" {
		return nil
	}
	return &FlareSolverr{URL: addr, HC: &http.Client{Timeout: 90 * time.Second}}
}

type fsSolution struct {
	URL      string `json:"url"`
	Status   int    `json:"status"`
	Response string `json:"response"`
	Cookies  []struct {
		Name   string  `json:"name"`
		Value  string  `json:"value"`
		Domain string  `json:"domain"`
		Path   string  `json:"path"`
		Expiry float64 `json:"expiry"`
	} `json:"cookies"`
	UserAgent string `json:"userAgent"`
}

// solve opens a page through FlareSolverr: GET, or POST with a form body.
func (f *FlareSolverr) solve(ctx context.Context, method, u, post string) (*fsSolution, error) {
	in := map[string]any{"cmd": "request.get", "url": u, "maxTimeout": 60000}
	if method == "POST" {
		in["cmd"], in["postData"] = "request.post", post
	}
	b, _ := json.Marshal(in)
	req, err := http.NewRequestWithContext(ctx, "POST", f.URL+"/v1", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.HC.Do(req)
	if err != nil {
		return nil, fmt.Errorf("a FlareSolverr nem érhető el: %w", err)
	}
	defer resp.Body.Close()
	var out struct {
		Status   string     `json:"status"`
		Message  string     `json:"message"`
		Solution fsSolution `json:"solution"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("a FlareSolverr válasza érthetetlen (HTTP %d)", resp.StatusCode)
	}
	if out.Status != "ok" {
		return nil, fmt.Errorf("FlareSolverr: %s", out.Message)
	}
	return &out.Solution, nil
}

// Test checks that the address answers like a FlareSolverr and returns its version.
func (f *FlareSolverr) Test(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", f.URL+"/", nil)
	if err != nil {
		return "", err
	}
	resp, err := f.HC.Do(req)
	if err != nil {
		return "", fmt.Errorf("a FlareSolverr nem érhető el: %w", err)
	}
	defer resp.Body.Close()
	var out struct {
		Msg     string `json:"msg"`
		Version string `json:"version"`
	}
	if json.NewDecoder(resp.Body).Decode(&out) != nil || !strings.Contains(strings.ToLower(out.Msg), "flaresolverr") {
		return "", errors.New("ezen a címen nem FlareSolverr válaszol")
	}
	return out.Version, nil
}

// challenged reports whether a response is a browser check rather than the page.
func challenged(resp *http.Response, body []byte) bool {
	switch resp.StatusCode {
	case 403, 429, 503:
	default:
		return false
	}
	srv := strings.ToLower(resp.Header.Get("Server"))
	if strings.Contains(srv, "cloudflare") || strings.Contains(srv, "ddos-guard") || resp.Header.Get("cf-mitigated") != "" {
		return true
	}
	b := string(body)
	for _, m := range []string{"Just a moment...", "cf-chl", "challenge-platform", "DDoS-Guard", "Attention Required!"} {
		if strings.Contains(b, m) {
			return true
		}
	}
	return false
}

// solveVia lets the solver open the page, keeps its cookies and User-Agent for the session, and returns
// the page as if the request had got through.
func (ix *Indexer) solveVia(ctx context.Context, req *http.Request, post string) ([]byte, *http.Response, error) {
	sol, err := ix.Solver.solve(ctx, req.Method, req.URL.String(), post)
	if err != nil {
		return nil, nil, err
	}
	if sol.UserAgent != "" {
		ix.ua = sol.UserAgent // the clearance cookie is bound to the browser that earned it
	}
	if ix.hc.Jar != nil {
		var cs []*http.Cookie
		for _, c := range sol.Cookies {
			ck := &http.Cookie{Name: c.Name, Value: c.Value, Domain: c.Domain, Path: c.Path}
			if c.Expiry > 0 {
				ck.Expires = time.Unix(int64(c.Expiry), 0)
			}
			cs = append(cs, ck)
		}
		ix.hc.Jar.SetCookies(req.URL, cs)
	}
	final := req.URL
	if u, err := url.Parse(sol.URL); err == nil && u.Host != "" {
		final = u
	}
	r := req.Clone(ctx)
	r.URL = final
	resp := &http.Response{StatusCode: sol.Status, Header: http.Header{}, Request: r}
	if resp.StatusCode == 0 {
		resp.StatusCode = 200
	}
	body := []byte(unwrapPre(sol.Response))
	if resp.StatusCode >= 400 {
		return body, resp, fmt.Errorf("%s: HTTP %d (FlareSolverr)", ix.Def.Name, resp.StatusCode)
	}
	return body, resp, nil
}

// unwrapPre takes a JSON answer out of the page the browser made of it (<html>…<pre>{…}</pre>…).
func unwrapPre(s string) string {
	t := strings.TrimSpace(s)
	if !strings.HasPrefix(t, "<") {
		return s
	}
	i := strings.Index(t, "<pre")
	if i < 0 {
		return s
	}
	j := strings.Index(t[i:], ">")
	k := strings.LastIndex(t, "</pre>")
	if j < 0 || k < i+j {
		return s
	}
	in := strings.TrimSpace(html.UnescapeString(t[i+j+1 : k]))
	if strings.HasPrefix(in, "{") || strings.HasPrefix(in, "[") {
		return in
	}
	return s
}
