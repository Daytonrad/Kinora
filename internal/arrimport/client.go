// Package arrimport moves the setup of Sonarr, Radarr and Prowlarr into Kinora's built-in acquisition:
// followed series and wanted films, quality profiles (where they map), indexers, download clients and root
// folders. Like the Jellyfin import it reads only the public API (with the app's API key), shows a preview
// that writes nothing, and can be run again without duplicating anything.
package arrimport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Apps.
const (
	Sonarr   = "sonarr"
	Radarr   = "radarr"
	Prowlarr = "prowlarr"
)

// Errors (the web client translates the codes).
var (
	ErrBadURL       = errors.New("bad_url")
	ErrNoKey        = errors.New("no_key")
	ErrUnauthorized = errors.New("unauthorized")
	ErrWrongApp     = errors.New("wrong_app") // the address answers, but as another app (or not an *arr at all)
)

// Client talks to one Sonarr, Radarr or Prowlarr.
type Client struct {
	App     string
	BaseURL string
	APIKey  string
	HTTP    *http.Client
}

// NewClient checks the address and returns a client.
func NewClient(app, base, apiKey string) (*Client, error) {
	switch app {
	case Sonarr, Radarr, Prowlarr:
	default:
		return nil, ErrWrongApp
	}
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, ErrBadURL
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, ErrNoKey
	}
	return &Client{App: app, BaseURL: base, APIKey: strings.TrimSpace(apiKey), HTTP: &http.Client{Timeout: 60 * time.Second}}, nil
}

func (c *Client) api() string {
	if c.App == Prowlarr {
		return "/api/v1"
	}
	return "/api/v3"
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+c.api()+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", c.APIKey)
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
		return fmt.Errorf("%w: HTTP 404 %s", ErrWrongApp, path)
	case resp.StatusCode >= 300:
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, path)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 128<<20)).Decode(out); err != nil {
		return fmt.Errorf("%w: %s: %v", ErrWrongApp, path, err)
	}
	return nil
}

// Status is /system/status.
type Status struct {
	AppName      string `json:"appName"`
	InstanceName string `json:"instanceName"`
	Version      string `json:"version"`
}

// Status checks the address, the key and which app answers.
func (c *Client) Status(ctx context.Context) (*Status, error) {
	var s Status
	if err := c.get(ctx, "/system/status", &s); err != nil {
		return nil, err
	}
	if !strings.EqualFold(s.AppName, c.App) {
		return nil, fmt.Errorf("%w: %q", ErrWrongApp, s.AppName)
	}
	return &s, nil
}

// Field is a setting of an indexer or a download client ("fields" in the API).
type Field struct {
	Name  string          `json:"name"`
	Value json.RawMessage `json:"value"`
}

type withFields struct {
	Fields []Field `json:"fields"`
}

// Str reads a text field; masked secrets ("********") count as missing.
func (w withFields) Str(name string) string {
	for _, f := range w.Fields {
		if strings.EqualFold(f.Name, name) {
			var s string
			if json.Unmarshal(f.Value, &s) == nil && strings.Trim(s, "*") != "" {
				return s
			}
			var n json.Number
			if json.Unmarshal(f.Value, &n) == nil {
				return n.String()
			}
		}
	}
	return ""
}

// Masked reports whether a field holds a hidden secret.
func (w withFields) Masked(name string) bool {
	for _, f := range w.Fields {
		var s string
		if strings.EqualFold(f.Name, name) && json.Unmarshal(f.Value, &s) == nil && s != "" && strings.Trim(s, "*") == "" {
			return true
		}
	}
	return false
}

// Bool reads a checkbox field.
func (w withFields) Bool(name string) bool {
	for _, f := range w.Fields {
		var b bool
		if strings.EqualFold(f.Name, name) && json.Unmarshal(f.Value, &b) == nil {
			return b
		}
	}
	return false
}

// Ints reads a list field (categories).
func (w withFields) Ints(name string) []int {
	for _, f := range w.Fields {
		var l []int
		if strings.EqualFold(f.Name, name) && json.Unmarshal(f.Value, &l) == nil {
			return l
		}
	}
	return nil
}

// Indexer is a Sonarr/Radarr indexer, or a Prowlarr one.
type Indexer struct {
	withFields
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	Implementation string `json:"implementation"` // Torznab | Newznab | (Prowlarr: Cardigann, …)
	Protocol       string `json:"protocol"`       // torrent | usenet
	Priority       int    `json:"priority"`
	EnableRss      bool   `json:"enableRss"`
	EnableSearch   bool   `json:"enableAutomaticSearch"`
	Enable         bool   `json:"enable"` // Prowlarr
	DefinitionName string `json:"definitionName"`
}

// DownloadClient is a Sonarr/Radarr download client.
type DownloadClient struct {
	withFields
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	Implementation string `json:"implementation"` // QBittorrent | Transmission | Deluge | Sabnzbd | Nzbget | …
	Protocol       string `json:"protocol"`
	Enable         bool   `json:"enable"`
}

// QualityItem is one entry of a quality profile; a group has Items of its own.
type QualityItem struct {
	Name    string `json:"name"`
	Allowed bool   `json:"allowed"`
	ID      int    `json:"id"` // a group's id
	Quality *struct {
		ID         int    `json:"id"`
		Name       string `json:"name"`
		Source     string `json:"source"`
		Resolution int    `json:"resolution"`
	} `json:"quality"`
	Items []QualityItem `json:"items"`
}

// QualityProfile is a Sonarr/Radarr quality profile.
type QualityProfile struct {
	ID             int64         `json:"id"`
	Name           string        `json:"name"`
	UpgradeAllowed bool          `json:"upgradeAllowed"`
	Cutoff         int           `json:"cutoff"`
	Items          []QualityItem `json:"items"`
}

// RootFolder is where an app keeps its library.
type RootFolder struct {
	ID         int64  `json:"id"`
	Path       string `json:"path"`
	Accessible bool   `json:"accessible"`
}

// Movie is a Radarr film.
type Movie struct {
	ID               int64  `json:"id"`
	Title            string `json:"title"`
	Year             int    `json:"year"`
	TmdbID           int64  `json:"tmdbId"`
	ImdbID           string `json:"imdbId"`
	Monitored        bool   `json:"monitored"`
	HasFile          bool   `json:"hasFile"`
	QualityProfileID int64  `json:"qualityProfileId"`
	Path             string `json:"path"`
	RootFolderPath   string `json:"rootFolderPath"`
}

// Series is a Sonarr series.
type Series struct {
	ID               int64  `json:"id"`
	Title            string `json:"title"`
	Year             int    `json:"year"`
	TvdbID           int64  `json:"tvdbId"`
	TmdbID           int64  `json:"tmdbId"`
	ImdbID           string `json:"imdbId"`
	Monitored        bool   `json:"monitored"`
	QualityProfileID int64  `json:"qualityProfileId"`
	Path             string `json:"path"`
	RootFolderPath   string `json:"rootFolderPath"`
	Seasons          []struct {
		SeasonNumber int  `json:"seasonNumber"`
		Monitored    bool `json:"monitored"`
	} `json:"seasons"`
}

// Lists.
func (c *Client) Indexers(ctx context.Context) ([]Indexer, error) {
	var l []Indexer
	return l, c.get(ctx, "/indexer", &l)
}

func (c *Client) DownloadClients(ctx context.Context) ([]DownloadClient, error) {
	var l []DownloadClient
	return l, c.get(ctx, "/downloadclient", &l)
}

func (c *Client) QualityProfiles(ctx context.Context) ([]QualityProfile, error) {
	var l []QualityProfile
	return l, c.get(ctx, "/qualityprofile", &l)
}

func (c *Client) RootFolders(ctx context.Context) ([]RootFolder, error) {
	var l []RootFolder
	return l, c.get(ctx, "/rootfolder", &l)
}

func (c *Client) Movies(ctx context.Context) ([]Movie, error) {
	var l []Movie
	return l, c.get(ctx, "/movie", &l)
}

func (c *Client) Series(ctx context.Context) ([]Series, error) {
	var l []Series
	return l, c.get(ctx, "/series", &l)
}
