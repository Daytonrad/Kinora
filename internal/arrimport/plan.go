package arrimport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ScotyLucas/kinora/internal/acquire"
	"github.com/ScotyLucas/kinora/internal/store"
)

// PathMap rewrites a path as the app sees it to the one Kinora sees ("/tv" → "/media/shows").
type PathMap struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func mapPath(p string, maps []PathMap) string {
	norm := func(s string) string { return strings.TrimRight(strings.ReplaceAll(s, `\`, "/"), "/") }
	pp := norm(p)
	for _, m := range maps {
		from := norm(m.From)
		if from != "" && (pp == from || strings.HasPrefix(pp, from+"/")) {
			return filepath.FromSlash(strings.TrimRight(m.To, `/\`) + strings.TrimPrefix(pp, from))
		}
	}
	return p
}

// Actions.
const (
	ActCreate   = "create"   // new in Kinora
	ActUpdate   = "update"   // made by an earlier import: updated
	ActExisting = "existing" // Kinora already has an equal one: left alone
	ActLink     = "link"     // a root folder that is already a Kinora library
	ActSkip     = "skip"
	ActRequest  = "request" // a wanted film becomes an approved request
	ActFollow   = "follow"  // a series becomes a followed series
)

// Reasons for skipping, and notes (codes; the web client translates them).
const (
	RsUnsupported    = "unsupported"    // an indexer or client type Kinora has no engine for
	RsDisabled       = "disabled"       // disabled in the app
	RsUnmonitored    = "unmonitored"    // not monitored in the app
	RsHasFile        = "has_file"       // the app already has the film: it is in the library, nothing to get
	RsInLibrary      = "in_library"     // Kinora already has the film
	RsRequested      = "requested"      // Kinora already has an open request for it
	RsNoTmdb         = "no_tmdb_id"     // no TMDB id, and none found
	RsAcqOff         = "acquire_off"    // Kinora's acquisition is switched off
	RsNoTMDBKey      = "no_tmdb_key"    // TMDB is needed to follow series and request films
	RsUnreachable    = "unreachable"    // the root folder does not exist on Kinora's side
	NoteNeedsSecret  = "needs_secret"   // the app hides the password / API key: enter it in Kinora, the entry stays off
	NoteSizes        = "sizes_default"  // size limits: Kinora's defaults (the app keeps them elsewhere)
	NoteCustom       = "custom_formats" // custom formats and language rules have no counterpart
	NoteNoResolution = "no_resolution"  // no allowed quality with a known resolution: Kinora's default list
)

// ProfilePlan is a quality profile.
type ProfilePlan struct {
	ArrID   int64           `json:"arr_id"`
	Name    string          `json:"name"`
	Action  string          `json:"action"`
	LocalID int64           `json:"local_id,omitempty"`
	Profile acquire.Profile `json:"profile"`
	Notes   []string        `json:"notes"`
}

// IndexerPlan is an indexer.
type IndexerPlan struct {
	ArrID      int64    `json:"arr_id"`
	Name       string   `json:"name"`
	Kind       string   `json:"kind"`
	URL        string   `json:"url"`
	Categories string   `json:"categories"`
	Priority   int      `json:"priority"`
	Enabled    bool     `json:"enabled"`
	Action     string   `json:"action"`
	LocalID    int64    `json:"local_id,omitempty"`
	Reason     string   `json:"reason,omitempty"`
	Notes      []string `json:"notes"`
	apiKey     string
}

// ClientPlan is a download client.
type ClientPlan struct {
	ArrID    int64    `json:"arr_id"`
	Name     string   `json:"name"`
	Kind     string   `json:"kind"`
	URL      string   `json:"url"`
	Username string   `json:"username"`
	Category string   `json:"category"`
	Action   string   `json:"action"`
	LocalID  int64    `json:"local_id,omitempty"`
	Reason   string   `json:"reason,omitempty"`
	Notes    []string `json:"notes"`
	password string
}

// FolderPlan is a root folder.
type FolderPlan struct {
	Path    string `json:"path"`
	Mapped  string `json:"mapped"`
	Action  string `json:"action"`
	LocalID int64  `json:"local_id,omitempty"` // the library
	Reason  string `json:"reason,omitempty"`
}

// TitlePlan is a film or a series.
type TitlePlan struct {
	ArrID     int64  `json:"arr_id"`
	Title     string `json:"title"`
	Year      int    `json:"year"`
	TmdbID    int64  `json:"tmdb_id"`
	TvdbID    int64  `json:"tvdb_id,omitempty"`
	ImdbID    string `json:"imdb_id,omitempty"`
	Monitor   string `json:"monitor,omitempty"` // series: all | seasons | future
	Seasons   []int  `json:"seasons,omitempty"`
	Folder    string `json:"folder,omitempty"` // the app's root folder of the title
	ProfileID int64  `json:"profile_arr_id,omitempty"`
	Action    string `json:"action"`
	Reason    string `json:"reason,omitempty"`
}

// Plan is the preview of an import from one app.
type Plan struct {
	ID       string         `json:"id"`
	App      string         `json:"app"`
	URL      string         `json:"url"`
	Version  string         `json:"version"`
	Instance string         `json:"instance"`
	Maps     []PathMap      `json:"path_maps"`
	Profiles []*ProfilePlan `json:"profiles"`
	Indexers []*IndexerPlan `json:"indexers"`
	Clients  []*ClientPlan  `json:"clients"`
	Folders  []*FolderPlan  `json:"folders"`
	Movies   []*TitlePlan   `json:"movies"`
	Series   []*TitlePlan   `json:"series"`
	Blockers []string       `json:"blockers"` // why films / series cannot be taken over now
	source   string
}

// TMDB is what the import needs from the metadata fetcher.
type TMDB interface {
	Enabled() bool
	FindByExternal(ctx context.Context, kind, source, id string) (int64, error)
}

// Deps are the parts of Kinora the import works with.
type Deps struct {
	Store   *store.Store
	Acquire *acquire.Manager
	TMDB    TMDB
	// Scan starts a scan of a library the import created.
	Scan func(libID int64)
}

func sourceKey(app, base string) string {
	return app + "@" + strings.ToLower(strings.TrimRight(base, "/"))
}

// Preview reads the app and decides what to do with each object. It writes nothing.
func Preview(ctx context.Context, c *Client, d Deps, maps []PathMap) (*Plan, error) {
	st, err := c.Status(ctx)
	if err != nil {
		return nil, err
	}
	p := &Plan{ID: store.RandomToken(8), App: c.App, URL: c.BaseURL, Version: st.Version, Instance: st.InstanceName, Maps: maps,
		Profiles: []*ProfilePlan{}, Indexers: []*IndexerPlan{}, Clients: []*ClientPlan{}, Folders: []*FolderPlan{},
		Movies: []*TitlePlan{}, Series: []*TitlePlan{}, Blockers: []string{}, source: sourceKey(c.App, c.BaseURL)}

	ixs, err := c.Indexers(ctx)
	if err != nil {
		return nil, err
	}
	existingIx, _ := d.Store.AcqIndexers()
	for _, ix := range ixs {
		p.Indexers = append(p.Indexers, planIndexer(c, d.Store, p.source, ix, existingIx))
	}
	if c.App == Prowlarr {
		return p, nil // Prowlarr has only indexers
	}

	qps, err := c.QualityProfiles(ctx)
	if err != nil {
		return nil, err
	}
	for _, qp := range qps {
		pp := &ProfilePlan{ArrID: qp.ID, Name: qp.Name, Action: ActCreate}
		pp.Profile, pp.Notes = MapProfile(qp)
		if id := d.Store.ImportedID(p.source, "profile", strconv.FormatInt(qp.ID, 10)); id > 0 {
			if _, err := d.Store.AcqProfileByID(id); err == nil {
				pp.Action, pp.LocalID = ActUpdate, id
			}
		}
		p.Profiles = append(p.Profiles, pp)
	}

	dcs, err := c.DownloadClients(ctx)
	if err != nil {
		return nil, err
	}
	existingCl, _ := d.Store.AcqClients()
	for _, dc := range dcs {
		p.Clients = append(p.Clients, planClient(c.App, d.Store, p.source, dc, existingCl))
	}

	rfs, err := c.RootFolders(ctx)
	if err != nil {
		return nil, err
	}
	libs, _ := d.Store.Libraries()
	for _, rf := range rfs {
		p.Folders = append(p.Folders, planFolder(rf.Path, maps, libs))
	}

	if d.Acquire == nil || !d.Acquire.Enabled() {
		p.Blockers = append(p.Blockers, RsAcqOff)
	}
	tmdbOK := d.TMDB != nil && d.TMDB.Enabled()
	if !tmdbOK {
		p.Blockers = append(p.Blockers, RsNoTMDBKey)
	}

	switch c.App {
	case Radarr:
		movies, err := c.Movies(ctx)
		if err != nil {
			return nil, err
		}
		have := map[int64]int64{}
		var ids []int64
		for _, m := range movies {
			ids = append(ids, m.TmdbID)
		}
		if len(ids) > 0 {
			have, _ = d.Store.MovieTmdbIDs(ids)
		}
		for _, m := range movies {
			t := &TitlePlan{ArrID: m.ID, Title: m.Title, Year: m.Year, TmdbID: m.TmdbID, ImdbID: m.ImdbID, ProfileID: m.QualityProfileID,
				Folder: firstNonEmpty(m.RootFolderPath, parent(m.Path)), Action: ActRequest}
			switch {
			case !m.Monitored:
				t.Action, t.Reason = ActSkip, RsUnmonitored
			case m.HasFile:
				t.Action, t.Reason = ActSkip, RsHasFile
			case have[m.TmdbID] > 0:
				t.Action, t.Reason = ActSkip, RsInLibrary
			case m.TmdbID <= 0:
				t.Action, t.Reason = ActSkip, RsNoTmdb
			default:
				if _, err := d.Store.OpenRequestFor("movie", m.TmdbID); err == nil {
					t.Action, t.Reason = ActExisting, RsRequested
				}
			}
			p.Movies = append(p.Movies, t)
		}
	case Sonarr:
		series, err := c.Series(ctx)
		if err != nil {
			return nil, err
		}
		for _, s := range series {
			t := &TitlePlan{ArrID: s.ID, Title: s.Title, Year: s.Year, TmdbID: s.TmdbID, TvdbID: s.TvdbID, ImdbID: s.ImdbID,
				ProfileID: s.QualityProfileID, Folder: firstNonEmpty(s.RootFolderPath, parent(s.Path)), Action: ActFollow}
			t.Monitor, t.Seasons = monitorOf(s)
			if !s.Monitored {
				t.Action, t.Reason = ActSkip, RsUnmonitored
				p.Series = append(p.Series, t)
				continue
			}
			if t.TmdbID <= 0 && tmdbOK {
				// older Sonarr answers without tmdbId: TMDB knows the series by its TVDB (or IMDb) id
				if id, err := d.TMDB.FindByExternal(ctx, "tv", "tvdb_id", strconv.FormatInt(s.TvdbID, 10)); err == nil && id > 0 {
					t.TmdbID = id
				} else if id, err := d.TMDB.FindByExternal(ctx, "tv", "imdb_id", s.ImdbID); err == nil && id > 0 {
					t.TmdbID = id
				}
			}
			if t.TmdbID <= 0 {
				t.Action, t.Reason = ActSkip, RsNoTmdb
			} else if x, err := d.Store.AcqSeriesByTmdb(t.TmdbID); err == nil && x != nil {
				t.Action = ActUpdate
			}
			p.Series = append(p.Series, t)
		}
	}
	return p, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func parent(p string) string {
	p = strings.TrimRight(p, `/\`)
	if i := strings.LastIndexAny(p, `/\`); i > 0 {
		return p[:i]
	}
	return ""
}

// monitorOf turns Sonarr's monitored seasons into a Kinora monitor mode. Specials (season 0) do not count.
func monitorOf(s Series) (string, []int) {
	var on []int
	all := true
	for _, se := range s.Seasons {
		if se.SeasonNumber == 0 {
			continue
		}
		if se.Monitored {
			on = append(on, se.SeasonNumber)
		} else {
			all = false
		}
	}
	switch {
	case len(on) == 0:
		return acquire.MonitorFuture, nil // only what airs from now on
	case all:
		return acquire.MonitorAll, nil
	}
	sort.Ints(on)
	return acquire.MonitorSeasons, on
}

// ------------------------------------------------------------------ quality profiles

// arrSource maps Radarr's and Sonarr's source names to Kinora's.
func arrSource(name, source string) string {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "remux"):
		return "remux"
	case strings.Contains(n, "dvdscr") || strings.Contains(n, "screener"):
		return "scr"
	}
	switch strings.ToLower(source) {
	case "bluray", "blurayraw":
		return "bluray"
	case "webdl", "web":
		return "webdl"
	case "webrip":
		return "webrip"
	case "tv", "television", "televisionraw":
		return "hdtv"
	case "dvd":
		return "dvd"
	case "cam":
		return "cam"
	case "telesync":
		return "ts"
	case "telecine":
		return "tc"
	}
	return ""
}

type flatQuality struct {
	id, res      int
	name, source string
	allowed      bool
	group        int
}

func flatten(items []QualityItem) []flatQuality {
	var out []flatQuality
	for _, it := range items {
		if it.Quality != nil {
			out = append(out, flatQuality{id: it.Quality.ID, res: it.Quality.Resolution, name: it.Quality.Name,
				source: arrSource(it.Quality.Name, it.Quality.Source), allowed: it.Allowed})
			continue
		}
		for _, sub := range flatten(it.Items) {
			sub.allowed = sub.allowed || it.Allowed // a group's "allowed" covers its members
			sub.group = it.ID
			out = append(out, sub)
		}
	}
	return out
}

// MapProfile maps a Sonarr/Radarr quality profile onto Kinora's: the allowed resolutions (best first), the
// blocked low-quality sources, and the upgrade cutoff. What has no counterpart keeps Kinora's default and is
// listed in the notes.
func MapProfile(qp QualityProfile) (acquire.Profile, []string) {
	p := acquire.DefaultProfile()
	notes := []string{NoteSizes, NoteCustom}
	qs := flatten(qp.Items)
	seen := map[int]bool{}
	var res []int
	allowedSrc := map[string]bool{}
	for _, q := range qs {
		if !q.allowed {
			continue
		}
		allowedSrc[q.source] = true
		if !seen[q.res] {
			seen[q.res] = true
			res = append(res, q.res)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(res)))
	if len(res) > 0 {
		p.Resolutions = res
	} else {
		notes = append(notes, NoteNoResolution)
	}
	p.BlockedSources = nil
	for _, s := range []string{"cam", "ts", "tc", "scr"} {
		if !allowedSrc[s] {
			p.BlockedSources = append(p.BlockedSources, s)
		}
	}
	p.Upgrade = qp.UpgradeAllowed
	for _, q := range qs {
		if q.id == qp.Cutoff || (q.group != 0 && q.group == qp.Cutoff) {
			p.CutoffRes, p.CutoffSource = q.res, q.source
			break
		}
	}
	return p, notes
}

// ------------------------------------------------------------------ indexers and clients

func planIndexer(c *Client, st *store.Store, source string, ix Indexer, existing []*store.AcqIndexer) *IndexerPlan {
	ip := &IndexerPlan{ArrID: ix.ID, Name: ix.Name, Priority: ix.Priority, Notes: []string{}}
	switch {
	case c.App == Prowlarr:
		// every Prowlarr indexer has a Torznab / Newznab feed of its own, behind Prowlarr's API key
		ip.URL = fmt.Sprintf("%s/%d/api", c.BaseURL, ix.ID)
		ip.apiKey = c.APIKey
		ip.Enabled = ix.Enable
		ip.Kind = map[string]string{"torrent": "torznab", "usenet": "newznab"}[ix.Protocol]
	case strings.EqualFold(ix.Implementation, "Torznab") || strings.EqualFold(ix.Implementation, "Newznab"):
		ip.Kind = strings.ToLower(ix.Implementation)
		ip.URL = strings.TrimRight(ix.Str("baseUrl"), "/") + "/" + strings.TrimLeft(firstNonEmpty(ix.Str("apiPath"), "/api"), "/")
		ip.apiKey = ix.Str("apiKey")
		ip.Enabled = ix.EnableRss || ix.EnableSearch
		var cats []string
		for _, n := range ix.Ints("categories") {
			cats = append(cats, strconv.Itoa(n))
		}
		for _, n := range ix.Ints("animeCategories") {
			cats = append(cats, strconv.Itoa(n))
		}
		ip.Categories = strings.Join(cats, ",")
		if ix.Masked("apiKey") {
			ip.Notes = append(ip.Notes, NoteNeedsSecret)
		}
	}
	if ip.Kind == "" {
		ip.Action, ip.Reason = ActSkip, RsUnsupported
		return ip
	}
	if ip.Priority <= 0 {
		ip.Priority = 25
	}
	if id := st.ImportedID(source, "indexer", strconv.FormatInt(ix.ID, 10)); id > 0 {
		if _, err := st.AcqIndexer(id); err == nil {
			ip.Action, ip.LocalID = ActUpdate, id
			return ip
		}
	}
	for _, e := range existing {
		if strings.EqualFold(strings.TrimRight(e.URL, "/"), strings.TrimRight(ip.URL, "/")) {
			ip.Action, ip.LocalID = ActExisting, e.ID
			return ip
		}
	}
	ip.Action = ActCreate
	if !ip.Enabled {
		ip.Reason = RsDisabled // imported, but stays off
	}
	return ip
}

var clientKinds = map[string]string{"qbittorrent": "qbittorrent", "transmission": "transmission", "deluge": "deluge", "sabnzbd": "sabnzbd", "nzbget": "nzbget"}

func planClient(app string, st *store.Store, source string, dc DownloadClient, existing []*store.AcqClient) *ClientPlan {
	cp := &ClientPlan{ArrID: dc.ID, Name: dc.Name, Kind: clientKinds[strings.ToLower(dc.Implementation)], Notes: []string{}}
	if cp.Kind == "" {
		cp.Action, cp.Reason = ActSkip, RsUnsupported
		return cp
	}
	scheme := "http"
	if dc.Bool("useSsl") {
		scheme = "https"
	}
	host, port := dc.Str("host"), dc.Str("port")
	cp.URL = scheme + "://" + host
	if port != "" {
		cp.URL += ":" + port
	}
	if base := strings.Trim(dc.Str("urlBase"), "/"); base != "" {
		cp.URL += "/" + base
	}
	cp.Username = dc.Str("username")
	secret := "password"
	if cp.Kind == "sabnzbd" {
		secret = "apiKey" // SABnzbd signs requests with its API key; Kinora keeps it in the password field
	}
	cp.password = dc.Str(secret)
	if dc.Masked(secret) {
		cp.Notes = append(cp.Notes, NoteNeedsSecret)
	}
	if app == Sonarr {
		cp.Category = dc.Str("tvCategory")
	} else {
		cp.Category = dc.Str("movieCategory")
	}
	if !dc.Enable {
		cp.Reason = RsDisabled
	}
	if id := st.ImportedID(source, "client", strconv.FormatInt(dc.ID, 10)); id > 0 {
		if _, err := st.AcqClient(id); err == nil {
			cp.Action, cp.LocalID = ActUpdate, id
			return cp
		}
	}
	// Sonarr and Radarr usually share one qBittorrent: the second import fills in its own category
	for _, e := range existing {
		if e.Kind == cp.Kind && strings.EqualFold(strings.TrimRight(e.URL, "/"), cp.URL) {
			cp.Action, cp.LocalID = ActUpdate, e.ID
			return cp
		}
	}
	cp.Action = ActCreate
	return cp
}

func planFolder(path string, maps []PathMap, libs []*store.Library) *FolderPlan {
	fp := &FolderPlan{Path: path, Mapped: mapPath(path, maps)}
	clean := filepath.Clean(fp.Mapped)
	for _, l := range libs {
		for _, lp := range l.Paths {
			if filepath.Clean(lp) == clean {
				fp.Action, fp.LocalID = ActLink, l.ID
				return fp
			}
		}
	}
	if st, err := os.Stat(fp.Mapped); err != nil || !st.IsDir() {
		fp.Action, fp.Reason = ActSkip, RsUnreachable
		return fp
	}
	fp.Action = ActCreate
	return fp
}

// ------------------------------------------------------------------ apply

// Sections of a plan the admin can choose.
const (
	SecProfiles = "profiles"
	SecIndexers = "indexers"
	SecClients  = "clients"
	SecFolders  = "folders"
	SecTitles   = "titles" // films (Radarr) or series (Sonarr)
)

// Progress is what Apply did so far; the web client polls its Snapshot.
type Progress struct {
	Running bool
	Done    int
	Total   int
	Counts  map[string]int // "<section>:<action>" → n
	Errors  []ItemError
	mu      sync.Mutex
}

// ItemError is one object that could not be imported.
type ItemError struct {
	Section string `json:"section"`
	Name    string `json:"name"`
	Error   string `json:"error"`
}

func (pr *Progress) step(section, action string) {
	pr.mu.Lock()
	pr.Done++
	pr.Counts[section+":"+action]++
	pr.mu.Unlock()
}

func (pr *Progress) fail(section, name string, err error) {
	pr.mu.Lock()
	pr.Done++
	pr.Errors = append(pr.Errors, ItemError{Section: section, Name: name, Error: err.Error()})
	pr.mu.Unlock()
}

// ProgressView is a copy of the progress for JSON.
type ProgressView struct {
	Running bool           `json:"running"`
	Done    int            `json:"done"`
	Total   int            `json:"total"`
	Counts  map[string]int `json:"counts"`
	Errors  []ItemError    `json:"errors"`
}

// Snapshot copies the progress for JSON.
func (pr *Progress) Snapshot() ProgressView {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	c := ProgressView{Running: pr.Running, Done: pr.Done, Total: pr.Total, Counts: map[string]int{}, Errors: append([]ItemError{}, pr.Errors...)}
	for k, v := range pr.Counts {
		c.Counts[k] = v
	}
	return c
}

// NewProgress counts the work of the chosen sections.
func NewProgress(p *Plan, sections []string) *Progress {
	want := wanted(sections)
	pr := &Progress{Running: true, Counts: map[string]int{}, Errors: []ItemError{}}
	if want[SecProfiles] {
		pr.Total += len(p.Profiles)
	}
	if want[SecIndexers] {
		pr.Total += len(p.Indexers)
	}
	if want[SecClients] {
		pr.Total += len(p.Clients)
	}
	if want[SecFolders] {
		pr.Total += len(p.Folders)
	}
	if want[SecTitles] {
		pr.Total += len(p.Movies) + len(p.Series)
	}
	return pr
}

func wanted(sections []string) map[string]bool {
	w := map[string]bool{}
	for _, s := range sections {
		w[s] = true
	}
	if len(w) == 0 {
		for _, s := range []string{SecProfiles, SecIndexers, SecClients, SecFolders, SecTitles} {
			w[s] = true
		}
	}
	return w
}

// Apply carries out the chosen sections of a plan, in dependency order: profiles and libraries first, so the
// series and films can use them. It records everything it creates, so running it again updates in place.
func Apply(ctx context.Context, p *Plan, d Deps, sections []string, pr *Progress) {
	defer func() { pr.mu.Lock(); pr.Running = false; pr.mu.Unlock() }()
	want := wanted(sections)
	st := d.Store
	profileIDs := map[int64]int64{} // app profile id → Kinora profile id
	if want[SecProfiles] {
		for _, pp := range p.Profiles {
			b, _ := json.Marshal(pp.Profile)
			ap := &store.AcqProfile{Name: pp.Name, JSON: string(b)}
			if pp.Action == ActUpdate {
				if old, err := st.AcqProfileByID(pp.LocalID); err == nil {
					ap.ID, ap.IsDefault = old.ID, old.IsDefault
				}
			} else if st.ImportedID(p.source, "profile", strconv.FormatInt(pp.ArrID, 10)) > 0 {
				ap.ID = st.ImportedID(p.source, "profile", strconv.FormatInt(pp.ArrID, 10))
			}
			if ap.ID == 0 && profileNameTaken(st, ap.Name) {
				ap.Name += " (" + p.App + ")"
			}
			if err := st.SaveAcqProfile(ap); err != nil {
				pr.fail(SecProfiles, pp.Name, err)
				continue
			}
			st.MapImported(p.source, "profile", strconv.FormatInt(pp.ArrID, 10), ap.ID)
			profileIDs[pp.ArrID] = ap.ID
			pr.step(SecProfiles, pp.Action)
		}
	}
	libFor := map[string]int64{} // app root folder → Kinora library
	for _, fp := range p.Folders {
		if fp.Action == ActLink {
			libFor[fp.Path] = fp.LocalID
		}
	}
	if want[SecFolders] {
		kind := "movies"
		if p.App == Sonarr {
			kind = "shows"
		}
		for _, fp := range p.Folders {
			if fp.Action != ActCreate {
				pr.step(SecFolders, fp.Action)
				continue
			}
			lib, err := st.CreateLibrary(filepath.Base(fp.Mapped), fp.Mapped, kind)
			if err != nil {
				pr.fail(SecFolders, fp.Path, err)
				continue
			}
			libFor[fp.Path] = lib.ID
			if d.Scan != nil {
				d.Scan(lib.ID)
			}
			pr.step(SecFolders, ActCreate)
		}
	}
	if want[SecClients] {
		for _, cp := range p.Clients {
			if cp.Action == ActSkip {
				pr.step(SecClients, ActSkip)
				continue
			}
			c := &store.AcqClient{Name: cp.Name, Kind: cp.Kind, URL: cp.URL, Username: cp.Username, Password: cp.password,
				Category: "kinora", Enabled: cp.Reason == "" && len(cp.Notes) == 0}
			if cp.LocalID > 0 {
				if old, err := st.AcqClient(cp.LocalID); err == nil {
					c = old // keep what the admin set in Kinora; fill in the app's category
				}
			}
			switch {
			case p.App == Sonarr && cp.Category != "":
				c.CategoryTV = cp.Category
			case p.App == Radarr && cp.Category != "":
				c.Category = cp.Category
			}
			if c.Password == "" && cp.password != "" {
				c.Password = cp.password
			}
			if err := st.SaveAcqClient(c); err != nil {
				pr.fail(SecClients, cp.Name, err)
				continue
			}
			st.MapImported(p.source, "client", strconv.FormatInt(cp.ArrID, 10), c.ID)
			pr.step(SecClients, cp.Action)
		}
	}
	if want[SecIndexers] {
		for _, ip := range p.Indexers {
			if ip.Action == ActSkip || ip.Action == ActExisting {
				pr.step(SecIndexers, ip.Action)
				continue
			}
			x := &store.AcqIndexer{Name: ip.Name, Kind: ip.Kind, URL: ip.URL, APIKey: ip.apiKey, Categories: ip.Categories,
				Priority: ip.Priority, Enabled: ip.Enabled && len(ip.Notes) == 0}
			if ip.LocalID > 0 {
				if old, err := st.AcqIndexer(ip.LocalID); err == nil {
					x.ID, x.Enabled = old.ID, old.Enabled
					if x.APIKey == "" {
						x.APIKey = old.APIKey // a key entered in Kinora survives a repeated import
					}
				}
			}
			if err := st.SaveAcqIndexer(x); err != nil {
				pr.fail(SecIndexers, ip.Name, err)
				continue
			}
			st.MapImported(p.source, "indexer", strconv.FormatInt(ip.ArrID, 10), x.ID)
			pr.step(SecIndexers, ip.Action)
		}
	}
	if want[SecTitles] {
		ready := d.Acquire != nil && d.Acquire.Enabled() && d.TMDB != nil && d.TMDB.Enabled()
		admin := firstAdmin(st)
		for _, t := range append(append([]*TitlePlan{}, p.Movies...), p.Series...) {
			if t.Action == ActSkip || t.Action == ActExisting {
				pr.step(SecTitles, t.Action)
				continue
			}
			if !ready {
				pr.fail(SecTitles, t.Title, errors.New(RsAcqOff))
				continue
			}
			cctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			err := applyTitle(cctx, p, d, t, libFor[t.Folder], profileIDs, admin)
			cancel()
			if err != nil {
				pr.fail(SecTitles, t.Title, err)
				continue
			}
			pr.step(SecTitles, t.Action)
		}
	}
}

func applyTitle(ctx context.Context, p *Plan, d Deps, t *TitlePlan, libID int64, profileIDs map[int64]int64, admin int64) error {
	if p.App == Radarr {
		_, err := d.Acquire.Request(ctx, acquire.Requester{UserID: admin, IsAdmin: true}, t.TmdbID, libID)
		if errors.Is(err, acquire.ErrHave) {
			return nil
		}
		return err
	}
	x, err := d.Acquire.AddSeries(ctx, acquire.SeriesInput{TmdbID: t.TmdbID, LibraryID: libID, Monitor: t.Monitor, Seasons: t.Seasons, UserID: admin})
	if err != nil {
		return err
	}
	if pid := profileIDs[t.ProfileID]; pid > 0 {
		d.Store.SetSeriesProfile(x.ID, pid)
	} else if pid := d.Store.ImportedID(p.source, "profile", strconv.FormatInt(t.ProfileID, 10)); pid > 0 {
		d.Store.SetSeriesProfile(x.ID, pid)
	}
	return nil
}

func profileNameTaken(st *store.Store, name string) bool {
	ps, _ := st.AcqProfiles()
	for _, p := range ps {
		if strings.EqualFold(p.Name, name) {
			return true
		}
	}
	return false
}

func firstAdmin(st *store.Store) int64 {
	us, _ := st.ListUsers()
	for _, u := range us {
		if u.IsAdmin {
			return u.ID
		}
	}
	return 0
}
