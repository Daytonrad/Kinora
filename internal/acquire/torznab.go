package acquire

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Indexer is a Torznab endpoint: a tracker with a Torznab API, or an indexer proxy
// (Jackett, Prowlarr) standing in for several trackers.
type Indexer struct {
	ID         int64
	Name       string
	URL        string
	APIKey     string
	Categories string // comma separated Torznab categories; empty = 2000 (movies)
}

// Result is one release an indexer returned.
type Result struct {
	IndexerID int64     `json:"indexer_id"`
	Indexer   string    `json:"indexer"`
	Title     string    `json:"title"`
	GUID      string    `json:"guid"`
	Link      string    `json:"link,omitempty"`   // .torrent download (may redirect to a magnet)
	Magnet    string    `json:"magnet,omitempty"` // magnet link when the indexer gives one
	InfoHash  string    `json:"info_hash,omitempty"`
	IMDb      string    `json:"imdb,omitempty"`
	Size      int64     `json:"size"`
	Seeders   int       `json:"seeders"` // -1 when unknown
	Peers     int       `json:"peers"`
	Published time.Time `json:"published"`
	Info      string    `json:"info_url,omitempty"`
	Release   Release   `json:"release"`
	Quality   string    `json:"quality"`
	Score     int       `json:"score"`
	Rejected  string    `json:"rejected,omitempty"`
}

// Query is what to look for. With an IMDb id the indexer is asked for that film;
// Title (+ Year) is the text search used when the id search finds nothing.
type Query struct {
	IMDb  string
	Title string
	Year  int
}

// Torznab is the client; one is enough for every indexer.
type Torznab struct {
	HC *http.Client
}

func endpoint(base string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", errors.New("az indexer címe http:// vagy https:// kezdetű legyen")
	}
	p := strings.TrimRight(u.Path, "/")
	if !strings.HasSuffix(p, "/api") {
		p += "/api"
	}
	u.Path = p
	return u.String(), nil
}

type rss struct {
	XMLName xml.Name
	Code    string `xml:"code,attr"` // <error code=".." description=".."/>
	Desc    string `xml:"description,attr"`
	Channel struct {
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
	// <caps>
	Server struct {
		Title   string `xml:"title,attr"`
		Version string `xml:"version,attr"`
	} `xml:"server"`
}

type rssItem struct {
	Title     string `xml:"title"`
	GUID      string `xml:"guid"`
	Link      string `xml:"link"`
	Comments  string `xml:"comments"`
	PubDate   string `xml:"pubDate"`
	Size      string `xml:"size"`
	Enclosure struct {
		URL    string `xml:"url,attr"`
		Length string `xml:"length,attr"`
		Type   string `xml:"type,attr"`
	} `xml:"enclosure"`
	Attrs []struct {
		Name  string `xml:"name,attr"`
		Value string `xml:"value,attr"`
	} `xml:"attr"` // torznab:attr and newznab:attr
}

func (c *Torznab) call(ctx context.Context, ix Indexer, q url.Values) (*rss, error) {
	ep, err := endpoint(ix.URL)
	if err != nil {
		return nil, err
	}
	if ix.APIKey != "" {
		q.Set("apikey", ix.APIKey)
	}
	req, err := http.NewRequestWithContext(ctx, "GET", ep+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/rss+xml, application/xml, text/xml")
	resp, err := c.HC.Do(req)
	if err != nil {
		return nil, fmt.Errorf("az indexer nem érhető el: %w", stripURL(err))
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	var r rss
	if xerr := xml.Unmarshal(body, &r); xerr != nil {
		if resp.StatusCode >= 300 {
			return nil, fmt.Errorf("az indexer hibát adott: HTTP %d", resp.StatusCode)
		}
		return nil, errors.New("az indexer válasza nem Torznab (XML) – jó a cím?")
	}
	if r.XMLName.Local == "error" {
		return nil, fmt.Errorf("az indexer hibát adott: %s (%s)", r.Desc, r.Code)
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("az indexer hibát adott: HTTP %d", resp.StatusCode)
	}
	return &r, nil
}

// stripURL drops the request URL from a transport error: it carries the API key.
func stripURL(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}

// Caps checks that the endpoint speaks Torznab and returns its name and version.
func (c *Torznab) Caps(ctx context.Context, ix Indexer) (string, error) {
	r, err := c.call(ctx, ix, url.Values{"t": {"caps"}})
	if err != nil {
		return "", err
	}
	if r.XMLName.Local != "caps" {
		return "", errors.New("az indexer válasza nem Torznab caps")
	}
	name := strings.TrimSpace(r.Server.Title + " " + r.Server.Version)
	return name, nil
}

func cats(ix Indexer) string {
	if strings.TrimSpace(ix.Categories) == "" {
		return "2000"
	}
	return strings.ReplaceAll(ix.Categories, " ", "")
}

// Search asks one indexer for a film: by IMDb id first, then by title and year.
func (c *Torznab) Search(ctx context.Context, ix Indexer, q Query) ([]Result, error) {
	var items []rssItem
	if q.IMDb != "" {
		r, err := c.call(ctx, ix, url.Values{"t": {"movie"}, "imdbid": {strings.TrimPrefix(q.IMDb, "tt")}, "cat": {cats(ix)}})
		if err == nil {
			items = r.Channel.Items
		} else if q.Title == "" {
			return nil, err
		}
	}
	if len(items) == 0 && q.Title != "" {
		text := q.Title
		if q.Year > 0 {
			text += " " + strconv.Itoa(q.Year)
		}
		r, err := c.call(ctx, ix, url.Values{"t": {"search"}, "q": {text}, "cat": {cats(ix)}})
		if err != nil {
			return nil, err
		}
		items = r.Channel.Items
	}
	out := make([]Result, 0, len(items))
	for _, it := range items {
		out = append(out, toResult(ix, it))
	}
	return out, nil
}

func toResult(ix Indexer, it rssItem) Result {
	r := Result{IndexerID: ix.ID, Indexer: ix.Name, Title: strings.TrimSpace(it.Title), GUID: it.GUID, Seeders: -1, Info: it.Comments}
	r.Link = it.Enclosure.URL
	if r.Link == "" {
		r.Link = it.Link
	}
	if strings.HasPrefix(r.Link, "magnet:") {
		r.Magnet, r.Link = r.Link, ""
	}
	r.Size, _ = strconv.ParseInt(it.Size, 10, 64)
	if r.Size == 0 {
		r.Size, _ = strconv.ParseInt(it.Enclosure.Length, 10, 64)
	}
	if t, err := time.Parse(time.RFC1123Z, it.PubDate); err == nil {
		r.Published = t
	} else if t, err := time.Parse(time.RFC1123, it.PubDate); err == nil {
		r.Published = t
	}
	for _, a := range it.Attrs {
		v := strings.TrimSpace(a.Value)
		switch strings.ToLower(a.Name) {
		case "seeders":
			r.Seeders, _ = strconv.Atoi(v)
		case "peers":
			r.Peers, _ = strconv.Atoi(v)
		case "size":
			if r.Size == 0 {
				r.Size, _ = strconv.ParseInt(v, 10, 64)
			}
		case "infohash":
			r.InfoHash = strings.ToLower(v)
		case "magneturl":
			if r.Magnet == "" {
				r.Magnet = v
			}
		case "imdb", "imdbid":
			if v != "" && v != "0" {
				if !strings.HasPrefix(v, "tt") {
					if len(v) < 7 {
						v = strings.Repeat("0", 7-len(v)) + v
					}
					v = "tt" + v
				}
				r.IMDb = v
			}
		}
	}
	if r.InfoHash == "" && r.Magnet != "" {
		r.InfoHash, _ = MagnetHash(r.Magnet)
	}
	r.Release = ParseRelease(r.Title)
	r.Quality = r.Release.QualityLabel()
	return r
}
