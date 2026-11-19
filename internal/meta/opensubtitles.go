package meta

// OpenSubtitles.com (REST API v1) client: subtitle search by file hash, IMDb / TMDB id or title,
// and download. It only runs when an admin entered an API key; a username and password are
// optional (without them OpenSubtitles allows only a few anonymous downloads a day).

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ScotyLucas/kinora/internal/store"
)

// ErrSubsNotConfigured is returned when no OpenSubtitles API key is set.
var ErrSubsNotConfigured = errors.New("a feliratkeresés nincs beállítva: adj meg egy OpenSubtitles API-kulcsot az Admin → Metaadatok oldalon")

// OpenSubtitles talks to api.opensubtitles.com.
type OpenSubtitles struct {
	st        *store.Store
	hc        *http.Client
	UserAgent string
	BaseURL   string // https://api.opensubtitles.com/api/v1

	mu      sync.Mutex
	token   string
	host    string // base_url from the login (VIP users get their own host)
	tokenAt time.Time
	tokenOf string // user|pass|key the token was made with
}

// NewOpenSubtitles creates the client; userAgent must name the app and its version.
func NewOpenSubtitles(st *store.Store, userAgent string) *OpenSubtitles {
	return &OpenSubtitles{st: st, hc: &http.Client{Timeout: 30 * time.Second}, UserAgent: userAgent,
		BaseURL: "https://api.opensubtitles.com/api/v1"}
}

// Enabled reports whether an API key is configured.
func (o *OpenSubtitles) Enabled() bool {
	return strings.TrimSpace(o.st.Setting("opensubtitles_key")) != ""
}

// SubtitleResult is one subtitle file offered for an item.
type SubtitleResult struct {
	FileID          int64   `json:"file_id"`
	Lang            string  `json:"lang"` // Kinora's 3-letter code (hun, eng…)
	Release         string  `json:"release"`
	FileName        string  `json:"file_name,omitempty"`
	Downloads       int     `json:"downloads"`
	Rating          float64 `json:"rating,omitempty"`
	HearingImpaired bool    `json:"hearing_impaired,omitempty"`
	Forced          bool    `json:"forced,omitempty"`
	HashMatch       bool    `json:"hash_match,omitempty"` // made for exactly this file: the timing fits
	Trusted         bool    `json:"trusted,omitempty"`
	MachineMade     bool    `json:"machine_made,omitempty"` // AI or machine translated
	FPS             float64 `json:"fps,omitempty"`
	Uploader        string  `json:"uploader,omitempty"`
	Uploaded        string  `json:"uploaded,omitempty"`
}

// SubtitleQuery describes what to search for.
type SubtitleQuery struct {
	Langs      []string // Kinora 3-letter codes, in the profile's order
	Hash       string   // OpenSubtitles hash of the file ("" = none)
	ImdbID     string   // movie: tt…
	TmdbID     int64    // movie, or the series for an episode
	Episode    bool
	Season     int
	EpNum      int
	SeriesImdb string
	Title      string
	Year       int
}

// os ↔ Kinora language codes
var osLang = map[string]string{
	"hun": "hu", "eng": "en", "ger": "de", "fre": "fr", "spa": "es", "ita": "it", "por": "pt-pt", "pob": "pt-br",
	"rus": "ru", "pol": "pl", "cze": "cs", "slo": "sk", "rum": "ro", "hrv": "hr", "srp": "sr", "slv": "sl", "ukr": "uk",
	"dut": "nl", "swe": "sv", "nor": "no", "dan": "da", "fin": "fi", "gre": "el", "tur": "tr", "jpn": "ja", "kor": "ko",
	"chi": "zh-cn", "ara": "ar", "heb": "he", "hin": "hi", "bul": "bg", "est": "et", "lav": "lv", "lit": "lt", "vie": "vi",
	"tha": "th", "ind": "id", "may": "ms", "per": "fa", "cat": "ca", "baq": "eu", "glg": "gl", "ice": "is", "mac": "mk",
	"alb": "sq", "bos": "bs",
}

func kinoraLang(os string) string {
	os = strings.ToLower(os)
	switch os {
	case "pt", "pt-pt":
		return "por"
	case "zh-tw", "zh-ca", "ze":
		return "chi"
	}
	for k, v := range osLang {
		if v == os {
			return k
		}
	}
	return os
}

// OSHash computes the OpenSubtitles hash of a file: its size plus the 64-bit little-endian
// word sums of the first and the last 64 KiB.
func OSHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return "", err
	}
	const chunk = 65536
	size := fi.Size()
	if size < chunk*2 {
		return "", errors.New("a fájl túl kicsi a hash-hez")
	}
	sum := uint64(size)
	buf := make([]byte, chunk)
	for _, off := range []int64{0, size - chunk} {
		if _, err := f.ReadAt(buf, off); err != nil {
			return "", err
		}
		for i := 0; i < chunk; i += 8 {
			sum += binary.LittleEndian.Uint64(buf[i:])
		}
	}
	return fmt.Sprintf("%016x", sum), nil
}

func (o *OpenSubtitles) creds() (key, user, pass string) {
	return strings.TrimSpace(o.st.Setting("opensubtitles_key")), strings.TrimSpace(o.st.Setting("opensubtitles_user")),
		o.st.Setting("opensubtitles_pass")
}

// do sends a request; auth adds the login token when credentials are set.
func (o *OpenSubtitles) do(ctx context.Context, method, path string, q url.Values, body any, auth bool, out any) error {
	key, _, _ := o.creds()
	if key == "" {
		return ErrSubsNotConfigured
	}
	base := o.BaseURL
	tok := ""
	if auth {
		var err error
		if tok, base, err = o.login(ctx); err != nil {
			return err
		}
	}
	u := base + path
	if len(q) > 0 {
		u += "?" + q.Encode() // Encode sorts the keys: OpenSubtitles redirects unsorted queries
	}
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Api-Key", key)
	req.Header.Set("User-Agent", o.UserAgent)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := o.hc.Do(req)
	if err != nil {
		return fmt.Errorf("az OpenSubtitles nem érhető el: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode >= 300 {
		var e struct {
			Message string   `json:"message"`
			Errors  []string `json:"errors"`
		}
		json.Unmarshal(data, &e)
		msg := e.Message
		if msg == "" && len(e.Errors) > 0 {
			msg = strings.Join(e.Errors, ", ")
		}
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			if auth {
				o.mu.Lock()
				o.token = "" // log in again next time
				o.mu.Unlock()
			}
			return fmt.Errorf("az OpenSubtitles elutasította a hozzáférést (API-kulcs vagy belépés): %s", msg)
		case http.StatusNotAcceptable:
			return fmt.Errorf("elfogyott a mai OpenSubtitles letöltési keret: %s", msg)
		case http.StatusTooManyRequests:
			return errors.New("túl sok kérés az OpenSubtitles felé, próbáld újra pár másodperc múlva")
		}
		if msg == "" {
			msg = resp.Status
		}
		return fmt.Errorf("OpenSubtitles hiba: %s", msg)
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("érthetetlen OpenSubtitles válasz: %w", err)
		}
	}
	return nil
}

// login returns a token and the API base for it (no token without a username).
func (o *OpenSubtitles) login(ctx context.Context) (string, string, error) {
	key, user, pass := o.creds()
	if user == "" || pass == "" {
		return "", o.BaseURL, nil
	}
	of := user + "|" + pass + "|" + key
	o.mu.Lock()
	if o.token != "" && o.tokenOf == of && time.Since(o.tokenAt) < 20*time.Hour {
		t, h := o.token, o.host
		o.mu.Unlock()
		return t, h, nil
	}
	o.mu.Unlock()
	var r struct {
		Token   string `json:"token"`
		BaseURL string `json:"base_url"`
	}
	if err := o.do(ctx, "POST", "/login", nil, map[string]string{"username": user, "password": pass}, false, &r); err != nil {
		return "", "", fmt.Errorf("OpenSubtitles belépés: %w", err)
	}
	host := o.BaseURL
	if r.BaseURL != "" && strings.HasPrefix(o.BaseURL, "https://api.opensubtitles.com") {
		host = "https://" + strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(r.BaseURL, "https://"), "http://"), "/") + "/api/v1"
	}
	o.mu.Lock()
	o.token, o.host, o.tokenAt, o.tokenOf = r.Token, host, time.Now(), of
	o.mu.Unlock()
	return r.Token, host, nil
}

type osSearchResp struct {
	Data []struct {
		Attributes struct {
			Language          string  `json:"language"`
			DownloadCount     int     `json:"download_count"`
			HearingImpaired   bool    `json:"hearing_impaired"`
			ForeignPartsOnly  bool    `json:"foreign_parts_only"`
			FromTrusted       bool    `json:"from_trusted"`
			AITranslated      bool    `json:"ai_translated"`
			MachineTranslated bool    `json:"machine_translated"`
			MoviehashMatch    bool    `json:"moviehash_match"`
			Ratings           float64 `json:"ratings"`
			FPS               float64 `json:"fps"`
			Release           string  `json:"release"`
			UploadDate        string  `json:"upload_date"`
			Uploader          struct {
				Name string `json:"name"`
			} `json:"uploader"`
			Files []struct {
				FileID   int64  `json:"file_id"`
				FileName string `json:"file_name"`
			} `json:"files"`
		} `json:"attributes"`
	} `json:"data"`
}

// Search lists subtitles for an item, best first: exact file matches, the profile's
// language order, trusted uploaders, then popularity. Multi-CD subtitles are skipped.
func (o *OpenSubtitles) Search(ctx context.Context, sq SubtitleQuery) ([]SubtitleResult, error) {
	if !o.Enabled() {
		return nil, ErrSubsNotConfigured
	}
	q := url.Values{}
	var langs []string
	rank := map[string]int{}
	for i, l := range sq.Langs {
		if c := osLang[l]; c != "" {
			if _, dup := rank[l]; !dup {
				langs = append(langs, c)
				rank[l] = i
			}
		}
	}
	if len(langs) > 0 {
		sorted := append([]string(nil), langs...)
		sort.Strings(sorted)
		q.Set("languages", strings.Join(sorted, ","))
	}
	if sq.Hash != "" {
		q.Set("moviehash", sq.Hash)
	}
	switch {
	case sq.Episode:
		if sq.SeriesImdb != "" {
			q.Set("parent_imdb_id", strings.TrimLeft(strings.TrimPrefix(sq.SeriesImdb, "tt"), "0"))
		} else if sq.TmdbID > 0 {
			q.Set("parent_tmdb_id", strconv.FormatInt(sq.TmdbID, 10))
		} else if sq.Title != "" {
			q.Set("query", strings.ToLower(sq.Title))
		}
		if sq.Season > 0 {
			q.Set("season_number", strconv.Itoa(sq.Season))
		}
		if sq.EpNum > 0 {
			q.Set("episode_number", strconv.Itoa(sq.EpNum))
		}
	case sq.ImdbID != "":
		q.Set("imdb_id", strings.TrimLeft(strings.TrimPrefix(sq.ImdbID, "tt"), "0"))
	case sq.TmdbID > 0:
		q.Set("tmdb_id", strconv.FormatInt(sq.TmdbID, 10))
	default:
		if sq.Title == "" {
			return nil, errors.New("nincs mi alapján keresni (cím vagy azonosító)")
		}
		q.Set("query", strings.ToLower(sq.Title))
		if sq.Year > 0 {
			q.Set("year", strconv.Itoa(sq.Year))
		}
		q.Set("type", "movie")
	}
	var r osSearchResp
	if err := o.do(ctx, "GET", "/subtitles", q, nil, false, &r); err != nil {
		return nil, err
	}
	out := []SubtitleResult{}
	for _, d := range r.Data {
		a := d.Attributes
		if len(a.Files) != 1 {
			continue
		}
		out = append(out, SubtitleResult{FileID: a.Files[0].FileID, FileName: a.Files[0].FileName, Lang: kinoraLang(a.Language),
			Release: strings.TrimSpace(a.Release), Downloads: a.DownloadCount, Rating: a.Ratings, HearingImpaired: a.HearingImpaired,
			Forced: a.ForeignPartsOnly, HashMatch: a.MoviehashMatch, Trusted: a.FromTrusted, MachineMade: a.AITranslated || a.MachineTranslated,
			FPS: a.FPS, Uploader: a.Uploader.Name, Uploaded: a.UploadDate})
	}
	langRank := func(l string) int {
		if r, ok := rank[l]; ok {
			return r
		}
		return len(rank)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if langRank(a.Lang) != langRank(b.Lang) {
			return langRank(a.Lang) < langRank(b.Lang)
		}
		if a.HashMatch != b.HashMatch {
			return a.HashMatch
		}
		if a.MachineMade != b.MachineMade {
			return !a.MachineMade
		}
		if a.Trusted != b.Trusted {
			return a.Trusted
		}
		return a.Downloads > b.Downloads
	})
	return out, nil
}

// Downloaded is a fetched subtitle file.
type Downloaded struct {
	Data      []byte
	FileName  string
	Remaining int // downloads left today (-1 = unknown)
}

// Download fetches a subtitle file as SRT.
func (o *OpenSubtitles) Download(ctx context.Context, fileID int64) (*Downloaded, error) {
	var r struct {
		Link      string `json:"link"`
		FileName  string `json:"file_name"`
		Remaining *int   `json:"remaining"`
		Message   string `json:"message"`
	}
	if err := o.do(ctx, "POST", "/download", nil, map[string]any{"file_id": fileID, "sub_format": "srt"}, true, &r); err != nil {
		return nil, err
	}
	if r.Link == "" {
		if r.Message == "" {
			r.Message = "nincs letöltési link"
		}
		return nil, fmt.Errorf("OpenSubtitles: %s", r.Message)
	}
	req, err := http.NewRequestWithContext(ctx, "GET", r.Link, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", o.UserAgent)
	resp, err := o.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("a feliratfájl letöltése nem sikerült: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("a feliratfájl letöltése nem sikerült: %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, errors.New("üres feliratfájl érkezett")
	}
	d := &Downloaded{Data: data, FileName: r.FileName, Remaining: -1}
	if r.Remaining != nil {
		d.Remaining = *r.Remaining
	}
	return d, nil
}
