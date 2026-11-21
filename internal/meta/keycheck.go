package meta

// Key checks: the admin page asks whether an entered API key works before (or right after) it
// is saved. Each check makes one small, cheap request to the service and sorts the answer into
// a state the UI can explain: ok, invalid, login (OpenSubtitles username/password), limit,
// network or error.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// KeyCheck is the outcome of testing one API key.
type KeyCheck struct {
	OK     bool   `json:"ok"`
	State  string `json:"state"`            // ok | invalid | login | limit | network | error | missing
	Detail string `json:"detail,omitempty"` // the service's own message, when it gave one
}

func keyFail(state, detail string) KeyCheck { return KeyCheck{State: state, Detail: detail} }

var reKeyParam = regexp.MustCompile(`(?i)[?&](?:apikey|api_key)=([^&#\s]+)`)

// ExtractKey returns the key itself when a whole example URL was pasted
// (e.g. http://www.omdbapi.com/?i=tt3896198&apikey=abcd1234 → abcd1234).
func ExtractKey(v string) string {
	v = strings.TrimSpace(v)
	if m := reKeyParam.FindStringSubmatch(v); m != nil {
		if k, err := url.QueryUnescape(m[1]); err == nil {
			return k
		}
		return m[1]
	}
	return v
}

// CheckTMDB tests a TMDB v3 key or v4 read access token.
func (f *Fetcher) CheckTMDB(ctx context.Context, key string) KeyCheck {
	key = ExtractKey(key)
	if key == "" {
		return keyFail("missing", "")
	}
	u := f.BaseURL + "/configuration"
	bearer := len(key) > 40
	if !bearer {
		u += "?" + url.Values{"api_key": {key}}.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return keyFail("error", err.Error())
	}
	req.Header.Set("Accept", "application/json")
	if bearer {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := f.hc.Do(req)
	if err != nil {
		return keyFail("network", err.Error())
	}
	defer resp.Body.Close()
	var e struct {
		Message string `json:"status_message"`
	}
	json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&e)
	switch {
	case resp.StatusCode < 300:
		return KeyCheck{OK: true, State: "ok"}
	case resp.StatusCode == http.StatusUnauthorized:
		return keyFail("invalid", e.Message)
	case resp.StatusCode == http.StatusTooManyRequests:
		return keyFail("limit", e.Message)
	}
	return keyFail("error", strings.TrimSpace(resp.Status+" "+e.Message))
}

// CheckOmdb tests an OMDb key with one lookup of a well-known title.
func (f *Fetcher) CheckOmdb(ctx context.Context, key string) KeyCheck {
	key = ExtractKey(key)
	if key == "" {
		return keyFail("missing", "")
	}
	base := f.OmdbURL
	if base == "" {
		base = DefaultOmdbURL
	}
	q := url.Values{"i": {"tt0111161"}, "apikey": {key}}
	req, err := http.NewRequestWithContext(ctx, "GET", base+"?"+q.Encode(), nil)
	if err != nil {
		return keyFail("error", err.Error())
	}
	req.Header.Set("Accept", "application/json")
	resp, err := f.hc.Do(req)
	if err != nil {
		return keyFail("network", err.Error())
	}
	defer resp.Body.Close()
	var d omdbResp
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&d); err != nil {
		return keyFail("error", resp.Status)
	}
	if d.Response == "True" {
		return KeyCheck{OK: true, State: "ok"}
	}
	msg := strings.ToLower(d.Error)
	switch {
	case strings.Contains(msg, "limit"):
		return keyFail("limit", d.Error)
	case strings.Contains(msg, "key") || resp.StatusCode == http.StatusUnauthorized:
		return keyFail("invalid", d.Error) // also what a key that was never activated gets
	}
	return keyFail("error", d.Error)
}

// CheckKey tests an OpenSubtitles API key and, when both are given, the username and password.
func (o *OpenSubtitles) CheckKey(ctx context.Context, key, user, pass string) KeyCheck {
	key = ExtractKey(key)
	if key == "" {
		return keyFail("missing", "")
	}
	call := func(method, path string, body any) (int, string, error) {
		var rd io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rd = bytes.NewReader(b)
		}
		req, err := http.NewRequestWithContext(ctx, method, o.BaseURL+path, rd)
		if err != nil {
			return 0, "", err
		}
		req.Header.Set("Api-Key", key)
		req.Header.Set("User-Agent", o.UserAgent)
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := o.hc.Do(req)
		if err != nil {
			return 0, "", err
		}
		defer resp.Body.Close()
		var e struct {
			Message string   `json:"message"`
			Errors  []string `json:"errors"`
		}
		json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&e)
		if e.Message == "" && len(e.Errors) > 0 {
			e.Message = strings.Join(e.Errors, ", ")
		}
		return resp.StatusCode, e.Message, nil
	}
	classify := func(code int, msg string, denied string) KeyCheck {
		switch {
		case code < 300:
			return KeyCheck{OK: true, State: "ok"}
		case code == http.StatusUnauthorized || code == http.StatusForbidden:
			return keyFail(denied, msg)
		case code == http.StatusTooManyRequests:
			return keyFail("limit", msg)
		}
		return keyFail("error", strings.TrimSpace(http.StatusText(code)+" "+msg))
	}
	code, msg, err := call("GET", "/infos/formats", nil)
	if err != nil {
		return keyFail("network", err.Error())
	}
	if r := classify(code, msg, "invalid"); !r.OK || user == "" || pass == "" {
		return r
	}
	code, msg, err = call("POST", "/login", map[string]string{"username": user, "password": pass})
	if err != nil {
		return keyFail("network", err.Error())
	}
	if code == http.StatusBadRequest {
		code = http.StatusUnauthorized // a wrong username is answered with 400
	}
	return classify(code, msg, "login")
}
