package meta

// Scores: what the critics and the box office said about a title, for the detail page and the
// Felfedezés dialog. TMDB gives its own score, the budget and the worldwide gross; an optional
// OMDb key (omdb_key) adds the IMDb rating, Rotten Tomatoes and Metacritic. Looked up live and
// kept in memory for a while, never stored in the database.

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

	"github.com/ScotyLucas/kinora/internal/store"
)

// Score is one site's rating. Value is on the site's own scale, 0..Max.
type Score struct {
	Source string  `json:"source"` // imdb | rt | metacritic | tmdb
	Value  float64 `json:"value"`
	Max    float64 `json:"max"`
	Votes  int64   `json:"votes,omitempty"`
	URL    string  `json:"url,omitempty"`
}

// Scores is everything known about a title's reception.
type Scores struct {
	Ratings []Score `json:"ratings"`
	Budget  int64   `json:"budget,omitempty"`  // USD, films only
	Revenue int64   `json:"revenue,omitempty"` // USD, worldwide gross, films only
	OMDb    bool    `json:"omdb"`              // an OMDb key is set (the UI hints at it when not)
}

const (
	scoresTTL    = 12 * time.Hour
	scoresErrTTL = 10 * time.Minute
	scoresMax    = 4000 // cache entries before it is emptied
)

// DefaultOmdbURL is the OMDb API endpoint.
const DefaultOmdbURL = "https://www.omdbapi.com/"

type scoresEntry struct {
	at  time.Time
	ttl time.Duration
	val Scores
}

// OmdbEnabled reports whether an OMDb key is configured.
func (f *Fetcher) OmdbEnabled() bool { return strings.TrimSpace(f.st.Setting("omdb_key")) != "" }

// TitleScores returns the ratings (and, for films, the budget and gross) of a title. kind is
// "movie" or "series"; tmdbID or imdbID may be 0 / "" when unknown. Sources that fail are left
// out: the result is whatever could be found, never an error for a single missing site.
func (f *Fetcher) TitleScores(ctx context.Context, kind string, tmdbID int64, imdbID string) Scores {
	imdbID = store.NormalizeImdbID(imdbID)
	key := fmt.Sprintf("%s:%d:%s", kind, tmdbID, imdbID)
	f.mu.Lock()
	if e, ok := f.scores[key]; ok && time.Since(e.at) < e.ttl {
		f.mu.Unlock()
		return e.val
	}
	f.mu.Unlock()

	out := Scores{Ratings: []Score{}, OMDb: f.OmdbEnabled()}
	failed := false
	var tmdb *Score
	if tmdbID > 0 && f.Enabled() {
		path := "/movie/" + strconv.FormatInt(tmdbID, 10)
		q := url.Values{}
		if kind == "series" {
			path = "/tv/" + strconv.FormatInt(tmdbID, 10)
			q.Set("append_to_response", "external_ids")
		}
		var d struct {
			ImdbID      string  `json:"imdb_id"`
			VoteAverage float64 `json:"vote_average"`
			VoteCount   int64   `json:"vote_count"`
			Budget      int64   `json:"budget"`
			Revenue     int64   `json:"revenue"`
			ExternalIDs struct {
				ImdbID string `json:"imdb_id"`
			} `json:"external_ids"`
		}
		if err := f.get(ctx, path, q, &d); err != nil {
			failed = !errors.Is(err, errNotFound)
			f.log.Debug("tmdb scores", "tmdb", tmdbID, "err", err)
		} else {
			if imdbID == "" {
				imdbID = store.NormalizeImdbID(d.ImdbID)
				if imdbID == "" {
					imdbID = store.NormalizeImdbID(d.ExternalIDs.ImdbID)
				}
			}
			if kind != "series" {
				out.Budget, out.Revenue = max(d.Budget, 0), max(d.Revenue, 0)
			}
			if d.VoteCount > 0 && d.VoteAverage > 0 {
				site := "movie"
				if kind == "series" {
					site = "tv"
				}
				tmdb = &Score{Source: "tmdb", Value: round1(d.VoteAverage), Max: 10, Votes: d.VoteCount,
					URL: fmt.Sprintf("https://www.themoviedb.org/%s/%d", site, tmdbID)}
			}
		}
	}
	if imdbID != "" && out.OMDb {
		rs, err := f.omdb(ctx, imdbID)
		if err != nil {
			failed = true
			f.log.Debug("omdb scores", "imdb", imdbID, "err", err)
		}
		out.Ratings = append(out.Ratings, rs...)
	}
	if tmdb != nil {
		out.Ratings = append(out.Ratings, *tmdb)
	}

	ttl := scoresTTL
	if failed {
		ttl = scoresErrTTL
	}
	f.mu.Lock()
	if f.scores == nil || len(f.scores) >= scoresMax {
		f.scores = map[string]scoresEntry{}
	}
	f.scores[key] = scoresEntry{time.Now(), ttl, out}
	f.mu.Unlock()
	return out
}

// omdb fetches the IMDb, Rotten Tomatoes and Metacritic ratings of an IMDb title.
func (f *Fetcher) omdb(ctx context.Context, imdbID string) ([]Score, error) {
	base := f.OmdbURL
	if base == "" {
		base = DefaultOmdbURL
	}
	q := url.Values{"i": {imdbID}, "apikey": {strings.TrimSpace(f.st.Setting("omdb_key"))}}
	req, err := http.NewRequestWithContext(ctx, "GET", base+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := f.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OMDb nem érhető el: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var d omdbResp
	if err := json.Unmarshal(body, &d); err != nil {
		return nil, fmt.Errorf("OMDb: HTTP %d", resp.StatusCode)
	}
	if d.Response != "True" {
		if strings.Contains(strings.ToLower(d.Error), "not found") {
			return nil, nil
		}
		return nil, fmt.Errorf("OMDb: %s", d.Error)
	}
	return d.scores(imdbID), nil
}

type omdbResp struct {
	Response   string `json:"Response"`
	Error      string `json:"Error"`
	ImdbRating string `json:"imdbRating"`
	ImdbVotes  string `json:"imdbVotes"`
	Metascore  string `json:"Metascore"`
	Ratings    []struct {
		Source string `json:"Source"`
		Value  string `json:"Value"`
	} `json:"Ratings"`
}

// scores turns an OMDb answer into IMDb, Rotten Tomatoes and Metacritic scores ("N/A" is skipped).
func (d omdbResp) scores(imdbID string) []Score {
	var out []Score
	if v, err := strconv.ParseFloat(d.ImdbRating, 64); err == nil && v > 0 {
		n, _ := strconv.ParseInt(strings.ReplaceAll(d.ImdbVotes, ",", ""), 10, 64)
		out = append(out, Score{Source: "imdb", Value: v, Max: 10, Votes: n, URL: "https://www.imdb.com/title/" + imdbID + "/"})
	}
	rt, mc := -1.0, -1.0
	for _, r := range d.Ratings {
		switch r.Source {
		case "Rotten Tomatoes":
			if v, err := strconv.ParseFloat(strings.TrimSuffix(r.Value, "%"), 64); err == nil {
				rt = v
			}
		case "Metacritic":
			if v, err := strconv.ParseFloat(strings.TrimSuffix(r.Value, "/100"), 64); err == nil {
				mc = v
			}
		}
	}
	if mc < 0 {
		if v, err := strconv.ParseFloat(d.Metascore, 64); err == nil {
			mc = v
		}
	}
	if rt >= 0 {
		out = append(out, Score{Source: "rt", Value: rt, Max: 100})
	}
	if mc >= 0 {
		out = append(out, Score{Source: "metacritic", Value: mc, Max: 100})
	}
	return out
}

func round1(v float64) float64 { return float64(int(v*10+0.5)) / 10 }
