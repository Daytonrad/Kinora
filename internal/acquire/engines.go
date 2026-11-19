package acquire

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/ScotyLucas/kinora/internal/acquire/cardigann"
	"github.com/ScotyLucas/kinora/internal/acquire/sites"
	"github.com/ScotyLucas/kinora/internal/store"
)

// The built-in indexer engine (M5): an indexer of kind "cardigann" runs an uploaded definition instead of
// calling a Torznab API. Searching, scoring and downloading go through the same paths as Torznab results.

type engineEntry struct {
	key string // definition + settings: a change makes a new engine (and a new session)
	ix  sites.Engine
}

type engines struct {
	mu sync.Mutex
	m  map[int64]engineEntry
}

// engine returns the running engine of an indexer, signed-in session included: a built-in site (Go code)
// or an uploaded definition.
func (m *Manager) engine(ix Indexer) (sites.Engine, error) {
	site, builtin := sites.Builtin[ix.DefinitionID]
	var d *store.AcqDefinition
	key := "builtin|" + ix.Config
	if !builtin {
		var err error
		if d, err = m.st.AcqDefinition(ix.DefinitionID); err != nil {
			return nil, fmt.Errorf("a(z) %q leíró nincs feltöltve", ix.DefinitionID)
		}
		key = strconv.Itoa(len(d.YAML)) + "|" + strconv.FormatInt(d.CreatedAt, 10) + "|" + ix.Config
	}
	m.eng.mu.Lock()
	defer m.eng.mu.Unlock()
	if m.eng.m == nil {
		m.eng.m = map[int64]engineEntry{}
	}
	if e, ok := m.eng.m[ix.ID]; ok && e.key == key && ix.ID != 0 {
		return e.ix, nil
	}
	values := map[string]any{}
	if ix.Config != "" {
		if err := json.Unmarshal([]byte(ix.Config), &values); err != nil {
			return nil, errors.New("az indexer beállításai érvénytelenek")
		}
	}
	var e sites.Engine
	if builtin {
		e = site.New(values, m.hc)
	} else {
		def, err := cardigann.Parse([]byte(d.YAML))
		if err != nil {
			return nil, err
		}
		e = cardigann.New(def, values, m.hc)
	}
	if ix.ID != 0 {
		m.eng.m[ix.ID] = engineEntry{key: key, ix: e}
	}
	return e, nil
}

// searchIx asks one indexer, whatever its kind.
func (m *Manager) searchIx(ctx context.Context, ix Indexer, q Query) ([]Result, error) {
	if ix.Kind != "cardigann" {
		return m.tz.Search(ctx, ix, q)
	}
	e, err := m.engine(ix)
	if err != nil {
		return nil, err
	}
	cq := cardigann.Query{Type: "movie", Keywords: q.Title, IMDBID: q.IMDb, IMDBIDShort: strings.TrimPrefix(q.IMDb, "tt")}
	if q.Year > 0 {
		cq.Year = strconv.Itoa(q.Year)
	}
	if q.TV {
		cq.Type = "tv"
		if q.TVDB > 0 {
			cq.TVDBID = strconv.FormatInt(q.TVDB, 10)
		}
		if q.Season >= 0 {
			cq.Season = strconv.Itoa(q.Season)
			cq.Keywords += fmt.Sprintf(" S%02d", q.Season)
			if q.Episode >= 0 {
				cq.Ep = strconv.Itoa(q.Episode)
				cq.Keywords += fmt.Sprintf("E%02d", q.Episode)
			}
		}
	} else if q.Year > 0 && q.Title != "" {
		cq.Keywords += " " + cq.Year
	}
	cq.Q = cq.Keywords
	rows, err := e.Search(ctx, cq)
	if err != nil {
		return nil, err
	}
	return rowsToResults(ix, rows), nil
}

// latestIx is an indexer's newest releases (RSS for Torznab, an empty search for a definition).
func (m *Manager) latestIx(ctx context.Context, ix Indexer, tv bool) ([]Result, error) {
	if ix.Kind != "cardigann" {
		return m.tz.Latest(ctx, ix, tv)
	}
	e, err := m.engine(ix)
	if err != nil {
		return nil, err
	}
	t := "movie"
	if tv {
		t = "tv"
	}
	rows, err := e.Search(ctx, cardigann.Query{Type: t})
	if err != nil {
		return nil, err
	}
	return rowsToResults(ix, rows), nil
}

func rowsToResults(ix Indexer, rows []cardigann.Row) []Result {
	out := make([]Result, 0, len(rows))
	for _, r := range rows {
		res := Result{IndexerID: ix.ID, Indexer: ix.Name, Title: r.Title, GUID: firstNonEmpty(r.Details, r.Download, r.Magnet), Link: r.Download,
			Magnet: r.Magnet, InfoHash: r.InfoHash, IMDb: r.IMDb, Size: r.Size, Seeders: r.Seeders, Peers: r.Seeders + r.Leechers,
			Published: r.Date, Info: r.Details, Protocol: "torrent"}
		if res.InfoHash == "" && res.Magnet != "" {
			res.InfoHash, _ = MagnetHash(res.Magnet)
		}
		res.Release = ParseRelease(res.Title)
		if r.Language != "" && !containsStr(res.Release.Languages, r.Language) {
			res.Release.Languages = append(res.Release.Languages, r.Language)
		}
		res.Quality = res.Release.QualityLabel()
		out = append(out, res)
	}
	return out
}

func containsStr(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

// fetch gets a release for the download client: through the engine's signed-in session for a definition,
// over plain HTTP otherwise.
func (m *Manager) fetch(ctx context.Context, rel Result) (Fetched, error) {
	x, err := m.st.AcqIndexer(rel.IndexerID)
	if err != nil || x.Kind != "cardigann" {
		return FetchRelease(ctx, m.hc, rel)
	}
	e, err := m.engine(Indexer{ID: x.ID, Kind: x.Kind, Name: x.Name, DefinitionID: x.DefinitionID, Config: x.Config})
	if err != nil {
		return Fetched{}, err
	}
	link := rel.Link
	if link == "" {
		link = rel.Magnet
	}
	torrent, magnet, err := e.Download(ctx, link)
	if err != nil {
		return Fetched{}, err
	}
	if magnet != "" {
		h, ok := MagnetHash(magnet)
		if !ok {
			return Fetched{}, errors.New("érvénytelen magnet link")
		}
		return Fetched{Magnet: magnet, InfoHash: h, Name: rel.Title}, nil
	}
	h, err := InfoHash(torrent)
	if err != nil {
		return Fetched{}, err
	}
	return Fetched{Torrent: torrent, InfoHash: h, Name: rel.Title}, nil
}

// testIx checks an indexer configuration.
func (m *Manager) testIx(ctx context.Context, ix Indexer) (string, error) {
	if ix.Kind != "cardigann" {
		return m.tz.Caps(ctx, ix)
	}
	e, err := m.engine(ix)
	if err != nil {
		return "", err
	}
	n, err := e.Test(ctx)
	if err != nil {
		return "", err
	}
	name := ix.Name
	if s, ok := sites.Builtin[ix.DefinitionID]; ok {
		name = s.Def.Name
	} else if c, ok := e.(*cardigann.Indexer); ok {
		name = c.Def.Name
	}
	return fmt.Sprintf("%s · %d", name, n), nil
}
