// Package jfimport moves users and their watch state from a Jellyfin server to Kinora.
//
// It reads Jellyfin only through its public HTTP API with an administrator API key (Dashboard → API
// Keys); it never opens Jellyfin's database. Nothing is written before the admin has seen the preview:
// Preview builds a Plan (what matches what, and why not), Apply carries it out. Apply is idempotent:
// accounts are remembered in import_links and watch state is merged, never duplicated or reset.
package jfimport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client talks to one Jellyfin server.
type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
	// PageSize is the number of items asked for at once.
	PageSize int
}

// NewClient checks the address and returns a client.
func NewClient(base, apiKey string) (*Client, error) {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, ErrBadURL
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, ErrNoKey
	}
	return &Client{BaseURL: base, APIKey: strings.TrimSpace(apiKey), HTTP: &http.Client{Timeout: 60 * time.Second}, PageSize: 500}, nil
}

// Errors shown to the admin (the web client translates the codes).
var (
	ErrBadURL       = errors.New("bad_url")
	ErrNoKey        = errors.New("no_key")
	ErrUnauthorized = errors.New("unauthorized") // wrong or non-admin API key
	ErrNotJellyfin  = errors.New("not_jellyfin")
)

const maxBody = 64 << 20 // one page of 500 items is far below this

func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	u := c.BaseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	// the documented authorization header; X-Emby-Token is what older servers read
	req.Header.Set("Authorization", fmt.Sprintf(`MediaBrowser Client="Kinora", Device="Kinora import", DeviceId="kinora-import", Version="1", Token="%s"`, c.APIKey))
	req.Header.Set("X-Emby-Token", c.APIKey)
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("unreachable: %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return ErrUnauthorized
	case resp.StatusCode == http.StatusNotFound:
		return &statusError{resp.StatusCode, path}
	case resp.StatusCode >= 300:
		return &statusError{resp.StatusCode, path}
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(out); err != nil {
		return fmt.Errorf("%w: %s: %v", ErrNotJellyfin, path, err)
	}
	return nil
}

type statusError struct {
	Code int
	Path string
}

func (e *statusError) Error() string { return fmt.Sprintf("HTTP %d: %s", e.Code, e.Path) }

// ServerInfo is /System/Info.
type ServerInfo struct {
	ServerName string `json:"ServerName"`
	Version    string `json:"Version"`
	ID         string `json:"Id"`
}

// Info checks the address and the key.
func (c *Client) Info(ctx context.Context) (*ServerInfo, error) {
	var si ServerInfo
	if err := c.get(ctx, "/System/Info", nil, &si); err != nil {
		var se *statusError
		if errors.As(err, &se) && se.Code == http.StatusNotFound {
			return nil, ErrNotJellyfin
		}
		return nil, err
	}
	if si.ID == "" && si.Version == "" {
		return nil, ErrNotJellyfin
	}
	return &si, nil
}

// User is a Jellyfin account.
type User struct {
	ID     string `json:"Id"`
	Name   string `json:"Name"`
	Policy struct {
		IsAdministrator bool `json:"IsAdministrator"`
		IsDisabled      bool `json:"IsDisabled"`
	} `json:"Policy"`
	LastActivityDate string `json:"LastActivityDate"`
}

// Users lists the accounts.
func (c *Client) Users(ctx context.Context) ([]User, error) {
	var us []User
	return us, c.get(ctx, "/Users", nil, &us)
}

// UserData is an item's state for one user.
type UserData struct {
	PlaybackPositionTicks int64  `json:"PlaybackPositionTicks"`
	PlayCount             int    `json:"PlayCount"`
	IsFavorite            bool   `json:"IsFavorite"`
	Played                bool   `json:"Played"`
	LastPlayedDate        string `json:"LastPlayedDate"`
}

// Interesting reports whether there is anything to import.
func (d UserData) Interesting() bool {
	return d.Played || d.IsFavorite || d.PlayCount > 0 || d.PlaybackPositionTicks > 0
}

// PositionSeconds converts the 100 ns ticks.
func (d UserData) PositionSeconds() float64 { return float64(d.PlaybackPositionTicks) / 1e7 }

// LastPlayed is the last play time as unix seconds (0 if unknown).
func (d UserData) LastPlayed() int64 {
	if d.LastPlayedDate == "" {
		return 0
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.0000000", "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, d.LastPlayedDate); err == nil && t.Year() > 1971 {
			return t.Unix()
		}
	}
	return 0
}

// Item is a film, series or episode with the user's state.
type Item struct {
	ID                string            `json:"Id"`
	Name              string            `json:"Name"`
	Type              string            `json:"Type"` // Movie | Series | Episode
	Path              string            `json:"Path"`
	ProductionYear    int               `json:"ProductionYear"`
	ProviderIDs       map[string]string `json:"ProviderIds"`
	SeriesID          string            `json:"SeriesId"`
	SeriesName        string            `json:"SeriesName"`
	ParentIndexNumber *int              `json:"ParentIndexNumber"` // season
	IndexNumber       *int              `json:"IndexNumber"`       // episode
	IndexNumberEnd    *int              `json:"IndexNumberEnd"`
	UserData          UserData          `json:"UserData"`
}

// Provider returns a provider id by name, case-insensitively ("Tmdb", "Imdb", "Tvdb").
func (it *Item) Provider(name string) string {
	for k, v := range it.ProviderIDs {
		if strings.EqualFold(k, name) {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// TmdbID is the TMDB id as a number (0 if none).
func (it *Item) TmdbID() int64 {
	n, _ := strconv.ParseInt(it.Provider("Tmdb"), 10, 64)
	return n
}

type itemPage struct {
	Items            []Item `json:"Items"`
	TotalRecordCount int    `json:"TotalRecordCount"`
}

// Items lists every film, series and episode of a user with the user's state, page by page.
// It uses /Users/{id}/Items and falls back to /Items?userId= on servers that dropped the former.
func (c *Client) Items(ctx context.Context, userID string) ([]Item, error) {
	var out []Item
	legacy := true
	for start := 0; ; {
		q := url.Values{
			"Recursive":        {"true"},
			"IncludeItemTypes": {"Movie,Series,Episode"},
			"Fields":           {"Path,ProviderIds,ProductionYear"},
			"EnableUserData":   {"true"},
			"EnableImages":     {"false"},
			"StartIndex":       {strconv.Itoa(start)},
			"Limit":            {strconv.Itoa(c.PageSize)},
		}
		var page itemPage
		var err error
		if legacy {
			err = c.get(ctx, "/Users/"+url.PathEscape(userID)+"/Items", q, &page)
			var se *statusError
			if errors.As(err, &se) && se.Code == http.StatusNotFound && start == 0 {
				legacy = false
			}
		}
		if !legacy {
			q.Set("userId", userID)
			err = c.get(ctx, "/Items", q, &page)
		}
		if err != nil {
			return nil, err
		}
		out = append(out, page.Items...)
		start += len(page.Items)
		if len(page.Items) == 0 || start >= page.TotalRecordCount {
			return out, nil
		}
	}
}
