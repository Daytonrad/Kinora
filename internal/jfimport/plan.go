package jfimport

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ScotyLucas/kinora/internal/store"
)

// Source is the import_links source of Jellyfin accounts.
const Source = "jellyfin"

// PathMap rewrites a Jellyfin path prefix to the one Kinora sees ("/data/movies" → "/media/movies").
type PathMap struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// apply maps p with the first rule whose From is a whole-folder prefix of it.
func apply(p string, maps []PathMap) string {
	norm := func(s string) string { return strings.TrimRight(strings.ReplaceAll(s, `\`, "/"), "/") }
	pp := norm(p)
	for _, m := range maps {
		from := norm(m.From)
		if from == "" {
			continue
		}
		if pp == from || strings.HasPrefix(pp, from+"/") {
			to := strings.TrimRight(m.To, `/\`)
			rest := strings.TrimPrefix(pp, from)
			if strings.Contains(m.To, `\`) { // a Windows target: keep its separators
				rest = strings.ReplaceAll(rest, "/", `\`)
			}
			return to + rest
		}
	}
	return p
}

// User actions in a plan.
const (
	ActionCreate   = "create"   // a new Kinora account, without a password (a password link is made)
	ActionExisting = "existing" // an account with the same name exists: its first profile gets the state
	ActionLinked   = "linked"   // an earlier import made this account
	ActionSkip     = "skip"     // disabled in Jellyfin
)

// Match methods and reasons (codes; the web client shows them translated).
const (
	ByPath  = "path"
	ByTmdb  = "tmdb"
	ByImdb  = "imdb"
	ByTitle = "title" // exact title + year, only when a single item fits

	NoMatch          = "no_match"           // neither path, ids nor title found it
	NoSeries         = "series_not_found"   // the episode's series is not in Kinora
	NoEpisode        = "episode_not_found"  // the series is, the episode number is not
	NoEpisodeNumbers = "no_episode_numbers" // Jellyfin has no season/episode number for it
	Ambiguous        = "ambiguous_title"    // the title fits several items
)

// Entry is one Jellyfin item with state, and what it matched.
type Entry struct {
	JellyfinID string  `json:"jellyfin_id"`
	Kind       string  `json:"kind"` // movie | series | episode
	Title      string  `json:"title"`
	Series     string  `json:"series,omitempty"`
	Season     *int    `json:"season,omitempty"`
	Episode    *int    `json:"episode,omitempty"`
	Year       int     `json:"year,omitempty"`
	Path       string  `json:"path,omitempty"`
	Played     bool    `json:"played"`
	PlayCount  int     `json:"play_count"`
	Position   float64 `json:"position"`
	Favorite   bool    `json:"favorite"`
	LastPlayed int64   `json:"last_played,omitempty"`

	Match  *store.ItemRef `json:"match,omitempty"`
	Method string         `json:"method,omitempty"`
	Reason string         `json:"reason,omitempty"`
}

// UserPlan is what happens to one Jellyfin account.
type UserPlan struct {
	JellyfinID  string  `json:"jellyfin_id"`
	Name        string  `json:"name"`
	IsAdmin     bool    `json:"is_admin"`
	Action      string  `json:"action"`
	KinoraID    int64   `json:"kinora_id,omitempty"`
	KinoraName  string  `json:"kinora_name"`
	Matched     int     `json:"matched"`
	Unmatched   int     `json:"unmatched"`
	Entries     []Entry `json:"entries"`
	ItemsListed int     `json:"items_listed"` // every item Jellyfin listed, also those without state
}

// Plan is the preview of an import.
type Plan struct {
	ID         string         `json:"id"`
	CreatedAt  int64          `json:"created_at"`
	ServerName string         `json:"server_name"`
	Version    string         `json:"version"`
	URL        string         `json:"url"`
	Maps       []PathMap      `json:"path_maps"`
	Users      []*UserPlan    `json:"users"`
	Matched    int            `json:"matched"`
	Unmatched  int            `json:"unmatched"`
	ByMethod   map[string]int `json:"by_method"`
	ByReason   map[string]int `json:"by_reason"`
}

// Preview reads Jellyfin and matches everything against the library. It writes nothing.
func Preview(ctx context.Context, c *Client, st *store.Store, maps []PathMap) (*Plan, error) {
	info, err := c.Info(ctx)
	if err != nil {
		return nil, err
	}
	users, err := c.Users(ctx)
	if err != nil {
		return nil, err
	}
	p := &Plan{ID: store.RandomToken(8), CreatedAt: time.Now().Unix(), ServerName: info.ServerName, Version: info.Version, URL: c.BaseURL,
		Maps: maps, ByMethod: map[string]int{}, ByReason: map[string]int{}}
	m := &matcher{st: st, maps: maps, series: map[string]*Item{}, seriesHit: map[string]seriesResult{}}
	sort.Slice(users, func(a, b int) bool { return strings.ToLower(users[a].Name) < strings.ToLower(users[b].Name) })
	for _, u := range users {
		up := &UserPlan{JellyfinID: u.ID, Name: u.Name, IsAdmin: u.Policy.IsAdministrator, KinoraName: kinoraName(u.Name), Entries: []Entry{}}
		p.Users = append(p.Users, up)
		switch k, err := st.ImportedUser(Source, u.ID); {
		case u.Policy.IsDisabled:
			up.Action = ActionSkip
			continue
		case err == nil:
			up.Action, up.KinoraID, up.KinoraName = ActionLinked, k.ID, k.Username
		default:
			if k, err := st.UserByName(up.KinoraName); err == nil {
				up.Action, up.KinoraID, up.KinoraName = ActionExisting, k.ID, k.Username
			} else {
				up.Action = ActionCreate
			}
		}
		items, err := c.Items(ctx, u.ID)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", u.Name, err)
		}
		up.ItemsListed = len(items)
		for i := range items {
			if items[i].Type == "Series" {
				m.series[items[i].ID] = &items[i]
			}
		}
		for i := range items {
			it := &items[i]
			if !it.UserData.Interesting() || (it.Type == "Series" && !it.UserData.IsFavorite) {
				continue // a series' own "played" is the sum of its episodes, which come one by one
			}
			e := m.match(it)
			if e == nil {
				continue
			}
			if e.Match != nil {
				up.Matched++
				p.ByMethod[e.Method]++
			} else {
				up.Unmatched++
				p.ByReason[e.Reason]++
			}
			up.Entries = append(up.Entries, *e)
		}
		p.Matched += up.Matched
		p.Unmatched += up.Unmatched
	}
	return p, nil
}

type seriesResult struct {
	ref    *store.ItemRef
	method string
	reason string
}

type matcher struct {
	st        *store.Store
	maps      []PathMap
	series    map[string]*Item // Jellyfin series by id (for their provider ids)
	seriesHit map[string]seriesResult
}

func (m *matcher) match(it *Item) *Entry {
	e := &Entry{JellyfinID: it.ID, Title: it.Name, Year: it.ProductionYear, Path: it.Path,
		Played: it.UserData.Played, PlayCount: it.UserData.PlayCount, Position: it.UserData.PositionSeconds(),
		Favorite: it.UserData.IsFavorite, LastPlayed: it.UserData.LastPlayed()}
	switch it.Type {
	case "Movie":
		e.Kind = "movie"
	case "Episode":
		e.Kind = "episode"
		e.Series, e.Season, e.Episode = it.SeriesName, it.ParentIndexNumber, it.IndexNumber
	case "Series":
		e.Kind = "series"
	default:
		return nil
	}
	// 1. the file path, mapped to how Kinora sees the library
	if e.Kind != "series" && it.Path != "" {
		if r, err := m.st.ItemByPath(apply(it.Path, m.maps)); err == nil {
			e.Match, e.Method = r, ByPath
			return e
		}
	}
	switch e.Kind {
	case "movie", "series":
		r, method, reason := m.byIDsOrTitle(e.Kind, it)
		e.Match, e.Method, e.Reason = r, method, reason
	case "episode":
		if it.ParentIndexNumber == nil || it.IndexNumber == nil {
			e.Reason = NoEpisodeNumbers
			return e
		}
		sr := m.seriesOf(it)
		if sr.ref == nil {
			e.Reason = NoSeries
			return e
		}
		r, err := m.st.EpisodeOf(sr.ref.ID, *it.ParentIndexNumber, *it.IndexNumber)
		if err != nil {
			e.Reason = NoEpisode
			return e
		}
		e.Match, e.Method = r, sr.method
	}
	return e
}

func (m *matcher) byIDsOrTitle(kind string, it *Item) (*store.ItemRef, string, string) {
	if r, method, err := m.st.FindByIDs(kind, it.TmdbID(), it.Provider("Imdb")); err == nil {
		return r, method, ""
	}
	r, err := m.st.FindByTitle(kind, it.Name, it.ProductionYear)
	switch {
	case err == nil:
		return r, ByTitle, ""
	case errors.Is(err, store.ErrAmbiguous):
		return nil, "", Ambiguous
	}
	return nil, "", NoMatch
}

// seriesOf finds the Kinora series of an episode (cached per Jellyfin series).
func (m *matcher) seriesOf(ep *Item) seriesResult {
	key := ep.SeriesID
	if key == "" {
		key = "name:" + ep.SeriesName
	}
	if r, ok := m.seriesHit[key]; ok {
		return r
	}
	s := m.series[ep.SeriesID]
	if s == nil {
		s = &Item{Name: ep.SeriesName, Type: "Series"}
	}
	var res seriesResult
	res.ref, res.method, res.reason = m.byIDsOrTitle("series", s)
	if res.ref == nil && ep.Path != "" {
		// the series folder: the episode's mapped path sits below a Kinora episode's series
		res.ref, res.method = m.seriesFromSiblings(apply(ep.Path, m.maps))
	}
	m.seriesHit[key] = res
	return res
}

// seriesFromSiblings looks for any Kinora episode in the episode's folder or the one above it
// (the series folder above a season folder).
func (m *matcher) seriesFromSiblings(p string) (*store.ItemRef, string) {
	dir := parentDir(p)
	for i := 0; i < 2 && len(dir) > 1; i++ {
		if id := m.st.SeriesUnder(dir); id > 0 {
			if r, err := m.st.ItemRefByID(id); err == nil {
				return r, ByPath
			}
		}
		dir = parentDir(dir)
	}
	return nil, ""
}

// parentDir strips the last element of a "/" or "\\" separated path, keeping its separators.
func parentDir(p string) string {
	p = strings.TrimRight(p, `/\`)
	if i := strings.LastIndexAny(p, `/\`); i > 0 {
		return p[:i]
	}
	return ""
}

// kinoraName turns a Jellyfin user name into a valid Kinora one (2–32 characters, no / \ < > " ').
func kinoraName(name string) string {
	n := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\<>"'`, r) {
			return '_'
		}
		return r
	}, strings.TrimSpace(name))
	if r := []rune(n); len(r) > 32 {
		n = string(r[:32])
	}
	for len([]rune(n)) < 2 {
		n += "_"
	}
	return n
}

// ------------------------------------------------------------------ apply

// UserResult is what Apply did for one account.
type UserResult struct {
	JellyfinID   string `json:"jellyfin_id"`
	Name         string `json:"name"`
	KinoraID     int64  `json:"kinora_id"`
	KinoraName   string `json:"kinora_name"`
	Created      bool   `json:"created"`
	PasswordLink string `json:"password_code,omitempty"` // code of the one-time link (new accounts)
	LinkExpires  int64  `json:"password_expires,omitempty"`
	Changed      int    `json:"changed"`   // items whose state changed
	Unchanged    int    `json:"unchanged"` // matched, already up to date (a repeated import)
	Unmatched    int    `json:"unmatched"`
	Error        string `json:"error,omitempty"`
}

// Report is the result of Apply.
type Report struct {
	PlanID  string        `json:"plan_id"`
	Users   []*UserResult `json:"users"`
	Changed int           `json:"changed"`
}

// Apply carries out a plan for the chosen Jellyfin users (nil = every user the plan does not skip).
func Apply(st *store.Store, p *Plan, only []string) (*Report, error) {
	want := map[string]bool{}
	for _, id := range only {
		want[id] = true
	}
	rep := &Report{PlanID: p.ID}
	for _, up := range p.Users {
		if up.Action == ActionSkip || (len(want) > 0 && !want[up.JellyfinID]) {
			continue
		}
		res := &UserResult{JellyfinID: up.JellyfinID, Name: up.Name, Unmatched: up.Unmatched}
		rep.Users = append(rep.Users, res)
		u, created, err := account(st, up)
		if err != nil {
			res.Error = err.Error()
			continue
		}
		res.KinoraID, res.KinoraName, res.Created = u.ID, u.Username, created
		if err := st.LinkImportedUser(Source, up.JellyfinID, u.ID); err != nil {
			return nil, err
		}
		if !u.HasPassword() {
			// a new account, or one an earlier import made whose link was never used: a fresh link
			if res.PasswordLink, res.LinkExpires, err = st.CreatePasswordLink(u.ID); err != nil {
				return nil, err
			}
		}
		prof, err := st.FirstProfile(u.ID)
		if err != nil {
			res.Error = err.Error()
			continue
		}
		for _, e := range up.Entries {
			if e.Match == nil {
				continue
			}
			changed, err := st.MergeUserData(prof.ID, e.Match.ID, store.ImportedUserData{
				Played: e.Played, PlayCount: e.PlayCount, Position: e.Position, Favorite: e.Favorite, LastPlayed: e.LastPlayed})
			if err != nil {
				if errors.Is(err, store.ErrNotFound) {
					continue // removed from the library since the preview
				}
				return nil, err
			}
			if changed {
				res.Changed++
			} else {
				res.Unchanged++
			}
		}
		rep.Changed += res.Changed
	}
	return rep, nil
}

// account returns the Kinora account of a planned user, creating it when needed. It re-checks the
// database: the plan may be some minutes old.
func account(st *store.Store, up *UserPlan) (*store.User, bool, error) {
	if u, err := st.ImportedUser(Source, up.JellyfinID); err == nil {
		return u, false, nil
	}
	name := kinoraName(up.Name)
	if u, err := st.UserByName(name); err == nil {
		return u, false, nil
	}
	u, err := st.CreateUserWithoutPassword(name)
	if err != nil {
		return nil, false, err
	}
	return u, true, nil
}
